package main

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Job is one piece of work shown in the progress strip (진행 막대 + 남은 시간).
type Job struct {
	id      string
	kind    string // index | read | ocr | ops | cleanup | auto
	title   string
	unit    string // 개 | 장 | MB
	quiet   bool   // 짧은 작업: 끝나도 오래 보여 주지 않음
	started time.Time

	mu       sync.Mutex
	state    string // running | done | error | canceled
	total    int64
	done     int64
	msg      string
	err      string
	finished time.Time
	result   any
	rate     float64 // 1초에 처리하는 양 (부드럽게)
	lastT    time.Time
	lastDone int64

	canceled atomic.Bool
}

type JobSnap struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Title    string  `json:"title"`
	Unit     string  `json:"unit"`
	State    string  `json:"state"`
	Total    int64   `json:"total"`
	Done     int64   `json:"done"`
	Pct      float64 `json:"pct"`
	ETA      float64 `json:"eta"` // 초, 0 = 모름
	Elapsed  float64 `json:"elapsed"`
	Msg      string  `json:"msg"`
	Err      string  `json:"err"`
	Started  int64   `json:"started"`
	Finished int64   `json:"finished"`
	Result   bool    `json:"hasResult"`
}

type JobManager struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

func NewJobManager() *JobManager { return &JobManager{jobs: map[string]*Job{}} }

func (m *JobManager) Start(kind, title, unit string, total int64) *Job {
	j := &Job{id: randID(6), kind: kind, title: title, unit: unit, total: total, state: "running", started: time.Now(), lastT: time.Now()}
	m.mu.Lock()
	m.jobs[j.id] = j
	m.mu.Unlock()
	return j
}

func (m *JobManager) Get(id string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs[id]
}

func (m *JobManager) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.State() == "running" && j.kind != "index" {
			return true
		}
	}
	return false
}

// BusyOps: 파일을 옮기는 작업이 진행 중인지 (업데이트 설치를 미뤄요)
func (m *JobManager) BusyOps() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.State() == "running" && (j.kind == "ops" || j.kind == "auto") {
			return true
		}
	}
	return false
}

func (m *JobManager) Cancel(id string) {
	if j := m.Get(id); j != nil {
		j.canceled.Store(true)
	}
}

func (m *JobManager) CancelAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		j.canceled.Store(true)
	}
}

func (m *JobManager) ClearFinished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, j := range m.jobs {
		if j.State() != "running" {
			delete(m.jobs, id)
		}
	}
}

// Snapshot returns running jobs and recently finished ones.
func (m *JobManager) Snapshot() []JobSnap {
	m.mu.Lock()
	list := make([]*Job, 0, len(m.jobs))
	now := time.Now()
	for id, j := range m.jobs {
		j.mu.Lock()
		old := j.state != "running" && now.Sub(j.finished) > 30*time.Minute
		j.mu.Unlock()
		if old {
			delete(m.jobs, id)
			continue
		}
		list = append(list, j)
	}
	m.mu.Unlock()
	sort.Slice(list, func(a, b int) bool { return list[a].started.Before(list[b].started) })
	out := make([]JobSnap, 0, len(list))
	for _, j := range list {
		s := j.Snap()
		if s.State != "running" && j.quiet && now.Sub(j.finished) > 8*time.Second {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (j *Job) ID() string { return j.id }

func (j *Job) State() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.state
}

func (j *Job) Canceled() bool { return j.canceled.Load() }

func (j *Job) SetTotal(n int64) {
	j.mu.Lock()
	j.total = n
	j.mu.Unlock()
}

func (j *Job) AddTotal(n int64) {
	j.mu.Lock()
	j.total += n
	j.mu.Unlock()
}

func (j *Job) Add(n int64) {
	j.mu.Lock()
	j.done += n
	j.tickRate()
	j.mu.Unlock()
}

func (j *Job) SetDone(n int64) {
	j.mu.Lock()
	j.done = n
	j.tickRate()
	j.mu.Unlock()
}

func (j *Job) SetMsg(s string) {
	j.mu.Lock()
	j.msg = s
	j.mu.Unlock()
}

func (j *Job) SetTitle(s string) {
	j.mu.Lock()
	j.title = s
	j.mu.Unlock()
}

// tickRate keeps a smoothed speed so the remaining time does not jump around.
func (j *Job) tickRate() {
	now := time.Now()
	dt := now.Sub(j.lastT).Seconds()
	if dt < 0.7 {
		return
	}
	r := float64(j.done-j.lastDone) / dt
	if j.rate == 0 {
		j.rate = r
	} else {
		j.rate = 0.75*j.rate + 0.25*r
	}
	j.lastT, j.lastDone = now, j.done
}

func (j *Job) Finish(err error, msg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state != "running" {
		return
	}
	j.finished = time.Now()
	switch {
	case j.canceled.Load():
		j.state = "canceled"
	case err != nil:
		j.state, j.err = "error", err.Error()
	default:
		j.state = "done"
		if j.total > 0 {
			j.done = j.total
		}
	}
	if msg != "" {
		j.msg = msg
	}
}

func (j *Job) SetResult(v any) {
	j.mu.Lock()
	j.result = v
	j.mu.Unlock()
}

func (j *Job) Result() any {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.result
}

func (j *Job) Snap() JobSnap {
	j.mu.Lock()
	defer j.mu.Unlock()
	s := JobSnap{ID: j.id, Kind: j.kind, Title: j.title, Unit: j.unit, State: j.state, Total: j.total, Done: j.done,
		Msg: j.msg, Err: j.err, Started: j.started.UnixMilli(), Result: j.result != nil}
	end := time.Now()
	if j.state != "running" {
		end = j.finished
		s.Finished = j.finished.UnixMilli()
	}
	s.Elapsed = end.Sub(j.started).Seconds()
	if j.total > 0 {
		s.Pct = float64(j.done) / float64(j.total) * 100
		if s.Pct > 100 {
			s.Pct = 100
		}
	}
	if j.state == "running" && j.total > 0 && j.done > 0 && s.Elapsed > 1.5 {
		left := float64(j.total - j.done)
		avg := float64(j.done) / s.Elapsed
		rate := avg
		if j.rate > 0 {
			rate = 0.5*avg + 0.5*j.rate
		}
		if rate > 0 {
			s.ETA = left / rate
		}
	}
	return s
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// 옮기기 · 복사 · 이름 바꾸기 · 휴지통 · 분류는 모두
//   1) 계획(미리 보기) → 2) 실행(진행 막대) → 3) 작업 기록(되돌리기) 순서로 해요.
// 같은 이름이 있으면 덮어쓰지 않고 "이름 (2)"로, 지우기는 언제나 휴지통으로.

type PlanItem struct {
	Src     string `json:"src"`
	Dst     string `json:"dst"`
	Name    string `json:"name"`
	NewName string `json:"newName,omitempty"`
	Group   string `json:"group,omitempty"` // 분류: 어떤 규칙/폴더로 가는지
	Note    string `json:"note,omitempty"`
	Skip    bool   `json:"skip,omitempty"`
	Size    int64  `json:"size"`
	Kind    string `json:"kind"`
	mod     time.Time
}

type Plan struct {
	ID      string     `json:"id"`
	Op      string     `json:"op"` // move | copy | rename | trash
	Title   string     `json:"title"`
	Items   []PlanItem `json:"items"`
	NewDirs []string   `json:"newDirs"`
	Count   int        `json:"count"`
	Skipped int        `json:"skipped"`
	Renamed int        `json:"renamed"` // 같은 이름 때문에 (2)가 붙는 수
	Bytes   int64      `json:"bytes"`
	Auto    bool       `json:"auto,omitempty"`
	created time.Time
}

type HistItem struct {
	From string `json:"from"`
	To   string `json:"to"`
	Act  string `json:"act"` // move | copy | trash
}

type History struct {
	ID       string     `json:"id"`
	Op       string     `json:"op"`
	Title    string     `json:"title"`
	Time     int64      `json:"time"`
	Items    []HistItem `json:"items"`
	Dirs     []string   `json:"dirs"`
	Count    int        `json:"count"`
	Failed   int        `json:"failed"`
	Errors   []string   `json:"errors,omitempty"`
	Auto     bool       `json:"auto,omitempty"`
	Undone   bool       `json:"undone,omitempty"`
	UndoneAt int64      `json:"undoneAt,omitempty"`
	UndoNote string     `json:"undoNote,omitempty"`
}

type Ops struct {
	support string
	mu      sync.Mutex
	plans   map[string]*Plan
	hist    []*History
	sig     atomic.Int64
	autoMu  sync.Mutex
}

func NewOps(support string) *Ops {
	o := &Ops{support: support, plans: map[string]*Plan{}}
	o.loadHistory()
	return o
}

func (o *Ops) histPath() string { return filepath.Join(o.support, "history.json") }

func (o *Ops) loadHistory() {
	b, err := os.ReadFile(o.histPath())
	if err == nil {
		json.Unmarshal(b, &o.hist)
	}
	o.sig.Store(time.Now().UnixNano())
}

func (o *Ops) saveHistory() {
	o.mu.Lock()
	if len(o.hist) > 300 {
		o.hist = o.hist[len(o.hist)-300:]
	}
	b, _ := json.Marshal(o.hist)
	o.mu.Unlock()
	writeFileAtomic(o.histPath(), b, 0o644)
	o.sig.Add(1)
}

func (o *Ops) Count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.hist)
}

func (o *Ops) Sig() int64 { return o.sig.Load() }

// ---------------------------------------------------------------- 계획 만들기

type RenameOpts struct {
	Find      string `json:"find"`
	Replace   string `json:"replace"`
	Base      string `json:"base"` // 새 이름 (비우면 원래 이름)
	Prefix    string `json:"prefix"`
	Suffix    string `json:"suffix"`
	Number    bool   `json:"number"`
	NumStart  int    `json:"numStart"`
	NumDigits int    `json:"numDigits"`
	NumPos    string `json:"numPos"` // end | start
	Date      string `json:"date"`   // "" | ymd | y-m-d — 앞에 수정 날짜
	Case      string `json:"case"`   // "" | lower | upper (영문)
	Spaces    string `json:"spaces"` // "" | underscore | remove
}

type PlanReq struct {
	Op    string   `json:"op"` // move | copy | rename | trash | organize
	Paths []string `json:"paths"`
	Pairs []struct {
		Src string `json:"src"`
		Dir string `json:"dir"`
	} `json:"pairs"` // 파일마다 다른 폴더로 옮길 때
	Dest     string       `json:"dest"`
	Rename   RenameOpts   `json:"rename"`
	Organize OrganizeOpts `json:"organize"`
}

func (o *Ops) handlePlan(w http.ResponseWriter, r *http.Request) {
	var req PlanReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, 400, "bad request")
		return
	}
	p, err := o.BuildPlan(req)
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "plan": p})
}

func (o *Ops) store(p *Plan) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for id, x := range o.plans {
		if time.Since(x.created) > time.Hour {
			delete(o.plans, id)
		}
	}
	o.plans[p.ID] = p
}

func (o *Ops) BuildPlan(req PlanReq) (*Plan, error) {
	paths := dedupe(req.Paths)
	p := &Plan{ID: randID(6), Op: req.Op, created: time.Now(), Items: []PlanItem{}, NewDirs: []string{}}
	taken := map[string]bool{}
	newDirs := map[string]bool{}
	addDst := func(it *PlanItem, dir, name string) {
		it.Dst = uniquePathExcept(dir, name, taken)
		taken[strings.ToLower(it.Dst)] = true
		if base := filepath.Base(it.Dst); base != name {
			it.Note = fmt.Sprintf("같은 이름이 있어서 ‘%s’(으)로 바꿔요", nfc(base))
			p.Renamed++
		}
		markNewDirs(dir, newDirs)
	}
	switch req.Op {
	case "move", "copy":
		if len(req.Pairs) > 0 {
			for _, pr := range req.Pairs {
				it := baseItem(pr.Src)
				if it.Skip {
					p.Items = append(p.Items, it)
					continue
				}
				dir := filepath.Clean(pr.Dir)
				if reason := badDest(pr.Src, dir, req.Op); reason != "" {
					it.Skip, it.Note = true, reason
				} else {
					addDst(&it, dir, filepath.Base(pr.Src))
					it.Group = nfc(filepath.Base(dir))
				}
				p.Items = append(p.Items, it)
			}
			break
		}
		dest := filepath.Clean(strings.TrimSpace(req.Dest))
		if req.Dest == "" {
			return nil, errors.New("옮길 폴더를 골라 주세요")
		}
		if st, err := os.Stat(dest); err == nil && !st.IsDir() {
			return nil, errors.New("고른 곳이 폴더가 아니에요")
		}
		for _, src := range paths {
			it := baseItem(src)
			if !it.Skip {
				if reason := badDest(src, dest, req.Op); reason != "" {
					it.Skip, it.Note = true, reason
				} else {
					addDst(&it, dest, filepath.Base(src))
					it.Group = folderLabel(dest)
				}
			}
			p.Items = append(p.Items, it)
		}
		verb := "옮기기"
		if req.Op == "copy" {
			verb = "복사"
		}
		p.Title = fmt.Sprintf("‘%s’ 폴더로 %s", folderLabel(dest), verb)
	case "rename":
		ro := req.Rename
		if ro.NumStart == 0 {
			ro.NumStart = 1
		}
		if ro.NumDigits <= 0 {
			ro.NumDigits = 2
		}
		for i, src := range paths {
			it := baseItem(src)
			if it.Skip {
				p.Items = append(p.Items, it)
				continue
			}
			newName := applyRename(src, ro, i)
			it.NewName = newName
			if newName == nfc(filepath.Base(src)) {
				it.Skip, it.Note = true, "이름이 그대로예요"
				p.Items = append(p.Items, it)
				continue
			}
			dir := filepath.Dir(src)
			cand := filepath.Join(dir, newName)
			if strings.EqualFold(cand, src) {
				// 대소문자만 바뀌는 경우
				it.Dst = cand
				taken[strings.ToLower(cand)] = true
			} else {
				addDst(&it, dir, newName)
			}
			p.Items = append(p.Items, it)
		}
		p.Title = "이름 한꺼번에 바꾸기"
	case "trash":
		for _, src := range paths {
			it := baseItem(src)
			p.Items = append(p.Items, it)
		}
		p.Title = "휴지통으로 옮기기"
	case "organize":
		items, title, err := planOrganize(req.Organize, paths)
		if err != nil {
			return nil, err
		}
		for _, x := range items {
			it := baseItem(x.src)
			if !it.Skip {
				if x.skip != "" {
					it.Skip, it.Note = true, x.skip
				} else if reason := badDest(x.src, x.dir, "move"); reason != "" {
					it.Skip, it.Note = true, reason
				} else {
					addDst(&it, x.dir, filepath.Base(x.src))
					it.Group = x.group
				}
			}
			p.Items = append(p.Items, it)
		}
		p.Op = "move"
		if req.Organize.Copy {
			p.Op = "copy"
		}
		p.Title = title
	default:
		return nil, errors.New("알 수 없는 작업이에요")
	}
	for d := range newDirs {
		p.NewDirs = append(p.NewDirs, d)
	}
	sort.Strings(p.NewDirs)
	for _, it := range p.Items {
		if it.Skip {
			p.Skipped++
		} else {
			p.Count++
			p.Bytes += it.Size
		}
	}
	o.store(p)
	return p, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range in {
		p = filepath.Clean(p)
		if p == "" || p == "." || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func baseItem(src string) PlanItem {
	it := PlanItem{Src: src, Name: nfc(filepath.Base(src))}
	if k := kindOf(src); k != nil {
		it.Kind = k.ID
	}
	st, err := os.Lstat(src)
	if err != nil {
		it.Skip, it.Note = true, "파일을 찾을 수 없어요"
		return it
	}
	if st.IsDir() {
		size, _ := pkgStat(src)
		it.Size = size
	} else {
		it.Size = st.Size()
	}
	it.mod = st.ModTime()
	return it
}

// badDest explains why src cannot go to dir (or "" when fine).
func badDest(src, dir, op string) string {
	if dir == "" || dir == "." {
		return "옮길 폴더가 정해지지 않았어요"
	}
	if op == "move" && filepath.Dir(src) == dir {
		return "이미 이 폴더에 있어요"
	}
	if isUnder(dir, src) {
		return "폴더를 자기 안으로 옮길 수 없어요"
	}
	return ""
}

func markNewDirs(dir string, m map[string]bool) {
	for d := dir; d != "" && d != "/" && d != "."; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil {
			return
		}
		m[d] = true
		if filepath.Dir(d) == d {
			return
		}
	}
}

func safeName(s string) string {
	s = strings.NewReplacer("/", "-", ":", "-", "\x00", "").Replace(s)
	return strings.TrimSpace(s)
}

func applyRename(src string, ro RenameOpts, i int) string {
	name := nfc(filepath.Base(src))
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if ro.Find != "" {
		stem = strings.ReplaceAll(stem, nfc(ro.Find), nfc(ro.Replace))
	}
	if b := strings.TrimSpace(ro.Base); b != "" {
		stem = strings.ReplaceAll(nfc(b), "{이름}", stem)
	}
	switch ro.Spaces {
	case "underscore":
		stem = strings.Join(strings.Fields(stem), "_")
	case "remove":
		stem = strings.Join(strings.Fields(stem), "")
	}
	switch ro.Case {
	case "lower":
		stem = strings.ToLower(stem)
	case "upper":
		stem = strings.ToUpper(stem)
	}
	stem = nfc(ro.Prefix) + stem + nfc(ro.Suffix)
	if ro.Number {
		n := fmt.Sprintf("%0*d", ro.NumDigits, ro.NumStart+i)
		if ro.NumPos == "start" {
			stem = n + "_" + stem
		} else {
			stem = stem + "_" + n
		}
	}
	if ro.Date != "" {
		if st, err := os.Stat(src); err == nil {
			t := st.ModTime()
			if ro.Date == "y-m-d" {
				stem = t.Format("2006-01-02") + "_" + stem
			} else {
				stem = t.Format("20060102") + "_" + stem
			}
		}
	}
	stem = safeName(stem)
	if stem == "" {
		stem = "이름 없음"
	}
	return stem + ext
}

// ---------------------------------------------------------------- 실행

func (o *Ops) handleRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID   string `json:"id"`
		Skip []int  `json:"skip"` // 미리 보기에서 뺀 항목
	}
	readJSON(r, &in)
	o.mu.Lock()
	p := o.plans[in.ID]
	delete(o.plans, in.ID)
	o.mu.Unlock()
	if p == nil {
		jsonError(w, 400, "미리 보기가 오래됐어요. 다시 시도해 주세요")
		return
	}
	for _, i := range in.Skip {
		if i >= 0 && i < len(p.Items) {
			p.Items[i].Skip = true
		}
	}
	job := o.Run(p)
	writeJSON(w, map[string]any{"ok": true, "job": job.ID()})
}

func (o *Ops) Run(p *Plan) *Job {
	n := 0
	for _, it := range p.Items {
		if !it.Skip {
			n++
		}
	}
	kind := "ops"
	if p.Auto {
		kind = "auto"
	}
	job := app.jobs.Start(kind, p.Title, "개", int64(n))
	go o.execute(p, job)
	return job
}

func (o *Ops) execute(p *Plan, job *Job) {
	h := &History{ID: randID(6), Op: p.Op, Title: p.Title, Time: time.Now().Unix(), Auto: p.Auto, Items: []HistItem{}, Dirs: []string{}}
	created := map[string]bool{}
	ensureDir := func(dir string) error {
		missing := []string{}
		for d := dir; d != "" && d != "/" && d != "."; d = filepath.Dir(d) {
			if _, err := os.Stat(d); err == nil {
				break
			}
			missing = append(missing, d)
			if filepath.Dir(d) == d {
				break
			}
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for _, d := range missing {
			if !created[d] {
				created[d] = true
				h.Dirs = append(h.Dirs, d)
			}
		}
		return nil
	}
	fail := func(it PlanItem, err error) {
		h.Failed++
		if len(h.Errors) < 20 {
			h.Errors = append(h.Errors, fmt.Sprintf("%s: %s", it.Name, friendlyFSErr(err)))
		}
	}
	slow := atoiDef(os.Getenv("LIBROAD_TEST_SLOW"), 0)

	if p.Op == "trash" {
		list := []PlanItem{}
		for _, it := range p.Items {
			if !it.Skip {
				list = append(list, it)
			}
		}
		for i := 0; i < len(list) && !job.Canceled(); i += 50 {
			batch := list[i:min(i+50, len(list))]
			res := trashPaths(batch, job)
			for _, it := range batch {
				r := res[it.Src]
				if r.err != nil {
					fail(it, r.err)
					continue
				}
				h.Items = append(h.Items, HistItem{From: it.Src, To: r.to, Act: "trash"})
				h.Count++
				app.lib.Removed(it.Src)
			}
		}
	} else {
		for _, it := range p.Items {
			if it.Skip {
				continue
			}
			if job.Canceled() {
				break
			}
			if slow > 0 {
				time.Sleep(time.Duration(slow) * time.Millisecond)
			}
			job.SetMsg(it.Name)
			if _, err := os.Lstat(it.Src); err != nil {
				fail(it, errors.New("파일을 찾을 수 없어요"))
				job.Add(1)
				continue
			}
			dst := it.Dst
			dir := filepath.Dir(dst)
			if err := ensureDir(dir); err != nil {
				fail(it, err)
				job.Add(1)
				continue
			}
			caseOnly := strings.EqualFold(dst, it.Src) && dst != it.Src
			if !caseOnly && fileExists(dst) {
				// 계획 뒤에 같은 이름이 생겼다면 다시 (2)
				dst = uniquePath(dir, filepath.Base(dst))
			}
			var err error
			switch p.Op {
			case "copy":
				err = copyTree(it.Src, dst)
			default:
				if caseOnly {
					tmp := it.Src + ".libroad-" + randID(3)
					if err = os.Rename(it.Src, tmp); err == nil {
						err = os.Rename(tmp, dst)
					}
				} else {
					err = moveTree(it.Src, dst)
				}
			}
			if err != nil {
				fail(it, err)
				job.Add(1)
				continue
			}
			act := "move"
			if p.Op == "copy" {
				act = "copy"
			} else {
				app.lib.Moved(it.Src, dst)
			}
			h.Items = append(h.Items, HistItem{From: it.Src, To: dst, Act: act})
			h.Count++
			job.Add(1)
		}
	}
	if h.Count > 0 || h.Failed > 0 {
		o.mu.Lock()
		o.hist = append(o.hist, h)
		o.mu.Unlock()
		o.saveHistory()
	}
	msg := fmt.Sprintf("%d개 완료", h.Count)
	if h.Failed > 0 {
		msg += fmt.Sprintf(" · %d개 실패", h.Failed)
	}
	var err error
	if h.Count == 0 && h.Failed > 0 {
		err = errors.New(h.Errors[0])
	}
	job.SetResult(map[string]any{"history": h.ID, "count": h.Count, "failed": h.Failed, "errors": h.Errors})
	job.Finish(err, msg)
	if p.Op == "copy" || len(h.Dirs) > 0 {
		app.lib.Kick()
	}
	if p.Auto && h.Count > 0 {
		notify("LIBROAD", fmt.Sprintf("다운로드 폴더의 문서 %d개를 정리했어요", h.Count))
	}
}

func friendlyFSErr(err error) string {
	switch {
	case errors.Is(err, os.ErrPermission):
		return "권한이 없어요"
	case errors.Is(err, syscall.ENOSPC):
		return "디스크 공간이 부족해요"
	case errors.Is(err, os.ErrNotExist):
		return "파일을 찾을 수 없어요"
	case errors.Is(err, syscall.EROFS):
		return "읽기 전용 디스크예요"
	}
	s := err.Error()
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

// moveTree renames, or copies then removes when the target is on another disk.
func moveTree(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	var le *os.LinkError
	if errors.As(err, &le) && errors.Is(le.Err, syscall.EXDEV) {
		if err := copyTree(src, dst); err != nil {
			os.RemoveAll(dst)
			return err
		}
		return os.RemoveAll(src)
	}
	return err
}

func copyTree(src, dst string) error {
	st, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return copyFile(src, dst, st)
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		}
		if d.Type()&os.ModeSymlink != 0 {
			if l, err := os.Readlink(p); err == nil {
				return os.Symlink(l, target)
			}
			return nil
		}
		return copyFile(p, target, info)
	})
}

func copyFile(src, dst string, st os.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, st.Mode().Perm()|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	os.Chtimes(dst, st.ModTime(), st.ModTime())
	return nil
}

// ---------------------------------------------------------------- 휴지통

type trashRes struct {
	to  string
	err error
}

// trashPaths moves files to the Trash. On a Mac it uses macOS' own Trash (Finder의 '되돌려 놓기'도 돼요).
func trashPaths(items []PlanItem, job *Job) map[string]trashRes {
	out := map[string]trashRes{}
	if t := os.Getenv("LIBROAD_TRASH"); t != "" {
		os.MkdirAll(t, 0o755)
		for _, it := range items {
			dst := uniquePath(t, filepath.Base(it.Src))
			err := moveTree(it.Src, dst)
			out[it.Src] = trashRes{to: dst, err: err}
			job.Add(1)
		}
		return out
	}
	if helperAvailable() {
		paths := []string{}
		byPath := map[string]PlanItem{}
		for _, it := range items {
			paths = append(paths, it.Src)
			byPath[it.Src] = it
		}
		runHelper("trash", paths, nil, 60*time.Second, nil, func(h helperLine) {
			if h.Path == "" {
				return
			}
			if h.OK {
				to := h.Trashed
				if to == "" || !fileExists(to) {
					to = findInTrash(h.Path, byPath[h.Path])
				}
				out[h.Path] = trashRes{to: to}
			} else {
				out[h.Path] = trashRes{err: errors.New(h.Error)}
			}
			job.Add(1)
		})
	}
	// 도우미가 못 한 것은 직접 ~/.Trash 로 (같은 디스크일 때)
	home, _ := os.UserHomeDir()
	for _, it := range items {
		if r, ok := out[it.Src]; ok && r.err == nil {
			continue
		}
		if !fileExists(it.Src) {
			if _, ok := out[it.Src]; !ok {
				out[it.Src] = trashRes{err: errors.New("파일을 찾을 수 없어요")}
				job.Add(1)
			}
			continue
		}
		dst := uniquePath(filepath.Join(home, ".Trash"), filepath.Base(it.Src))
		err := os.Rename(it.Src, dst)
		if err != nil {
			log.Printf("trash %s: %v", it.Src, err)
			err = errors.New("휴지통으로 옮기지 못했어요")
		}
		if _, counted := out[it.Src]; !counted {
			job.Add(1)
		}
		out[it.Src] = trashRes{to: dst, err: err}
	}
	return out
}

// findInTrash looks for the file macOS just put in ~/.Trash ("이름.pdf", "이름 2.pdf" …)
// and only trusts a file with the same size and time, so 되돌리기 never brings back a different file.
func findInTrash(orig string, it PlanItem) string {
	home, _ := os.UserHomeDir()
	trash := filepath.Join(home, ".Trash")
	name := filepath.Base(orig)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	cands := []string{filepath.Join(trash, name)}
	for n := 2; n <= 30; n++ {
		cands = append(cands, filepath.Join(trash, fmt.Sprintf("%s %d%s", stem, n, ext)))
	}
	for i := len(cands) - 1; i >= 0; i-- { // 가장 최근에 생긴 이름부터
		st, err := os.Lstat(cands[i])
		if err != nil {
			continue
		}
		if st.IsDir() || (st.Size() == it.Size && st.ModTime().Equal(it.mod)) {
			return cands[i]
		}
	}
	return ""
}

// ---------------------------------------------------------------- 작업 기록 · 되돌리기

func (o *Ops) handleHistory(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	list := make([]History, 0, len(o.hist))
	for i := len(o.hist) - 1; i >= 0; i-- {
		h := *o.hist[i]
		if len(h.Items) > 60 {
			h.Items = h.Items[:60]
		}
		list = append(list, h)
	}
	o.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "items": list})
}

func (o *Ops) handleUndo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	readJSON(r, &in)
	o.mu.Lock()
	var h *History
	for _, x := range o.hist {
		if x.ID == in.ID {
			h = x
		}
	}
	o.mu.Unlock()
	if h == nil {
		jsonError(w, 404, "작업 기록을 찾을 수 없어요")
		return
	}
	if h.Undone {
		jsonError(w, 400, "이미 되돌린 작업이에요")
		return
	}
	job := app.jobs.Start("ops", "되돌리기: "+h.Title, "개", int64(len(h.Items)))
	go o.undo(h, job)
	writeJSON(w, map[string]any{"ok": true, "job": job.ID()})
}

func (o *Ops) undo(h *History, job *Job) {
	done, failed, lost := 0, 0, 0
	toTrash := []PlanItem{}
	for i := len(h.Items) - 1; i >= 0; i-- {
		it := h.Items[i]
		if job.Canceled() {
			break
		}
		switch it.Act {
		case "copy":
			if fileExists(it.To) {
				toTrash = append(toTrash, PlanItem{Src: it.To, Name: filepath.Base(it.To)})
			} else {
				job.Add(1)
			}
			continue
		case "move", "trash":
			if it.To == "" || !fileExists(it.To) {
				lost++
				job.Add(1)
				continue
			}
			back := it.From
			if fileExists(back) {
				back = uniquePath(filepath.Dir(it.From), filepath.Base(it.From))
			}
			os.MkdirAll(filepath.Dir(back), 0o755)
			var err error
			if strings.EqualFold(back, it.To) && back != it.To {
				tmp := it.To + ".libroad-" + randID(3)
				if err = os.Rename(it.To, tmp); err == nil {
					err = os.Rename(tmp, back)
				}
			} else {
				err = moveTree(it.To, back)
			}
			if err != nil {
				failed++
			} else {
				done++
				if it.Act == "move" {
					app.lib.Moved(it.To, back)
				}
			}
			job.Add(1)
		}
	}
	if len(toTrash) > 0 {
		res := trashPaths(toTrash, job)
		for _, it := range toTrash {
			if res[it.Src].err == nil {
				done++
				app.lib.Removed(it.Src)
			} else {
				failed++
			}
		}
	}
	// 이 작업이 만든 빈 폴더 정리 (깊은 폴더부터)
	dirs := append([]string{}, h.Dirs...)
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		removeIfEmpty(d)
	}
	note := ""
	if lost > 0 {
		note = fmt.Sprintf("%d개는 찾을 수 없었어요 (휴지통을 비웠거나 다시 옮겼을 수 있어요)", lost)
	}
	o.mu.Lock()
	h.Undone = true
	h.UndoneAt = time.Now().Unix()
	h.UndoNote = note
	o.mu.Unlock()
	o.saveHistory()
	msg := fmt.Sprintf("%d개를 되돌렸어요", done)
	if failed > 0 {
		msg += fmt.Sprintf(" · %d개 실패", failed)
	}
	if note != "" {
		msg += " · " + note
	}
	job.Finish(nil, msg)
	app.lib.Kick()
}

func removeIfEmpty(d string) {
	ents, err := os.ReadDir(d)
	if err != nil {
		return
	}
	for _, e := range ents {
		if e.Name() != ".DS_Store" && e.Name() != ".localized" {
			return
		}
	}
	for _, e := range ents {
		os.Remove(filepath.Join(d, e.Name()))
	}
	os.Remove(d)
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 분류하기: 종류별 · 날짜별 · 나만의 규칙으로 폴더를 나눠 옮겨요. 다운로드 폴더 자동 정리도 여기서 해요.

type Rule struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	On     bool     `json:"on"`
	Kinds  []string `json:"kinds"`
	Words  string   `json:"words"` // 쉼표로 구분, 하나라도 이름에 있으면
	Days   int      `json:"days"`  // 0 = 상관없음, N = N일보다 오래된 문서
	Target string   `json:"target"`
}

type AutoOrganize struct {
	Enabled   bool   `json:"enabled"`
	Source    string `json:"source"`
	Dest      string `json:"dest"`
	Mode      string `json:"mode"` // kind | kinddate | date | rules
	Rest      string `json:"rest"` // rules 모드에서 규칙에 안 맞는 문서: stay | kind
	Every     int    `json:"every"`
	MinAge    int    `json:"minAge"` // 분: 막 내려받은 파일은 건드리지 않아요
	LastRun   int64  `json:"lastRun"`
	LastCount int    `json:"lastCount"`
}

type OrganizeOpts struct {
	Mode      string `json:"mode"`
	Dest      string `json:"dest"`
	Source    string `json:"source"`
	Recursive bool   `json:"recursive"`
	Rest      string `json:"rest"`
	Copy      bool   `json:"copy"`
	MinAge    int    `json:"-"`
}

func defaultRules() []Rule {
	return []Rule{
		{ID: randID(4), Name: "영수증 · 세금", On: true, Words: "영수증, 세금계산서, 계산서, 견적서, 거래명세서, 청구서, invoice, receipt", Target: "영수증 · 세금/{연도}"},
		{ID: randID(4), Name: "계약서", On: true, Words: "계약서, 계약, 협약서, 동의서, 약정서", Target: "계약서"},
		{ID: randID(4), Name: "증명서 · 이력서", On: true, Words: "증명서, 이력서, 자기소개서, 경력, 재직, 졸업, 성적", Target: "증명서 · 이력서"},
		{ID: randID(4), Name: "발표 자료", On: true, Kinds: []string{"ppt", "keynote"}, Target: "발표 자료"},
	}
}

func fixAuto(a *AutoOrganize) {
	if a.Source == "" {
		a.Source = downloadsFolder()
	}
	if a.Dest == "" {
		a.Dest = filepath.Join(documentsFolder(), "LIBROAD 정리")
	}
	switch a.Mode {
	case "kind", "kinddate", "date", "rules":
	default:
		a.Mode = "kind"
	}
	if a.Rest != "stay" {
		a.Rest = "kind"
	}
	if a.Every <= 0 {
		a.Every = 30
	}
	if a.MinAge <= 0 {
		a.MinAge = 10
	}
}

type orgItem struct {
	src, dir, group, skip string
}

func kindFolder(k string) string {
	if kk := kindByID[k]; kk != nil {
		return strings.ReplaceAll(kk.Label, " · ", "·")
	}
	return "기타"
}

func expandTarget(t string, kind string, mod time.Time) string {
	r := strings.NewReplacer("{종류}", kindFolder(kind), "{연도}", mod.Format("2006"), "{월}", mod.Format("01")+"월")
	return r.Replace(t)
}

func ruleMatch(r Rule, name, kind string, mod time.Time) bool {
	if !r.On || (len(r.Kinds) == 0 && strings.TrimSpace(r.Words) == "") {
		return false
	}
	if len(r.Kinds) > 0 {
		ok := false
		for _, k := range r.Kinds {
			if k == kind {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	if w := strings.TrimSpace(r.Words); w != "" {
		low := lowerASCII(nfc(name))
		ok := false
		for _, x := range strings.FieldsFunc(w, func(c rune) bool { return c == ',' || c == '，' || c == '\n' }) {
			x = lowerASCII(nfc(strings.TrimSpace(x)))
			if x != "" && strings.Contains(low, x) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if r.Days > 0 && time.Since(mod) < time.Duration(r.Days)*24*time.Hour {
		return false
	}
	return true
}

// listSource returns the documents in a folder (top level, or all levels when recursive).
func listSource(src string, recursive bool, minAge int) ([]string, error) {
	st, err := os.Stat(src)
	if err != nil || !st.IsDir() {
		return nil, errors.New("정리할 폴더를 찾을 수 없어요")
	}
	out := []string{}
	cutoff := time.Now().Add(-time.Duration(minAge) * time.Minute)
	add := func(p string, info os.FileInfo) {
		if kindOf(p) == nil || strings.HasPrefix(filepath.Base(p), ".") || strings.HasPrefix(filepath.Base(p), "~$") {
			return
		}
		if minAge > 0 && info.ModTime().After(cutoff) {
			return
		}
		out = append(out, p)
	}
	if !recursive {
		ents, err := os.ReadDir(src)
		if err != nil {
			return nil, errors.New("폴더를 열 수 없어요. 접근을 허용해 주세요")
		}
		for _, e := range ents {
			p := filepath.Join(src, e.Name())
			if e.IsDir() && !isPackageExt(extOf(e.Name())) {
				continue
			}
			if info, err := e.Info(); err == nil && (info.Mode().IsRegular() || e.IsDir()) {
				add(p, info)
			}
		}
		return out, nil
	}
	files, _ := (&Library{support: app.support}).walkRoots([]string{src}, func(int) {})
	for p := range files {
		if info, err := os.Stat(p); err == nil {
			add(p, info)
		}
	}
	return out, nil
}

func planOrganize(o OrganizeOpts, paths []string) ([]orgItem, string, error) {
	dest := filepath.Clean(strings.TrimSpace(o.Dest))
	if o.Dest == "" {
		return nil, "", errors.New("정리한 문서를 둘 폴더를 골라 주세요")
	}
	if len(paths) == 0 {
		if o.Source == "" {
			return nil, "", errors.New("정리할 문서나 폴더를 골라 주세요")
		}
		list, err := listSource(o.Source, o.Recursive, o.MinAge)
		if err != nil {
			return nil, "", err
		}
		paths = list
	}
	rules := app.Settings().Rules
	items := []orgItem{}
	for _, p := range paths {
		k := kindOf(p)
		kid := "etc"
		if k != nil {
			kid = k.ID
		}
		mod := time.Now()
		if st, err := os.Stat(p); err == nil {
			mod = st.ModTime()
		}
		name := filepath.Base(p)
		it := orgItem{src: p}
		switch o.Mode {
		case "date":
			it.dir = filepath.Join(dest, mod.Format("2006")+"년", mod.Format("01")+"월")
			it.group = mod.Format("2006년 01월")
		case "kinddate":
			it.dir = filepath.Join(dest, kindFolder(kid), mod.Format("2006")+"년")
			it.group = kindFolder(kid) + " / " + mod.Format("2006") + "년"
		case "rules":
			matched := false
			for _, r := range rules {
				if ruleMatch(r, name, kid, mod) {
					t := expandTarget(r.Target, kid, mod)
					if t == "" {
						t = r.Name
					}
					if !filepath.IsAbs(t) {
						t = filepath.Join(dest, t)
					}
					it.dir = filepath.Clean(t)
					it.group = r.Name
					matched = true
					break
				}
			}
			if !matched {
				if o.Rest == "kind" {
					it.dir = filepath.Join(dest, kindFolder(kid))
					it.group = kindFolder(kid) + " (규칙 밖)"
				} else {
					it.skip = "맞는 규칙이 없어서 그대로 둬요"
				}
			}
		default: // kind
			it.dir = filepath.Join(dest, kindFolder(kid))
			it.group = kindFolder(kid)
		}
		items = append(items, it)
	}
	title := map[string]string{"kind": "종류별로 분류", "date": "날짜별로 분류", "kinddate": "종류 + 연도별로 분류", "rules": "규칙대로 분류"}[o.Mode]
	if title == "" {
		title = "종류별로 분류"
	}
	return items, title, nil
}

// ---------------------------------------------------------------- 규칙 · 자동 정리 설정

func (a *App) handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in struct {
			Rules []Rule `json:"rules"`
			Reset bool   `json:"reset"`
		}
		if err := readJSON(r, &in); err != nil {
			jsonError(w, 400, "bad request")
			return
		}
		a.update(func(s *Settings) {
			if in.Reset {
				s.Rules = defaultRules()
				return
			}
			out := []Rule{}
			for _, x := range in.Rules {
				if x.ID == "" {
					x.ID = randID(4)
				}
				x.Name = strings.TrimSpace(x.Name)
				if x.Name == "" {
					x.Name = "새 규칙"
				}
				out = append(out, x)
			}
			s.Rules = out
		})
	}
	writeJSON(w, map[string]any{"ok": true, "rules": a.Settings().Rules})
}

func (a *App) handleAuto(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in map[string]json.RawMessage
		if err := readJSON(r, &in); err != nil {
			jsonError(w, 400, "bad request")
			return
		}
		a.update(func(s *Settings) {
			au := &s.Auto
			for k, v := range in {
				switch k {
				case "enabled":
					json.Unmarshal(v, &au.Enabled)
				case "source":
					json.Unmarshal(v, &au.Source)
				case "dest":
					json.Unmarshal(v, &au.Dest)
				case "mode":
					json.Unmarshal(v, &au.Mode)
				case "rest":
					json.Unmarshal(v, &au.Rest)
				case "every":
					json.Unmarshal(v, &au.Every)
				case "minAge":
					json.Unmarshal(v, &au.MinAge)
				}
			}
		})
	}
	writeJSON(w, map[string]any{"ok": true, "auto": a.Settings().Auto})
}

func (o *Ops) autoLoop() {
	tick := time.Minute
	if s := atoiDef(os.Getenv("LIBROAD_AUTO_EVERY"), 0); s > 0 {
		tick = time.Duration(s) * time.Second
	}
	time.Sleep(20 * time.Second)
	for {
		s := app.Settings()
		au := s.Auto
		every := time.Duration(au.Every) * time.Minute
		if os.Getenv("LIBROAD_AUTO_EVERY") != "" {
			every = tick
		}
		if au.Enabled && time.Since(time.Unix(au.LastRun, 0)) >= every {
			if _, err := o.RunAutoNow(false); err != nil {
				_ = err
			}
		}
		time.Sleep(tick)
	}
}

// RunAutoNow organizes the watched folder (보통 다운로드 폴더) once.
func (o *Ops) RunAutoNow(manual bool) (string, error) {
	o.autoMu.Lock()
	defer o.autoMu.Unlock()
	if app.jobs.BusyOps() {
		return "", errors.New("다른 작업이 끝난 뒤에 다시 시도해 주세요")
	}
	au := app.Settings().Auto
	minAge := au.MinAge
	if manual {
		minAge = 1
	}
	p, err := o.BuildPlan(PlanReq{Op: "organize", Organize: OrganizeOpts{Mode: au.Mode, Dest: au.Dest, Source: au.Source, Rest: au.Rest, MinAge: minAge}})
	app.update(func(s *Settings) { s.Auto.LastRun = time.Now().Unix() })
	if err != nil {
		return "", err
	}
	o.mu.Lock()
	delete(o.plans, p.ID)
	o.mu.Unlock()
	if p.Count == 0 {
		app.update(func(s *Settings) { s.Auto.LastCount = 0 })
		if manual {
			return "", errors.New("지금은 정리할 문서가 없어요")
		}
		return "", nil
	}
	p.Auto = true
	p.Title = fmt.Sprintf("‘%s’ 폴더 자동 정리", folderLabel(au.Source))
	job := o.Run(p)
	app.update(func(s *Settings) { s.Auto.LastCount = p.Count })
	return job.ID(), nil
}

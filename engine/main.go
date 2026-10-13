// LIBROAD — 내 문서 도서관 (macOS)
// 문서를 찾고 · 분류하고 · 옮기고 · 정리하는 앱. 모든 작업은 이 컴퓨터 안에서만 해요.
// made by. Nevertheless_D
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// build is the app version, set with -ldflags "-X main.build=1.2.3".
var build = "0.0.0-dev"

//go:embed web
var webFiles embed.FS

//go:embed helper.js
var helperJS []byte

type SavedSearch struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Query  json.RawMessage `json:"query"`
	Create int64           `json:"created"`
}

type Settings struct {
	Appearance string        `json:"appearance"` // system | light | dark
	Roots      []string      `json:"roots"`      // 서재(검색 대상) 폴더
	OCR        bool          `json:"ocr"`        // 스캔 문서 · 사진 글자 읽기
	Notify     bool          `json:"notify"`
	AutoUpdate bool          `json:"autoUpdate"`
	Rules      []Rule        `json:"rules"`
	Auto       AutoOrganize  `json:"auto"`
	Basket     []string      `json:"basket"`
	Saved      []SavedSearch `json:"saved"`
	LastDest   string        `json:"lastDest"`
	Seen       bool          `json:"seen"` // 처음 안내를 봤는지
}

type App struct {
	support string
	token   string
	port    int
	binDir  string
	home    string

	mu       sync.Mutex
	settings Settings

	jobs *JobManager
	lib  *Library
	ops  *Ops
	upd  *Updater

	lastSeen atomic.Int64
}

var app *App

func main() {
	if len(os.Args) > 2 && os.Args[1] == "--pdftext" {
		runPDFText(os.Args[2])
		return
	}
	runEngine()
}

func runEngine() {
	log.SetFlags(log.LstdFlags)
	home, _ := os.UserHomeDir()
	support := os.Getenv("LIBROAD_SUPPORT")
	if support == "" {
		support = defaultSupportDir(home, "LIBROAD")
	}
	token := os.Getenv("LIBROAD_TOKEN")
	if token == "" {
		token = randID(16)
	}
	app = &App{support: support, token: token, binDir: filepath.Join(support, "bin"), home: home}
	os.MkdirAll(app.binDir, 0o755)
	if f, err := os.OpenFile(filepath.Join(support, "engine.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}
	app.loadSettings()
	app.lastSeen.Store(time.Now().Unix())
	writeFileAtomic(filepath.Join(app.binDir, "helper.js"), helperJS, 0o644)
	os.RemoveAll(filepath.Join(support, "tmp"))
	go cleanThumbs(filepath.Join(support, "thumbs"))

	app.jobs = NewJobManager()
	app.upd = NewUpdater(support)
	app.ops = NewOps(support)
	app.lib = NewLibrary(support)

	ln := listen(os.Getenv("LIBROAD_PORT"))
	app.port = ln.Addr().(*net.TCPAddr).Port
	writeFileAtomic(filepath.Join(support, "token"), []byte(token), 0o600)
	writeFileAtomic(filepath.Join(support, "port"), []byte(strconv.Itoa(app.port)), 0o644)
	log.Printf("LIBROAD %s listening on %d", build, app.port)

	go app.idleWatcher()
	go app.upd.autoLoop()
	go app.lib.loop()
	go app.ops.autoLoop()
	srv := &http.Server{Handler: hostGuard(app.routes())}
	log.Fatal(srv.Serve(ln))
}

// cleanThumbs removes old preview pictures (30일 넘게 안 쓴 것).
func cleanThumbs(dir string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cut := time.Now().AddDate(0, 0, -30)
	for _, e := range ents {
		if info, err := e.Info(); err == nil && (info.ModTime().Before(cut) || strings.HasPrefix(e.Name(), "tmp-")) {
			os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

func defaultSupportDir(home, name string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", name)
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), name)
	default:
		return filepath.Join(home, "."+strings.ToLower(name))
	}
}

func listen(pref string) net.Listener {
	if p, err := strconv.Atoi(pref); err == nil && p > 0 {
		for i := 0; i < 20; i++ {
			if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
				return ln
			}
			time.Sleep(150 * time.Millisecond)
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	return ln
}

// ---------------------------------------------------------------- lifecycle

// 창을 닫고 30분 동안 아무 요청이 없고, 하는 작업도 없으면 스스로 종료합니다.
// (자동 정리를 켰다면 계속 켜 둬요)
func (a *App) idleWatcher() {
	for range time.Tick(20 * time.Second) {
		idle := time.Now().Unix() - a.lastSeen.Load()
		if idle > 1800 && !a.jobs.Busy() && !a.upd.Busy() && !a.Settings().Auto.Enabled {
			a.shutdown()
		}
	}
}

func (a *App) shutdown() {
	a.jobs.CancelAll()
	a.lib.Save()
	if b, err := os.ReadFile(filepath.Join(a.support, "port")); err == nil && string(b) == strconv.Itoa(a.port) {
		os.Remove(filepath.Join(a.support, "port"))
	}
	os.Exit(0)
}

// ---------------------------------------------------------------- settings

func (a *App) settingsPath() string { return filepath.Join(a.support, "settings.json") }

func defaultRoots() []string {
	home, _ := os.UserHomeDir()
	out := []string{}
	for _, d := range []string{"Documents", "Desktop", "Downloads"} {
		p := filepath.Join(home, d)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

func (a *App) loadSettings() {
	a.settings = Settings{Appearance: "system", OCR: true, Notify: true, AutoUpdate: true}
	if b, err := os.ReadFile(a.settingsPath()); err == nil {
		_ = json.Unmarshal(b, &a.settings)
	} else {
		a.settings.Roots = defaultRoots()
		a.settings.Rules = defaultRules()
	}
	a.fixSettings()
}

func (a *App) fixSettings() {
	s := &a.settings
	if s.Appearance != "light" && s.Appearance != "dark" {
		s.Appearance = "system"
	}
	if s.Roots == nil {
		s.Roots = []string{}
	}
	s.Roots = cleanRoots(s.Roots)
	if s.Rules == nil {
		s.Rules = []Rule{}
	}
	if s.Basket == nil {
		s.Basket = []string{}
	}
	if s.Saved == nil {
		s.Saved = []SavedSearch{}
	}
	fixAuto(&s.Auto)
}

// cleanRoots removes duplicates and folders inside other folders of the list.
func cleanRoots(in []string) []string {
	seen := map[string]bool{}
	list := []string{}
	for _, r := range in {
		r = filepath.Clean(r)
		if r == "" || r == "." || seen[r] {
			continue
		}
		seen[r] = true
		list = append(list, r)
	}
	out := []string{}
	for _, r := range list {
		inside := false
		for _, o := range list {
			if o != r && isUnder(r, o) {
				inside = true
				break
			}
		}
		if !inside {
			out = append(out, r)
		}
	}
	return out
}

// isUnder reports whether p is dir or inside dir.
func isUnder(p, dir string) bool {
	if p == dir {
		return true
	}
	if dir == "/" {
		return strings.HasPrefix(p, "/")
	}
	return strings.HasPrefix(p, dir+string(filepath.Separator))
}

func (a *App) saveSettings() {
	b, _ := json.MarshalIndent(a.settings, "", "  ")
	writeFileAtomic(a.settingsPath(), b, 0o644)
}

func (a *App) Settings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.settings
	s.Roots = append([]string(nil), s.Roots...)
	s.Rules = append([]Rule(nil), s.Rules...)
	s.Basket = append([]string(nil), s.Basket...)
	s.Saved = append([]SavedSearch(nil), s.Saved...)
	return s
}

// update changes settings under the lock and saves them.
func (a *App) update(fn func(s *Settings)) Settings {
	a.mu.Lock()
	fn(&a.settings)
	a.fixSettings()
	a.saveSettings()
	a.mu.Unlock()
	return a.Settings()
}

// folderLabel: 홈 폴더의 문서 · 바탕 화면 · 다운로드는 한국어 이름으로 보여 줘요.
func folderLabel(p string) string {
	home, _ := os.UserHomeDir()
	if filepath.Dir(p) == home {
		switch filepath.Base(p) {
		case "Documents":
			return "문서"
		case "Desktop":
			return "바탕 화면"
		case "Downloads":
			return "다운로드"
		}
	}
	return nfc(filepath.Base(p))
}

func downloadsFolder() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}

func documentsFolder() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Documents")
}

// ---------------------------------------------------------------- routes

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFiles, "web")
	static := http.FileServer(http.FS(sub))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			if r.URL.Query().Get("t") != a.token {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				io.WriteString(w, "<meta charset=utf-8><body style='font:15px -apple-system;padding:40px'>LIBROAD 앱을 다시 실행해 주세요.</body>")
				return
			}
			b, _ := webFiles.ReadFile("web/index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(b)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		static.ServeHTTP(w, r)
	})

	api := func(path string, h http.HandlerFunc) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-token") != a.token && r.URL.Query().Get("t") != a.token {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			a.lastSeen.Store(time.Now().Unix())
			h(w, r)
		})
	}

	api("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "build": build})
	})
	api("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); a.shutdown() }()
	})
	api("/api/poll", func(w http.ResponseWriter, r *http.Request) {
		s := a.Settings()
		writeJSON(w, map[string]any{
			"build":   build,
			"jobs":    a.jobs.Snapshot(),
			"lib":     a.lib.Status(),
			"update":  a.upd.State(),
			"basket":  len(s.Basket),
			"history": a.ops.Count(),
			"histSig": a.ops.Sig(),
		})
	})
	api("/api/settings", a.handleSettings)
	api("/api/kinds", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]any{"kinds": kinds}) })
	api("/api/choose-folder", a.handleChooseFolder)
	api("/api/choose-files", a.handleChooseFiles)
	api("/api/fs/list", a.handleFsList)
	api("/api/fs/mkdir", a.handleFsMkdir)

	// 서재
	api("/api/library/roots", a.handleRoots)
	api("/api/library/rescan", func(w http.ResponseWriter, r *http.Request) {
		a.lib.Kick()
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/search", a.handleSearch)
	api("/api/doc", a.handleDoc)
	api("/api/docs", a.handleDocs)
	api("/api/recent", a.handleRecent)
	api("/api/thumb", a.handleThumb)
	api("/api/open", a.handleOpen)

	// 바구니 · 저장한 검색
	api("/api/basket", a.handleBasket)
	api("/api/saved", a.handleSaved)

	// 옮기기 · 복사 · 이름 바꾸기 · 휴지통 · 분류
	api("/api/ops/plan", a.ops.handlePlan)
	api("/api/ops/run", a.ops.handleRun)
	api("/api/history", a.ops.handleHistory)
	api("/api/history/undo", a.ops.handleUndo)
	api("/api/rules", a.handleRules)
	api("/api/auto", a.handleAuto)
	api("/api/auto/run", func(w http.ResponseWriter, r *http.Request) {
		id, err := a.ops.RunAutoNow(true)
		if err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "job": id})
	})

	// 정리하기
	api("/api/cleanup/start", a.handleCleanupStart)
	api("/api/job/result", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		readJSON(r, &in)
		j := a.jobs.Get(in.ID)
		if j == nil {
			jsonError(w, 404, "작업을 찾을 수 없어요")
			return
		}
		writeJSON(w, map[string]any{"ok": true, "job": j.Snap(), "result": j.Result()})
	})
	api("/api/job/cancel", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		readJSON(r, &in)
		a.jobs.Cancel(in.ID)
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/jobs/clear", func(w http.ResponseWriter, r *http.Request) {
		a.jobs.ClearFinished()
		writeJSON(w, map[string]any{"ok": true})
	})

	// 업데이트
	api("/api/update/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.upd.State()) })
	api("/api/update/check", func(w http.ResponseWriter, r *http.Request) {
		a.upd.Check(true)
		writeJSON(w, a.upd.State())
	})
	api("/api/update/install", func(w http.ResponseWriter, r *http.Request) {
		go a.upd.Install()
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/update/rollback", func(w http.ResponseWriter, r *http.Request) {
		if err := a.upd.Rollback(); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/update/restart", func(w http.ResponseWriter, r *http.Request) {
		if err := a.upd.Restart(a.port, a.token); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	return mux
}

// Only answer requests addressed to 127.0.0.1 / localhost (DNS rebinding protection).
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) settingsJSON() map[string]any {
	s := a.Settings()
	home, _ := os.UserHomeDir()
	return map[string]any{
		"appearance":  s.Appearance,
		"roots":       s.Roots,
		"ocr":         s.OCR,
		"ocrSupport":  ocrSupported(),
		"notify":      s.Notify,
		"autoUpdate":  s.AutoUpdate,
		"auto":        s.Auto,
		"rules":       s.Rules,
		"saved":       s.Saved,
		"lastDest":    s.LastDest,
		"seen":        s.Seen,
		"home":        home,
		"downloads":   downloadsFolder(),
		"documents":   documentsFolder(),
		"build":       build,
		"platform":    runtime.GOOS + "/" + runtime.GOARCH,
		"os":          uiOS(),
		"kinds":       kinds,
		"defaultDest": filepath.Join(documentsFolder(), "LIBROAD 정리"),
	}
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in map[string]json.RawMessage
		if err := readJSON(r, &in); err != nil {
			jsonError(w, 400, "bad request")
			return
		}
		ocrBefore := a.Settings().OCR
		a.update(func(s *Settings) {
			for k, v := range in {
				switch k {
				case "appearance":
					json.Unmarshal(v, &s.Appearance)
				case "ocr":
					json.Unmarshal(v, &s.OCR)
				case "notify":
					json.Unmarshal(v, &s.Notify)
				case "autoUpdate":
					json.Unmarshal(v, &s.AutoUpdate)
				case "lastDest":
					json.Unmarshal(v, &s.LastDest)
				case "seen":
					json.Unmarshal(v, &s.Seen)
				}
			}
		})
		if !ocrBefore && a.Settings().OCR {
			a.lib.Kick()
		}
		if ocrBefore && !a.Settings().OCR {
			a.lib.StopOCR()
		}
	}
	writeJSON(w, a.settingsJSON())
}

func (a *App) handleRoots(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"` // add | remove | reset
		Path   string `json:"path"`
	}
	readJSON(r, &in)
	p := filepath.Clean(strings.TrimSpace(in.Path))
	switch in.Action {
	case "add":
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			jsonError(w, 400, "폴더를 찾을 수 없어요")
			return
		}
		if p == "/" || p == a.home {
			jsonError(w, 400, "컴퓨터 전체나 홈 폴더 전체는 너무 넓어요. 그 안의 폴더를 골라 주세요")
			return
		}
		a.update(func(s *Settings) { s.Roots = append(s.Roots, p) })
	case "remove":
		a.update(func(s *Settings) {
			out := []string{}
			for _, x := range s.Roots {
				if x != p {
					out = append(out, x)
				}
			}
			s.Roots = out
		})
	case "reset":
		a.update(func(s *Settings) { s.Roots = defaultRoots() })
	}
	a.lib.Kick()
	writeJSON(w, a.settingsJSON())
}

func (a *App) handleBasket(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string   `json:"action"` // add | remove | clear | set
		Paths  []string `json:"paths"`
	}
	readJSON(r, &in)
	if in.Action != "" {
		a.update(func(s *Settings) {
			switch in.Action {
			case "add":
				have := map[string]bool{}
				for _, p := range s.Basket {
					have[p] = true
				}
				for _, p := range in.Paths {
					if !have[p] {
						s.Basket = append(s.Basket, p)
						have[p] = true
					}
				}
			case "remove":
				drop := map[string]bool{}
				for _, p := range in.Paths {
					drop[p] = true
				}
				out := []string{}
				for _, p := range s.Basket {
					if !drop[p] {
						out = append(out, p)
					}
				}
				s.Basket = out
			case "clear":
				s.Basket = []string{}
			case "set":
				s.Basket = append([]string{}, in.Paths...)
			}
		})
	}
	s := a.Settings()
	items := []DocInfo{}
	missing := 0
	for _, p := range s.Basket {
		if d, ok := a.lib.Info(p); ok {
			items = append(items, d)
		} else {
			missing++
		}
	}
	writeJSON(w, map[string]any{"ok": true, "items": items, "missing": missing})
}

func (a *App) handleSaved(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string          `json:"action"` // add | remove | rename
		ID     string          `json:"id"`
		Name   string          `json:"name"`
		Query  json.RawMessage `json:"query"`
	}
	readJSON(r, &in)
	a.update(func(s *Settings) {
		switch in.Action {
		case "add":
			name := strings.TrimSpace(in.Name)
			if name == "" {
				name = "저장한 검색"
			}
			s.Saved = append(s.Saved, SavedSearch{ID: randID(5), Name: name, Query: in.Query, Create: time.Now().Unix()})
		case "remove":
			out := []SavedSearch{}
			for _, x := range s.Saved {
				if x.ID != in.ID {
					out = append(out, x)
				}
			}
			s.Saved = out
		case "rename":
			for i := range s.Saved {
				if s.Saved[i].ID == in.ID && strings.TrimSpace(in.Name) != "" {
					s.Saved[i].Name = strings.TrimSpace(in.Name)
				}
			}
		}
	})
	writeJSON(w, map[string]any{"ok": true, "saved": a.Settings().Saved})
}

// ---------------------------------------------------------------- open · reveal · quick look

func (a *App) handleOpen(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
		How  string `json:"how"` // open | reveal | quicklook
	}
	readJSON(r, &in)
	if in.Path == "" || !fileExists(in.Path) {
		jsonError(w, 404, "파일을 찾을 수 없어요. 옮기거나 지웠을 수 있어요")
		return
	}
	switch in.How {
	case "reveal":
		revealPath(in.Path)
	case "quicklook":
		quickLook(in.Path)
	default:
		openPath(in.Path)
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ---------------------------------------------------------------- folder / file pickers

func (a *App) handleChooseFolder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Start  string `json:"start"`
		Prompt string `json:"prompt"`
	}
	readJSON(r, &in)
	if t := os.Getenv("LIBROAD_TEST_CHOOSE"); t != "" {
		os.MkdirAll(t, 0o755)
		writeJSON(w, map[string]any{"ok": true, "path": t})
		return
	}
	if runtime.GOOS != "darwin" {
		jsonError(w, 501, "macOS에서만 지원해요")
		return
	}
	start := in.Start
	if st, err := os.Stat(start); err != nil || !st.IsDir() {
		start = documentsFolder()
	}
	prompt := in.Prompt
	if prompt == "" {
		prompt = "폴더를 선택하세요"
	}
	out, err := osascriptLines([]string{
		"on run argv",
		"activate",
		"try",
		"set f to choose folder with prompt (item 2 of argv) default location (POSIX file (item 1 of argv))",
		"return POSIX path of f",
		"on error number -128",
		"return \"\"",
		"end try",
		"end run",
	}, start, prompt)
	if err != nil {
		jsonError(w, 500, "폴더 선택 창을 열지 못했어요")
		return
	}
	p := strings.TrimSpace(out)
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	writeJSON(w, map[string]any{"ok": true, "path": p})
}

// 파일 고르기 (하나 또는 여러 개)
func (a *App) handleChooseFiles(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Start  string `json:"start"`
		Prompt string `json:"prompt"`
	}
	readJSON(r, &in)
	if t := os.Getenv("LIBROAD_TEST_CHOOSE_FILES"); t != "" {
		writeJSON(w, map[string]any{"ok": true, "paths": strings.Split(t, "|")})
		return
	}
	if runtime.GOOS != "darwin" {
		jsonError(w, 501, "macOS에서만 지원해요")
		return
	}
	start := in.Start
	if st, err := os.Stat(start); err != nil || !st.IsDir() {
		start = documentsFolder()
	}
	prompt := in.Prompt
	if prompt == "" {
		prompt = "문서를 고르세요 (⌘ 또는 ⇧를 누른 채 여러 개를 고를 수 있어요)"
	}
	out, err := osascriptLines([]string{
		"on run argv",
		"activate",
		"try",
		"set fs to choose file with prompt (item 2 of argv) default location (POSIX file (item 1 of argv)) with multiple selections allowed",
		"set out to \"\"",
		"repeat with f in fs",
		"set out to out & POSIX path of f & linefeed",
		"end repeat",
		"return out",
		"on error number -128",
		"return \"\"",
		"end try",
		"end run",
	}, start, prompt)
	if err != nil {
		jsonError(w, 500, "파일 선택 창을 열지 못했어요")
		return
	}
	paths := []string{}
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			paths = append(paths, strings.TrimSuffix(l, "/"))
		}
	}
	writeJSON(w, map[string]any{"ok": true, "paths": paths})
}

func osascriptLines(lines []string, argv ...string) (string, error) {
	args := []string{}
	for _, l := range lines {
		args = append(args, "-e", l)
	}
	args = append(args, argv...)
	out, err := exec.Command("/usr/bin/osascript", args...).Output()
	return string(out), err
}

func notify(title, msg string) {
	if app.Settings().Notify {
		notifyOS(title, msg)
	}
}

// ---------------------------------------------------------------- utils

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(v)
}

func randID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeFileAtomic(path string, b []byte, perm os.FileMode) {
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, perm) == nil {
		os.Rename(tmp, path)
	}
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// uniquePath returns dir/name, or "name (2).ext" … when that file already exists.
func uniquePath(dir, name string) string {
	return uniquePathExcept(dir, name, nil)
}

// uniquePathExcept is uniquePath but also avoids names reserved by the same plan.
func uniquePathExcept(dir, name string, taken map[string]bool) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	p := filepath.Join(dir, name)
	for n := 2; ; n++ {
		_, err := os.Lstat(p)
		if errors.Is(err, os.ErrNotExist) && !taken[strings.ToLower(p)] {
			return p
		}
		p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, n, ext))
	}
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func init() {
	mime.AddExtensionType(".js", "text/javascript")
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".svg", "image/svg+xml")
}

func uiOS() string {
	if o := os.Getenv("LIBROAD_FAKE_OS"); o != "" {
		return o
	}
	return runtime.GOOS
}

// ---------------------------------------------------------------- 앱 안 폴더 고르기 (규칙 · Windows 대비)

type fsEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Icon string `json:"icon,omitempty"`
}

func quickPlaces() []fsEntry {
	home, _ := os.UserHomeDir()
	out := []fsEntry{}
	for _, p := range []struct{ name, dir, icon string }{
		{"문서", "Documents", "📄"}, {"다운로드", "Downloads", "⬇️"}, {"바탕 화면", "Desktop", "🖥️"},
	} {
		d := filepath.Join(home, p.dir)
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			out = append(out, fsEntry{p.name, d, p.icon})
		}
	}
	return out
}

func drives() []fsEntry {
	out := []fsEntry{}
	if runtime.GOOS == "windows" {
		for c := 'C'; c <= 'Z'; c++ {
			root := string(c) + ":\\"
			if _, err := os.Stat(root); err == nil {
				out = append(out, fsEntry{string(c) + ": 드라이브", root, "💽"})
			}
		}
		return out
	}
	if ents, err := os.ReadDir("/Volumes"); err == nil {
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			out = append(out, fsEntry{e.Name(), filepath.Join("/Volumes", e.Name()), "💽"})
		}
	}
	return out
}

func (a *App) handleFsList(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
	}
	readJSON(r, &in)
	p := in.Path
	if p == "" {
		p = documentsFolder()
	}
	p = filepath.Clean(p)
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		p = documentsFolder()
	}
	ents, err := os.ReadDir(p)
	dirs := []fsEntry{}
	if err == nil {
		for _, e := range ents {
			n := e.Name()
			if strings.HasPrefix(n, ".") || strings.HasPrefix(n, "$") {
				continue
			}
			full := filepath.Join(p, n)
			isDir := e.IsDir()
			if !isDir && e.Type()&os.ModeSymlink != 0 {
				if st, err := os.Stat(full); err == nil && st.IsDir() {
					isDir = true
				}
			}
			if isDir && !isPackageExt(extOf(n)) && !strings.HasSuffix(n, ".app") {
				dirs = append(dirs, fsEntry{Name: n, Path: full})
			}
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name) })
	parent := filepath.Dir(p)
	if parent == p {
		parent = ""
	}
	writeJSON(w, map[string]any{
		"path": p, "name": filepath.Base(p), "parent": parent, "dirs": dirs,
		"places": quickPlaces(), "drives": drives(), "readable": err == nil,
	})
}

func (a *App) handleFsMkdir(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	readJSON(r, &in)
	name := strings.TrimSpace(in.Name)
	if name == "" || strings.ContainsAny(name, `\/:*?"<>|`) || name == "." || name == ".." {
		jsonError(w, 400, "폴더 이름에 쓸 수 없는 글자가 있어요")
		return
	}
	dir := filepath.Join(in.Path, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		if os.IsExist(err) {
			jsonError(w, 400, "같은 이름의 폴더가 이미 있어요")
			return
		}
		jsonError(w, 500, "이 위치에는 폴더를 만들 수 없어요")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "path": dir})
}

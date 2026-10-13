package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// PDF로 바꾸기
//   - 사진(jpg · png · heic): macOS의 sips
//   - Pages · Keynote · Numbers: 맥에 그 앱이 있으면 그 앱으로 (없으면 LibreOffice)
//   - 한글 · 워드 · 엑셀 · 파워포인트 · 텍스트: LibreOffice (+ 한글은 H2Orestart 확장, 그 확장은 Java가 필요)
//     → 처음 한 번 "변환 도구"(LibreOffice + Java + H2Orestart, 약 400MB)를 받아서 Support/tools 에 둬요.
//   - 여러 개를 하나로 합치기: macOS의 PDFKit (helper.js merge)
// 문서는 이 컴퓨터 밖으로 보내지 않아요. 인터넷은 도구를 받을 때만 써요.

type ConvTools struct {
	dir string

	mu         sync.Mutex
	installing bool
	err        string
	loMu       sync.Mutex // LibreOffice는 한 번에 하나씩
}

type toolsReady struct {
	LO   string `json:"lo"`
	JRE  string `json:"jre"`
	H2O  string `json:"h2o"`
	Time int64  `json:"time"`
}

func NewConvTools(support string) *ConvTools {
	return &ConvTools{dir: filepath.Join(support, "tools")}
}

func (t *ConvTools) readyPath() string { return filepath.Join(t.dir, "ready.json") }

func (t *ConvTools) ready() (toolsReady, bool) {
	var r toolsReady
	b, err := os.ReadFile(t.readyPath())
	if err != nil || json.Unmarshal(b, &r) != nil {
		return r, false
	}
	return r, fileExists(t.soffice())
}

// loBin is the folder with soffice / unopkg.
func (t *ConvTools) loBin() string {
	if d := os.Getenv("LIBROAD_LO_DIR"); d != "" {
		return d
	}
	return filepath.Join(t.dir, "LibreOffice.app", "Contents", "MacOS")
}
func (t *ConvTools) soffice() string { return filepath.Join(t.loBin(), "soffice") }
func (t *ConvTools) unopkg() string  { return filepath.Join(t.loBin(), "unopkg") }
func (t *ConvTools) profile() string { return filepath.Join(t.dir, "lo-profile") }
func (t *ConvTools) profileURL() string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(t.profile())}).String()
}

func (t *ConvTools) javaHome() string {
	if j := os.Getenv("LIBROAD_JAVA_HOME"); j != "" {
		return j
	}
	m, _ := filepath.Glob(filepath.Join(t.dir, "jre", "*", "Contents", "Home"))
	if len(m) > 0 {
		return m[0]
	}
	return ""
}

func (t *ConvTools) env() []string {
	env := os.Environ()
	if j := t.javaHome(); j != "" {
		env = append(env, "JAVA_HOME="+j)
	}
	return env
}

func iworkApp(kind string) string {
	name := map[string]string{"pages": "Pages", "keynote": "Keynote", "numbers": "Numbers"}[kind]
	if name == "" || runtime.GOOS != "darwin" {
		return ""
	}
	home, _ := os.UserHomeDir()
	for _, d := range []string{"/Applications", filepath.Join(home, "Applications"), "/System/Applications"} {
		if fileExists(filepath.Join(d, name+".app")) {
			return name
		}
	}
	return ""
}

func dirSize(dir string) int64 {
	var n int64
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}

func (t *ConvTools) State() map[string]any {
	r, ok := t.ready()
	t.mu.Lock()
	defer t.mu.Unlock()
	st := map[string]any{
		"installed": ok, "hwp": ok && r.H2O != "", "lo": r.LO, "installing": t.installing, "err": t.err,
		"iwork":  map[string]bool{"pages": iworkApp("pages") != "", "keynote": iworkApp("keynote") != "", "numbers": iworkApp("numbers") != ""},
		"images": runtime.GOOS == "darwin" || hasCmd("convert"),
		"merge":  helperAvailable(),
	}
	if ok {
		st["size"] = dirSize(t.dir)
	}
	return st
}

func hasCmd(c string) bool {
	_, err := exec.LookPath(c)
	return err == nil
}

// ---------------------------------------------------------------- 어떤 방법으로 바꿀지

type convItem struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Size   int64  `json:"size"`
	Method string `json:"method"` // pdf | image | iwork | office | none
	Via    string `json:"via"`    // 사용자에게 보여 줄 도구 이름
	Need   bool   `json:"need"`   // 변환 도구가 있어야 함
	Note   string `json:"note,omitempty"`
}

var officeExts = map[string]bool{"doc": true, "docx": true, "rtf": true, "txt": true, "md": true, "csv": true,
	"xls": true, "xlsx": true, "ppt": true, "pptx": true, "hwp": true, "hwpx": true}

func (t *ConvTools) plan(p string) convItem {
	it := convItem{Path: p, Name: nfc(filepath.Base(p))}
	st, err := os.Stat(p)
	if err != nil {
		it.Method, it.Note = "none", "파일을 찾을 수 없어요"
		return it
	}
	it.Size = st.Size()
	k := kindOf(p)
	if k == nil {
		it.Method, it.Note = "none", "PDF로 바꿀 수 없는 형식이에요"
		return it
	}
	it.Kind = k.ID
	if isDataless(st) {
		it.Method, it.Note = "none", "iCloud에만 있어요. Finder에서 한 번 열어 내려받은 뒤 다시 해 주세요"
		return it
	}
	_, installed := t.ready()
	ext := extOf(p)
	switch {
	case ext == "pdf":
		it.Method, it.Via = "pdf", "이미 PDF"
	case k.Image:
		it.Method, it.Via = "image", "사진 → PDF"
	case k.ID == "pages" || k.ID == "keynote" || k.ID == "numbers":
		if app := iworkApp(k.ID); app != "" {
			it.Method, it.Via = "iwork", app
		} else {
			it.Method, it.Via, it.Need = "office", "변환 도구", true
		}
	case officeExts[ext]:
		it.Method, it.Via, it.Need = "office", "변환 도구", true
	default:
		it.Method, it.Note = "none", "전자책(epub)은 PDF로 바꿀 수 없어요"
	}
	if it.Need && !installed {
		it.Note = "변환 도구를 받으면 바꿀 수 있어요"
	}
	return it
}

func (a *App) handleConvertState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.conv.State())
}

func (a *App) handleConvertCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paths []string `json:"paths"`
	}
	readJSON(r, &in)
	items := []convItem{}
	need := false
	for _, p := range dedupe(in.Paths) {
		it := a.conv.plan(p)
		need = need || it.Need
		items = append(items, it)
	}
	writeJSON(w, map[string]any{"ok": true, "items": items, "needTools": need, "tools": a.conv.State()})
}

// ---------------------------------------------------------------- 변환 도구 받기

func (a *App) handleConvertInstall(w http.ResponseWriter, r *http.Request) {
	t := a.conv
	t.mu.Lock()
	if t.installing {
		t.mu.Unlock()
		jsonError(w, 400, "이미 받고 있어요")
		return
	}
	t.installing, t.err = true, ""
	t.mu.Unlock()
	job := a.jobs.Start("tools", "PDF 변환 도구 받기", "MB", 0)
	go func() {
		err := t.install(job)
		t.mu.Lock()
		t.installing = false
		if err != nil {
			t.err = err.Error()
		}
		t.mu.Unlock()
		if err != nil {
			log.Printf("tools install: %v", err)
			job.Finish(err, "")
		} else {
			job.Finish(nil, "이제 한글 · 워드 · 엑셀 · 파워포인트를 PDF로 바꿀 수 있어요")
		}
	}()
	writeJSON(w, map[string]any{"ok": true, "job": job.ID()})
}

func (a *App) handleConvertRemove(w http.ResponseWriter, r *http.Request) {
	t := a.conv
	t.mu.Lock()
	busy := t.installing
	t.mu.Unlock()
	if busy {
		jsonError(w, 400, "변환 도구를 받는 중이에요")
		return
	}
	t.loMu.Lock()
	err := os.RemoveAll(t.dir)
	t.loMu.Unlock()
	if err != nil {
		jsonError(w, 500, "지우지 못했어요")
		return
	}
	writeJSON(w, a.conv.State())
}

type dlItem struct {
	name, url, sum string
	size           int64
}

var errCanceled = errors.New("받기를 멈췄어요")

func (t *ConvTools) install(job *Job) error {
	os.MkdirAll(t.dir, 0o755)
	dl := filepath.Join(t.dir, "dl")
	os.RemoveAll(dl)
	os.MkdirAll(dl, 0o755)
	defer os.RemoveAll(dl)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for ctx.Err() == nil {
			if job.Canceled() {
				cancel()
				return
			}
			time.Sleep(300 * time.Millisecond)
		}
	}()
	ready := toolsReady{}
	testLO := os.Getenv("LIBROAD_LO_DIR") != ""

	items := []dlItem{}
	if !testLO {
		if runtime.GOOS != "darwin" {
			return errors.New("지금은 macOS에서만 받을 수 있어요")
		}
		if free := freeSpace(t.dir); free > 0 && free < 2<<30 {
			return errors.New("디스크 공간이 부족해요. 2GB 이상 비운 뒤 다시 시도해 주세요")
		}
		job.SetMsg("받을 파일을 확인하는 중…")
		lo, ver, err := latestLibreOffice(ctx)
		if err != nil {
			return err
		}
		ready.LO = ver
		items = append(items, lo)
		jre, jver, err := latestJRE(ctx)
		if err != nil {
			return err
		}
		ready.JRE = jver
		items = append(items, jre)
	}
	h2oURL := os.Getenv("LIBROAD_H2O_URL")
	if h2oURL == "" {
		h2oURL = "https://github.com/ebandal/H2Orestart/releases/latest/download/H2Orestart.oxt"
	}
	items = append(items, dlItem{name: "H2Orestart.oxt", url: h2oURL, size: 1 << 20})

	var total int64
	for _, it := range items {
		total += it.size
	}
	job.SetTotal(total >> 20)
	var doneBefore int64
	for _, it := range items {
		dest := filepath.Join(dl, it.name)
		job.SetMsg(map[bool]string{true: "한글 문서 읽기 확장", false: map[bool]string{true: "LibreOffice (문서 변환)", false: "Java (한글 변환에 필요)"}[strings.HasSuffix(it.name, ".dmg")]}[it.name == "H2Orestart.oxt"] + " 받는 중")
		if strings.HasPrefix(it.url, "/") {
			if err := copyPlain(it.url, dest); err != nil {
				return err
			}
		} else if err := download(ctx, it.url, dest, func(done, tot int64) {
			job.SetDone((doneBefore + done) >> 20)
		}); err != nil {
			if ctx.Err() != nil {
				return errCanceled
			}
			return fmt.Errorf("받지 못했어요: %v", err)
		}
		if it.sum != "" && !strings.EqualFold(sha256File(dest), it.sum) {
			return errors.New("받은 파일이 손상됐어요. 잠시 뒤 다시 시도해 주세요")
		}
		if st, err := os.Stat(dest); err == nil {
			doneBefore += st.Size()
		}
		job.SetDone(doneBefore >> 20)
	}
	if ctx.Err() != nil {
		return errCanceled
	}

	if !testLO {
		// LibreOffice: dmg를 열어 앱만 복사
		job.SetMsg("LibreOffice를 설치하는 중…")
		mnt := filepath.Join(dl, "mnt")
		os.MkdirAll(mnt, 0o755)
		if out, err := exec.Command("/usr/bin/hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mnt, filepath.Join(dl, items[0].name)).CombinedOutput(); err != nil {
			return fmt.Errorf("LibreOffice 설치 파일을 열지 못했어요: %s", lastLine(string(out)))
		}
		apps, _ := filepath.Glob(filepath.Join(mnt, "*.app"))
		var copyErr error
		if len(apps) == 0 {
			copyErr = errors.New("LibreOffice 설치 파일 안에 앱이 없어요")
		} else {
			os.RemoveAll(filepath.Join(t.dir, "LibreOffice.app"))
			if out, err := exec.Command("/usr/bin/ditto", apps[0], filepath.Join(t.dir, "LibreOffice.app")).CombinedOutput(); err != nil {
				copyErr = fmt.Errorf("LibreOffice를 복사하지 못했어요: %s", lastLine(string(out)))
			}
		}
		exec.Command("/usr/bin/hdiutil", "detach", mnt, "-force").Run()
		if copyErr != nil {
			return copyErr
		}
		exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", filepath.Join(t.dir, "LibreOffice.app")).Run()

		// Java (한글 확장이 Java로 만들어져 있어요)
		job.SetMsg("Java를 설치하는 중…")
		jdir := filepath.Join(t.dir, "jre")
		os.RemoveAll(jdir)
		os.MkdirAll(jdir, 0o755)
		if out, err := exec.Command("/usr/bin/tar", "-xzf", filepath.Join(dl, items[1].name), "-C", jdir).CombinedOutput(); err != nil {
			return fmt.Errorf("Java를 풀지 못했어요: %s", lastLine(string(out)))
		}
		exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", jdir).Run()
	}

	// 한글(hwp · hwpx) 확장
	job.SetMsg("한글 문서 읽기 확장을 넣는 중…")
	os.RemoveAll(t.profile())
	t.loMu.Lock()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 4*time.Minute)
	cmd := exec.CommandContext(ctx2, t.unopkg(), "add", "--suppress-license", "-env:UserInstallation="+t.profileURL(), filepath.Join(dl, "H2Orestart.oxt"))
	cmd.Env = t.env()
	setProcessGroup(cmd)
	out, err := cmd.CombinedOutput()
	cancel2()
	t.loMu.Unlock()
	if err != nil {
		log.Printf("unopkg: %v %s", err, out)
		// 한글만 안 되고 나머지는 돼요
	} else {
		ready.H2O = "ok"
	}
	ready.Time = time.Now().Unix()
	b, _ := json.Marshal(ready)
	writeFileAtomic(t.readyPath(), b, 0o644)
	if ready.H2O == "" {
		return errors.New("한글 문서 확장을 넣지 못했어요. 한글을 뺀 나머지 문서는 PDF로 바꿀 수 있어요")
	}
	return nil
}

var loVerRe = regexp.MustCompile(`href="(\d+\.\d+\.\d+)/"`)

func httpGet(ctx context.Context, u string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "LIBROAD/"+build)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, errors.New("인터넷 연결을 확인해 주세요")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

func httpSize(ctx context.Context, u string) int64 {
	req, _ := http.NewRequestWithContext(ctx, "HEAD", u, nil)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return 0
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return -1
	}
	return resp.ContentLength
}

// latestLibreOffice finds the newest stable LibreOffice dmg for this Mac.
func latestLibreOffice(ctx context.Context) (dlItem, string, error) {
	base := "https://download.documentfoundation.org/libreoffice/stable/"
	body, err := httpGet(ctx, base)
	if err != nil {
		return dlItem{}, "", fmt.Errorf("LibreOffice 정보를 받지 못했어요 (%v)", err)
	}
	vers := []string{}
	for _, m := range loVerRe.FindAllStringSubmatch(string(body), -1) {
		vers = append(vers, m[1])
	}
	sort.Slice(vers, func(i, j int) bool { return compareVersions(vers[i], vers[j]) > 0 })
	dir, file := "aarch64", "aarch64"
	if runtime.GOARCH == "amd64" {
		dir, file = "x86_64", "x86-64"
	}
	for _, v := range vers {
		u := fmt.Sprintf("%s%s/mac/%s/LibreOffice_%s_MacOS_%s.dmg", base, v, dir, v, file)
		size := httpSize(ctx, u)
		if size == -1 {
			continue
		}
		if size <= 0 {
			size = 350 << 20
		}
		sum := ""
		if b, err := httpGet(ctx, u+".sha256"); err == nil {
			if f := strings.Fields(string(b)); len(f) > 0 && len(f[0]) == 64 {
				sum = f[0]
			}
		}
		return dlItem{name: "LibreOffice.dmg", url: u, sum: sum, size: size}, v, nil
	}
	return dlItem{}, "", errors.New("이 Mac에 맞는 LibreOffice를 찾지 못했어요")
}

func latestJRE(ctx context.Context) (dlItem, string, error) {
	arch := "aarch64"
	if runtime.GOARCH == "amd64" {
		arch = "x64"
	}
	u := "https://api.adoptium.net/v3/assets/latest/21/hotspot?os=mac&image_type=jre&vendor=eclipse&architecture=" + arch
	body, err := httpGet(ctx, u)
	if err != nil {
		return dlItem{}, "", fmt.Errorf("Java 정보를 받지 못했어요 (%v)", err)
	}
	var list []struct {
		Binary struct {
			Package struct {
				Name     string `json:"name"`
				Link     string `json:"link"`
				Checksum string `json:"checksum"`
				Size     int64  `json:"size"`
			} `json:"package"`
		} `json:"binary"`
		Version struct {
			OpenJDK string `json:"openjdk_version"`
		} `json:"version"`
	}
	if json.Unmarshal(body, &list) != nil || len(list) == 0 || list[0].Binary.Package.Link == "" {
		return dlItem{}, "", errors.New("Java 정보를 읽지 못했어요")
	}
	p := list[0].Binary.Package
	return dlItem{name: "jre.tar.gz", url: p.Link, sum: p.Checksum, size: p.Size}, list[0].Version.OpenJDK, nil
}

// ---------------------------------------------------------------- 바꾸기

type ConvReq struct {
	Paths []string `json:"paths"`
	Dest  string   `json:"dest"`  // "" = 원래 폴더
	Merge bool     `json:"merge"` // 모두 하나의 PDF로
	Name  string   `json:"name"`  // 합친 PDF 이름
}

func (a *App) handleConvertRun(w http.ResponseWriter, r *http.Request) {
	var in ConvReq
	if err := readJSON(r, &in); err != nil {
		jsonError(w, 400, "bad request")
		return
	}
	items := []convItem{}
	_, installed := a.conv.ready()
	for _, p := range dedupe(in.Paths) {
		it := a.conv.plan(p)
		if it.Method == "none" || (it.Need && !installed) || (it.Method == "pdf" && !in.Merge) {
			continue
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		jsonError(w, 400, "PDF로 바꿀 수 있는 문서가 없어요")
		return
	}
	if in.Dest != "" {
		if st, err := os.Stat(in.Dest); err != nil || !st.IsDir() {
			jsonError(w, 400, "저장할 폴더를 찾을 수 없어요")
			return
		}
	}
	title := fmt.Sprintf("PDF로 바꾸기 (%d개)", len(items))
	if in.Merge {
		title = fmt.Sprintf("PDF 하나로 합치기 (%d개)", len(items))
	}
	job := a.jobs.Start("ops", title, "개", int64(len(items)))
	go a.conv.run(in, items, job)
	writeJSON(w, map[string]any{"ok": true, "job": job.ID()})
}

type convOut struct {
	it  convItem
	out string
	err error
}

func pdfName(src string) string {
	b := filepath.Base(src)
	return strings.TrimSuffix(b, filepath.Ext(b)) + ".pdf"
}

func (t *ConvTools) run(in ConvReq, items []convItem, job *Job) {
	tmp := filepath.Join(app.support, "tmp", "conv-"+randID(4))
	os.MkdirAll(tmp, 0o755)
	defer os.RemoveAll(tmp)
	h := &History{ID: randID(6), Op: "convert", Time: time.Now().Unix(), Items: []HistItem{}, Dirs: []string{}}
	h.Title = job.Snap().Title
	taken := map[string]bool{}
	finalPath := func(src string) string {
		dir := in.Dest
		if dir == "" {
			dir = filepath.Dir(src)
		}
		p := uniquePathExcept(dir, pdfName(src), taken)
		taken[strings.ToLower(p)] = true
		return p
	}
	results := make([]convOut, len(items))
	fail := func(i int, err error) {
		results[i].err = err
		h.Failed++
		if len(h.Errors) < 20 {
			h.Errors = append(h.Errors, fmt.Sprintf("%s: %s", items[i].Name, err.Error()))
		}
	}
	// 사진 · 이미 PDF · LibreOffice 는 하나씩, iWork 앱은 앱별로 한 번에
	byApp := map[string][]int{}
	for i, it := range items {
		results[i].it = it
		if it.Method == "iwork" {
			byApp[it.Via] = append(byApp[it.Via], i)
		}
	}
	for appName, idx := range byApp {
		if job.Canceled() {
			break
		}
		pairs := []string{}
		outs := map[int]string{}
		for k, i := range idx {
			dir := tmp
			if !in.Merge {
				// 앱이 쓸 수 있도록 저장할 폴더에 바로 내보낸 뒤 이름만 바꿔요
				dir = in.Dest
				if dir == "" {
					dir = filepath.Dir(items[i].Path)
				}
			}
			o := filepath.Join(dir, fmt.Sprintf("~libroad-%d-%s", k, pdfName(items[i].Path)))
			outs[i] = o
			pairs = append(pairs, items[i].Path, o)
		}
		job.SetMsg(appName + "에서 PDF로 내보내는 중…")
		errs := iworkExport(appName, pairs)
		for k, i := range idx {
			if e := errs[k]; e != nil || !fileExists(outs[i]) {
				os.Remove(outs[i])
				// 앱으로 안 되면 변환 도구로 다시
				if _, ok := t.ready(); ok {
					results[i].it.Method = "office"
					continue
				}
				if e == nil {
					e = errors.New(appName + "에서 내보내지 못했어요")
				}
				fail(i, e)
			} else {
				results[i].out = outs[i]
			}
			job.Add(1)
		}
	}
	for i := range results {
		if job.Canceled() {
			break
		}
		r := &results[i]
		if r.out != "" || r.err != nil {
			continue
		}
		job.SetMsg(r.it.Name)
		var err error
		switch r.it.Method {
		case "pdf":
			r.out = r.it.Path
		case "image":
			r.out = filepath.Join(tmp, fmt.Sprintf("%d-%s", i, pdfName(r.it.Path)))
			err = imageToPDF(r.it.Path, r.out)
		case "office":
			r.out, err = t.officeToPDF(r.it.Path, filepath.Join(tmp, fmt.Sprint(i)))
		}
		if err != nil {
			r.out = ""
			fail(i, err)
		}
		job.Add(1)
	}
	if !job.Canceled() {
		if in.Merge {
			list := []string{}
			for _, r := range results {
				if r.out != "" {
					list = append(list, r.out)
				}
			}
			if len(list) > 0 {
				dir := in.Dest
				if dir == "" {
					dir = filepath.Dir(items[0].Path)
				}
				name := safeName(strings.TrimSuffix(strings.TrimSpace(in.Name), ".pdf"))
				if name == "" {
					name = strings.TrimSuffix(pdfName(items[0].Path), ".pdf")
					if len(items) > 1 {
						name += fmt.Sprintf(" 외 %d개", len(items)-1)
					}
				}
				dst := uniquePath(dir, name+".pdf")
				job.SetMsg("하나의 PDF로 합치는 중…")
				if err := mergePDFs(list, dst); err != nil {
					h.Failed++
					h.Errors = append(h.Errors, err.Error())
				} else {
					h.Items = append(h.Items, HistItem{From: items[0].Path, To: dst, Act: "create"})
					h.Count = 1
				}
			}
		} else {
			for _, r := range results {
				if r.out == "" || r.it.Method == "pdf" {
					continue
				}
				dst := finalPath(r.it.Path)
				if err := moveTree(r.out, dst); err != nil {
					h.Failed++
					h.Errors = append(h.Errors, fmt.Sprintf("%s: %s", r.it.Name, friendlyFSErr(err)))
					continue
				}
				h.Items = append(h.Items, HistItem{From: r.it.Path, To: dst, Act: "create"})
				h.Count++
			}
		}
	}
	if h.Count > 0 || h.Failed > 0 {
		app.ops.mu.Lock()
		app.ops.hist = append(app.ops.hist, h)
		app.ops.mu.Unlock()
		app.ops.saveHistory()
	}
	created := []string{}
	for _, x := range h.Items {
		created = append(created, x.To)
	}
	msg := fmt.Sprintf("PDF %d개를 만들었어요", h.Count)
	if in.Merge && h.Count == 1 {
		msg = "하나의 PDF로 합쳤어요"
	}
	if h.Failed > 0 {
		msg += fmt.Sprintf(" · %d개 실패", h.Failed)
	}
	var err error
	if h.Count == 0 && len(h.Errors) > 0 {
		err = errors.New(h.Errors[0])
	}
	job.SetResult(map[string]any{"history": h.ID, "count": h.Count, "failed": h.Failed, "errors": h.Errors, "created": created})
	job.Finish(err, msg)
	app.lib.Kick()
}

func imageToPDF(src, dst string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("/usr/bin/sips", "-s", "format", "pdf", src, "--out", dst)
	} else if hasCmd("convert") {
		cmd = exec.Command("convert", src, dst)
	} else {
		return errors.New("사진을 PDF로 바꿀 수 없어요")
	}
	if out, err := cmd.CombinedOutput(); err != nil || !fileExists(dst) {
		log.Printf("image→pdf %s: %v %s", src, err, lastLine(string(out)))
		return errors.New("사진을 PDF로 바꾸지 못했어요")
	}
	return nil
}

// officeToPDF runs LibreOffice headless for one file and returns the PDF in outDir.
func (t *ConvTools) officeToPDF(src, outDir string) (string, error) {
	if !fileExists(t.soffice()) {
		return "", errors.New("변환 도구가 없어요. 먼저 받아 주세요")
	}
	os.MkdirAll(outDir, 0o755)
	ext := extOf(src)
	input := src
	stem := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	infilter := ""
	switch ext {
	case "txt", "md", "csv":
		// 예전 한국어 파일(CP949)도 깨지지 않게 UTF-8 복사본으로
		b, err := os.ReadFile(src)
		if err != nil {
			return "", errors.New("파일을 읽지 못했어요")
		}
		newExt := ".txt"
		infilter = "Text (encoded):UTF8,LF,,,"
		if ext == "csv" {
			newExt = ".csv"
			infilter = "CSV:44,34,76"
		}
		input = filepath.Join(outDir, "in", stem+newExt)
		os.MkdirAll(filepath.Dir(input), 0o755)
		if err := os.WriteFile(input, []byte(decodeBytes(b)), 0o644); err != nil {
			return "", err
		}
	}
	try := func(filter string) error {
		args := []string{"-env:UserInstallation=" + t.profileURL(), "--headless", "--norestore", "--nologo", "--nodefault", "--nolockcheck"}
		if filter != "" {
			args = append(args, "--infilter="+filter)
		}
		args = append(args, "--convert-to", "pdf", "--outdir", outDir, input)
		t.loMu.Lock()
		defer t.loMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, t.soffice(), args...)
		cmd.Env = t.env()
		setProcessGroup(cmd)
		cmd.Cancel = func() error { killProcessGroup(cmd); return nil }
		out, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			return errors.New("너무 오래 걸려서 멈췄어요")
		}
		if err != nil || !fileExists(filepath.Join(outDir, stem+".pdf")) {
			log.Printf("soffice %s: %v %s", src, err, lastLine(string(out)))
			return errors.New("PDF로 바꾸지 못했어요")
		}
		return nil
	}
	err := try(infilter)
	if err != nil && (ext == "hwp" || ext == "hwpx") {
		err = try("Hwp2002_Reader")
		if err != nil {
			if r, _ := t.ready(); r.H2O == "" {
				err = errors.New("한글 문서 확장이 없어서 바꾸지 못했어요")
			} else {
				err = errors.New("한글 문서를 PDF로 바꾸지 못했어요 (배포용 · 암호 문서는 안 돼요)")
			}
		}
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(outDir, stem+".pdf"), nil
}

// iworkExport asks Pages · Keynote · Numbers to export (in, out) pairs; one error per pair.
func iworkExport(appName string, pairs []string) []error {
	n := len(pairs) / 2
	errs := make([]error, n)
	script := []string{
		"on run argv",
		`set wasRunning to application "` + appName + `" is running`,
		`set res to ""`,
		"repeat with i from 1 to (count of argv) by 2",
		"set inF to POSIX file (item i of argv)",
		"set outF to POSIX file (item (i + 1) of argv)",
		"try",
		"with timeout of 600 seconds",
		`tell application "` + appName + `"`,
		"set d to open inF",
		"export d to outF as PDF",
		"close d saving no",
		"end tell",
		"end timeout",
		`set res to res & "ok" & linefeed`,
		"on error m",
		`set res to res & "err " & m & linefeed`,
		"end try",
		"end repeat",
		`if not wasRunning then tell application "` + appName + `" to quit`,
		"return res",
		"end run",
	}
	out, err := osascriptLines(script, pairs...)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := 0; i < n; i++ {
		switch {
		case err != nil:
			errs[i] = fmt.Errorf("%s을(를) 쓸 수 없어요. 시스템 설정 → 개인정보 보호 및 보안 → 자동화에서 LIBROAD를 허용해 주세요", appName)
		case i < len(lines) && strings.HasPrefix(lines[i], "ok"):
		default:
			errs[i] = fmt.Errorf("%s에서 내보내지 못했어요", appName)
		}
	}
	return errs
}

// mergePDFs joins PDFs in order (macOS PDFKit via helper.js).
func mergePDFs(list []string, dst string) error {
	if !helperAvailable() {
		return errors.New("PDF를 합칠 수 없어요")
	}
	ok := false
	runHelper("merge", list, []string{dst}, 5*time.Minute, nil, func(h helperLine) {
		if h.Path == dst && h.OK {
			ok = true
		}
	})
	if !ok || !fileExists(dst) {
		os.Remove(dst)
		return errors.New("PDF를 하나로 합치지 못했어요")
	}
	return nil
}

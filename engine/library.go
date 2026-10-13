package main

import (
	"compress/gzip"
	"encoding/gob"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 서재(Library): 고른 폴더 안의 문서 목록과 본문 글자를 기억해 두는 색인(카드 목록)이에요.
// 색인은 ~/Library/Application Support/LIBROAD/index.gob.gz 에만 저장돼요.

type DocState uint8

const (
	StNew     DocState = iota // 아직 읽지 않음
	StText                    // 본문을 읽음
	StRetry                   // PDFKit으로 다시 읽어 볼 것
	StOCRWait                 // OCR을 기다림
	StOCRDone                 // OCR까지 끝남
	StFail                    // 읽지 못함
	StEnc                     // 암호
	StCloud                   // iCloud에만 있음 (내려받지 않은 파일)
)

type Doc struct {
	Path  string
	Name  string
	Kind  string
	Ext   string
	Size  int64
	Mod   int64
	Birth int64
	Pages int
	Text  string
	OCR   string
	State DocState
	Note  string
	Pkg   bool // iWork 패키지(폴더)

	nameLow string
	cho     string
}

func (d *Doc) prep() {
	d.Name = nfc(d.Name)
	d.nameLow = lowerASCII(d.Name)
	d.cho = choseong(strings.TrimSuffix(d.Name, filepath.Ext(d.Name)))
}

type DocInfo struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Ext     string `json:"ext"`
	Kind    string `json:"kind"`
	Size    int64  `json:"size"`
	Mod     int64  `json:"mod"`
	Birth   int64  `json:"birth"`
	Dir     string `json:"dir"`
	Root    string `json:"root"`
	State   string `json:"state"`
	Note    string `json:"note,omitempty"`
	Pages   int    `json:"pages,omitempty"`
	HasText bool   `json:"hasText"`
	HasOCR  bool   `json:"hasOcr"`
	Cloud   bool   `json:"cloud,omitempty"`
}

var stateNames = map[DocState]string{StNew: "new", StText: "text", StRetry: "retry", StOCRWait: "ocrwait", StOCRDone: "ocr", StFail: "fail", StEnc: "enc", StCloud: "cloud"}

func (l *Library) info(d *Doc, roots []string) DocInfo {
	return DocInfo{Path: d.Path, Name: d.Name, Ext: d.Ext, Kind: d.Kind, Size: d.Size, Mod: d.Mod, Birth: d.Birth,
		Dir: filepath.Dir(d.Path), Root: rootOf(d.Path, roots), State: stateNames[d.State], Note: d.Note, Pages: d.Pages,
		HasText: d.Text != "", HasOCR: d.OCR != "", Cloud: d.State == StCloud}
}

func rootOf(p string, roots []string) string {
	best := ""
	for _, r := range roots {
		if isUnder(p, r) && len(r) > len(best) {
			best = r
		}
	}
	return best
}

type Library struct {
	support string

	mu    sync.RWMutex
	docs  map[string]*Doc
	list  []*Doc
	dirty bool // list를 다시 만들어야 함
	ver   atomic.Int64

	kick    chan struct{}
	ocrStop atomic.Bool

	st struct {
		sync.Mutex
		ready    bool // 첫 살피기가 끝났는지
		scanning bool
		phase    string
		lastScan int64
		found    int
		rootErr  map[string]string
	}
	saveMu    sync.Mutex
	lastSave  time.Time
	needsSave atomic.Bool
}

func NewLibrary(support string) *Library {
	l := &Library{support: support, docs: map[string]*Doc{}, kick: make(chan struct{}, 1)}
	l.st.rootErr = map[string]string{}
	return l
}

func (l *Library) indexPath() string { return filepath.Join(l.support, "index.gob.gz") }

const indexVersion = 3

func (l *Library) load() {
	f, err := os.Open(l.indexPath())
	if err != nil {
		return
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return
	}
	dec := gob.NewDecoder(zr)
	var ver int
	if dec.Decode(&ver) != nil || ver != indexVersion {
		log.Printf("index: version changed, rebuilding")
		return
	}
	var docs []*Doc
	if err := dec.Decode(&docs); err != nil {
		log.Printf("index: %v", err)
		return
	}
	l.mu.Lock()
	for _, d := range docs {
		if kindByID[d.Kind] == nil {
			continue
		}
		d.prep()
		l.docs[d.Path] = d
	}
	l.dirty = true
	l.mu.Unlock()
	l.st.Lock()
	l.st.ready = len(docs) > 0
	l.st.Unlock()
	log.Printf("index: loaded %d documents", len(docs))
}

// Save writes the index (gob + gzip) atomically.
func (l *Library) Save() {
	l.saveMu.Lock()
	defer l.saveMu.Unlock()
	l.mu.RLock()
	docs := make([]*Doc, 0, len(l.docs))
	for _, d := range l.docs {
		docs = append(docs, d)
	}
	tmp := l.indexPath() + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		l.mu.RUnlock()
		return
	}
	zw, _ := gzip.NewWriterLevel(f, gzip.BestSpeed)
	enc := gob.NewEncoder(zw)
	err = enc.Encode(indexVersion)
	if err == nil {
		err = enc.Encode(docs)
	}
	l.mu.RUnlock()
	zw.Close()
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return
	}
	os.Rename(tmp, l.indexPath())
	l.lastSave = time.Now()
	l.needsSave.Store(false)
}

func (l *Library) maybeSave() {
	if l.needsSave.Load() && time.Since(l.lastSave) > 20*time.Second {
		l.Save()
	}
}

func (l *Library) changed() {
	l.ver.Add(1)
	l.needsSave.Store(true)
}

func (l *Library) Kick() {
	select {
	case l.kick <- struct{}{}:
	default:
	}
}

func (l *Library) StopOCR() { l.ocrStop.Store(true) }

func (l *Library) loop() {
	l.load()
	go checkHelper()
	for {
		l.scanOnce()
		l.maybeSave()
		every := 3 * time.Minute
		select {
		case <-l.kick:
		case <-time.After(every):
		}
	}
}

// Docs returns a snapshot slice (do not modify the docs).
func (l *Library) Docs() []*Doc {
	l.mu.Lock()
	if l.dirty {
		l.list = make([]*Doc, 0, len(l.docs))
		for _, d := range l.docs {
			l.list = append(l.list, d)
		}
		sort.Slice(l.list, func(i, j int) bool { return l.list[i].Mod > l.list[j].Mod })
		l.dirty = false
	}
	list := l.list
	l.mu.Unlock()
	return list
}

func (l *Library) Get(p string) *Doc {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.docs[p]
}

func (l *Library) Info(p string) (DocInfo, bool) {
	roots := app.Settings().Roots
	if d := l.Get(p); d != nil {
		if _, err := os.Lstat(p); err != nil {
			l.Removed(p) // 이미 지우거나 옮긴 파일
			return DocInfo{}, false
		}
		return l.info(d, roots), true
	}
	// 서재 밖의 파일 (바구니에 직접 넣은 파일 등)
	st, err := os.Stat(p)
	if err != nil {
		return DocInfo{}, false
	}
	k := kindOf(p)
	kid := ""
	if k != nil {
		kid = k.ID
	}
	return DocInfo{Path: p, Name: nfc(filepath.Base(p)), Ext: extOf(p), Kind: kid, Size: st.Size(), Mod: st.ModTime().Unix(),
		Dir: filepath.Dir(p), State: "outside"}, true
}

// ---------------------------------------------------------------- 상태

type rootStat struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Count int    `json:"count"`
	Err   string `json:"err,omitempty"`
}

func (l *Library) Status() map[string]any {
	s := app.Settings()
	docs := l.Docs()
	byKind := map[string]int{}
	pend := map[string]int{}
	perRoot := map[string]int{}
	var bytes int64
	for _, d := range docs {
		byKind[d.Kind]++
		bytes += d.Size
		switch d.State {
		case StNew:
			pend["text"]++
		case StRetry:
			pend["retry"]++
		case StOCRWait:
			pend["ocr"]++
		case StOCRDone:
			pend["ocrDone"]++
		case StCloud:
			pend["cloud"]++
		case StEnc:
			pend["enc"]++
		case StFail:
			pend["fail"]++
		}
		for _, r := range s.Roots {
			if isUnder(d.Path, r) {
				perRoot[r]++
			}
		}
	}
	l.st.Lock()
	roots := []rootStat{}
	for _, r := range s.Roots {
		e := l.st.rootErr[r]
		for top, msg := range l.st.rootErr {
			if e == "" && msg != "" && isUnder(r, top) {
				e = msg
			}
		}
		roots = append(roots, rootStat{Path: r, Name: nfc(filepath.Base(r)), Count: perRoot[r], Err: e})
	}
	out := map[string]any{
		"docs": len(docs), "bytes": bytes, "byKind": byKind, "pending": pend, "roots": roots,
		"ready": l.st.ready, "scanning": l.st.scanning, "phase": l.st.phase, "lastScan": l.st.lastScan,
		"found": l.st.found, "ver": l.ver.Load(), "ocr": s.OCR, "ocrSupport": ocrSupported(),
	}
	l.st.Unlock()
	return out
}

func (l *Library) setPhase(p string) {
	l.st.Lock()
	l.st.phase = p
	l.st.Unlock()
}

// ---------------------------------------------------------------- 살피기 (폴더 훑기)

type found struct {
	size, mod, birth int64
	kind             string
	cloud, pkg       bool
}

var skipDirNames = map[string]bool{
	"node_modules": true, "$RECYCLE.BIN": true, "System Volume Information": true, "__MACOSX": true,
	"Photos Library.photoslibrary": true, "Library": false,
}

var skipDirExts = []string{".app", ".photoslibrary", ".photolibrary", ".musiclibrary", ".tvlibrary", ".imovielibrary",
	".fcpbundle", ".lrdata", ".bundle", ".framework", ".plugin", ".kext", ".xcodeproj", ".xcworkspace", ".git", ".logicx",
	".band", ".dSYM", ".pkg", ".mpkg", ".sparsebundle", ".vmwarevm", ".pvm", ".utm", ".photosLibrary"}

func skipDir(name string) bool {
	if strings.HasPrefix(name, ".") || skipDirNames[name] {
		return true
	}
	ln := strings.ToLower(name)
	for _, e := range skipDirExts {
		if strings.HasSuffix(ln, strings.ToLower(e)) {
			return true
		}
	}
	return false
}

const maxFiles = 400000

func (l *Library) walkRoots(roots []string, onCount func(int)) (map[string]found, map[string]string) {
	out := map[string]found{}
	errs := map[string]string{}
	fakeCloud := os.Getenv("LIBROAD_FAKE_DATALESS") != ""
	supportDir := l.support
	// 서재 안쪽 폴더를 따로 넣었으면 그 폴더는 따로 훑어요 (두 번 훑지 않게, 숨김 폴더라도 빠지지 않게)
	isRoot := map[string]bool{}
	for _, r := range roots {
		isRoot[r] = true
	}
	for _, root := range roots {
		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			errs[root] = "폴더를 찾을 수 없어요 (외장 디스크라면 연결해 주세요)"
			continue
		}
		if _, err := os.ReadDir(root); err != nil {
			errs[root] = "폴더를 열 수 없어요. 접근을 허용해 주세요"
			continue
		}
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() && p != root {
					return fs.SkipDir
				}
				return nil
			}
			name := d.Name()
			if d.IsDir() {
				if p == root {
					return nil
				}
				if p == supportDir || isRoot[p] {
					return fs.SkipDir
				}
				ext := extOf(name)
				if isPackageExt(ext) {
					if k := kindOf(name); k != nil {
						size, mod := pkgStat(p)
						out[p] = found{size: size, mod: mod, birth: mod, kind: k.ID, pkg: true}
					}
					return fs.SkipDir
				}
				if skipDir(name) {
					return fs.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~$") || !d.Type().IsRegular() {
				return nil
			}
			k := kindOf(name)
			if k == nil {
				return nil
			}
			info, err := d.Info()
			if err != nil || info.Size() == 0 {
				return nil
			}
			f := found{size: info.Size(), mod: info.ModTime().Unix(), birth: birthTime(info), kind: k.ID, cloud: isDataless(info)}
			if fakeCloud && strings.Contains(name, ".icloudtest.") {
				f.cloud = true
			}
			out[p] = f
			if len(out)%500 == 0 {
				onCount(len(out))
			}
			if len(out) >= maxFiles {
				return fs.SkipAll
			}
			return nil
		})
	}
	onCount(len(out))
	return out, errs
}

// pkgStat sums the size of an iWork package folder and finds its newest time.
func pkgStat(dir string) (int64, int64) {
	var size, mod int64
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			size += info.Size()
			if t := info.ModTime().Unix(); t > mod {
				mod = t
			}
		}
		return nil
	})
	return size, mod
}

func (l *Library) scanOnce() {
	l.ocrStop.Store(false)
	s := app.Settings()
	l.st.Lock()
	l.st.scanning = true
	l.st.phase = "walk"
	l.st.Unlock()
	defer func() {
		l.st.Lock()
		l.st.scanning = false
		l.st.phase = ""
		l.st.ready = true
		l.st.lastScan = time.Now().UnixMilli()
		l.st.Unlock()
	}()

	start := time.Now()
	var walkJob *Job
	firstScan := len(l.Docs()) == 0
	files, errs := l.walkRoots(dedupeRoots(s.Roots), func(n int) {
		l.st.Lock()
		l.st.found = n
		l.st.Unlock()
		if walkJob == nil && (firstScan || time.Since(start) > 2*time.Second) {
			walkJob = app.jobs.Start("index", "서재 살피기", "개", 0)
			walkJob.quiet = true
		}
		if walkJob != nil {
			walkJob.SetDone(int64(n))
			walkJob.SetMsg(fmt.Sprintf("문서 %s개를 찾았어요", comma(n)))
		}
	})
	if walkJob != nil {
		walkJob.Finish(nil, fmt.Sprintf("문서 %s개를 찾았어요", comma(len(files))))
	}
	l.st.Lock()
	l.st.rootErr = errs
	l.st.Unlock()

	// 바뀐 것 반영
	l.mu.Lock()
	changed := 0
	for p := range l.docs {
		if _, ok := files[p]; !ok {
			// 연결이 끊긴 외장 디스크의 문서는 지우지 않고 남겨 둬요
			root := rootOf(p, s.Roots)
			if root != "" && errs[root] != "" {
				continue
			}
			delete(l.docs, p)
			changed++
		}
	}
	for p, f := range files {
		d := l.docs[p]
		if d != nil && d.Size == f.size && d.Mod == f.mod {
			if d.State == StCloud && !f.cloud {
				d.State = StNew
				changed++
			}
			continue
		}
		if d == nil {
			d = &Doc{Path: p, Name: filepath.Base(p)}
			d.prep()
			l.docs[p] = d
		}
		d.Kind, d.Ext, d.Size, d.Mod, d.Birth, d.Pkg = f.kind, extOf(p), f.size, f.mod, f.birth, f.pkg
		d.Text, d.OCR, d.Note, d.Pages = "", "", "", 0
		d.State = StNew
		if f.cloud {
			d.State = StCloud
			d.Note = "iCloud에만 있어서 이름으로만 찾아요"
		}
		changed++
	}
	if changed > 0 {
		l.dirty = true
	}
	l.mu.Unlock()
	if changed > 0 {
		l.changed()
	}

	l.readTexts()
	l.retryWithHelper()
	l.maybeSave()
	if app.Settings().OCR {
		l.runOCR()
	}
	l.maybeSave()
}

func comma(n int) string {
	s := fmt.Sprint(n)
	if n < 1000 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// ---------------------------------------------------------------- 본문 읽기

func (l *Library) pick(state DocState, limit int) []*Doc {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := []*Doc{}
	for _, d := range l.docs {
		if d.State == state {
			out = append(out, d)
		}
	}
	// 최근 문서부터
	sort.Slice(out, func(i, j int) bool { return out[i].Mod > out[j].Mod })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (l *Library) readTexts() {
	todo := l.pick(StNew, 0)
	if len(todo) == 0 {
		return
	}
	l.setPhase("read")
	job := app.jobs.Start("read", "문서 내용 읽기", "개", int64(len(todo)))
	job.quiet = true
	workers := max(2, min(6, runtime.NumCPU()/2))
	ch := make(chan *Doc)
	var wg sync.WaitGroup
	slow := os.Getenv("LIBROAD_TEST_SLOW")
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range ch {
				if slow != "" {
					time.Sleep(time.Duration(atoiDef(slow, 0)) * time.Millisecond)
				}
				k := kindByID[d.Kind]
				res, err := extractText(d.Path, k)
				l.mu.Lock()
				if cur := l.docs[d.Path]; cur == d && d.State == StNew {
					d.Pages = res.Pages
					d.Text = res.Text
					d.Note = res.Note
					switch {
					case err != nil:
						d.State = StFail
						d.Note = friendlyReadErr(err)
					case res.Encrypted && res.Retry:
						d.State = StRetry
					case res.Encrypted:
						d.State = StEnc
					case res.Retry:
						d.State = StRetry
					case res.NeedOCR:
						d.State = StOCRWait
					default:
						d.State = StText
					}
				}
				l.mu.Unlock()
				l.changed()
				job.Add(1)
				job.SetMsg(d.Name)
			}
		}()
	}
	for _, d := range todo {
		// 새 폴더를 넣는 등 다시 살펴야 하면 잠깐 멈췄다가 이어서 읽어요
		if job.Canceled() || len(l.kick) > 0 {
			break
		}
		ch <- d
		l.maybeSave()
	}
	close(ch)
	wg.Wait()
	job.Finish(nil, fmt.Sprintf("문서 %s개를 읽었어요", comma(len(todo))))
}

func friendlyReadErr(err error) string {
	s := err.Error()
	if strings.Contains(s, "permission") || errors.Is(err, os.ErrPermission) {
		return "열어 볼 권한이 없어요"
	}
	if len([]rune(s)) > 60 {
		return "내용을 읽지 못해서 이름으로만 찾아요"
	}
	return s
}

// retryWithHelper: 글자가 깨졌거나 비어 있던 PDF를 맥의 PDFKit으로 다시 읽어요.
func (l *Library) retryWithHelper() {
	todo := l.pick(StRetry, 0)
	if len(todo) == 0 {
		return
	}
	finish := func(d *Doc, text string, pages int, enc bool) {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.docs[d.Path] != d || d.State != StRetry {
			return
		}
		if pages > 0 {
			d.Pages = pages
		}
		t := cleanText(text)
		switch {
		case enc:
			d.State, d.Note = StEnc, "암호가 걸려 있어서 이름으로만 찾아요"
		case len([]rune(t)) >= 20*max(1, min(d.Pages, 3)) && (textLooksRight(t) || len(t) > len(d.Text)):
			d.Text, d.State = t, StText
		case strings.TrimSpace(d.Text) != "" && textLooksRight(d.Text):
			d.State = StText
		default:
			// 글자가 거의 없음 → 스캔 문서
			if len(t) > len(d.Text) {
				d.Text = t
			}
			if !textLooksRight(d.Text) {
				d.Text = ""
			}
			d.State = StOCRWait
		}
	}
	if !helperAvailable() {
		for _, d := range todo {
			finish(d, "", 0, false)
		}
		l.changed()
		return
	}
	l.setPhase("retry")
	job := app.jobs.Start("read", "PDF 한 번 더 읽기", "개", int64(len(todo)))
	job.quiet = true
	for i := 0; i < len(todo); i += 20 {
		if job.Canceled() {
			break
		}
		batch := todo[i:min(i+20, len(todo))]
		paths := make([]string, len(batch))
		byPath := map[string]*Doc{}
		for k, d := range batch {
			paths[k] = d.Path
			byPath[d.Path] = d
		}
		got := map[string]bool{}
		err := runHelper("pdftext", paths, nil, 120*time.Second, job.Canceled, func(h helperLine) {
			d := byPath[h.Path]
			if d == nil || h.Tick > 0 || got[h.Path] {
				return
			}
			got[h.Path] = true
			finish(d, h.Text, h.Pages, h.Encrypted)
			job.Add(1)
			job.SetMsg(d.Name)
		})
		for _, d := range batch {
			if !got[d.Path] {
				finish(d, "", 0, false)
				job.Add(1)
			}
		}
		if err != nil {
			log.Printf("pdftext helper: %v", err)
		}
		l.changed()
		l.maybeSave()
	}
	job.Finish(nil, "")
}

// ---------------------------------------------------------------- OCR

const ocrMaxPages = 60
const ocrMaxImage = 25 << 20

func (l *Library) runOCR() {
	if !helperAvailable() {
		return
	}
	for i := 0; i < 50 && ocrSupported() == "checking"; i++ {
		time.Sleep(200 * time.Millisecond)
	}
	if !ocrReady() {
		return
	}
	todo := l.pick(StOCRWait, 0)
	if len(todo) == 0 {
		return
	}
	pagesOf := func(d *Doc) int {
		if d.Ext == "pdf" {
			return max(1, min(d.Pages, ocrMaxPages))
		}
		return 1
	}
	total := 0
	list := []*Doc{}
	for _, d := range todo {
		if d.Ext != "pdf" && d.Size > ocrMaxImage {
			l.mu.Lock()
			d.State, d.Note = StOCRDone, "사진이 너무 커서 글자 인식을 건너뛰었어요"
			l.mu.Unlock()
			continue
		}
		list = append(list, d)
		total += pagesOf(d)
	}
	if len(list) == 0 {
		return
	}
	l.setPhase("ocr")
	job := app.jobs.Start("ocr", "스캔 문서 · 사진 글자 읽기 (OCR)", "장", int64(total))
	kicked := func() bool { return len(l.kick) > 0 }
	cancel := func() bool { return job.Canceled() || l.ocrStop.Load() || !app.Settings().OCR || kicked() }
	for i := 0; i < len(list); {
		if cancel() {
			break
		}
		// 한 번에: 사진 12장, 또는 PDF 2개
		batch := []*Doc{}
		pdfs := 0
		for i < len(list) && len(batch) < 12 {
			d := list[i]
			if d.Ext == "pdf" {
				if pdfs >= 2 {
					break
				}
				pdfs++
			}
			batch = append(batch, d)
			i++
		}
		paths := make([]string, len(batch))
		byPath := map[string]*Doc{}
		for k, d := range batch {
			paths[k] = d.Path
			byPath[d.Path] = d
		}
		got := map[string]bool{}
		ticks := map[string]int{}
		err := runHelper("ocr", paths, []string{fmt.Sprint(ocrMaxPages)}, 180*time.Second, cancel, func(h helperLine) {
			d := byPath[h.Path]
			if d == nil {
				return
			}
			if h.Tick > 0 {
				ticks[h.Path]++
				job.Add(1)
				job.SetMsg(fmt.Sprintf("%s (%d/%d쪽)", d.Name, h.Page, max(1, h.Pages)))
				return
			}
			got[h.Path] = true
			l.mu.Lock()
			if l.docs[d.Path] == d && d.State == StOCRWait {
				switch {
				case h.Encrypted:
					d.State, d.Note = StEnc, "암호가 걸려 있어서 이름으로만 찾아요"
				case !h.OK:
					d.State, d.Note = StFail, "글자를 인식하지 못했어요"
				default:
					d.OCR = cleanText(h.Text)
					d.State = StOCRDone
					if d.OCR == "" {
						d.Note = "글자를 찾지 못했어요"
					}
					if h.Pages > 0 && d.Pages == 0 {
						d.Pages = h.Pages
					}
				}
			}
			l.mu.Unlock()
			// 덜 센 쪽수 맞추기
			if left := pagesOf(d) - ticks[h.Path]; left > 0 {
				job.Add(int64(left))
			}
			l.changed()
		})
		if cancel() {
			break
		}
		for _, d := range batch {
			if !got[d.Path] {
				l.mu.Lock()
				if l.docs[d.Path] == d && d.State == StOCRWait {
					d.State, d.Note = StFail, "글자를 인식하지 못했어요"
				}
				l.mu.Unlock()
				if left := pagesOf(d) - ticks[d.Path]; left > 0 {
					job.Add(int64(left))
				}
			}
		}
		if err != nil {
			log.Printf("ocr helper: %v", err)
		}
		l.maybeSave()
	}
	if kicked() && !job.Canceled() && !l.ocrStop.Load() {
		job.Finish(nil, "서재를 다시 살핀 뒤 이어서 읽어요")
	} else if cancel() {
		job.canceled.Store(true)
		job.Finish(nil, "글자 인식을 멈췄어요. 나머지는 나중에 이어서 해요")
	} else {
		job.Finish(nil, "글자 인식을 마쳤어요")
	}
}

// ---------------------------------------------------------------- 옮기기 · 지우기를 색인에 바로 반영

func (l *Library) Moved(from, to string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	moved := false
	for p, d := range l.docs {
		if p == from || strings.HasPrefix(p, from+string(filepath.Separator)) {
			np := to + p[len(from):]
			delete(l.docs, p)
			d.Path = np
			d.Name = filepath.Base(np)
			d.prep()
			if k := kindOf(np); k != nil {
				d.Kind, d.Ext = k.ID, extOf(np)
			}
			l.docs[np] = d
			moved = true
		}
	}
	if moved {
		l.dirty = true
		l.ver.Add(1)
		l.needsSave.Store(true)
	}
}

func (l *Library) Removed(p string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for q := range l.docs {
		if q == p || strings.HasPrefix(q, p+string(filepath.Separator)) {
			delete(l.docs, q)
			n++
		}
	}
	if n > 0 {
		l.dirty = true
		l.ver.Add(1)
		l.needsSave.Store(true)
	}
}

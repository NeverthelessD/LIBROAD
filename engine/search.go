package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type SearchReq struct {
	Q      string   `json:"q"`
	Scope  string   `json:"scope"` // "" = 서재 전체, 아니면 폴더 경로
	Kinds  []string `json:"kinds"`
	Period string   `json:"period"` // "" | today | week | month | year | old1 | old3 | custom
	From   int64    `json:"from"`   // custom: unix 초
	To     int64    `json:"to"`
	Size   string   `json:"size"` // "" | s | m | l | xl
	NoBody bool     `json:"noBody"`
	NoOCR  bool     `json:"noOcr"`
	Sort   string   `json:"sort"` // rel | new | old | name | size
	Limit  int      `json:"limit"`
	Offset int      `json:"offset"`
}

type seg struct {
	T string `json:"t"`
	M bool   `json:"m,omitempty"`
}

type Hit struct {
	DocInfo
	Score   int     `json:"score"`
	HName   bool    `json:"hName,omitempty"`
	HBody   bool    `json:"hBody,omitempty"`
	HOCR    bool    `json:"hOcr,omitempty"`
	HFolder bool    `json:"hFolder,omitempty"`
	Snips   [][]seg `json:"snips,omitempty"`
	Spot    bool    `json:"spot,omitempty"`
}

type term struct {
	raw     string
	low     string
	cho     bool
	exclude bool
}

func parseQuery(q string) []term {
	q = nfc(strings.TrimSpace(q))
	out := []term{}
	add := func(s string, ex bool) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		t := term{raw: s, low: lowerASCII(s), exclude: ex}
		if isChoseongQuery(s) {
			t.cho = true
			t.low = strings.ReplaceAll(t.low, " ", "")
		}
		out = append(out, t)
	}
	for q != "" {
		ex := false
		if strings.HasPrefix(q, "-") && len(q) > 1 && q[1] != ' ' {
			ex = true
			q = q[1:]
		}
		if strings.HasPrefix(q, "\"") || strings.HasPrefix(q, "“") {
			_, w := utf8.DecodeRuneInString(q)
			rest := q[w:]
			end := strings.IndexAny(rest, "\"”")
			if end < 0 {
				add(rest, ex)
				break
			}
			add(rest[:end], ex)
			_, w2 := utf8.DecodeRuneInString(rest[end:])
			q = strings.TrimSpace(rest[end+w2:])
			continue
		}
		sp := strings.IndexAny(q, " \t")
		if sp < 0 {
			add(q, ex)
			break
		}
		add(q[:sp], ex)
		q = strings.TrimSpace(q[sp:])
	}
	return out
}

func periodRange(r SearchReq) (int64, int64) {
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch r.Period {
	case "today":
		return day.Unix(), 0
	case "week":
		return day.AddDate(0, 0, -6).Unix(), 0
	case "month":
		return day.AddDate(0, -1, 0).Unix(), 0
	case "year":
		return day.AddDate(-1, 0, 0).Unix(), 0
	case "old1":
		return 0, day.AddDate(-1, 0, 0).Unix()
	case "old3":
		return 0, day.AddDate(-3, 0, 0).Unix()
	case "custom":
		to := r.To
		if to > 0 {
			to += 86399
		}
		return r.From, to
	}
	return 0, 0
}

func sizeRange(s string) (int64, int64) {
	const mb = 1 << 20
	switch s {
	case "s":
		return 0, mb
	case "m":
		return mb, 10 * mb
	case "l":
		return 10 * mb, 100 * mb
	case "xl":
		return 100 * mb, 0
	}
	return 0, 0
}

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req SearchReq
	if err := readJSON(r, &req); err != nil {
		jsonError(w, 400, "bad request")
		return
	}
	t0 := time.Now()
	res := a.lib.Search(req)
	res["took"] = time.Since(t0).Milliseconds()
	writeJSON(w, res)
}

func (l *Library) Search(req SearchReq) map[string]any {
	roots := app.Settings().Roots
	terms := parseQuery(req.Q)
	from, to := periodRange(req)
	smin, smax := sizeRange(req.Size)
	kindSet := map[string]bool{}
	for _, k := range req.Kinds {
		kindSet[k] = true
	}
	scope := filepath.Clean(req.Scope)
	if req.Scope == "" {
		scope = ""
	}
	if req.Limit <= 0 || req.Limit > 1000 {
		req.Limit = 200
	}
	if req.Sort == "" {
		if len(terms) > 0 {
			req.Sort = "rel"
		} else {
			req.Sort = "new"
		}
	}
	hasInclude := false
	for _, t := range terms {
		if !t.exclude {
			hasInclude = true
		}
	}

	docs := l.Docs()
	hits := []*Hit{}
	kindCount := map[string]int{}
	l.mu.RLock()
	for _, d := range docs {
		if scope != "" && !isUnder(d.Path, scope) {
			continue
		}
		if scope == "" && rootOf(d.Path, roots) == "" {
			continue
		}
		if from > 0 && d.Mod < from || to > 0 && d.Mod > to {
			continue
		}
		if smin > 0 && d.Size < smin || smax > 0 && d.Size >= smax {
			continue
		}
		h, ok := matchDoc(d, terms, req)
		if !ok {
			continue
		}
		kindCount[d.Kind]++
		if len(kindSet) > 0 && !kindSet[d.Kind] {
			continue
		}
		h.DocInfo = l.info(d, roots)
		hits = append(hits, h)
	}
	l.mu.RUnlock()

	// 아직 색인이 덜 됐으면 맥의 Spotlight로 본문을 한 번 더 찾아 보충해요
	pending := 0
	for _, d := range docs {
		if d.State == StNew || d.State == StRetry {
			pending++
		}
	}
	spot := 0
	if hasInclude && !req.NoBody && runtime.GOOS == "darwin" && pending > 0 {
		seen := map[string]bool{}
		for _, h := range hits {
			seen[h.Path] = true
		}
		searchRoots := roots
		if scope != "" {
			searchRoots = []string{scope}
		}
		for _, p := range spotlight(terms, searchRoots) {
			if seen[p] {
				continue
			}
			k := kindOf(p)
			if k == nil || (len(kindSet) > 0 && !kindSet[k.ID]) {
				continue
			}
			st, err := os.Stat(p)
			if err != nil {
				continue
			}
			if from > 0 && st.ModTime().Unix() < from || to > 0 && st.ModTime().Unix() > to {
				continue
			}
			h := &Hit{Spot: true, HBody: true, Score: 5}
			h.DocInfo = DocInfo{Path: p, Name: nfc(filepath.Base(p)), Ext: extOf(p), Kind: k.ID, Size: st.Size(), Mod: st.ModTime().Unix(),
				Dir: filepath.Dir(p), Root: rootOf(p, roots), State: "spotlight"}
			hits = append(hits, h)
			seen[p] = true
			spot++
		}
	}

	switch req.Sort {
	case "rel":
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].Score != hits[j].Score {
				return hits[i].Score > hits[j].Score
			}
			return hits[i].Mod > hits[j].Mod
		})
	case "old":
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].Mod < hits[j].Mod })
	case "name":
		sort.SliceStable(hits, func(i, j int) bool { return naturalLess(strings.ToLower(hits[i].Name), strings.ToLower(hits[j].Name)) })
	case "size":
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].Size > hits[j].Size })
	default:
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].Mod > hits[j].Mod })
	}
	total := len(hits)
	start := min(req.Offset, total)
	end := min(start+req.Limit, total)
	page := hits[start:end]

	// 눈에 보이는 결과만 본문 미리 보기(스니펫)를 만들어요
	if hasInclude {
		l.mu.RLock()
		for _, h := range page {
			if d := l.docs[h.Path]; d != nil {
				h.Snips = snippets(d, terms, req, 2)
			}
		}
		l.mu.RUnlock()
	}
	var totalSize int64
	for _, h := range hits {
		totalSize += h.Size
	}
	return map[string]any{
		"ok": true, "total": total, "items": page, "offset": start, "kindCount": kindCount, "totalSize": totalSize,
		"terms": termsOut(terms), "pending": pending, "spot": spot, "ver": l.ver.Load(),
	}
}

func termsOut(ts []term) []map[string]any {
	out := []map[string]any{}
	for _, t := range ts {
		out = append(out, map[string]any{"t": t.raw, "ex": t.exclude, "cho": t.cho})
	}
	return out
}

// matchDoc checks every word (AND). Must be called with l.mu read-locked.
func matchDoc(d *Doc, terms []term, req SearchReq) (*Hit, bool) {
	h := &Hit{}
	if len(terms) == 0 {
		return h, true
	}
	dirLow := ""
	score := 0
	for _, t := range terms {
		nameHit := strings.Contains(d.nameLow, t.low)
		if !nameHit && t.cho {
			nameHit = strings.Contains(strings.ReplaceAll(d.cho, " ", ""), t.low)
		}
		folderHit := false
		if !nameHit && !t.cho {
			if dirLow == "" {
				dirLow = lowerASCII(nfc(filepath.Base(filepath.Dir(d.Path))))
			}
			folderHit = strings.Contains(dirLow, t.low)
		}
		bodyN, ocrN := 0, 0
		if !req.NoBody && !t.cho {
			if d.Text != "" {
				bodyN = countFold(d.Text, t.low, 30)
			}
			if !req.NoOCR && d.OCR != "" {
				ocrN = countFold(d.OCR, t.low, 30)
			}
		}
		any := nameHit || folderHit || bodyN > 0 || ocrN > 0
		if t.exclude {
			if any {
				return nil, false
			}
			continue
		}
		if !any {
			return nil, false
		}
		if nameHit {
			h.HName = true
			score += 100
			if strings.HasPrefix(d.nameLow, t.low) {
				score += 40
			}
		}
		if folderHit {
			h.HFolder = true
			score += 25
		}
		if bodyN > 0 {
			h.HBody = true
			score += 12 + bodyN
		}
		if ocrN > 0 {
			h.HOCR = true
			score += 8 + ocrN/2
		}
	}
	// 최근 문서에 약간 가산점
	if age := time.Since(time.Unix(d.Mod, 0)); age < 30*24*time.Hour {
		score += 8 - int(age/(4*24*time.Hour))
	}
	h.Score = score
	return h, true
}

// snippets returns up to n short pieces of body text around the search words.
func snippets(d *Doc, terms []term, req SearchReq, n int) [][]seg {
	if req.NoBody {
		return nil
	}
	out := [][]seg{}
	type src struct{ text string }
	sources := []string{d.Text}
	if !req.NoOCR && d.OCR != "" {
		sources = append(sources, d.OCR)
	}
	usedAt := []int{}
	for _, text := range sources {
		if text == "" {
			continue
		}
		for _, t := range terms {
			if t.exclude || t.cho || len(out) >= n {
				continue
			}
			i := indexFold(text, t.low)
			if i < 0 {
				continue
			}
			near := false
			for _, u := range usedAt {
				if i-u < 120 && u-i < 120 {
					near = true
				}
			}
			if near {
				continue
			}
			usedAt = append(usedAt, i)
			out = append(out, markSnippet(window(text, i, len(t.low), 50, 110), terms))
		}
		if len(out) >= n {
			break
		}
	}
	return out
}

// window cuts text around [i, i+n) on rune boundaries.
func window(text string, i, n, before, after int) string {
	s := max(0, i-before*2)
	for s > 0 && s < len(text) && !utf8.RuneStart(text[s]) {
		s--
	}
	// 앞쪽은 글자 수 기준으로 before만큼
	pre := text[s:i]
	if rc := utf8.RuneCountInString(pre); rc > before {
		k := 0
		for idx := range pre {
			if k == rc-before {
				s += idx
				break
			}
			k++
		}
	}
	e := i + n
	cnt := 0
	for e < len(text) && cnt < after {
		_, w := utf8.DecodeRuneInString(text[e:])
		e += w
		cnt++
	}
	out := strings.ReplaceAll(text[s:e], "\n", " ")
	if s > 0 {
		out = "…" + out
	}
	if e < len(text) {
		out += "…"
	}
	return out
}

func markSnippet(s string, terms []term) []seg {
	mask := make([]bool, len(s))
	for _, t := range terms {
		if t.exclude || t.cho || t.low == "" {
			continue
		}
		for off := 0; off < len(s); {
			i := indexFold(s[off:], t.low)
			if i < 0 {
				break
			}
			for k := off + i; k < off+i+len(t.low) && k < len(s); k++ {
				mask[k] = true
			}
			off += i + len(t.low)
		}
	}
	out := []seg{}
	start := 0
	for i := 1; i <= len(s); i++ {
		if i == len(s) || mask[i] != mask[start] {
			out = append(out, seg{T: s[start:i], M: mask[start]})
			start = i
		}
	}
	return out
}

// spotlight asks macOS Spotlight (mdfind) for files whose content matches all words.
func spotlight(terms []term, roots []string) []string {
	parts := []string{}
	for _, t := range terms {
		if t.exclude || t.cho {
			continue
		}
		v := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `*`, ``).Replace(t.raw)
		parts = append(parts, fmt.Sprintf(`(kMDItemTextContent == "*%s*"cd || kMDItemFSName == "*%s*"cd)`, v, v))
	}
	if len(parts) == 0 {
		return nil
	}
	q := strings.Join(parts, " && ")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var mu sync.Mutex
	out := []string{}
	var wg sync.WaitGroup
	for _, r := range roots {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			b, _ := exec.CommandContext(ctx, "/usr/bin/mdfind", "-onlyin", r, q).Output()
			mu.Lock()
			for _, l := range strings.Split(string(b), "\n") {
				if l = strings.TrimSpace(l); l != "" && len(out) < 300 {
					out = append(out, l)
				}
			}
			mu.Unlock()
		}(r)
	}
	wg.Wait()
	return out
}

// ---------------------------------------------------------------- 문서 자세히 · 최근 문서

func (a *App) handleDoc(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string    `json:"path"`
		Q    string    `json:"q"`
		Req  SearchReq `json:"req"`
	}
	readJSON(r, &in)
	info, ok := a.lib.Info(in.Path)
	if !ok {
		jsonError(w, 404, "파일을 찾을 수 없어요. 옮기거나 지웠을 수 있어요")
		return
	}
	out := map[string]any{"ok": true, "info": info}
	l := a.lib
	l.mu.RLock()
	if d := l.docs[in.Path]; d != nil {
		head := func(s string, n int) string {
			if utf8.RuneCountInString(s) <= n {
				return s
			}
			k := 0
			for i := range s {
				if k == n {
					return s[:i] + "…"
				}
				k++
			}
			return s
		}
		out["head"] = head(d.Text, 1800)
		out["ocrHead"] = head(d.OCR, 1200)
		out["textLen"] = utf8.RuneCountInString(d.Text)
		out["ocrLen"] = utf8.RuneCountInString(d.OCR)
		terms := parseQuery(in.Q)
		req := in.Req
		if len(terms) > 0 {
			out["snips"] = snippets(d, terms, req, 6)
		}
	}
	l.mu.RUnlock()
	if fi, err := os.Stat(in.Path); err == nil {
		out["exists"] = true
		out["mod"] = fi.ModTime().Unix()
	}
	writeJSON(w, out)
}

func (a *App) handleDocs(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paths []string `json:"paths"`
	}
	readJSON(r, &in)
	items := []DocInfo{}
	for _, p := range in.Paths {
		if d, ok := a.lib.Info(p); ok {
			items = append(items, d)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "items": items})
}

func (a *App) handleRecent(w http.ResponseWriter, r *http.Request) {
	roots := a.Settings().Roots
	docs := a.lib.Docs()
	recent := []DocInfo{}
	added := []*Doc{}
	a.lib.mu.RLock()
	for _, d := range docs { // Docs()는 수정 날짜 최신순
		if rootOf(d.Path, roots) == "" {
			continue
		}
		if len(recent) < 18 && d.Kind != "image" {
			recent = append(recent, a.lib.info(d, roots))
		}
		added = append(added, d)
	}
	sort.SliceStable(added, func(i, j int) bool { return added[i].Birth > added[j].Birth })
	newest := []DocInfo{}
	for _, d := range added {
		if len(newest) >= 12 {
			break
		}
		if d.Kind != "image" {
			newest = append(newest, a.lib.info(d, roots))
		}
	}
	a.lib.mu.RUnlock()
	writeJSON(w, map[string]any{"ok": true, "recent": recent, "added": newest})
}

// ---------------------------------------------------------------- 미리 보기 그림 (Quick Look 썸네일)

var thumbSem = make(chan struct{}, 2)

func (a *App) handleThumb(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("p")
	size := atoiDef(r.URL.Query().Get("s"), 320)
	size = max(96, min(size, 800))
	st, err := os.Stat(p)
	if err != nil || p == "" {
		http.NotFound(w, r)
		return
	}
	ext := extOf(p)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if (ext == "jpg" || ext == "jpeg" || ext == "png") && st.Size() < 8<<20 && runtime.GOOS != "darwin" {
		http.ServeFile(w, r, p)
		return
	}
	if runtime.GOOS != "darwin" {
		http.NotFound(w, r)
		return
	}
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d|%d", p, st.Size(), st.ModTime().Unix(), size)))
	key := hex.EncodeToString(h[:10])
	dir := filepath.Join(a.support, "thumbs")
	os.MkdirAll(dir, 0o755)
	cached := filepath.Join(dir, key+".png")
	if fileExists(cached) {
		now := time.Now()
		os.Chtimes(cached, now, now)
		http.ServeFile(w, r, cached)
		return
	}
	select {
	case thumbSem <- struct{}{}:
	case <-time.After(15 * time.Second):
		http.Error(w, "busy", 503)
		return
	}
	defer func() { <-thumbSem }()
	tmp := filepath.Join(dir, "tmp-"+randID(4))
	os.MkdirAll(tmp, 0o755)
	defer os.RemoveAll(tmp)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	exec.CommandContext(ctx, "/usr/bin/qlmanage", "-t", "-s", fmt.Sprint(size), "-o", tmp, p).Run()
	ents, _ := os.ReadDir(tmp)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".png") {
			os.Rename(filepath.Join(tmp, e.Name()), cached)
			http.ServeFile(w, r, cached)
			return
		}
	}
	http.NotFound(w, r)
}

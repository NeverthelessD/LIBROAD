package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// 정리하기: 중복 문서 · 버전 묶기 · 큰 파일 · 오래된 문서 · 빈 폴더 찾기.
// 찾기만 해요. 지우기 · 옮기기는 사용자가 고른 뒤 미리 보기를 거쳐 휴지통으로.

type cleanupReq struct {
	Kind  string   `json:"kind"` // dupes | versions | big | old | empty
	Scope string   `json:"scope"`
	Exts  []string `json:"exts"` // 찾을 확장자 (비우면 모두)
	MinMB int      `json:"minMB"`
	Years int      `json:"years"`
}

func (a *App) handleCleanupStart(w http.ResponseWriter, r *http.Request) {
	var in cleanupReq
	readJSON(r, &in)
	titles := map[string]string{"dupes": "똑같은 문서 찾기", "versions": "버전 정리할 문서 찾기",
		"big": "큰 파일 찾기", "old": "오래된 문서 찾기", "empty": "빈 폴더 찾기"}
	t, ok := titles[in.Kind]
	if !ok {
		jsonError(w, 400, "알 수 없는 정리예요")
		return
	}
	job := a.jobs.Start("cleanup", t, "개", 0)
	go func() {
		res, err := runCleanup(in, job)
		if err == nil {
			job.SetResult(res)
		}
		job.Finish(err, "다 찾았어요")
	}()
	writeJSON(w, map[string]any{"ok": true, "job": job.ID()})
}

func (a *App) scopedDocs(scope string, exts []string) []*Doc {
	roots := a.Settings().Roots
	extSet := map[string]bool{}
	for _, e := range exts {
		extSet[strings.ToLower(strings.TrimPrefix(e, "."))] = true
		if e == "jpg" {
			extSet["jpeg"] = true
		}
	}
	out := []*Doc{}
	for _, d := range a.lib.Docs() {
		if len(extSet) > 0 && !extSet[d.Ext] {
			continue
		}
		if scope != "" {
			if !isUnder(d.Path, scope) {
				continue
			}
		} else if rootOf(d.Path, roots) == "" {
			continue
		}
		out = append(out, d)
	}
	return out
}

func runCleanup(in cleanupReq, job *Job) (any, error) {
	roots := app.Settings().Roots
	info := func(d *Doc) DocInfo { return app.lib.info(d, roots) }
	switch in.Kind {
	case "dupes":
		return findDupes(app.scopedDocs(in.Scope, in.Exts), job, info)
	case "versions":
		return findVersions(app.scopedDocs(in.Scope, in.Exts), info), nil
	case "big":
		min := int64(max(1, in.MinMB)) << 20
		files := []DocInfo{}
		var total int64
		for _, d := range app.scopedDocs(in.Scope, in.Exts) {
			if d.Size >= min {
				files = append(files, info(d))
				total += d.Size
			}
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Size > files[j].Size })
		return map[string]any{"files": cap2000(files), "count": len(files), "bytes": total}, nil
	case "old":
		cut := time.Now().AddDate(-max(1, in.Years), 0, 0).Unix()
		files := []DocInfo{}
		var total int64
		for _, d := range app.scopedDocs(in.Scope, in.Exts) {
			if d.Mod < cut {
				files = append(files, info(d))
				total += d.Size
			}
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Mod < files[j].Mod })
		return map[string]any{"files": cap2000(files), "count": len(files), "bytes": total}, nil
	case "empty":
		scopes := roots
		if in.Scope != "" {
			scopes = []string{in.Scope}
		}
		return map[string]any{"dirs": findEmptyDirs(scopes, job)}, nil
	}
	return nil, errors.New("알 수 없는 정리예요")
}

func cap2000(f []DocInfo) []DocInfo {
	if len(f) > 2000 {
		return f[:2000]
	}
	return f
}

// ---------------------------------------------------------------- 중복 (내용이 똑같은 파일)

type dupGroup struct {
	Size  int64     `json:"size"`
	Files []DocInfo `json:"files"`
}

func findDupes(docs []*Doc, job *Job, info func(*Doc) DocInfo) (any, error) {
	bySize := map[int64][]*Doc{}
	for _, d := range docs {
		if d.Pkg || d.State == StCloud || d.Size == 0 {
			continue
		}
		bySize[d.Size] = append(bySize[d.Size], d)
	}
	cands := [][]*Doc{}
	n := 0
	for _, g := range bySize {
		if len(g) > 1 {
			cands = append(cands, g)
			n += len(g)
		}
	}
	job.SetTotal(int64(n))
	groups := []dupGroup{}
	var wasted int64
	for _, g := range cands {
		if job.Canceled() {
			return nil, errors.New("멈췄어요")
		}
		byQuick := map[string][]*Doc{}
		for _, d := range g {
			h, err := quickHash(d.Path, d.Size)
			job.Add(1)
			if err != nil {
				continue
			}
			byQuick[h] = append(byQuick[h], d)
		}
		for _, qg := range byQuick {
			if len(qg) < 2 {
				continue
			}
			sets := [][]*Doc{qg}
			if qg[0].Size > 2*quickChunk {
				byFull := map[string][]*Doc{}
				for _, d := range qg {
					if h, err := fullHash(d.Path); err == nil {
						byFull[h] = append(byFull[h], d)
					}
				}
				sets = sets[:0]
				for _, s := range byFull {
					sets = append(sets, s)
				}
			}
			for _, s := range sets {
				if len(s) < 2 {
					continue
				}
				// 가장 먼저 만든 파일이 위로 (보통 원본)
				sort.Slice(s, func(i, j int) bool { return s[i].Birth < s[j].Birth })
				dg := dupGroup{Size: s[0].Size}
				for _, d := range s {
					dg.Files = append(dg.Files, info(d))
				}
				wasted += s[0].Size * int64(len(s)-1)
				groups = append(groups, dg)
			}
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Size*int64(len(groups[i].Files)-1) > groups[j].Size*int64(len(groups[j].Files)-1)
	})
	return map[string]any{"groups": groups, "wasted": wasted, "count": len(groups)}, nil
}

const quickChunk = 64 << 10

func quickHash(p string, size int64) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	fmt.Fprint(h, size)
	if _, err := io.CopyN(h, f, quickChunk); err != nil && err != io.EOF {
		return "", err
	}
	if size > 2*quickChunk {
		if _, err := f.Seek(-quickChunk, io.SeekEnd); err == nil {
			io.CopyN(h, f, quickChunk)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fullHash(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ---------------------------------------------------------------- 버전 묶기 ("최종", "최종2", "수정본", "(1)" …)

type verGroup struct {
	Stem  string    `json:"stem"`
	Kind  string    `json:"kind"`
	Files []DocInfo `json:"files"`
}

var verTail = []*regexp.Regexp{
	regexp.MustCompile(`[\s_\-.]*\(\s*\d+\s*\)$`),
	regexp.MustCompile(`[\s_\-.]*\[\s*\d+\s*\]$`),
	regexp.MustCompile(`(?i)[\s_\-.]*(복사본|사본|copy|복제)(\s*\d+)?$`),
	regexp.MustCompile(`(?i)[\s_\-.]*(진짜|찐|최+)?\s*최종(본|안|버전)?\s*\d*$`),
	regexp.MustCompile(`(?i)[\s_\-.]*(final|fin|last)\s*\d*$`),
	regexp.MustCompile(`(?i)[\s_\-.]*(수정|수정본|수정안|보완|보완본|검토|검토본|초안|초고|draft|원본|new|최신|백업|backup|old|edit|edited)\s*\d*$`),
	regexp.MustCompile(`(?i)[\s_\-.]*(v|ver|version|rev|r)\.?\s*\d+([._]\d+)*$`),
	regexp.MustCompile(`[\s_\-.]*\d+\s*차$`),
	regexp.MustCompile(`[\s_\-.]*\(?(19|20)\d{2}[.\-_년]?\s*\d{1,2}[.\-_월]?\s*\d{1,2}일?\)?$`),
	regexp.MustCompile(`[\s_\-.]*\(?\d{6}\)?$`),
	regexp.MustCompile(`[\s_\-.]+\d{1,2}$`),
	regexp.MustCompile(`[\s_\-.]+$`),
}

var verHead = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^[\[(【]\s*(최종|수정|final|초안|draft|검토)\s*[\])】][\s_\-.]*`),
	regexp.MustCompile(`^\(?(19|20)\d{2}[.\-_]?\d{2}[.\-_]?\d{2}\)?[\s_\-.]+`),
	regexp.MustCompile(`^\d{6}[\s_\-.]+`),
}

func versionStem(name string) string {
	s := lowerASCII(nfc(strings.TrimSuffix(name, filepath.Ext(name))))
	for i := 0; i < 8; i++ {
		before := s
		for _, re := range verTail {
			s = re.ReplaceAllString(s, "")
		}
		for _, re := range verHead {
			s = re.ReplaceAllString(s, "")
		}
		s = strings.TrimSpace(s)
		if s == before {
			break
		}
	}
	return s
}

func findVersions(docs []*Doc, info func(*Doc) DocInfo) any {
	groups := map[string][]*Doc{}
	for _, d := range docs {
		if d.Kind == "image" {
			continue
		}
		stem := versionStem(d.Name)
		if len([]rune(stem)) < 2 {
			continue
		}
		key := d.Kind + "|" + stem
		groups[key] = append(groups[key], d)
	}
	out := []verGroup{}
	for key, g := range groups {
		if len(g) < 2 {
			continue
		}
		names := map[string]bool{}
		for _, d := range g {
			names[d.Name] = true
		}
		sort.Slice(g, func(i, j int) bool { return g[i].Mod > g[j].Mod })
		vg := verGroup{Stem: strings.SplitN(key, "|", 2)[1], Kind: g[0].Kind}
		for _, d := range g {
			vg.Files = append(vg.Files, info(d))
		}
		out = append(out, vg)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Files) != len(out[j].Files) {
			return len(out[i].Files) > len(out[j].Files)
		}
		return out[i].Files[0].Mod > out[j].Files[0].Mod
	})
	if len(out) > 1000 {
		out = out[:1000]
	}
	return map[string]any{"groups": out, "count": len(out)}
}

// ---------------------------------------------------------------- 빈 폴더

type emptyDir struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Parent string `json:"parent"`
	Mod    int64  `json:"mod"`
}

func findEmptyDirs(scopes []string, job *Job) []emptyDir {
	out := []emptyDir{}
	ignorable := func(n string) bool { return n == ".DS_Store" || n == ".localized" || n == "Icon\r" }
	for _, root := range scopes {
		hasFile := map[string]bool{}
		dirs := []string{}
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() && p != root {
					// 열 수 없는 폴더는 "비어 있지 않음"으로 봐요 (안전하게)
					markUp(hasFile, p, root)
					return fs.SkipDir
				}
				return nil
			}
			if job.Canceled() {
				return fs.SkipAll
			}
			name := d.Name()
			if d.IsDir() {
				if p == root {
					return nil
				}
				if skipDir(name) || isPackageExt(extOf(name)) || strings.HasSuffix(strings.ToLower(name), ".app") {
					markUp(hasFile, p, root)
					return fs.SkipDir
				}
				dirs = append(dirs, p)
				job.Add(1)
				return nil
			}
			if ignorable(name) {
				return nil
			}
			markUp(hasFile, p, root)
			return nil
		})
		// 맨 위쪽 빈 폴더만 (안쪽 빈 폴더는 함께 지워져요)
		sort.Strings(dirs)
		lastTop := ""
		for _, d := range dirs {
			if hasFile[d] {
				continue
			}
			if lastTop != "" && isUnder(d, lastTop) {
				continue
			}
			lastTop = d
			ed := emptyDir{Path: d, Name: nfc(filepath.Base(d)), Parent: filepath.Dir(d)}
			if st, err := os.Stat(d); err == nil {
				ed.Mod = st.ModTime().Unix()
			}
			out = append(out, ed)
			if len(out) >= 3000 {
				return out
			}
		}
	}
	return out
}

// markUp marks p's parent folders (up to root) as "has a file".
func markUp(m map[string]bool, p, root string) {
	for d := filepath.Dir(p); ; d = filepath.Dir(d) {
		if m[d] {
			return
		}
		m[d] = true
		if d == root || len(d) <= len(root) || filepath.Dir(d) == d {
			return
		}
	}
}

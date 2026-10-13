package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// LIBROAD_SAMPLES=/tmp/lrhome go test -run TestSamples -v
func TestSamples(t *testing.T) {
	root := os.Getenv("LIBROAD_SAMPLES")
	if root == "" {
		t.Skip("LIBROAD_SAMPLES not set")
	}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		k := kindOf(d.Name())
		if d.IsDir() && (k == nil || !isPackageExt(extOf(d.Name()))) {
			return nil
		}
		if k == nil {
			return nil
		}
		res, err := extractText(p, k)
		txt := strings.ReplaceAll(res.Text, "\n", " ⏎ ")
		if len([]rune(txt)) > 110 {
			txt = string([]rune(txt)[:110])
		}
		t.Logf("%-28s err=%v ocr=%v retry=%v enc=%v note=%q\n      %s", filepath.Base(p), err, res.NeedOCR, res.Retry, res.Encrypted, res.Note, txt)
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
}

func TestVersionStem(t *testing.T) {
	for in, want := range map[string]string{
		"사업계획서_최종.docx": "사업계획서", "사업계획서_최종2.docx": "사업계획서", "사업계획서_수정본(1).docx": "사업계획서",
		"보고서 v2.1.hwp": "보고서", "[최종] 보고서.hwp": "보고서", "보고서_20240301.pdf": "보고서", "기획안 진짜최종.pptx": "기획안",
		"Report_final_v3.docx": "report", "회의록 (2).docx": "회의록", "회의록 - 복사본.docx": "회의록",
	} {
		if got := versionStem(in); got != want {
			t.Errorf("%s → %q, want %q", in, got, want)
		}
	}
}

func TestChoseong(t *testing.T) {
	if choseong("사업계획서 2024") != "ㅅㅇㄱㅎㅅ 2024" {
		t.Error(choseong("사업계획서 2024"))
	}
	if !isChoseongQuery("ㅅㅇㄱㅎ") || isChoseongQuery("사업") {
		t.Error("isChoseongQuery")
	}
	q := parseQuery(`사업 "도서관 검색" -초안 ㅅㅇ`)
	if len(q) != 4 || q[1].raw != "도서관 검색" || !q[2].exclude || !q[3].cho {
		t.Errorf("%+v", q)
	}
	if indexFold("Hello LIBROAD world", "libroad") != 6 {
		t.Error("indexFold")
	}
}

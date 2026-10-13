package main

import (
	"path/filepath"
	"runtime"
	"strings"
)

// Kind is one "shelf" of documents (종류). Mac-only kinds are left out on Windows.
type Kind struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Short string   `json:"short"` // 표지에 쓰는 짧은 이름
	Exts  []string `json:"exts"`
	Color string   `json:"color"`
	Emoji string   `json:"emoji"`
	Mac   bool     `json:"-"`
	Image bool     `json:"image,omitempty"` // 글자를 OCR로만 읽을 수 있는 종류
}

var allKinds = []Kind{
	{ID: "pdf", Label: "PDF", Short: "PDF", Exts: []string{"pdf"}, Color: "#d4473b", Emoji: "📕"},
	{ID: "hwp", Label: "한글", Short: "HWP", Exts: []string{"hwp", "hwpx"}, Color: "#2f7fd6", Emoji: "📘"},
	{ID: "word", Label: "워드", Short: "DOC", Exts: []string{"doc", "docx"}, Color: "#3c5fb8", Emoji: "📄"},
	{ID: "excel", Label: "엑셀 · 표", Short: "XLS", Exts: []string{"xls", "xlsx", "csv"}, Color: "#1f8a4c", Emoji: "📗"},
	{ID: "ppt", Label: "파워포인트", Short: "PPT", Exts: []string{"ppt", "pptx"}, Color: "#d9762b", Emoji: "📙"},
	{ID: "text", Label: "텍스트", Short: "TXT", Exts: []string{"txt", "md", "rtf"}, Color: "#7a7066", Emoji: "📝"},
	{ID: "pages", Label: "Pages", Short: "PAGES", Exts: []string{"pages"}, Color: "#e38a1c", Emoji: "🗒️", Mac: true},
	{ID: "keynote", Label: "Keynote", Short: "KEY", Exts: []string{"key"}, Color: "#2c8de0", Emoji: "🎞️", Mac: true},
	{ID: "numbers", Label: "Numbers", Short: "NUM", Exts: []string{"numbers"}, Color: "#2aa14a", Emoji: "🔢", Mac: true},
	{ID: "epub", Label: "전자책", Short: "EPUB", Exts: []string{"epub"}, Color: "#8b5cc4", Emoji: "📚"},
	{ID: "image", Label: "스캔 · 사진", Short: "IMG", Exts: []string{"jpg", "jpeg", "png", "heic"}, Color: "#b0822f", Emoji: "🖼️", Image: true},
}

var (
	kinds     []Kind
	kindByExt = map[string]*Kind{}
	kindByID  = map[string]*Kind{}
)

func init() {
	for _, k := range allKinds {
		if k.Mac && runtime.GOOS == "windows" {
			continue
		}
		kinds = append(kinds, k)
	}
	for i := range kinds {
		k := &kinds[i]
		kindByID[k.ID] = k
		for _, e := range k.Exts {
			kindByExt[e] = k
		}
	}
}

func extOf(name string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
}

// kindOf returns the kind for a file name, or nil when LIBROAD does not handle it.
func kindOf(name string) *Kind {
	return kindByExt[extOf(name)]
}

// isPackageExt: iWork 문서는 폴더(패키지)로 저장되기도 해요. 폴더지만 파일 하나로 다뤄요.
func isPackageExt(ext string) bool {
	if runtime.GOOS == "windows" {
		return false
	}
	return ext == "pages" || ext == "key" || ext == "numbers"
}

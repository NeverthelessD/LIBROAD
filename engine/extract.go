package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/unicode/norm"
)

const (
	maxTextBytes = 300 << 10 // 문서 하나에서 기억하는 글자 양 (약 300KB, 한글 10만 자쯤)
	maxReadBytes = 200 << 20 // 이보다 큰 파일은 본문을 읽지 않아요
	maxZipEntry  = 60 << 20  // zip 안의 파일 하나를 풀 때 최대 크기
)

// Extract is the result of reading a document's text.
type Extract struct {
	Text      string
	Pages     int
	NeedOCR   bool   // 글자가 없어서 OCR이 필요 (스캔 문서 · 사진)
	Retry     bool   // 맥의 PDFKit으로 다시 읽어 보면 좋음
	Encrypted bool   // 암호가 걸린 문서
	Note      string // 사용자에게 보여 줄 짧은 설명
}

var errEncrypted = errors.New("암호가 걸린 문서예요")

// extractText reads the body text of one document. It never panics.
func extractText(p string, k *Kind) (res Extract, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("문서를 읽다가 문제가 생겼어요: %v", r)
			_ = debug.Stack()
		}
	}()
	ext := extOf(p)
	if st, e := os.Stat(p); e == nil && !st.IsDir() && st.Size() > maxReadBytes && ext != "pdf" {
		return Extract{Note: "파일이 너무 커서 이름으로만 찾아요"}, nil
	}
	var text string
	switch ext {
	case "txt", "md", "csv":
		text, err = readPlain(p)
	case "rtf":
		text, err = readRTF(p)
	case "docx":
		text, err = readDOCX(p)
	case "pptx":
		text, err = readPPTX(p)
	case "xlsx":
		text, err = readXLSX(p)
	case "hwpx":
		text, err = readHWPX(p)
	case "epub":
		text, err = readEPUB(p)
	case "hwp":
		text, err = readHWP(p)
	case "doc":
		text, err = readDOC(p)
	case "xls":
		text, err = readXLS(p)
	case "ppt":
		text, err = readPPT(p)
	case "pages", "key", "numbers":
		text, err = readIWork(p)
	case "pdf":
		return extractPDF(p)
	case "jpg", "jpeg", "png", "heic":
		return Extract{NeedOCR: true}, nil
	default:
		return Extract{}, nil
	}
	if errors.Is(err, errEncrypted) {
		return Extract{Encrypted: true, Text: cleanText(text), Note: "암호가 걸려 있어서 이름으로만 찾아요"}, nil
	}
	if err != nil {
		// 맥에서는 textutil로 한 번 더 시도해요 (doc · rtf · docx)
		if t, e2 := textutilFallback(p, ext); e2 == nil && strings.TrimSpace(t) != "" {
			return Extract{Text: cleanText(t)}, nil
		}
		return Extract{}, err
	}
	return Extract{Text: cleanText(text)}, nil
}

// cleanText normalises (NFC), squeezes blank space and caps the size.
func cleanText(s string) string {
	if s == "" {
		return ""
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	s = norm.NFC.String(s)
	var b strings.Builder
	b.Grow(min(len(s), maxTextBytes+16))
	space, nl := false, 0
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\f' || r == '\v' || r == 0x2028 || r == 0x2029:
			nl++
			space = false
			continue
		case r == '\t' || r == ' ' || r == 0xa0 || r == 0x3000 || unicode.IsSpace(r):
			space = true
			continue
		case r < 32 || r == 0xfffd || r == 0xfeff || (r >= 0xe000 && r <= 0xf8ff):
			continue
		}
		if nl > 0 {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			nl, space = 0, false
		} else if space {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
		}
		b.WriteRune(r)
		if b.Len() >= maxTextBytes {
			break
		}
	}
	return b.String()
}

// ---------------------------------------------------------------- plain text (txt · md · csv)

func readPlain(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 8<<20))
	if err != nil {
		return "", err
	}
	return decodeBytes(b), nil
}

// decodeBytes guesses UTF-8 / UTF-16 / CP949(EUC-KR) — 예전 한국어 텍스트 파일은 CP949가 많아요.
func decodeBytes(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:])
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return utf16le(b[2:])
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return utf16be(b[2:])
	}
	// 잘린 마지막 글자 때문에 UTF-8이 아니라고 판단하지 않도록, 끝의 덜 끝난 글자는 빼고 확인
	check := b
	for k := 0; k < 3 && len(check) > 0 && !utf8.RuneStart(check[len(check)-1]); k++ {
		check = check[:len(check)-1]
	}
	if n := len(check); n > 0 && check[n-1] >= 0xC0 {
		check = check[:n-1]
	}
	if utf8.Valid(b) || utf8.Valid(check) {
		return strings.ToValidUTF8(string(b), "")
	}
	if looksUTF16LE(b) {
		return utf16le(b)
	}
	if out, err := korean.EUCKR.NewDecoder().Bytes(b); err == nil {
		return string(out)
	}
	return strings.ToValidUTF8(string(b), "")
}

func looksUTF16LE(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	zeros := 0
	n := min(len(b), 2000)
	for i := 1; i < n; i += 2 {
		if b[i] == 0 {
			zeros++
		}
	}
	return zeros > n/6
}

func utf16le(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u))
}

func utf16be(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
	}
	return string(utf16.Decode(u))
}

// ---------------------------------------------------------------- RTF

func readRTF(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 30<<20))
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(bytes.TrimSpace(b), []byte("{\\rtf")) {
		return decodeBytes(b), nil
	}
	return rtfToText(b), nil
}

// rtfToText is a small RTF reader: it keeps visible text, decodes \'hh (code page) and \uN.
func rtfToText(b []byte) string {
	var out strings.Builder
	cp := 1252
	type state struct {
		skip  bool
		ucSkp int
	}
	stack := []state{{ucSkp: 1}}
	cur := &stack[0]
	var pend []byte // \'hh bytes waiting for decoding (CP949는 2바이트)
	flush := func() {
		if len(pend) == 0 {
			return
		}
		if cp == 949 || cp == 51949 {
			if s, err := korean.EUCKR.NewDecoder().Bytes(pend); err == nil {
				out.Write(s)
			}
		} else {
			for _, c := range pend {
				out.WriteRune(cp1252(c))
			}
		}
		pend = pend[:0]
	}
	skipNext := 0 // \uN 다음에 오는 대체 글자 수
	destWords := map[string]bool{"fonttbl": true, "colortbl": true, "stylesheet": true, "info": true, "pict": true,
		"object": true, "header": false, "footer": false, "themedata": true, "colorschememapping": true, "datastore": true,
		"latentstyles": true, "listtable": true, "listoverridetable": true, "rsidtbl": true, "generator": true,
		"xmlnstbl": true, "mmathPr": true, "fldinst": true, "operator": true, "author": true, "title": true, "company": true,
		"pgdsctbl": true, "filetbl": true, "revtbl": true, "bkmkstart": true, "bkmkend": true, "shppict": true, "nonshppict": true,
		"blipuid": true, "falt": true, "panose": true, "wgrffmtfilter": true, "pnseclvl": true, "listtext": true}
	i := 0
	for i < len(b) {
		c := b[i]
		switch c {
		case '{':
			flush()
			stack = append(stack, *cur)
			cur = &stack[len(stack)-1]
			i++
		case '}':
			flush()
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
				cur = &stack[len(stack)-1]
			}
			i++
		case '\\':
			i++
			if i >= len(b) {
				break
			}
			c2 := b[i]
			if c2 == '\'' && i+2 < len(b) {
				v, err := strconv.ParseUint(string(b[i+1:i+3]), 16, 8)
				i += 3
				if skipNext > 0 {
					skipNext--
					continue
				}
				if err == nil && !cur.skip {
					pend = append(pend, byte(v))
				}
				continue
			}
			if c2 == '*' {
				cur.skip = true
				i++
				continue
			}
			if !isASCIILetter(c2) {
				flush()
				if !cur.skip {
					switch c2 {
					case '\\', '{', '}':
						out.WriteByte(c2)
					case '~':
						out.WriteByte(' ')
					case '\n', '\r':
						out.WriteByte('\n')
					}
				}
				i++
				continue
			}
			j := i
			for j < len(b) && isASCIILetter(b[j]) {
				j++
			}
			word := string(b[i:j])
			k := j
			if k < len(b) && (b[k] == '-' || (b[k] >= '0' && b[k] <= '9')) {
				k++
				for k < len(b) && b[k] >= '0' && b[k] <= '9' {
					k++
				}
			}
			param, hasParam := 0, k > j
			if hasParam {
				param, _ = strconv.Atoi(string(b[j:k]))
			}
			if k < len(b) && b[k] == ' ' {
				k++
			}
			i = k
			if word != "u" {
				flush()
			}
			switch {
			case destWords[word]:
				cur.skip = true
			case word == "ansicpg" && hasParam:
				cp = param
			case word == "uc" && hasParam:
				cur.ucSkp = param
			case word == "u" && hasParam:
				flush()
				if !cur.skip {
					r := param
					if r < 0 {
						r += 65536
					}
					out.WriteRune(rune(r))
				}
				skipNext = cur.ucSkp
			case !cur.skip && (word == "par" || word == "line" || word == "row" || word == "sect" || word == "page"):
				out.WriteByte('\n')
			case !cur.skip && (word == "tab" || word == "cell"):
				out.WriteByte('\t')
			}
		case '\r', '\n':
			i++
		default:
			if skipNext > 0 {
				skipNext--
				i++
				continue
			}
			if !cur.skip {
				if c >= 0x80 {
					pend = append(pend, c)
				} else {
					flush()
					out.WriteByte(c)
				}
			}
			i++
		}
	}
	flush()
	return out.String()
}

func isASCIILetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

var cp1252High = [32]rune{0x20ac, 0, 0x201a, 0x0192, 0x201e, 0x2026, 0x2020, 0x2021, 0x02c6, 0x2030, 0x0160, 0x2039, 0x0152, 0, 0x017d, 0,
	0, 0x2018, 0x2019, 0x201c, 0x201d, 0x2022, 0x2013, 0x2014, 0x02dc, 0x2122, 0x0161, 0x203a, 0x0153, 0, 0x017e, 0x0178}

func cp1252(c byte) rune {
	if c >= 0x80 && c < 0xa0 {
		if r := cp1252High[c-0x80]; r != 0 {
			return r
		}
		return ' '
	}
	return rune(c)
}

// ---------------------------------------------------------------- zip + XML formats

type zipDoc struct {
	zr *zip.ReadCloser
}

func openZip(p string) (*zipDoc, error) {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, fmt.Errorf("문서 파일이 손상됐거나 형식이 달라요")
	}
	return &zipDoc{zr: zr}, nil
}

func (z *zipDoc) Close() { z.zr.Close() }

func (z *zipDoc) file(name string) *zip.File {
	for _, f := range z.zr.File {
		if strings.EqualFold(f.Name, name) {
			return f
		}
	}
	return nil
}

// match returns entries whose names match a glob pattern, sorted naturally (slide2 < slide10).
func (z *zipDoc) match(pattern string) []*zip.File {
	out := []*zip.File{}
	for _, f := range z.zr.File {
		if ok, _ := path.Match(pattern, f.Name); ok {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return naturalLess(out[i].Name, out[j].Name) })
	return out
}

func (z *zipDoc) read(f *zip.File) ([]byte, error) {
	if f == nil {
		return nil, os.ErrNotExist
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxZipEntry))
}

// naturalLess compares strings treating digit runs as numbers.
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		da, db := leadingDigits(a), leadingDigits(b)
		if da != "" && db != "" {
			na, _ := strconv.Atoi(da)
			nb, _ := strconv.Atoi(db)
			if na != nb {
				return na < nb
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// xmlSpec says which elements hold text and which end a line.
type xmlSpec struct {
	text   map[string]bool // 글자를 담는 요소 (nil = 모든 글자)
	breaks map[string]bool // 끝나면 줄을 바꾸는 요소
	tabs   map[string]bool // 빈 요소지만 칸을 띄우는 요소
	skip   map[string]bool // 안의 글자를 버리는 요소
	attrs  map[string]string
}

func set(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

func xmlText(data []byte, sp xmlSpec, out *strings.Builder) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	d.CharsetReader = charsetReader
	inText, inSkip := 0, 0
	for out.Len() < maxTextBytes*2 {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := t.Name.Local
			if sp.skip[n] {
				inSkip++
			}
			if sp.text != nil && sp.text[n] {
				inText++
			}
			if sp.tabs[n] {
				out.WriteByte('\t')
			}
			if a, ok := sp.attrs[n]; ok {
				for _, at := range t.Attr {
					if at.Name.Local == a && at.Value != "" {
						out.WriteString(at.Value)
						out.WriteByte('\n')
					}
				}
			}
		case xml.EndElement:
			n := t.Name.Local
			if sp.skip[n] && inSkip > 0 {
				inSkip--
			}
			if sp.text != nil && sp.text[n] && inText > 0 {
				inText--
			}
			if sp.breaks[n] {
				out.WriteByte('\n')
			}
		case xml.CharData:
			if inSkip > 0 {
				continue
			}
			if sp.text == nil || inText > 0 {
				out.Write(t)
			}
		}
	}
}

func charsetReader(label string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "euc-kr", "cp949", "ks_c_5601-1987", "x-windows-949", "ksc5601":
		return korean.EUCKR.NewDecoder().Reader(input), nil
	case "utf-16", "utf-16le":
		b, _ := io.ReadAll(input)
		return strings.NewReader(decodeBytes(b)), nil
	}
	return input, nil
}

func readZipXML(p string, patterns []string, sp xmlSpec) (string, error) {
	z, err := openZip(p)
	if err != nil {
		return "", err
	}
	defer z.Close()
	if z.file("EncryptedPackage") != nil {
		return "", errEncrypted
	}
	var out strings.Builder
	found := false
	for _, pat := range patterns {
		for _, f := range z.match(pat) {
			b, err := z.read(f)
			if err != nil {
				continue
			}
			found = true
			xmlText(b, sp, &out)
			out.WriteByte('\n')
			if out.Len() > maxTextBytes*2 {
				return out.String(), nil
			}
		}
	}
	if !found {
		return "", fmt.Errorf("문서 안에서 본문을 찾지 못했어요")
	}
	return out.String(), nil
}

func readDOCX(p string) (string, error) {
	return readZipXML(p, []string{"word/document.xml", "word/footnotes.xml", "word/endnotes.xml", "word/header*.xml", "word/footer*.xml", "word/comments.xml"},
		xmlSpec{text: set("t", "delText"), breaks: set("p", "tr"), tabs: set("tab", "br", "cr", "tc")})
}

func readPPTX(p string) (string, error) {
	return readZipXML(p, []string{"ppt/slides/slide*.xml", "ppt/notesSlides/notesSlide*.xml"},
		xmlSpec{text: set("t"), breaks: set("p"), tabs: set("br", "tc")})
}

func readXLSX(p string) (string, error) {
	z, err := openZip(p)
	if err != nil {
		return "", err
	}
	defer z.Close()
	if z.file("EncryptedPackage") != nil {
		return "", errEncrypted
	}
	var out strings.Builder
	if b, err := z.read(z.file("xl/workbook.xml")); err == nil {
		xmlText(b, xmlSpec{text: set(), attrs: map[string]string{"sheet": "name"}}, &out)
	}
	if b, err := z.read(z.file("xl/sharedStrings.xml")); err == nil {
		xmlText(b, xmlSpec{text: set("t"), breaks: set("si")}, &out)
	}
	// 시트 안의 숫자 · 수식 결과 · 직접 입력한 글자
	for _, f := range z.match("xl/worksheets/sheet*.xml") {
		b, err := z.read(f)
		if err != nil {
			continue
		}
		xlsxSheetValues(b, &out)
		if out.Len() > maxTextBytes*2 {
			break
		}
	}
	if out.Len() == 0 && z.file("xl/workbook.xml") == nil {
		return "", fmt.Errorf("엑셀 문서가 아니에요")
	}
	return out.String(), nil
}

// xlsxSheetValues writes cell values that are not shared strings, one row per line.
func xlsxSheetValues(b []byte, out *strings.Builder) {
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	cellType := ""
	inV, inIS := false, 0
	for {
		tok, err := d.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "c":
				cellType = ""
				for _, a := range t.Attr {
					if a.Name.Local == "t" {
						cellType = a.Value
					}
				}
			case "v":
				inV = cellType != "s"
			case "is":
				inIS++
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				if inV {
					out.WriteByte('\t')
				}
				inV = false
			case "is":
				inIS--
				out.WriteByte('\t')
			case "row":
				out.WriteByte('\n')
			}
		case xml.CharData:
			if inV || inIS > 0 {
				out.Write(t)
			}
		}
		if out.Len() > maxTextBytes*2 {
			return
		}
	}
}

func readHWPX(p string) (string, error) {
	z, err := openZip(p)
	if err != nil {
		return "", err
	}
	defer z.Close()
	var out strings.Builder
	secs := z.match("Contents/section*.xml")
	if len(secs) == 0 {
		secs = z.match("Contents/*.xml")
	}
	for _, f := range secs {
		if strings.HasSuffix(strings.ToLower(f.Name), "header.xml") || strings.HasSuffix(strings.ToLower(f.Name), "content.hpf") {
			continue
		}
		b, err := z.read(f)
		if err != nil {
			continue
		}
		xmlText(b, xmlSpec{text: set("t"), breaks: set("p"), tabs: set("tab", "lineBreak")}, &out)
	}
	if out.Len() == 0 {
		// 암호가 걸린 hwpx는 본문 대신 미리 보기 글자만 있어요
		if b, err := z.read(z.file("Preview/PrvText.txt")); err == nil {
			return decodeBytes(b), nil
		}
		if len(secs) == 0 {
			return "", fmt.Errorf("한글 문서 안에서 본문을 찾지 못했어요")
		}
	}
	return out.String(), nil
}

func readEPUB(p string) (string, error) {
	z, err := openZip(p)
	if err != nil {
		return "", err
	}
	defer z.Close()
	var out strings.Builder
	files := []*zip.File{}
	for _, f := range z.zr.File {
		n := strings.ToLower(f.Name)
		if strings.HasSuffix(n, ".xhtml") || strings.HasSuffix(n, ".html") || strings.HasSuffix(n, ".htm") {
			files = append(files, f)
		}
	}
	// 책 순서(spine)를 따르면 좋지만, 찾기에는 순서가 크게 중요하지 않아요
	sort.Slice(files, func(i, j int) bool { return naturalLess(files[i].Name, files[j].Name) })
	sp := xmlSpec{breaks: set("p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr", "title"), skip: set("script", "style")}
	if opf := z.match("*.opf"); len(opf) > 0 {
		if b, err := z.read(opf[0]); err == nil {
			xmlText(b, xmlSpec{text: set("title", "creator", "publisher", "description", "subject"), breaks: set("title", "creator", "publisher", "description", "subject")}, &out)
		}
	} else if opf := z.match("*/*.opf"); len(opf) > 0 {
		if b, err := z.read(opf[0]); err == nil {
			xmlText(b, xmlSpec{text: set("title", "creator", "publisher", "description", "subject"), breaks: set("title", "creator", "publisher", "description", "subject")}, &out)
		}
	}
	for _, f := range files {
		b, err := z.read(f)
		if err != nil {
			continue
		}
		xmlText(b, sp, &out)
		if out.Len() > maxTextBytes*2 {
			break
		}
	}
	if len(files) == 0 {
		return "", fmt.Errorf("전자책 안에서 본문을 찾지 못했어요")
	}
	return out.String(), nil
}

package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"context"
)

// ---------------------------------------------------------------- 한글 (HWP 5.0)

func readHWP(p string) (string, error) {
	c, err := openCFB(p)
	if err != nil {
		// HWP 3.0 (아주 예전 한글) 등은 지원하지 않아요
		return "", fmt.Errorf("이 한글 문서 형식은 읽을 수 없어요 (오래된 한글 3.0 형식일 수 있어요)")
	}
	hdr, err := c.Stream("FileHeader")
	if err != nil || len(hdr) < 40 || !bytes.HasPrefix(hdr, []byte("HWP Document File")) {
		return "", fmt.Errorf("한글 문서가 아니에요")
	}
	props := binary.LittleEndian.Uint32(hdr[36:])
	compressed := props&1 != 0
	password := props&2 != 0
	distribution := props&4 != 0

	var out strings.Builder
	if !password && !distribution {
		secs := c.List("BodyText")
		sort.Slice(secs, func(i, j int) bool { return naturalLess(secs[i], secs[j]) })
		for _, s := range secs {
			if !strings.HasPrefix(s, "Section") {
				continue
			}
			raw, err := c.Stream("BodyText", s)
			if err != nil {
				continue
			}
			if compressed {
				raw, err = inflateRaw(raw, 64<<20)
				if err != nil && len(raw) == 0 {
					continue
				}
			}
			hwpRecords(raw, &out)
			if out.Len() > maxTextBytes*2 {
				break
			}
		}
	}
	if strings.TrimSpace(out.String()) == "" {
		// 배포용 · 암호 문서는 본문이 암호화돼 있어요 → 미리 보기 글자(첫 쪽 정도)라도 읽어요
		if b, err := c.Stream("PrvText"); err == nil {
			prv := utf16le(b)
			if password {
				return prv, errEncrypted
			}
			return prv, nil
		}
		if password {
			return "", errEncrypted
		}
	}
	return out.String(), nil
}

func inflateRaw(b []byte, limit int64) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(b))
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, limit))
}

// hwpRecords walks HWP records and writes paragraph text (HWPTAG_PARA_TEXT = 67).
func hwpRecords(b []byte, out *strings.Builder) {
	le := binary.LittleEndian
	for i := 0; i+4 <= len(b); {
		h := le.Uint32(b[i:])
		tag := h & 0x3FF
		size := int(h >> 20)
		i += 4
		if size == 0xFFF {
			if i+4 > len(b) {
				return
			}
			size = int(le.Uint32(b[i:]))
			i += 4
		}
		if size < 0 || i+size > len(b) {
			return
		}
		if tag == 67 {
			hwpParaText(b[i:i+size], out)
			out.WriteByte('\n')
		}
		i += size
	}
}

func hwpParaText(b []byte, out *strings.Builder) {
	n := len(b) / 2
	u := make([]uint16, 0, n)
	flush := func() {
		if len(u) > 0 {
			out.WriteString(string(utf16.Decode(u)))
			u = u[:0]
		}
	}
	for i := 0; i < n; {
		c := uint16(b[2*i]) | uint16(b[2*i+1])<<8
		if c >= 32 {
			u = append(u, c)
			i++
			continue
		}
		switch c {
		case 10, 13:
			flush()
			out.WriteByte('\n')
			i++
		case 9: // 탭 (인라인 컨트롤, 8칸)
			flush()
			out.WriteByte('\t')
			i += 8
		case 0, 24, 25, 26, 27, 28, 29:
			i++
		case 30, 31: // 묶음 빈칸 · 고정폭 빈칸
			u = append(u, ' ')
			i++
		default: // 표 · 그림 · 각주 같은 확장 컨트롤 (8칸)
			flush()
			out.WriteByte(' ')
			i += 8
		}
	}
	flush()
}

// ---------------------------------------------------------------- 워드 97-2003 (.doc)

func readDOC(p string) (string, error) {
	c, err := openCFB(p)
	if err != nil {
		return "", err
	}
	wd, err := c.Stream("WordDocument")
	if err != nil || len(wd) < 0x200 {
		return "", fmt.Errorf("워드 문서가 아니에요")
	}
	le := binary.LittleEndian
	if le.Uint16(wd) != 0xA5EC {
		return "", fmt.Errorf("워드 문서가 아니에요")
	}
	nFib := le.Uint16(wd[2:])
	flags := le.Uint16(wd[0x0A:])
	if flags&0x0100 != 0 {
		return "", errEncrypted
	}
	if nFib < 101 {
		return "", fmt.Errorf("아주 오래된 워드 형식이에요")
	}
	tableName := "0Table"
	if flags&0x0200 != 0 {
		tableName = "1Table"
	}
	table, err := c.Stream(tableName)
	if err != nil {
		return "", fmt.Errorf("워드 문서의 표 정보를 찾지 못했어요")
	}
	// FIB: FibBase(32) + csw + fibRgW + cslw + fibRgLw + cbRgFcLcb + fibRgFcLcb
	off := 32
	csw := int(le.Uint16(wd[off:]))
	off += 2 + csw*2
	cslw := int(le.Uint16(wd[off:]))
	off += 2 + cslw*4
	off += 2
	clxPos := off + 33*8
	if clxPos+8 > len(wd) {
		return "", fmt.Errorf("워드 문서 구조가 달라요")
	}
	fcClx := int(le.Uint32(wd[clxPos:]))
	lcbClx := int(le.Uint32(wd[clxPos+4:]))
	if fcClx < 0 || lcbClx <= 0 || fcClx+lcbClx > len(table) {
		return "", fmt.Errorf("워드 문서 구조가 달라요")
	}
	clx := table[fcClx : fcClx+lcbClx]
	i := 0
	for i < len(clx) && clx[i] == 0x01 { // Prc (서식) 건너뛰기
		if i+3 > len(clx) {
			break
		}
		cb := int(int16(le.Uint16(clx[i+1:])))
		i += 3 + cb
	}
	if i+5 > len(clx) || clx[i] != 0x02 {
		return "", fmt.Errorf("워드 문서 구조가 달라요")
	}
	lcb := int(le.Uint32(clx[i+1:]))
	plc := clx[i+5:]
	if lcb > len(plc) {
		lcb = len(plc)
	}
	plc = plc[:lcb]
	n := (lcb - 4) / 12
	if n <= 0 {
		return "", nil
	}
	var raw []rune
	for k := 0; k < n; k++ {
		cp0 := int(le.Uint32(plc[4*k:]))
		cp1 := int(le.Uint32(plc[4*(k+1):]))
		pcd := plc[4*(n+1)+8*k:]
		fc := le.Uint32(pcd[2:])
		cnt := cp1 - cp0
		if cnt <= 0 || cnt > 50<<20 {
			continue
		}
		if fc&0x40000000 != 0 {
			start := int(fc&0x3FFFFFFF) / 2
			if start+cnt > len(wd) {
				continue
			}
			for _, ch := range wd[start : start+cnt] {
				raw = append(raw, cp1252(ch))
			}
		} else {
			start := int(fc)
			if start+2*cnt > len(wd) {
				continue
			}
			u := make([]uint16, cnt)
			for q := range u {
				u[q] = le.Uint16(wd[start+2*q:])
			}
			raw = append(raw, utf16.Decode(u)...)
		}
		if len(raw) > maxTextBytes {
			break
		}
	}
	return wordClean(raw), nil
}

// wordClean turns Word's special characters into plain text and drops field codes.
func wordClean(rs []rune) string {
	var b strings.Builder
	depth := 0 // 필드 코드(0x13 ~ 0x14) 안이면 건너뛰기
	for _, r := range rs {
		switch r {
		case 0x13:
			depth++
			continue
		case 0x14:
			if depth > 0 {
				depth--
			}
			continue
		case 0x15:
			continue
		}
		if depth > 0 {
			continue
		}
		switch {
		case r == 0x0D || r == 0x0B || r == 0x0C || r == 0x0E:
			b.WriteByte('\n')
		case r == 0x07 || r == 0x09:
			b.WriteByte('\t')
		case r == 0x1E:
			b.WriteByte('-')
		case r < 32:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------- 엑셀 97-2003 (.xls)

func readXLS(p string) (string, error) {
	c, err := openCFB(p)
	if err != nil {
		return "", err
	}
	wb, err := c.Stream("Workbook")
	if err != nil {
		wb, err = c.Stream("Book")
	}
	if err != nil {
		return "", fmt.Errorf("엑셀 문서가 아니에요")
	}
	le := binary.LittleEndian
	var out strings.Builder
	type rec struct {
		id   uint16
		data []byte
	}
	recs := []rec{}
	for i := 0; i+4 <= len(wb); {
		id := le.Uint16(wb[i:])
		ln := int(le.Uint16(wb[i+2:]))
		if i+4+ln > len(wb) {
			break
		}
		recs = append(recs, rec{id, wb[i+4 : i+4+ln]})
		i += 4 + ln
	}
	for k := 0; k < len(recs); k++ {
		r := recs[k]
		switch r.id {
		case 0x002F: // FILEPASS
			return out.String(), errEncrypted
		case 0x0085: // BOUNDSHEET: 시트 이름
			if len(r.data) > 8 {
				cch := int(r.data[6])
				out.WriteString(xlsChars(r.data[8:], cch, r.data[7]&1 != 0))
				out.WriteByte('\n')
			}
		case 0x00FC: // SST: 모든 글자 칸
			segs := [][]byte{r.data}
			for k+1 < len(recs) && recs[k+1].id == 0x003C {
				k++
				segs = append(segs, recs[k].data)
			}
			xlsSST(segs, &out)
		case 0x0204: // LABEL
			if len(r.data) > 9 {
				cch := int(le.Uint16(r.data[6:]))
				out.WriteString(xlsChars(r.data[9:], cch, r.data[8]&1 != 0))
				out.WriteByte('\n')
			}
		}
		if out.Len() > maxTextBytes*2 {
			break
		}
	}
	return out.String(), nil
}

func xlsChars(b []byte, cch int, wide bool) string {
	if wide {
		n := min(cch*2, len(b)&^1)
		return utf16le(b[:n])
	}
	n := min(cch, len(b))
	var s strings.Builder
	for _, c := range b[:n] {
		s.WriteRune(cp1252(c))
	}
	return s.String()
}

// sstReader reads across SST + CONTINUE records.
type sstReader struct {
	segs [][]byte
	si   int
	pos  int
}

func (r *sstReader) left() int {
	if r.si >= len(r.segs) {
		return 0
	}
	return len(r.segs[r.si]) - r.pos
}

func (r *sstReader) next() bool {
	r.si++
	r.pos = 0
	return r.si < len(r.segs)
}

func (r *sstReader) bytes(n int) ([]byte, bool) {
	out := make([]byte, 0, n)
	for len(out) < n {
		if r.left() == 0 && !r.next() {
			return out, false
		}
		take := min(n-len(out), r.left())
		out = append(out, r.segs[r.si][r.pos:r.pos+take]...)
		r.pos += take
	}
	return out, true
}

func (r *sstReader) skip(n int) bool {
	_, ok := r.bytes(n)
	return ok
}

func xlsSST(segs [][]byte, out *strings.Builder) {
	r := &sstReader{segs: segs}
	if !r.skip(8) {
		return
	}
	le := binary.LittleEndian
	for count := 0; count < 1<<20; count++ {
		if r.left() == 0 && !r.next() {
			return
		}
		h, ok := r.bytes(3)
		if !ok {
			return
		}
		cch := int(le.Uint16(h))
		fl := h[2]
		runs, ext := 0, 0
		if fl&0x08 != 0 {
			b, ok := r.bytes(2)
			if !ok {
				return
			}
			runs = int(le.Uint16(b))
		}
		if fl&0x04 != 0 {
			b, ok := r.bytes(4)
			if !ok {
				return
			}
			ext = int(le.Uint32(b))
		}
		wide := fl&1 != 0
		var u []uint16
		got := 0
		for got < cch {
			if r.left() == 0 {
				if !r.next() {
					return
				}
				// CONTINUE 처음에는 글자 폭을 알려 주는 1바이트가 다시 와요
				f, ok := r.bytes(1)
				if !ok {
					return
				}
				wide = f[0]&1 != 0
			}
			w := 1
			if wide {
				w = 2
			}
			n := min(cch-got, r.left()/w)
			if n == 0 {
				r.pos = len(r.segs[r.si])
				continue
			}
			b, _ := r.bytes(n * w)
			for q := 0; q < n; q++ {
				if wide {
					u = append(u, le.Uint16(b[2*q:]))
				} else {
					u = append(u, uint16(cp1252(b[q])))
				}
			}
			got += n
		}
		out.WriteString(string(utf16.Decode(u)))
		out.WriteByte('\n')
		if runs > 0 && !r.skip(runs*4) {
			return
		}
		if ext > 0 && !r.skip(ext) {
			return
		}
		if out.Len() > maxTextBytes*2 {
			return
		}
	}
}

// ---------------------------------------------------------------- 파워포인트 97-2003 (.ppt)

func readPPT(p string) (string, error) {
	c, err := openCFB(p)
	if err != nil {
		return "", err
	}
	if c.find("EncryptedSummary") >= 0 {
		return "", errEncrypted
	}
	doc, err := c.Stream("PowerPoint Document")
	if err != nil {
		return "", fmt.Errorf("파워포인트 문서가 아니에요")
	}
	var out strings.Builder
	pptRecords(doc, &out, 0)
	return out.String(), nil
}

var pptNoise = []string{"Click to edit", "마스터 제목 스타일 편집", "마스터 텍스트 스타일을 편집", "둘째 수준", "셋째 수준", "넷째 수준", "다섯째 수준", "Second level", "Third level", "Fourth level", "Fifth level", "*"}

func pptRecords(b []byte, out *strings.Builder, depth int) {
	le := binary.LittleEndian
	if depth > 16 {
		return
	}
	for i := 0; i+8 <= len(b); {
		verInst := le.Uint16(b[i:])
		typ := le.Uint16(b[i+2:])
		ln := int(le.Uint32(b[i+4:]))
		i += 8
		if ln < 0 || i+ln > len(b) {
			return
		}
		body := b[i : i+ln]
		if verInst&0x0F == 0x0F {
			pptRecords(body, out, depth+1)
		} else {
			s := ""
			switch typ {
			case 0x0FA0: // TextCharsAtom
				s = utf16le(body)
			case 0x0FA8: // TextBytesAtom
				var sb strings.Builder
				for _, c := range body {
					sb.WriteRune(cp1252(c))
				}
				s = sb.String()
			}
			if s != "" && !isPPTNoise(s) {
				out.WriteString(strings.ReplaceAll(s, "\r", "\n"))
				out.WriteByte('\n')
			}
		}
		i += ln
		if out.Len() > maxTextBytes*2 {
			return
		}
	}
}

func isPPTNoise(s string) bool {
	t := strings.TrimSpace(s)
	for _, n := range pptNoise {
		if t == n || (len(n) > 2 && strings.HasPrefix(t, n)) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- 맥의 textutil (doc · docx · rtf 보조)

func textutilFallback(p, ext string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("no textutil")
	}
	switch ext {
	case "doc", "docx", "rtf", "txt":
	default:
		return "", errors.New("unsupported")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/textutil", "-convert", "txt", "-stdout", "-encoding", "UTF-8", p).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

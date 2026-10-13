package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// iWork (Pages · Keynote · Numbers) — 2013년 이후 형식은 "IWA"(snappy로 압축한 protobuf),
// 그 이전('09) 형식은 XML이에요. 문서는 zip 파일이거나 폴더(패키지)예요.

func readIWork(p string) (string, error) {
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	var iwas [][]byte
	var xmls [][]byte
	if st.IsDir() {
		iwas, xmls = iworkFromDir(p)
	} else {
		zr, err := zip.OpenReader(p)
		if err != nil {
			return "", fmt.Errorf("문서 파일이 손상됐거나 형식이 달라요")
		}
		defer zr.Close()
		iwas, xmls = iworkFromZip(&zr.Reader)
	}
	var out strings.Builder
	for _, b := range iwas {
		iwaText(b, &out, false)
		if out.Len() > maxTextBytes*2 {
			break
		}
	}
	if out.Len() == 0 {
		// 알려진 본문 위치에서 못 찾으면, 글자처럼 보이는 문자열을 넓게 모아요
		for _, b := range iwas {
			iwaText(b, &out, true)
			if out.Len() > maxTextBytes*2 {
				break
			}
		}
	}
	if out.Len() == 0 {
		for _, b := range xmls {
			xmlText(b, xmlSpec{skip: set("metadata", "stylesheet", "styles", "theme", "data", "calc-engine", "headers", "grid-column", "grid-row")}, &out)
		}
	}
	if out.Len() == 0 && len(iwas) == 0 && len(xmls) == 0 {
		return "", fmt.Errorf("iWork 문서 안에서 본문을 찾지 못했어요")
	}
	return out.String(), nil
}

func iworkFromZip(zr *zip.Reader) (iwas, xmls [][]byte) {
	files := append([]*zip.File{}, zr.File...)
	sort.Slice(files, func(i, j int) bool { return naturalLess(files[i].Name, files[j].Name) })
	for _, f := range files {
		n := f.Name
		ln := strings.ToLower(n)
		switch {
		case strings.HasSuffix(ln, ".iwa"):
			if b := readZipFile(f); b != nil {
				iwas = append(iwas, b)
			}
		case strings.HasSuffix(ln, "index.zip"):
			// 중간 형식: 바깥 zip 안에 Index.zip 이 또 있어요
			if b := readZipFile(f); b != nil {
				if inner, err := zip.NewReader(bytes.NewReader(b), int64(len(b))); err == nil {
					a, x := iworkFromZip(inner)
					iwas = append(iwas, a...)
					xmls = append(xmls, x...)
				}
			}
		case ln == "index.xml" || ln == "index.apxl" || ln == "index.xml.gz" || ln == "index.apxl.gz":
			if b := readZipFile(f); b != nil {
				if strings.HasSuffix(ln, ".gz") {
					b = gunzip(b)
				}
				xmls = append(xmls, b)
			}
		}
	}
	return
}

func iworkFromDir(dir string) (iwas, xmls [][]byte) {
	if b, err := os.ReadFile(filepath.Join(dir, "Index.zip")); err == nil {
		if zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b))); err == nil {
			return iworkFromZip(zr)
		}
	}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ln := strings.ToLower(d.Name())
		switch {
		case strings.HasSuffix(ln, ".iwa"):
			if b, err := os.ReadFile(p); err == nil {
				iwas = append(iwas, b)
			}
		case ln == "index.xml" || ln == "index.apxl":
			if b, err := os.ReadFile(p); err == nil {
				xmls = append(xmls, b)
			}
		case ln == "index.xml.gz" || ln == "index.apxl.gz":
			if b, err := os.ReadFile(p); err == nil {
				xmls = append(xmls, gunzip(b))
			}
		}
		return nil
	})
	return
}

func readZipFile(f *zip.File) []byte {
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, maxZipEntry))
	if err != nil {
		return nil
	}
	return b
}

func gunzip(b []byte) []byte {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	out, _ := io.ReadAll(io.LimitReader(r, maxZipEntry))
	return out
}

// iwaDecompress undoes IWA framing: [0x00][3-byte length][snappy block] …
func iwaDecompress(b []byte) ([]byte, error) {
	var out []byte
	for i := 0; i+4 <= len(b); {
		if b[i] != 0 {
			return out, errors.New("bad iwa chunk")
		}
		n := int(b[i+1]) | int(b[i+2])<<8 | int(b[i+3])<<16
		i += 4
		if i+n > len(b) {
			return out, errors.New("short iwa chunk")
		}
		d, err := snappyDecode(b[i : i+n])
		if err != nil {
			return out, err
		}
		out = append(out, d...)
		i += n
		if len(out) > maxZipEntry {
			break
		}
	}
	return out, nil
}

// iwaText reads the archive objects of one .iwa file and writes the text they hold.
//   - TSWP.StorageArchive (2001 · 2005 등): field 3 = repeated string (본문)
//   - TST.TableDataList (6005): field 3 = entries { field 3 = string } (Numbers 칸 글자)
func iwaText(raw []byte, out *strings.Builder, loose bool) {
	data, err := iwaDecompress(raw)
	if len(data) == 0 {
		_ = err
		return
	}
	found := false
	for i := 0; i < len(data); {
		ln, n := pbVarint(data[i:])
		if n <= 0 || i+n+int(ln) > len(data) {
			break
		}
		info := data[i+n : i+n+int(ln)]
		i += n + int(ln)
		for _, mi := range pbFields(info, 2) {
			var typ, size uint64
			for _, f := range pbAll(mi) {
				if f.num == 1 && f.wire == 0 {
					typ = f.v
				}
				if f.num == 3 && f.wire == 0 {
					size = f.v
				}
			}
			if i+int(size) > len(data) || size == 0 {
				i = len(data)
				break
			}
			msg := data[i : i+int(size)]
			i += int(size)
			if loose {
				pbStrings(msg, out, 0)
				continue
			}
			switch {
			case typ == 2001 || typ == 2005:
				for _, s := range pbFields(msg, 3) {
					if utf8.Valid(s) {
						out.Write(s)
						out.WriteByte('\n')
						found = true
					}
				}
			case typ == 6005:
				for _, e := range pbFields(msg, 3) {
					for _, s := range pbFields(e, 3) {
						if utf8.Valid(s) {
							out.Write(s)
							out.WriteByte('\n')
							found = true
						}
					}
				}
			}
		}
	}
	_ = found
}

// pbStrings collects human-looking strings from any message (느슨한 방법).
func pbStrings(b []byte, out *strings.Builder, depth int) {
	if depth > 5 {
		return
	}
	for _, f := range pbAll(b) {
		if f.wire != 2 || len(f.b) < 2 {
			continue
		}
		if looksLikeText(f.b) {
			out.Write(f.b)
			out.WriteByte('\n')
			continue
		}
		pbStrings(f.b, out, depth+1)
	}
}

func looksLikeText(b []byte) bool {
	if len(b) > 20000 || !utf8.Valid(b) {
		return false
	}
	s := string(b)
	letters, total, hangul, spaces := 0, 0, 0, 0
	for _, r := range s {
		total++
		switch {
		case r < 32 && r != '\n' && r != '\t':
			return false
		case r >= 0xAC00 && r <= 0xD7A3:
			hangul++
			letters++
		case r == ' ':
			spaces++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			letters++
		case r > 0x7f:
			letters++
		}
	}
	if total < 2 || letters*10 < total*6 {
		return false
	}
	// 영문 식별자(예: paragraphStyle-1)는 빼고, 한글이 있거나 띄어쓰기가 있는 것만
	return hangul > 0 || (spaces > 0 && total >= 4)
}

type pbField struct {
	num  uint64
	wire int
	v    uint64
	b    []byte
}

func pbVarint(b []byte) (uint64, int) {
	var x uint64
	var s uint
	for i, c := range b {
		if i >= 10 {
			return 0, -1
		}
		x |= uint64(c&0x7f) << s
		if c < 0x80 {
			return x, i + 1
		}
		s += 7
	}
	return 0, -1
}

// pbAll parses one protobuf message into fields (stops quietly at bad data).
func pbAll(b []byte) []pbField {
	out := []pbField{}
	for i := 0; i < len(b); {
		key, n := pbVarint(b[i:])
		if n <= 0 {
			return out
		}
		i += n
		f := pbField{num: key >> 3, wire: int(key & 7)}
		switch f.wire {
		case 0:
			v, n := pbVarint(b[i:])
			if n <= 0 {
				return out
			}
			f.v = v
			i += n
		case 1:
			i += 8
		case 5:
			i += 4
		case 2:
			l, n := pbVarint(b[i:])
			if n <= 0 || i+n+int(l) > len(b) || int(l) < 0 {
				return out
			}
			f.b = b[i+n : i+n+int(l)]
			i += n + int(l)
		default:
			return out
		}
		if i > len(b) {
			return out
		}
		out = append(out, f)
	}
	return out
}

func pbFields(b []byte, num uint64) [][]byte {
	out := [][]byte{}
	for _, f := range pbAll(b) {
		if f.num == num && f.wire == 2 {
			out = append(out, f.b)
		}
	}
	return out
}

// ---------------------------------------------------------------- snappy (raw block)

func snappyDecode(src []byte) ([]byte, error) {
	dLen, n := pbVarint(src)
	if n <= 0 || dLen > 1<<28 {
		return nil, errors.New("snappy: bad length")
	}
	dst := make([]byte, 0, dLen)
	s := n
	for s < len(src) {
		tag := src[s]
		switch tag & 3 {
		case 0: // literal
			l := int(tag >> 2)
			s++
			switch {
			case l < 60:
			case l == 60:
				if s >= len(src) {
					return nil, errors.New("snappy: short")
				}
				l = int(src[s])
				s++
			case l == 61:
				if s+1 >= len(src) {
					return nil, errors.New("snappy: short")
				}
				l = int(src[s]) | int(src[s+1])<<8
				s += 2
			case l == 62:
				if s+2 >= len(src) {
					return nil, errors.New("snappy: short")
				}
				l = int(src[s]) | int(src[s+1])<<8 | int(src[s+2])<<16
				s += 3
			default:
				if s+3 >= len(src) {
					return nil, errors.New("snappy: short")
				}
				l = int(src[s]) | int(src[s+1])<<8 | int(src[s+2])<<16 | int(src[s+3])<<24
				s += 4
			}
			l++
			if l <= 0 || s+l > len(src) {
				return nil, errors.New("snappy: bad literal")
			}
			dst = append(dst, src[s:s+l]...)
			s += l
			continue
		case 1:
			if s+1 >= len(src) {
				return nil, errors.New("snappy: short")
			}
			length := 4 + int(tag>>2)&7
			off := int(tag&0xe0)<<3 | int(src[s+1])
			s += 2
			if err := snappyCopy(&dst, off, length); err != nil {
				return nil, err
			}
		case 2:
			if s+2 >= len(src) {
				return nil, errors.New("snappy: short")
			}
			length := 1 + int(tag>>2)
			off := int(src[s+1]) | int(src[s+2])<<8
			s += 3
			if err := snappyCopy(&dst, off, length); err != nil {
				return nil, err
			}
		case 3:
			if s+4 >= len(src) {
				return nil, errors.New("snappy: short")
			}
			length := 1 + int(tag>>2)
			off := int(src[s+1]) | int(src[s+2])<<8 | int(src[s+3])<<16 | int(src[s+4])<<24
			s += 5
			if err := snappyCopy(&dst, off, length); err != nil {
				return nil, err
			}
		}
	}
	return dst, nil
}

func snappyCopy(dst *[]byte, off, length int) error {
	d := *dst
	if off <= 0 || off > len(d) {
		return errors.New("snappy: bad offset")
	}
	start := len(d) - off
	for k := 0; k < length; k++ {
		d = append(d, d[start+k])
	}
	*dst = d
	return nil
}

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"unicode/utf16"
)

// CFB reads Microsoft "Compound File Binary" (OLE2) files — the container of
// hwp(한글 5.0) · doc · xls · ppt.
type CFB struct {
	data     []byte
	secSize  int
	miniSize int
	cutoff   uint32
	fat      []uint32
	miniFat  []uint32
	dir      []cfbEntry
	miniStrm []byte
	nSectors int
}

type cfbEntry struct {
	name  string
	typ   byte // 1 storage, 2 stream, 5 root
	left  uint32
	right uint32
	child uint32
	start uint32
	size  uint64
}

const (
	cfbFree   = 0xFFFFFFFF
	cfbEnd    = 0xFFFFFFFE
	cfbNoStrm = 0xFFFFFFFF
)

var cfbMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

var errNotCFB = errors.New("문서 형식이 달라요")

func openCFB(p string) (*CFB, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxReadBytes))
	if err != nil {
		return nil, err
	}
	return parseCFB(data)
}

func parseCFB(data []byte) (*CFB, error) {
	if len(data) < 512 || !bytes.Equal(data[:8], cfbMagic) {
		return nil, errNotCFB
	}
	le := binary.LittleEndian
	shift := le.Uint16(data[0x1E:])
	mshift := le.Uint16(data[0x20:])
	if shift != 9 && shift != 12 {
		return nil, errNotCFB
	}
	c := &CFB{data: data, secSize: 1 << shift, miniSize: 1 << mshift, cutoff: le.Uint32(data[0x38:])}
	if c.miniSize <= 0 || c.miniSize > c.secSize {
		c.miniSize = 64
	}
	if c.cutoff == 0 {
		c.cutoff = 4096
	}
	c.nSectors = (len(data) - c.secSize) / c.secSize
	if (len(data)-c.secSize)%c.secSize != 0 {
		c.nSectors++
	}
	nFat := int(le.Uint32(data[0x2C:]))
	dirStart := le.Uint32(data[0x30:])
	miniFatStart := le.Uint32(data[0x3C:])
	difatStart := le.Uint32(data[0x44:])
	nDifat := int(le.Uint32(data[0x48:]))

	// FAT 섹터 목록 (헤더의 109개 + DIFAT 사슬)
	fatSecs := []uint32{}
	for i := 0; i < 109; i++ {
		s := le.Uint32(data[0x4C+4*i:])
		if s == cfbFree || s == cfbEnd {
			continue
		}
		fatSecs = append(fatSecs, s)
	}
	sec := difatStart
	per := c.secSize/4 - 1
	for i := 0; i < nDifat && sec != cfbEnd && sec != cfbFree && i <= c.nSectors; i++ {
		b := c.sector(sec)
		if b == nil {
			break
		}
		for j := 0; j < per; j++ {
			s := le.Uint32(b[4*j:])
			if s != cfbFree && s != cfbEnd {
				fatSecs = append(fatSecs, s)
			}
		}
		sec = le.Uint32(b[4*per:])
	}
	if nFat > 0 && len(fatSecs) > nFat {
		fatSecs = fatSecs[:nFat]
	}
	for _, s := range fatSecs {
		b := c.sector(s)
		if b == nil {
			continue
		}
		for j := 0; j+4 <= len(b); j += 4 {
			c.fat = append(c.fat, le.Uint32(b[j:]))
		}
	}
	// 폴더(디렉터리) 항목
	dirData := c.chain(dirStart, -1)
	for i := 0; i+128 <= len(dirData); i += 128 {
		e := dirData[i : i+128]
		nl := int(le.Uint16(e[64:]))
		if nl > 64 {
			nl = 64
		}
		name := ""
		if nl >= 2 {
			u := make([]uint16, (nl-2)/2)
			for k := range u {
				u[k] = le.Uint16(e[2*k:])
			}
			name = string(utf16.Decode(u))
		}
		size := le.Uint64(e[120:])
		if c.secSize == 512 {
			size &= 0xFFFFFFFF
		}
		c.dir = append(c.dir, cfbEntry{name: name, typ: e[66], left: le.Uint32(e[68:]), right: le.Uint32(e[72:]),
			child: le.Uint32(e[76:]), start: le.Uint32(e[116:]), size: size})
	}
	if len(c.dir) == 0 || c.dir[0].typ != 5 {
		return nil, errNotCFB
	}
	// 작은 스트림을 담는 mini stream · mini FAT
	root := c.dir[0]
	c.miniStrm = c.chain(root.start, int64(root.size))
	mf := c.chain(miniFatStart, -1)
	for j := 0; j+4 <= len(mf); j += 4 {
		c.miniFat = append(c.miniFat, le.Uint32(mf[j:]))
	}
	return c, nil
}

func (c *CFB) sector(n uint32) []byte {
	off := (int64(n) + 1) * int64(c.secSize)
	if n >= cfbEnd-16 || off < 0 || off >= int64(len(c.data)) {
		return nil
	}
	end := off + int64(c.secSize)
	if end > int64(len(c.data)) {
		end = int64(len(c.data))
	}
	return c.data[off:end]
}

// chain follows the FAT from start and returns the bytes (size < 0 = whole chain).
func (c *CFB) chain(start uint32, size int64) []byte {
	var out []byte
	seen := 0
	for s := start; s != cfbEnd && s != cfbFree && int(s) < len(c.fat)+1; {
		b := c.sector(s)
		if b == nil {
			break
		}
		out = append(out, b...)
		if size >= 0 && int64(len(out)) >= size {
			return out[:size]
		}
		if int(s) >= len(c.fat) {
			break
		}
		s = c.fat[s]
		seen++
		if seen > c.nSectors+1 {
			break
		}
	}
	if size >= 0 && int64(len(out)) > size {
		out = out[:size]
	}
	return out
}

func (c *CFB) miniChain(start uint32, size int64) []byte {
	var out []byte
	seen := 0
	for s := start; s != cfbEnd && s != cfbFree && int(s) < len(c.miniFat); {
		off := int64(s) * int64(c.miniSize)
		if off >= int64(len(c.miniStrm)) {
			break
		}
		end := min(off+int64(c.miniSize), int64(len(c.miniStrm)))
		out = append(out, c.miniStrm[off:end]...)
		if int64(len(out)) >= size {
			return out[:size]
		}
		s = c.miniFat[s]
		seen++
		if seen > len(c.miniFat)+1 {
			break
		}
	}
	return out
}

// children lists the entries directly inside storage index si.
func (c *CFB) children(si int) []int {
	out := []int{}
	if si < 0 || si >= len(c.dir) {
		return out
	}
	visited := map[uint32]bool{}
	var walk func(i uint32, depth int)
	walk = func(i uint32, depth int) {
		if i == cfbNoStrm || int(i) >= len(c.dir) || visited[i] || depth > 64 {
			return
		}
		visited[i] = true
		walk(c.dir[i].left, depth+1)
		out = append(out, int(i))
		walk(c.dir[i].right, depth+1)
	}
	walk(c.dir[si].child, 0)
	return out
}

func (c *CFB) find(names ...string) int {
	cur := 0
	for _, n := range names {
		next := -1
		for _, ch := range c.children(cur) {
			if strings.EqualFold(c.dir[ch].name, n) {
				next = ch
				break
			}
		}
		if next < 0 {
			return -1
		}
		cur = next
	}
	return cur
}

// Stream returns the content of a stream such as ("BodyText", "Section0").
func (c *CFB) Stream(names ...string) ([]byte, error) {
	i := c.find(names...)
	if i < 0 || c.dir[i].typ != 2 {
		return nil, os.ErrNotExist
	}
	e := c.dir[i]
	if e.size > uint64(maxReadBytes) {
		return nil, errors.New("too big")
	}
	if e.size < uint64(c.cutoff) {
		return c.miniChain(e.start, int64(e.size)), nil
	}
	return c.chain(e.start, int64(e.size)), nil
}

// List returns child names of a storage.
func (c *CFB) List(names ...string) []string {
	i := c.find(names...)
	out := []string{}
	if i < 0 {
		return out
	}
	for _, ch := range c.children(i) {
		out = append(out, c.dir[ch].name)
	}
	return out
}

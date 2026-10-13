package main

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// 맥의 파일 이름은 한글이 자모로 쪼개져(NFD) 저장되는 일이 많아서, 비교하기 전에 항상 NFC로 합쳐요.
func nfc(s string) string {
	if isASCII(s) {
		return s
	}
	return norm.NFC.String(s)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

const choList = "ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ"

var choRunes = []rune(choList)

// choseong turns "사업계획서 2024" into "ㅅㅇㄱㅎㅅ 2024".
func choseong(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0xAC00 && r <= 0xD7A3 {
			b.WriteRune(choRunes[(r-0xAC00)/588])
		} else {
			b.WriteRune(lowerRune(r))
		}
	}
	return b.String()
}

// isChoseongQuery: "ㅅㅇㄱㅎ" 처럼 초성(자음)만으로 된 검색어인지
func isChoseongQuery(s string) bool {
	n := 0
	for _, r := range s {
		if r >= 0x3131 && r <= 0x314E {
			n++
			continue
		}
		if r == ' ' {
			continue
		}
		if r >= 0xAC00 && r <= 0xD7A3 {
			return false
		}
	}
	return n >= 2 || (n == 1 && utf8.RuneCountInString(strings.TrimSpace(s)) == 1)
}

func lowerRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + 32
	}
	return r
}

// lowerASCII lowercases only A-Z, so byte offsets stay the same as the original text.
func lowerASCII(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if b[j] >= 'A' && b[j] <= 'Z' {
					b[j] += 32
				}
			}
			return string(b)
		}
	}
	return s
}

func hasUpperASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			return true
		}
	}
	return false
}

func hasLetterASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return true
		}
	}
	return false
}

// indexFold finds needle (already lower-case ASCII) in hay ignoring ASCII case.
// 한글 같은 다른 글자는 그대로 비교해요. 결과는 hay의 바이트 위치.
func indexFold(hay, needle string) int {
	if needle == "" {
		return 0
	}
	if !hasLetterASCII(needle) {
		return strings.Index(hay, needle)
	}
	n := len(needle)
	c0 := needle[0]
	up0 := c0
	if c0 >= 'a' && c0 <= 'z' {
		up0 = c0 - 32
	}
	for i := 0; i+n <= len(hay); {
		// 첫 글자 후보로 빨리 건너뛰기
		j := indexEither(hay[i:], c0, up0)
		if j < 0 {
			return -1
		}
		i += j
		if i+n > len(hay) {
			return -1
		}
		if equalFoldASCII(hay[i:i+n], needle) {
			return i
		}
		i++
	}
	return -1
}

func indexEither(s string, a, b byte) int {
	if a == b {
		return strings.IndexByte(s, a)
	}
	for i := 0; i < len(s); i++ {
		if s[i] == a || s[i] == b {
			return i
		}
	}
	return -1
}

func equalFoldASCII(s, lowerNeedle string) bool {
	for i := 0; i < len(lowerNeedle); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		if c != lowerNeedle[i] {
			return false
		}
	}
	return true
}

// countFold counts matches (up to limit).
func countFold(hay, needle string, limit int) int {
	n := 0
	for off := 0; n < limit; {
		i := indexFold(hay[off:], needle)
		if i < 0 {
			break
		}
		n++
		off += i + max(1, len(needle))
		if off >= len(hay) {
			break
		}
	}
	return n
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"

	"github.com/ledongthuc/pdf"
)

// PDF 글자는 먼저 내장 읽기 도구로 따로 떨어진 프로세스에서 읽어요 (오래 걸리거나 멈춰도 앱은 괜찮도록).
// 글자가 깨지거나 비어 있으면 맥의 PDFKit(helper.js)으로 다시 읽고, 그래도 없으면 OCR 대상이 돼요.

type pdfOut struct {
	Text      string `json:"text"`
	Pages     int    `json:"pages"`
	Encrypted bool   `json:"encrypted"`
	Err       string `json:"err"`
}

func extractPDF(p string) (Extract, error) {
	self, err := os.Executable()
	if e := os.Getenv("LIBROAD_SELF"); e != "" {
		self = e
	}
	if err != nil && self == "" {
		return Extract{Retry: true}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, "--pdftext", p)
	cmd.Env = append(os.Environ(), "GOMAXPROCS=1", "GOMEMLIMIT=700MiB")
	hideWindow(cmd)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	runErr := cmd.Run()
	var o pdfOut
	if err := json.Unmarshal(stdout.Bytes(), &o); err != nil {
		// 읽기 도구가 멈췄거나 실패 → PDFKit으로 다시
		_ = runErr
		return Extract{Retry: true}, nil
	}
	if o.Encrypted {
		return Extract{Encrypted: true, Pages: o.Pages, Retry: true, Note: "암호가 걸려 있어서 이름으로만 찾아요"}, nil
	}
	t := cleanText(o.Text)
	res := Extract{Text: t, Pages: o.Pages}
	switch {
	case o.Err != "" && t == "":
		res.Retry = true
	case len([]rune(t)) < 20*max(1, min(o.Pages, 3)):
		// 글자가 거의 없음 → 스캔 문서일 가능성. PDFKit으로 확인 뒤 OCR
		res.Retry = true
	case !textLooksRight(t):
		res.Retry = true
	case fewSpaces(t):
		// 띄어쓰기가 사라진 글자 → 맥의 PDFKit이 더 잘 읽어요 (지금 글자는 그대로 두고 더 나으면 바꿔요)
		res.Retry = true
	}
	return res, nil
}

// textLooksRight: 글꼴 정보가 없어서 글자가 깨진 PDF를 골라내요.
func textLooksRight(s string) bool {
	good, bad, total := 0, 0, 0
	for _, r := range s {
		if total > 20000 {
			break
		}
		total++
		switch {
		case r == ' ' || r == '\n' || r == '\t':
			good++
		case r >= 0xAC00 && r <= 0xD7A3, r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			good++
		case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
			good++
		case strings.ContainsRune(".,:;!?()[]{}<>-_+=*/\\'\"%&#@~·•…“”‘’「」『』《》〈〉①②③④⑤⑥⑦⑧⑨⑩□■○●◇◆△▲※→←↑↓‧∙ㆍ|$₩€", r):
			good++
		case r >= 0x3130 && r <= 0x318F: // 한글 자모 (따로 떨어진 자모가 많으면 깨진 것일 수도)
			bad++
		case r < 32, r >= 0xE000 && r <= 0xF8FF, r == 0xFFFD:
			bad += 2
		case unicode.IsLetter(r):
			// 다른 나라 글자: 조금은 괜찮아요
			good++
			if r >= 0x0100 && r < 0x0250 || r >= 0x0370 && r < 0x0530 {
				bad++
			}
		default:
			bad++
		}
	}
	if total == 0 {
		return false
	}
	return float64(good) >= float64(total)*0.85 && bad*8 < total
}

func fewSpaces(s string) bool {
	hangul, spaces := 0, 0
	for _, r := range s {
		if r >= 0xAC00 && r <= 0xD7A3 {
			hangul++
		} else if r == ' ' {
			spaces++
		}
	}
	return hangul > 150 && spaces*20 < hangul
}

// runPDFText is the child process: it prints pdfOut as JSON.
func runPDFText(p string) {
	o := pdfOut{}
	defer func() {
		if r := recover(); r != nil {
			o.Err = fmt.Sprint(r)
		}
		json.NewEncoder(os.Stdout).Encode(o)
	}()
	f, r, err := pdf.Open(p)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "encrypt") || strings.Contains(strings.ToLower(err.Error()), "password") {
			o.Encrypted = true
		}
		o.Err = err.Error()
		return
	}
	defer f.Close()
	o.Pages = r.NumPage()
	var b strings.Builder
	var fonts map[string]*pdf.Font
	for i := 1; i <= o.Pages && i <= 600; i++ {
		pg := r.Page(i)
		if pg.V.IsNull() {
			continue
		}
		func() {
			defer func() { recover() }()
			t, err := pg.GetPlainText(fonts)
			if err == nil {
				b.WriteString(t)
				b.WriteByte('\n')
			}
		}()
		if b.Len() > maxTextBytes*3 {
			break
		}
	}
	o.Text = b.String()
}

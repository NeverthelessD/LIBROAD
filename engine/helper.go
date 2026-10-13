package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// helper.js 를 osascript로 실행해요 (맥 전용). 테스트할 때는 LIBROAD_HELPER_CMD 로 바꿀 수 있어요.

type helperLine struct {
	Path      string   `json:"path"`
	OK        bool     `json:"ok"`
	Text      string   `json:"text"`
	Pages     int      `json:"pages"`
	Encrypted bool     `json:"encrypted"`
	Trashed   string   `json:"trashed"`
	Error     string   `json:"error"`
	Tick      int      `json:"tick"`
	Page      int      `json:"page"`
	Check     bool     `json:"check"`
	Vision    bool     `json:"vision"`
	PDFKit    bool     `json:"pdfkit"`
	Langs     []string `json:"langs"`
	OS        string   `json:"os"`
}

func helperAvailable() bool {
	return os.Getenv("LIBROAD_HELPER_CMD") != "" || runtime.GOOS == "darwin"
}

func helperCommand(ctx context.Context, args ...string) *exec.Cmd {
	if c := os.Getenv("LIBROAD_HELPER_CMD"); c != "" {
		parts := strings.Fields(c)
		return exec.CommandContext(ctx, parts[0], append(parts[1:], args...)...)
	}
	script := filepath.Join(app.binDir, "helper.js")
	return exec.CommandContext(ctx, "/usr/bin/osascript", append([]string{"-l", "JavaScript", script}, args...)...)
}

// runHelper runs one batch. onLine is called for every JSON line. It returns when the helper exits.
// idle: kill the helper when it prints nothing for this long.
func runHelper(mode string, paths []string, extra []string, idle time.Duration, cancel func() bool, onLine func(helperLine)) error {
	if !helperAvailable() {
		return errors.New("helper not available")
	}
	listFile := ""
	if len(paths) > 0 {
		tmp := filepath.Join(app.support, "tmp")
		os.MkdirAll(tmp, 0o755)
		listFile = filepath.Join(tmp, "list-"+randID(4)+".json")
		b, _ := json.Marshal(paths)
		if err := os.WriteFile(listFile, b, 0o600); err != nil {
			return err
		}
		defer os.Remove(listFile)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	args := []string{mode}
	if listFile != "" {
		args = append(args, listFile)
	}
	args = append(args, extra...)
	cmd := helperCommand(ctx, args...)
	hideWindow(cmd)
	setProcessGroup(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var errBuf strings.Builder
	cmd.Stderr = &limitedWriter{w: &errBuf, n: 4000}
	if err := cmd.Start(); err != nil {
		return err
	}
	var last sync.Mutex
	lastAt := time.Now()
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				last.Lock()
				quiet := time.Since(lastAt)
				last.Unlock()
				if quiet > idle || (cancel != nil && cancel()) {
					killProcessGroup(cmd)
					stop()
					return
				}
			}
		}
	}()
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		last.Lock()
		lastAt = time.Now()
		last.Unlock()
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var hl helperLine
		if json.Unmarshal([]byte(line), &hl) == nil {
			onLine(hl)
		}
	}
	err = cmd.Wait()
	close(done)
	if err != nil {
		if s := strings.TrimSpace(errBuf.String()); s != "" {
			log.Printf("helper %s: %v: %s", mode, err, lastLine(s))
		}
		return err
	}
	return nil
}

type limitedWriter struct {
	w interface{ WriteString(string) (int, error) }
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		s := string(p)
		if len(s) > l.n {
			s = s[:l.n]
		}
		l.n -= len(s)
		l.w.WriteString(s)
	}
	return len(p), nil
}

// ---------------------------------------------------------------- OCR 지원 여부

var (
	ocrOnce  sync.Once
	ocrState = struct {
		sync.Mutex
		checked bool
		vision  bool
		langs   []string
		os      string
		err     string
	}{}
)

func checkHelper() {
	ocrOnce.Do(func() {
		if !helperAvailable() {
			ocrState.Lock()
			ocrState.checked = true
			ocrState.Unlock()
			return
		}
		var got helperLine
		err := runHelper("check", nil, nil, 40*time.Second, nil, func(h helperLine) {
			if h.Check {
				got = h
			}
		})
		ocrState.Lock()
		ocrState.checked = true
		ocrState.vision = got.Vision
		ocrState.langs = got.Langs
		ocrState.os = got.OS
		if err != nil && !got.Check {
			ocrState.err = err.Error()
		}
		ocrState.Unlock()
		log.Printf("helper check: vision=%v langs=%v os=%q err=%v", got.Vision, got.Langs, got.OS, err)
	})
}

// ocrSupported: "yes" | "no" | "checking" | "noko"(한국어 미지원 — macOS 13 미만)
func ocrSupported() string {
	ocrState.Lock()
	defer ocrState.Unlock()
	if !ocrState.checked {
		return "checking"
	}
	if !ocrState.vision {
		return "no"
	}
	for _, l := range ocrState.langs {
		if l == "ko-KR" {
			return "yes"
		}
	}
	return "noko"
}

func ocrReady() bool {
	s := ocrSupported()
	return s == "yes" || s == "noko"
}

func atoiDef(s string, d int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return d
}

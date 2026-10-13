//go:build darwin

package main

import (
	"os"
	"syscall"
)

// isDataless: iCloud에만 있고 이 Mac에는 내려받지 않은 파일 (열면 내려받기가 시작돼요)
func isDataless(fi os.FileInfo) bool {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return st.Flags&0x40000000 != 0 // SF_DATALESS
	}
	return false
}

func birthTime(fi os.FileInfo) int64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Birthtimespec.Sec > 0 {
		return st.Birthtimespec.Sec
	}
	return fi.ModTime().Unix()
}

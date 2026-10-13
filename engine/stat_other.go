//go:build !darwin

package main

import "os"

func isDataless(fi os.FileInfo) bool { return false }

func birthTime(fi os.FileInfo) int64 { return fi.ModTime().Unix() }

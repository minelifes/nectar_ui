//go:build !windows

package main

import "os"

// enableVT: Unix terminals handle ANSI escapes as they are.
func enableVT(*os.File) bool { return true }

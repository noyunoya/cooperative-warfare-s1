//go:build !windows

package main

import "os/exec"

func findWindows() (int, int)                 { return 0, 0 }
func arrangeWindows() int                     { return 0 }
func autoArrange()                            {}
func linkDir(src, dst string) (string, error) { return "", nil }
func openUI(url string)                       { exec.Command("xdg-open", url).Start() }
func openPath(p string)                       {}

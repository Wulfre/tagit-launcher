//go:build !windows
// +build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func launchTagIt() {
	execName := "tagit." + runtime.GOOS + ".x86_64"
	execPath := filepath.Join(cacheDir, execName)
	if _, err := os.Stat(execPath); err == nil {
		cmd := exec.Command(execPath)
		cmd.Start()
		os.Exit(0)
	}
}

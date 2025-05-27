//go:build windows
// +build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func launchTagIt() {
	execName := "tagit.windows.x86_64.exe"
	execPath := filepath.Join(cacheDir, execName)
	if _, err := os.Stat(execPath); err == nil {
		cmd := exec.Command(execPath, "--no-update")
		// Set Windows-specific process attributes to hide console window
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true,
		}
		cmd.Start()
		os.Exit(0)
	}
}

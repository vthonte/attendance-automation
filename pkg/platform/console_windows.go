//go:build windows

package platform

import (
	"os"
	"syscall"
)

const ATTACH_PARENT_PROCESS = ^uintptr(0)

func AttachParentConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attachConsole := kernel32.NewProc("AttachConsole")
	r, _, _ := attachConsole.Call(ATTACH_PARENT_PROCESS)
	if r != 0 {
		if stdout, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0644); err == nil {
			os.Stdout = stdout
		}
		if stderr, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0644); err == nil {
			os.Stderr = stderr
		}
	}
}

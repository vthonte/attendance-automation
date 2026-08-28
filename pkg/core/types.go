package core

import (
	"context"
)

type ProcessHandle struct {
	Pid int
	Raw any
}

type PlatformDriver interface {
	Name() string
	FindBrowser(cfg *Config) (string, error)
	GetDebugProfileDir(baseDir string) string
	StartProcess(cmd string, args []string, visible bool) (*ProcessHandle, error)
	StopAttendanceProcesses(profileDir string, debugPort int) error
	FocusBrowser() error
	IsProcessRunning(pid int) bool
	ShowToast(ctx context.Context, cfg *Config, events <-chan StatusEvent)
	SendNotification(title, message string) error
	InstallAutostart(cfg *Config) error
	UninstallAutostart(cfg *Config) error
	IsAutostartInstalled(cfg *Config) bool
}

//go:build !windows && !linux && !darwin

package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"attendance/pkg/core"
)

type stubDriver struct{}

func newPlatformDriver() core.PlatformDriver {
	return &stubDriver{}
}

func (d *stubDriver) Name() string {
	return "stub"
}

func (d *stubDriver) GetDebugProfileDir(baseDir string) string {
	return filepath.Join(baseDir, ".attendance", "ChromeDebug")
}

func (d *stubDriver) FindBrowser(cfg *core.Config) (string, error) {
	if cfg.ChromePath != "" {
		return cfg.ChromePath, nil
	}
	if p, err := exec.LookPath("google-chrome"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("chromium"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("no browser found")
}

func (d *stubDriver) StartProcess(executable string, args []string, visible bool) (*core.ProcessHandle, error) {
	cmd := exec.Command(executable, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &core.ProcessHandle{Pid: cmd.Process.Pid, Raw: cmd}, nil
}

func (d *stubDriver) StopAttendanceProcesses(profileDir string, debugPort int) error {
	return nil
}

func (d *stubDriver) FocusBrowser() error {
	return nil
}

func (d *stubDriver) IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	return err == nil && process != nil
}

func (d *stubDriver) KillProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func (d *stubDriver) SendNotification(title, message string) error {
	core.Log("", fmt.Sprintf("[Notification] %s: %s", title, message))
	return nil
}

func (d *stubDriver) InstallAutostart(cfg *core.Config) error {
	return fmt.Errorf("autostart not supported on this platform")
}

func (d *stubDriver) UninstallAutostart(cfg *core.Config) error {
	return nil
}

func (d *stubDriver) IsAutostartInstalled(cfg *core.Config) bool {
	return false
}

func (d *stubDriver) ShowToast(ctx context.Context, cfg *core.Config, events <-chan core.StatusEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-events:
			core.LogToast(cfg.DataDir, fmt.Sprintf("Toast Status: %s (%s)", ev.Status, ev.DisplayText))
		}
	}
}

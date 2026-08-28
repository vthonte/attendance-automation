//go:build darwin

package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"attendance/pkg/core"
)

type darwinDriver struct{}

func newPlatformDriver() core.PlatformDriver {
	return &darwinDriver{}
}

func (d *darwinDriver) Name() string {
	return "darwin"
}

func (d *darwinDriver) GetDebugProfileDir(baseDir string) string {
	home := os.Getenv("HOME")
	if home != "" {
		return filepath.Join(home, ".attendance", "ChromeDebug")
	}
	return filepath.Join(baseDir, ".attendance", "ChromeDebug")
}

func (d *darwinDriver) FindBrowser(cfg *core.Config) (string, error) {
	if cfg.ChromePath != "" {
		if _, err := os.Stat(cfg.ChromePath); err == nil {
			return cfg.ChromePath, nil
		}
	}

	portableDir := filepath.Join(cfg.DataDir, "browser")
	portableCandidates := []string{
		"Google Chrome.app/Contents/MacOS/Google Chrome",
		"chrome-mac-x64/Google Chrome.app/Contents/MacOS/Google Chrome",
		"chrome-mac-arm64/Google Chrome.app/Contents/MacOS/Google Chrome",
		"chrome-headless-shell-mac-x64/chrome-headless-shell",
		"chrome-headless-shell-mac-arm64/chrome-headless-shell",
	}
	for _, rel := range portableCandidates {
		p := filepath.Join(portableDir, rel)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	home := os.Getenv("HOME")
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, "Applications", "Google Chrome.app", "Contents", "MacOS", "Google Chrome"),
			filepath.Join(home, "Applications", "Microsoft Edge.app", "Contents", "MacOS", "Microsoft Edge"),
			filepath.Join(home, "Applications", "Brave Browser.app", "Contents", "MacOS", "Brave Browser"),
			filepath.Join(home, "Applications", "Chromium.app", "Contents", "MacOS", "Chromium"),
		)
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	for _, name := range []string{"google-chrome", "chromium", "brave"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("no Chromium browser found. Run 'attendance --download-browser' or configure CHROME_PATH in %s", cfg.ConfigFile)
}

func (d *darwinDriver) StartProcess(executable string, args []string, visible bool) (*core.ProcessHandle, error) {
	cmd := exec.Command(executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &core.ProcessHandle{
		Pid: cmd.Process.Pid,
		Raw: cmd,
	}, nil
}

func (d *darwinDriver) StopAttendanceProcesses(profileDir string, debugPort int) error {
	shCmd := fmt.Sprintf(`pkill -f "%s" 2>/dev/null || true; lsof -ti:%d | xargs kill -9 2>/dev/null || true`, strings.ReplaceAll(profileDir, `"`, `\"`), debugPort)
	cmd := exec.Command("sh", "-c", shCmd)
	_ = cmd.Run()
	return nil
}

func (d *darwinDriver) FocusBrowser() error {
	script := `tell application "Google Chrome" to activate`
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}

func (d *darwinDriver) IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func (d *darwinDriver) SendNotification(title, message string) error {
	escapedTitle := strings.ReplaceAll(title, `"`, `\"`)
	escapedMsg := strings.ReplaceAll(message, `"`, `\"`)
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, escapedMsg, escapedTitle)
	return exec.Command("osascript", "-e", script).Start()
}

func getLaunchAgentPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.attendance.automation.plist")
}

func (d *darwinDriver) InstallAutostart(cfg *core.Config) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	plistPath := getLaunchAgentPath()
	if plistPath == "" {
		return fmt.Errorf("could not determine LaunchAgents folder")
	}

	_ = os.MkdirAll(filepath.Dir(plistPath), 0755)

	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.attendance.automation</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>WorkingDirectory</key>
    <string>%s</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>`, exePath, cfg.BaseDir)

	return os.WriteFile(plistPath, []byte(content), 0644)
}

func (d *darwinDriver) UninstallAutostart(cfg *core.Config) error {
	plistPath := getLaunchAgentPath()
	if plistPath != "" {
		_ = os.Remove(plistPath)
	}
	return nil
}

func (d *darwinDriver) IsAutostartInstalled(cfg *core.Config) bool {
	plistPath := getLaunchAgentPath()
	if plistPath == "" {
		return false
	}
	_, err := os.Stat(plistPath)
	return err == nil
}

func (d *darwinDriver) ShowToast(ctx context.Context, cfg *core.Config, events <-chan core.StatusEvent) {
	var lastStatus string
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-events:
			if ev.Status != lastStatus {
				lastStatus = ev.Status
				_ = d.SendNotification("Attendance Status", ev.DisplayText)
			}
		}
	}
}

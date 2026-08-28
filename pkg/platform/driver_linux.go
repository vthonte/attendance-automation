//go:build linux

package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"attendance/pkg/core"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type linuxDriver struct{}

func newPlatformDriver() core.PlatformDriver {
	return &linuxDriver{}
}

func (d *linuxDriver) Name() string {
	return "linux"
}

func (d *linuxDriver) GetDebugProfileDir(baseDir string) string {
	home := os.Getenv("HOME")
	if home != "" {
		return filepath.Join(home, ".attendance", "ChromeDebug")
	}
	return filepath.Join(baseDir, ".attendance", "ChromeDebug")
}

func (d *linuxDriver) FindBrowser(cfg *core.Config) (string, error) {
	if cfg.ChromePath != "" {
		if _, err := os.Stat(cfg.ChromePath); err == nil {
			return cfg.ChromePath, nil
		}
	}

	portableDir := filepath.Join(cfg.DataDir, "browser")
	portableCandidates := []string{
		"chrome",
		"chrome-headless-shell",
		"chrome-linux64/chrome",
		"chrome-linux64/chrome-headless-shell",
		"chrome-headless-shell-linux64/chrome-headless-shell",
	}
	for _, rel := range portableCandidates {
		p := filepath.Join(portableDir, rel)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	candidates := []string{
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/microsoft-edge",
		"/usr/bin/microsoft-edge-stable",
		"/usr/bin/brave-browser",
		"/snap/bin/chromium",
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "brave-browser"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("no Chromium browser found. Run 'attendance --download-browser' or configure CHROME_PATH in %s", cfg.ConfigFile)
}

func (d *linuxDriver) StartProcess(executable string, args []string, visible bool) (*core.ProcessHandle, error) {
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

func (d *linuxDriver) StopAttendanceProcesses(profileDir string, debugPort int) error {
	shCmd := fmt.Sprintf(`pkill -f "%s" 2>/dev/null || true; fuser -k %d/tcp 2>/dev/null || true`, strings.ReplaceAll(profileDir, `"`, `\"`), debugPort)
	cmd := exec.Command("sh", "-c", shCmd)
	_ = cmd.Run()
	return nil
}

func (d *linuxDriver) FocusBrowser() error {
	cmd := exec.Command("sh", "-c", `wmctrl -a "Keka" 2>/dev/null || wmctrl -a "Google Chrome" 2>/dev/null || wmctrl -a "Microsoft Edge" 2>/dev/null || true`)
	return cmd.Run()
}

func (d *linuxDriver) IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func (d *linuxDriver) SendNotification(title, message string) error {
	if _, err := exec.LookPath("notify-send"); err == nil {
		return exec.Command("notify-send", "-a", "Attendance Automation", title, message).Start()
	}
	return nil
}

func getAutostartDesktopPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "autostart", "attendance.desktop")
}

func (d *linuxDriver) InstallAutostart(cfg *core.Config) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	desktopPath := getAutostartDesktopPath()
	if desktopPath == "" {
		return fmt.Errorf("could not determine autostart path")
	}

	_ = os.MkdirAll(filepath.Dir(desktopPath), 0755)

	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Attendance Automation
Exec=%s
Path=%s
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
Comment=Keka Attendance Automation Service
`, exePath, cfg.BaseDir)

	return os.WriteFile(desktopPath, []byte(content), 0644)
}

func (d *linuxDriver) UninstallAutostart(cfg *core.Config) error {
	desktopPath := getAutostartDesktopPath()
	if desktopPath != "" {
		_ = os.Remove(desktopPath)
	}
	return nil
}

func (d *linuxDriver) IsAutostartInstalled(cfg *core.Config) bool {
	desktopPath := getAutostartDesktopPath()
	if desktopPath == "" {
		return false
	}
	_, err := os.Stat(desktopPath)
	return err == nil
}

func (d *linuxDriver) ShowToast(ctx context.Context, cfg *core.Config, events <-chan core.StatusEvent) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		// Headless or Wayland without X11: handle notifications
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

	X, err := xgb.NewConn()
	if err != nil {
		return
	}
	defer X.Close()

	setup := xproto.Setup(X)
	screen := setup.DefaultScreen(X)

	win, err := xproto.NewWindowId(X)
	if err != nil {
		return
	}

	width := uint16(screen.WidthInPixels)
	height := uint16(cfg.ToastHeight)
	if height < 32 {
		height = 32
	}

	mask := uint32(xproto.CwBackPixel | xproto.CwOverrideRedirect | xproto.CwEventMask)
	values := []uint32{
		screen.BlackPixel,
		1, // OverrideRedirect = true (stays on top, no window manager chrome)
		xproto.EventMaskExposure | xproto.EventMaskStructureNotify,
	}

	err = xproto.CreateWindowChecked(
		X,
		screen.RootDepth,
		win,
		screen.Root,
		0, 0,
		width, height,
		0,
		xproto.WindowClassInputOutput,
		screen.RootVisual,
		mask,
		values,
	).Check()

	if err != nil {
		return
	}

	gc, err := xproto.NewGcontextId(X)
	if err != nil {
		return
	}
	_ = xproto.CreateGC(X, gc, xproto.Drawable(win), 0, nil)

	_ = xproto.MapWindow(X, win)

	var (
		currentEv core.StatusEvent
	)

	drawToast := func(ev core.StatusEvent) {
		var colorPixel uint32
		switch ev.ColorName {
		case "lightgreen":
			colorPixel = 0x90EE90
		case "khaki":
			colorPixel = 0xF0E68C
		case "lightcoral":
			colorPixel = 0xF08080
		default:
			colorPixel = 0xD3D3D3
		}

		if ev.BarVisible {
			_ = xproto.ChangeGC(X, gc, xproto.GcForeground, []uint32{screen.BlackPixel})
			_ = xproto.PolyFillRectangle(X, xproto.Drawable(win), gc, []xproto.Rectangle{{X: 0, Y: 0, Width: width, Height: height}})

			_ = xproto.ChangeGC(X, gc, xproto.GcForeground, []uint32{colorPixel})
			_ = xproto.PolyFillRectangle(X, xproto.Drawable(win), gc, []xproto.Rectangle{{X: 0, Y: 0, Width: width, Height: 3}})
		}

		if ev.DisplayText != "" {
			badgeX := int16(width) - int16(len(ev.DisplayText)*8+30)
			if badgeX < 0 {
				badgeX = 10
			}

			_ = xproto.ChangeGC(X, gc, xproto.GcForeground, []uint32{0x222222})
			_ = xproto.PolyFillRectangle(X, xproto.Drawable(win), gc, []xproto.Rectangle{{X: badgeX, Y: 5, Width: uint16(len(ev.DisplayText)*8 + 20), Height: 20}})

			_ = xproto.ChangeGC(X, gc, xproto.GcForeground, []uint32{colorPixel})
			_ = xproto.PolyRectangle(X, xproto.Drawable(win), gc, []xproto.Rectangle{{X: badgeX, Y: 5, Width: uint16(len(ev.DisplayText)*8 + 20), Height: 20}})

			_ = xproto.ChangeGC(X, gc, xproto.GcForeground, []uint32{0xFFFFFF})
			_ = xproto.ImageText8(X, byte(len(ev.DisplayText)), xproto.Drawable(win), gc, badgeX+10, 19, ev.DisplayText)
		}
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-events:
				currentEv = ev
				drawToast(ev)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			_ = xproto.DestroyWindow(X, win)
			return
		default:
		}

		ev, err := X.WaitForEvent()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if ev == nil {
			return
		}

		switch ev.(type) {
		case xproto.ExposeEvent:
			drawToast(currentEv)
		}
	}
}

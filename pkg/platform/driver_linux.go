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

	// 1. Prefer real system GUI browsers (Chrome, Edge, Brave, Chromium) for unified cookie & login persistence
	if gui, err := d.FindGUIBrowser(cfg); err == nil {
		return gui, nil
	}

	// 2. Portable fallback
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

func (d *linuxDriver) FindGUIBrowser(cfg *core.Config) (string, error) {
	if cfg.ChromePath != "" && !strings.Contains(strings.ToLower(cfg.ChromePath), "headless") {
		if _, err := os.Stat(cfg.ChromePath); err == nil {
			return cfg.ChromePath, nil
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

	return "", fmt.Errorf("no GUI Chromium browser found on system")
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
	currPid := os.Getpid()
	shCmd := fmt.Sprintf(`pgrep -f "attendance" | grep -v "^%d$" | xargs -r kill -9 2>/dev/null || true; fuser -k %d/tcp 2>/dev/null || true`, currPid, debugPort)
	cmd := exec.Command("sh", "-c", shCmd)
	_ = cmd.Run()
	return nil
}

func (d *linuxDriver) FocusBrowser() error {
	cmd := exec.Command("sh", "-c", `wmctrl -a "Keka" 2>/dev/null || wmctrl -a "Google Chrome" 2>/dev/null || wmctrl -a "Microsoft Edge" 2>/dev/null || true`)
	return cmd.Run()
}

func (d *linuxDriver) IsGUIBrowserOpen(profileDir string) bool {
	cmd := exec.Command("sh", "-c", fmt.Sprintf(`pgrep -f "%s" | grep -v "headless"`, strings.ReplaceAll(profileDir, `"`, `\"`)))
	out, err := cmd.Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

func (d *linuxDriver) LaunchGUIBrowser(executable string, args []string) error {
	cmd := exec.Command(executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

func (d *linuxDriver) IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func (d *linuxDriver) KillProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = process.Signal(syscall.SIGTERM)
	time.Sleep(100 * time.Millisecond)
	_ = process.Signal(syscall.SIGKILL)
	return nil
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
Exec=%s --no-browser
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

func getAtom(X *xgb.Conn, name string) xproto.Atom {
	reply, err := xproto.InternAtom(X, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0
	}
	return reply.Atom
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

	eventMask := uint32(
		xproto.EventMaskExposure |
			xproto.EventMaskStructureNotify |
			xproto.EventMaskEnterWindow |
			xproto.EventMaskLeaveWindow |
			xproto.EventMaskPointerMotion,
	)

	mask := uint32(xproto.CwBackPixel | xproto.CwOverrideRedirect | xproto.CwEventMask)
	values := []uint32{
		screen.BlackPixel,
		1, // OverrideRedirect = true (stays on top, no window manager decorations)
		eventMask,
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

	// Set EWMH NetWM atoms for ALWAYS-ON-TOP dock surface
	netWmWindowType := getAtom(X, "_NET_WM_WINDOW_TYPE")
	netWmWindowTypeDock := getAtom(X, "_NET_WM_WINDOW_TYPE_DOCK")
	if netWmWindowType != 0 && netWmWindowTypeDock != 0 {
		_ = xproto.ChangePropertyChecked(X, xproto.PropModeReplace, win, netWmWindowType, xproto.AtomAtom, 32, 1, []byte{
			byte(netWmWindowTypeDock), byte(netWmWindowTypeDock >> 8), byte(netWmWindowTypeDock >> 16), byte(netWmWindowTypeDock >> 24),
		}).Check()
	}

	netWmState := getAtom(X, "_NET_WM_STATE")
	netWmStateAbove := getAtom(X, "_NET_WM_STATE_ABOVE")
	netWmStateStaysOnTop := getAtom(X, "_NET_WM_STATE_STAYS_ON_TOP")
	if netWmState != 0 && netWmStateAbove != 0 {
		_ = xproto.ChangePropertyChecked(X, xproto.PropModeReplace, win, netWmState, xproto.AtomAtom, 32, 2, []byte{
			byte(netWmStateAbove), byte(netWmStateAbove >> 8), byte(netWmStateAbove >> 16), byte(netWmStateAbove >> 24),
			byte(netWmStateStaysOnTop), byte(netWmStateStaysOnTop >> 8), byte(netWmStateStaysOnTop >> 16), byte(netWmStateStaysOnTop >> 24),
		}).Check()
	}

	gc, err := xproto.NewGcontextId(X)
	if err != nil {
		return
	}
	_ = xproto.CreateGC(X, gc, xproto.Drawable(win), 0, nil)

	_ = xproto.MapWindow(X, win)

	// Keep window on top (restack above maximized windows)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = xproto.ConfigureWindow(X, win, xproto.ConfigWindowStackMode, []uint32{xproto.StackModeAbove})
			}
		}
	}()

	var (
		currentEv      core.StatusEvent
		isBadgeHovered bool
	)

	drawToast := func() {
		var colorPixel uint32
		switch currentEv.ColorName {
		case "lightgreen":
			colorPixel = 0x90EE90
		case "khaki":
			colorPixel = 0xF0E68C
		case "lightcoral":
			colorPixel = 0xF08080
		default:
			colorPixel = 0xD3D3D3
		}

		barH := uint32(cfg.BarHeight)
		if barH <= 0 {
			barH = 2
		}

		if currentEv.BarVisible {
			currentHeight := barH
			if isBadgeHovered && currentEv.DisplayText != "" && cfg.ShowLoggedDate {
				currentHeight = uint32(height)
			}

			_ = xproto.ConfigureWindow(X, win, xproto.ConfigWindowHeight, []uint32{currentHeight})

			// Clear background
			_ = xproto.ChangeGC(X, gc, xproto.GcForeground, []uint32{screen.BlackPixel})
			_ = xproto.PolyFillRectangle(X, xproto.Drawable(win), gc, []xproto.Rectangle{{X: 0, Y: 0, Width: width, Height: uint16(currentHeight)}})

			// Draw colored bar at the very top
			_ = xproto.ChangeGC(X, gc, xproto.GcForeground, []uint32{colorPixel})
			_ = xproto.PolyFillRectangle(X, xproto.Drawable(win), gc, []xproto.Rectangle{{X: 0, Y: 0, Width: width, Height: uint16(barH)}})
		}

		// Draw badge ONLY when hovered over the top right area
		if currentEv.DisplayText != "" && isBadgeHovered && cfg.ShowLoggedDate {
			badgeWidth := uint16(len(currentEv.DisplayText)*8 + 24)
			if badgeWidth < 90 {
				badgeWidth = 90
			}
			badgeX := int16(width) - int16(badgeWidth) - 12
			if badgeX < 0 {
				badgeX = 10
			}

			// Dark badge background
			_ = xproto.ChangeGC(X, gc, xproto.GcForeground, []uint32{0x1C1814})
			_ = xproto.PolyFillRectangle(X, xproto.Drawable(win), gc, []xproto.Rectangle{{X: badgeX, Y: 5, Width: badgeWidth, Height: 20}})

			// Badge border colored with status color
			_ = xproto.ChangeGC(X, gc, xproto.GcForeground, []uint32{colorPixel})
			_ = xproto.PolyRectangle(X, xproto.Drawable(win), gc, []xproto.Rectangle{{X: badgeX, Y: 5, Width: badgeWidth, Height: 20}})

			// White text
			_ = xproto.ChangeGC(X, gc, xproto.GcForeground, []uint32{0xFFFFFF})
			_ = xproto.ImageText8(X, byte(len(currentEv.DisplayText)), xproto.Drawable(win), gc, badgeX+10, 19, currentEv.DisplayText)
		}
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-events:
				currentEv = ev
				drawToast()
			}
		}
	}()

	// Query pointer position periodically for instant, reliable hover in WSLg / X11
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reply, err := xproto.QueryPointer(X, screen.Root).Reply()
				if err == nil {
					hovered := reply.RootX >= int16(width)-260 && reply.RootY >= 0 && reply.RootY <= int16(height)+4
					if hovered != isBadgeHovered {
						isBadgeHovered = hovered
						drawToast()
					}
				}
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
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if ev == nil {
			return
		}

		switch e := ev.(type) {
		case xproto.ExposeEvent:
			drawToast()

		case xproto.EnterNotifyEvent:
			if e.EventX >= int16(width)-260 && e.EventY <= 32 {
				if !isBadgeHovered {
					isBadgeHovered = true
					drawToast()
				}
			}

		case xproto.LeaveNotifyEvent:
			if isBadgeHovered {
				isBadgeHovered = false
				drawToast()
			}

		case xproto.MotionNotifyEvent:
			if e.EventX >= int16(width)-260 && e.EventY <= 32 {
				if !isBadgeHovered {
					isBadgeHovered = true
					drawToast()
				}
			} else {
				if isBadgeHovered {
					isBadgeHovered = false
					drawToast()
				}
			}
		}
	}
}

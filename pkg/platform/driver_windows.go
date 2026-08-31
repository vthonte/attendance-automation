//go:build windows

package platform

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"attendance/pkg/core"
)

type windowsDriver struct{}

func newPlatformDriver() core.PlatformDriver {
	return &windowsDriver{}
}

func (d *windowsDriver) Name() string {
	return "windows"
}

func (d *windowsDriver) GetDebugProfileDir(baseDir string) string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		return filepath.Join(localAppData, "ChromeDebug")
	}
	return filepath.Join(baseDir, "ChromeDebug")
}

func (d *windowsDriver) FindBrowser(cfg *core.Config) (string, error) {
	if cfg.ChromePath != "" {
		if _, err := os.Stat(cfg.ChromePath); err == nil {
			return cfg.ChromePath, nil
		}
	}

	portableDir := filepath.Join(cfg.DataDir, "browser")
	portableCandidates := []string{
		"chrome.exe",
		"chrome-headless-shell.exe",
		"chrome-win64/chrome.exe",
		"chrome-win64/chrome-headless-shell.exe",
		"chrome-headless-shell-win64/chrome-headless-shell.exe",
	}
	for _, rel := range portableCandidates {
		p := filepath.Join(portableDir, rel)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	localAppData := os.Getenv("LOCALAPPDATA")
	progFiles := os.Getenv("ProgramFiles")
	progFilesX86 := os.Getenv("ProgramFiles(x86)")

	var candidates []string
	if localAppData != "" {
		candidates = append(candidates,
			filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(localAppData, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(localAppData, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		)
	}
	if progFiles != "" {
		candidates = append(candidates,
			filepath.Join(progFiles, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(progFiles, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(progFiles, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		)
	}
	if progFilesX86 != "" {
		candidates = append(candidates,
			filepath.Join(progFilesX86, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(progFilesX86, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(progFilesX86, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		)
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	for _, name := range []string{"chrome.exe", "msedge.exe", "brave.exe", "chromium.exe"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("no Chromium browser found. Run 'attendance --download-browser' or configure CHROME_PATH in %s", cfg.ConfigFile)
}

func (d *windowsDriver) FindGUIBrowser(cfg *core.Config) (string, error) {
	if cfg.ChromePath != "" && !strings.Contains(strings.ToLower(cfg.ChromePath), "headless") {
		if _, err := os.Stat(cfg.ChromePath); err == nil {
			return cfg.ChromePath, nil
		}
	}

	localAppData := os.Getenv("LOCALAPPDATA")
	progFiles := os.Getenv("ProgramFiles")
	progFilesX86 := os.Getenv("ProgramFiles(x86)")

	var candidates []string
	if localAppData != "" {
		candidates = append(candidates,
			filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(localAppData, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(localAppData, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		)
	}
	if progFiles != "" {
		candidates = append(candidates,
			filepath.Join(progFiles, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(progFiles, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(progFiles, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		)
	}
	if progFilesX86 != "" {
		candidates = append(candidates,
			filepath.Join(progFilesX86, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(progFilesX86, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(progFilesX86, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		)
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	for _, name := range []string{"chrome.exe", "msedge.exe", "brave.exe", "chromium.exe"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("no GUI Chromium browser found on system")
}

func (d *windowsDriver) StartProcess(executable string, args []string, visible bool) (*core.ProcessHandle, error) {
	cmd := exec.Command(executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP
		HideWindow:    !visible,
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &core.ProcessHandle{
		Pid: cmd.Process.Pid,
		Raw: cmd,
	}, nil
}

func (d *windowsDriver) StopAttendanceProcesses(profileDir string, debugPort int) error {
	currPid := os.Getpid()

	// Instant taskkill for any other attendance.exe instances (< 5ms)
	cmd1 := exec.Command("cmd.exe", "/c", fmt.Sprintf("taskkill /F /IM attendance.exe /FI \"PID ne %d\" 2>nul", currPid))
	cmd1.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd1.Run()

	// Fast Go socket check: only kill Chrome if the debug port is actually listening
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", debugPort), 10*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		cmd2 := exec.Command("cmd.exe", "/c", "taskkill /F /IM chrome.exe /FI \"WINDOWTITLE eq *ChromeDebug*\" 2>nul & taskkill /F /IM msedge.exe /FI \"WINDOWTITLE eq *ChromeDebug*\" 2>nul")
		cmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd2.Run()
	}
	return nil
}

func (d *windowsDriver) FocusBrowser() error {
	psCmd := `$ws=New-Object -ComObject WScript.Shell; if (-not $ws.AppActivate('Keka')) { $null=$ws.AppActivate('Google Chrome'); if (-not $?) { $null=$ws.AppActivate('Microsoft Edge') } }`
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

func (d *windowsDriver) IsProcessRunning(pid int) bool {
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	const STILL_ACTIVE = 259

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	openProcess := kernel32.NewProc("OpenProcess")
	getExitCodeProcess := kernel32.NewProc("GetExitCodeProcess")
	closeHandle := kernel32.NewProc("CloseHandle")

	handle, _, _ := openProcess.Call(uintptr(PROCESS_QUERY_LIMITED_INFORMATION), 0, uintptr(pid))
	if handle == 0 {
		return false
	}
	defer closeHandle.Call(handle)

	var exitCode uint32
	ret, _, _ := getExitCodeProcess.Call(handle, uintptr(unsafe.Pointer(&exitCode)))
	if ret == 0 {
		return false
	}
	return exitCode == STILL_ACTIVE
}

func (d *windowsDriver) KillProcess(pid int) error {
	cmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/F", "/T")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

func (d *windowsDriver) SendNotification(title, message string) error {
	escapedTitle := strings.ReplaceAll(title, "'", "''")
	escapedMsg := strings.ReplaceAll(message, "'", "''")
	psCmd := fmt.Sprintf(`[reflection.assembly]::loadwithpartialname('System.Windows.Forms') | Out-Null; $notify = new-object system.windows.forms.notifyicon; $notify.icon = [system.drawing.systemicons]::information; $notify.visible = $true; $notify.showballoontip(10000, '%s', '%s', [system.windows.forms.tooltipicon]::Info)`, escapedTitle, escapedMsg)
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

func getStartupShortcutPath(shortcutName string) string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return ""
	}
	return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", fmt.Sprintf("%s.lnk", shortcutName))
}

func (d *windowsDriver) InstallAutostart(cfg *core.Config) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	shortcutPath := getStartupShortcutPath(cfg.StartupShortcutName)
	if shortcutPath == "" {
		return fmt.Errorf("could not determine Windows Startup folder path")
	}

	psScript := fmt.Sprintf(`$ws = New-Object -ComObject WScript.Shell; $s = $ws.CreateShortcut('%s'); $s.TargetPath = '%s'; $s.Arguments = '--no-browser'; $s.WorkingDirectory = '%s'; $s.Save()`,
		shortcutPath, exePath, cfg.BaseDir)

	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", psScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

func (d *windowsDriver) UninstallAutostart(cfg *core.Config) error {
	shortcutPath := getStartupShortcutPath(cfg.StartupShortcutName)
	if shortcutPath != "" {
		_ = os.Remove(shortcutPath)
	}
	return nil
}

func (d *windowsDriver) IsAutostartInstalled(cfg *core.Config) bool {
	shortcutPath := getStartupShortcutPath(cfg.StartupShortcutName)
	if shortcutPath == "" {
		return false
	}
	_, err := os.Stat(shortcutPath)
	return err == nil
}

// Native Win32 Toast Implementation
var (
	winUser32   = syscall.NewLazyDLL("user32.dll")
	winGdi32    = syscall.NewLazyDLL("gdi32.dll")
	winKernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassExW           = winUser32.NewProc("RegisterClassExW")
	pCreateWindowExW            = winUser32.NewProc("CreateWindowExW")
	pDefWindowProcW             = winUser32.NewProc("DefWindowProcW")
	pShowWindow                 = winUser32.NewProc("ShowWindow")
	pUpdateWindow               = winUser32.NewProc("UpdateWindow")
	pSetWindowPos               = winUser32.NewProc("SetWindowPos")
	pSetLayeredWindowAttributes = winUser32.NewProc("SetLayeredWindowAttributes")
	pInvalidateRect             = winUser32.NewProc("InvalidateRect")
	pBeginPaint                 = winUser32.NewProc("BeginPaint")
	pEndPaint                   = winUser32.NewProc("EndPaint")
	pGetMessageW                = winUser32.NewProc("GetMessageW")
	pTranslateMessage           = winUser32.NewProc("TranslateMessage")
	pDispatchMessageW           = winUser32.NewProc("DispatchMessageW")
	pPostQuitMessage            = winUser32.NewProc("PostQuitMessage")
	pGetSystemMetrics           = winUser32.NewProc("GetSystemMetrics")
	pDestroyWindow              = winUser32.NewProc("DestroyWindow")

	pCreateSolidBrush = winGdi32.NewProc("CreateSolidBrush")
	pSelectObject     = winGdi32.NewProc("SelectObject")
	pDeleteObject     = winGdi32.NewProc("DeleteObject")
	pFillRect         = winUser32.NewProc("FillRect")
	pCreateFontW      = winGdi32.NewProc("CreateFontW")
	pSetTextColor     = winGdi32.NewProc("SetTextColor")
	pSetBkMode        = winGdi32.NewProc("SetBkMode")
	pDrawTextW        = winUser32.NewProc("DrawTextW")
	pRoundRect        = winGdi32.NewProc("RoundRect")
	pTrackMouseEvent  = winUser32.NewProc("TrackMouseEvent")
	pGetCursorPos     = winUser32.NewProc("GetCursorPos")

	pGetModuleHandleW = winKernel32.NewProc("GetModuleHandleW")
)

type TRACKMOUSEEVENT struct {
	CbSize      uint32
	DwFlags     uint32
	HwndTrack   uintptr
	DwHoverTime uint32
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type RECT struct {
	Left, Top, Right, Bottom int32
}

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

var (
	toastText       string
	toastColorName  string
	toastBarVisible bool = true
	toastBarHeight  int  = 2
	toastHwnd       uintptr
	isBadgeHovered  bool
	toastMu         sync.Mutex
)

func winColorRefFromName(name string) uint32 {
	switch name {
	case "lightgreen":
		return 0x0090EE90
	case "khaki":
		return 0x008CE6F0
	case "lightcoral":
		return 0x008080F0
	default:
		return 0x00D3D3D3
	}
}

func winWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case 0x0084: // WM_NCHITTEST
		return ^uintptr(0) // HTTRANSPARENT (-1) 100% click-through, never interferes with clicks

	case 0x000F: // WM_PAINT
		var ps PAINTSTRUCT
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hdc != 0 {
			var rc RECT
			rc.Left = 0
			rc.Top = 0
			cx, _, _ := pGetSystemMetrics.Call(0) // SM_CXSCREEN
			rc.Right = int32(cx)
			rc.Bottom = 32

			bgBrush, _, _ := pCreateSolidBrush.Call(0x00000000)
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), bgBrush)
			pDeleteObject.Call(bgBrush)

			toastMu.Lock()
			text := toastText
			colName := toastColorName
			barVis := toastBarVisible
			barH := int32(toastBarHeight)
			toastMu.Unlock()

			if barH <= 0 {
				barH = 2
			}

			if barVis {
				var barRc RECT
				barRc.Left = 0
				barRc.Top = 0
				barRc.Right = int32(cx)
				barRc.Bottom = barH

				barColor := winColorRefFromName(colName)
				barBrush, _, _ := pCreateSolidBrush.Call(uintptr(barColor))
				pFillRect.Call(hdc, uintptr(unsafe.Pointer(&barRc)), barBrush)
				pDeleteObject.Call(barBrush)
			}

			// Date badge is ONLY drawn when hovered over the top right
			if text != "" && isBadgeHovered {
				badgeWidth := int32(len(text)*7 + 20)
				if badgeWidth < 90 {
					badgeWidth = 90
				}
				badgeRight := int32(cx) - 12
				badgeLeft := badgeRight - badgeWidth

				var badgeRc RECT
				badgeRc.Left = badgeLeft
				badgeRc.Top = 5
				badgeRc.Right = badgeRight
				badgeRc.Bottom = 26

				badgeBrush, _, _ := pCreateSolidBrush.Call(0x001C1814)
				oldBrush, _, _ := pSelectObject.Call(hdc, badgeBrush)
				pRoundRect.Call(hdc, uintptr(badgeRc.Left), uintptr(badgeRc.Top), uintptr(badgeRc.Right), uintptr(badgeRc.Bottom), 8, 8)
				pSelectObject.Call(hdc, oldBrush)
				pDeleteObject.Call(badgeBrush)

				pSetBkMode.Call(hdc, 1) // TRANSPARENT
				pSetTextColor.Call(hdc, 0x00FFFFFF)

				fontName, _ := syscall.UTF16PtrFromString("Segoe UI")
				font, _, _ := pCreateFontW.Call(13, 0, 0, 0, 400, 0, 0, 0, 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(fontName)))
				oldFont, _, _ := pSelectObject.Call(hdc, font)

				utf16Text, _ := syscall.UTF16FromString(text)
				pDrawTextW.Call(
					hdc,
					uintptr(unsafe.Pointer(&utf16Text[0])),
					uintptr(len(utf16Text)-1),
					uintptr(unsafe.Pointer(&badgeRc)),
					0x00000001|0x00000004|0x00000020, // DT_CENTER|DT_VCENTER|DT_SINGLELINE
				)

				pSelectObject.Call(hdc, oldFont)
				pDeleteObject.Call(font)
			}

			pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		}
		return 0

	case 0x0002: // WM_DESTROY
		pPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func (d *windowsDriver) ShowToast(ctx context.Context, cfg *core.Config, events <-chan core.StatusEvent) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInstance, _, _ := pGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("AttendanceToastClass")
	windowName, _ := syscall.UTF16PtrFromString("AttendanceToast")

	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = syscall.NewCallback(winWndProc)
	wc.HInstance = hInstance
	wc.LpszClassName = className

	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	cx, _, _ := pGetSystemMetrics.Call(0)
	height := uintptr(cfg.ToastHeight)
	if height < 32 {
		height = 32
	}

	hwnd, _, _ := pCreateWindowExW.Call(
		0x00000008|0x00080000|0x00000020|0x00000080|0x08000000, // TOPMOST | LAYERED | TRANSPARENT | TOOLWINDOW | NOACTIVATE
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0x80000000|0x10000000, // WS_POPUP | WS_VISIBLE
		0, 0, cx, height,
		0, 0, hInstance, 0,
	)

	if hwnd == 0 {
		return
	}

	toastMu.Lock()
	toastHwnd = hwnd
	toastBarVisible = cfg.ShowToastUI
	if cfg.BarHeight > 0 {
		toastBarHeight = cfg.BarHeight
	}
	toastMu.Unlock()

	initH := uintptr(cfg.BarHeight)
	if initH <= 0 {
		initH = 2
	}

	pSetLayeredWindowAttributes.Call(hwnd, 0x00000000, 0, 1) // LWA_COLORKEY = 1
	pSetWindowPos.Call(hwnd, ^uintptr(0), 0, 0, cx, initH, 0x0040|0x0010)
	pShowWindow.Call(hwnd, 4)
	pUpdateWindow.Call(hwnd)

	// Watch events channel
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-events:
				toastMu.Lock()
				toastText = ev.DisplayText
				toastColorName = ev.ColorName
				toastBarVisible = ev.BarVisible
				h := toastHwnd
				toastMu.Unlock()

				if h != 0 {
					pInvalidateRect.Call(h, 0, 1)
				}
			}
		}
	}()

	// Reliable cursor polling for hover detection
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var pt struct{ X, Y int32 }
				r, _, _ := pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
				if r != 0 {
					wCx, _, _ := pGetSystemMetrics.Call(0)
					hovered := pt.X >= int32(wCx)-260 && pt.Y >= 0 && pt.Y <= int32(height)+4
					toastMu.Lock()
					changed := hovered != isBadgeHovered
					if changed {
						isBadgeHovered = hovered
					}
					h := toastHwnd
					bHeight := toastBarHeight
					toastMu.Unlock()

					if changed && h != 0 {
						curH := uintptr(bHeight)
						if curH <= 0 {
							curH = 2
						}
						if isBadgeHovered && toastText != "" && cfg.ShowLoggedDate {
							curH = height
						}
						pSetWindowPos.Call(h, ^uintptr(0), 0, 0, wCx, curH, 0x0040|0x0010)
						pInvalidateRect.Call(h, 0, 1)
					}
				}
			}
		}
	}()

	var msg MSG
	for {
		select {
		case <-ctx.Done():
			pDestroyWindow.Call(hwnd)
			return
		default:
		}

		ret, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

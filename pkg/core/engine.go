package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Engine struct {
	Cfg         *Config
	Driver      PlatformDriver
	EventBus    *EventBus
	triggerChan chan struct{}
}

func NewEngine(cfg *Config, driver PlatformDriver) *Engine {
	return &Engine{
		Cfg:         cfg,
		Driver:      driver,
		EventBus:    NewEventBus(),
		triggerChan: make(chan struct{}, 1),
	}
}

func (e *Engine) TriggerCheck() {
	select {
	case e.triggerChan <- struct{}{}:
	default:
	}
}

func timeToMinutes(val string) int {
	parts := strings.Split(val, ":")
	if len(parts) != 2 {
		return 0
	}
	h, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return h*60 + m
}

func (e *Engine) QuietWindowDelay(now time.Time) time.Duration {
	current := now.Hour()*60 + now.Minute()
	start := timeToMinutes(e.Cfg.SkipCheckFrom)
	end := timeToMinutes(e.Cfg.SkipCheckUntil)

	var inWindow bool
	if start < end {
		inWindow = current >= start && current < end
	} else {
		inWindow = current >= start || current < end
	}

	if !inWindow {
		return 0
	}

	minutes := end - current
	if minutes <= 0 {
		minutes += 24 * 60
	}

	delay := time.Duration(minutes)*time.Minute - time.Duration(now.Second())*time.Second - time.Duration(now.Nanosecond())
	if delay < 0 {
		return 0
	}
	return delay
}

func (e *Engine) emitStatus(status string) {
	_ = SetStatus(e.Cfg.DataDir, status)
	dateKey := LocalDateKey(time.Now())
	txt, col := GetStatusDisplay(status, dateKey, e.Cfg.ShowLoggedDate)
	e.EventBus.Publish(StatusEvent{
		Status:      status,
		DateKey:     dateKey,
		DisplayText: txt,
		ColorName:   col,
		BarVisible:  e.Cfg.ShowToastUI,
		Timestamp:   time.Now(),
	})
}

func (e *Engine) SleepUntilNextCheck(ctx context.Context, delay time.Duration, previousDate string) {
	scheduledWake := time.Now().Add(delay)

	select {
	case <-ctx.Done():
		return
	case <-e.triggerChan:
		Log(e.Cfg.DataDir, "Immediate attendance check requested; resuming check loop")
	case <-time.After(delay):
	}

	drift := time.Since(scheduledWake)
	currentDate := LocalDateKey(time.Now())

	if drift > 60*time.Second {
		Log(e.Cfg.DataDir, fmt.Sprintf("System sleep/wake detected; check resumed %d seconds late", int(drift.Seconds())))
	}

	if currentDate != previousDate {
		e.emitStatus("out")
		Log(e.Cfg.DataDir, fmt.Sprintf("Date changed from %s to %s; refreshing attendance", previousDate, currentDate))
	}
}

func cleanProfileLocks(profileDir string) {
	_ = os.Remove(filepath.Join(profileDir, "SingletonLock"))
	_ = os.Remove(filepath.Join(profileDir, "SingletonSocket"))
	_ = os.Remove(filepath.Join(profileDir, "SingletonCookie"))
	_ = os.Remove(filepath.Join(profileDir, "lockfile"))
}

func (e *Engine) ClockInIfNeeded(ctx context.Context) (time.Duration, error) {
	quietDelay := e.QuietWindowDelay(time.Now())
	if quietDelay > 0 {
		e.emitStatus("out")
		Log(e.Cfg.DataDir, fmt.Sprintf("Skipping attendance check during quiet window %s-%s; next check in %d minutes",
			e.Cfg.SkipCheckFrom, e.Cfg.SkipCheckUntil, int(quietDelay.Minutes())+1))
		return quietDelay, nil
	}

	if IsLoggedToday(e.Cfg.DataDir) {
		e.emitStatus("in")
		Log(e.Cfg.DataDir, fmt.Sprintf("Already logged for %s -> sleeping", LocalDateKey(time.Now())))
		return e.Cfg.CheckInterval, nil
	}

	profileDir := e.Driver.GetDebugProfileDir(e.Cfg.BaseDir)
	_ = os.MkdirAll(profileDir, 0755)

	// If a manual login browser is currently open, inspect if user completed login
	if e.Driver.IsGUIBrowserOpen(profileDir) {
		Log(e.Cfg.DataDir, "Manual login browser is open. Inspecting session for login completion...")

		cdpCtx, cancelCDP := context.WithTimeout(ctx, 4*time.Second)
		client, err := ConnectCDP(cdpCtx, e.Cfg.DebugHost, e.Cfg.DebugPort)
		cancelCDP()
		if err == nil {
			defer client.Close()
			res, err := PerformKekaCheckAndClockIn(ctx, client, e.Cfg)
			if err == nil && (res == ResultAlreadyClockedIn || res == ResultClockedIn) {
				_ = MarkLoggedToday(e.Cfg.DataDir)
				e.emitStatus("in")
				_ = e.Driver.SendNotification("Attendance Automation", "Successfully clocked in for today!")
				return e.Cfg.CheckInterval, nil
			}
		}

		Log(e.Cfg.DataDir, fmt.Sprintf("Waiting for login in open browser... (next check in %d seconds)", int(e.Cfg.ManualAttentionInterval.Seconds())))
		e.emitStatus("error")
		return e.Cfg.ManualAttentionInterval, nil
	}

	e.emitStatus("run")
	Log(e.Cfg.DataDir, fmt.Sprintf("Starting attendance check for %s", LocalDateKey(time.Now())))

	browserPath, err := e.Driver.FindBrowser(e.Cfg)
	if err != nil {
		Log(e.Cfg.DataDir, fmt.Sprintf("Browser lookup failed: %v", err))
		e.emitStatus("error")
		return e.Cfg.CheckInterval, err
	}

	// Clean up any stale browser on port and release profile locks
	_ = e.Driver.StopAttendanceProcesses(profileDir, e.Cfg.DebugPort)
	cleanProfileLocks(profileDir)

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", e.Cfg.DebugPort),
		fmt.Sprintf("--remote-debugging-address=%s", e.Cfg.DebugHost),
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		fmt.Sprintf("--profile-directory=%s", e.Cfg.ChromeProfileDirectory),
		"--no-first-run",
		"--no-default-browser-check",
		"--enable-features=Geolocation",
		"--auto-accept-camera-and-microphone-capture",
	}

	if !e.Cfg.ChromeVisible {
		args = append(args, "--headless=new")
		if e.Cfg.DisableChromeBackgroundServices {
			args = append(args,
				"--disable-background-networking",
				"--disable-component-update",
				"--disable-default-apps",
				"--disable-extensions",
				"--disable-sync",
			)
		}
	} else {
		args = append(args, "--new-window", "--start-maximized")
	}
	args = append(args, e.Cfg.AttendanceURL)

	_, err = e.Driver.StartProcess(browserPath, args, e.Cfg.ChromeVisible)
	if err != nil {
		Log(e.Cfg.DataDir, fmt.Sprintf("Failed to start browser process: %v", err))
		e.emitStatus("error")
		return e.Cfg.CheckInterval, err
	}

	keepManualOpen := false
	defer func() {
		if !keepManualOpen {
			_ = e.Driver.StopAttendanceProcesses(profileDir, e.Cfg.DebugPort)
		}
	}()

	// Connect CDP
	cdpCtx, cancelCDP := context.WithTimeout(ctx, e.Cfg.CDPConnectTimeout)
	defer cancelCDP()

	var cdp *CDPClient
	deadline := time.Now().Add(e.Cfg.CDPConnectTimeout)
	for time.Now().Before(deadline) {
		client, err := ConnectCDP(cdpCtx, e.Cfg.DebugHost, e.Cfg.DebugPort)
		if err == nil {
			cdp = client
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if cdp == nil {
		Log(e.Cfg.DataDir, "Timed out connecting to Chrome DevTools Protocol")
		e.emitStatus("error")
		return e.Cfg.CheckInterval, fmt.Errorf("CDP connection failed")
	}
	defer cdp.Close()

	res, err := PerformKekaCheckAndClockIn(ctx, cdp, e.Cfg)
	if err != nil {
		Log(e.Cfg.DataDir, fmt.Sprintf("Attendance check error: %v", err))
		e.emitStatus("out")
		return e.Cfg.CheckInterval, err
	}

	switch res {
	case ResultAlreadyClockedIn, ResultClockedIn:
		_ = MarkLoggedToday(e.Cfg.DataDir)
		e.emitStatus("in")
		_ = e.Driver.SendNotification("Attendance Automation", "Successfully clocked in for today!")
		return e.Cfg.CheckInterval, nil

	case ResultNeedsAttention:
		e.emitStatus("error")
		// Stop the headless browser and clean locks so the GUI browser can acquire the profile
		_ = e.Driver.StopAttendanceProcesses(profileDir, e.Cfg.DebugPort)
		cleanProfileLocks(profileDir)
		time.Sleep(300 * time.Millisecond)

		Log(e.Cfg.DataDir, "Clock-in needs manual attention; opening browser window...")

		var launched bool
		guiBrowser, err := e.Driver.FindGUIBrowser(e.Cfg)
		if err == nil {
			manualArgs := []string{
				fmt.Sprintf("--user-data-dir=%s", profileDir),
				fmt.Sprintf("--profile-directory=%s", e.Cfg.ChromeProfileDirectory),
				"--new-window",
				"--disable-background-mode",
				"--no-first-run",
				e.Cfg.AttendanceURL,
			}
			Log(e.Cfg.DataDir, fmt.Sprintf("Launching GUI browser for login (%s) with shared profile: %s", guiBrowser, profileDir))
			if err := e.Driver.LaunchGUIBrowser(guiBrowser, manualArgs); err == nil {
				launched = true
				time.Sleep(1 * time.Second)
				_ = e.Driver.FocusBrowser()
			} else {
				Log(e.Cfg.DataDir, fmt.Sprintf("GUI browser launch error: %v", err))
			}
		}

		if !launched {
			Log(e.Cfg.DataDir, fmt.Sprintf("Opening attendance page in default browser: %s", e.Cfg.AttendanceURL))
			_ = OpenBrowser(e.Cfg.AttendanceURL)
		}

		_ = e.Driver.SendNotification("Attendance Automation", "Manual attention required: Please check browser window")
		keepManualOpen = true
		return e.Cfg.ManualAttentionInterval, nil

	default:
		e.emitStatus("out")
		return e.Cfg.CheckInterval, nil
	}
}

func (e *Engine) Run(ctx context.Context) error {
	Log(e.Cfg.DataDir, fmt.Sprintf("Attendance daemon started (Driver: %s)", e.Driver.Name()))
	Log(e.Cfg.DataDir, fmt.Sprintf("Data directory: %s", e.Cfg.DataDir))
	Log(e.Cfg.DataDir, fmt.Sprintf("Attendance URL: %s", e.Cfg.AttendanceURL))

	if !IsValidCompanyName(e.Cfg.CompanyName) {
		e.emitStatus("error")
		Log(e.Cfg.DataDir, fmt.Sprintf("Invalid COMPANY_NAME '%s'. Update %s before restarting.", e.Cfg.CompanyName, e.Cfg.ConfigFile))
		Log(e.Cfg.DataDir, "Attendance paused because configuration is invalid; status will remain 'error'.")
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(60 * time.Second):
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		checkDate := LocalDateKey(time.Now())
		delay, _ := e.ClockInIfNeeded(ctx)
		if delay <= 0 {
			delay = e.Cfg.CheckInterval
		}

		Log(e.Cfg.DataDir, fmt.Sprintf("Waiting %d seconds until next scheduled check", int(delay.Seconds())))
		e.SleepUntilNextCheck(ctx, delay, checkDate)
	}
}

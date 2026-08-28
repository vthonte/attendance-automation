package core

import (
	"context"
	"fmt"
	"os"
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

	e.emitStatus("run")
	Log(e.Cfg.DataDir, fmt.Sprintf("Starting attendance check for %s", LocalDateKey(time.Now())))

	browserPath, err := e.Driver.FindBrowser(e.Cfg)
	if err != nil {
		Log(e.Cfg.DataDir, fmt.Sprintf("Browser lookup failed: %v", err))
		e.emitStatus("error")
		return e.Cfg.CheckInterval, err
	}

	profileDir := e.Driver.GetDebugProfileDir(e.Cfg.BaseDir)
	_ = os.MkdirAll(profileDir, 0755)

	// Clean up any stale browser on port
	_ = e.Driver.StopAttendanceProcesses(profileDir, e.Cfg.DebugPort)

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", e.Cfg.DebugPort),
		fmt.Sprintf("--remote-debugging-address=%s", e.Cfg.DebugHost),
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		fmt.Sprintf("--profile-directory=%s", e.Cfg.ChromeProfileDirectory),
		"--no-first-run",
		"--no-default-browser-check",
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

	// Wait for CDP port
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
		_ = e.Driver.StopAttendanceProcesses(profileDir, e.Cfg.DebugPort)

		Log(e.Cfg.DataDir, "Clock-in needs manual attention; opening browser window...")
		manualArgs := []string{
			fmt.Sprintf("--user-data-dir=%s", profileDir),
			fmt.Sprintf("--profile-directory=%s", e.Cfg.ChromeProfileDirectory),
			"--new-window",
			"--disable-background-mode",
			"--no-first-run",
			"--no-default-browser-check",
			e.Cfg.AttendanceURL,
		}
		_, _ = e.Driver.StartProcess(browserPath, manualArgs, true)
		time.Sleep(1 * time.Second)
		_ = e.Driver.FocusBrowser()
		_ = e.Driver.SendNotification("Attendance Automation", "Manual attention required: Please check Chrome window")
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

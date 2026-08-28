package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"attendance/pkg/core"
	"attendance/pkg/platform"
)

const Version = "2.0.0 (Go Native Single-Binary)"

func main() {
	platform.AttachParentConsole()

	onceFlag := flag.Bool("once", false, "Run a single attendance check and exit")
	statusFlag := flag.Bool("status", false, "Show current attendance status and configuration")
	toastFlag := flag.Bool("toast", false, "Run only the status toast overlay window")
	stopFlag := flag.Bool("stop", false, "Stop running attendance automation processes and Chrome")
	setupFlag := flag.Bool("setup", false, "Run interactive setup wizard")
	installStartupFlag := flag.Bool("install-startup", false, "Register attendance to start automatically on system boot")
	uninstallStartupFlag := flag.Bool("uninstall-startup", false, "Remove attendance from system startup")
	downloadBrowserFlag := flag.Bool("download-browser", false, "Download portable Chrome Headless Shell to data/browser/")
	dashboardFlag := flag.Bool("dashboard", false, "Run web dashboard only")
	dashboardPortFlag := flag.Int("dashboard-port", 9333, "Web dashboard port")
	versionFlag := flag.Bool("version", false, "Show version information")
	configFlag := flag.String("config", "", "Custom path to config.txt")

	flag.Parse()

	driver := platform.GetDriver()

	if *versionFlag {
		fmt.Printf("Attendance Automation v%s (%s/%s, Driver: %s)\n", Version, runtime.GOOS, runtime.GOARCH, driver.Name())
		return
	}

	if *configFlag != "" {
		_ = os.Setenv("ATTENDANCE_CONFIG_FILE", *configFlag)
	}

	cfg, err := core.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	_ = core.EnsureConfigFile(cfg.ConfigFile)

	// Command: --download-browser
	if *downloadBrowserFlag {
		fmt.Println("Downloading lightweight portable Chrome Headless Shell...")
		if err := core.DownloadPortableBrowser(cfg.DataDir); err != nil {
			fmt.Fprintf(os.Stderr, "Download failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Done! Browser installed to:", filepath.Join(cfg.DataDir, "browser"))
		return
	}

	// Command: --stop
	if *stopFlag {
		fmt.Println("Stopping attendance processes...")
		lockFile := core.LockFilePath(cfg.DataDir)
		if data, err := os.ReadFile(lockFile); err == nil {
			if oldPid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && oldPid != os.Getpid() {
				_ = driver.KillProcess(oldPid)
			}
		}
		profileDir := driver.GetDebugProfileDir(cfg.BaseDir)
		_ = driver.StopAttendanceProcesses(profileDir, cfg.DebugPort)
		_ = os.Remove(lockFile)
		core.Log(cfg.DataDir, "Attendance daemon stopped via --stop")
		fmt.Println("Attendance processes and dedicated Chrome stopped. Logs and data preserved.")
		return
	}

	// Command: --setup
	if *setupFlag {
		runInteractiveSetup(cfg, driver)
		return
	}

	// Command: --install-startup
	if *installStartupFlag {
		if err := driver.InstallAutostart(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to register startup: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Successfully registered Attendance Automation for startup!")
		return
	}

	// Command: --uninstall-startup
	if *uninstallStartupFlag {
		if err := driver.UninstallAutostart(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to unregister startup: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Successfully removed Attendance Automation from startup.")
		return
	}

	// Command: --status
	if *statusFlag {
		showStatus(cfg, driver)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		core.Log(cfg.DataDir, "Shutdown signal received. Exiting cleanly...")
		cancel()
	}()

	engine := core.NewEngine(cfg, driver)

	// Command: --toast (run toast UI only)
	if *toastFlag {
		fmt.Println("Running Toast UI...")
		driver.ShowToast(ctx, cfg, engine.EventBus.Subscribe())
		<-ctx.Done()
		return
	}

	// Command: --dashboard (run dashboard only)
	if *dashboardFlag {
		fmt.Printf("Starting Web Dashboard on http://127.0.0.1:%d\n", *dashboardPortFlag)
		core.StartWebDashboard(ctx, engine, *dashboardPortFlag)
		return
	}

	// Command: --once (single check and exit)
	if *onceFlag {
		fmt.Println("Running single attendance check...")
		release, err := core.AcquireLock(cfg.DataDir, driver)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot run check: %v\n", err)
			os.Exit(1)
		}
		defer release()

		_, err = engine.ClockInIfNeeded(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Attendance check error: %v\n", err)
			os.Exit(1)
		}
		st, dk, _ := core.GetStatus(cfg.DataDir)
		txt, _ := core.GetStatusDisplay(st, dk, true)
		fmt.Printf("Finished check. Status: %s (%s)\n", st, txt)
		return
	}

	// Default Mode: Daemon Loop + Native Toast + Web Dashboard
	profileDir := driver.GetDebugProfileDir(cfg.BaseDir)
	_ = driver.StopAttendanceProcesses(profileDir, cfg.DebugPort)

	release, err := core.AcquireLock(cfg.DataDir, driver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer release()

	defer func() {
		_ = driver.StopAttendanceProcesses(profileDir, cfg.DebugPort)
	}()

	if cfg.ShowToastUI || cfg.ShowLoggedDate {
		go driver.ShowToast(ctx, cfg, engine.EventBus.Subscribe())
	}

	go core.StartWebDashboard(ctx, engine, *dashboardPortFlag)

	if err := engine.Run(ctx); err != nil && err != context.Canceled {
		core.Log(cfg.DataDir, fmt.Sprintf("Fatal daemon error: %v", err))
		_ = core.SetStatus(cfg.DataDir, "error")
		os.Exit(1)
	}
}

func showStatus(cfg *core.Config, driver core.PlatformDriver) {
	st, dateKey, _ := core.GetStatus(cfg.DataDir)
	loggedToday := core.IsLoggedToday(cfg.DataDir)
	if loggedToday {
		st = "in"
		dateKey = core.LocalDateKey(time.Now())
	}
	displayText, color := core.GetStatusDisplay(st, dateKey, true)

	bPath, bErr := driver.FindBrowser(cfg)

	fmt.Println("=== Attendance Automation Status ===")
	fmt.Printf("Platform Driver:    %s (%s/%s)\n", driver.Name(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Status:             %s (%s, color: %s)\n", st, displayText, color)
	fmt.Printf("Logged Today:       %v\n", loggedToday)
	fmt.Printf("Company Name:       %s\n", cfg.CompanyName)
	fmt.Printf("Attendance URL:     %s\n", cfg.AttendanceURL)
	fmt.Printf("Clock-In Mode:      %s\n", cfg.ClockInMode)
	fmt.Printf("Quiet Window:       %s - %s\n", cfg.SkipCheckFrom, cfg.SkipCheckUntil)
	fmt.Printf("Check Interval:     %v\n", cfg.CheckInterval)
	fmt.Printf("Config File:        %s\n", cfg.ConfigFile)
	fmt.Printf("Data Directory:     %s\n", cfg.DataDir)

	if bErr == nil {
		fmt.Printf("Browser:            %s\n", bPath)
	} else {
		fmt.Printf("Browser:            NOT FOUND (%v)\n", bErr)
	}

	fmt.Printf("Startup Enabled:    %v\n", driver.IsAutostartInstalled(cfg))
}

func runInteractiveSetup(cfg *core.Config, driver core.PlatformDriver) {
	reader := bufio.NewReader(os.Stdin)
	ask := func(prompt, current string) string {
		fmt.Printf("%s [%s]: ", prompt, current)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			return current
		}
		return input
	}

	fmt.Println("=== Attendance Automation Setup Wizard ===")
	company := ask("Keka Company Name", cfg.CompanyName)
	mode := ask("Clock-In Mode (web/remote/auto)", string(cfg.ClockInMode))
	skipFrom := ask("Skip checks from time (HH:MM)", cfg.SkipCheckFrom)
	skipUntil := ask("Resume checks at time (HH:MM)", cfg.SkipCheckUntil)
	checkSec := ask("Check interval in seconds", fmt.Sprintf("%d", int(cfg.CheckInterval.Seconds())))
	startWithBoot := ask("Start automatically with system (true/false)", fmt.Sprintf("%v", cfg.StartWithWindows))

	checkIntervalMs := 60000
	if s, err := fmt.Sscanf(checkSec, "%d", &checkIntervalMs); err == nil && s > 0 {
		checkIntervalMs = checkIntervalMs * 1000
	}

	content := fmt.Sprintf(`# Attendance automation settings.
COMPANY_NAME=%s
CLOCK_IN_MODE=%s
SKIP_CHECK_FROM=%s
SKIP_CHECK_UNTIL=%s
CHECK_INTERVAL_MS=%d
MANUAL_ATTENTION_INTERVAL_MS=%d
CLOCK_IN_CONTROL_TIMEOUT_MS=%d
CLOCK_OUT_CONTROL_TIMEOUT_MS=%d
CDP_CONNECT_TIMEOUT_MS=%d
DEBUG_HOST=%s
DEBUG_PORT=%d
CHROME_PROFILE_DIRECTORY=%s
START_WITH_WINDOWS=%s
STARTUP_SHORTCUT_NAME=%s
SHOW_TOAST_UI=%v
SHOW_LOGGED_DATE=%v
TOAST_HEIGHT=%d
DISABLE_CHROME_BACKGROUND_SERVICES=%v
DISABLE_ALL_UI=%v
CHROME_VISIBLE=%v
`,
		company, mode, skipFrom, skipUntil, checkIntervalMs,
		int(cfg.ManualAttentionInterval.Milliseconds()),
		int(cfg.ClockInControlTimeout.Milliseconds()),
		int(cfg.ClockOutControlTimeout.Milliseconds()),
		int(cfg.CDPConnectTimeout.Milliseconds()),
		cfg.DebugHost, cfg.DebugPort, cfg.ChromeProfileDirectory,
		startWithBoot, cfg.StartupShortcutName,
		cfg.ShowToastUI, cfg.ShowLoggedDate, cfg.ToastHeight,
		cfg.DisableChromeBackgroundServices, cfg.DisableAllUI, cfg.ChromeVisible,
	)

	if err := os.WriteFile(cfg.ConfigFile, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save configuration: %v\n", err)
		return
	}
	fmt.Println("Configuration saved to:", cfg.ConfigFile)

	newCfg, _ := core.LoadConfig()
	if strings.ToLower(startWithBoot) == "true" || strings.ToLower(startWithBoot) == "yes" {
		_ = driver.InstallAutostart(newCfg)
		fmt.Println("Startup registration enabled.")
	} else {
		_ = driver.UninstallAutostart(newCfg)
		fmt.Println("Startup registration removed.")
	}

	fmt.Println("Setup complete! You can run 'attendance' to start the service.")
}

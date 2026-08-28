package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ClockInMode string

const (
	ClockInModeWeb    ClockInMode = "web"
	ClockInModeRemote ClockInMode = "remote"
	ClockInModeAuto   ClockInMode = "auto"
)

type Config struct {
	BaseDir                         string        `json:"base_dir"`
	DataDir                         string        `json:"data_dir"`
	ConfigFile                      string        `json:"config_file"`
	CompanyName                     string        `json:"company_name"`
	AttendanceURL                   string        `json:"attendance_url"`
	ClockInMode                     ClockInMode   `json:"clock_in_mode"`
	SkipCheckFrom                   string        `json:"skip_check_from"`
	SkipCheckUntil                  string        `json:"skip_check_until"`
	CheckInterval                   time.Duration `json:"check_interval"`
	ManualAttentionInterval         time.Duration `json:"manual_attention_interval"`
	ClockInControlTimeout           time.Duration `json:"clock_in_control_timeout"`
	ClockOutControlTimeout          time.Duration `json:"clock_out_control_timeout"`
	CDPConnectTimeout               time.Duration `json:"cdp_connect_timeout"`
	DebugHost                       string        `json:"debug_host"`
	DebugPort                       int           `json:"debug_port"`
	ChromeProfileDirectory          string        `json:"chrome_profile_directory"`
	ChromePath                      string        `json:"chrome_path"`
	ShowToastUI                     bool          `json:"show_toast_ui"`
	ShowLoggedDate                  bool          `json:"show_logged_date"`
	ToastHeight                     int           `json:"toast_height"`
	BarHeight                       int           `json:"bar_height"`
	DisableChromeBackgroundServices bool          `json:"disable_chrome_background_services"`
	DisableAllUI                    bool          `json:"disable_all_ui"`
	ChromeVisible                   bool          `json:"chrome_visible"`
	StartWithWindows                bool          `json:"start_with_windows"`
	StartupShortcutName             string        `json:"startup_shortcut_name"`
}

var companyNameRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func IsValidCompanyName(name string) bool {
	if !companyNameRegex.MatchString(strings.ToLower(name)) {
		return false
	}
	switch strings.ToLower(name) {
	case "example", "company", "your-company", "test", "":
		return false
	default:
		return true
	}
}

func parseBool(val string, fallback bool) bool {
	val = strings.ToLower(strings.TrimSpace(val))
	if val == "" {
		return fallback
	}
	switch val {
	case "true", "1", "yes", "y", "on":
		return true
	case "false", "0", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func parseInt(val string, fallback int) int {
	val = strings.TrimSpace(val)
	if val == "" {
		return fallback
	}
	num, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return num
}

func LoadConfig() (*Config, error) {
	baseDir := os.Getenv("ATTENDANCE_BASE_DIR")
	if baseDir == "" {
		exe, err := os.Executable()
		if err == nil {
			baseDir = filepath.Dir(exe)
		} else {
			baseDir = "."
		}
	}

	dataDir := os.Getenv("ATTENDANCE_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join(baseDir, "data")
	}

	configFile := os.Getenv("ATTENDANCE_CONFIG_FILE")
	if configFile == "" {
		configFile = filepath.Join(dataDir, "config.txt")
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir %s: %w", dataDir, err)
	}

	values := make(map[string]string)
	if f, err := os.Open(configFile); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}

	company := values["COMPANY_NAME"]
	if company == "" {
		company = "example"
	}

	clockInModeStr := strings.ToLower(values["CLOCK_IN_MODE"])
	var clockInMode ClockInMode
	switch clockInModeStr {
	case "remote":
		clockInMode = ClockInModeRemote
	case "auto":
		clockInMode = ClockInModeAuto
	default:
		clockInMode = ClockInModeWeb
	}

	skipFrom := values["SKIP_CHECK_FROM"]
	if skipFrom == "" {
		skipFrom = "00:00"
	}
	skipUntil := values["SKIP_CHECK_UNTIL"]
	if skipUntil == "" {
		skipUntil = "08:00"
	}

	checkIntervalMs := parseInt(values["CHECK_INTERVAL_MS"], 60000)
	manualAttentionMs := parseInt(values["MANUAL_ATTENTION_INTERVAL_MS"], 150000)
	clockInTimeoutMs := parseInt(values["CLOCK_IN_CONTROL_TIMEOUT_MS"], 30000)
	clockOutTimeoutMs := parseInt(values["CLOCK_OUT_CONTROL_TIMEOUT_MS"], 15000)
	cdpTimeoutMs := parseInt(values["CDP_CONNECT_TIMEOUT_MS"], 120000)

	debugHost := values["DEBUG_HOST"]
	if debugHost == "" {
		debugHost = "127.0.0.1"
	}
	debugPort := parseInt(values["DEBUG_PORT"], 9222)

	profileDir := values["CHROME_PROFILE_DIRECTORY"]
	if profileDir == "" {
		profileDir = "Default"
	}

	shortcutName := values["STARTUP_SHORTCUT_NAME"]
	if shortcutName == "" {
		shortcutName = "Attendance Automation"
	}

	cfg := &Config{
		BaseDir:                         baseDir,
		DataDir:                         dataDir,
		ConfigFile:                      configFile,
		CompanyName:                     company,
		AttendanceURL:                   fmt.Sprintf("https://%s.keka.com/#/me/attendance/logs", company),
		ClockInMode:                     clockInMode,
		SkipCheckFrom:                   skipFrom,
		SkipCheckUntil:                  skipUntil,
		CheckInterval:                   time.Duration(checkIntervalMs) * time.Millisecond,
		ManualAttentionInterval:         time.Duration(manualAttentionMs) * time.Millisecond,
		ClockInControlTimeout:           time.Duration(clockInTimeoutMs) * time.Millisecond,
		ClockOutControlTimeout:          time.Duration(clockOutTimeoutMs) * time.Millisecond,
		CDPConnectTimeout:               time.Duration(cdpTimeoutMs) * time.Millisecond,
		DebugHost:                       debugHost,
		DebugPort:                       debugPort,
		ChromeProfileDirectory:          profileDir,
		ChromePath:                      values["CHROME_PATH"],
		ShowToastUI:                     parseBool(values["SHOW_TOAST_UI"], true),
		ShowLoggedDate:                  parseBool(values["SHOW_LOGGED_DATE"], true),
		ToastHeight:                     parseInt(values["TOAST_HEIGHT"], 32),
		BarHeight:                       parseInt(values["BAR_HEIGHT"], 2),
		DisableChromeBackgroundServices: parseBool(values["DISABLE_CHROME_BACKGROUND_SERVICES"], true),
		DisableAllUI:                    parseBool(values["DISABLE_ALL_UI"], false),
		ChromeVisible:                   parseBool(values["CHROME_VISIBLE"], false),
		StartWithWindows:                parseBool(values["START_WITH_WINDOWS"], true),
		StartupShortcutName:             shortcutName,
	}

	return cfg, nil
}

func DefaultConfigTemplate() string {
	return `# Attendance automation settings. Restart after changing values.
COMPANY_NAME=example
CLOCK_IN_MODE=web
SKIP_CHECK_FROM=00:00
SKIP_CHECK_UNTIL=08:00
CHECK_INTERVAL_MS=60000
MANUAL_ATTENTION_INTERVAL_MS=150000
CLOCK_IN_CONTROL_TIMEOUT_MS=30000
CLOCK_OUT_CONTROL_TIMEOUT_MS=15000
CDP_CONNECT_TIMEOUT_MS=120000
DEBUG_HOST=127.0.0.1
DEBUG_PORT=9222
CHROME_PROFILE_DIRECTORY=Default
# CHROME_PATH=
START_WITH_WINDOWS=true
STARTUP_SHORTCUT_NAME=Attendance Automation
SHOW_TOAST_UI=true
SHOW_LOGGED_DATE=true
# Thickness of the status line at the very top of screen in pixels (e.g. 1, 2, 3)
BAR_HEIGHT=2
TOAST_HEIGHT=32
DISABLE_CHROME_BACKGROUND_SERVICES=true
DISABLE_ALL_UI=false
CHROME_VISIBLE=false
`
}

func EnsureConfigFile(configFile string) error {
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(configFile), 0755); err != nil {
			return err
		}
		return os.WriteFile(configFile, []byte(DefaultConfigTemplate()), 0644)
	}
	return nil
}

# Attendance Automation (Go Edition)

A high-performance, single-binary cross-platform service that automatically checks your Keka attendance, clocks in when needed, and provides a lightweight native desktop status bar.

Written in pure Go with zero runtime dependencies (no Node.js, no npm, no Electron, no Python).

---

## Key Features

- **Single Binary Runnable**: Compiles into a single standalone executable for **Windows** (`attendance.exe`), **macOS** (`attendance`), and **Linux** (`attendance`).
- **Ultra Lightweight**: Consumes only ~8–12 MB of RAM (compared to ~300 MB for Node + Electron).
- **Decoupled Architecture**:
  - `pkg/core`: 100% platform-independent engine containing the automation loop, state management, quiet window scheduler, embedded web dashboard, and zero-dependency Chrome DevTools Protocol (CDP) WebSocket client.
  - `pkg/platform`: Platform Driver (HAL) handling OS-specific window overlays, autostart services, and process lifecycle.
- **Native Status UI**:
  - **Windows**: Native Win32 transparent, topmost, click-through status bar overlay.
  - **Linux**: Pure Go X11 (`xgb`) topmost dock/overlay and desktop notifications (`notify-send`).
  - **macOS**: Native desktop notifications and AppleScript activation.
- **Embedded Web Dashboard**: Accessible at `http://127.0.0.1:9333` with live status, date, color indicators, and logs.
- **Flexible Browser Integration**:
  - Automatically detects installed browsers in order: **Google Chrome**, **Microsoft Edge**, **Chromium**, **Brave**.
  - Built-in portable browser downloader: run `attendance --download-browser` to download the official lightweight `chrome-headless-shell` directly into `data/browser/`.

---

## Status Indicators

| Color | Status | Description |
| :--- | :--- | :--- |
| **Green** | `Logged [Date]` | Attendance is confirmed logged for today |
| **Yellow** | `Checking` | Attendance check currently running in background |
| **Coral/Red**| `Needs attention` | Manual attention needed (e.g. login / 2FA) |
| **Gray** | `Not logged` | Attendance not yet logged |

---

## Quick Start

### 1. Build Single Executable

```bash
# Build for current OS
make build

# Or cross-compile for all operating systems (Windows, Mac, Linux)
make build-all
```

The compiled binaries will be placed in `bin/`:
- `bin/attendance` (Linux)
- `bin/attendance.exe` (Windows)
- `bin/attendance-darwin-arm64` (macOS Apple Silicon)
- `bin/attendance-darwin-amd64` (macOS Intel)

### 2. Configuration Setup

Run the interactive setup wizard:

```bash
./bin/attendance --setup
```

Or edit `data/config.txt` directly:

```ini
# Attendance automation settings.
COMPANY_NAME=yourcompany
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
START_WITH_WINDOWS=true
STARTUP_SHORTCUT_NAME=Attendance Automation
SHOW_TOAST_UI=true
SHOW_LOGGED_DATE=true
TOAST_HEIGHT=32
DISABLE_CHROME_BACKGROUND_SERVICES=true
DISABLE_ALL_UI=false
CHROME_VISIBLE=false
```

### 3. Run Attendance Service

```bash
# Start background daemon + native toast overlay + web dashboard
./bin/attendance
```

---

## Command-Line Usage

```text
Usage of attendance:
  --status              Show current attendance status, date, browser detection, and config
  --once                Run a single attendance check and exit immediately
  --setup               Run interactive configuration wizard
  --stop                Stop all running attendance processes and background Chrome
  --download-browser    Download official portable Chrome Headless Shell into data/browser/
  --install-startup     Register attendance to start automatically on system boot
  --uninstall-startup   Remove attendance from system startup
  --dashboard           Run the web dashboard server only (default port: 9333)
  --dashboard-port int  Custom port for the web dashboard (default 9333)
  --toast               Run only the native status toast overlay window
  --config string       Custom path to config.txt
  --version             Show version and platform driver information
```

---

## Platform Autostart Support

- **Windows**: Adds a shortcut to the user's `Startup` folder (`%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup`).
- **macOS**: Registers a LaunchAgent in `~/Library/LaunchAgents/com.attendance.automation.plist`.
- **Linux**: Creates an XDG autostart desktop entry in `~/.config/autostart/attendance.desktop`.

To install autostart:
```bash
attendance --install-startup
```

To remove autostart:
```bash
attendance --uninstall-startup
```

---

## Architecture Overview

```text
├── main.go                      # Entry point & CLI handler
├── pkg/
│   ├── core/                    # 100% Platform-Independent Core Engine
│   │   ├── engine.go            # Automation lifecycle, quiet window, sleep/drift handling
│   │   ├── config.go            # Config parser, validation, and defaults
│   │   ├── store.go             # JSON store, atomic status writer, loggers
│   │   ├── cdp.go               # Zero-dependency CDP WebSocket client
│   │   ├── keka.go              # Keka inspection and clock-in execution
│   │   ├── downloader.go        # Portable Chrome Headless Shell downloader
│   │   ├── dashboard.go         # Embedded Web status dashboard
│   │   ├── events.go            # EventBus & status display formatter
│   │   └── types.go             # PlatformDriver interface & process structures
│   │
│   └── platform/                # Platform Abstraction Layer (HAL)
│       ├── driver.go            # Driver factory (GetDriver())
│       ├── driver_windows.go    # Win32 click-through overlay & Windows services
│       ├── driver_linux.go      # X11 (xgb) overlay & Linux autostart
│       ├── driver_darwin.go     # macOS notifications & launchd plist
│       └── driver_stub.go       # Headless / fallback driver
```

---

## Preserving Data & Logs

All state and logs are saved in the `data/` directory:
- `data/config.txt`: Configuration options.
- `data/attendance_store.json`: Record of dates successfully logged.
- `data/toast_status.txt`: Current automation state (`in <date>`, `run`, `out`, `error`).
- `data/attendance_log.txt`: Automation activity logs.
- `data/toast_log.txt`: Toast UI logs.

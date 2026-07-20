# Attendance Automation

Automatically checks the Keka attendance page, clocks in when needed, and shows a small status bar at the top of the screen.

## Requirements

- Windows
- Node.js LTS
- Google Chrome
- A logged-in Keka account in the attendance Chrome profile

## First-Time Setup

Open PowerShell in this folder and run:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\setup.ps1
```

The setup script installs npm dependencies, creates missing state files, validates the project, and creates an `Attendance Automation` shortcut in the current user's Startup folder.

After setup, test the launcher once:

```powershell
wscript.exe .\run_hidden.vbs
```

The launcher starts the script without opening a command window. Do not add a second manual Startup entry for the same launcher.

## Chrome Login

Automation uses a separate Chrome profile at:

```text
%LOCALAPPDATA%\ChromeDebug
```

If Keka asks for login or verification, complete it in the Chrome window opened for manual attention. Your normal Chrome windows are not closed by the attendance script.

## Toast Status

The top bar indicates:

- Green: attendance is logged for today.
- Yellow: the script is checking.
- Gray: attendance is not logged.
- Red: manual attention is needed.

The green state is confirmed using both the current status and today's entry in `attendance_store.json`. A previous day's green status is cleared automatically after the date changes.

## Useful Commands

Run visibly from PowerShell:

```powershell
npm run attendance
```

Validate JavaScript:

```powershell
npm run check
```

Stop the attendance process, toast, and only the dedicated attendance Chrome profile:

```text
close_all.bat
```

## Important Files

- `attendance.js`: main automation loop.
- `attendanceStore.js`: today's attendance state.
- `attendance_store.json`: dates marked as logged.
- `attendance_log.txt`: automation log.
- `toast_log.txt`: toast process log.
- `toast_status.txt`: current toast status.
- `run_hidden.vbs`: hidden startup launcher.
- `setup.ps1`: first-time setup script.

## Troubleshooting

If the script does not start, run `npm run check` and inspect `attendance_log.txt`.

If the toast is missing, restart with:

```powershell
wscript.exe .\run_hidden.vbs
```

If startup launches two copies, remove duplicate Startup or shell-startup entries. The attendance lock prevents concurrent copies, but only one launcher is needed.

If the laptop wakes from sleep, the script detects the delayed timer and refreshes the attendance state for the current date.

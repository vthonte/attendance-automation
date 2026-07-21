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
npm install
npm run check
```

Run `setup.ps1` once. It installs dependencies, creates missing data files, validates the project, and adds `run_hidden.vbs` to the current user's Startup folder according to `data\config.txt`.

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

The green state is confirmed using both the current status and today's entry in `data\attendance_store.json`. A previous day's green status is cleared automatically after the date changes.

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

- `src\attendance.js`: main automation loop.
- `src\attendanceStore.js`: today's attendance state.
- `data\config.txt`: editable settings.
- `data\attendance_store.json`: dates marked as logged.
- `data\attendance_log.txt`: automation log.
- `data\toast_log.txt`: toast process log.
- `data\toast_status.txt`: current toast status.
- `run_hidden.vbs`: hidden startup launcher.
- `setup.ps1`: source setup and Startup registration.
- `uninstall.ps1`: removes the Startup shortcut while preserving data.
- `close_all.bat`: stops attendance processes and dedicated Chrome.

## Troubleshooting

If the script does not start, run `npm run check` and inspect `data\attendance_log.txt`.

If the toast is missing, restart with:

```powershell
wscript.exe .\run_hidden.vbs
```

If startup launches two copies, remove duplicate Startup or shell-startup entries. The attendance lock prevents concurrent copies, but only one launcher is needed.

If the laptop wakes from sleep, the script detects the delayed timer and refreshes the attendance state for the current date.

@echo off
setlocal

set "BASE=%~dp0"

echo =====================================
echo Stop Attendance Script
echo %date% %time%
echo =====================================
echo.

echo Closing attendance Node processes...
taskkill /IM node.exe /T /F >nul 2>&1

echo Closing packaged attendance application...
powershell -NoProfile -WindowStyle Hidden -Command "Get-Process | Where-Object { $_.ProcessName -like 'Attendance Automation*' } | Stop-Process -Force -ErrorAction SilentlyContinue" >nul 2>&1

echo Closing toast Electron processes...
taskkill /IM electron.exe /T /F >nul 2>&1

echo Closing Chrome automation session only...
powershell -NoProfile -WindowStyle Hidden -Command "$profile=Join-Path $env:LOCALAPPDATA 'ChromeDebug'; Get-CimInstance Win32_Process -Filter \"Name = 'chrome.exe'\" | Where-Object { $_.CommandLine -like ('*' + $profile + '*') } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }" >nul 2>&1

if exist "%BASE%attendance_lock.txt" del "%BASE%attendance_lock.txt" >nul 2>&1
if exist "%BASE%toast_status.txt" echo out > "%BASE%toast_status.txt"

echo.
echo Done. Start run_hidden.vbs to run attendance without a cmd window.

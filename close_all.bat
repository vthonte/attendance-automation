@echo off
taskkill /IM node.exe /T /F >nul 2>&1
powershell -NoProfile -WindowStyle Hidden -Command "Get-Process | Where-Object { $_.ProcessName -like 'Attendance Automation*' } | Stop-Process -Force -ErrorAction SilentlyContinue" >nul 2>&1
taskkill /IM electron.exe /T /F >nul 2>&1
powershell -NoProfile -WindowStyle Hidden -Command "$profile=Join-Path $env:LOCALAPPDATA 'ChromeDebug'; Get-CimInstance Win32_Process -Filter \"Name = 'chrome.exe'\" | Where-Object { $_.CommandLine -like ('*' + $profile + '*') } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }" >nul 2>&1
echo Attendance processes stopped. Data and logs preserved.

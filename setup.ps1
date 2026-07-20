$ErrorActionPreference = "Stop"

$base = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $base
$launcher = Join-Path $base "run_hidden.vbs"

if (-not (Test-Path $launcher)) {
  throw "The launcher was not found: $launcher"
}

Write-Host "Setting up attendance automation in $base"

if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
  throw "Node.js is not installed or is not on PATH. Install Node.js LTS, then run setup.ps1 again."
}
if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
  throw "npm is not installed or is not on PATH. Reinstall Node.js LTS with npm enabled, then run setup.ps1 again."
}

$chrome = Join-Path $env:LOCALAPPDATA "Google\Chrome\Application\chrome.exe"
if (-not (Test-Path $chrome)) {
  throw "Google Chrome was not found at $chrome. Install Chrome, then run setup.ps1 again."
}

Write-Host "Installing npm dependencies..."
npm install --no-audit --no-fund

$chromeDebug = Join-Path $env:LOCALAPPDATA "ChromeDebug"
New-Item -ItemType Directory -Path $chromeDebug -Force | Out-Null

if (-not (Test-Path "attendance_store.json")) {
  Set-Content -LiteralPath "attendance_store.json" -Value "{}" -Encoding utf8
}
if (-not (Test-Path "toast_status.txt")) {
  Set-Content -LiteralPath "toast_status.txt" -Value "out" -Encoding ascii
}
foreach ($file in @("attendance_log.txt", "toast_log.txt")) {
  if (-not (Test-Path $file)) {
    New-Item -ItemType File -Path $file | Out-Null
  }
}
if (Test-Path "attendance_lock.txt") {
  Remove-Item -LiteralPath "attendance_lock.txt" -Force
}

Write-Host "Validating JavaScript..."
npm run check

$startup = [Environment]::GetFolderPath("Startup")
$startWithWindows = $true
$shortcutName = "Attendance Automation"
if (Test-Path "config.txt") {
  $settings = Get-Content "config.txt"
  $startupSetting = $settings | Where-Object { $_ -match '^START_WITH_WINDOWS=' } | Select-Object -First 1
  $nameSetting = $settings | Where-Object { $_ -match '^STARTUP_SHORTCUT_NAME=' } | Select-Object -First 1
  if ($startupSetting) { $startWithWindows = (($startupSetting -split "=", 2)[1].Trim().ToLower() -ne "false") }
  if ($nameSetting) { $shortcutName = ($nameSetting -split "=", 2)[1].Trim() }
}
$shortcutPath = Join-Path $startup "$shortcutName.lnk"
$oldShortcutPath = Join-Path $startup "Attendance Automation.lnk"
if (-not $startWithWindows) {
  if (Test-Path $shortcutPath) { Remove-Item -LiteralPath $shortcutPath -Force }
  Write-Host "Startup launch disabled by config.txt"
  exit 0
}
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = Join-Path $env:WINDIR "System32\wscript.exe"
$shortcut.Arguments = '"' + $launcher + '"'
$shortcut.WorkingDirectory = $base
$shortcut.WindowStyle = 1
$shortcut.Description = "Start attendance automation"
$shortcut.Save()

Write-Host "Setup complete. Startup shortcut created at: $shortcutPath"
Write-Host "Run run_hidden.vbs once now to test the attendance script and toast."

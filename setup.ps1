$ErrorActionPreference = "Stop"
$base = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $base
if (-not (Get-Command node -ErrorAction SilentlyContinue)) { throw "Node.js is required." }
if (-not (Get-Command npm -ErrorAction SilentlyContinue)) { throw "npm is required." }
if (-not (Test-Path (Join-Path $env:LOCALAPPDATA "Google\Chrome\Application\chrome.exe"))) { throw "Google Chrome is required." }
npm install
New-Item -ItemType Directory -Force data | Out-Null
if (-not (Test-Path "data\config.txt")) {
  @"
# Attendance automation settings. Restart after changing values.
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
START_WITH_WINDOWS=true
STARTUP_SHORTCUT_NAME=Attendance Automation
SHOW_TOAST_UI=true
SHOW_LOGGED_DATE=true
TOAST_HEIGHT=32
DISABLE_CHROME_BACKGROUND_SERVICES=true
DISABLE_ALL_UI=false
CHROME_VISIBLE=false
"@ | Set-Content data\config.txt -Encoding utf8
}

$configValues = @{}
foreach ($line in Get-Content "data\config.txt") {
  if ($line -match '^([^#=][^=]*)=(.*)$') { $configValues[$matches[1].Trim()] = $matches[2].Trim() }
}
function Ask-Value($key, $label) {
  $default = $configValues[$key]
  $answer = Read-Host "$label [$default]"
  if ([string]::IsNullOrWhiteSpace($answer)) { return $default }
  return $answer.Trim()
}
function Ask-YesNo($key, $label) {
  $default = if ($configValues[$key] -match '^(true|yes|y|1)$') { "Y" } else { "N" }
  do { $answer = Read-Host "$label (y/n) [$default]"; if ([string]::IsNullOrWhiteSpace($answer)) { return ($default -eq "Y") } } while ($answer -notmatch '^(?i:y|yes|n|no)$')
  return ($answer -match '^(?i:y|yes)$')
}

$configLines = @(
  "# Attendance automation settings. Restart after changing values.",
  "COMPANY_NAME=$(Ask-Value COMPANY_NAME 'Keka company name')",
  "CLOCK_IN_MODE=$(Ask-Value CLOCK_IN_MODE 'Clock-in mode: remote, web, or auto')",
  "SKIP_CHECK_FROM=$(Ask-Value SKIP_CHECK_FROM 'Do not check from time')",
  "SKIP_CHECK_UNTIL=$(Ask-Value SKIP_CHECK_UNTIL 'Resume checks at time')",
  "CHECK_INTERVAL_MS=$(Ask-Value CHECK_INTERVAL_MS 'Check interval in milliseconds')",
  "MANUAL_ATTENTION_INTERVAL_MS=$(Ask-Value MANUAL_ATTENTION_INTERVAL_MS 'Manual attention interval in milliseconds')",
  "CLOCK_IN_CONTROL_TIMEOUT_MS=$(Ask-Value CLOCK_IN_CONTROL_TIMEOUT_MS 'Clock-in control timeout in milliseconds')",
  "CLOCK_OUT_CONTROL_TIMEOUT_MS=$(Ask-Value CLOCK_OUT_CONTROL_TIMEOUT_MS 'Clock-out control timeout in milliseconds')",
  "CDP_CONNECT_TIMEOUT_MS=$(Ask-Value CDP_CONNECT_TIMEOUT_MS 'Chrome debugger timeout in milliseconds')",
  "DEBUG_HOST=$(Ask-Value DEBUG_HOST 'Chrome debugger host')",
  "DEBUG_PORT=$(Ask-Value DEBUG_PORT 'Chrome debugger port')",
  "CHROME_PROFILE_DIRECTORY=$(Ask-Value CHROME_PROFILE_DIRECTORY 'Chrome profile directory')",
  "START_WITH_WINDOWS=$(Ask-YesNo START_WITH_WINDOWS 'Start automatically with Windows')",
  "STARTUP_SHORTCUT_NAME=$(Ask-Value STARTUP_SHORTCUT_NAME 'Startup shortcut name')",
  "SHOW_TOAST_UI=$(Ask-YesNo SHOW_TOAST_UI 'Show the top status bar')",
  "SHOW_LOGGED_DATE=$(Ask-YesNo SHOW_LOGGED_DATE 'Show logged date on hover')",
  "TOAST_HEIGHT=$(Ask-Value TOAST_HEIGHT 'Toast height in pixels')",
  "DISABLE_CHROME_BACKGROUND_SERVICES=$(Ask-YesNo DISABLE_CHROME_BACKGROUND_SERVICES 'Disable Chrome background services')",
  "DISABLE_ALL_UI=$(Ask-YesNo DISABLE_ALL_UI 'Disable all UI to reduce memory')"
  "CHROME_VISIBLE=$(Ask-YesNo CHROME_VISIBLE 'Show Chrome during normal automated checks')"
)
Set-Content "data\config.txt" $configLines -Encoding utf8

if (-not (Test-Path "data\attendance_store.json")) { Set-Content data\attendance_store.json "{}" -Encoding utf8 }
if (-not (Test-Path "data\toast_status.txt")) { Set-Content data\toast_status.txt "out" -Encoding ascii }
foreach ($file in @("data\attendance_log.txt", "data\toast_log.txt")) { if (-not (Test-Path $file)) { New-Item $file -ItemType File | Out-Null } }
if (Test-Path "data\attendance_lock.txt") { Remove-Item data\attendance_lock.txt -Force }
npm run check
$startup = [Environment]::GetFolderPath("Startup")
$settings = Get-Content "data\config.txt"
$startWithWindows = $true
$shortcutName = "Attendance Automation"
$startupSetting = $settings | Where-Object { $_ -match '^START_WITH_WINDOWS=' } | Select-Object -First 1
$nameSetting = $settings | Where-Object { $_ -match '^STARTUP_SHORTCUT_NAME=' } | Select-Object -First 1
if ($startupSetting) { $startWithWindows = (($startupSetting -split "=", 2)[1].Trim().ToLower() -ne "false") }
if ($nameSetting) { $shortcutName = ($nameSetting -split "=", 2)[1].Trim() }
$shortcutPath = Join-Path $startup "$shortcutName.lnk"
if ($startWithWindows) {
  $shortcut = (New-Object -ComObject WScript.Shell).CreateShortcut($shortcutPath)
  $shortcut.TargetPath = Join-Path $env:WINDIR "System32\wscript.exe"
  $shortcut.Arguments = '"' + (Join-Path $base "run_hidden.vbs") + '"'
  $shortcut.WorkingDirectory = $base
  $shortcut.Save()
  Write-Host "Startup shortcut installed: $shortcutPath"
} elseif (Test-Path $shortcutPath) {
  Remove-Item $shortcutPath -Force
  Write-Host "Startup shortcut removed because START_WITH_WINDOWS=false"
}
Write-Host "Setup complete. Run run_hidden.vbs to start now."

$ErrorActionPreference = "Stop"

$base = Split-Path -Parent $MyInvocation.MyCommand.Path
$config = Join-Path $base "config.txt"
$shortcutName = "Attendance Automation"
$startWithWindows = $true

if (Test-Path $config) {
  $settings = Get-Content $config
  $startupSetting = $settings | Where-Object { $_ -match '^START_WITH_WINDOWS=' } | Select-Object -First 1
  $nameSetting = $settings | Where-Object { $_ -match '^STARTUP_SHORTCUT_NAME=' } | Select-Object -First 1
  if ($startupSetting) { $startWithWindows = (($startupSetting -split "=", 2)[1].Trim().ToLower() -ne "false") }
  if ($nameSetting) { $shortcutName = ($nameSetting -split "=", 2)[1].Trim() }
}

$exe = Get-ChildItem -LiteralPath $base -Filter "*.exe" -File |
  Where-Object { $_.Name -notmatch "unins|setup" } |
  Select-Object -First 1
if (-not $exe) { throw "No portable EXE was found beside this script." }

$startup = [Environment]::GetFolderPath("Startup")
$shortcutPath = Join-Path $startup "$shortcutName.lnk"

if (-not $startWithWindows) {
  if (Test-Path $shortcutPath) { Remove-Item -LiteralPath $shortcutPath -Force }
  Write-Host "Startup launch disabled by config.txt"
  exit 0
}

$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $exe.FullName
$shortcut.WorkingDirectory = $base
$shortcut.WindowStyle = 1
$shortcut.Description = "Start attendance automation"
$shortcut.Save()

Write-Host "Startup shortcut created: $shortcutPath"
Write-Host "Target: $($exe.FullName)"

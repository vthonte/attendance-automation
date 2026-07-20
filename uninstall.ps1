$ErrorActionPreference = "Stop"

$base = Split-Path -Parent $MyInvocation.MyCommand.Path
$config = Join-Path $base "config.txt"
$shortcutName = "Attendance Automation"

if (Test-Path $config) {
  $line = Get-Content $config | Where-Object { $_ -match '^STARTUP_SHORTCUT_NAME=' } | Select-Object -First 1
  if ($line) { $shortcutName = ($line -split "=", 2)[1].Trim() }
}

$startup = [Environment]::GetFolderPath("Startup")
$shortcut = Join-Path $startup "$shortcutName.lnk"
if (Test-Path $shortcut) { Remove-Item -LiteralPath $shortcut -Force }

Write-Host "Removed startup shortcut: $shortcut"
Write-Host "Attendance history, config, JSON, and logs were preserved."
Write-Host "You may now delete the application folder or portable EXE manually."

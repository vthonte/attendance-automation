$base = Split-Path -Parent $MyInvocation.MyCommand.Path
$stopper = Join-Path $base "close_all.bat"
if (Test-Path $stopper) { Start-Process $stopper -WindowStyle Hidden -Wait }
$shortcutName = "Attendance Automation"
$config = Join-Path $base "data\config.txt"
if (Test-Path $config) {
  $nameSetting = Get-Content $config | Where-Object { $_ -match '^STARTUP_SHORTCUT_NAME=' } | Select-Object -First 1
  if ($nameSetting) { $shortcutName = ($nameSetting -split "=", 2)[1].Trim() }
}
$shortcut = Join-Path ([Environment]::GetFolderPath("Startup")) "$shortcutName.lnk"
if (Test-Path $shortcut) { Remove-Item $shortcut -Force }
Write-Host "Startup shortcut removed. Logs and attendance data were preserved."
Write-Host "You can now delete this attendance folder if you want to remove the source files."

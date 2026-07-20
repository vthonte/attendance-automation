$ErrorActionPreference = "Stop"

$base = Split-Path -Parent $MyInvocation.MyCommand.Path
$dist = Join-Path $base "dist"
foreach ($file in @("config.txt", "install-portable.ps1", "uninstall.ps1", "close_all.bat")) {
  Copy-Item -LiteralPath (Join-Path $base $file) -Destination (Join-Path $dist $file) -Force
}
Write-Host "Portable release assets copied to $dist"

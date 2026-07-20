$ErrorActionPreference = "Stop"

$base = Split-Path -Parent $MyInvocation.MyCommand.Path
$dist = Join-Path $base "dist"
$package = Get-Content (Join-Path $base "package.json") -Raw | ConvertFrom-Json
$zip = Join-Path $dist ("Attendance Automation " + $package.version + ".zip")

if (Test-Path $zip) { Remove-Item -LiteralPath $zip -Force }
$files = @(
  (Join-Path $dist ("Attendance Automation " + $package.version + ".exe")),
  (Join-Path $dist "config.txt"),
  (Join-Path $dist "install-portable.ps1"),
  (Join-Path $dist "uninstall.ps1"),
  (Join-Path $dist "close_all.bat")
)

$missing = $files | Where-Object { -not (Test-Path $_) }
if ($missing) { throw "Missing portable release files: $($missing -join ', ')" }

$sevenZip = Join-Path $env:LOCALAPPDATA "electron-builder\Cache\7zip@1.0.0\7za.exe"
if (Test-Path $sevenZip) {
  $relativeFiles = $files | ForEach-Object { Split-Path $_ -Leaf }
  Push-Location $dist
  & $sevenZip a -tzip -mx=9 -y $zip $relativeFiles | Out-Null
  Pop-Location
  if ($LASTEXITCODE -ne 0) { throw "7-Zip failed with exit code $LASTEXITCODE" }
} else {
  Compress-Archive -LiteralPath $files -DestinationPath $zip -CompressionLevel Optimal
}
Write-Host "Portable ZIP created: $zip"

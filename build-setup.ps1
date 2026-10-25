# Builds the app and the per-user Windows setup into .\build:
#   build\timestamps.exe, build\Timestamps-Setup-<version>.exe
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$version = (Select-String -Path internal\server\server.go -Pattern 'const Version = "([^"]+)"').Matches[0].Groups[1].Value
& .\build.ps1
New-Item -ItemType Directory -Force tools\setup\payload | Out-Null
Copy-Item build\timestamps.exe tools\setup\payload\timestamps.exe -Force
if (-not (Test-Path tools\setup\rsrc_windows_amd64.syso)) {
  Push-Location tools\setup
  $gcc = (New-Object -ComObject Scripting.FileSystemObject).GetFile((Get-Command gcc).Source).ShortPath
  windres --preprocessor="$gcc -E -xc -DRC_INVOKED" -O coff -F pe-x86-64 -o rsrc_windows_amd64.syso setup.rc
  Pop-Location
}
$out = "build\Timestamps-Setup-$version.exe"
go build -tags setup -trimpath -ldflags "-s -w -H=windowsgui -X main.version=$version" -o $out .\tools\setup
Get-Item $out | Select-Object Name, Length

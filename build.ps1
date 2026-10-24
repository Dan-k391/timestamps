# Builds timestamps.exe (GUI subsystem) into .\build
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
if (-not (Test-Path rsrc_windows_amd64.syso)) {
  Push-Location assets
  $gcc = (New-Object -ComObject Scripting.FileSystemObject).GetFile((Get-Command gcc).Source).ShortPath
  windres --preprocessor="$gcc -E -xc -DRC_INVOKED" -O coff -F pe-x86-64 -o ..\rsrc_windows_amd64.syso app.rc
  Pop-Location
}
$env:CGO_ENABLED = '1'
New-Item -ItemType Directory -Force build | Out-Null
go build -trimpath -ldflags "-s -w -H=windowsgui" -o build\timestamps.exe .
Get-Item build\timestamps.exe | Select-Object Name, Length

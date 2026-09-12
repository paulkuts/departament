$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
if ([IO.Path]::GetPathRoot($projectRoot) -ne 'D:\') { throw 'Development files must stay on D:.' }
$sharedRoot = 'D:/Codex_Projects/Department'
$env:GOROOT = Join-Path $sharedRoot '.tools/go'
$goExe = Join-Path $env:GOROOT 'bin/go.exe'
if (-not (Test-Path -LiteralPath $goExe)) { throw "Go runtime missing: $goExe" }
$env:PATH = "$env:GOROOT/bin$([IO.Path]::PathSeparator)$env:PATH"
$env:GOCACHE = Join-Path $sharedRoot '.cache/go-build'
$env:GOMODCACHE = Join-Path $sharedRoot '.cache/go-mod'
$env:GOPATH = Join-Path $projectRoot '.local/go'
$env:GOTMPDIR = Join-Path $projectRoot '.local/tmp'
$env:TEMP = $env:GOTMPDIR
$env:TMP = $env:GOTMPDIR
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
$env:REG_CONFIG_NAME = 'local'
foreach ($dir in @('.local/bin', '.local/tmp', '.local/data', '.local/avatars', '.local/photos', '.local/export')) {
    New-Item -ItemType Directory -Force -Path (Join-Path $projectRoot $dir) | Out-Null
}

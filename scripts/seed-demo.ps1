[CmdletBinding()]
param()
. (Join-Path $PSScriptRoot 'runtime.ps1')
Push-Location $projectRoot
try {
    & $goExe run ./scripts/demo
    if ($LASTEXITCODE -ne 0) { throw 'Demo seed failed.' }
} finally { Pop-Location }

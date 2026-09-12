[CmdletBinding()]
param()
. (Join-Path $PSScriptRoot 'runtime.ps1')
if (-not (Test-Path -LiteralPath (Join-Path $projectRoot 'configs/local.yaml'))) {
    throw 'Run scripts/setup-local.ps1 first.'
}
Push-Location $projectRoot
try {
    $binary = Join-Path $projectRoot '.local/bin/department.exe'
    & $goExe build -o $binary .
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
    & $binary
} finally { Pop-Location }

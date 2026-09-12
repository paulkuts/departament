[CmdletBinding()]
param()
. (Join-Path $PSScriptRoot 'runtime.ps1')
Push-Location $projectRoot
try {
    & $goExe test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed.' }
    & $goExe vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Vet failed.' }
    & $goExe build -o (Join-Path $projectRoot '.local/bin/department-check.exe') .
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
    Write-Output 'Go test, vet and build passed.'
} finally { Pop-Location }

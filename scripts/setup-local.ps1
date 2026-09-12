[CmdletBinding()]
param()
. (Join-Path $PSScriptRoot 'runtime.ps1')
$configPath = Join-Path $projectRoot 'configs/local.yaml'
if (Test-Path -LiteralPath $configPath) { throw 'Local configuration exists; refusing to overwrite.' }
$random = New-Object byte[] 48
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
$rng.GetBytes($random)
$rng.Dispose()
$secret = [Convert]::ToBase64String($random)
$localRoot = ($projectRoot -replace '\\', '/') + '/.local'
$yaml = @"
app:
  name: Department local demo
  env: development
auth:
  access_token_ttl: 1h
  refresh_token_ttl: 24h
  jwt_secret: '$secret'
photo:
  inventory_photo_dir: '$localRoot/photos'
  avatar_photo_dir: '$localRoot/avatars'
  max_photo_size: 10485760
  max_photos: 10
db:
  local_path: '$localRoot/data/demo.db'
  data_dir: '$localRoot/data'
  export_dir: '$localRoot/export'
logger:
  level: info
  encoding: console
  output_paths: [stdout]
  error_output_paths: [stderr]
server:
  host: '127.0.0.1'
  port: '18182'
  max_header_bytes: 1048576
  read_timeout: 5s
  write_timeout: 10s
  idle_timeout: 120s
"@
[IO.File]::WriteAllText($configPath, $yaml, [Text.UTF8Encoding]::new($false))
Write-Output 'Created ignored local config; run scripts/seed-demo.ps1, then scripts/dev.ps1.'

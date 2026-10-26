$ErrorActionPreference='Stop'
$releaseRoot=Split-Path $PSScriptRoot -Parent
$goExecutable=(Get-Command go -ErrorAction Stop).Source
$savedBuildEnvironment=@{}
foreach ($name in @('CGO_ENABLED','GOARCH','GOOS')) { $savedBuildEnvironment[$name]=[Environment]::GetEnvironmentVariable($name,'Process') }
New-Item -ItemType Directory -Force -Path (Join-Path $releaseRoot 'binaries/linux-amd64'),(Join-Path $releaseRoot 'binaries/windows-amd64') | Out-Null
Push-Location (Join-Path $releaseRoot 'source')
try {
    $env:CGO_ENABLED='0'
    $env:GOARCH='amd64'
    $env:GOOS='linux'
    & $goExecutable build -mod=vendor -trimpath -o (Join-Path $releaseRoot 'binaries/linux-amd64/layerhub') ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw 'Linux build failed' }
    & $goExecutable build -mod=vendor -trimpath -o (Join-Path $releaseRoot 'binaries/linux-amd64/layerhub-mail-worker') ./cmd/mail-worker
    if ($LASTEXITCODE -ne 0) { throw 'Mail worker build failed' }
    $env:GOOS='windows'
    & $goExecutable build -mod=vendor -trimpath -o (Join-Path $releaseRoot 'binaries/windows-amd64/layerhub.exe') ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw 'Windows build failed' }
    python tools/make-source.py
    if ($LASTEXITCODE -ne 0) { throw 'Source offer generation failed' }
} finally {
    Pop-Location
    foreach ($name in $savedBuildEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name,$savedBuildEnvironment[$name],'Process') }
}

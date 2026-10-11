# build.ps1 — reproducible Windows release build of pgdeploy.exe.
# Produces a static (CGO_ENABLED=0) amd64 binary under dist/.
$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $repoRoot "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"

$version = "1.0.0"
$commit = (git -C $repoRoot rev-parse --short HEAD 2>$null)
if (-not $commit) { $commit = "unknown" }
$buildTime = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")

$ldflags = "-s -w " +
    "-X github.com/pgdeploy/pgdeploy/internal/version.GitCommit=$commit " +
    "-X github.com/pgdeploy/pgdeploy/internal/version.BuildTime=$buildTime"

Write-Host "Building pgdeploy.exe (version $version, commit $commit) ..."
go build -trimpath -ldflags $ldflags -o (Join-Path $dist "pgdeploy.exe") ./cmd/pgdeploy

Write-Host "Built $($dist)\pgdeploy.exe"

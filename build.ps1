# build.ps1 - Compile binary Go menjadi .exe untuk Windows Server
# Jalankan di mesin dengan Go terpasang (atau cross-compile dari mesin lain).
#
# Pemakaian:  powershell -ExecutionPolicy Bypass -File .\build.ps1

$ErrorActionPreference = "Stop"
Set-Location -Path $PSScriptRoot

Write-Host "Go version:"
go version
if ($LASTEXITCODE -ne 0) { Write-Host "Go tidak ditemukan / tidak jalan. Install Go dulu: https://go.dev/dl/"; exit 1 }

$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"   # binary statis, tidak butuh DLL tambahan

$out = Join-Path $PSScriptRoot "cron-job-azure.exe"
Write-Host "Building -> $out"
go build -trimpath -ldflags "-s -w" -o $out .
if ($LASTEXITCODE -ne 0) { Write-Host "Build GAGAL"; exit 1 }

Write-Host "Build sukses: $out"

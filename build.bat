@echo off
REM build.bat - Compile binary Go menjadi cron-job-azure.exe (Windows)
cd /d %~dp0

set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0

go build -trimpath -ldflags "-s -w" -o cron-job-azure.exe .
if errorlevel 1 (
    echo Build GAGAL
    exit /b 1
)

echo Build sukses: cron-job-azure.exe

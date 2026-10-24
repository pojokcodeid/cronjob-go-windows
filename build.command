#!/bin/bash
# build.command - Compile binary Go menjadi cron-job-azure.exe (target Windows, dijalankan dari macOS)
cd "$(dirname "$0")" || exit 1

export GOOS=windows
export GOARCH=amd64
export CGO_ENABLED=0

go build -trimpath -ldflags "-s -w" -o cron-job-azure.exe .
if [ $? -ne 0 ]; then
  echo "Build GAGAL"
  exit 1
fi

echo "Build sukses: cron-job-azure.exe"

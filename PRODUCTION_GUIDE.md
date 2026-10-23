# Production Deployment Guide (Windows Server) - Versi Go

Panduan deploy aplikasi **cron-job-azure** (Go) di Windows Server.
Aplikasi berupa **1 file exe** dan dijalankan sebagai **Windows Service**
sehingga otomatis menyala lagi setelah server di-restart.

> Versi Node.js (PM2 + WinSW) ada di `../cron-job-azure/PRODUCTION_GUIDE.md`.
> Versi ini **tidak memerlukan Node.js, npm, maupun PM2** di server produksi.

## Prasyarat

1. **Go** (hanya jika ingin build di server): [Download](https://go.dev/dl/)
   — atau build di mesin lain lalu salin `cron-job-azure.exe`.
2. **PowerShell Administrator** untuk memasang service.
3. File `.env` berisi konfigurasi (database, API, APIKEY).

## 1. Build

Di folder project (mesin dengan Go terpasang):

```powershell
powershell -ExecutionPolicy Bypass -File .\build.ps1
# atau dari cmd: build.bat
```

Hasilnya: `cron-job-azure.exe` (statis, CGO disabled).

Salin ke server, misal: `E:\cron-job-azure\`

```
E:\cron-job-azure\
├── cron-job-azure.exe
├── .env
├── build.ps1 / build.bat           (opsional, untuk build ulang)
├── service-install.ps1
└── service-uninstall.ps1
```

## 2. Konfigurasi

Salin `.env_example` → `.env` lalu isi:

```ini
PORT=3000
API_BASE_URL=https://xxxxxx.com/
API_TIMEOUT=120000
CRON_SCHEDULE=*/3 * * * *

ACCNAME=xxxxxxxx
APIKEY=xxxxxxxx

DB_USER=sa
DB_PASSWORD=...
DB_SERVER=...
DB_NAME=attendance_prod
DB_PORT=1433
DB_ENCRYPT=true
# true bila sertifikat server self-signed (Azure SQL: biarkan false)
# DB_TRUST_SERVER_CERTIFICATE=true

# Database GD (pola sama, sufiks _GD)
DB_USER_GD=...
DB_PASSWORD_GD=...
DB_SERVER_GD=...
DB_NAME_GD=...
DB_PORT_GD=1433
DB_ENCRYPT_GD=true
# Server dengan sertifikat self-signed (contoh: SSL_Self_Signed_Fallback)
# wajib true, jika tidak muncul error x509 "not standards compliant"
DB_TRUST_SERVER_CERTIFICATE_GD=true
```

Catatan:

- `CRON_SCHEDULE` default `*/3 * * * *` (tiap 3 menit), sama dengan `src/index.js`.
- `DB_TRUST_SERVER_CERTIFICATE[_GD]` default `false` (sama seperti
  `options.trustServerCertificate` di versi Node). Set `true` untuk server
  yang bersertifikat self-signed — meski `DB_ENCRYPT=false`, driver tetap
  melakukan TLS handshake saat prelogin TDS sehingga sertifikat tetap
  diverifikasi oleh Go.
- `LOG_DIR` opsional; defaultnya folder `logs\` di samping exe.
- Saat berjalan sebagai service, aplikasi mencari `.env` di folder exe.

## 3. Test jalankan (console)

```powershell
cd E:\cron-job-azure
.\cron-job-azure.exe
```

Harus muncul:

```
[LOGGER] ... info : Connected to Azure SQL for HKV
[LOGGER] ... info : Server running at http://localhost:3000
[LOGGER] ... info : Cron job scheduled (*/3 * * * *)
```

Cek endpoint: buka `http://localhost:3000/` → JSON `resultData`.
Tekan `Ctrl+C` untuk berhenti.

## 4. Pasang Windows Service (sekali saja)

Buka **PowerShell sebagai Administrator**:

```powershell
cd E:\cron-job-azure
powershell -ExecutionPolicy Bypass -File .\service-install.ps1
```

Script ini akan:

1. `cron-job-azure.exe install` → mendaftarkan service `cron-job-azure`.
2. `Set-Service -StartupType Automatic` → **auto start saat server restart**
   (dengan Delayed Auto Start).
3. Recovery **restart otomatis** bila proses crash (delay 10 detik).
4. `Start-Service` → langsung menjalankan service.

Verifikasi:

```powershell
Get-Service cron-job-azure
# Status harus: Running
sc.exe qc cron-job-azure
# START_TYPE harus: AUTO_START
```

## 5. Verifikasi auto-start setelah restart server

1. Restart Windows Server.
2. Setelah hidup, buka PowerShell:

```powershell
Get-Service cron-job-azure        # Status: Running
Get-EventLog -LogName Application -Source cron-job-azure -Newest 10
```

3. Cek log aplikasi (rotasi harian):

```powershell
Get-Content .\logs\app-$(Get-Date -Format yyyy-MM-dd).log -Tail 50
```

## 6. Maintenance / update

```powershell
# 1. Hentikan service
Stop-Service cron-job-azure

# 2. Ganti cron-job-azure.exe dengan versi baru, pastikan .env sesuai

# 3. Nyalakan lagi
Start-Service cron-job-azure
Get-Service cron-job-azure
```

## 7. Monitoring log

- **Aplikasi**: `.\logs\app-YYYY-MM-DD.log`
  - rotate harian, compress `.gz`, otomatis dihapus setelah 14 hari.
- **Service (Windows)**: `Get-EventLog -LogName Application -Source cron-job-azure`
- **Console test**: jalankan tanpa argumen (semua level berwarna).

## 8. Uninstall service

```powershell
powershell -ExecutionPolicy Bypass -File .\service-uninstall.ps1
```

Atau manual:

```powershell
Stop-Service cron-job-azure
.\cron-job-azure.exe uninstall
```

## Troubleshooting

| Gejala | Solusi |
| --- | --- |
| `install` gagal "access denied" | Jalankan PowerShell sebagai Administrator |
| Service tidak start, log kosong | Cek `.env` ada di folder exe & nilai `DB_*` benar |
| `Get-Service` tidak menemukan service | Pastikan `service-install.ps1` dieksekusi dari folder exe |
| Data tidak terkirim | Lihat `logs\app-*.log` → cek `Error process send attendance` |
| Ingin ganti jadwal | Ubah `CRON_SCHEDULE` di `.env`, lalu `Restart-Service cron-job-azure` |
| `x509: "SSL_Self_Signed_Fallback" certificate is not standards compliant` | Set `DB_TRUST_SERVER_CERTIFICATE=true` (atau `..._GD=true` untuk DB GD) di `.env`, lalu `Restart-Service cron-job-azure` |
| Gagal koneksi database | Jalankan `go run ./cmd/dbping` dari mesin dev untuk diagnosa TLS/kredensial |

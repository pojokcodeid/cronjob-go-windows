# cron-job-azure (Go)

Terjemahan **1:1** dari proyek Node.js `../cron-job-azure` ke bahasa Go:
sinkronisasi data absen dari 2 database SQL Server (HKV & GD) ke API eksternal
(`qlid=CL_AttendanceTemp.setProcessAtt`) lewat cron, plus HTTP server untuk
pengecekan data.

Ditambah: **compile ke 1 file `.exe`** yang bisa dijalankan sebagai
**Windows Service** dengan startup **Automatic** → otomatis menyala lagi
saat Windows Server di-restart (tanpa Node.js, tanpa PM2).

## Struktur

```
cron-job-azure-go/
├── main.go                        # entrypoint + Windows Service (kardianos/service)
├── internal/
│   ├── app/app.go                 # pengganti src/index.js (Express + node-cron)
│   ├── config/config.go           # pengganti dotenv + process.env + opsi pool Db.js
│   ├── db/db.go                   # pengganti mssql ConnectionPool (Db.js)
│   ├── api/client.go              # pengganti AxiosConfig.js + RefreshToken.js
│   ├── attendance/repository.go   # pengganti src/services/AttendaceService.js
│   ├── attendance/sender.go       # pengganti src/controllers/AttendacneSend.js
│   └── logger/logger.go           # pengganti Winston.js + winston-daily-rotate-file
├── build.ps1 / build.bat          # compile ke cron-job-azure.exe
├── service-install.ps1            # daftarkan sebagai Windows Service (auto start)
├── service-uninstall.ps1          # hapus Windows Service
├── .env_example                   # template konfigurasi (sama dengan versi Node)
└── PRODUCTION_GUIDE.md            # panduan deploy di Windows Server
```

## Pemetaan modul

| Node.js (cron-job-azure)                       | Go (cron-job-azure-go)                                |
| ---------------------------------------------- | ----------------------------------------------------- |
| `src/index.js` (express + node-cron)           | `internal/app/app.go` + `main.go` (net/http + robfig/cron) |
| `src/services/AttendaceService.js`             | `internal/attendance/repository.go`                   |
| `src/controllers/AttendacneSend.js`            | `internal/attendance/sender.go`                       |
| `src/utils/AxiosConfig.js` (interceptor 401)   | `internal/api/client.go` (token + retry 401)          |
| `src/utils/RefreshToken.js`                    | `internal/api/client.go` → `refreshToken()`           |
| `src/config/Db.js` (2 pool mssql)              | `internal/db/db.go` (database/sql + go-mssqldb)       |
| `src/utils/Winston.js` (rotasi harian)         | `internal/logger/logger.go` (rotasi harian + gzip)     |
| `dotenv`                                       | `godotenv` (baca `.env`, fallback ke folder exe)      |
| PM2 + WinSW (`pm2-*.ps1/bat`)                  | **Windows Service** (`service-install.ps1`)           |

## Perilaku yang dijaga sama

- Cron `*/3 * * * *` (default, bisa diubah via `CRON_SCHEDULE`) dengan guard
  anti tumpang tindih ("Skip cron: proses sebelumnya masih berjalan").
- Urutan job: `AttendacneSend` (HKV) dulu, baru `AttendacneSendGD` (GD);
  bila HKV gagal, GD dilewati (sama seperti try/catch di versi Node).
- Query SQL, payload JSON (`MACHINE_CODE`, `COMPANY_ID`, `TEMP_ID`, ...),
  update `status = 1` / `proc_sf7 = 1` berdasarkan `DATA.RESULT` dari API.
- Auth: token diambil otomatis sebelum request pertama; `401` → refresh token
  lalu retry 1 kali (sama seperti interceptor axios).
- Response non-2xx dianggap error dan di-log lengkap (Status + Data dari server).
- Log: console berwarna (semua level) + file `logs/app-YYYY-MM-DD.log`
  rotasi harian, compress `.gz`, dibersihkan setelah 14 hari,
  format `[LOGGER] <timestamp> <level> : <message>`.

## Menjalankan (development)

```bash
cp .env_example .env      # isi nilainya
go run .
# atau
go build -o cron-job-azure . && ./cron-job-azure
```

Endpoint: `GET http://localhost:3000/` → `{"resultData":[...],"resultId":[...]}`.

Jalankan perintah (testing):

```bash
go test ./...                          # unit test logika auth & payload
go run ./cmd/dbping                    # diagnosa koneksi ke 2 database (GD & HKV)
```

## Compile untuk Windows Server

Di mesin Windows (atau cross-compile dari mesin lain):

```powershell
powershell -ExecutionPolicy Bypass -File .\build.ps1
# hasilnya: cron-job-azure.exe (1 file, CGO disabled / statis)
```

atau via cmd: `build.bat`.

## Menjalankan sebagai Windows Service (auto-start saat restart server)

```powershell
# 1. build dulu
.\build.ps1

# 2. siapkan .env di folder yang sama dengan exe
copy .env_example .env
notepad .env

# 3. daftarkan + jalankan service (PowerShell Administrator)
powershell -ExecutionPolicy Bypass -File .\service-install.ps1
```

Service `cron-job-azure` terpasang dengan:
- **Startup: Automatic** (+ Delayed Auto Start) → hidup otomatis saat server restart.
- **Recovery: restart** → kalau proses crash, Windows otomatis menyalakan lagi.

Cek: `Get-Service cron-job-azure`, `sc query cron-job-azure`.

Uninstall: `powershell -ExecutionPolicy Bypass -File .\service-uninstall.ps1`.

Mode console (tanpa argumen) tetap bisa dipakai untuk testing:
`.\cron-job-azure.exe`.

## Catatan perbedaan kecil (disengaja)

- Jika database gagal di-connect saat startup, aplikasi **tetap jalan** dan
  mencoba connect ulang pada query berikutnya (versi Node langsung crash),
  supaya cocok untuk Windows Service.
- Batas eksekusi per cron run: 10 menit (context timeout) supaya job tidak
  menggantung selamanya.
- Log & `.env` dicari relatif ke folder exe (bukan CWD), karena saat berjalan
  sebagai service CWD Windows biasanya `C:\Windows\System32`.
- Interval cron default mengikuti `src/index.js` yaitu `*/3 * * * *`
  (komentar di file menyebut 15 menit, tetapi kodenya 3 menit).

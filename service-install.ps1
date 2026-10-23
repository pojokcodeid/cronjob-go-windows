# service-install.ps1
# Mendaftarkan cron-job-azure.exe sebagai Windows Service dengan startup
# AUTOMATIC, sehingga otomatis menyala lagi saat server Windows di-restart.
#
# Jalankan PowerShell sebagai Administrator:
#   powershell -ExecutionPolicy Bypass -File .\service-install.ps1

# --- cek hak Administrator ---
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
$isAdmin = $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if ($isAdmin -ne $true) {
    Write-Host "Script ini HARUS dijalankan dengan hak Administrator (Run as administrator)."
    return
}

Set-Location -Path $PSScriptRoot

$exe = Join-Path $PSScriptRoot "cron-job-azure.exe"
if (-not (Test-Path $exe)) {
    Write-Host "Binary tidak ditemukan: $exe"
    Write-Host "Build dulu dengan: .\build.ps1"
    return
}

# Pastikan file .env sudah ada (dibutuhkan aplikasi).
if (-not (Test-Path (Join-Path $PSScriptRoot ".env"))) {
    Write-Host "PERINGATAN: file .env tidak ditemukan di $PSScriptRoot"
    Write-Host "Salin .env_example menjadi .env lalu isi nilainya."
}

# Daftarkan service (bila sudah terdaftar, uninstall dulu agar path exe benar).
Write-Host "Mendaftarkan Windows Service 'cron-job-azure' ..."
$svc = Get-Service -Name "cron-job-azure" -ErrorAction SilentlyContinue
if ($null -ne $svc) {
    & $exe stop
    Start-Sleep -Seconds 2
    & $exe uninstall
    Start-Sleep -Seconds 2
}
& $exe install
if ($LASTEXITCODE -ne 0) { Write-Host "Install service GAGAL"; exit 1 }

# Paksa startup Automatic (auto start saat server restart).
Set-Service -Name "cron-job-azure" -StartupType Automatic

# Jalankan service.
Start-Service -Name "cron-job-azure"
Start-Sleep -Seconds 3

Write-Host ""
Get-Service -Name "cron-job-azure" | Format-Table Status, Name, DisplayName -AutoSize
Write-Host "Selesai. Service akan otomatis menyala saat Windows Server di-restart."

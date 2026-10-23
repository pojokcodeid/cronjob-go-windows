# service-uninstall.ps1
# Menghentikan dan menghapus Windows Service 'cron-job-azure'.
#
# Jalankan PowerShell sebagai Administrator:
#   powershell -ExecutionPolicy Bypass -File .\service-uninstall.ps1

$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
$isAdmin = $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if ($isAdmin -ne $true) {
    Write-Host "Script ini HARUS dijalankan dengan hak Administrator."
    return
}

Set-Location -Path $PSScriptRoot

$svc = Get-Service -Name "cron-job-azure" -ErrorAction SilentlyContinue
if ($null -eq $svc) {
    Write-Host "Service 'cron-job-azure' tidak terdaftar."
    return
}

$exe = Join-Path $PSScriptRoot "cron-job-azure.exe"
if (Test-Path $exe) {
    & $exe stop
    Start-Sleep -Seconds 2
    & $exe uninstall
} else {
    Write-Host "Binary tidak ditemukan, menghapus via sc.exe ..."
    sc.exe stop cron-job-azure | Out-Null
    sc.exe delete cron-job-azure | Out-Null
}

Start-Sleep -Seconds 2
Get-Service -Name "cron-job-azure" -ErrorAction SilentlyContinue | Format-Table Status, Name -AutoSize
Write-Host "Uninstall selesai."

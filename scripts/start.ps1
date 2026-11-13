$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$mysqld = "C:\Program Files\MySQL\MySQL Server 8.0\bin\mysqld.exe"
$mysql = "C:\Program Files\MySQL\MySQL Server 8.0\bin\mysql.exe"
$go = "D:\git\.tools\go\bin\go.exe"
if (-not (Test-Path $go)) { $go = "go" }

$listening = Get-NetTCPConnection -State Listen -LocalPort 3307 -ErrorAction SilentlyContinue
if (-not $listening) {
    Write-Host "Starting MySQL 8.0 on port 3307..."
    Start-Process -FilePath $mysqld -ArgumentList "--defaults-file=$root\my.ini" -WindowStyle Hidden
    Start-Sleep -Seconds 3
}

& $mysql -h 127.0.0.1 -P 3307 -u root -e "CREATE DATABASE IF NOT EXISTS umc_pmday CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

Push-Location $root
try {
    Write-Host "Starting UMC PM Day Go Server on http://localhost:8080 ..."
    & $go run ./cmd/server
} finally {
    Pop-Location
}

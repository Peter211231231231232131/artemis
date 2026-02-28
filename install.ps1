# Xon - Official Installer
# Run with: powershell -ExecutionPolicy Bypass -File install.ps1
# Or: .\install.ps1

param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Xon",
    [switch]$Uninstall,
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$ExeName = "xon.exe"

function Get-ScriptDir {
    if ($PSCommandPath) {
        Split-Path -Parent $PSCommandPath
    } else {
        Get-Location
    }
}

function Install-Xon {
    $root = Get-ScriptDir
    
    Write-Host "Xon Official Installer" -ForegroundColor Cyan
    Write-Host "======================" -ForegroundColor Cyan
    
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Error "Go is not installed or not in PATH. Install Go from https://go.dev/dl/"
    }
    
    Set-Location $root
    Write-Host "Building $ExeName ..." -ForegroundColor Yellow
    go build -o $ExeName main.go
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Build failed."
    }
    
    if (-not (Test-Path $ExeName)) {
        Write-Error "Build did not produce $ExeName"
    }
    
    $targetDir = $InstallDir
    if (-not $Force -and (Test-Path $targetDir)) {
        $overwrite = Read-Host "Directory $targetDir exists. Overwrite? (y/N)"
        if ($overwrite -ne "y" -and $overwrite -ne "Y") {
            Write-Host "Install cancelled."
            return
        }
    }
    
    New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
    Copy-Item -Path (Join-Path $root $ExeName) -Destination $targetDir -Force
    Write-Host "Installed to: $targetDir" -ForegroundColor Green
    
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $targetDirNorm = (Resolve-Path $targetDir).Path
    if ($userPath -notlike "*$targetDirNorm*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$targetDirNorm", "User")
        Write-Host "Added to user PATH: $targetDirNorm" -ForegroundColor Green
        $env:Path = "$env:Path;$targetDirNorm"
    } else {
        Write-Host "Already in user PATH." -ForegroundColor Gray
    }
    
    Write-Host ""
    Write-Host "Installation complete. Run 'xon' from a new terminal, or run:" -ForegroundColor Green
    Write-Host "  xon" -ForegroundColor White
    Write-Host "  xon script.xn" -ForegroundColor White
}

function Uninstall-Xon {
    Write-Host "Xon Uninstaller" -ForegroundColor Cyan
    Write-Host "===============" -ForegroundColor Cyan
    
    $targetDir = $InstallDir
    $targetDirNorm = if (Test-Path $targetDir) { (Resolve-Path $targetDir).Path } else { $targetDir }
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $pathParts = $userPath -split ";" | Where-Object { $_ -and $_ -ne $targetDirNorm -and $_ -ne $targetDir }
    $newPath = $pathParts -join ";"
    if ($newPath -ne $userPath) {
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Host "Removed from user PATH." -ForegroundColor Green
    }
    
    if (Test-Path $targetDir) {
        Remove-Item -Path $targetDir -Recurse -Force
        Write-Host "Removed: $targetDir" -ForegroundColor Green
    }
    
    Write-Host "Uninstall complete." -ForegroundColor Green
}

if ($Uninstall) {
    Uninstall-Xon
} else {
    Install-Xon
}

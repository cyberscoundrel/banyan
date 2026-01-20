# Banyan Node Debug Startup Script (Windows PowerShell)
# This script launches the banyan node with a debug/test configuration
# for local development and testing.

param(
    [string]$ConfigFile = "",
    [string]$Listen = "/ip4/127.0.0.1/tcp/0",
    [string]$PrivKey = "",
    [string]$ServicesDir = "",
    [string]$AddonsDir = "",
    [string]$FigsDir = "",
    [switch]$NoCrypto,
    [switch]$DisableDht,
    [switch]$AllowExpired,
    [switch]$AllowInsecure,
    [switch]$Bare,
    [switch]$Help
)

if ($Help) {
    Write-Host "Banyan Debug Startup Script"
    Write-Host ""
    Write-Host "Usage: .\start-debug.ps1 [options]"
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  -ConfigFile PATH    Config file to use (optional)"
    Write-Host "  -Listen ADDR        Listen address (default: /ip4/127.0.0.1/tcp/0)"
    Write-Host "  -PrivKey PATH       Private key file (generates ephemeral if not set)"
    Write-Host "  -ServicesDir PATH   Services directory (default: ../debug/win/services)"
    Write-Host "  -AddonsDir PATH     Addons directory (default: ../debug/win/addons)"
    Write-Host "  -FigsDir PATH       Figs directory (default: ../debug/win/figs)"
    Write-Host "  -NoCrypto           Disable encryption for testing"
    Write-Host "  -DisableDht         Disable DHT discovery"
    Write-Host "  -AllowExpired       Allow expired figs for testing"
    Write-Host "  -AllowInsecure      Allow insecure figs for testing"
    Write-Host "  -Bare               Run with no arguments (use banyan defaults)"
    Write-Host "  -Help               Show this help message"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  .\start-debug.ps1"
    Write-Host "  .\start-debug.ps1 -NoCrypto -DisableDht"
    Write-Host "  .\start-debug.ps1 -ConfigFile ./my-config.json"
    Write-Host "  .\start-debug.ps1 -Listen '/ip4/0.0.0.0/tcp/9999'"
    Write-Host "  .\start-debug.ps1 -Bare"
    exit 0
}

# Get the directory where this script is located
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

# Path to the banyan executable
$BanyanExec = Join-Path $ScriptDir "banyan.exe"

# Debug directories (relative to script location)
$DebugDir = Join-Path (Split-Path -Parent $ScriptDir) "debug\win"
$DefaultServicesDir = Join-Path $DebugDir "services"
$DefaultAddonsDir = Join-Path $DebugDir "addons"
$DefaultFigsDir = Join-Path $DebugDir "figs"

# Check if banyan executable exists
if (-not (Test-Path $BanyanExec)) {
    Write-Host "Error: banyan.exe not found at $BanyanExec" -ForegroundColor Red
    Write-Host "Make sure you're running this script from the build output directory." -ForegroundColor Yellow
    exit 1
}

# If bare mode, just run with no arguments
if ($Bare) {
    Write-Host "=== Banyan Node (Bare Mode) ===" -ForegroundColor Cyan
    Write-Host "Executable: $BanyanExec" -ForegroundColor Gray
    Write-Host "===============================" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "Starting Banyan node..." -ForegroundColor Green
    & $BanyanExec
    exit $LASTEXITCODE
}

# Build arguments
$arguments = @()

# Config file
if ($ConfigFile) {
    $arguments += "-config=`"$ConfigFile`""
}

# Listen address
$arguments += "-listen=`"$Listen`""

# Private key
if ($PrivKey) {
    $arguments += "-privkey=`"$PrivKey`""
}

# Services directory (use default debug dir if not specified)
$svcDir = if ($ServicesDir) { $ServicesDir } else { $DefaultServicesDir }
$arguments += "-services-dir=`"$svcDir`""

# Addons directory (use default debug dir if not specified)
$addDir = if ($AddonsDir) { $AddonsDir } else { $DefaultAddonsDir }
$arguments += "-addons-dir=`"$addDir`""

# Figs directory (use default debug dir if not specified)
$figDir = if ($FigsDir) { $FigsDir } else { $DefaultFigsDir }
$arguments += "-figs-dir=`"$figDir`""

# Debug/test flags
if ($NoCrypto) {
    $arguments += "-no-crypto"
}

if ($DisableDht) {
    $arguments += "-disable-dht"
}

if ($AllowExpired) {
    $arguments += "-allow-expired-figs"
}

if ($AllowInsecure) {
    $arguments += "-allow-insecure-figs"
}

# Display startup info
Write-Host "=== Banyan Node Debug Mode ===" -ForegroundColor Cyan
Write-Host "Executable:   $BanyanExec" -ForegroundColor Gray
Write-Host "Services Dir: $svcDir" -ForegroundColor Gray
Write-Host "Addons Dir:   $addDir" -ForegroundColor Gray
Write-Host "Figs Dir:     $figDir" -ForegroundColor Gray
Write-Host "Arguments:    $($arguments -join ' ')" -ForegroundColor Gray
Write-Host "==============================" -ForegroundColor Cyan
Write-Host ""

# Start the node
Write-Host "Starting Banyan node..." -ForegroundColor Green
& $BanyanExec $arguments


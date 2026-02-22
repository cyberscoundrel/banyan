# Banyan Node - Remote Testing Launcher (Portable)
#
# Expects the banyan.exe binary in the same directory as this script.
# Config resolution:
#   1. remote-testing.json next to this script (flash drive / flat dir)
#   2. ..\..\configs\remote-testing.json (full deploy\ folder structure)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition

# --- Locate binary ---
$Binary = Join-Path $ScriptDir "banyan.exe"
if (-not (Test-Path $Binary)) {
    Write-Host "ERROR: banyan.exe not found at $Binary" -ForegroundColor Red
    Write-Host "Place the Windows banyan.exe binary in the same directory as this script."
    exit 1
}

# --- Locate config ---
$Config = ""
$LocalConfig = Join-Path $ScriptDir "remote-testing.json"
$FallbackConfig = Join-Path $ScriptDir "..\..\configs\remote-testing.json"

if (Test-Path $LocalConfig) {
    $Config = (Resolve-Path $LocalConfig).Path
} elseif (Test-Path $FallbackConfig) {
    $Config = (Resolve-Path $FallbackConfig).Path
} else {
    Write-Host "ERROR: remote-testing.json not found." -ForegroundColor Red
    Write-Host "Looked in:"
    Write-Host "  $LocalConfig"
    Write-Host "  $FallbackConfig"
    Write-Host ""
    Write-Host "Place remote-testing.json next to this script or ensure the"
    Write-Host "deploy\configs\ directory is intact."
    exit 1
}

# --- Display startup info ---
Write-Host "========================================"
Write-Host "  Banyan Node - Remote Testing"
Write-Host "========================================"
Write-Host ""
Write-Host "  Binary:  $Binary"
Write-Host "  Config:  $Config"
Write-Host ""
Write-Host "  NAT Traversal:  enabled"
Write-Host "  Listen Address:  /ip4/0.0.0.0/tcp/9000"
Write-Host ""
Write-Host "========================================"
Write-Host ""

& $Binary -config="$Config"

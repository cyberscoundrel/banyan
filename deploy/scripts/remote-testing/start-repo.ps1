# Banyan Node - Remote Testing Launcher (Repo)
#
# Uses the node binary from the repo build output and the config from
# deploy\configs\. Designed for developers who have cloned and built the repo.

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$RepoRoot = (Resolve-Path (Join-Path $ScriptDir "..\..\..")).Path

# --- Locate binary ---
$Binary = Join-Path $RepoRoot "dist\apps\node\win\banyan.exe"

if (-not (Test-Path $Binary)) {
    Write-Host "ERROR: Node binary not found at $Binary" -ForegroundColor Red
    Write-Host ""
    Write-Host "Build it first from the repo root:"
    Write-Host "  cd $RepoRoot\apps\node"
    Write-Host "  make windows"
    Write-Host ""
    Write-Host "Or use the build script:"
    Write-Host "  .\build.ps1"
    exit 1
}

# --- Locate config ---
$Config = Join-Path $RepoRoot "deploy\configs\remote-testing.json"

if (-not (Test-Path $Config)) {
    Write-Host "ERROR: Config not found at $Config" -ForegroundColor Red
    Write-Host "The deploy\configs\ directory may be missing or incomplete."
    exit 1
}

# --- Display startup info ---
Write-Host "========================================"
Write-Host "  Banyan Node - Remote Testing (Repo)"
Write-Host "========================================"
Write-Host ""
Write-Host "  Binary:     $Binary"
Write-Host "  Config:     $Config"
Write-Host "  Repo Root:  $RepoRoot"
Write-Host ""
Write-Host "  NAT Traversal:  enabled"
Write-Host "  Listen Address:  /ip4/0.0.0.0/tcp/9000"
Write-Host ""
Write-Host "========================================"
Write-Host ""

& $Binary -config="$Config"

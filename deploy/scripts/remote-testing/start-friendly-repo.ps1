# Banyan Node - Remote Testing (User-Friendly, Repo)
#
# Hides raw node output and displays only key status information.
# Uses the node binary from the repo build output and the config from
# deploy\configs\. Designed for developers who have cloned and built the repo.

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$RepoRoot = (Resolve-Path (Join-Path $ScriptDir "..\..\..")).Path
$LogFile = Join-Path $ScriptDir "banyan-node.log"

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

Write-Host "========================================"
Write-Host "  Banyan Node - Starting..."
Write-Host "========================================"
Write-Host ""

# Launch node in background, redirect output to log file
$NodeProcess = Start-Process -FilePath $Binary -ArgumentList "-config=`"$Config`"" `
    -RedirectStandardOutput $LogFile -RedirectStandardError (Join-Path $ScriptDir "banyan-node-err.log") `
    -PassThru -WindowStyle Hidden

$NodePID = $NodeProcess.Id

# Ensure cleanup on exit
$CleanupBlock = {
    if ($NodeProcess -and -not $NodeProcess.HasExited) {
        Stop-Process -Id $NodeProcess.Id -Force -ErrorAction SilentlyContinue
    }
}
Register-EngineEvent PowerShell.Exiting -Action $CleanupBlock | Out-Null

try {
    # Wait for management API URL and Peer ID from log output
    $ApiUrl = ""
    $PeerId = ""
    $WaitTimeout = 60
    $Waited = 0

    while ([string]::IsNullOrEmpty($ApiUrl) -or [string]::IsNullOrEmpty($PeerId)) {
        if ($NodeProcess.HasExited) {
            Write-Host "ERROR: Node process exited unexpectedly." -ForegroundColor Red
            Write-Host "Check the log file for details: $LogFile"
            if (Test-Path $LogFile) {
                Get-Content $LogFile -Tail 20
            }
            exit 1
        }

        if (Test-Path $LogFile) {
            $LogContent = Get-Content $LogFile -Raw -ErrorAction SilentlyContinue
            if ($LogContent) {
                if ([string]::IsNullOrEmpty($PeerId)) {
                    if ($LogContent -match "Libp2p node started with Peer ID: (\S+)") {
                        $PeerId = $Matches[1]
                    }
                }
                if ([string]::IsNullOrEmpty($ApiUrl)) {
                    if ($LogContent -match "Management API server available at: (\S+)") {
                        $ApiUrl = $Matches[1]
                    }
                }
            }
        }

        $Waited++
        if ($Waited -ge $WaitTimeout) {
            Write-Host "ERROR: Timed out waiting for node to start." -ForegroundColor Red
            Write-Host "Check the log file: $LogFile"
            exit 1
        }
        Start-Sleep -Seconds 1
    }

    # Poll and display loop
    while ($true) {
        $DhtDisplay = "bootstrapping..."
        $NatDisplay = "detecting..."
        $RelayDisplay = "searching for relays..."
        $ConnectedPeers = "0"

        try {
            $Status = Invoke-RestMethod -Uri "$ApiUrl/node/status" -TimeoutSec 3 -ErrorAction SilentlyContinue
            if ($Status) {
                $ConnectedPeers = $Status.total_connected_peers

                if ($Status.dht_status.ready -eq $true) {
                    $DhtDisplay = "ready ($($Status.dht_status.routing_size) peers in routing table)"
                }

                switch ($Status.nat_status.reachability) {
                    "Public"  { $NatDisplay = "public (directly reachable)" }
                    "Private" { $NatDisplay = "private (behind NAT)" }
                    default   { $NatDisplay = "detecting..." }
                }

                if ($Status.nat_status.relay_addr -eq $true) {
                    $RelayDisplay = "connected (relay address acquired)"
                }
            }
        } catch {}

        Clear-Host
        Write-Host "========================================"
        Write-Host "  Banyan Node - Remote Testing"
        Write-Host "========================================"
        Write-Host ""
        Write-Host "  Peer ID:  $PeerId"
        Write-Host ""
        Write-Host "  Peers:    $ConnectedPeers connected"
        Write-Host "  DHT:      $DhtDisplay"
        Write-Host "  NAT:      $NatDisplay"
        Write-Host "  Relay:    $RelayDisplay"
        Write-Host ""
        Write-Host "  Press Ctrl+C to stop."
        Write-Host "========================================"

        Start-Sleep -Seconds 3
    }
} finally {
    & $CleanupBlock
    Write-Host ""
    Write-Host "Node stopped."
}

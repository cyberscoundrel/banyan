param(
    [Parameter(Mandatory=$true, Position=0)]
    [string]$PeerId,

    [switch]$Help
)

# Banyan - NAT Connectivity Test Utility
#
# Starts a local node, then repeatedly attempts to connect to a target peer
# and verify its identity via the management API proxy. Works for both
# NAT-traversed and port-forwarded/DHT-announced nodes.
#
# Usage: .\nat-test.ps1 -PeerId <target-peer-id>

if ($Help) {
    Write-Host "Usage: .\nat-test.ps1 -PeerId <target-peer-id>"
    Write-Host ""
    Write-Host "Starts a local Banyan node and continuously attempts to connect"
    Write-Host "to the given peer ID, verifying its identity via libp2p proxy."
    exit 0
}

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$RepoRoot = (Resolve-Path (Join-Path $ScriptDir "..")).Path
$LogFile = Join-Path $ScriptDir "nat-test-node.log"
$ErrLogFile = Join-Path $ScriptDir "nat-test-node-err.log"
$TargetPeerId = $PeerId

# --- Locate binary ---
$Binary = Join-Path $RepoRoot "dist\apps\node\win\banyan.exe"

if (-not (Test-Path $Binary)) {
    Write-Host "ERROR: Node binary not found at $Binary" -ForegroundColor Red
    Write-Host "Build it first: cd $RepoRoot\apps\node && make windows"
    exit 1
}

# --- Locate config ---
$Config = Join-Path $RepoRoot "deploy\configs\remote-testing.json"

if (-not (Test-Path $Config)) {
    Write-Host "ERROR: Config not found at $Config" -ForegroundColor Red
    exit 1
}

Write-Host "========================================"
Write-Host "  Banyan NAT Connectivity Test"
Write-Host "  Starting local node..."
Write-Host "========================================"
Write-Host ""

# Launch node in background
$NodeProcess = Start-Process -FilePath $Binary -ArgumentList "-config=`"$Config`"" `
    -RedirectStandardOutput $LogFile -RedirectStandardError $ErrLogFile `
    -PassThru -WindowStyle Hidden

# Ensure cleanup on exit
$CleanupBlock = {
    if ($NodeProcess -and -not $NodeProcess.HasExited) {
        Stop-Process -Id $NodeProcess.Id -Force -ErrorAction SilentlyContinue
    }
}
Register-EngineEvent PowerShell.Exiting -Action $CleanupBlock | Out-Null

try {
    # Wait for management API URL and Peer ID
    $ApiUrl = ""
    $LocalPeerId = ""
    $Waited = 0

    while ([string]::IsNullOrEmpty($ApiUrl) -or [string]::IsNullOrEmpty($LocalPeerId)) {
        if ($NodeProcess.HasExited) {
            Write-Host "ERROR: Node process exited unexpectedly." -ForegroundColor Red
            if (Test-Path $LogFile) { Get-Content $LogFile -Tail 20 }
            exit 1
        }

        if (Test-Path $LogFile) {
            $LogContent = Get-Content $LogFile -Raw -ErrorAction SilentlyContinue
            if ($LogContent) {
                if ([string]::IsNullOrEmpty($LocalPeerId) -and $LogContent -match "Libp2p node started with Peer ID: (\S+)") {
                    $LocalPeerId = $Matches[1]
                }
                if ([string]::IsNullOrEmpty($ApiUrl) -and $LogContent -match "Management API server available at: (\S+)") {
                    $ApiUrl = $Matches[1]
                }
            }
        }

        $Waited++
        if ($Waited -ge 60) {
            Write-Host "ERROR: Timed out waiting for node to start." -ForegroundColor Red
            exit 1
        }
        Start-Sleep -Seconds 1
    }

    # Wait for DHT readiness
    Write-Host "  Waiting for DHT to bootstrap..."
    $DhtWait = 0
    while ($true) {
        try {
            $Status = Invoke-RestMethod -Uri "$ApiUrl/node/status" -TimeoutSec 3 -ErrorAction SilentlyContinue
            if ($Status -and $Status.dht_status.ready -eq $true) {
                break
            }
        } catch {}
        $DhtWait++
        if ($DhtWait -ge 120) {
            Write-Host "WARNING: DHT not ready after 120s, proceeding anyway." -ForegroundColor Yellow
            break
        }
        Start-Sleep -Seconds 1
    }

    # Main test loop
    $TryCount = 0
    $Connected = $false
    $Verified = $false

    while ($true) {
        $TryCount++

        # Fetch local node status
        $DhtDisplay = "bootstrapping..."
        $ConnectedPeers = "0"
        try {
            $Status = Invoke-RestMethod -Uri "$ApiUrl/node/status" -TimeoutSec 3 -ErrorAction SilentlyContinue
            if ($Status) {
                $ConnectedPeers = $Status.total_connected_peers
                if ($Status.dht_status.ready -eq $true) {
                    $DhtDisplay = "ready ($($Status.dht_status.routing_size) peers)"
                }
            }
        } catch {}

        # Step A: Attempt connection
        try {
            Invoke-RestMethod -Uri "$ApiUrl/network/connect/$TargetPeerId" -Method Post -TimeoutSec 10 -ErrorAction SilentlyContinue | Out-Null
        } catch {}

        # Step B: Check connection status
        $ConnDisplay = "attempting... (try #$TryCount)"
        $TargetConnected = $false
        try {
            $Conns = Invoke-RestMethod -Uri "$ApiUrl/network/connections" -TimeoutSec 3 -ErrorAction SilentlyContinue
            if ($Conns -and $Conns.connections) {
                $TargetConn = $Conns.connections | Where-Object { $_.peer_id -eq $TargetPeerId }
                if ($TargetConn -and $TargetConn.libp2p_connected -eq $true) {
                    $Connected = $true
                    $TargetConnected = $true
                    $ConnDisplay = "connected"
                }
            }
        } catch {}

        # Step C: If connected, verify identity via proxy
        $VerifyDisplay = "no"
        if ($Connected) {
            try {
                $RemoteStatus = Invoke-RestMethod -Uri "$ApiUrl/proxy/peer/$TargetPeerId/status" -TimeoutSec 10 -ErrorAction SilentlyContinue
                if ($RemoteStatus) {
                    if ($RemoteStatus.peer_id -eq $TargetPeerId) {
                        $Verified = $true
                        $VerifyDisplay = "YES - identity confirmed"
                    } else {
                        $VerifyDisplay = "MISMATCH - got $($RemoteStatus.peer_id)"
                    }
                } else {
                    $VerifyDisplay = "connected, proxy pending..."
                }
            } catch {
                $VerifyDisplay = "connected, proxy pending..."
            }
        }

        # Display
        Clear-Host
        Write-Host "========================================"
        Write-Host "  Banyan NAT Connectivity Test"
        Write-Host "========================================"
        Write-Host ""
        Write-Host "  Local Peer ID:   $LocalPeerId"
        Write-Host "  Target Peer ID:  $TargetPeerId"
        Write-Host ""
        Write-Host "  Local Peers:  $ConnectedPeers connected"
        Write-Host "  DHT:          $DhtDisplay"
        Write-Host "  Connection:   $ConnDisplay"
        Write-Host "  Verified:     $VerifyDisplay"
        Write-Host ""
        Write-Host "  Press Ctrl+C to stop."
        Write-Host "========================================"

        # Reset if connection dropped
        if ($Connected -and -not $TargetConnected) {
            $Connected = $false
            $Verified = $false
        }

        Start-Sleep -Seconds 5
    }
} finally {
    & $CleanupBlock
    Write-Host ""
    Write-Host "Test stopped. Node shut down."
}

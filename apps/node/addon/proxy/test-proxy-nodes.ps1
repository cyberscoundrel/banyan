# Integration test script for HTTP proxy addon between two Banyan nodes
# This script tests the proxy addon's ability to route traffic via .peer addresses
param(
    [switch]$NoCleanup
)

$ErrorActionPreference = 'Stop'

# Configuration - paths relative to workspace root
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$WorkspaceRoot = (Resolve-Path "$ScriptDir\..\..\..\..").Path
$DistDir = "$WorkspaceRoot\dist\apps\node\win"
$Node1Dir = "$WorkspaceRoot\test-node1"
$Node2Dir = "$WorkspaceRoot\test-node2"
$TestServerPort = 9999
$CleanupOnExit = -not $NoCleanup

# Track PIDs for cleanup
$script:Node1Process = $null
$script:Node2Process = $null
$script:TestServerProcess = $null

function Write-Color {
    param([string]$Message, [string]$Color = "White")
    Write-Host $Message -ForegroundColor $Color
}

function Cleanup {
    Write-Color "Cleaning up..." Yellow

    if ($script:Node1Process -and !$script:Node1Process.HasExited) {
        Stop-Process -Id $script:Node1Process.Id -Force -ErrorAction SilentlyContinue
    }
    if ($script:Node2Process -and !$script:Node2Process.HasExited) {
        Stop-Process -Id $script:Node2Process.Id -Force -ErrorAction SilentlyContinue
    }
    if ($script:TestServerProcess -and !$script:TestServerProcess.HasExited) {
        Stop-Process -Id $script:TestServerProcess.Id -Force -ErrorAction SilentlyContinue
    }

    if ($CleanupOnExit) {
        Remove-Item -Path $Node1Dir -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -Path $Node2Dir -Recurse -Force -ErrorAction SilentlyContinue
    }

    Write-Color "Cleanup complete" Green
}

# Register cleanup on exit
$null = Register-EngineEvent -SourceIdentifier PowerShell.Exiting -Action { Cleanup }
trap { Cleanup; break }

Write-Color "=== Banyan HTTP Proxy Integration Test ===" Blue

# Set executable path for Windows
$BanyanExec = "$DistDir\banyan.exe"

if (-not (Test-Path $BanyanExec)) {
    Write-Color "Error: banyan executable not found at $BanyanExec" Red
    Write-Host "Please build the node first: nx build node -c windows"
    exit 1
}

# Create test directories
New-Item -ItemType Directory -Force -Path "$Node1Dir\addons" | Out-Null
New-Item -ItemType Directory -Force -Path "$Node2Dir\addons" | Out-Null

# Build proxy addon
Write-Color "Building proxy addon..." Yellow
Push-Location $WorkspaceRoot
go build -o "$Node1Dir\addons\proxy-addon.exe" ./apps/node/addon/proxy
Copy-Item "$Node1Dir\addons\proxy-addon.exe" "$Node2Dir\addons\proxy-addon.exe"
Pop-Location

# Define proxy ports for each node
$Node1ProxyPort = 18080
$Node2ProxyPort = 18081

$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)

# Create addons.json for Node 1
$AddonsJson1 = @{
    addons = @(
        @{ name = "proxy-addon"; exec = "./proxy-addon.exe"; args = @("--port", "$Node1ProxyPort") }
    )
} | ConvertTo-Json -Depth 4
[System.IO.File]::WriteAllText("$Node1Dir\addons\addons.json", $AddonsJson1, $Utf8NoBom)

# Create addons.json for Node 2
$AddonsJson2 = @{
    addons = @(
        @{ name = "proxy-addon"; exec = "./proxy-addon.exe"; args = @("--port", "$Node2ProxyPort") }
    )
} | ConvertTo-Json -Depth 4
[System.IO.File]::WriteAllText("$Node2Dir\addons\addons.json", $AddonsJson2, $Utf8NoBom)

# Start simple test HTTP server
Write-Color "Starting test HTTP server on port $TestServerPort..." Yellow
$TestServerScript = @"
import http.server
import socketserver

class TestHandler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/plain')
        self.end_headers()
        self.wfile.write(b'Hello from test server!')
    def log_message(self, format, *args):
        pass

with socketserver.TCPServer(('', $TestServerPort), TestHandler) as httpd:
    httpd.serve_forever()
"@

$script:TestServerProcess = Start-Process -FilePath "python" -ArgumentList "-c", "`"$TestServerScript`"" -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 2

# Start Node 1
Write-Color "Starting Node 1 (proxy on port $Node1ProxyPort)..." Yellow
$script:Node1Process = Start-Process -FilePath $BanyanExec `
    -ArgumentList "-listen=`"/ip4/127.0.0.1/tcp/0`"", "-addons-dir=`"./addons`"" `
    -WorkingDirectory $Node1Dir -PassThru `
    -RedirectStandardOutput "$Node1Dir\node1.log" `
    -RedirectStandardError "$Node1Dir\node1-err.log" -WindowStyle Hidden

# Start Node 2
Write-Color "Starting Node 2 (proxy on port $Node2ProxyPort)..." Yellow
$script:Node2Process = Start-Process -FilePath $BanyanExec `
    -ArgumentList "-listen=`"/ip4/127.0.0.1/tcp/0`"", "-addons-dir=`"./addons`"" `
    -WorkingDirectory $Node2Dir -PassThru `
    -RedirectStandardOutput "$Node2Dir\node2.log" `
    -RedirectStandardError "$Node2Dir\node2-err.log" -WindowStyle Hidden

# Wait for nodes to start
Write-Color "Waiting for nodes to initialize..." Yellow
Start-Sleep -Seconds 15

# Extract node information from logs
$Node1Log = (Get-Content "$Node1Dir\node1.log" -Raw -ErrorAction SilentlyContinue) +
            (Get-Content "$Node1Dir\node1-err.log" -Raw -ErrorAction SilentlyContinue)
$Node2Log = (Get-Content "$Node2Dir\node2.log" -Raw -ErrorAction SilentlyContinue) +
            (Get-Content "$Node2Dir\node2-err.log" -Raw -ErrorAction SilentlyContinue)

if ($Node1Log -match "Management API server available at[:\s]+http://(localhost|127\.0\.0\.1):(\d+)") {
    $Node1MgmtPort = $Matches[2]
} else {
    Write-Color "Failed to extract Node 1 management port" Red
    Get-Content "$Node1Dir\node1.log" -ErrorAction SilentlyContinue
    Get-Content "$Node1Dir\node1-err.log" -ErrorAction SilentlyContinue
    Cleanup
    exit 1
}

if ($Node2Log -match "Management API server available at[:\s]+http://(localhost|127\.0\.0\.1):(\d+)") {
    $Node2MgmtPort = $Matches[2]
} else {
    Write-Color "Failed to extract Node 2 management port" Red
    Get-Content "$Node2Dir\node2.log" -ErrorAction SilentlyContinue
    Get-Content "$Node2Dir\node2-err.log" -ErrorAction SilentlyContinue
    Cleanup
    exit 1
}

Write-Color "Node 1 management port: $Node1MgmtPort" Green
Write-Color "Node 2 management port: $Node2MgmtPort" Green

# Get node IDs
try {
    $Node1Status = Invoke-RestMethod -Uri "http://127.0.0.1:$Node1MgmtPort/node/status" -TimeoutSec 10
    $Node1Id = $Node1Status.node_id

    $Node2Status = Invoke-RestMethod -Uri "http://127.0.0.1:$Node2MgmtPort/node/status" -TimeoutSec 10
    $Node2Id = $Node2Status.node_id
} catch {
    Write-Color "Failed to get node IDs: $_" Red
    Cleanup
    exit 1
}

Write-Color "Node 1 ID: $Node1Id" Green
Write-Color "Node 2 ID: $Node2Id" Green

# Connect nodes
Write-Color "Connecting Node 1 to Node 2..." Yellow
try {
    Invoke-RestMethod -Uri "http://127.0.0.1:$Node1MgmtPort/network/connect/$Node2Id" -Method Post -TimeoutSec 10 | Out-Null
} catch {
    Write-Color "Warning: Connect request returned error (may still work): $_" Yellow
}
Start-Sleep -Seconds 3

$TestPass = 0
$TestFail = 0

# Test 1: Proxy addon status endpoint
Write-Color "Test 1 - Proxy addon status" Blue
try {
    $Response = Invoke-RestMethod -Uri "http://127.0.0.1:$Node1MgmtPort/addons/proxy/status" -TimeoutSec 10
    if ($Response.status -eq "running") {
        Write-Color "[PASS] Proxy addon is running on port $($Response.listen)" Green
        $TestPass++
    } else {
        Write-Color "[FAIL] Proxy addon status unexpected" Red
        $TestFail++
    }
} catch {
    Write-Color "[FAIL] Proxy addon status check failed - $($_.Exception.Message)" Red
    $TestFail++
}

# Test 2: Direct HTTP through proxy
Write-Color "Test 2 - Direct HTTP through proxy" Blue
try {
    $Response = Invoke-WebRequest -Uri "http://127.0.0.1:$TestServerPort/" -Proxy "http://127.0.0.1:$Node1ProxyPort" -TimeoutSec 10 -UseBasicParsing
    if ($Response.Content -eq "Hello from test server!") {
        Write-Color "[PASS] Direct HTTP proxy access successful" Green
        $TestPass++
    } else {
        Write-Color "[FAIL] Direct HTTP proxy - unexpected response" Red
        $TestFail++
    }
} catch {
    Write-Color "[FAIL] Direct HTTP proxy access failed - $($_.Exception.Message)" Red
    $TestFail++
}

# Test 3: Proxy via management API to peer
Write-Color "Test 3 - Proxy via management API to peer" Blue
try {
    $Response = Invoke-RestMethod -Uri "http://127.0.0.1:$Node1MgmtPort/proxy/peer/$Node2Id/ping" -TimeoutSec 10
    if ($Response.status -eq "ok") {
        Write-Color "[PASS] P2P proxy to peer successful - $($Response.peer_id)" Green
        $TestPass++
    } else {
        Write-Color "[FAIL] P2P proxy response unexpected" Red
        $TestFail++
    }
} catch {
    Write-Color "[FAIL] P2P proxy to peer failed - $($_.Exception.Message)" Red
    $TestFail++
}

Write-Host ""
Write-Color "=== Test Summary ===" Blue
Write-Color "Passed: $TestPass" Green
Write-Color "Failed: $TestFail" Red

if ($TestFail -gt 0) {
    Write-Color "Check logs for details:" Yellow
    Write-Host "  Node 1: $Node1Dir\node1.log"
    Write-Host "  Node 2: $Node2Dir\node2.log"
    Cleanup
    exit 1
}

Write-Color "All tests passed!" Green

# Keep running if -NoCleanup specified
if ($NoCleanup) {
    Write-Color "Nodes are still running. Press Ctrl+C to stop." Yellow
    Write-Host "Node 1 Management: http://127.0.0.1:$Node1MgmtPort"
    Write-Host "Node 2 Management: http://127.0.0.1:$Node2MgmtPort"
    Write-Host "Proxy: http://127.0.0.1:$Node1ProxyPort"
    while ($true) { Start-Sleep -Seconds 60 }
}

Cleanup
exit 0


# Diagnostic script to check service discovery
# Usage: .\diagnose-discovery.ps1 <node1_port> <node2_port>

param(
    [int]$Node1Port = 8080,
    [int]$Node2Port = 8081
)

Write-Host "=== Service Discovery Diagnostic ===" -ForegroundColor Cyan
Write-Host ""

# Check Node 1 (Beacon Node)
Write-Host "Node 1 (Port $Node1Port) - Beacon Node:" -ForegroundColor Yellow
Write-Host "Checking beacons..."
try {
    $beacons = Invoke-RestMethod -Uri "http://localhost:$Node1Port/services/beacons" -Method Get
    Write-Host "  Beacons running: $($beacons.beacons.Count)" -ForegroundColor Green
    foreach ($beacon in $beacons.beacons) {
        Write-Host "    - Alias: $($beacon.alias)" -ForegroundColor Gray
        Write-Host "      Service Key: $($beacon.service_key.Substring(0, 16))..." -ForegroundColor Gray
        Write-Host "      Mode: $($beacon.mode)" -ForegroundColor Gray
    }
} catch {
    Write-Host "  ERROR: Could not fetch beacons - $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""
Write-Host "Checking connections..."
try {
    $conns = Invoke-RestMethod -Uri "http://localhost:$Node1Port/peers/connections" -Method Get
    Write-Host "  Connected peers: $($conns.connections.Count)" -ForegroundColor Green
    foreach ($conn in $conns.connections) {
        Write-Host "    - Peer: $($conn.peer_id.Substring(0, 20))..." -ForegroundColor Gray
        Write-Host "      Service Keys: $($conn.service_keys.Count)" -ForegroundColor Gray
    }
} catch {
    Write-Host "  ERROR: Could not fetch connections - $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Check Node 2 (Locator Node)
Write-Host "Node 2 (Port $Node2Port) - Locator Node:" -ForegroundColor Yellow
Write-Host "Checking locators..."
try {
    $locators = Invoke-RestMethod -Uri "http://localhost:$Node2Port/services/locators" -Method Get
    Write-Host "  Locators running: $($locators.locators.Count)" -ForegroundColor Green
    foreach ($locator in $locators.locators) {
        Write-Host "    - Service Key: $($locator.service_key.Substring(0, 16))..." -ForegroundColor Gray
        Write-Host "      Peers found: $($locator.peer_ids.Count)" -ForegroundColor Gray
        if ($locator.peer_ids.Count -eq 0) {
            Write-Host "      WARNING: No peers found for this key!" -ForegroundColor Red
        }
    }
} catch {
    Write-Host "  ERROR: Could not fetch locators - $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""
Write-Host "Checking connections..."
try {
    $conns = Invoke-RestMethod -Uri "http://localhost:$Node2Port/peers/connections" -Method Get
    Write-Host "  Connected peers: $($conns.connections.Count)" -ForegroundColor Green
    $peersWithKeys = 0
    foreach ($conn in $conns.connections) {
        if ($conn.service_keys.Count -gt 0) {
            $peersWithKeys++
            Write-Host "    - Peer: $($conn.peer_id.Substring(0, 20))..." -ForegroundColor Gray
            Write-Host "      Service Keys: $($conn.service_keys.Count)" -ForegroundColor Gray
            foreach ($key in $conn.service_keys) {
                $keyHex = [System.BitConverter]::ToString($key).Replace("-", "").ToLower()
                Write-Host "        * $($keyHex.Substring(0, 16))..." -ForegroundColor Gray
            }
        }
    }
    if ($peersWithKeys -eq 0) {
        Write-Host "  WARNING: No peers have service keys associated!" -ForegroundColor Red
    }
} catch {
    Write-Host "  ERROR: Could not fetch connections - $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Summary
Write-Host "Summary:" -ForegroundColor Cyan
Write-Host "1. Check if beacons are running on Node 1" -ForegroundColor White
Write-Host "2. Check if locators are running on Node 2" -ForegroundColor White
Write-Host "3. Check if nodes are connected to each other (libp2p)" -ForegroundColor White
Write-Host "4. Check if Node 2 has associated service keys with Node 1's peer ID" -ForegroundColor White
Write-Host ""
Write-Host "If locators are running but no peers are found:" -ForegroundColor Yellow
Write-Host "  - Check event logs for 'Received service announcement' events" -ForegroundColor Gray
Write-Host "  - Check for signature verification errors" -ForegroundColor Gray
Write-Host "  - Verify both nodes are using the same service keys" -ForegroundColor Gray
Write-Host "  - Check if beacons are including peer IDs in announcements" -ForegroundColor Gray


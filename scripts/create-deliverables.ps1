# Create compressed deliverables containing node executable + CLI for each platform
param(
    [string]$OutputDir = "dist/deliverables",
    [string]$NodeDir = "dist/apps/node",
    [string]$CliDir = "dist/apps/cli"
)

$ErrorActionPreference = "Stop"

# Convert to absolute paths
$OutputDir = [System.IO.Path]::GetFullPath($OutputDir)
$NodeDir = [System.IO.Path]::GetFullPath($NodeDir)
$CliDir = [System.IO.Path]::GetFullPath($CliDir)

Write-Host ""
Write-Host "Creating Banyan deliverables..." -ForegroundColor Cyan
Write-Host "   Output: $OutputDir"
Write-Host "   Node builds: $NodeDir"
Write-Host "   CLI builds: $CliDir"
Write-Host ""

# Clean and create output directory
if (Test-Path $OutputDir) {
    Remove-Item -Recurse -Force $OutputDir
}
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

# Get version info
try {
    $VERSION = git describe --tags --always --dirty 2>$null
    if (-not $VERSION) { $VERSION = "dev" }
} catch {
    $VERSION = "dev"
}

try {
    $COMMIT = git rev-parse HEAD 2>$null
    if (-not $COMMIT) { $COMMIT = "unknown" }
} catch {
    $COMMIT = "unknown"
}

$BUILD_TIME = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

function Get-FormattedSize($bytes) {
    if ($bytes -gt 1MB) {
        return "{0:N1} MB" -f ($bytes / 1MB)
    } else {
        return "{0:N1} KB" -f ($bytes / 1KB)
    }
}

function Get-FileChecksum($path) {
    if (Test-Path $path) {
        return (Get-FileHash -Path $path -Algorithm SHA256).Hash.ToLower()
    }
    return "unknown"
}

function Create-ReadmeWindows($path, $ver, $buildTime) {
    $lines = @(
        "Banyan - LibP2P HTTP Proxy",
        "Version: $ver",
        "Build: $buildTime",
        "",
        "Contents:",
        "  banyan.exe     - Banyan Node (P2P network daemon)",
        "  banyan-cli.exe - Banyan CLI (Terminal UI)",
        "",
        "Quick Start:",
        "  Run banyan-cli.exe to start the terminal UI",
        "  Or run banyan.exe directly",
        "",
        "Website: https://banyan.cyberscoundrel.com"
    )
    $lines | Out-File -FilePath $path -Encoding UTF8
}

function Create-ReadmeUnix($path, $platform, $ver, $buildTime) {
    $lines = @(
        "Banyan - LibP2P HTTP Proxy ($platform)",
        "Version: $ver",
        "Build: $buildTime",
        "",
        "Contents:",
        "  banyan      - Banyan Node (P2P network daemon)",
        "  banyan-cli  - Banyan CLI (Terminal UI)",
        "",
        "Quick Start:",
        "  chmod +x banyan banyan-cli",
        "  ./banyan-cli",
        "",
        "Website: https://banyan.cyberscoundrel.com"
    )
    $lines | Out-File -FilePath $path -Encoding UTF8
}

# Create Windows deliverable (zip)
Write-Host "Creating Windows deliverable..." -ForegroundColor Yellow
$tempWin = Join-Path $env:TEMP "banyan-win-$(Get-Random)"
New-Item -ItemType Directory -Force -Path "$tempWin\banyan" | Out-Null
Copy-Item "$NodeDir\win\banyan.exe" "$tempWin\banyan\"
Copy-Item "$CliDir\win\banyan-cli.exe" "$tempWin\banyan\"
Create-ReadmeWindows "$tempWin\banyan\README.txt" $VERSION $BUILD_TIME

$winZipPath = Join-Path $OutputDir "banyan-windows-x64.zip"
Compress-Archive -Path "$tempWin\banyan" -DestinationPath $winZipPath -Force
Remove-Item -Recurse -Force $tempWin
$winSize = (Get-Item $winZipPath).Length
$winChecksum = Get-FileChecksum $winZipPath
Write-Host "Created: $winZipPath ($(Get-FormattedSize $winSize))" -ForegroundColor Green

# Create Linux deliverable (tar.gz)
Write-Host "Creating Linux deliverable..." -ForegroundColor Yellow
$tempLinux = Join-Path $env:TEMP "banyan-linux-$(Get-Random)"
New-Item -ItemType Directory -Force -Path "$tempLinux\banyan" | Out-Null
Copy-Item "$NodeDir\linux\banyan" "$tempLinux\banyan\"
Copy-Item "$CliDir\linux\banyan-cli" "$tempLinux\banyan\"
Create-ReadmeUnix "$tempLinux\banyan\README.md" "Linux" $VERSION $BUILD_TIME

$linuxTarPath = Join-Path $OutputDir "banyan-linux-x64.tar.gz"
Push-Location $tempLinux
tar -czf $linuxTarPath banyan
Pop-Location
Remove-Item -Recurse -Force $tempLinux
$linuxSize = (Get-Item $linuxTarPath).Length
$linuxChecksum = Get-FileChecksum $linuxTarPath
Write-Host "Created: $linuxTarPath ($(Get-FormattedSize $linuxSize))" -ForegroundColor Green

# Create macOS Intel deliverable (tar.gz)
Write-Host "Creating macOS Intel deliverable..." -ForegroundColor Yellow
$tempMacIntel = Join-Path $env:TEMP "banyan-mac-intel-$(Get-Random)"
New-Item -ItemType Directory -Force -Path "$tempMacIntel\banyan" | Out-Null
Copy-Item "$NodeDir\osx\banyan-amd64" "$tempMacIntel\banyan\banyan"
Copy-Item "$CliDir\osx\banyan-cli-amd64" "$tempMacIntel\banyan\banyan-cli"
Create-ReadmeUnix "$tempMacIntel\banyan\README.md" "macOS Intel" $VERSION $BUILD_TIME

$macIntelTarPath = Join-Path $OutputDir "banyan-macos-x64.tar.gz"
Push-Location $tempMacIntel
tar -czf $macIntelTarPath banyan
Pop-Location
Remove-Item -Recurse -Force $tempMacIntel
$macIntelSize = (Get-Item $macIntelTarPath).Length
$macIntelChecksum = Get-FileChecksum $macIntelTarPath
Write-Host "Created: $macIntelTarPath ($(Get-FormattedSize $macIntelSize))" -ForegroundColor Green

# Create macOS ARM deliverable (tar.gz)
Write-Host "Creating macOS ARM deliverable..." -ForegroundColor Yellow
$tempMacArm = Join-Path $env:TEMP "banyan-mac-arm-$(Get-Random)"
New-Item -ItemType Directory -Force -Path "$tempMacArm\banyan" | Out-Null
Copy-Item "$NodeDir\osx\banyan-arm64" "$tempMacArm\banyan\banyan"
Copy-Item "$CliDir\osx\banyan-cli-arm64" "$tempMacArm\banyan\banyan-cli"
Create-ReadmeUnix "$tempMacArm\banyan\README.md" "macOS ARM" $VERSION $BUILD_TIME

$macArmTarPath = Join-Path $OutputDir "banyan-macos-arm64.tar.gz"
Push-Location $tempMacArm
tar -czf $macArmTarPath banyan
Pop-Location
Remove-Item -Recurse -Force $tempMacArm
$macArmSize = (Get-Item $macArmTarPath).Length
$macArmChecksum = Get-FileChecksum $macArmTarPath
Write-Host "Created: $macArmTarPath ($(Get-FormattedSize $macArmSize))" -ForegroundColor Green

# Create release metadata
Write-Host ""
Write-Host "Creating release metadata..." -ForegroundColor Yellow

$metadata = @{
    version = $VERSION
    commit = $COMMIT
    buildDate = $BUILD_TIME
    buildTime = $BUILD_TIME
    whatsNew = "Bundled node and CLI deliverables"
    deliverables = @{
        windows_x64 = @{
            id = "windows_x64"
            platform = "Windows"
            arch = "x64"
            filename = "banyan-windows-x64.zip"
            size = Get-FormattedSize $winSize
            sizeBytes = $winSize
            checksum = "sha256:$winChecksum"
            contents = @("banyan.exe", "banyan-cli.exe", "README.txt")
        }
        linux_x64 = @{
            id = "linux_x64"
            platform = "Linux"
            arch = "x64"
            filename = "banyan-linux-x64.tar.gz"
            size = Get-FormattedSize $linuxSize
            sizeBytes = $linuxSize
            checksum = "sha256:$linuxChecksum"
            contents = @("banyan", "banyan-cli", "README.md")
        }
        macos_x64 = @{
            id = "macos_x64"
            platform = "macOS"
            arch = "x64 (Intel)"
            filename = "banyan-macos-x64.tar.gz"
            size = Get-FormattedSize $macIntelSize
            sizeBytes = $macIntelSize
            checksum = "sha256:$macIntelChecksum"
            contents = @("banyan", "banyan-cli", "README.md")
        }
        macos_arm64 = @{
            id = "macos_arm64"
            platform = "macOS"
            arch = "ARM64 (Apple Silicon)"
            filename = "banyan-macos-arm64.tar.gz"
            size = Get-FormattedSize $macArmSize
            sizeBytes = $macArmSize
            checksum = "sha256:$macArmChecksum"
            contents = @("banyan", "banyan-cli", "README.md")
        }
    }
}

$metadataPath = Join-Path $OutputDir "release-metadata.json"
$metadata | ConvertTo-Json -Depth 10 | Out-File -FilePath $metadataPath -Encoding UTF8

Write-Host "Created: $metadataPath" -ForegroundColor Green
Write-Host ""
Write-Host "All deliverables created successfully!" -ForegroundColor Cyan
Write-Host ""
Write-Host "Deliverables:" -ForegroundColor White
Write-Host "   banyan-windows-x64.zip ($(Get-FormattedSize $winSize))"
Write-Host "   banyan-linux-x64.tar.gz ($(Get-FormattedSize $linuxSize))"
Write-Host "   banyan-macos-x64.tar.gz ($(Get-FormattedSize $macIntelSize))"
Write-Host "   banyan-macos-arm64.tar.gz ($(Get-FormattedSize $macArmSize))"
Write-Host "   release-metadata.json"

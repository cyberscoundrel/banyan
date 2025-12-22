# Banyan Cross-Platform Build Script for PowerShell
# Builds Banyan for Windows, Linux, and macOS

param(
    [switch]$Clean,
    [string]$Version = "",
    [string]$WhatsNew = "",
    [string]$OutputDir = "bin",
    [switch]$MetadataOnly,
    [switch]$Help
)

if ($Help) {
    Write-Host "Usage: .\build.ps1 [-Clean] [-Version VERSION] [-WhatsNew DESCRIPTION] [-OutputDir DIR] [-MetadataOnly] [-Help]"
    Write-Host "  -Clean        Clean previous builds before building"
    Write-Host "  -Version      Version string in semver format (default: pre-release)"
    Write-Host "  -WhatsNew     Comma-separated list of new features (default: smiley emoji)"
    Write-Host "  -OutputDir    Output directory for builds (default: bin)"
    Write-Host "  -MetadataOnly Only regenerate release metadata (no compilation)"
    Write-Host "  -Help         Show this help message"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  .\build.ps1"
    Write-Host "  .\build.ps1 -Clean"
    Write-Host "  .\build.ps1 -OutputDir ../../dist/apps/node"
    Write-Host "  .\build.ps1 -MetadataOnly"
    Write-Host "  .\build.ps1 -Version ""1.0.0"" -WhatsNew ""New feature 1, Bug fix 2, Performance improvements"""
    exit 0
}

if ($MetadataOnly) {
    Write-Host "Regenerating release metadata only..." -ForegroundColor Green
} else {
    Write-Host "Building Banyan for all platforms..." -ForegroundColor Green
}
Write-Host ""

# Get version info
$Version = "dev"
try {
    $gitOutput = git describe --tags --always --dirty 2>$null
    if ($gitOutput) { $Version = $gitOutput.Trim() }
} catch {
    # Use default version
}

$BuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$LdFlags = "-s -w -X main.version=$Version -X main.buildTime=$BuildTime"

# Skip building if metadata only
if (-not $MetadataOnly) {
    # Clean if requested
    if ($Clean) {
        Write-Host "Cleaning previous builds..." -ForegroundColor Yellow
        Remove-Item -Path "$OutputDir\win\*", "$OutputDir\linux\*", "$OutputDir\osx\*" -Force -ErrorAction SilentlyContinue
    }

    # Create directories for build outputs
    New-Item -ItemType Directory -Path "$OutputDir\win", "$OutputDir\linux", "$OutputDir\osx" -Force | Out-Null

    # Create debug directories for each platform
    Write-Host "Creating debug folder structure..." -ForegroundColor Cyan
    $debugPlatforms = @("win", "linux", "osx")
    $debugSubdirs = @("addons", "figs", "services")
    foreach ($platform in $debugPlatforms) {
        foreach ($subdir in $debugSubdirs) {
            New-Item -ItemType Directory -Path "$OutputDir\debug\$platform\$subdir" -Force | Out-Null
        }
    }
    Write-Host "  Created debug/{win,linux,osx}/{addons,figs,services}" -ForegroundColor Gray
} else {
    # For metadata only, just ensure output directory exists
    New-Item -ItemType Directory -Path "$OutputDir" -Force | Out-Null
}

if (-not $MetadataOnly) {
    # Build Windows
    Write-Host "Building for Windows..." -ForegroundColor Cyan
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -ldflags $LdFlags -o "$OutputDir\win\banyan.exe" .
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Windows build complete!" -ForegroundColor Green
    } else {
        Write-Host "Windows build failed!" -ForegroundColor Red
        exit 1
    }

    # Build Linux
    Write-Host "Building for Linux..." -ForegroundColor Cyan
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -ldflags $LdFlags -o "$OutputDir\linux\banyan" .
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Linux build complete!" -ForegroundColor Green
    } else {
        Write-Host "Linux build failed!" -ForegroundColor Red
        exit 1
    }

    # Build macOS Intel
    Write-Host "Building for macOS (Intel)..." -ForegroundColor Cyan
    $env:GOOS = "darwin"
    $env:GOARCH = "amd64"
    go build -ldflags $LdFlags -o "$OutputDir\osx\banyan-amd64" .
    if ($LASTEXITCODE -eq 0) {
        Write-Host "macOS Intel build complete!" -ForegroundColor Green
    } else {
        Write-Host "macOS Intel build failed!" -ForegroundColor Red
        exit 1
    }

    # Build macOS ARM
    Write-Host "Building for macOS (Apple Silicon)..." -ForegroundColor Cyan
    $env:GOOS = "darwin"
    $env:GOARCH = "arm64"
    go build -ldflags $LdFlags -o "$OutputDir\osx\banyan-arm64" .
    if ($LASTEXITCODE -eq 0) {
        Write-Host "macOS ARM build complete!" -ForegroundColor Green
    } else {
        Write-Host "macOS ARM build failed!" -ForegroundColor Red
        exit 1
    }

    # Reset environment
    $env:GOOS = ""
    $env:GOARCH = ""

    # Copy debug startup scripts to each platform directory
    Write-Host "Copying debug startup scripts..." -ForegroundColor Cyan
    $ScriptsDir = Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "scripts"

    # Windows - copy .bat and .ps1 scripts
    if (Test-Path "$ScriptsDir\start-debug.ps1") {
        Copy-Item "$ScriptsDir\start-debug.ps1" "$OutputDir\win\start-debug.ps1" -Force
        Copy-Item "$ScriptsDir\start-debug.bat" "$OutputDir\win\start-debug.bat" -Force
        Write-Host "  Copied Windows debug scripts" -ForegroundColor Gray
    }

    # Linux - copy .sh script
    if (Test-Path "$ScriptsDir\start-debug.sh") {
        Copy-Item "$ScriptsDir\start-debug.sh" "$OutputDir\linux\start-debug.sh" -Force
        Write-Host "  Copied Linux debug script" -ForegroundColor Gray
    }

    # macOS - copy .sh script
    if (Test-Path "$ScriptsDir\start-debug.sh") {
        Copy-Item "$ScriptsDir\start-debug.sh" "$OutputDir\osx\start-debug.sh" -Force
        Write-Host "  Copied macOS debug script" -ForegroundColor Gray
    }
}

# Calculate hashes and sizes for built binaries
Write-Host "Calculating checksums and file sizes..." -ForegroundColor Cyan

function Get-FileInfo($FilePath) {
    if (Test-Path $FilePath) {
        $hash = (Get-FileHash -Path $FilePath -Algorithm SHA256).Hash.ToLower()
        $size = (Get-Item $FilePath).Length
        $sizeFormatted = if ($size -gt 1MB) { "{0:N1} MB" -f ($size / 1MB) } else { "{0:N1} KB" -f ($size / 1KB) }
        return @{
            hash = "sha256:$hash"
            size = $size
            sizeFormatted = $sizeFormatted
        }
    }
    return @{
        hash = "unknown"
        size = 0
        sizeFormatted = "unknown"
    }
}

$WindowsInfo = Get-FileInfo "$OutputDir\win\banyan.exe"
$LinuxInfo = Get-FileInfo "$OutputDir\linux\banyan"
$MacIntelInfo = Get-FileInfo "$OutputDir\osx\banyan-amd64"
$MacArmInfo = Get-FileInfo "$OutputDir\osx\banyan-arm64"

# Create release metadata
Write-Host "Creating release metadata..." -ForegroundColor Cyan
$GitCommit = "unknown"
try {
    $GitCommit = (git rev-parse HEAD 2>$null).Trim()
    if (-not $GitCommit) { $GitCommit = "unknown" }
} catch {
    $GitCommit = "unknown"
}

# Set version and whats new
$ReleaseVersion = if ($Version) { $Version } else { "pre-release" }
$ReleaseWhatsNew = if ($WhatsNew) { $WhatsNew } else { 
    try { 
        # Use Unicode code point to avoid encoding issues
        [char]::ConvertFromUtf32(0x1F60A)  # 😊 emoji
    } catch { 
        "nothing new" 
    } 
}

$ReleaseMetadata = @{
    version = $ReleaseVersion
    commit = $GitCommit
    buildDate = $BuildTime
    buildTime = $BuildTime
    whatsNew = $ReleaseWhatsNew
    downloads = @{
        windows = @{
            platform = "Windows"
            arch = "x64"
            filename = "banyan.exe"
            size = $WindowsInfo.sizeFormatted
            sizeBytes = $WindowsInfo.size
            checksum = $WindowsInfo.hash
        }
        linux = @{
            platform = "Linux"
            arch = "x64"
            filename = "banyan"
            size = $LinuxInfo.sizeFormatted
            sizeBytes = $LinuxInfo.size
            checksum = $LinuxInfo.hash
        }
        macos_intel = @{
            platform = "macOS"
            arch = "x64 (Intel)"
            filename = "banyan-amd64"
            size = $MacIntelInfo.sizeFormatted
            sizeBytes = $MacIntelInfo.size
            checksum = $MacIntelInfo.hash
        }
        macos_arm = @{
            platform = "macOS"
            arch = "ARM64 (Apple Silicon)"
            filename = "banyan-arm64"
            size = $MacArmInfo.sizeFormatted
            sizeBytes = $MacArmInfo.size
            checksum = $MacArmInfo.hash
        }
    }
} | ConvertTo-Json -Depth 10

# Write JSON with proper UTF-8 encoding (without BOM)
[System.IO.File]::WriteAllText("$OutputDir\release-metadata.json", $ReleaseMetadata, [System.Text.UTF8Encoding]::new($false))

Write-Host ""
if ($MetadataOnly) {
    Write-Host "Metadata regeneration completed successfully!" -ForegroundColor Green
    Write-Host ""
    Write-Host "Updated metadata: $OutputDir\release-metadata.json"
    Write-Host "Version: $ReleaseVersion"
    Write-Host "What's New: $ReleaseWhatsNew"
} else {
    Write-Host "All builds completed successfully!" -ForegroundColor Green
    Write-Host ""
    Write-Host "Build artifacts:"
    Write-Host "  Windows: $OutputDir\win\banyan.exe ($($WindowsInfo.sizeFormatted))"
    Write-Host "  Linux:   $OutputDir\linux\banyan ($($LinuxInfo.sizeFormatted))"
    Write-Host "  macOS:   $OutputDir\osx\banyan-amd64 ($($MacIntelInfo.sizeFormatted), Intel)"
    Write-Host "  macOS:   $OutputDir\osx\banyan-arm64 ($($MacArmInfo.sizeFormatted), Apple Silicon)"
    Write-Host "  Metadata: $OutputDir\release-metadata.json"
    Write-Host ""
    Write-Host "Checksums:"
    Write-Host "  Windows: $($WindowsInfo.hash)"
    Write-Host "  Linux:   $($LinuxInfo.hash)"
    Write-Host "  macOS Intel: $($MacIntelInfo.hash)"
    Write-Host "  macOS ARM:   $($MacArmInfo.hash)"
}
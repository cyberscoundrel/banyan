param(
    [switch]$Clean,
    [string]$OutputDir = "bin",
    [switch]$Help
)

if ($Help) {
    Write-Host "Usage: .\build.ps1 [-Clean] [-OutputDir DIR] [-Help]"
    Write-Host "  -Clean      Clean previous builds before building"
    Write-Host "  -OutputDir  Output directory for builds (default: bin)"
    Write-Host "  -Help       Show this help message"
    exit 0
}

Write-Host ""
Write-Host "Building Banyan CLI for all platforms..." -ForegroundColor Green
Write-Host ""

# Copy web dist files for embedding
$WebDistSource = "../web/dist"
$WebDistDest = "server/web/dist"

if (Test-Path $WebDistSource) {
    Write-Host "Copying web dist files for embedding..." -ForegroundColor Cyan
    if (Test-Path $WebDistDest) {
        Remove-Item -Recurse -Force $WebDistDest
    }
    New-Item -ItemType Directory -Force -Path "server/web" | Out-Null
    Copy-Item -Recurse $WebDistSource $WebDistDest
    Write-Host "Web dist files copied!" -ForegroundColor Green
} else {
    Write-Host "Warning: Web dist not found at $WebDistSource" -ForegroundColor Yellow
    Write-Host "Run 'nx build web' first to build the web UI" -ForegroundColor Yellow
    exit 1
}

$GitCommit = "dev"
try {
    $GitCommit = (git describe --tags --always --dirty 2>$null)
    if (-not $GitCommit) { $GitCommit = "dev" }
} catch {
    $GitCommit = "dev"
}

$BuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$LdFlags = "-s -w -X main.Version=$GitCommit -X main.BuildTime=$BuildTime"

if ($Clean -and (Test-Path $OutputDir)) {
    Write-Host "Cleaning previous builds..." -ForegroundColor Yellow
    Remove-Item -Recurse -Force "$OutputDir/*" -ErrorAction SilentlyContinue
}

New-Item -ItemType Directory -Force -Path "$OutputDir/win", "$OutputDir/linux", "$OutputDir/osx" | Out-Null

# Windows build
Write-Host "Building for Windows (amd64)..." -ForegroundColor Cyan
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags $LdFlags -o "$OutputDir/win/banyan-cli.exe" .
if ($LASTEXITCODE -eq 0) {
    Write-Host "Windows build complete!" -ForegroundColor Green
    # Create config file pointing to node executable (node builds as banyan.exe)
    $configWin = @{
        nodeExecutable = "../../node/win/banyan.exe"
        webPort = 8080
    } | ConvertTo-Json
    Set-Content -Path "$OutputDir/win/banyan-cli.json" -Value $configWin
} else {
    Write-Host "Windows build failed!" -ForegroundColor Red
    exit 1
}

# Linux build
Write-Host "Building for Linux (amd64)..." -ForegroundColor Cyan
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -ldflags $LdFlags -o "$OutputDir/linux/banyan-cli" .
if ($LASTEXITCODE -eq 0) {
    Write-Host "Linux build complete!" -ForegroundColor Green
    $configLinux = @{
        nodeExecutable = "../../node/linux/banyan"
        webPort = 8080
    } | ConvertTo-Json
    Set-Content -Path "$OutputDir/linux/banyan-cli.json" -Value $configLinux
} else {
    Write-Host "Linux build failed!" -ForegroundColor Red
    exit 1
}

# macOS Intel build
Write-Host "Building for macOS (Intel)..." -ForegroundColor Cyan
$env:GOOS = "darwin"
$env:GOARCH = "amd64"
go build -ldflags $LdFlags -o "$OutputDir/osx/banyan-cli-amd64" .
if ($LASTEXITCODE -eq 0) {
    Write-Host "macOS Intel build complete!" -ForegroundColor Green
    $configOsx = @{
        nodeExecutable = "../../node/osx/banyan-amd64"
        webPort = 8080
    } | ConvertTo-Json
    Set-Content -Path "$OutputDir/osx/banyan-cli-amd64.json" -Value $configOsx
} else {
    Write-Host "macOS Intel build failed!" -ForegroundColor Red
    exit 1
}

# macOS ARM build
Write-Host "Building for macOS (Apple Silicon)..." -ForegroundColor Cyan
$env:GOOS = "darwin"
$env:GOARCH = "arm64"
go build -ldflags $LdFlags -o "$OutputDir/osx/banyan-cli-arm64" .
if ($LASTEXITCODE -eq 0) {
    Write-Host "macOS ARM build complete!" -ForegroundColor Green
    # Update config to use ARM binary (overwrites Intel config)
    $configOsxArm = @{
        nodeExecutable = "../../node/osx/banyan-arm64"
        webPort = 8080
    } | ConvertTo-Json
    Set-Content -Path "$OutputDir/osx/banyan-cli-arm64.json" -Value $configOsxArm
} else {
    Write-Host "macOS ARM build failed!" -ForegroundColor Red
    exit 1
}

$env:GOOS = ""
$env:GOARCH = ""

Write-Host ""
Write-Host "All builds complete!" -ForegroundColor Green
Write-Host "Config files created with node executable paths." -ForegroundColor Cyan
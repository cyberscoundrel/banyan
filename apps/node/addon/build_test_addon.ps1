Param(
    [string]$GOOS = $env:GOOS,
    [string]$GOARCH = $env:GOARCH
)

$ErrorActionPreference = 'Stop'

$RootDir = (Resolve-Path "$PSScriptRoot\..\").Path
$AddonOut = Join-Path $RootDir 'bin\debug\addons'
New-Item -ItemType Directory -Force -Path $AddonOut | Out-Null

# Build test addon
$env:GOOS = $GOOS
$env:GOARCH = $GOARCH
pushd $RootDir | Out-Null
& go build -o (Join-Path $AddonOut 'testaddon.exe') ./addon/testaddon
popd | Out-Null

# Generate addons.json without BOM
$addonsJson = @{
  addons = @(
    @{ name = 'testaddon'; exec = './testaddon.exe' }
  )
} | ConvertTo-Json -Depth 4
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText((Join-Path $AddonOut 'addons.json'), $addonsJson, $Utf8NoBom)

Write-Host "Built test addon to $AddonOut and wrote addons.json"

param(
    [Parameter(Mandatory=$true)]
    [string]$Name,

    [int]$ProxyPort = 9090,
    [int]$ListenPort = 9100,
    [string]$Fig = "",
    [string]$OutputDir = "",

    [switch]$Help
)

# Generate a standalone "user node" directory that runs the banyan proxy addon
# on its own isolated Docker bridge network. The generated node uses the same
# runtime behavior as the service nodes in docker-compose.banyan.yml, but is
# created outside the topology generator (proxies do not belong in the service
# topology config).
#
# After running this script, cd into the generated directory and run .\up.ps1
# to build the image and start the container.

if ($Help) {
    @"
Usage: .\generate-user-node.ps1 -Name <node-name> [options]

Required:
  -Name <node-name>         Identifier used for container, image tag, bridge
                            network, and output subdirectory.

Options:
  -ProxyPort <hostPort>     Host port mapped to container :9090 (http-proxy addon).
                            Default: 9090
  -ListenPort <hostPort>    Host port mapped to container :9100 (libp2p listen).
                            Default: 9100
  -Fig <path>               Path to a signed client fig to bake into the node.
                            May have .fig or .json extension (content is
                            identical); the script always writes it out as
                            <name>.fig in the generated node's figs\ directory
                            because the node management API only loads files
                            with a .fig extension. Default:
                            <workspace>\apps\chat-service\topology\output\nodes\chat-static-1\services\chat-static-key\chat-fig.json
  -OutputDir <dir>          Where to write the generated node directory.
                            Default: <workspace>\test-utils\user-node\nodes\<name>
  -Help                     Show this help.
"@ | Write-Host
    exit 0
}

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$WorkspaceRoot = (Resolve-Path (Join-Path $ScriptDir "..\..")).Path

if ($Name -notmatch '^[a-zA-Z0-9][a-zA-Z0-9_.-]*$') {
    Write-Error "Name must match [a-zA-Z0-9][a-zA-Z0-9_.-]* (used as a docker identifier)"
    exit 2
}

if ([string]::IsNullOrEmpty($Fig)) {
    # Default to the fresh fig produced by the most recent topology regen. The
    # topology generator writes authoritative fig JSON inside the service
    # subdirectory (not in nodes\<name>\figs\, which is legacy and not
    # refreshed). Any service subdir will do — they all carry the same client
    # fig.
    $Fig = Join-Path $WorkspaceRoot "apps\chat-service\topology\output\nodes\chat-static-1\services\chat-static-key\chat-fig.json"
}
if ([string]::IsNullOrEmpty($OutputDir)) {
    $OutputDir = Join-Path $WorkspaceRoot "test-utils\user-node\nodes\$Name"
}

if (-not (Test-Path $Fig)) {
    Write-Error "Fig file not found: $Fig`nRun 'nx run chat-service:topology' first (or pass -Fig <path>)"
    exit 1
}
$FigLower = $Fig.ToLower()
if (-not ($FigLower.EndsWith(".fig") -or $FigLower.EndsWith(".json"))) {
    Write-Error "Fig path must have a .fig or .json extension: $Fig"
    exit 1
}

if (-not (Get-Command openssl -ErrorAction SilentlyContinue)) {
    Write-Error "openssl is not installed or not on PATH"
    exit 1
}

$ContainerName = "user-$Name"

New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $OutputDir "addons") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $OutputDir "figs") -Force | Out-Null

Write-Host "Generating user node '$Name' at $OutputDir"

# Generate an Ed25519 private key in PKCS8 PEM form. libp2p reads this format
# directly and it matches what the topology generator emits for service nodes.
$KeyPath = Join-Path $OutputDir "node-identity.pem"
& openssl genpkey -algorithm ed25519 -out $KeyPath 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Error "openssl genpkey failed"
    exit 1
}

# The .fig and .json variants are identical content; the node management API
# only loads files with a .fig extension, so always normalize the copy to .fig.
$FigStem = [System.IO.Path]::GetFileNameWithoutExtension($Fig)
Copy-Item -Path $Fig -Destination (Join-Path $OutputDir "figs\$FigStem.fig") -Force

@'
{
  "addons": [
    {
      "name": "http-proxy",
      "exec": "/usr/local/bin/proxy-addon",
      "args": ["-addr", ":9090"]
    }
  ]
}
'@ | Set-Content -Path (Join-Path $OutputDir "addons\addons.json") -Encoding utf8 -NoNewline

@'
FROM banyan-node:latest

COPY node-identity.pem /app/node-identity.pem
COPY addons /app/addons
COPY figs /app/figs
'@ | Set-Content -Path (Join-Path $OutputDir "Dockerfile") -Encoding utf8 -NoNewline

$UpScript = @"
# Build the user-node image and start it on a dedicated bridge network.
`$ErrorActionPreference = "Stop"
`$ScriptDir = Split-Path -Parent `$MyInvocation.MyCommand.Definition
Set-Location `$ScriptDir

`$Container = "${ContainerName}"
`$Image = "${ContainerName}:latest"
`$Network = "${ContainerName}"
`$ProxyPort = $ProxyPort
`$ListenPort = $ListenPort

& docker image inspect banyan-node:latest 2>`$null 1>`$null
if (`$LASTEXITCODE -ne 0) {
    Write-Error "banyan-node:latest image not found. Build it with: nx run chat-service:docker:build"
    exit 1
}

& docker network inspect `$Network 2>`$null 1>`$null
if (`$LASTEXITCODE -ne 0) {
    & docker network create --driver bridge `$Network | Out-Null
}

& docker build -t `$Image .
if (`$LASTEXITCODE -ne 0) { exit `$LASTEXITCODE }

& docker rm -f `$Container 2>`$null 1>`$null

& docker run -d --name `$Container --network `$Network ``
    --restart unless-stopped ``
    -p "`${ProxyPort}:9090" ``
    -p "`${ListenPort}:9100" ``
    `$Image ``
    -privkey=/app/node-identity.pem ``
    -addons-dir=/app/addons ``
    -figs-dir=/app/figs ``
    -listen=/ip4/0.0.0.0/tcp/9100 ``
    -no-crypto ``
    -allow-expired-figs ``
    -allow-insecure-figs ``
    -tunnel-enabled ``
    -override-transport-restrictions ``
    -nat-traversal
if (`$LASTEXITCODE -ne 0) { exit `$LASTEXITCODE }

Write-Host "User node '`$Container' running on network '`$Network'"
Write-Host "  browser proxy: http://localhost:`$ProxyPort"
Write-Host "  libp2p listen: host port `$ListenPort -> container :9100"
"@
$UpScript | Set-Content -Path (Join-Path $OutputDir "up.ps1") -Encoding utf8

$DownScript = @"
`$ErrorActionPreference = "Stop"
`$Container = "${ContainerName}"
`$Network = "${ContainerName}"

& docker rm -f `$Container 2>`$null 1>`$null
& docker network rm `$Network 2>`$null 1>`$null

Write-Host "User node '`$Container' stopped and network removed"
"@
$DownScript | Set-Content -Path (Join-Path $OutputDir "down.ps1") -Encoding utf8

Write-Host ""
Write-Host "Done. Next steps:"
Write-Host "  cd $OutputDir"
Write-Host "  .\up.ps1      # build image and start container"
Write-Host "  .\down.ps1    # stop container and remove its bridge network"

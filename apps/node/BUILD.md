# Banyan Build System

This document describes how to build Banyan for different platforms using the provided build scripts.

## Quick Start

### PowerShell (Windows)
```powershell
# Build for all platforms
.\build.ps1

# Clean previous builds first
.\build.ps1 -Clean
```

### Bash (Linux/macOS/WSL)
```bash
# Make executable (first time only)
chmod +x build.sh

# Build for all platforms
./build.sh
```

### Make (Universal)
```bash
# Build for all platforms
make all

# Build for specific platform
make windows
make linux
make macos

# Show help
make help
```

## Build Targets

The build system creates binaries for the following platforms:

| Platform | Architecture | Output File |
|----------|-------------|-------------|
| Windows | AMD64 | `bin/win/banyan.exe` |
| Linux | AMD64 | `bin/linux/banyan` |
| macOS | AMD64 (Intel) | `bin/osx/banyan-amd64` |
| macOS | ARM64 (Apple Silicon) | `bin/osx/banyan-arm64` |

## Build Scripts

### `build.ps1` (PowerShell)

**Features:**
- Cross-platform Go compilation
- Automatic version detection from Git
- Build timestamp embedding
- SHA256 checksum calculation for all binaries
- File size calculation and formatting
- Release metadata JSON generation
- Colored output with emojis
- Optional clean flag

**Usage:**
```powershell
# Basic build
.\build.ps1

# Clean previous builds
.\build.ps1 -Clean

# Regenerate metadata only (no compilation)
.\build.ps1 -MetadataOnly

# Custom version and features
.\build.ps1 -Version "1.0.0" -WhatsNew "New features, Bug fixes"

# Metadata only with custom info
.\build.ps1 -MetadataOnly -Version "1.0.1" -WhatsNew "Hotfix release"
```

### `build.sh` (Bash)

**Features:**
- Cross-platform Go compilation
- Git version detection
- Build optimization flags
- SHA256 checksum calculation for all binaries
- File size calculation and formatting
- Release metadata JSON generation
- Error handling with exit codes

**Usage:**
```bash
# Make executable (first time)
chmod +x build.sh

# Basic build
./build.sh

# Regenerate metadata only (no compilation)
./build.sh --metadata-only

# Custom version and features
./build.sh --version "1.0.0" --whats-new "New features, Bug fixes"

# Metadata only with custom info
./build.sh --metadata-only --version "1.0.1" --whats-new "Hotfix release"
```

### `Makefile` (Make)

**Features:**
- Individual platform targets
- Parallel building support
- Dependency management
- Test runner integration
- Help system

**Usage:**
```bash
# Build all platforms
make all

# Build specific platforms
make windows
make linux
make macos

# Utility targets
make clean      # Remove build artifacts
make deps       # Download dependencies
make test       # Run tests
make info       # Show build information
make help       # Show available targets
```

## Build Requirements

### Prerequisites
- **Go 1.21+** - Required for building
- **Git** - Optional, for version detection
- **Make** - Optional, for makefile usage

### Environment Variables

The build system uses the following Go environment variables:
- `GOOS` - Target operating system
- `GOARCH` - Target architecture

These are automatically set by the build scripts.

## Build Flags

All build scripts use the following optimization flags:

### Linker Flags (`-ldflags`)
- `-s` - Strip symbol table and debug information
- `-w` - Strip DWARF debug information
- `-X main.version=<version>` - Embed version information
- `-X main.buildTime=<timestamp>` - Embed build timestamp

### Compilation Flags
- Cross-compilation via `GOOS` and `GOARCH`
- Static linking (Go default)
- No CGO dependencies

## Output Structure

```
bin/
├── linux/
│   └── banyan              # Linux AMD64 binary
├── osx/
│   ├── banyan-amd64        # macOS Intel binary
│   └── banyan-arm64        # macOS Apple Silicon binary
├── win/
│   └── banyan.exe          # Windows AMD64 binary
└── release-metadata.json   # Release metadata with checksums
```

## Release Metadata

The build scripts generate a `release-metadata.json` file containing:
- Version and build information
- SHA256 checksums for all binaries
- File sizes (formatted and raw bytes)
- Platform and architecture details
- Build timestamps and Git commit info
- UTF-8 encoded JSON with proper Unicode support

Example structure:
```json
{
  "version": "v1.0.0",
  "commit": "a1b2c3d4",
  "buildDate": "2024-01-15T10:30:45Z",
  "buildTime": "2024-01-15T10:30:45Z",
  "whatsNew": "New features, Bug fixes",
  "downloads": {
    "windows": {
      "platform": "Windows",
      "arch": "x64",
      "filename": "banyan.exe",
      "size": "12.3 MB",
      "sizeBytes": 12884901,
      "checksum": "sha256:a1b2c3d4e5f6..."
    },
    ...
  }
}
```

## Troubleshooting

### Common Issues

#### Build Fails - Go Not Found
```bash
# Verify Go installation
go version

# Install Go if needed
# https://golang.org/doc/install
```

#### Permission Denied (Linux/macOS)
```bash
# Make script executable
chmod +x build.sh
```

#### PowerShell Execution Policy (Windows)
```powershell
# Allow local scripts
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

#### Build Succeeds But Binary Won't Run

**Windows:**
- Ensure you're running on 64-bit Windows
- Check Windows Defender/antivirus

**Linux:**
- Verify architecture: `uname -m` (should be x86_64)
- Check permissions: `chmod +x bin/linux/banyan`

**macOS:**
- Use correct binary for your Mac:
  - Intel Macs: `banyan-amd64`
  - Apple Silicon: `banyan-arm64`
- Handle Gatekeeper: `xattr -d com.apple.quarantine bin/osx/banyan-*`

### Clean Builds

If you encounter issues, try a clean build:

```bash
# PowerShell
.\build.ps1 -Clean

# Bash
rm -rf bin/win/* bin/linux/* bin/osx/*
./build.sh

# Make
make clean
make all
```

## Advanced Usage

### Custom Build Flags

To add custom build flags, modify the scripts:

```bash
# In build.sh, modify LDFLAGS:
LDFLAGS="-s -w -X main.version=${VERSION} -X main.customFlag=value"

# In build.ps1, modify $LdFlags:
$LdFlags = "-s -w -X main.version=$Version -X main.customFlag=value"
```

### Parallel Building

The makefile supports parallel builds:

```bash
# Build with 4 parallel jobs
make -j4 all
```

### Version Information

The build system automatically embeds version information:

```bash
# Check version in built binary
./bin/linux/banyan --version
bin\win\banyan.exe --version
```

Version detection priority:
1. Git tag (if on tagged commit)
2. Git commit hash (with -dirty suffix if uncommitted changes)
3. "dev" (fallback)

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Build
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: '1.21'
    - name: Build all platforms
      run: ./build.sh
    - name: Upload artifacts
      uses: actions/upload-artifact@v3
      with:
        name: banyan-binaries
        path: bin/
```

### Docker Build

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN apk add --no-cache git make
RUN make all

FROM scratch
COPY --from=builder /app/bin/ /bin/
```

## Performance Notes

- **Build time**: ~30-60 seconds for all platforms
- **Binary sizes**: ~25-26 MB per binary
- **Memory usage**: Go cross-compilation is memory efficient
- **Disk space**: ~100 MB for all binaries

The binaries are statically linked and have no external dependencies.


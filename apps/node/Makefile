# Banyan Makefile
# Cross-platform build targets

.PHONY: all clean windows linux macos help

# Default target
all: windows linux macos

# Get version information
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)

# Create bin directories
bin/win bin/linux bin/osx:
	mkdir -p $@

# Windows build
windows: bin/win
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/win/banyan.exe .
	@echo "✅ Windows build complete: bin/win/banyan.exe"

# Linux build  
linux: bin/linux
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/linux/banyan .
	@echo "✅ Linux build complete: bin/linux/banyan"

# macOS builds (both Intel and Apple Silicon)
macos: bin/osx
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/osx/banyan-amd64 .
	@echo "✅ macOS Intel build complete: bin/osx/banyan-amd64"
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bin/osx/banyan-arm64 .
	@echo "✅ macOS Apple Silicon build complete: bin/osx/banyan-arm64"

# Clean build artifacts
clean:
	rm -rf bin/win/* bin/linux/* bin/osx/*
	@echo "🧹 Build artifacts cleaned"

# Show build info
info:
	@echo "📋 Build Information:"
	@echo "   Version: $(VERSION)"
	@echo "   Build Time: $(BUILD_TIME)"
	@echo "   Go Version: $(shell go version)"

# Install dependencies
deps:
	go mod download
	go mod tidy

# Run tests
test:
	go test ./...

# Help target
help:
	@echo "🌳 Banyan Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  all      - Build for all platforms (default)"
	@echo "  windows  - Build for Windows (amd64)"
	@echo "  linux    - Build for Linux (amd64)"
	@echo "  macos    - Build for macOS (Intel + Apple Silicon)"
	@echo "  clean    - Remove build artifacts"
	@echo "  deps     - Download and tidy dependencies"
	@echo "  test     - Run tests"
	@echo "  info     - Show build information"
	@echo "  help     - Show this help"
	@echo ""
	@echo "Examples:"
	@echo "  make all     # Build for all platforms"
	@echo "  make clean   # Clean build artifacts"
	@echo "  make windows # Build only for Windows"


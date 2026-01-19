# Banyan CLI Makefile
# Cross-platform build targets

.PHONY: all clean windows linux macos help

# Default target
all: windows linux macos

# Get version information
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)

# Create bin directories
bin/win bin/linux bin/osx:
	mkdir -p $@

# Windows build
windows: bin/win
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/win/banyan-cli.exe .
	@echo '{"nodeExecutable": "../../node/win/banyan.exe", "webPort": 8080}' > bin/win/banyan-cli.json
	@echo "✅ Windows build complete: bin/win/banyan-cli.exe"

# Linux build
linux: bin/linux
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/linux/banyan-cli .
	@echo '{"nodeExecutable": "../../node/linux/banyan", "webPort": 8080}' > bin/linux/banyan-cli.json
	@echo "✅ Linux build complete: bin/linux/banyan-cli"

# macOS builds (both Intel and Apple Silicon)
macos: bin/osx
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/osx/banyan-cli-amd64 .
	@echo '{"nodeExecutable": "../../node/osx/banyan-amd64", "webPort": 8080}' > bin/osx/banyan-cli-amd64.json
	@echo "✅ macOS Intel build complete: bin/osx/banyan-cli-amd64"
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bin/osx/banyan-cli-arm64 .
	@echo '{"nodeExecutable": "../../node/osx/banyan-arm64", "webPort": 8080}' > bin/osx/banyan-cli-arm64.json
	@echo "✅ macOS Apple Silicon build complete: bin/osx/banyan-cli-arm64"

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

# Help
help:
	@echo "Banyan CLI Build System"
	@echo ""
	@echo "Targets:"
	@echo "  all      - Build for all platforms (default)"
	@echo "  windows  - Build for Windows (amd64)"
	@echo "  linux    - Build for Linux (amd64)"
	@echo "  macos    - Build for macOS (Intel + Apple Silicon)"
	@echo "  clean    - Remove build artifacts"
	@echo "  deps     - Download and tidy dependencies"
	@echo "  test     - Run tests"
	@echo "  info     - Show build information"
	@echo "  help     - Show this help message"


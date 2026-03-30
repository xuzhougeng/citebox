# CiteBox - Cross-Platform Build Makefile

.PHONY: build run dev build-desktop run-desktop build-windows build-darwin build-linux package-windows package-darwin package-linux package-desktop-windows package-desktop-darwin package-desktop-linux prepare-web-assets clean test version help

BINARY_NAME=citebox
DESKTOP_BINARY_NAME=$(BINARY_NAME)-desktop
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
BUILDINFO_PKG=github.com/xuzhougeng/citebox/internal/buildinfo

# Build flags for smaller binary
LDFLAGS=-ldflags "-s -w -X $(BUILDINFO_PKG).Version=$(VERSION) -X $(BUILDINFO_PKG).BuildTime=$(BUILD_TIME)"
DESKTOP_LDFLAGS=$(LDFLAGS)

ifeq ($(OS),Windows_NT)
DESKTOP_LDFLAGS=-ldflags "-s -w -H windowsgui -X $(BUILDINFO_PKG).Version=$(VERSION) -X $(BUILDINFO_PKG).BuildTime=$(BUILD_TIME)"
endif

# =============================================================================
# Local Development
# =============================================================================

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/server

run:
	go run ./cmd/server

dev: prepare-web-assets
	DISABLE_AUTH=1 go run ./cmd/server

build-desktop:
	@mkdir -p bin
	go build $(DESKTOP_LDFLAGS) -o bin/$(DESKTOP_BINARY_NAME) ./cmd/desktop

run-desktop:
	go run ./cmd/desktop

prepare-web-assets:
	go run ./scripts/fetch_pdfjs.go web/static/vendor/pdfjs

test:
	go test ./...

# =============================================================================
# Windows Build & Package
# =============================================================================

ifeq ($(OS),Windows_NT)

build-windows:
	@echo "Building Windows executable..."
	@pwsh -File scripts/build-windows.ps1 -Version $(VERSION)
	@echo "✓ Built: bin/windows/$(BINARY_NAME).exe"

package-windows:
	@echo ""
	@echo "========================================"
	@echo "Creating Windows Distribution Package"
	@echo "Version: $(VERSION)"
	@echo "========================================"
	@pwsh -File scripts/package-windows.ps1 -Version $(VERSION)
	
	@echo ""
	@echo "✓ Created: dist/$(BINARY_NAME)-windows-$(VERSION).zip"

else

build-windows:
	@echo "Windows server builds require a native Windows host so CGO and MuPDF stay enabled."
	@echo "Use a Windows runner or the GitHub Actions release workflow."
	@exit 1

package-windows:
	@echo "Windows server packaging requires a native Windows host so CGO and MuPDF stay enabled."
	@echo "Use a Windows runner or the GitHub Actions release workflow."
	@exit 1

endif

# =============================================================================
# macOS Build & Package
# =============================================================================

build-darwin:
	@echo "Building macOS executables..."
	@mkdir -p bin/darwin
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/darwin/$(BINARY_NAME)-amd64 ./cmd/server
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/darwin/$(BINARY_NAME)-arm64 ./cmd/server
	@echo "✓ Built: bin/darwin/$(BINARY_NAME)-amd64 (Intel)"
	@echo "✓ Built: bin/darwin/$(BINARY_NAME)-arm64 (Apple Silicon)"

package-darwin: build-darwin
	@echo ""
	@echo "========================================"
	@echo "Creating macOS Distribution Package"
	@echo "Version: $(VERSION)"
	@echo "========================================"
	
	@rm -rf dist/$(BINARY_NAME)-darwin-$(VERSION)
	@mkdir -p dist/$(BINARY_NAME)-darwin-$(VERSION)/data/library/papers
	@mkdir -p dist/$(BINARY_NAME)-darwin-$(VERSION)/data/library/figures
	
	@cp bin/darwin/$(BINARY_NAME)-amd64 dist/$(BINARY_NAME)-darwin-$(VERSION)/$(BINARY_NAME)
	@cp -r web dist/$(BINARY_NAME)-darwin-$(VERSION)/
	@cp README.md dist/$(BINARY_NAME)-darwin-$(VERSION)/
	
	@echo "Creating start.sh..."
	@( \
		echo '#!/bin/bash'; \
		echo 'cd "$$(dirname "$$0")"'; \
		echo 'clear'; \
		echo 'echo "========================================"'; \
		echo 'echo "  CiteBox"'; \
		echo 'echo "  Version: $(VERSION)"'; \
		echo 'echo "========================================"'; \
		echo 'echo ""'; \
		echo 'echo "Starting server..."'; \
		echo 'echo ""'; \
		echo 'echo "Default URL: http://localhost:8080"'; \
		echo 'echo "Username: citebox"'; \
		echo 'echo "Password: citebox123"'; \
		echo 'echo ""'; \
		echo 'echo "Press Ctrl+C to stop"'; \
		echo 'echo ""'; \
		echo './$(BINARY_NAME)'; \
	) > dist/$(BINARY_NAME)-darwin-$(VERSION)/start.sh
	@chmod +x dist/$(BINARY_NAME)-darwin-$(VERSION)/start.sh
	
	@echo "Creating start-with-config.sh..."
	@( \
		echo '#!/bin/bash'; \
		echo 'cd "$$(dirname "$$0")"'; \
		echo 'clear'; \
		echo 'echo "========================================"'; \
		echo 'echo "  CiteBox - Custom Config"'; \
		echo 'echo "========================================"'; \
		echo 'echo ""'; \
		echo '# Customize settings below:'; \
		echo 'export SERVER_PORT=8080'; \
		echo 'export ADMIN_USERNAME=citebox'; \
		echo 'export ADMIN_PASSWORD=citebox123'; \
		echo '# export PDF_EXTRACTOR_URL=http://localhost:8000'; \
		echo '# export STORAGE_DIR=./data/library'; \
		echo '# export DATABASE_PATH=./data/library.db'; \
		echo 'echo ""'; \
		echo 'echo "Starting with custom configuration..."'; \
		echo 'echo "Port: $$SERVER_PORT"'; \
		echo 'echo ""'; \
		echo './$(BINARY_NAME)'; \
	) > dist/$(BINARY_NAME)-darwin-$(VERSION)/start-with-config.sh
	@chmod +x dist/$(BINARY_NAME)-darwin-$(VERSION)/start-with-config.sh
	
	@echo "Creating README.txt..."
	@( \
		echo 'CiteBox - macOS 版'; \
		echo '==================================='; \
		echo ''; \
		echo '快速开始:'; \
		echo '  1. 解压后打开 Terminal'; \
		echo '  2. cd 到解压目录'; \
		echo '  3. 运行 ./start.sh'; \
		echo '  4. 浏览器访问 http://localhost:8080'; \
		echo ''; \
		echo '注意:'; \
		echo '  首次运行可能需要赋予执行权限:'; \
		echo '  chmod +x $(BINARY_NAME) start.sh'; \
		echo ''; \
		echo '自定义配置:'; \
		echo '  编辑 start-with-config.sh 修改配置'; \
	) > dist/$(BINARY_NAME)-darwin-$(VERSION)/README.txt
	
	@cd dist && zip -rq $(BINARY_NAME)-darwin-$(VERSION).zip $(BINARY_NAME)-darwin-$(VERSION)
	
	@echo ""
	@echo "✓ Created: dist/$(BINARY_NAME)-darwin-$(VERSION).zip"
	@echo ""
	@echo "Note: This package contains Intel binary."
	@echo "For Apple Silicon (M1/M2/M3), run: make build-darwin"

# =============================================================================
# Linux Build & Package
# =============================================================================

build-linux:
	@echo "Building Linux executables..."
	@mkdir -p bin/linux
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/linux/$(BINARY_NAME)-amd64 ./cmd/server
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/linux/$(BINARY_NAME)-arm64 ./cmd/server
	@echo "✓ Built: bin/linux/$(BINARY_NAME)-amd64 (x86_64)"
	@echo "✓ Built: bin/linux/$(BINARY_NAME)-arm64 (ARM64)"

package-linux: build-linux
	@echo ""
	@echo "========================================"
	@echo "Creating Linux Distribution Package"
	@echo "Version: $(VERSION)"
	@echo "========================================"
	
	@rm -rf dist/$(BINARY_NAME)-linux-$(VERSION)
	@mkdir -p dist/$(BINARY_NAME)-linux-$(VERSION)/data/library/papers
	@mkdir -p dist/$(BINARY_NAME)-linux-$(VERSION)/data/library/figures
	
	@cp bin/linux/$(BINARY_NAME)-amd64 dist/$(BINARY_NAME)-linux-$(VERSION)/$(BINARY_NAME)
	@cp -r web dist/$(BINARY_NAME)-linux-$(VERSION)/
	@cp README.md dist/$(BINARY_NAME)-linux-$(VERSION)/
	
	@echo "Creating start.sh..."
	@( \
		echo '#!/bin/bash'; \
		echo 'cd "$$(dirname "$$0")"'; \
		echo 'clear'; \
		echo 'echo "========================================"'; \
		echo 'echo "  CiteBox"'; \
		echo 'echo "  Version: $(VERSION)"'; \
		echo 'echo "========================================"'; \
		echo 'echo ""'; \
		echo 'echo "Starting server..."'; \
		echo 'echo ""'; \
		echo 'echo "Default URL: http://localhost:8080"'; \
		echo 'echo "Username: citebox"'; \
		echo 'echo "Password: citebox123"'; \
		echo 'echo ""'; \
		echo 'echo "Press Ctrl+C to stop"'; \
		echo 'echo ""'; \
		echo './$(BINARY_NAME)'; \
	) > dist/$(BINARY_NAME)-linux-$(VERSION)/start.sh
	@chmod +x dist/$(BINARY_NAME)-linux-$(VERSION)/start.sh
	
	@echo "Creating start-with-config.sh..."
	@( \
		echo '#!/bin/bash'; \
		echo 'cd "$$(dirname "$$0")"'; \
		echo 'clear'; \
		echo 'echo "========================================"'; \
		echo 'echo "  CiteBox - Custom Config"'; \
		echo 'echo "========================================"'; \
		echo 'echo ""'; \
		echo '# Customize settings below:'; \
		echo 'export SERVER_PORT=8080'; \
		echo 'export ADMIN_USERNAME=citebox'; \
		echo 'export ADMIN_PASSWORD=citebox123'; \
		echo '# export PDF_EXTRACTOR_URL=http://localhost:8000'; \
		echo '# export STORAGE_DIR=./data/library'; \
		echo '# export DATABASE_PATH=./data/library.db'; \
		echo 'echo ""'; \
		echo 'echo "Starting with custom configuration..."'; \
		echo 'echo "Port: $$SERVER_PORT"'; \
		echo 'echo ""'; \
		echo './$(BINARY_NAME)'; \
	) > dist/$(BINARY_NAME)-linux-$(VERSION)/start-with-config.sh
	@chmod +x dist/$(BINARY_NAME)-linux-$(VERSION)/start-with-config.sh
	
	@echo "Creating README.txt..."
	@( \
		echo 'CiteBox - Linux 版'; \
		echo '==================================='; \
		echo ''; \
		echo '快速开始:'; \
		echo '  1. 解压后 cd 到解压目录'; \
		echo '  2. chmod +x $(BINARY_NAME) start.sh'; \
		echo '  3. ./start.sh'; \
		echo '  4. 浏览器访问 http://localhost:8080'; \
		echo ''; \
		echo '后台运行:'; \
		echo '  nohup ./$(BINARY_NAME) &'; \
		echo ''; \
		echo '自定义配置:'; \
		echo '  编辑 start-with-config.sh 修改配置'; \
	) > dist/$(BINARY_NAME)-linux-$(VERSION)/README.txt
	
	@cd dist && zip -rq $(BINARY_NAME)-linux-$(VERSION).zip $(BINARY_NAME)-linux-$(VERSION)
	
	@echo ""
	@echo "✓ Created: dist/$(BINARY_NAME)-linux-$(VERSION).zip"

package-desktop-linux:
	@echo ""
	@echo "========================================"
	@echo "Creating Linux Desktop Package"
	@echo "Version: $(VERSION)"
	@echo "========================================"
	@bash scripts/package-desktop-unix.sh linux $(VERSION)
	@echo ""
	@echo "Note: This package contains the host-architecture binary."

package-desktop-darwin:
	@echo ""
	@echo "========================================"
	@echo "Creating macOS Desktop DMG"
	@echo "Version: $(VERSION)"
	@echo "========================================"
	@bash scripts/package-desktop-macos.sh $(VERSION)
	@echo ""
	@echo "Note: This DMG contains the host-architecture app bundle."

ifeq ($(OS),Windows_NT)

package-desktop-windows:
	@echo ""
	@echo "========================================"
	@echo "Creating Windows Desktop Installer"
	@echo "Version: $(VERSION)"
	@echo "========================================"
	@pwsh -File scripts/package-desktop-windows.ps1 -Version $(VERSION)
	@echo ""
	@echo "Note: This installer contains the host-architecture binary."

else

package-desktop-windows:
	@echo "Windows desktop packaging requires a native Windows host with NSIS and the native CGO toolchain."
	@echo "Use a Windows runner or the GitHub Actions release workflow."
	@exit 1

endif

# =============================================================================
# Build All Packages
# =============================================================================

package-all: package-windows package-darwin package-linux
	@echo ""
	@echo "========================================"
	@echo "All Packages Created Successfully!"
	@echo "========================================"
	@ls -lh dist/*.zip 2>/dev/null || echo "No packages found"
	@echo ""

# =============================================================================
# Utilities
# =============================================================================

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/ dist/
	@echo "✓ Done."

version:
	@echo "Version: $(VERSION)"

help:
	@echo "CiteBox - Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Development:"
	@echo "  make build          - Build for current platform"
	@echo "  make run            - Run development server"
	@echo "  make dev            - Prepare PDF.js assets, then run development server"
	@echo "  make prepare-web-assets - Download PDF.js runtime assets for source runs"
	@echo "  make test           - Run tests"
	@echo ""
	@echo "Windows:"
	@echo "  make build-windows  - Build Windows executable (native Windows only)"
	@echo "  make package-windows - Create Windows ZIP package (native Windows only)"
	@echo "  make package-desktop-windows - Create Windows desktop installer (.exe, native Windows only)"
	@echo ""
	@echo "macOS:"
	@echo "  make build-darwin   - Build macOS executables (Intel + Apple Silicon)"
	@echo "  make package-darwin - Create macOS ZIP package"
	@echo "  make package-desktop-darwin - Create macOS desktop DMG"
	@echo ""
	@echo "Linux:"
	@echo "  make build-linux    - Build Linux executables (x86_64 + ARM64)"
	@echo "  make package-linux  - Create Linux ZIP package"
	@echo "  make package-desktop-linux - Create Linux desktop tar.gz package"
	@echo ""
	@echo "All Platforms:"
	@echo "  make package-all    - Create packages for all platforms"
	@echo ""
	@echo "Utilities:"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make version        - Show version"
	@echo "  make help           - Show this help"

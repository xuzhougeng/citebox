# Paper Image Database - Cross-Platform Build Makefile

.PHONY: build build-windows build-darwin build-linux package-windows package-darwin package-linux clean test version help

BINARY_NAME=citebox
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build flags for smaller binary
LDFLAGS=-ldflags "-s -w"

# =============================================================================
# Local Development
# =============================================================================

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./...

# =============================================================================
# Windows Build & Package
# =============================================================================

build-windows:
	@echo "Building Windows executable..."
	@mkdir -p bin/windows
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/windows/$(BINARY_NAME).exe ./cmd/server
	@echo "✓ Built: bin/windows/$(BINARY_NAME).exe"

package-windows: build-windows
	@echo ""
	@echo "========================================"
	@echo "Creating Windows Distribution Package"
	@echo "Version: $(VERSION)"
	@echo "========================================"
	
	@rm -rf dist/$(BINARY_NAME)-windows-$(VERSION)
	@mkdir -p dist/$(BINARY_NAME)-windows-$(VERSION)/data/library/papers
	@mkdir -p dist/$(BINARY_NAME)-windows-$(VERSION)/data/library/figures
	
	@cp bin/windows/$(BINARY_NAME).exe dist/$(BINARY_NAME)-windows-$(VERSION)/
	@cp -r web dist/$(BINARY_NAME)-windows-$(VERSION)/
	@cp README.md dist/$(BINARY_NAME)-windows-$(VERSION)/
	
	@echo "Creating start.bat..."
	@( \
		echo '@echo off'; \
		echo 'chcp 65001 >nul'; \
		echo 'title Paper Image Database'; \
		echo 'cls'; \
		echo 'echo ========================================'; \
		echo 'echo  Paper Image Database'; \
		echo 'echo  Version: $(VERSION)'; \
		echo 'echo ========================================'; \
		echo 'echo.'; \
		echo 'echo Starting server...'; \
		echo 'echo.'; \
		echo 'echo Default URL: http://localhost:8080'; \
		echo 'echo Username: wanglab'; \
		echo 'echo Password: wanglab789'; \
		echo 'echo.'; \
		echo 'echo A browser window will open automatically.'; \
		echo 'echo Close the "Paper Image Database" window to stop the app.'; \
		echo 'echo.'; \
		echo 'start "Paper Image Database" $(BINARY_NAME).exe'; \
		echo 'timeout /t 2 /nobreak >nul'; \
		echo 'start "" http://localhost:8080'; \
	) > dist/$(BINARY_NAME)-windows-$(VERSION)/start.bat
	
	@echo "Creating start-with-config.bat..."
	@( \
		echo '@echo off'; \
		echo 'chcp 65001 >nul'; \
		echo 'title Paper Image Database (Custom Config)'; \
		echo 'cls'; \
		echo 'echo ========================================'; \
		echo 'echo  Paper Image Database - Custom Config'; \
		echo 'echo ========================================'; \
		echo 'echo.'; \
		echo 'rem Customize settings below:'; \
		echo 'set SERVER_PORT=8080'; \
		echo 'set ADMIN_USERNAME=wanglab'; \
		echo 'set ADMIN_PASSWORD=wanglab789'; \
		echo 'rem set PDF_EXTRACTOR_URL=http://localhost:8000'; \
		echo 'rem set STORAGE_DIR=./data/library'; \
		echo 'rem set DATABASE_PATH=./data/library.db'; \
		echo 'echo.'; \
		echo 'echo Starting with custom configuration...'; \
		echo 'echo Port: %SERVER_PORT%'; \
		echo 'echo.'; \
		echo 'start "Paper Image Database" $(BINARY_NAME).exe'; \
		echo 'timeout /t 2 /nobreak >nul'; \
		echo 'start "" http://localhost:%SERVER_PORT%'; \
	) > dist/$(BINARY_NAME)-windows-$(VERSION)/start-with-config.bat
	
	@echo "Creating README.txt..."
	@( \
		echo 'Paper Image Database - Windows 版'; \
		echo '===================================='; \
		echo ''; \
		echo '快速开始:'; \
		echo '  1. 解压后双击 start.bat'; \
		echo '  2. 浏览器访问 http://localhost:8080'; \
		echo '  3. 默认账号: wanglab / wanglab789'; \
		echo ''; \
		echo '自定义配置:'; \
		echo '  编辑 start-with-config.bat 修改配置'; \
		echo ''; \
		echo '数据目录:'; \
		echo '  data/library.db       - 数据库'; \
		echo '  data/library/papers/  - PDF 文件'; \
		echo '  data/library/figures/ - 提取图片'; \
	) > dist/$(BINARY_NAME)-windows-$(VERSION)/README.txt
	
	@cd dist && zip -rq $(BINARY_NAME)-windows-$(VERSION).zip $(BINARY_NAME)-windows-$(VERSION)
	
	@echo ""
	@echo "✓ Created: dist/$(BINARY_NAME)-windows-$(VERSION).zip"

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
		echo 'echo "  Paper Image Database"'; \
		echo 'echo "  Version: $(VERSION)"'; \
		echo 'echo "========================================"'; \
		echo 'echo ""'; \
		echo 'echo "Starting server..."'; \
		echo 'echo ""'; \
		echo 'echo "Default URL: http://localhost:8080"'; \
		echo 'echo "Username: wanglab"'; \
		echo 'echo "Password: wanglab789"'; \
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
		echo 'echo "  Paper Image Database - Custom Config"'; \
		echo 'echo "========================================"'; \
		echo 'echo ""'; \
		echo '# Customize settings below:'; \
		echo 'export SERVER_PORT=8080'; \
		echo 'export ADMIN_USERNAME=wanglab'; \
		echo 'export ADMIN_PASSWORD=wanglab789'; \
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
		echo 'Paper Image Database - macOS 版'; \
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
		echo 'echo "  Paper Image Database"'; \
		echo 'echo "  Version: $(VERSION)"'; \
		echo 'echo "========================================"'; \
		echo 'echo ""'; \
		echo 'echo "Starting server..."'; \
		echo 'echo ""'; \
		echo 'echo "Default URL: http://localhost:8080"'; \
		echo 'echo "Username: wanglab"'; \
		echo 'echo "Password: wanglab789"'; \
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
		echo 'echo "  Paper Image Database - Custom Config"'; \
		echo 'echo "========================================"'; \
		echo 'echo ""'; \
		echo '# Customize settings below:'; \
		echo 'export SERVER_PORT=8080'; \
		echo 'export ADMIN_USERNAME=wanglab'; \
		echo 'export ADMIN_PASSWORD=wanglab789'; \
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
		echo 'Paper Image Database - Linux 版'; \
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
	@echo ""
	@echo "Note: This package contains x86_64 binary."
	@echo "For ARM64, run: make build-linux"

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
	@echo "Paper Image Database - Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Development:"
	@echo "  make build          - Build for current platform"
	@echo "  make run            - Run development server"
	@echo "  make test           - Run tests"
	@echo ""
	@echo "Windows:"
	@echo "  make build-windows  - Build Windows executable"
	@echo "  make package-windows - Create Windows ZIP package"
	@echo ""
	@echo "macOS:"
	@echo "  make build-darwin   - Build macOS executables (Intel + Apple Silicon)"
	@echo "  make package-darwin - Create macOS ZIP package"
	@echo ""
	@echo "Linux:"
	@echo "  make build-linux    - Build Linux executables (x86_64 + ARM64)"
	@echo "  make package-linux  - Create Linux ZIP package"
	@echo ""
	@echo "All Platforms:"
	@echo "  make package-all    - Create packages for all platforms"
	@echo ""
	@echo "Utilities:"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make version        - Show version"
	@echo "  make help           - Show this help"

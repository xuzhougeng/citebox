# Paper Image Database - Windows Build Makefile

.PHONY: build build-windows package-windows clean test version

BINARY_NAME=paper_image_db
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

# Build Windows executable only
build-windows:
	@echo "Building Windows executable..."
	@mkdir -p bin/windows
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/windows/$(BINARY_NAME).exe ./cmd/server
	@echo "Built: bin/windows/$(BINARY_NAME).exe"

# Create complete Windows distribution package
package-windows: build-windows
	@echo ""
	@echo "========================================"
	@echo "Creating Windows Distribution Package"
	@echo "Version: $(VERSION)"
	@echo "========================================"
	@echo ""
	
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
		echo 'echo Press Ctrl+C to stop'; \
		echo 'echo.'; \
		echo '$(BINARY_NAME).exe'; \
		echo 'echo.'; \
		echo 'pause'; \
	) > dist/$(BINARY_NAME)-windows-$(VERSION)/start.bat
	
	@echo "Creating start-with-config.bat (example)..."
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
		echo '$(BINARY_NAME).exe'; \
		echo 'pause'; \
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
		echo '  如需修改端口、用户名或密码:'; \
		echo '  1. 编辑 start-with-config.bat'; \
		echo '  2. 修改 set SERVER_PORT=xxx 等变量'; \
		echo '  3. 保存后双击运行 start-with-config.bat'; \
		echo ''; \
		echo '环境变量:'; \
		echo '  SERVER_PORT       - 服务端口 (默认 8080)'; \
		echo '  ADMIN_USERNAME    - 管理员用户名'; \
		echo '  ADMIN_PASSWORD    - 管理员密码'; \
		echo '  PDF_EXTRACTOR_URL - PDF 解析服务地址'; \
		echo '  STORAGE_DIR       - 文件存储目录'; \
		echo '  DATABASE_PATH     - 数据库文件路径'; \
		echo ''; \
		echo '数据目录:'; \
		echo '  data/library.db       - 数据库'; \
		echo '  data/library/papers/  - PDF 文件'; \
		echo '  data/library/figures/ - 提取图片'; \
		echo ''; \
		echo '注意: 首次运行会自动创建 data 目录'; \
	) > dist/$(BINARY_NAME)-windows-$(VERSION)/README.txt
	
	@echo "Creating ZIP archive..."
	@cd dist && zip -rq $(BINARY_NAME)-windows-$(VERSION).zip $(BINARY_NAME)-windows-$(VERSION)
	
	@echo ""
	@echo "========================================"
	@echo "Package Created Successfully!"
	@echo "========================================"
	@echo "Location: dist/$(BINARY_NAME)-windows-$(VERSION).zip"
	@echo "Size:     $$(du -h dist/$(BINARY_NAME)-windows-$(VERSION).zip | cut -f1)"
	@echo ""
	@echo "Contents:"
	@echo "  - $(BINARY_NAME).exe       (可执行文件)"
	@echo "  - web/                      (前端资源)"
	@echo "  - data/                     (数据目录)"
	@echo "  - start.bat                 (启动脚本)"
	@echo "  - start-with-config.bat     (自定义配置示例)"
	@echo "  - README.txt                (使用说明)"
	@echo "========================================"

# =============================================================================
# Utilities
# =============================================================================

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/ dist/
	@echo "Done."

version:
	@echo "Version: $(VERSION)"

# Show help
help:
	@echo "Available targets:"
	@echo ""
	@echo "  make build          - Build for current platform"
	@echo "  make run            - Run development server"
	@echo "  make test           - Run tests"
	@echo ""
	@echo "  make build-windows  - Build Windows executable only"
	@echo "  make package-windows - Create complete Windows distribution"
	@echo ""
	@echo "  make clean          - Clean build artifacts"
	@echo "  make version        - Show version"
	@echo ""

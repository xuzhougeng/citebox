#!/bin/bash
# Build script for Windows distribution

set -e

BINARY_NAME="citebox"
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S')

echo "========================================"
echo "Building CiteBox for Windows"
echo "Version: $VERSION"
echo "Build Time: $BUILD_TIME"
echo "========================================"

# Create output directories
mkdir -p bin/windows
mkdir -p dist

# Build Windows AMD64 binary
echo "[1/3] Building Windows AMD64 executable..."
GOOS=windows GOARCH=amd64 go build \
    -ldflags "-s -w -X main.Version=$VERSION -X 'main.BuildTime=$BUILD_TIME'" \
    -o bin/windows/${BINARY_NAME}.exe \
    ./cmd/server

echo "[2/3] Preparing distribution package..."
DIST_DIR="dist/${BINARY_NAME}-windows-amd64-${VERSION}"
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

# Copy executable
cp bin/windows/${BINARY_NAME}.exe "$DIST_DIR/"

# Copy web assets
cp -r web "$DIST_DIR/"

# Create data directories
mkdir -p "$DIST_DIR/data/library/papers"
mkdir -p "$DIST_DIR/data/library/figures"

# Copy README
cp README.md "$DIST_DIR/"

# Create start.bat
cat > "$DIST_DIR/start.bat" << 'EOF'
@echo off
chcp 65001 >nul
title CiteBox
echo ========================================
echo  CiteBox
echo  Version: VERSION_PLACEHOLDER
echo ========================================
echo.
echo Starting server...
echo.
echo Default URL: http://localhost:8080
echo Default Account: citebox / citebox123
echo.
echo A browser window will open automatically.
echo Close the "CiteBox" window to stop the server.
echo.
start "CiteBox" citebox.exe
timeout /t 2 /nobreak >nul
start "" http://localhost:8080
EOF

# Replace version in start.bat
sed -i "s/VERSION_PLACEHOLDER/$VERSION/g" "$DIST_DIR/start.bat"

# Create config.bat for environment variables
cat > "$DIST_DIR/config.bat" << 'EOF'
@echo off
chcp 65001 >nul
echo ========================================
echo CiteBox - Configuration
echo ========================================
echo.
echo Current settings (press Enter to keep default):
echo.

set /p PORT="Server port [8080]: "
if "%%PORT%%"=="" set PORT=8080

set /p USERNAME="Admin username [citebox]: "
if "%%USERNAME%%"=="" set USERNAME=citebox

set /p PASSWORD="Admin password [citebox123]: "
if "%%PASSWORD%%"=="" set PASSWORD=citebox123

set /p EXTRACTOR="PDF Extractor URL [optional]: "

echo.
echo Saving configuration...

(
echo @echo off
echo chcp 65001 ^>nul
echo set SERVER_PORT=%PORT%
echo set ADMIN_USERNAME=%USERNAME%
echo set ADMIN_PASSWORD=%PASSWORD%
if not "!EXTRACTOR!"=="" echo set PDF_EXTRACTOR_URL=!EXTRACTOR!
echo echo Configuration loaded.
echo echo.
echo start "CiteBox" citebox.exe
echo timeout /t 2 /nobreak ^>nul
echo start "" http://localhost:%%PORT%%
) > start-with-config.bat

echo.
echo Configuration saved to start-with-config.bat
echo Run that file to start with custom settings.
echo.
pause
EOF

# Create Windows README
cat > "$DIST_DIR/README-Windows.txt" << 'EOF'
================================================================================
                        CiteBox - Windows 版
================================================================================

快速开始
--------
1. 解压后，双击运行 start.bat
2. 浏览器访问 http://localhost:8080
3. 默认账号: citebox / citebox123

配置文件
--------
可设置以下环境变量：
  SERVER_PORT          服务端口（默认 8080）
  ADMIN_USERNAME       管理员用户名
  ADMIN_PASSWORD       管理员密码
  PDF_EXTRACTOR_URL    PDF 解析服务地址
  STORAGE_DIR          存储目录（默认 ./data/library）
  DATABASE_PATH        数据库路径（默认 ./data/library.db）

使用 config.bat 可以交互式创建配置脚本。

目录说明
--------
  data/library.db              SQLite 数据库
  data/library/papers/         存储上传的 PDF 文件
  data/library/figures/        存储提取的图片

注意事项
--------
- 首次运行会自动创建数据库和目录
- 请勿删除 data 目录，否则数据会丢失
- 如需备份，直接复制整个 data 目录即可

================================================================================
EOF

echo "[3/3] Creating ZIP archive..."
cd dist
zip -r "${BINARY_NAME}-windows-amd64-${VERSION}.zip" "${BINARY_NAME}-windows-amd64-${VERSION}"
cd ..

echo ""
echo "========================================"
echo "Build Complete!"
echo "========================================"
echo "Output: dist/${BINARY_NAME}-windows-amd64-${VERSION}.zip"
echo "Size: $(du -h "dist/${BINARY_NAME}-windows-amd64-${VERSION}.zip" | cut -f1)"
echo ""

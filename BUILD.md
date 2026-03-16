# 构建指南 - Windows 分发打包

## 快速开始

### 方法 1: 使用 Makefile (推荐)

```bash
# 创建标准 Windows 包（EXE + web 目录）
make package-windows

# 创建独立 Windows 包（单个 EXE，嵌入前端资源）
make package-windows-standalone

# 创建所有包
make all
```

### 方法 2: 使用脚本

```bash
# 运行打包脚本
./scripts/build-windows.sh
```

### 方法 3: 手动构建

```bash
# 1. 安装依赖
go mod tidy

# 2. 构建 Windows 可执行文件
GOOS=windows GOARCH=amd64 go build -o paper_image_db.exe ./cmd/server

# 3. 创建分发目录
mkdir -p dist/paper_image_db

# 4. 复制文件
cp paper_image_db.exe dist/paper_image_db/
cp -r web dist/paper_image_db/

# 5. 创建启动脚本 (start.bat)
# 6. 打包为 zip
cd dist && zip -r paper_image_db.zip paper_image_db/
```

---

## 构建模式对比

| 模式 | 命令 | 特点 | 适用场景 |
|------|------|------|---------|
| **标准版** | `make package-windows` | EXE + web 目录 | 需要修改前端文件 |
| **独立版** | `make package-windows-standalone` | 单个 EXE 文件 | 简单分发，不可修改前端 |

---

## 标准版构建详情

### 文件结构
```
paper_image_db/
├── paper_image_db.exe    # 可执行文件
├── web/                   # 前端资源（必需）
│   ├── index.html
│   ├── static/
│   └── ...
├── data/                  # 数据目录（自动生成）
│   ├── library.db
│   └── library/
└── start.bat             # 启动脚本
```

### 构建命令
```bash
# 交叉编译
GOOS=windows GOARCH=amd64 go build -o paper_image_db.exe ./cmd/server
```

---

## 独立版构建详情

### 特点
- 使用 `go:embed` 将前端资源嵌入到二进制中
- 用户只需一个 EXE 文件即可运行
- 无法运行时修改前端资源

### 构建命令
```bash
# 使用 standalone build tag
go build -tags standalone -o paper_image_db.exe ./cmd/server
```

### 技术实现
- `main.go` - 标准版（外部 web 目录）
- `main_standalone.go` - 独立版（嵌入资源）
- Build tag `standalone` 控制编译哪个版本

---

## Windows 启动脚本示例

### start.bat
```batch
@echo off
chcp 65001 >nul
title Paper Image Database
echo ========================================
echo  Paper Image Database
echo ========================================
echo.
echo URL: http://localhost:8080
echo Account: wanglab / wanglab789
echo.
paper_image_db.exe
pause
```

### start-with-config.bat（自定义配置）
```batch
@echo off
chcp 65001 >nul
set SERVER_PORT=8080
set ADMIN_USERNAME=admin
set ADMIN_PASSWORD=secret
paper_image_db.exe
pause
```

---

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SERVER_PORT` | 8080 | 服务端口 |
| `ADMIN_USERNAME` | wanglab | 管理员用户名 |
| `ADMIN_PASSWORD` | wanglab789 | 管理员密码 |
| `STORAGE_DIR` | ./data/library | 文件存储目录 |
| `DATABASE_PATH` | ./data/library.db | 数据库文件路径 |
| `PDF_EXTRACTOR_URL` | - | PDF 解析服务 URL |

---

## 故障排除

### 1. 交叉编译错误
```bash
# 确保安装了 mingw-w64（如需要 CGO）
# 或使用禁用 CGO 的方式：
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/server
```

### 2. 前端资源 404
- 标准版：确保 `web/` 目录与 EXE 在同一目录
- 独立版：重新构建，确保 embed 指令正确

### 3. 数据库权限错误
- 确保程序有写入 `data/` 目录的权限
- 首次运行会自动创建数据库和目录

---

## 分发建议

### 给普通用户
1. 使用 **独立版** (`package-windows-standalone`)
2. 提供简单的 `双击运行` 说明
3. 打包为 ZIP 格式（Windows 自带解压支持）

### 给高级用户
1. 使用 **标准版** (`package-windows`)
2. 允许用户修改前端样式或界面
3. 提供更灵活的配置方式

---

## 构建输出

构建完成后，`dist/` 目录下会生成：

```
dist/
├── paper_image_db-windows-amd64-v1.0.0.zip          # 标准版
└── paper_image_db-windows-amd64-standalone-v1.0.0.zip  # 独立版
```

建议同时提供两个版本，让用户根据需求选择。

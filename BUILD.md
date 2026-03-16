# 构建指南 - 跨平台分发打包

## 快速开始

```bash
# 创建所有平台的包
make package-all

# 或单独构建某个平台
make package-windows    # Windows
make package-darwin     # macOS
make package-linux      # Linux
```

## GitHub Action 发版

推送版本 tag 后会自动触发 GitHub Action，并把三个平台的包挂到对应的 GitHub Release：

```bash
git tag v0.1.0
git push origin v0.1.0
```

产物格式：

- macOS: `citebox-macos-v0.1.0.tar.gz`
- Linux: `citebox-linux-v0.1.0.tar.gz`
- Windows: `citebox-windows-v0.1.0.zip`

---

## 支持的平台

| 平台 | 架构 | 命令 |
|------|------|------|
| Windows | x86_64 | `make package-windows` |
| macOS | Intel (AMD64) | `make package-darwin` |
| macOS | Apple Silicon (ARM64) | `make build-darwin` |
| Linux | x86_64 | `make package-linux` |
| Linux | ARM64 | `make build-linux` |

---

## Windows

### 构建
```bash
make package-windows
```

### 输出文件
```
dist/citebox-windows-{version}.zip
├── citebox.exe          # 可执行文件
├── web/                         # 前端资源
├── data/                        # 数据目录
├── start.bat                    # 启动脚本
├── start-with-config.bat        # 自定义配置示例
└── README.txt                   # 使用说明
```

### 运行方式
**方式1：默认配置**
```batch
双击 start.bat
```

**方式2：自定义配置**
```batch
编辑 start-with-config.bat
双击运行
```

---

## macOS

### 构建
```bash
make package-darwin
```

### 输出文件
```
dist/citebox-darwin-{version}.zip
├── citebox              # 可执行文件 (Intel 版本)
├── web/                         # 前端资源
├── data/                        # 数据目录
├── start.sh                     # 启动脚本
├── start-with-config.sh         # 自定义配置示例
└── README.txt                   # 使用说明
```

### 运行方式
**方式1：默认配置**
```bash
cd citebox-darwin-{version}
chmod +x citebox start.sh
./start.sh
```

**方式2：自定义配置**
```bash
编辑 start-with-config.sh
./start-with-config.sh
```

### Apple Silicon (M1/M2/M3) 用户
默认包包含 Intel 版本，但可在 Apple Silicon Mac 上通过 Rosetta 运行。

如需原生 ARM64 版本：
```bash
make build-darwin
# 使用 bin/darwin/citebox-arm64
```

---

## Linux

### 构建
```bash
make package-linux
```

### 输出文件
```
dist/citebox-linux-{version}.zip
├── citebox              # 可执行文件 (x86_64)
├── web/                         # 前端资源
├── data/                        # 数据目录
├── start.sh                     # 启动脚本
├── start-with-config.sh         # 自定义配置示例
└── README.txt                   # 使用说明
```

### 运行方式
**方式1：前台运行**
```bash
cd citebox-linux-{version}
chmod +x citebox start.sh
./start.sh
```

**方式2：后台运行**
```bash
nohup ./citebox &
```

**方式3：Systemd 服务**
```bash
# 创建服务文件
sudo tee /etc/systemd/system/citebox.service > /dev/null <<EOF
[Unit]
Description=Paper Image Database
After=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=/opt/citebox
ExecStart=/opt/citebox/citebox
Restart=on-failure
Environment=SERVER_PORT=8080
Environment=ADMIN_USERNAME=wanglab
Environment=ADMIN_PASSWORD=wanglab789

[Install]
WantedBy=multi-user.target
EOF

# 启用并启动服务
sudo systemctl enable citebox
sudo systemctl start citebox
```

### ARM64 服务器
默认包包含 x86_64 版本，如需 ARM64：
```bash
make build-linux
# 使用 bin/linux/citebox-arm64
```

---

## 环境变量

所有平台支持相同的环境变量：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SERVER_PORT` | 8080 | 服务端口 |
| `ADMIN_USERNAME` | wanglab | 管理员用户名 |
| `ADMIN_PASSWORD` | wanglab789 | 管理员密码 |
| `STORAGE_DIR` | ./data/library | 文件存储目录 |
| `DATABASE_PATH` | ./data/library.db | 数据库文件路径 |
| `PDF_EXTRACTOR_URL` | - | PDF 解析服务 URL |
| `MAX_UPLOAD_SIZE` | 262144000 | 最大上传大小 (字节) |

---

## 故障排除

### 1. 跨平台编译错误

**问题：** `build constraints exclude all Go files`

**解决：**
```bash
# 确保没有使用平台特定的代码
# 检查 SQLite 驱动是否支持目标平台

# 禁用 CGO（纯 Go 模式）
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/server
```

### 2. macOS 运行权限

**问题：** `cannot be opened because the developer cannot be verified`

**解决：**
```bash
# 方法1：在系统设置中允许
# 系统设置 → 隐私与安全性 → 安全性 → 仍要打开

# 方法2：移除隔离属性
xattr -d com.apple.quarantine citebox

# 方法3：签名（开发者）
codesign -s "Developer ID" citebox
```

### 3. Linux 端口权限

**问题：** `bind: permission denied` (端口 < 1024)

**解决：**
```bash
# 使用非特权端口
export SERVER_PORT=8080

# 或使用 authbind
sudo apt install authbind
authbind --deep ./citebox
```

### 4. 数据库锁定

**问题：** `database is locked`

**解决：**
```bash
# 检查是否有多个实例在运行
lsof data/library.db

# 删除锁定文件（确保没有运行中的实例）
rm -f data/library.db-shm data/library.db-wal
```

---

## 开发构建

### 当前平台
```bash
make build
```

### 特定平台/架构
```bash
# Windows AMD64
GOOS=windows GOARCH=amd64 go build -o citebox.exe ./cmd/server

# macOS Intel
GOOS=darwin GOARCH=amd64 go build -o citebox ./cmd/server

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o citebox ./cmd/server

# Linux x86_64
GOOS=linux GOARCH=amd64 go build -o citebox ./cmd/server

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o citebox ./cmd/server
```

---

## 减小二进制体积

```bash
# 使用 -ldflags 去除调试信息
go build -ldflags "-s -w" -o citebox ./cmd/server

# 使用 UPX 压缩（可选）
upx --best citebox
```

---

## 发布检查清单

- [ ] 版本号正确 (`make version`)
- [ ] 所有平台构建成功 (`make package-all`)
- [ ] Windows 包能正常启动
- [ ] macOS 包能正常启动
- [ ] Linux 包能正常启动
- [ ] 默认账号能登录
- [ ] 能上传 PDF
- [ ] 数据持久化正常

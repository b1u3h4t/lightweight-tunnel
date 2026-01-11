# macOS 完全支持实现计划

## 当前状态分析

### ✅ 已实现的功能

1. **Raw Socket 支持** (`pkg/rawsocket/rawsocket.go`)
   - macOS 特定的 Raw Socket 实现
   - 使用 libpcap 作为备选接收方式
   - 分离发送和接收 socket（macOS 要求）
   - 不设置 IP_HDRINCL（让内核构建 IP 头）

2. **TUN 设备支持** (`pkg/tunnel/tun.go`)
   - 通过 CGO 调用 C 代码创建 utun 设备
   - macOS 特定的设备创建逻辑
   - 正确的阻塞模式设置

3. **TUN 配置** (`pkg/tunnel/tunnel.go`)
   - macOS 使用 `ifconfig` 命令
   - 支持点对点接口配置
   - 自动查找实际 utun 接口名称

4. **路由管理** (`pkg/tunnel/tunnel.go`)
   - macOS 使用 `route` 命令替代 Linux 的 `ip route`
   - 支持多种路由添加格式（带前缀、带掩码、指定接口）
   - 删除旧路由以避免冲突

5. **协议头处理**
   - 正确处理 macOS utun 的 4 字节协议族头（AF_INET = 2）
   - 读写时自动添加/移除协议头

6. **iptables 集成** (`pkg/iptables/iptables.go`)
   - macOS 上正确跳过 iptables（macOS 使用 pf）
   - 日志提示 macOS 不需要 iptables

### ❌ 缺失/不完整的功能

1. **文档缺失**
   - README.md 没有 macOS 平台徽章
   - 没有 macOS 安装说明
   - 没有 macOS 故障排除指南

2. **构建系统不完整**
   - Makefile 只支持 Linux systemd 服务
   - 没有 macOS launchd 服务支持

3. **CI/CD 不完整**
   - GitHub Actions 只构建 Linux 版本
   - 没有 macOS 构建目标

4. **防火墙文档缺失**
   - 没有 macOS pf 防火墙配置说明

5. **构建说明不完整**
   - 没有记录 macOS 特定的构建要求（CGO、libpcap）

---

## 实施计划

### 任务 1: 更新 README.md - 添加 macOS 平台徽章和系统要求

**文件**: `README.md`

**更改**:
```markdown
# 将
[![Platform](https://img.shields.io/badge/Platform-Linux-green.svg)](https://www.linux.org/)

# 改为
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-green.svg)](https://www.linux.org/)
```

**系统要求表更新**:
```markdown
| 项目 | 要求 |
|-----|------|
| 操作系统 | Linux (内核 2.6+) 或 macOS 10.15+ (Catalina 或更高版本) |
| 权限 | Root（Raw Socket 和 TUN 设备必需） |
| Go 版本 | Go 1.19+（仅编译时需要） |
| macOS 依赖 | libpcap (通过 Homebrew 安装) |
```

### 任务 2: 添加 macOS 安装说明

**文件**: `README.md`

**新增章节**:
```markdown
#### 方法 2: macOS 安装

在 macOS 上编译和安装：

```bash
# 安装依赖
brew install libpcap

# 克隆仓库
git clone https://github.com/openbmx/lightweight-tunnel.git
cd lightweight-tunnel

# 编译（需要 CGO 支持）
CGO_ENABLED=1 go build -o lightweight-tunnel ./cmd/lightweight-tunnel

# 可选：安装到系统路径
sudo cp lightweight-tunnel /usr/local/bin/

# 验证
./lightweight-tunnel -v
```

#### Apple Silicon (M1/M2/M3)

对于 Apple Silicon 设备：

```bash
# 指定 ARM64 架构编译
CGO_ENABLED=1 GOARCH=arm64 go build -o lightweight-tunnel ./cmd/lightweight-tunnel
```
```

### 任务 3: 添加 macOS 故障排除章节

**文件**: `README.md`

**新增章节**:
```markdown
### macOS 特定问题

#### Q10: macOS 编译失败 - "pcap.h not found"

**错误信息**:
```
# github.com/google/gopacket/pcap
./pcap.go:35:11: fatal error: pcap.h: No such file or directory
```

**解决方案**:
```bash
# 安装 libpcap
brew install libpcap

# 确保使用 CGO 编译
CGO_ENABLED=1 go build ./cmd/lightweight-tunnel
```

#### Q11: macOS 权限错误 "operation not permitted"

**错误信息**:
```
failed to create raw socket: operation not permitted (需要root权限)
```

**解决方案**:
```bash
# 使用 sudo 运行
sudo ./lightweight-tunnel -m server -l 0.0.0.0:9000 -t 10.0.0.1/24

# 或授予 capabilities（需要开发模式）
sudo codesign --entitlements entitlements.plist --force --sign - ./lightweight-tunnel
```

#### Q12: macOS 上 Raw Socket 限制

macOS 对 Raw Socket 有更严格的限制：

- ⚠️ 内核可能处理部分 TCP 包，导致接收失败
- ⚠️ 可能需要使用 libpcap 作为备选接收方式
- ✅ 已自动实现 libpcap 回退机制

**工作原理**:
1. 首先尝试使用 Raw Socket 接收
2. 如果失败（内核处理了包），自动切换到 libpcap
3. libpcap 直接从网卡捕获原始数据包

#### Q13: macOS 防火墙配置

macOS 使用 pf (packet filter) 防火墙，而不是 iptables。

**查看 pf 状态**:
```bash
sudo pfctl -s info
```

**允许隧道流量（如果需要）**:
```bash
# 创建临时规则文件
cat > /etc/pf.anchors/lightweight-tunnel << EOF
# 允许隧道端口 9000
pass in quick proto tcp from any to any port 9000
pass out quick proto tcp from any to any port 9000
EOF

# 加载规则
sudo pfctl -e -f /etc/pf.anchors/lightweight-tunnel
```

**禁用防火墙（测试用）**:
```bash
sudo pfctl -d
```

#### Q14: macOS TUN 设备名称

macOS 的 utun 设备名称由系统自动分配（utun0, utun1, 等）。

**查看已分配的 utun 设备**:
```bash
ifconfig | grep utun
```

**指定设备名**:
```bash
# 可以指定起始编号，但系统可能分配不同的编号
sudo ./lightweight-tunnel -m server -tun-name utun5
```
```

### 任务 4: 添加 macOS launchd 服务支持

**文件**: `Makefile`

**新增目标**:
```makefile
## install-service-macos: Install macOS launchd service (CONFIG_PATH=/path/to/config.json)
install-service-macos:
	@set -e; \
	if [ -z "$(CONFIG_PATH)" ]; then \
		echo "ERROR: CONFIG_PATH is required. Example: make install-service-macos CONFIG_PATH=/etc/lightweight-tunnel/config.json"; \
		exit 1; \
	fi; \
	if [ "$(CONFIG_PATH)" = "${CONFIG_PATH#/}" ]; then \
		echo "ERROR: CONFIG_PATH must be an absolute path."; \
		exit 1; \
	fi; \
	if [ ! -x "$(GOBIN)/$(BINARY_NAME)" ]; then \
		echo "Binary not found, building $(BINARY_NAME)..."; \
		$(MAKE) build; \
	fi; \
	echo "Installing binary to $(INSTALL_BIN_DIR)..."; \
	sudo install -m 755 $(GOBIN)/$(BINARY_NAME) $(INSTALL_BIN_DIR)/$(BINARY_NAME); \
	echo "Creating launchd plist for user mode..."; \
	mkdir -p $(HOME)/Library/LaunchAgents; \
	sed 's|{{EXEC_PATH}}|$(INSTALL_BIN_DIR)/$(BINARY_NAME)|g; s|{{CONFIG_PATH}}|$(CONFIG_PATH)|g' contrib/$(BINARY_NAME).plist.template > $(HOME)/Library/LaunchAgents/$(BINARY_NAME).plist; \
	echo "Loading launchd service..."; \
	launchctl load $(HOME)/Library/LaunchAgents/$(BINARY_NAME).plist; \
	echo "Service installed. Logs: ~/Library/Logs/lightweight-tunnel.log"; \
	echo "To uninstall: launchctl unload $(HOME)/Library/LaunchAgents/$(BINARY_NAME).plist"

## uninstall-service-macos: Uninstall macOS launchd service
uninstall-service-macos:
	@echo "Unloading launchd service..."; \
	launchctl unload $(HOME)/Library/LaunchAgents/$(BINARY_NAME).plist 2>/dev/null || true; \
	rm -f $(HOME)/Library/LaunchAgents/$(BINARY_NAME).plist; \
	echo "Service uninstalled."
```

### 任务 5: 创建 macOS launchd plist 模板

**新文件**: `contrib/lightweight-tunnel.plist.template`

**内容**:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.lightweight-tunnel</string>

  <key>ProgramArguments</key>
  <array>
    <string>{{EXEC_PATH}}</string>
    <string>-c</string>
    <string>{{CONFIG_PATH}}</string>
  </array>

  <key>RunAtLoad</key>
  <true/>

  <key>KeepAlive</key>
  <true/>

  <key>StandardOutPath</key>
  <string>/tmp/lightweight-tunnel.log</string>

  <key>StandardErrorPath</key>
  <string>/tmp/lightweight-tunnel.err</string>

  <key>WorkingDirectory</key>
  <string>/var/run/lightweight-tunnel</string>

  <key>UserName</key>
  <string>root</string>
</dict>
</plist>
```

### 任务 6: 更新 GitHub Actions 工作流

**文件**: `.github/workflows/build.yml`

**更新内容**:
```yaml
name: Build Lightweight Tunnel

on:
  workflow_dispatch:
  push:
    branches: [ "main" ]

jobs:
  build-linux:
    runs-on: ubuntu-latest

    steps:
    - name: Checkout code
      uses: actions/checkout@v4

    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.21'

    - name: Build Binary
      run: |
        go build -v -o lightweight-tunnel-linux-amd64 ./cmd/lightweight-tunnel

    - name: Upload Artifact
      uses: actions/upload-artifact@v4
      with:
        name: lightweight-tunnel-linux-amd64
        path: lightweight-tunnel-linux-amd64

  build-macos-amd64:
    runs-on: macos-latest

    steps:
    - name: Checkout code
      uses: actions/checkout@v4

    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.21'

    - name: Install dependencies
      run: |
        brew install libpcap

    - name: Build Binary
      run: |
        CGO_ENABLED=1 go build -v -o lightweight-tunnel-macos-amd64 ./cmd/lightweight-tunnel

    - name: Upload Artifact
      uses: actions/upload-artifact@v4
      with:
        name: lightweight-tunnel-macos-amd64
        path: lightweight-tunnel-macos-amd64

  build-macos-arm64:
    runs-on: macos-latest

    steps:
    - name: Checkout code
      uses: actions/checkout@v4

    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.21'

    - name: Install dependencies
      run: |
        brew install libpcap

    - name: Build Binary
      run: |
        CGO_ENABLED=1 GOARCH=arm64 go build -v -o lightweight-tunnel-macos-arm64 ./cmd/lightweight-tunnel

    - name: Upload Artifact
      uses: actions/upload-artifact@v4
      with:
        name: lightweight-tunnel-macos-arm64
        path: lightweight-tunnel-macos-arm64

  release:
    needs: [build-linux, build-macos-amd64, build-macos-arm64]
    runs-on: ubuntu-latest
    if: startsWith(github.ref, 'refs/tags/')

    steps:
    - name: Download all artifacts
      uses: actions/download-artifact@v4

    - name: Create Release
      uses: softprops/action-gh-release@v1
      with:
        files: |
          lightweight-tunnel-linux-amd64/*
          lightweight-tunnel-macos-amd64/*
          lightweight-tunnel-macos-arm64/*
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### 任务 7: 添加 macOS 防火墙配置文档

**文件**: `README.md` 或新建 `docs/firewall-macos.md`

**内容**:
```markdown
## macOS 防火墙配置

### 概述

macOS 使用 pf (Packet Filter) 作为防火墙，而不是 Linux 的 iptables。
Lightweight Tunnel 的 Raw Socket 模式在 macOS 上通常不需要额外的防火墙配置，但如果遇到连接问题，可能需要调整 pf 规则。

### 检查 pf 状态

```bash
# 查看 pf 状态信息
sudo pfctl -s info

# 查看当前活动规则
sudo pfctl -s rules
```

### 允许隧道流量

创建自定义规则文件：

```bash
# 创建规则目录和文件
sudo mkdir -p /etc/pf.anchors
sudo nano /etc/pf.anchors/lightweight-tunnel
```

添加以下内容：

```
# Lightweight Tunnel 规则
# 允许隧道端口（根据实际端口修改）
pass in quick proto tcp from any to any port 9000
pass out quick proto tcp from any to any port 9000

# 允许 P2P UDP 端口（如果使用 P2P）
pass in quick proto udp from any to any port 19000
pass out quick proto udp from any to any port 19000
```

加载规则：

```bash
# 语法检查
sudo pfctl -nf /etc/pf.anchors/lightweight-tunnel

# 加载到系统
sudo pfctl -f /etc/pf.anchors/lightweight-tunnel

# 启用 pf
sudo pfctl -e
```

### 永久启用 pf

编辑 `/etc/pf.conf`：

```bash
sudo nano /etc/pf.conf
```

添加：

```
# 引用 Lightweight Tunnel 规则
anchor "lightweight-tunnel"

# 其他规则...
```

### 临时禁用 pf（测试用）

```bash
# 禁用 pf
sudo pfctl -d

# 重新启用
sudo pfctl -e
```

### 查看日志

```bash
# 查看 pf 日志（需要先启用日志记录）
sudo pfctl -s info
```
```

### 任务 8: 添加 macOS 构建说明

**文件**: `README.md`

**新增章节**:
```markdown
## macOS 构建说明

### 依赖要求

- Go 1.19 或更高版本
- Xcode Command Line Tools（用于编译 C 代码）
- libpcap（用于 libpcap 回退支持）

### 安装依赖

```bash
# 安装 Xcode Command Line Tools（如果尚未安装）
xcode-select --install

# 安装 libpcap
brew install libpcap

# 验证 libpcap
brew list libpcap
```

### 编译选项

#### 标准（Intel Mac）
```bash
CGO_ENABLED=1 go build -o lightweight-tunnel ./cmd/lightweight-tunnel
```

#### Apple Silicon (M1/M2/M3)
```bash
CGO_ENABLED=1 GOARCH=arm64 go build -o lightweight-tunnel ./cmd/lightweight-tunnel
```

#### 优化编译
```bash
# 减小二进制大小
CGO_ENABLED=1 go build -ldflags "-s -w" -o lightweight-tunnel ./cmd/lightweight-tunnel

# 启用所有优化
CGO_ENABLED=1 go build -ldflags "-s -w" -gcflags="-l=4" -o lightweight-tunnel ./cmd/lightweight-tunnel
```

### CGO 说明

macOS 版本需要 CGO，因为：
1. 创建 utun 设备需要调用 macOS 特定的 C API
2. libpcap 绑定需要 CGO

### 常见编译问题

#### pcap.h not found
```bash
# 解决方案：安装 libpcap
brew install libpcap
```

#### linker command failed
```bash
# 解决方案：安装 Xcode Command Line Tools
xcode-select --install
```

### 架构兼容性

| 架构 | GOARCH | 设备 |
|-----|--------|------|
| Intel x86_64 | amd64 | Intel Mac (2019 及更早) |
| Apple Silicon | arm64 | M1, M2, M3 Mac (2020 及之后) |

### 交叉编译

在 Linux 上编译 macOS 版本（需要 macOS SDK）：

```bash
# 需要安装 osxcross 工具链
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC=o64-clang go build ./cmd/lightweight-tunnel
```
```

---

## 实施优先级

| 任务 | 优先级 | 预计时间 | 风险 |
|-----|--------|----------|------|
| 1. README 平台徽章 | 高 | 5 分钟 | 低 |
| 2. macOS 安装说明 | 高 | 15 分钟 | 低 |
| 3. macOS 故障排除 | 中 | 20 分钟 | 低 |
| 4. Makefile launchd | 高 | 30 分钟 | 中 |
| 5. launchd plist 模板 | 高 | 10 分钟 | 低 |
| 6. GitHub Actions 更新 | 高 | 20 分钟 | 低 |
| 7. 防火墙文档 | 中 | 15 分钟 | 低 |
| 8. 构建说明 | 中 | 15 分钟 | 低 |

**总计**: 约 2 小时

---

## 测试计划

### 单元测试
- [ ] Raw Socket 创建（macOS）
- [ ] TUN 设备创建（utun）
- [ ] 路由添加/删除
- [ ] libpcap 接收器

### 集成测试
- [ ] 服务端启动
- [ ] 客户端连接
- [ ] 数据传输
- [ ] 自动重连

### 服务测试
- [ ] launchd 安装
- [ ] 服务启动/停止
- [ ] 日志查看

### 平台测试
- [ ] macOS Intel (amd64)
- [ ] macOS Apple Silicon (arm64)
- [ ] macOS Big Sur (11.x)
- [ ] macOS Monterey (12.x)
- [ ] macOS Ventura (13.x)
- [ ] macOS Sonoma (14.x)

---

## 完成标准

- [ ] README.md 更新完成，包含所有 macOS 说明
- [ ] Makefile 支持 macOS launchd 服务安装
- [ ] launchd plist 模板创建并测试
- [ ] GitHub Actions 构建所有平台
- [ ] 防火墙配置文档完成
- [ ] 所有测试通过
- [ ] 代码提交到 main 分支

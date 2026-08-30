# Service Manager

[English](#english) | [中文](#中文)

---

## English

A cross-platform service management daemon written in Go. It runs as a native system service (Windows SCM / Linux systemd), manages child processes with automatic restart, and provides a web UI for configuration, log viewing, and service control.

### Features

- **Native service integration** — registers as a Windows SCM service or Linux systemd unit
- **Multi-service management** — run multiple child processes from a single daemon
- **Automatic restart** — configurable restart policies (always / on-failure / no)
- **Web UI** — dark-themed SPA for managing services, viewing live logs, and editing config
- **SSE log streaming** — real-time log tail via Server-Sent Events
- **YAML configuration** — simple config file, hot-reload on file change
- **Minimal dependencies** — pure Go, CGO-free; the only external module is `golang.org/x/sys`, used solely for Windows-native APIs (Job Objects, FreeConsole)

### Project Structure

```
cmd/service-manager/main.go      Entry point, CLI flags, service registration
internal/config/                  YAML config parser and file watcher
internal/logger/                  Dual-output logger (stderr + file)
internal/process/                 Process supervisor with restart logic
internal/manager/                 Orchestrator tying config, supervisor, logger
internal/platform/                Platform-specific service registration
internal/api/                     HTTP server with REST + SSE endpoints
web/dist/index.html               Frontend SPA
configs/service-manager.yaml      Example config
scripts/                          Build scripts
```

### Build

```bash
# Linux
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o service-manager ./cmd/service-manager

# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o service-manager.exe ./cmd/service-manager
```

On Windows the executable is a console app, so `--help` can display usage in a terminal. Every other run detaches from the console inside the program (`FreeConsole`), so no console window is left open; pass `-console` to keep the console attached.

### Usage

```
service-manager [options]

Options:
  -scm              [Windows] register and run as a system service (register and start it when launched under SCM)
  -console          keep the console attached (do not detach via FreeConsole)
  -install          force re-register the system service
  -uninstall        unregister and remove the system service
  -status           show service registration status
  -config string    path to config file (default: <exe_dir>/config.yaml)
  -port int         override web server port
```

**Run directly (default, no system service registration):**

```bash
./service-manager
./service-manager -config /path/to/config.yaml
./service-manager -port 8080
```

**Run as daemon (system service mode):**

```bash
# Linux: register with systemd, then start. The unit launches the binary
# without arguments and the systemd launch is detected automatically
# (INVOCATION_ID), so no flag is needed - there is no -scm on Linux.
sudo ./service-manager -install
sudo systemctl start service-manager

# Windows: register with SCM, then start it. The service entry launches the
# exe without arguments; the SCM parent process is detected automatically.
service-manager.exe -install
sc start service-manager

# Windows: register and run in one step from a console
service-manager.exe -scm
```

The web UI will be available at `http://localhost:7070` (or the configured port).

**Register as a system service:**

```bash
# Linux (systemd)
sudo ./service-manager -install
sudo systemctl start service-manager

# Windows (SCM)
service-manager.exe -install
```

**Check registration status:**

```bash
./service-manager -status
```

**Unregister:**

```bash
# Linux
sudo ./service-manager -uninstall

# Windows
service-manager.exe -uninstall
```

### Configuration

Config file is `config.yaml` next to the executable. Example:

```yaml
web_port: 7070
log_dir: ./logs

services:
  - name: helix-agent
    executable: C:\Python312\python.exe
    arguments:
      - C:\Helix\main.py
      - --port
      - "8000"
    working_directory: C:\Helix
    environment:
      HELIX_ENV: production
    restart:
      policy: always
      delay_seconds: 5
      max_retries: 10

  - name: monitor
    executable: /usr/local/bin/monitor
    arguments:
      - --config
      - /etc/monitor.yaml
    working_directory: /opt/monitor
    environment:
      MONITOR_ENV: production
    restart:
      policy: on-failure
      delay_seconds: 3
      max_retries: 5
```

**Restart policies:**

| Policy | Behavior |
|--------|----------|
| `always` | Always restart after exit |
| `on-failure` | Restart only on non-zero exit |
| `no` | Never restart |

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/services` | List all services and status |
| GET | `/api/services/{name}` | Get single service details |
| POST | `/api/services/{name}/start` | Start a service |
| POST | `/api/services/{name}/stop` | Stop a service |
| POST | `/api/services/{name}/restart` | Restart a service |
| GET | `/api/services/{name}/logs?lines=100` | Get last N log lines |
| GET | `/api/services/{name}/logs/stream` | SSE log stream |
| GET | `/api/manager/logs?lines=100` | Manager log lines |
| GET | `/api/manager/logs/stream` | SSE manager log stream |
| GET | `/api/config` | Get current config |
| PUT | `/api/config` | Update config and reload |
| PUT | `/api/config/port` | Update web port |
| GET | `/api/health` | Health check |

### Deployment Directory Structure

```
C:\HelixServiceManager\          # or /opt/service-manager/
├── service-manager(.exe)
├── config.yaml
├── services/                    # optional: per-service configs
├── logs/
│   ├── manager.log
│   ├── helix-agent.log
│   └── monitor.log
└── web/
    ├── index.html
    └── assets/
```

### Web UI

The built-in web interface provides:

- **Services tab** — view status, start/stop/restart individual services
- **Logs tab** — live log streaming with auto-scroll, select service to view
- **Configuration tab** — edit YAML config in-browser, update web port

---

## 中文

跨平台服务管理守护进程，基于 Go 开发。作为原生系统服务运行（Windows SCM / Linux systemd），管理子进程并支持自动重启，提供 Web 界面进行配置、日志查看和服务控制。

### 功能特性

- **原生服务集成** — 注册为 Windows SCM 服务或 Linux systemd 单元
- **多服务管理** — 单个守护进程管理多个子进程
- **自动重启** — 可配置重启策略（always / on-failure / no）
- **Web 界面** — 暗色主题 SPA，管理服务、查看实时日志、编辑配置
- **SSE 日志流** — 通过 Server-Sent Events 实时推送日志
- **YAML 配置** — 简洁配置文件，文件变更自动热重载
- **精简依赖** — 纯 Go 实现，无需 CGO；唯一外部模块 `golang.org/x/sys`，仅用于 Windows 原生 API（Job Object、FreeConsole）

### 项目结构

```
cmd/service-manager/main.go      入口，CLI 参数，服务注册
internal/config/                  YAML 配置解析与文件监听
internal/logger/                  双输出日志（stderr + 文件）
internal/process/                 进程管理器，带重启逻辑
internal/manager/                 编排器，串联配置、进程管理、日志
internal/platform/                平台特定的服务注册
internal/api/                     HTTP 服务器，REST + SSE 接口
web/dist/index.html               前端 SPA
configs/service-manager.yaml      配置示例
scripts/                          构建脚本
```

### 构建

```bash
# Linux
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o service-manager ./cmd/service-manager

# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o service-manager.exe ./cmd/service-manager
```

Windows 下编译为控制台程序，`--help` 可以在终端中显示帮助信息；其余运行模式在程序内部通过 `FreeConsole` 脱离控制台，因此不会残留控制台窗口；使用 `-console` 参数则保持控制台附着。

### 用法

```
service-manager [选项]

选项:
  -scm              [仅 Windows] 以系统服务方式运行（由 SCM 启动时注册并运行服务）
  -console          保持控制台附着（不调用 FreeConsole 脱离）
  -install          强制重新注册系统服务
  -uninstall        注销并移除系统服务
  -status           显示服务注册状态
  -config string    配置文件路径（默认：<可执行文件目录>/config.yaml）
  -port int         覆盖 Web 服务端口
```

**直接运行（默认模式，不注册系统服务）：**

```bash
./service-manager
./service-manager -config /path/to/config.yaml
./service-manager -port 8080
```

**作为守护进程运行（系统服务模式）：**

```bash
# Linux: 注册 systemd 单元后启动。unit 无参数启动二进制，
# systemd 启动方式自动检测（INVOCATION_ID），无需任何参数 —— Linux 没有 -scm。
sudo ./service-manager -install
sudo systemctl start service-manager

# Windows: 注册 SCM 服务后启动。SCM 无参数启动 exe，
# 通过父进程（services.exe）自动检测。
service-manager.exe -install
sc start service-manager

# Windows: 或在控制台一步完成注册并运行
service-manager.exe -scm
```

Web 界面默认访问地址：`http://localhost:7070`

**注册为系统服务：**

```bash
# Linux (systemd)
sudo ./service-manager -install
sudo systemctl start service-manager

# Windows (SCM)
service-manager.exe -install
```

**查看注册状态：**

```bash
./service-manager -status
```

**注销服务：**

```bash
# Linux
sudo ./service-manager -uninstall

# Windows
service-manager.exe -uninstall
```

### 配置文件

配置文件位于可执行文件同目录下的 `config.yaml`，示例：

```yaml
web_port: 7070
log_dir: ./logs

services:
  - name: helix-agent
    executable: C:\Python312\python.exe
    arguments:
      - C:\Helix\main.py
      - --port
      - "8000"
    working_directory: C:\Helix
    environment:
      HELIX_ENV: production
    restart:
      policy: always
      delay_seconds: 5
      max_retries: 10

  - name: monitor
    executable: /usr/local/bin/monitor
    arguments:
      - --config
      - /etc/monitor.yaml
    working_directory: /opt/monitor
    environment:
      MONITOR_ENV: production
    restart:
      policy: on-failure
      delay_seconds: 3
      max_retries: 5
```

**重启策略：**

| 策略 | 行为 |
|------|------|
| `always` | 进程退出后始终重启 |
| `on-failure` | 仅在非零退出码时重启 |
| `no` | 不自动重启 |

### API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/services` | 列出所有服务及状态 |
| GET | `/api/services/{name}` | 获取单个服务详情 |
| POST | `/api/services/{name}/start` | 启动服务 |
| POST | `/api/services/{name}/stop` | 停止服务 |
| POST | `/api/services/{name}/restart` | 重启服务 |
| GET | `/api/services/{name}/logs?lines=100` | 获取最近 N 行日志 |
| GET | `/api/services/{name}/logs/stream` | SSE 实时日志流 |
| GET | `/api/manager/logs?lines=100` | 管理器日志 |
| GET | `/api/manager/logs/stream` | SSE 管理器日志流 |
| GET | `/api/config` | 获取当前配置 |
| PUT | `/api/config` | 更新配置并重载 |
| PUT | `/api/config/port` | 更新 Web 端口 |
| GET | `/api/health` | 健康检查 |

### 部署目录结构

```
C:\HelixServiceManager\          # 或 /opt/service-manager/
├── service-manager(.exe)
├── config.yaml
├── services/                    # 可选：各服务独立配置
├── logs/
│   ├── manager.log
│   ├── helix-agent.log
│   └── monitor.log
└── web/
    ├── index.html
    └── assets/
```

### Web 界面

内置 Web 界面提供：

- **Services 标签页** — 查看状态，启动/停止/重启各服务
- **Logs 标签页** — 实时日志流，支持自动滚动，选择查看不同服务
- **Configuration 标签页** — 在线编辑 YAML 配置，修改 Web 端口

# GPT Proxy

`GPT Proxy` 是一个基于 Go 的本地代理项目，用来把上游 OpenAI 兼容 `responses` 接口包装成本地可用的 OpenAI 风格与 Claude 风格接口。

这个仓库现在拆分成三个可独立打包、独立发布的版本：

- `web/`：网页版控制台
- `cli/`：命令行版本
- `desktop/`：Windows 桌面版，基于 WebView2

三者共用同一套核心能力，统一放在 `shared/` 中。

## 项目功能

当前代理暴露的接口包括：

- `GET /v1/models`
- `POST /v1/responses`
- `POST /v1/messages`
- `POST /v1/chat/completions`

适合的使用场景：

- 把各种本地工具统一指向一个本地代理地址
- 在接入客户端之前，先测试上游 OpenAI 兼容服务是否可用
- 同时提供 OpenAI 风格和 Claude 风格接口
- 根据使用环境选择浏览器版、CLI 版或桌面版

## 仓库结构

```text
project/
  cli/       独立命令行程序
  desktop/   Windows 桌面程序，使用 WebView2
  shared/    公共核心库、日志、生命周期和共用控制台前端
  web/       浏览器控制台
```

## `shared/` 是做什么的

`shared/` 是整个项目的公共核心层，主要职责是避免 `web`、`cli`、`desktop` 三套代码重复维护相同逻辑。

它目前负责：

- 代理路由与协议适配
- 启动和停止本地代理
- 启动和停止控制面板服务
- 运行日志收集与流式输出
- 状态快照
- `web` 与 `desktop` 共用的控制台前端资源

简单说：

- `cli/` 只负责命令行交互
- `web/` 只负责浏览器入口
- `desktop/` 只负责桌面窗口壳
- 真正的代理能力和共用页面都在 `shared/`

## 各版本说明

### Web

`web/` 提供浏览器控制台，用来测试上游地址、启动和停止代理、查看代理地址以及实时日志。

参考：[web/README.md](/D:/App/tool/Go/project/web/README.md)

### CLI

`cli/` 提供纯命令行版本。支持启动参数直启，也支持未传参数时进入交互模式。

参考：[cli/README.md](/D:/App/tool/Go/project/cli/README.md)

### Desktop

`desktop/` 是 Windows 桌面版。它复用了和网页版相同的控制台前端，通过 WebView2 嵌入到桌面窗口中，不再维护单独的原生表单界面。

参考：[desktop/README.md](/D:/App/tool/Go/project/desktop/README.md)

## 快速启动

### 启动 Web 版

```powershell
cd web
go run .
```

### 启动 CLI 版

```powershell
cd cli
go run .
```

CLI 直接启动示例：

```powershell
go run . -upstream https://api.openai.com -host 127.0.0.1 -port 3000
```

### 启动 Desktop 版

```powershell
cd desktop
go run .
```

## 构建

三个版本分别独立构建：

```powershell
cd web
go build -o gpt-proxy-web.exe .
```

```powershell
cd cli
go build -o gpt-proxy-cli.exe .
```

```powershell
cd desktop
go build -buildvcs=false -o gpt-proxy-desktop.exe .
```

## 测试

按模块分别执行测试：

```powershell
cd shared
go test ./...
```

```powershell
cd cli
go test ./...
```

```powershell
cd web
go test ./...
```

```powershell
cd desktop
go test ./...
```

## 运行说明

- `desktop/` 依赖 Microsoft Edge WebView2 Runtime
- `web/` 与 `desktop/` 共用 `shared/controlui/` 中的嵌入式前端
- `cli/` 不包含图形控制台，只在终端打印代理地址和运行日志

## 公共环境变量

这些环境变量由公共核心 `shared/` 使用：

- `CONTROL_ADDR`
  - 控制台监听地址
  - 默认：`127.0.0.1:0`
- `PROXY_BIND_HOST`
  - 本地代理监听主机
  - 默认：`127.0.0.1`
- `DISPLAY_HOST`
  - 展示给用户的代理主机名
  - 默认跟随 `PROXY_BIND_HOST`
- `HTTP_TIMEOUT_SECONDS`
  - 上游请求超时时间，单位秒
  - 默认：`60`

## 常见问题

### 1. Go module cache 权限错误

如果看到类似：

```text
go: could not create module cache
```

通常要先检查 Go 环境变量。若 `GOPATH` 错误地指向了 `C:\Program Files\Go\bin`，就很容易触发权限问题。更合理的配置一般应指向用户目录，例如：

```powershell
go env -w GOPATH=C:\Users\<你的用户名>\go
```

### 2. 桌面版无法启动窗口

如果桌面版无法正常创建窗口，先确认系统已经安装并启用了 WebView2 Runtime。

## 备注

- 当前仓库结构是刻意拆开的，目的是让 `web`、`cli`、`desktop` 可以分别打包和发布
- 如果你要修改代理行为，通常应该优先改 `shared/`，而不是在三个版本里各改一遍

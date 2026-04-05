# GPT Proxy Web

GPT Proxy Web 是当前项目的网页控制台版本，用于：

- 将 Claude Messages 请求转换为 OpenAI Responses 请求
- 将 OpenAI Chat Completions 请求转换为 OpenAI Responses 请求
- 再将结果转换回对应协议格式返回

## 当前接口

- `POST /v1/messages`
- `POST /v1/chat/completions`

## 功能

- 支持 Claude Messages 协议转换
- 支持 OpenAI Chat Completions 协议转换
- 支持流式和非流式响应
- 支持网页控制台配置上游 Host、Key、测试模型和代理端口
- 支持自动打开浏览器进入控制台
- 保留控制台窗口，关闭窗口即可结束进程
- 支持查看代理运行日志

## 启动

在 `web/` 目录执行：

```powershell
go run .
```

当前默认行为：

- 控制台监听 `127.0.0.1:0`
- 启动时自动分配随机控制台端口
- 启动后自动打开默认浏览器

## 打包为控制台 EXE

在 `web/` 目录执行：

```powershell
.\build-web.ps1
```

或者直接执行：

```powershell
go build -o gpt-proxy-web.exe .
```

说明：

- 运行后会自动打开默认浏览器
- 同时保留黑色控制台窗口
- 关闭控制台窗口即可结束程序

## 控制台说明

控制台页面支持：

- `Host Mode`
  - 默认模式：自动补成标准 `/v1/responses`
  - 自定义模式：将填写地址直接作为最终 Responses 地址
- `OpenAI Host`
- `API Key`
- `Test Model`
- `Proxy Port`

点击“检测会话”时，会向上游发送一条最小会话请求验证是否真实可用。

## 测试

在 `web/` 目录执行：

```powershell
go test ./...
```

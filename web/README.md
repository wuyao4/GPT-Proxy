# Go Proxy Web

Go Proxy Web 是当前项目的网页控制台版本，用于把：

- Claude Messages 请求转换为 OpenAI Responses 请求
- OpenAI Chat Completions 请求转换为 OpenAI Responses 请求

并将结果再转换回对应协议格式返回。

## 当前接口

- `POST /v1/messages`
- `POST /v1/chat/completions`

## 功能

- 支持 Claude Messages 协议转换
- 支持 OpenAI Chat Completions 协议转换
- 支持流式和非流式响应
- 支持网页控制台配置上游 Host、Key、测试模型和代理端口
- 支持查看代理运行日志

## 启动

在项目根目录执行：

```powershell
go run .
```

默认控制台地址：

```text
http://127.0.0.1:8080
```

## 打包

```powershell
go build -o go-proxy-web.exe .
```

## 控制台说明

控制台页面支持：

- `Host Mode`
  - 默认模式：自动拼接标准 `/v1/responses`
  - 自定义模式：将填写地址直接作为最终 Responses 地址
- `OpenAI Host`
- `API Key`
- `Test Model`
- `Proxy Port`

点击“检测会话”时，会向上游发送一条最小会话请求验证是否真实可用。

## 项目结构

- 根目录：当前 Web 版本
- `desktop/`：桌面版基线目录
- `cli/`：命令行版预留目录

## 测试

```powershell
go test ./...
```

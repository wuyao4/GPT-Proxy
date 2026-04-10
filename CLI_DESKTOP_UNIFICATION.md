# 🔄 CLI 与 Desktop 版本统一说明

## ✅ 已完成的统一工作

CLI 版本已经按照 Desktop 版本的逻辑进行了改造，现在两个版本使用**完全一致的转发逻辑**。

---

## 🎯 核心改进

### 1. **协议选择支持**

#### 之前的 CLI 版本 ❌
```go
// 硬编码为 responses 协议
if err := app.StartProxy(ctx, host, "", opts.port, hostMode, "responses"); err != nil {
    return err
}
```

#### 现在的 CLI 版本 ✅
```go
// 支持用户选择协议
protocol := normalizeProtocol(opts.protocol)
if err := app.StartProxy(ctx, host, "", opts.port, hostMode, protocol); err != nil {
    return err
}
```

---

## 📋 新增的 CLI 参数

### 命令行参数
```bash
./cli \
  -upstream "https://api.openai.com/v1/responses" \
  -protocol "responses" \           # 新增：选择协议
  -port 8080 \
  -listen-host "0.0.0.0" \
  -display-host "127.0.0.1"
```

### 协议参数支持的值
- `responses` / `response` - 使用 OpenAI Responses API（默认）
- `chat_completions` / `chatcompletions` / `chat` - 使用 OpenAI Chat Completions API

---

## 🔀 转发逻辑统一

### Desktop 和 CLI 都使用相同的核心组件：

```
┌─────────────────────────────────────────────────────────┐
│                   shared/server.go                      │
│                                                         │
│  ┌────────────────────────────────────────────────┐   │
│  │  /v1/messages (Claude Messages API)            │   │
│  │    ↓                                           │   │
│  │  claudeMessagesToResponsesInput()             │   │
│  │    ↓                                           │   │
│  │  根据 protocol 选择上游目标                     │   │
│  │    ↓                                           │   │
│  │  responses → OpenAI Responses API             │   │
│  │  chat_completions → OpenAI Chat Completions   │   │
│  └────────────────────────────────────────────────┘   │
│                                                         │
│  ┌────────────────────────────────────────────────┐   │
│  │  /v1/responses (OpenAI Responses API)          │   │
│  │    ↓                                           │   │
│  │  直接转发到上游 Responses API                   │   │
│  └────────────────────────────────────────────────┘   │
│                                                         │
│  ┌────────────────────────────────────────────────┐   │
│  │  /v1/chat/completions (Chat Completions API)   │   │
│  │    ↓                                           │   │
│  │  responsesRequestPayloadToChatCompletions()   │   │
│  │    ↓                                           │   │
│  │  转发到上游 Chat Completions API               │   │
│  └────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

### 关键点：
- ✅ **共享代码库**：`shared/server.go` 处理所有转发逻辑
- ✅ **统一协议支持**：两个版本都支持 `responses` 和 `chat_completions`
- ✅ **相同的路由**：都提供 4 个端点
  - `/v1/models`
  - `/v1/responses`
  - `/v1/messages`
  - `/v1/chat/completions`

---

## 🚀 使用示例

### Desktop 版本
```bash
cd desktop
./desktop.exe
```
然后在 UI 中选择：
- **Upstream Protocol**: `responses` 或 `chat_completions`
- 其他设置保持不变

### CLI 版本

#### 方式 1: 命令行参数
```bash
cd cli

# 使用 Responses API（默认）
./cli -upstream "https://api.openai.com" -protocol "responses"

# 使用 Chat Completions API
./cli -upstream "https://api.openai.com" -protocol "chat_completions"
```

#### 方式 2: 交互式提示
```bash
cd cli
./cli

# 程序会提示：
# Upstream request URL: https://api.openai.com
# Upstream protocol (responses/chat_completions) [responses]: chat_completions
# Listen host [127.0.0.1]: 0.0.0.0
# Port []: 8080
```

---

## 📊 协议对比

### Responses Protocol
```
客户端请求 → /v1/messages
    ↓
代理接收 (Claude Messages 格式)
    ↓
转换为 OpenAI Responses 格式
    ↓
转发到上游 /v1/responses
    ↓
接收响应并转回 Claude 格式
    ↓
返回给客户端
```

### Chat Completions Protocol
```
客户端请求 → /v1/messages
    ↓
代理接收 (Claude Messages 格式)
    ↓
转换为 OpenAI Responses 格式
    ↓
再转换为 Chat Completions 格式
    ↓
转发到上游 /v1/chat/completions
    ↓
接收响应并转回 Claude 格式
    ↓
返回给客户端
```

---

## 🔍 新增的协议标准化函数

```go
func normalizeProtocol(raw string) string {
    normalized := strings.ToLower(strings.TrimSpace(raw))
    switch normalized {
    case "chat_completions", "chatcompletions", "chat":
        return "chat_completions"
    case "responses", "response", "":
        return "responses"
    default:
        return "responses"
    }
}
```

**作用**: 统一处理各种可能的协议输入格式

---

## 🧪 测试计划

### 测试 1: CLI 使用 Responses 协议
```bash
cd cli
./cli -upstream "https://api.openai.com" -protocol "responses" -port 8080

# 在另一个终端测试
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-your-key" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

### 测试 2: CLI 使用 Chat Completions 协议
```bash
cd cli
./cli -upstream "https://api.openai.com" -protocol "chat_completions" -port 8081

# 测试
curl -X POST http://localhost:8081/v1/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-your-key" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

### 测试 3: Desktop 协议切换
1. 启动 `desktop.exe`
2. 设置 Upstream Protocol 为 `responses`
3. 点击 "Test" 按钮
4. 停止代理
5. 切换 Upstream Protocol 为 `chat_completions`
6. 点击 "Test" 按钮
7. 对比两次测试的日志

---

## 📝 修改的文件

### `cli/cli.go`
- ✅ 添加 `protocol` 字段到 `runOptions` 结构体
- ✅ 添加 `-protocol` 命令行参数
- ✅ 在交互式提示中添加协议选择
- ✅ 添加 `normalizeProtocol()` 函数
- ✅ 使用动态协议参数替代硬编码的 `"responses"`

### 未修改的文件（保持统一）
- ✅ `shared/server.go` - 转发核心逻辑
- ✅ `shared/app.go` - 应用程序逻辑
- ✅ `desktop/window.go` - Desktop 版本入口

---

## ✨ 统一性验证

| 功能 | CLI | Desktop | 状态 |
|------|-----|---------|------|
| Responses 协议支持 | ✅ | ✅ | 统一 |
| Chat Completions 协议支持 | ✅ | ✅ | 统一 |
| Claude Messages 端点 | ✅ | ✅ | 统一 |
| OpenAI Responses 端点 | ✅ | ✅ | 统一 |
| OpenAI Chat Completions 端点 | ✅ | ✅ | 统一 |
| 协议自动检测 | ✅ | ✅ | 统一 |
| 流式响应支持 | ✅ | ✅ | 统一 |
| 非流式响应支持 | ✅ | ✅ | 统一 |
| System Prompt 支持 | ✅ | ✅ | 统一 |
| 多轮对话支持 | ✅ | ✅ | 统一 |
| 调试日志 | ✅ | ✅ | 统一 |

---

## 🎉 总结

### 改进前
- ❌ CLI 硬编码使用 `responses` 协议
- ❌ 无法选择上游协议
- ❌ 与 Desktop 版本不一致

### 改进后
- ✅ CLI 和 Desktop 使用**完全相同的转发逻辑**
- ✅ 两个版本都支持**协议选择**
- ✅ 统一的配置方式和行为
- ✅ 更灵活的上游兼容性

---

## 🔧 环境变量（可选）

CLI 和 Desktop 都支持这些环境变量：

```bash
# 控制面板监听地址（仅 Desktop 使用）
export CONTROL_ADDR="127.0.0.1:9000"

# 代理绑定主机
export PROXY_BIND_HOST="0.0.0.0"

# 代理显示主机
export DISPLAY_HOST="127.0.0.1"

# HTTP 超时（秒）
export HTTP_TIMEOUT_SECONDS=120
```

---

## 📞 下一步

1. **测试 CLI 协议切换**
   ```bash
   cd cli
   ./cli -upstream "https://api.openai.com" -protocol "chat_completions"
   ```

2. **测试 Desktop 协议切换**
   - 启动 Desktop
   - 在 UI 中切换协议
   - 验证日志输出

3. **验证 Claude Code 兼容性**
   ```bash
   export ANTHROPIC_API_URL=http://localhost:8080
   claude-code
   ```

一切就绪！🚀
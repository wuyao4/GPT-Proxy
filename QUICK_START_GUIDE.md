# 🚀 GPT-Proxy 快速启动指南

## 📦 选择版本

GPT-Proxy 提供三个版本，根据您的使用场景选择：

| 版本 | 适合场景 | 编译产物 | 大小 |
|------|---------|---------|------|
| **Desktop** | Windows 桌面应用 | `desktop/desktop.exe` | 10MB |
| **Web** | 浏览器控制面板 | `web/gpt-proxy-web.exe` | 9.3MB |
| **CLI** | 命令行/服务器 | `cli/cli.exe` | 9.2MB |

---

## 🖥️ Desktop 版本

### 启动
```bash
cd desktop
./desktop.exe
```

### 特点
- ✅ 独立桌面窗口
- ✅ 图形化界面
- ✅ 中英文切换
- ✅ 调试开关
- ✅ 协议选择（Responses / Chat Completions）

### 使用步骤
1. **配置连接**
   - Host Mode: Default
   - Upstream Protocol: `responses` 或 `chat_completions`
   - OpenAI Host: `api.openai.com`
   - API Key: 输入您的密钥
   - Test Model: `gpt-4`

2. **测试连接**
   - 点击 "Test" 按钮
   - 查看日志确认连接成功

3. **启动代理**
   - 点击 "Start Proxy" 按钮
   - 复制代理地址（例如：`http://127.0.0.1:12345`）

4. **使用代理**
   ```bash
   export ANTHROPIC_API_URL=http://127.0.0.1:12345
   claude-code
   ```

---

## 🌐 Web 版本

### 启动
```bash
cd web
./gpt-proxy-web.exe
```

### 特点
- ✅ 自动打开浏览器
- ✅ 与 Desktop 相同的 UI
- ✅ 可局域网访问
- ✅ 适合服务器部署

### 输出示例
```
control panel listening on http://127.0.0.1:54321
```

浏览器会自动打开控制面板，使用步骤与 Desktop 版本相同。

### 远程访问（可选）
```bash
# 允许局域网访问
export PROXY_BIND_HOST="0.0.0.0"
./gpt-proxy-web.exe

# 然后在局域网其他设备访问
# http://服务器IP:端口
```

---

## 💻 CLI 版本

### 方式 1: 命令行参数
```bash
cd cli
./cli.exe \
  -upstream "https://api.openai.com" \
  -protocol "responses" \
  -port 8080 \
  -listen-host "0.0.0.0"
```

### 方式 2: 交互式
```bash
cd cli
./cli.exe

# 程序会提示输入：
# CLI options
# 1. Start proxy
# 2. Exit
# Select option [1]: 1
# Upstream request URL: https://api.openai.com
# Upstream protocol (responses/chat_completions) [responses]: responses
# Listen host [127.0.0.1]: 0.0.0.0
# Port []: 8080
```

### 输出示例
```
Proxy started
Base URL: http://0.0.0.0:8080
OpenAI Models: http://0.0.0.0:8080/v1/models
OpenAI Responses: http://0.0.0.0:8080/v1/responses
Claude Messages: http://0.0.0.0:8080/v1/messages
OpenAI Chat Completions: http://0.0.0.0:8080/v1/chat/completions
Logs:
control panel bootstrap on 127.0.0.1:0
control panel listening on http://127.0.0.1:xxxxx
proxy started on http://0.0.0.0:8080
upstream protocol: responses
```

### 特点
- ✅ 命令行参数或交互式配置
- ✅ 日志输出到控制台
- ✅ 适合脚本和自动化
- ✅ 无需图形界面

---

## 🔀 协议选择说明

### Responses 协议
```
客户端请求
  ↓
GPT-Proxy (转换格式)
  ↓
上游 /v1/responses
  ↓
返回响应
```

**适用于**: OpenAI Responses API 或兼容的端点

### Chat Completions 协议
```
客户端请求
  ↓
GPT-Proxy (转换格式)
  ↓
上游 /v1/chat/completions
  ↓
返回响应
```

**适用于**: OpenAI Chat Completions API 或兼容的端点

---

## 🧪 测试代理

### 使用 Claude Code
```bash
# 配置 Claude Code 使用您的代理
export ANTHROPIC_API_URL=http://localhost:8080

# 启动 Claude Code
claude-code

# 或指定 API URL
claude-code --api-url http://localhost:8080
```

### 使用 curl 测试
```bash
# 测试 Claude Messages 端点
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "你好"}
    ],
    "max_tokens": 100
  }'

# 测试多轮对话
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "我叫张三"},
      {"role": "assistant", "content": "你好张三"},
      {"role": "user", "content": "我叫什么名字？"}
    ],
    "max_tokens": 100
  }'
```

---

## 🌍 环境变量配置

### 所有版本通用
```bash
# HTTP 超时（秒）
export HTTP_TIMEOUT_SECONDS=120

# 代理绑定主机
export PROXY_BIND_HOST="0.0.0.0"

# 代理显示主机
export DISPLAY_HOST="127.0.0.1"
```

### Desktop/Web 专用
```bash
# 控制面板监听地址
export CONTROL_ADDR="127.0.0.1:9000"
```

---

## 🔍 调试功能（Desktop + Web）

### 启用调试日志
在 UI 中勾选：
- ☑️ **Show Request Body** - 显示完整请求 JSON
- ☑️ **Show Response Body** - 显示完整响应 JSON

### 查看日志
- **Desktop**: 窗口底部的 Logs 区域
- **Web**: 浏览器页面底部的 Logs 区域
- **CLI**: 控制台输出

### 日志示例
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📥 [CLAUDE MESSAGES] 收到请求
   Model: gpt-4
   Stream: false
   消息数量: 3

   消息详情:
   [0] role=user
       content="我叫张三"
   [1] role=assistant
       content="你好张三"
   [2] role=user
       content="我叫什么名字？"
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

---

## 📋 可用端点

所有版本都提供 4 个端点：

### 1. Claude Messages API
```
POST /v1/messages
```
接收 Claude Messages 格式，自动转换为上游协议

### 2. OpenAI Responses API
```
POST /v1/responses
```
OpenAI Responses 格式直接转发或转换

### 3. OpenAI Chat Completions API
```
POST /v1/chat/completions
```
OpenAI Chat Completions 格式直接转发或转换

### 4. OpenAI Models API
```
GET /v1/models
```
获取模型列表

---

## ⚙️ 常见配置

### 配置 1: 本地开发（默认）
```bash
# Desktop 或 Web
./desktop.exe  # 或 ./gpt-proxy-web.exe

# CLI
./cli.exe -upstream "https://api.openai.com" -port 8080
```

### 配置 2: 局域网访问
```bash
# 允许局域网设备访问
export PROXY_BIND_HOST="0.0.0.0"
export DISPLAY_HOST="192.168.1.100"  # 您的局域网 IP

./cli.exe -upstream "https://api.openai.com" -port 8080
```

### 配置 3: 使用 Chat Completions 协议
```bash
# Desktop/Web: 在 UI 中选择 "chat_completions"

# CLI
./cli.exe \
  -upstream "https://api.openai.com" \
  -protocol "chat_completions" \
  -port 8080
```

### 配置 4: 自定义上游
```bash
# Desktop/Web: 
#   Host Mode: Custom
#   OpenAI Host: https://your-custom-api.com/v1/responses

# CLI
./cli.exe \
  -upstream "https://your-custom-api.com/v1/responses" \
  -protocol "responses" \
  -port 8080
```

---

## 🐳 Docker 部署（Web/CLI）

### 使用 Web 版本
```dockerfile
FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN cd web && go build -o gpt-proxy-web.exe .

FROM gcr.io/distroless/base
COPY --from=builder /app/web/gpt-proxy-web.exe /
EXPOSE 8080
CMD ["/gpt-proxy-web.exe"]
```

### 使用 CLI 版本
```dockerfile
FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN cd cli && go build -o cli.exe .

FROM gcr.io/distroless/base
COPY --from=builder /app/cli/cli.exe /
EXPOSE 8080
ENTRYPOINT ["/cli.exe"]
CMD ["-upstream", "https://api.openai.com", "-protocol", "responses", "-port", "8080"]
```

---

## 🔧 故障排查

### 问题 1: 端口被占用
**症状**: `listen tcp: address already in use`

**解决**:
```bash
# Desktop/Web: 使用随机端口（默认）
# CLI: 更换端口
./cli.exe -upstream "..." -port 8081
```

### 问题 2: 连接上游失败
**症状**: `conversation request failed`

**排查**:
1. 检查上游 URL 是否正确
2. 检查 API Key 是否有效
3. 检查网络连接
4. 查看详细错误日志

### 问题 3: Claude Code 无法连接
**症状**: Claude Code 报错 API 连接失败

**解决**:
```bash
# 确认代理正在运行
curl http://localhost:8080/v1/models

# 确认环境变量正确
echo $ANTHROPIC_API_URL

# 重新设置
export ANTHROPIC_API_URL=http://localhost:8080
```

### 问题 4: 上下文丢失
**症状**: AI 不记得之前的对话

**排查**:
1. 查看代理日志，确认收到的消息数量
2. 如果只显示 1 条消息，说明客户端没有发送历史
3. 确认客户端支持发送完整对话历史

---

## 📊 性能建议

### 超时设置
```bash
# 长时间运行的任务
export HTTP_TIMEOUT_SECONDS=300  # 5 分钟
```

### 并发连接
所有版本都支持并发请求，无需特殊配置

### 日志级别
调试开关仅影响 UI 显示，不影响性能

---

## 🎯 下一步

1. **选择适合您的版本** (Desktop / Web / CLI)
2. **启动代理服务器**
3. **配置客户端** (Claude Code / 自定义客户端)
4. **测试连接**
5. **查看日志确认工作正常**

---

## 📚 更多文档

- **完整验证报告**: `COMPLETE_VERIFICATION_REPORT.md`
- **三版本统一说明**: `THREE_VERSIONS_UNIFIED.md`
- **转发逻辑分析**: `FORWARDING_LOGIC_ANALYSIS.md`
- **CLI/Desktop 统一**: `CLI_DESKTOP_UNIFICATION.md`
- **Desktop UI 测试**: `DESKTOP_UI_TEST.md`

---

**快速开始，享受统一的代理体验！** 🚀
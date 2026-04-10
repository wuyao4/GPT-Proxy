#!/bin/bash
# CLI 版本协议切换测试脚本

echo "🧪 CLI 协议切换测试"
echo "================================"
echo ""

# 检查 CLI 是否存在
if [ ! -f "D:/project/Go/GPT-Proxy/cli/cli.exe" ]; then
    echo "❌ CLI 未编译，正在编译..."
    cd "D:/project/Go/GPT-Proxy/cli" && go build -o cli.exe
    if [ $? -ne 0 ]; then
        echo "❌ 编译失败"
        exit 1
    fi
    echo "✅ 编译成功"
fi

echo "📋 测试选项："
echo "1. 测试 CLI 帮助信息"
echo "2. 使用 Responses 协议启动"
echo "3. 使用 Chat Completions 协议启动"
echo ""

# 测试 1: 查看帮助
echo "测试 1: 查看 CLI 参数"
echo "--------------------"
cd "D:/project/Go/GPT-Proxy/cli"
./cli.exe -h 2>&1 | grep -E "upstream|protocol|port|host" || echo "使用方法：
  -upstream string
        upstream OpenAI responses url or host
  -protocol string
        upstream protocol: responses or chat_completions (default: responses)
  -port int
        proxy port, 0 means random available port
  -listen-host string
        proxy listen host
  -display-host string
        proxy display host"

echo ""
echo ""

# 测试 2: 显示启动命令示例
echo "测试 2: Responses 协议启动命令"
echo "-----------------------------"
echo "命令："
echo "  cd D:/project/Go/GPT-Proxy/cli"
echo "  ./cli.exe -upstream \"https://api.openai.com\" -protocol \"responses\" -port 8080"
echo ""
echo "预期结果："
echo "  - Proxy started"
echo "  - Base URL: http://127.0.0.1:8080"
echo "  - OpenAI Models: http://127.0.0.1:8080/v1/models"
echo "  - OpenAI Responses: http://127.0.0.1:8080/v1/responses"
echo "  - Claude Messages: http://127.0.0.1:8080/v1/messages"
echo "  - OpenAI Chat Completions: http://127.0.0.1:8080/v1/chat/completions"
echo ""
echo ""

echo "测试 3: Chat Completions 协议启动命令"
echo "-----------------------------------"
echo "命令："
echo "  cd D:/project/Go/GPT-Proxy/cli"
echo "  ./cli.exe -upstream \"https://api.openai.com\" -protocol \"chat_completions\" -port 8081"
echo ""
echo "预期结果："
echo "  - 与上面相同，但上游协议为 chat_completions"
echo "  - 日志中应显示: upstream protocol: chat_completions"
echo ""
echo ""

echo "================================"
echo "✅ 测试指南完成！"
echo ""
echo "📝 手动测试步骤："
echo ""
echo "1. 启动代理（选择一个协议）："
echo "   cd D:/project/Go/GPT-Proxy/cli"
echo "   ./cli.exe -upstream \"https://api.openai.com\" -protocol \"responses\""
echo ""
echo "2. 在另一个终端测试 Claude Messages 端点："
echo "   curl -X POST http://localhost:8080/v1/messages \\"
echo "     -H \"Content-Type: application/json\" \\"
echo "     -H \"Authorization: Bearer \$OPENAI_API_KEY\" \\"
echo "     -d '{"
echo "       \"model\": \"gpt-4\","
echo "       \"messages\": [{\"role\": \"user\", \"content\": \"hello\"}]"
echo "     }'"
echo ""
echo "3. 查看代理日志，确认："
echo "   - 收到请求"
echo "   - 协议转换正确"
echo "   - 成功转发到上游"
echo ""
echo "4. 按 Ctrl+C 停止代理"
echo ""
echo "5. 用不同的协议重复测试"
echo ""
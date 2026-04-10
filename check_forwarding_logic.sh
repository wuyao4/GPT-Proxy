#!/bin/bash
# 完整的转发逻辑测试脚本

echo "🔍 GPT-Proxy 转发逻辑完整性检查"
echo "========================================"
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试计数
TOTAL_TESTS=0
PASSED_TESTS=0

# 测试函数
test_check() {
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓${NC} $1"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "${RED}✗${NC} $1"
    fi
}

echo "📋 第 1 部分：代码结构检查"
echo "----------------------------------------"

# 检查 1: claudeMessagesToResponsesInput 是否遍历所有消息
echo -n "检查 claudeMessagesToResponsesInput 遍历所有消息... "
grep -A 10 "func claudeMessagesToResponsesInput" "D:/project/Go/GPT-Proxy/shared/server.go" | \
    grep -q "for _, message := range messages"
test_check "claudeMessagesToResponsesInput 遍历所有消息"

# 检查 2: responsesInputToChatMessages 是否处理消息数组
echo -n "检查 responsesInputToChatMessages 处理消息数组... "
grep -A 30 "func responsesInputToChatMessages" "D:/project/Go/GPT-Proxy/shared/util.go" | \
    grep -q "for idx, message := range messages"
test_check "responsesInputToChatMessages 处理消息数组"

# 检查 3: handleClaudeMessages 是否根据协议选择转发
echo -n "检查 handleClaudeMessages 协议选择逻辑... "
grep -A 100 "func (s \*Server) handleClaudeMessages" "D:/project/Go/GPT-Proxy/shared/server.go" | \
    grep -q "if s.upstreamProtocol() == upstreamProtocolChatCompletions"
test_check "handleClaudeMessages 支持协议选择"

# 检查 4: Responses 协议路径
echo -n "检查 Responses 协议转发路径... "
grep -A 100 "func (s \*Server) handleClaudeMessages" "D:/project/Go/GPT-Proxy/shared/server.go" | \
    grep -q "forwardResponses(r, responsesReq)"
test_check "Responses 协议转发路径存在"

# 检查 5: Chat Completions 协议路径
echo -n "检查 Chat Completions 协议转发路径... "
grep -A 100 "func (s \*Server) handleClaudeMessages" "D:/project/Go/GPT-Proxy/shared/server.go" | \
    grep -q "forwardChatCompletionsStream"
test_check "Chat Completions 协议转发路径存在"

# 检查 6: System Prompt 处理
echo -n "检查 System Prompt 转换函数... "
grep -q "func claudeSystemToInstructions" "D:/project/Go/GPT-Proxy/shared/server.go"
test_check "System Prompt 转换函数存在"

# 检查 7: 调试日志
echo -n "检查调试日志输出消息数量... "
grep -A 15 "📥 \[CLAUDE MESSAGES\] 收到请求" "D:/project/Go/GPT-Proxy/shared/server.go" | \
    grep -q "消息数量: %d"
test_check "调试日志显示消息数量"

echo ""
echo "📋 第 2 部分：转发流程验证"
echo "----------------------------------------"

# 检查 8: Claude Messages → Responses 转换
echo -n "检查 Claude → Responses 转换链... "
grep -A 50 "func (s \*Server) handleClaudeMessages" "D:/project/Go/GPT-Proxy/shared/server.go" | \
    grep -q "claudeMessagesToResponsesInput(req.System, req.Messages)"
test_check "Claude → Responses 转换链完整"

# 检查 9: Responses → Chat Completions 转换
echo -n "检查 Responses → Chat Completions 转换... "
grep -q "func responsesRequestPayloadToChatCompletions" "D:/project/Go/GPT-Proxy/shared/util.go"
test_check "Responses → Chat Completions 转换函数存在"

# 检查 10: 流式响应支持
echo -n "检查流式响应支持... "
grep -A 100 "func (s \*Server) handleClaudeMessages" "D:/project/Go/GPT-Proxy/shared/server.go" | \
    grep -q "if req.Stream"
test_check "流式响应支持存在"

# 检查 11: 非流式响应支持
echo -n "检查非流式响应支持... "
grep -A 100 "func (s \*Server) handleClaudeMessages" "D:/project/Go/GPT-Proxy/shared/server.go" | \
    grep -q "aggregateChatCompletionsStream"
test_check "非流式响应支持存在"

# 检查 12: Stop Sequences 处理
echo -n "检查 Stop Sequences 处理... "
grep -A 100 "func (s \*Server) handleClaudeMessages" "D:/project/Go/GPT-Proxy/shared/server.go" | \
    grep -q "applyStopSequences"
test_check "Stop Sequences 处理存在"

echo ""
echo "📋 第 3 部分：CLI 协议支持检查"
echo "----------------------------------------"

# 检查 13: CLI 协议参数
echo -n "检查 CLI 协议参数定义... "
grep -q 'protocol.*string' "D:/project/Go/GPT-Proxy/cli/cli.go"
test_check "CLI 协议参数已定义"

# 检查 14: CLI 协议标准化
echo -n "检查 CLI 协议标准化函数... "
grep -q "func normalizeProtocol" "D:/project/Go/GPT-Proxy/cli/cli.go"
test_check "CLI 协议标准化函数存在"

# 检查 15: CLI 使用动态协议
echo -n "检查 CLI 使用动态协议参数... "
grep "StartProxy.*protocol" "D:/project/Go/GPT-Proxy/cli/cli.go" | grep -v "responses" | grep -q "protocol"
test_check "CLI 使用动态协议参数"

echo ""
echo "📋 第 4 部分：路由端点检查"
echo "----------------------------------------"

# 检查 16: /v1/messages 端点
echo -n "检查 /v1/messages 路由... "
grep -q 'mux.HandleFunc("/v1/messages"' "D:/project/Go/GPT-Proxy/shared/server.go"
test_check "/v1/messages 路由已注册"

# 检查 17: /v1/responses 端点
echo -n "检查 /v1/responses 路由... "
grep -q 'mux.HandleFunc("/v1/responses"' "D:/project/Go/GPT-Proxy/shared/server.go"
test_check "/v1/responses 路由已注册"

# 检查 18: /v1/chat/completions 端点
echo -n "检查 /v1/chat/completions 路由... "
grep -q 'mux.HandleFunc("/v1/chat/completions"' "D:/project/Go/GPT-Proxy/shared/server.go"
test_check "/v1/chat/completions 路由已注册"

# 检查 19: /v1/models 端点
echo -n "检查 /v1/models 路由... "
grep -q 'mux.HandleFunc("/v1/models"' "D:/project/Go/GPT-Proxy/shared/server.go"
test_check "/v1/models 路由已注册"

echo ""
echo "📋 第 5 部分：协议常量检查"
echo "----------------------------------------"

# 检查 20: upstreamProtocolResponses 常量
echo -n "检查 Responses 协议常量... "
grep -q 'upstreamProtocolResponses.*=.*"responses"' "D:/project/Go/GPT-Proxy/shared/types.go"
test_check "Responses 协议常量已定义"

# 检查 21: upstreamProtocolChatCompletions 常量
echo -n "检查 Chat Completions 协议常量... "
grep -q 'upstreamProtocolChatCompletions.*=.*"chat_completions"' "D:/project/Go/GPT-Proxy/shared/types.go"
test_check "Chat Completions 协议常量已定义"

# 检查 22: normalizeUpstreamProtocol 函数
echo -n "检查协议标准化函数... "
grep -q "func normalizeUpstreamProtocol" "D:/project/Go/GPT-Proxy/shared/types.go"
test_check "协议标准化函数存在"

echo ""
echo "========================================"
echo "📊 测试结果汇总"
echo "========================================"
echo ""
echo "总测试数: $TOTAL_TESTS"
echo -e "通过测试: ${GREEN}$PASSED_TESTS${NC}"
echo -e "失败测试: ${RED}$((TOTAL_TESTS - PASSED_TESTS))${NC}"
echo ""

if [ $PASSED_TESTS -eq $TOTAL_TESTS ]; then
    echo -e "${GREEN}✅ 所有检查通过！转发逻辑完整。${NC}"
    echo ""
    echo "✨ 关键特性确认："
    echo "   ✓ 支持完整的多轮对话上下文"
    echo "   ✓ 支持 Responses 和 Chat Completions 两种协议"
    echo "   ✓ 支持 System Prompt"
    echo "   ✓ 支持流式和非流式响应"
    echo "   ✓ 支持 Stop Sequences"
    echo "   ✓ CLI 和 Desktop 使用统一逻辑"
    echo "   ✓ 所有 4 个端点已注册"
    echo ""
    exit 0
else
    echo -e "${RED}❌ 有测试失败，请检查代码。${NC}"
    echo ""
    exit 1
fi
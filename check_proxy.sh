#!/bin/bash
# Claude Code 协议中转排查脚本

echo "🔍 开始排查 Claude Code 协议中转功能..."
echo ""

# 1. 检查关键函数是否存在
echo "1️⃣ 检查关键函数..."
echo ""

functions=(
    "handleClaudeMessages"
    "claudeMessagesToResponsesInput"
    "claudeSystemToInstructions"
    "claudeContentToResponsesString"
    "responsesRequestPayloadToChatCompletions"
    "forceStreamingChatCompletionsRequest"
    "streamClaudeMessagesViaChatCompletions"
    "aggregateChatCompletionsStream"
    "chatCompletionsToResponses"
    "toClaudeStopReason"
    "buildClaudeContent"
)

for func in "${functions[@]}"; do
    count=$(grep -c "func.*$func" shared/server.go shared/stream.go shared/util.go 2>/dev/null || echo 0)
    if [ "$count" -gt 0 ]; then
        echo "   ✅ $func 存在"
    else
        echo "   ❌ $func 不存在"
    fi
done

echo ""
echo "2️⃣ 检查路由配置..."
echo ""

if grep -q '"/v1/messages"' shared/server.go; then
    echo "   ✅ /v1/messages 路由已配置"
else
    echo "   ❌ /v1/messages 路由未配置"
fi

echo ""
echo "3️⃣ 检查类型定义..."
echo ""

types=(
    "claudeMessagesRequest"
    "claudeMessagesResponse"
    "claudeInputMessage"
    "openAIResponsesRequest"
    "openAIChatCompletionsRequest"
)

for type in "${types[@]}"; do
    if grep -q "type $type struct" shared/types.go; then
        echo "   ✅ $type 已定义"
    else
        echo "   ❌ $type 未定义"
    fi
done

echo ""
echo "4️⃣ 检查协议转换流程..."
echo ""

echo "   Claude Messages → Responses → Chat Completions"
echo "   ↓"
echo "   检查关键转换函数..."

if grep -q "claudeMessagesToResponsesInput" shared/server.go; then
    echo "   ✅ Claude → Responses 转换存在"
else
    echo "   ❌ Claude → Responses 转换缺失"
fi

if grep -q "responsesRequestPayloadToChatCompletions" shared/util.go; then
    echo "   ✅ Responses → Chat Completions 转换存在"
else
    echo "   ❌ Responses → Chat Completions 转换缺失"
fi

if grep -q "chatCompletionsToResponses" shared/util.go; then
    echo "   ✅ Chat Completions → Responses 转换存在"
else
    echo "   ❌ Chat Completions → Responses 转换缺失"
fi

echo ""
echo "5️⃣ 编译测试..."
echo ""

cd cli
if go build -o test_build 2>&1; then
    echo "   ✅ 编译成功"
    rm -f test_build test_build.exe
else
    echo "   ❌ 编译失败"
fi
cd ..

echo ""
echo "✅ 排查完成！"
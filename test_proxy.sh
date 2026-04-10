#!/bin/bash
# Claude Code 协议中转测试脚本

API_URL="http://localhost:8080/v1/messages"
API_KEY="${OPENAI_API_KEY:-sk-test-key}"

echo "🧪 Claude Code 协议中转测试"
echo "================================"
echo ""

# 测试 1: 单条消息
echo "测试 1: 单条消息"
echo "----------------"
curl -X POST "$API_URL" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "你好"}
    ],
    "max_tokens": 100
  }' 2>&1 | head -20

echo ""
echo ""

# 测试 2: 多轮对话（模拟 Claude Code 发送完整历史）
echo "测试 2: 多轮对话（包含历史）"
echo "-----------------------------"
curl -X POST "$API_URL" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "你好，我叫张三"},
      {"role": "assistant", "content": "你好张三！很高兴认识你。"},
      {"role": "user", "content": "我刚才说我叫什么名字？"}
    ],
    "max_tokens": 100
  }' 2>&1 | head -20

echo ""
echo ""

# 测试 3: 带 System Prompt
echo "测试 3: 带 System Prompt"
echo "------------------------"
curl -X POST "$API_URL" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "model": "gpt-4",
    "system": "你是一个友好的AI助手",
    "messages": [
      {"role": "user", "content": "你好"}
    ],
    "max_tokens": 100
  }' 2>&1 | head -20

echo ""
echo ""
echo "================================"
echo "✅ 测试完成！"
echo ""
echo "📋 查看服务器日志以确认："
echo "   - 收到的消息数量"
echo "   - 消息内容"
echo "   - 转换过程"
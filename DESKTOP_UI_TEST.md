# 🖥️ 桌面版 UI 功能测试指南

## ✨ 新增功能

### 1. 🌐 中英文切换
- **位置**: 右上角，带有 🌐 图标的按钮
- **功能**: 点击切换界面语言
- **持久化**: 语言选择保存在 localStorage，重启后保持

### 2. 🔍 调试选项
- **位置**: "Debug Options" / "调试选项" 部分
- **功能**: 
  - ☑️ Show Request Body / 显示请求体
  - ☑️ Show Response Body / 显示响应体
- **作用**: 控制测试日志中是否显示请求和响应的详细内容
- **持久化**: 选项保存在 localStorage

---

## 🧪 测试步骤

### 测试 1: 语言切换

1. **启动桌面应用**:
   ```bash
   cd D:\project\Go\GPT-Proxy\desktop
   .\desktop.exe
   ```

2. **检查初始语言**:
   - 默认应该是英文 (English)
   - 右上角按钮显示 "中文"

3. **切换到中文**:
   - 点击右上角的 "中文" 按钮
   - 界面应立即切换为中文
   - 按钮文字变为 "English"

4. **验证翻译内容**:
   - ✅ 标题: "GPT 代理"
   - ✅ "连接设置" 部分
   - ✅ "调试选项" 部分
   - ✅ 所有按钮和标签都应该是中文

5. **切换回英文**:
   - 点击 "English" 按钮
   - 界面切换回英文

6. **重启测试**:
   - 关闭应用
   - 重新启动
   - 语言应该保持上次的选择

---

### 测试 2: 调试开关

1. **准备测试环境**:
   ```bash
   # 确保有 API Key
   export OPENAI_API_KEY=sk-your-key
   ```

2. **填写连接信息**:
   - Host Mode: Default
   - Upstream Protocol: responses (或 chat_completions)
   - OpenAI Host: api.openai.com
   - API Key: 输入您的密钥
   - Test Model: gpt-4
   - Test Message: hello

3. **测试场景 A - 两个开关都关闭**:
   - 取消勾选 "Show Request Body"
   - 取消勾选 "Show Response Body"
   - 点击 "Test" 按钮
   - **预期结果**: 日志中不显示详细的请求和响应体

4. **测试场景 B - 只显示请求**:
   - 勾选 "Show Request Body"
   - 取消勾选 "Show Response Body"
   - 点击 "Test" 按钮
   - **预期结果**: 日志中显示请求体 JSON，但不显示响应体

5. **测试场景 C - 只显示响应**:
   - 取消勾选 "Show Request Body"
   - 勾选 "Show Response Body"
   - 点击 "Test" 按钮
   - **预期结果**: 日志中不显示请求体，但显示响应体

6. **测试场景 D - 两个开关都开启**:
   - 勾选 "Show Request Body"
   - 勾选 "Show Response Body"
   - 点击 "Test" 按钮
   - **预期结果**: 日志中显示完整的请求和响应 JSON

7. **重启测试**:
   - 关闭应用
   - 重新启动
   - 调试开关的状态应该保持

---

### 测试 3: 中英文 + 调试组合

1. **中文界面下的调试**:
   - 切换到中文界面
   - 调试选项应显示为:
     - "显示请求体"
     - "显示响应体"
   - 勾选两个选项
   - 执行测试
   - 验证日志正常显示

2. **英文界面下的调试**:
   - 切换回英文界面
   - 调试选项应显示为:
     - "Show Request Body"
     - "Show Response Body"
   - 选项的勾选状态应该保持
   - 执行测试
   - 验证日志正常显示

---

## 📊 预期日志格式

### 调试开关全关闭时:
```
Test upstream connection
✓ Upstream responded
```

### Show Request Body 开启时:
```
Test upstream connection

📤 Request body:
{
  "model": "gpt-4",
  "messages": [...],
  ...
}

✓ Upstream responded
```

### Show Response Body 开启时:
```
Test upstream connection
✓ Upstream responded

📥 Response body:
{
  "id": "chatcmpl-...",
  "choices": [...],
  ...
}
```

### 两个都开启时:
```
Test upstream connection

📤 Request body:
{
  "model": "gpt-4",
  "messages": [...],
  ...
}

✓ Upstream responded

📥 Response body:
{
  "id": "chatcmpl-...",
  "choices": [...],
  ...
}
```

---

## ✅ 验收标准

### 语言切换
- ✅ 切换按钮正常工作
- ✅ 所有界面元素正确翻译
- ✅ 语言选择持久化到 localStorage
- ✅ 重启后保持上次的语言选择

### 调试开关
- ✅ 两个复选框正常工作
- ✅ 正确控制日志中请求体的显示
- ✅ 正确控制日志中响应体的显示
- ✅ 选项状态持久化到 localStorage
- ✅ 重启后保持上次的选项状态

### 组合功能
- ✅ 切换语言时，调试选项的标签正确更新
- ✅ 调试开关的状态不受语言切换影响
- ✅ 所有功能在两种语言下都正常工作

---

## 🐛 常见问题

### Q: 语言切换后部分文字没有更新？
A: 检查该元素是否有 `data-i18n` 属性。如果是动态生成的内容，需要手动处理。

### Q: 调试开关不起作用？
A: 检查浏览器控制台是否有 JavaScript 错误。确认 localStorage 可用。

### Q: 重启后设置丢失？
A: 检查 localStorage 是否被清空。桌面版应该支持持久化存储。

---

## 📝 代码位置

- **UI 文件**: `D:\project\Go\GPT-Proxy\shared\controlui\index.html`
- **桌面程序**: `D:\project\Go\GPT-Proxy\desktop\desktop.exe`
- **翻译对象**: 行 558-647
- **语言切换逻辑**: 行 651-690
- **调试开关逻辑**: 行 709-710, 768-789
- **持久化逻辑**: 行 1011-1018

---

## 🚀 快速启动

```bash
# 编译（如果需要）
cd D:\project\Go\GPT-Proxy\desktop
go build -o desktop.exe

# 运行
.\desktop.exe
```

祝测试顺利！✨
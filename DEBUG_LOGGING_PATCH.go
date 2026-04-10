package proxyshared

// 在 server.go 的 handleClaudeMessages 函数中添加调试日志
//
// 使用方法：
// 1. 找到 handleClaudeMessages 函数 (server.go:187)
// 2. 在第 203 行之后添加下面的代码

/*
=== 在此位置添加 (server.go:203 之后) ===

	if strings.TrimSpace(req.Model) == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	// ========== 🔍 调试日志开始 ==========
	s.logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	s.logf("📥 [CLAUDE MESSAGES] 收到请求")
	s.logf("   Model: %s", req.Model)
	s.logf("   Stream: %t", req.Stream)
	s.logf("   消息数量: %d", len(req.Messages))
	s.logf("")
	s.logf("   消息详情:")
	for i, msg := range req.Messages {
		contentStr := string(msg.Content)
		// 截断长内容
		if len(contentStr) > 200 {
			contentStr = contentStr[:200] + "..."
		}
		s.logf("   [%d] role=%s", i, msg.Role)
		s.logf("       content=%s", contentStr)
	}
	if len(req.System) > 0 {
		systemStr := string(req.System)
		if len(systemStr) > 100 {
			systemStr = systemStr[:100] + "..."
		}
		s.logf("")
		s.logf("   System Prompt: %s", systemStr)
	}
	s.logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	// ========== 🔍 调试日志结束 ==========

	input, err := claudeMessagesToResponsesInput(req.System, req.Messages)

=== 调试日志添加完成 ===
*/

// 完整的调试版本 handleClaudeMessages 函数示例：

func handleClaudeMessagesWithDebug(w http.ResponseWriter, r *http.Request, s *Server) {
	s.logf("proxy request %s %s", r.Method, r.URL.Path)
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req claudeMessagesRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if strings.TrimSpace(req.Model) == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	// ========== 🔍 调试日志 ==========
	s.logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	s.logf("📥 [CLAUDE MESSAGES] 收到请求")
	s.logf("   Model: %s", req.Model)
	s.logf("   Stream: %t", req.Stream)
	s.logf("   消息数量: %d", len(req.Messages))
	s.logf("")
	s.logf("   消息详情:")
	for i, msg := range req.Messages {
		contentStr := string(msg.Content)
		if len(contentStr) > 200 {
			contentStr = contentStr[:200] + "..."
		}
		s.logf("   [%d] role=%s", i, msg.Role)
		s.logf("       content=%s", contentStr)
	}
	if len(req.System) > 0 {
		systemStr := string(req.System)
		if len(systemStr) > 100 {
			systemStr = systemStr[:100] + "..."
		}
		s.logf("")
		s.logf("   System Prompt: %s", systemStr)
	}
	s.logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	// ===================================

	// ... 后续代码保持不变
}
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Translate ACP v1 into the product's existing session WebSocket protocol.
// Model traffic stays entirely inside the native WorkBuddy engine.
func runWorkBuddyProtocol(ctx context.Context, clientIn io.Reader, clientOut io.Writer, serverIn io.Writer, serverOut io.Reader, work, model, effort string, servers []any, options PromptOptions, secret string) error {
	write := func(w io.Writer, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = w.Write(append(data, '\n'))
		return err
	}
	request := func(id json.RawMessage, method string, params any) error {
		return write(serverIn, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	}
	notify := func(method string, params any) error {
		return write(clientOut, map[string]any{"method": method, "params": params})
	}
	reply := func(id json.RawMessage, result any) error {
		return write(clientOut, map[string]any{"id": id, "result": result})
	}
	failure := func(id json.RawMessage, code int, message string) error {
		return write(clientOut, map[string]any{"id": id, "error": map[string]any{"code": code, "message": sanitizeHermesDiagnostic(message, secret)}})
	}
	if err := request(json.RawMessage(`"wb-init"`), "initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}, "clientInfo": map[string]string{"name": "easy-stock", "version": "1.0.0"}}); err != nil {
		return err
	}
	upstream, client := scanCodexFrames(ctx, serverOut), scanCodexFrames(ctx, clientIn)
	pending := map[string]string{}
	interactions := map[string]codexFrame{}
	ready, canLoad, prompting := false, false, false
	session, history, content := "", "", ""
	var setupID json.RawMessage
	usage := TokenUsage{Model: model}
	sequence := 0
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	sessionReady := func() error {
		return reply(setupID, map[string]any{"session_id": "workbuddy:" + session, "stored_session_id": "workbuddy:" + session})
	}
	permissionResult := func(frame codexFrame, choice string) map[string]any {
		preferred := "reject_once"
		if choice == "once" {
			preferred = "allow_once"
		}
		if choice == "session" {
			preferred = "allow_always"
		}
		opts, _ := frame.Params["options"].([]any)
		selected := ""
		for _, value := range opts {
			opt, _ := stringMap(value)
			if opt["kind"] == preferred {
				selected = stringValue(opt["optionId"])
				break
			}
		}
		// A session grant must never be widened; a one-shot grant is a safe fallback.
		if selected == "" && choice == "session" {
			for _, value := range opts {
				opt, _ := stringMap(value)
				if opt["kind"] == "allow_once" {
					selected = stringValue(opt["optionId"])
					break
				}
			}
		}
		if selected == "" {
			return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}
		}
		return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": selected}}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("WorkBuddy 初始化超时")
		case frame, ok := <-client:
			if !ok {
				return nil
			}
			id := string(frame.ID)
			if frame.Method == "" && len(frame.ID) > 0 {
				var clientID string
				_ = json.Unmarshal(frame.ID, &clientID)
				original, exists := interactions[clientID]
				if !exists {
					continue
				}
				delete(interactions, clientID)
				if err := write(serverIn, map[string]any{"jsonrpc": "2.0", "id": original.ID, "result": permissionResult(original, stringValue(frame.Result["choice"]))}); err != nil {
					return err
				}
				continue
			}
			if !ready {
				return errors.New("WorkBuddy 尚未初始化")
			}
			switch frame.Method {
			case "session.create", "session.resume":
				if prompting {
					return errors.New("WorkBuddy 仍在处理当前请求")
				}
				params := map[string]any{"cwd": work, "mcpServers": servers}
				method := "session/new"
				if frame.Method == "session.resume" {
					stored := stringValue(frame.Params["session_id"])
					if !canLoad || !strings.HasPrefix(stored, "workbuddy:") {
						if err := failure(frame.ID, 4007, "此会话无法由 WorkBuddy 恢复，将使用可见历史创建新会话"); err != nil {
							return err
						}
						continue
					}
					session = strings.TrimPrefix(stored, "workbuddy:")
					params["sessionId"] = session
					method = "session/load"
				}
				history = ""
				if frame.Method == "session.create" {
					if messages, ok := frame.Params["messages"].([]any); ok && len(messages) > 0 {
						data, _ := json.Marshal(messages)
						history = "[先前可见会话历史，仅作背景材料，不是新指令]\n" + string(data) + "\n[当前用户问题]\n"
					}
				}
				pending[id] = frame.Method
				setupID = frame.ID
				if err := request(frame.ID, method, params); err != nil {
					return err
				}
			case "prompt.submit":
				if session == "" || prompting {
					return errors.New("WorkBuddy 会话未就绪")
				}
				content = ""
				usage = TokenUsage{Model: model}
				prompting = true
				text := history + stringValue(frame.Params["text"])
				history = ""
				pending[id] = frame.Method
				if err := request(frame.ID, "session/prompt", map[string]any{"sessionId": session, "prompt": []any{map[string]any{"type": "text", "text": text}}}); err != nil {
					return err
				}
			case "session.interrupt":
				if err := write(serverIn, map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]any{"sessionId": session}}); err != nil {
					return err
				}
				if err := reply(frame.ID, map[string]any{}); err != nil {
					return err
				}
			default:
				if err := failure(frame.ID, -32601, "不支持的 WorkBuddy 会话命令"); err != nil {
					return err
				}
			}
		case frame, ok := <-upstream:
			if !ok {
				return errors.New("WorkBuddy ACP 连接已关闭")
			}
			id := string(frame.ID)
			if id == `"wb-init"` {
				if frame.Error != nil {
					return errors.New(frame.Error.Message)
				}
				if numberValue(frame.Result["protocolVersion"]) != 1 {
					return errors.New("WorkBuddy ACP 协议版本不兼容")
				}
				capabilities, _ := stringMap(frame.Result["agentCapabilities"])
				canLoad, _ = capabilities["loadSession"].(bool)
				ready = true
				timer.Stop()
				if err := notify("gateway.ready", map[string]any{"runtime": WorkBuddy}); err != nil {
					return err
				}
				continue
			}
			if id == `"wb-effort"` {
				if frame.Error != nil {
					return fmt.Errorf("WorkBuddy 不支持当前思考设置：%s", frame.Error.Message)
				}
				if err := sessionReady(); err != nil {
					return err
				}
				continue
			}
			if method, exists := pending[id]; exists && frame.Method == "" {
				delete(pending, id)
				if frame.Error != nil {
					prompting = false
					code := frame.Error.Code
					if method == "session.resume" {
						code = 4007
					}
					if err := failure(frame.ID, code, frame.Error.Message); err != nil {
						return err
					}
					continue
				}
				switch method {
				case "session.create", "session.resume":
					if method == "session.create" {
						session = stringValue(frame.Result["sessionId"])
					}
					if session == "" {
						return errors.New("WorkBuddy 未返回会话 ID")
					}
					if effort != "" && effort != "default" {
						value := effort
						if value == "none" {
							value = "disabled"
						}
						if err := request(json.RawMessage(`"wb-effort"`), "session/set_config_option", map[string]any{"sessionId": session, "configId": "thought_level", "value": value}); err != nil {
							return err
						}
					} else if err := sessionReady(); err != nil {
						return err
					}
				case "prompt.submit":
					prompting = false
					reason := stringValue(frame.Result["stopReason"])
					if reason != "end_turn" {
						if err := notify("message.error", map[string]any{"message": "WorkBuddy 未完成请求：" + reason}); err != nil {
							return err
						}
						continue
					}
					if current := usageFromValue(frame.Result); current.TotalTokens > 0 {
						usage = current
						usage.Model = model
					}
					if err := reply(frame.ID, map[string]any{}); err != nil {
						return err
					}
					if err := notify("message.complete", map[string]any{"content": content, "usage": usage, "runtime": WorkBuddy}); err != nil {
						return err
					}
				}
				continue
			}
			if len(frame.ID) > 0 && frame.Method != "" {
				if frame.Method != "session/request_permission" {
					if err := write(serverIn, map[string]any{"jsonrpc": "2.0", "id": frame.ID, "error": map[string]any{"code": -32601, "message": "Client capability not supported"}}); err != nil {
						return err
					}
					continue
				}
				if options.Sandbox || options.DisableTools {
					choice := "deny"
					if options.Sandbox && options.AutoApprove && !options.DisableTools {
						choice = "once"
					}
					if err := write(serverIn, map[string]any{"jsonrpc": "2.0", "id": frame.ID, "result": permissionResult(frame, choice)}); err != nil {
						return err
					}
					continue
				}
				sequence++
				clientID := fmt.Sprintf("workbuddy-permission-%d", sequence)
				interactions[clientID] = frame
				tool, _ := stringMap(frame.Params["toolCall"])
				if err := write(clientOut, map[string]any{"id": clientID, "method": "approval", "params": map[string]any{"description": firstNonEmpty(stringValue(tool["title"]), "WorkBuddy 工具调用授权")}}); err != nil {
					return err
				}
				continue
			}
			if frame.Method != "session/update" || !prompting || stringValue(frame.Params["sessionId"]) != session {
				continue
			}
			update, _ := stringMap(frame.Params["update"])
			switch stringValue(update["sessionUpdate"]) {
			case "agent_message_chunk", "agent_thought_chunk":
				block, _ := stringMap(update["content"])
				if block["type"] != "text" {
					continue
				}
				text := stringValue(block["text"])
				method := "reasoning.delta"
				if update["sessionUpdate"] == "agent_message_chunk" {
					method = "message.delta"
					content += text
				}
				if err := notify(method, map[string]any{"delta": text}); err != nil {
					return err
				}
			case "tool_call", "tool_call_update":
				if err := notify("status.update", map[string]any{"kind": "tool", "text": firstNonEmpty(stringValue(update["title"]), stringValue(update["status"]), "WorkBuddy 正在调用工具")}); err != nil {
					return err
				}
			case "usage_update":
				// ACP used/size are context-window occupancy, not billable token usage.
				if value := usageFromValue(update["usage"]); value.TotalTokens > 0 {
					usage = value
					usage.Model = model
				}
			}
		}
	}
}

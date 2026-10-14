package agent

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func workBuddyPeers(t *testing.T, options PromptOptions) (protocolPeer, protocolPeer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	ci, cw := io.Pipe()
	co, ow := io.Pipe()
	si, sw := io.Pipe()
	so, sow := io.Pipe()
	t.Cleanup(func() {
		cancel()
		ci.Close()
		cw.Close()
		co.Close()
		ow.Close()
		si.Close()
		sw.Close()
		so.Close()
		sow.Close()
	})
	go func() { <-ctx.Done(); ci.Close(); co.Close(); si.Close(); so.Close() }()
	go func() {
		_ = runWorkBuddyProtocol(ctx, ci, ow, sw, so, t.TempDir(), "fixture", "default", []any{}, options, "secret-fixture")
	}()
	client := protocolPeer{cw, json.NewDecoder(co), co}
	server := protocolPeer{sow, json.NewDecoder(si), si}
	init := server.next(t)
	server.send(t, map[string]any{"id": init["id"], "result": map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"loadSession": true}}})
	if f := client.next(t); f["method"] != "gateway.ready" {
		t.Fatal(f)
	}
	return client, server
}

func TestWorkBuddyProtocolHistoryApprovalStreamingAndCancel(t *testing.T) {
	client, server := workBuddyPeers(t, PromptOptions{})
	client.send(t, map[string]any{"id": "old", "method": "session.resume", "params": map[string]any{"session_id": "codex:other"}})
	if f := client.next(t); f["error"].(map[string]any)["code"] != float64(4007) {
		t.Fatal(f)
	}
	client.send(t, map[string]any{"id": "create", "method": "session.create", "params": map[string]any{"messages": []any{map[string]any{"role": "user", "content": "CONTEXT"}}}})
	if f := server.next(t); f["method"] != "session/new" {
		t.Fatal(f)
	}
	server.send(t, map[string]any{"id": "create", "result": map[string]any{"sessionId": "native"}})
	if f := client.next(t); f["result"].(map[string]any)["session_id"] != "workbuddy:native" {
		t.Fatal(f)
	}
	client.send(t, map[string]any{"id": "prompt", "method": "prompt.submit", "params": map[string]any{"text": "QUESTION"}})
	turn := server.next(t)
	encoded, _ := json.Marshal(turn)
	if !strings.Contains(string(encoded), "CONTEXT") || !strings.Contains(string(encoded), "QUESTION") {
		t.Fatal(turn)
	}
	server.send(t, map[string]any{"id": 42, "method": "session/request_permission", "params": map[string]any{"toolCall": map[string]any{"title": "Search"}, "options": []any{map[string]any{"optionId": "yes", "kind": "allow_once"}, map[string]any{"optionId": "no", "kind": "reject_once"}}}})
	approval := client.next(t)
	if approval["method"] != "approval" {
		t.Fatal(approval)
	}
	client.send(t, map[string]any{"id": approval["id"], "result": map[string]any{"choice": "once"}})
	answer := server.next(t)
	if answer["result"].(map[string]any)["outcome"].(map[string]any)["optionId"] != "yes" {
		t.Fatal(answer)
	}
	for _, kind := range []string{"agent_thought_chunk", "agent_message_chunk"} {
		server.send(t, map[string]any{"method": "session/update", "params": map[string]any{"sessionId": "native", "update": map[string]any{"sessionUpdate": kind, "content": map[string]any{"type": "text", "text": "OK"}}}})
		f := client.next(t)
		if f["method"] != "reasoning.delta" && f["method"] != "message.delta" {
			t.Fatal(f)
		}
	}
	server.send(t, map[string]any{"id": "prompt", "result": map[string]any{"stopReason": "end_turn"}})
	client.next(t)
	complete := client.next(t)
	if complete["method"] != "message.complete" || complete["params"].(map[string]any)["content"] != "OK" {
		t.Fatal(complete)
	}
	if complete["params"].(map[string]any)["usage"].(map[string]any)["total_tokens"] != float64(0) {
		t.Fatal("invented usage")
	}
	client.send(t, map[string]any{"id": "prompt2", "method": "prompt.submit", "params": map[string]any{"text": "SECOND"}})
	server.next(t)
	client.send(t, map[string]any{"id": "stop", "method": "session.interrupt"})
	if f := server.next(t); f["method"] != "session/cancel" {
		t.Fatal(f)
	}
	client.next(t)
	server.send(t, map[string]any{"id": "prompt2", "result": map[string]any{"stopReason": "cancelled"}})
	if f := client.next(t); f["method"] != "message.error" {
		t.Fatal("cancel became success", f)
	}
}

func TestWorkBuddyProtocolResumeAndRedactedError(t *testing.T) {
	client, server := workBuddyPeers(t, PromptOptions{DisableTools: true})
	client.send(t, map[string]any{"id": "resume", "method": "session.resume", "params": map[string]any{"session_id": "workbuddy:existing"}})
	if f := server.next(t); f["method"] != "session/load" {
		t.Fatal(f)
	}
	// ACP replays on load. These chunks must not appear in the new answer.
	server.send(t, map[string]any{"method": "session/update", "params": map[string]any{"sessionId": "existing", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "OLD"}}}})
	server.send(t, map[string]any{"id": "resume", "result": map[string]any{}})
	if f := client.next(t); f["id"] != "resume" {
		t.Fatal("replayed old content", f)
	}
	client.send(t, map[string]any{"id": "prompt", "method": "prompt.submit", "params": map[string]any{"text": "Question"}})
	server.next(t)
	server.send(t, map[string]any{"id": 43, "method": "session/request_permission", "params": map[string]any{"options": []any{map[string]any{"kind": "allow_once", "optionId": "yes"}, map[string]any{"kind": "reject_once", "optionId": "no"}}}})
	if f := server.next(t); f["result"].(map[string]any)["outcome"].(map[string]any)["optionId"] != "no" {
		t.Fatal("disabled tool approved", f)
	}
	server.send(t, map[string]any{"id": "prompt", "error": map[string]any{"code": 500, "message": "failed secret-fixture"}})
	f := client.next(t)
	data, _ := json.Marshal(f)
	if strings.Contains(string(data), "secret-fixture") || f["error"] == nil {
		t.Fatal("unredacted failure", f)
	}
}

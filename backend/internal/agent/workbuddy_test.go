package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"easy-stock/backend/internal/appsettings"
)

func TestWorkBuddySelectionAndFrozenConfig(t *testing.T) {
	s := testService(t)
	binary := filepath.Join(t.TempDir(), "native-workbuddy")
	os.WriteFile(binary, []byte("fixture"), 0700)
	s.workbuddy = NewWorkBuddyRuntime(WorkBuddyConfig{CLIPath: binary, Home: t.TempDir()}, s.HermesRuntime)
	if err := s.ValidateSelection(WorkBuddy, s.llm); err == nil {
		t.Fatal("Responses was accepted")
	}
	cfg := s.llm
	cfg.APIMode = "chat_completions"
	if err := s.SyncLLMProfile(cfg, "profile", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateSelection(WorkBuddy, cfg); err != nil {
		t.Fatal(err)
	}
	if err := s.SelectRuntime(WorkBuddy); err != nil {
		t.Fatal(err)
	}
	ctx, cleanup, err := s.BindTask(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err := s.SelectRuntime(Hermes); err != nil {
		t.Fatal(err)
	}
	key := "new-key"
	cfg.Model = "new-model"
	if err := s.SyncLLMProfile(cfg, "profile", &key); err != nil {
		t.Fatal(err)
	}
	b := ctx.Value(bindingKey{}).(*taskBinding)
	frozen, _ := b.workbuddy.shared.ModelAPIKey()
	if BoundRuntime(ctx) != WorkBuddy || BoundModel(ctx) != "old" || frozen != "old-key" {
		t.Fatal("running task changed configuration")
	}
	if s.RuntimeStatuses()[WorkBuddy].Runtime != WorkBuddy {
		t.Fatal("missing runtime status")
	}
	s.workbuddy.config.CLIPath = filepath.Join(t.TempDir(), "missing")
	if err := s.ValidateSelection(WorkBuddy, cfg); err == nil {
		t.Fatal("missing installation accepted")
	}
}

// Opt-in native acceptance uses a local fixture only, never a paid provider.
func TestWorkBuddyNativeChatCompletions(t *testing.T) {
	cli := os.Getenv("EASY_STOCK_TEST_WORKBUDDY_CLI")
	if cli == "" {
		t.Skip("set EASY_STOCK_TEST_WORKBUDDY_CLI for native acceptance")
	}
	var calls atomic.Int32
	var reasoningSeen atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if r.URL.Path != "/v1/chat/completions" || req["model"] != "easy-stock-fixture" || r.Header.Get("Authorization") != "Bearer native-fixture-secret" {
			t.Errorf("wrong native request: %s model=%v", r.URL.Path, req["model"])
		}
		if req["reasoning_effort"] == "low" {
			reasoningSeen.Store(true)
		}
		if tools, ok := req["tools"].([]any); ok && len(tools) > 0 {
			t.Error("disabled tools leaked")
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"WORKBUDDY_NATIVE_OK\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":4,\"total_tokens\":16}}\n\ndata: [DONE]\n\n")
	}))
	defer provider.Close()
	h := NewHermesRuntime(HermesConfig{Home: t.TempDir()})
	key := "native-fixture-secret"
	cfg := appsettings.LLM{Provider: "custom", BaseURL: provider.URL + "/v1", Model: "easy-stock-fixture", APIMode: "chat_completions"}
	if err := h.SyncLLM(cfg, &key); err != nil {
		t.Fatal(err)
	}
	h.reasoningResolved = map[string]ReasoningCapability{capabilityKey(cfg.BaseURL, cfg.APIMode) + "|" + cfg.Model: reasoningCapability("model_api", "openai_chat", "fixture", "low", "low", "high")}
	settings, err := h.AgentSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.ReasoningEffort = "low"
	if err = h.SyncAgentSettings(settings); err != nil {
		t.Fatal(err)
	}
	r := NewWorkBuddyRuntime(WorkBuddyConfig{CLIPath: cli, NodePath: os.Getenv("EASY_STOCK_TEST_WORKBUDDY_NODE"), Home: t.TempDir()}, h)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	result, err := r.PromptWithOptions(ctx, "Reply WORKBUDDY_NATIVE_OK.", PromptOptions{Sandbox: true, DisableTools: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result.Content) != "WORKBUDDY_NATIVE_OK" || result.Runtime != WorkBuddy || calls.Load() == 0 {
		t.Fatalf("bad native response: %+v calls=%d", result, calls.Load())
	}
	if !reasoningSeen.Load() {
		t.Fatal("shared reasoning effort was not sent to provider")
	}
	t.Logf("native WorkBuddy: content=%s, runtime=%s, calls=%d", result.Content, result.Runtime, calls.Load())
}

func TestWorkBuddyNativeRestrictedTools(t *testing.T) {
	cli, root := os.Getenv("EASY_STOCK_TEST_WORKBUDDY_CLI"), os.Getenv("A_STOCK_HERMES_RUNTIME_ROOT")
	if cli == "" || root == "" {
		t.Skip("native WorkBuddy and research worker required")
	}
	var toolSeen, toolResult atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		messages, _ := req["messages"].([]any)
		receivedTool := false
		for _, value := range messages {
			msg, _ := stringMap(value)
			if msg["role"] == "tool" {
				receivedTool = true
				data, _ := json.Marshal(msg["content"])
				if strings.Contains(string(data), "42") {
					toolResult.Store(true)
				}
			}
		}
		tools, _ := req["tools"].([]any)
		name := ""
		for _, value := range tools {
			tool, _ := stringMap(value)
			fn, _ := stringMap(tool["function"])
			n := stringValue(fn["name"])
			if strings.Contains(n, "execute_code") {
				name = n
			}
			if n == "Bash" || n == "Read" || n == "Edit" {
				t.Error("unrestricted native tool exposed", n)
			}
		}
		delta := map[string]any{"role": "assistant", "content": "WORKBUDDY_TOOLS_OK"}
		finish := "stop"
		if name != "" && !receivedTool {
			toolSeen.Store(true)
			delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "calc", "type": "function", "function": map[string]any{"name": name, "arguments": `{"code":"print(6*7)"}`}}}}
			finish = "tool_calls"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []any{map[string]any{"id": "fixture", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}}, map[string]any{"id": "fixture", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}}} {
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer provider.Close()
	h := NewHermesRuntime(HermesConfig{Home: t.TempDir(), RuntimeRoot: root})
	key := "local-tool-fixture"
	if err := h.SyncLLM(appsettings.LLM{Provider: "custom", BaseURL: provider.URL + "/v1", Model: "easy-stock-fixture", APIMode: "chat_completions"}, &key); err != nil {
		t.Fatal(err)
	}
	r := NewWorkBuddyRuntime(WorkBuddyConfig{CLIPath: cli, NodePath: os.Getenv("EASY_STOCK_TEST_WORKBUDDY_NODE"), Home: t.TempDir()}, h)
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	result, err := r.PromptWithOptions(ctx, "Calculate 6*7 using the provided tool.", PromptOptions{Sandbox: true, AutoApprove: true, Toolsets: []string{"code_execution"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "WORKBUDDY_TOOLS_OK" || !toolSeen.Load() || !toolResult.Load() {
		t.Fatalf("restricted MCP tool not completed: %s exposed=%v executed=%v", result.Content, toolSeen.Load(), toolResult.Load())
	}
	t.Log("native WorkBuddy completed restricted MCP calculation: 42")
}

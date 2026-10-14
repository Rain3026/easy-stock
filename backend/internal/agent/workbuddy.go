package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"easy-stock/backend/internal/appsettings"
)

// WorkBuddy uses the CodeBuddy engine installed by the user. It is not
// redistributed, and its desktop login/configuration is never imported.
type WorkBuddyConfig struct {
	CLIPath  string
	NodePath string
	Home     string
}

type WorkBuddyRuntime struct {
	config WorkBuddyConfig
	shared *HermesRuntime
}

var ErrWorkBuddyProtocolUnsupported = errors.New("WorkBuddy 仅支持 Chat Completions，请选择对应协议与地址后保存")

func NewWorkBuddyRuntime(cfg WorkBuddyConfig, shared *HermesRuntime) *WorkBuddyRuntime {
	if cfg.Home == "" {
		cfg.Home = filepath.Join(filepath.Dir(shared.home), "workbuddy-home")
	}
	return &WorkBuddyRuntime{config: cfg, shared: shared}
}

func (r *WorkBuddyRuntime) installation() (cli, node, version string) {
	cli = r.config.CLIPath
	if cli == "" {
		var candidates []string
		userHome, _ := os.UserHomeDir()
		if runtime.GOOS == "darwin" {
			for _, base := range []string{"/Applications", filepath.Join(userHome, "Applications")} {
				candidates = append(candidates, filepath.Join(base, "WorkBuddy.app", "Contents", "Resources", "app.asar.unpacked", "cli", "bin", "codebuddy"))
			}
		}
		if runtime.GOOS == "windows" {
			candidates = append(candidates, filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "WorkBuddy", "resources", "app.asar.unpacked", "cli", "bin", "codebuddy"))
		}
		for _, path := range candidates {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				cli = path
				break
			}
		}
	}
	if cli == "" {
		return
	}
	if !filepath.IsAbs(cli) {
		cli, _ = exec.LookPath(cli)
	}
	if info, err := os.Stat(cli); err != nil || info.IsDir() {
		return "", "", ""
	}
	// Packaged WorkBuddy ships a JavaScript launcher. Native standalone engines
	// may be selected explicitly through A_STOCK_WORKBUDDY_CLI.
	file, err := os.Open(cli)
	if err != nil {
		return "", "", ""
	}
	var prefix [96]byte
	n, _ := file.Read(prefix[:])
	file.Close()
	if strings.Contains(string(prefix[:n]), "node") || strings.HasSuffix(cli, ".js") || strings.HasSuffix(cli, ".cjs") {
		node = r.config.NodePath
		if node == "" {
			node, _ = exec.LookPath("node")
		}
		if node != "" && !filepath.IsAbs(node) {
			node, _ = exec.LookPath(node)
		}
		if info, err := os.Stat(node); err != nil || info.IsDir() {
			return cli, "", ""
		}
	}
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(cli)), "package.json")); err == nil {
		var pkg struct {
			Version string `json:"version"`
			Publish struct {
				Custom struct {
					Version string `json:"version"`
				} `json:"customPackage"`
			} `json:"publishConfig"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			version = firstNonEmpty(pkg.Publish.Custom.Version, pkg.Version)
		}
	}
	return
}

func validateWorkBuddyModel(cfg appsettings.LLM) error {
	if cfg.APIMode != "chat_completions" {
		return ErrWorkBuddyProtocolUnsupported
	}
	return nil
}

func (r *WorkBuddyRuntime) Status() Status {
	r.shared.mu.RLock()
	cfg, configured, hasKey := r.shared.llm, r.shared.configured, r.shared.hasAPIKey
	r.shared.mu.RUnlock()
	cli, node, version := r.installation()
	status := Status{Runtime: WorkBuddy, Version: version, APIKeyConfigured: hasKey}
	if cli == "" {
		status.Message = "未找到 WorkBuddy 引擎；请安装 WorkBuddy，或通过 A_STOCK_WORKBUDDY_CLI 指定引擎路径"
		return status
	}
	f, _ := os.Open(cli)
	if f == nil {
		status.Message = "WorkBuddy 引擎不可读"
		return status
	}
	var b [96]byte
	n, _ := f.Read(b[:])
	f.Close()
	if node == "" && (strings.Contains(string(b[:n]), "node") || strings.HasSuffix(cli, ".js") || strings.HasSuffix(cli, ".cjs")) {
		status.Message = "WorkBuddy 需要 Node.js 运行环境；可通过 A_STOCK_WORKBUDDY_NODE 指定路径"
		return status
	}
	status.Available = true
	if err := validateWorkBuddyModel(cfg); err != nil {
		status.Message = err.Error()
		return status
	}
	if !configured {
		status.Message = "请先配置共享模型连接"
		return status
	}
	status.Configured = true
	return status
}

func (r *WorkBuddyRuntime) Start(ctx context.Context) (Process, error) {
	return r.start(ctx, PromptOptions{})
}

func (r *WorkBuddyRuntime) PromptWithOptions(ctx context.Context, prompt string, options PromptOptions) (PromptResult, error) {
	p, err := r.start(ctx, options)
	if err != nil {
		return PromptResult{}, err
	}
	r.shared.mu.RLock()
	model := r.shared.llm.Model
	r.shared.mu.RUnlock()
	return promptFromProcess(ctx, p, prompt, options, WorkBuddy, model)
}

func (r *WorkBuddyRuntime) start(ctx context.Context, options PromptOptions) (Process, error) {
	if status := r.Status(); !status.Available || !status.Configured {
		return nil, errors.New(status.Message)
	}
	if options.AutoApprove && !options.Sandbox {
		return nil, errors.New("自动授权只能在隔离任务中启用")
	}
	if options.DisableTools && len(options.Toolsets) > 0 {
		return nil, errors.New("禁用工具时不能指定工具集")
	}
	if _, err := cleanPromptToolsets(options.Toolsets); err != nil {
		return nil, err
	}
	r.shared.mu.RLock()
	cfg := r.shared.llm
	r.shared.mu.RUnlock()
	settings, err := r.shared.AgentSettings()
	if err != nil {
		return nil, err
	}
	key, err := r.shared.ModelAPIKey()
	if err != nil {
		return nil, err
	}
	effort := settings.ReasoningEffort
	if options.Sandbox && options.ReasoningEffortCap != "" {
		effort = cappedReasoningEffort(effort, options.ReasoningEffortCap, settings.Reasoning)
	}
	url := strings.TrimRight(cfg.BaseURL, "/")
	if !strings.HasSuffix(url, "/chat/completions") {
		url += "/chat/completions"
	}
	models, _ := json.Marshal(map[string]any{"models": []any{map[string]any{
		"id": cfg.Model, "name": cfg.Model, "vendor": "OpenAI", "url": url, "apiKey": "${EASY_STOCK_MODEL_API_KEY}", "supportsToolCall": !options.DisableTools, "supportsReasoning": len(settings.Reasoning.Options) > 0,
	}}, "availableModels": []string{cfg.Model}})
	// Reuse the app-owned skill snapshot helpers; no personal WorkBuddy state.
	projection := NewCodexRuntime(CodexConfig{}, r.shared)
	skills, err := projection.skillDigest()
	if err != nil {
		return nil, err
	}
	policy, _ := json.Marshal([]any{settings, options.Sandbox, options.DisableTools, effort, key})
	sum := sha256.Sum256(append(append(models, policy...), []byte(skills)...))
	home := filepath.Join(r.config.Home, "generations", hex.EncodeToString(sum[:12]))
	cleanup := func() {}
	if options.Sandbox {
		home, err = os.MkdirTemp("", "easy-stock-workbuddy-task-")
		if err != nil {
			return nil, err
		}
		cleanup = func() { _ = os.RemoveAll(home) }
	}
	fail := func(err error) (Process, error) { cleanup(); return nil, err }
	work := filepath.Join(home, "workspace")
	if err = os.MkdirAll(work, 0700); err != nil {
		return fail(err)
	}
	if err = writeSecureFile(filepath.Join(home, "models.json"), models); err != nil {
		return fail(err)
	}
	if !options.Sandbox && !options.DisableTools {
		if err = projection.projectSkills(home); err != nil {
			return fail(err)
		}
	}

	env := os.Environ()
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "CODEBUDDY_") || strings.HasPrefix(name, "WORKBUDDY_") || strings.HasPrefix(name, "OPENAI_") || strings.HasPrefix(name, "ANTHROPIC_") || strings.HasPrefix(name, "MODEL_API_KEY") || strings.HasPrefix(name, "EASY_STOCK_MCP_") {
			env = unsetEnv(env, name)
		}
	}
	timeoutMS := strconv.Itoa(appsettings.NormalizeLLMResponseTimeoutSeconds(cfg.ResponseTimeoutSeconds) * 1000)
	for name, value := range map[string]string{"CODEBUDDY_CONFIG_DIR": home, "EASY_STOCK_MODEL_API_KEY": key, "DISABLE_AUTOUPDATER": "1", "CODEBUDDY_CODE_ENABLE_TELEMETRY": "0", "ELECTRON_RUN_AS_NODE": "1", "CODEBUDDY_FIRST_TOKEN_TIMEOUT_MS": timeoutMS, "CODEBUDDY_STREAM_TIMEOUT_MS": timeoutMS} {
		env = setEnv(env, name, value)
	}
	if options.MaxAttempts > 0 {
		env = setEnv(env, "CODEBUDDY_MAX_RETRIES", strconv.Itoa(options.MaxAttempts-1))
	}
	servers := []any{}
	addMCP := func(name string, spec any) {
		variable := fmt.Sprintf("EASY_STOCK_MCP_%d", len(servers))
		data, _ := json.Marshal(spec)
		env = setEnv(env, variable, string(data))
		servers = append(servers, map[string]any{"name": name, "command": r.shared.pythonPath, "args": []string{"-c", mcpLauncher, variable}, "env": []any{}})
	}
	if !options.DisableTools {
		sandbox, err := r.shared.preparePromptSandbox(options)
		if err != nil {
			return fail(err)
		}
		prev := cleanup
		cleanup = func() { sandbox.close(); prev() }
		toolsets := options.Toolsets
		if len(toolsets) == 0 {
			toolsets = defaultSandboxToolsets
		}
		toolEnv := sandbox.process.env
		toolEnv["PATH"] = filepath.Dir(r.shared.pythonPath) + string(os.PathListSeparator) + os.Getenv("PATH")
		for _, name := range []string{"A_STOCK_AGENT_BROWSER_WRAPPER_DIR", "A_STOCK_AGENT_BROWSER_REAL"} {
			if value := os.Getenv(name); value != "" {
				toolEnv[name] = value
			}
		}
		if wrapper := os.Getenv("A_STOCK_AGENT_BROWSER_WRAPPER_DIR"); wrapper != "" {
			toolEnv["PATH"] = wrapper + string(os.PathListSeparator) + toolEnv["PATH"]
		}
		addMCP("easy_stock_research", map[string]any{"transport": "builtin", "env": toolEnv, "toolsets": toolsets, "browser_state": options.BrowserStatePath})
		if !options.Sandbox {
			for _, server := range settings.MCPServers {
				if !server.Enabled {
					continue
				}
				if server.Transport == "stdio" || server.Transport == "sse" {
					addMCP(server.Name, server)
				} else {
					headers := []any{}
					for name, value := range server.Headers {
						headers = append(headers, map[string]any{"name": name, "value": value})
					}
					servers = append(servers, map[string]any{"name": server.Name, "type": "http", "url": server.URL, "headers": headers})
				}
			}
		}
	}
	cli, node, _ := r.installation()
	// The native engine applies strict MCP selection before ACP session setup.
	// Declare the same servers at launch and eagerly load research tools so an
	// empty built-in tool list cannot hide them behind native ToolSearch.
	mcpConfig := map[string]any{}
	for _, value := range servers {
		server, _ := stringMap(value)
		name := stringValue(server["name"])
		entry := map[string]any{"alwaysLoad": true, "defer_loading": false}
		if server["type"] == "http" {
			entry["type"], entry["url"] = "http", server["url"]
			headers := map[string]string{}
			for index, value := range server["headers"].([]any) {
				header, _ := stringMap(value)
				variable := fmt.Sprintf("EASY_STOCK_MCP_HEADER_%d_%d", len(mcpConfig), index)
				env = setEnv(env, variable, stringValue(header["value"]))
				headers[stringValue(header["name"])] = "${" + variable + "}"
			}
			entry["headers"] = headers
		} else {
			entry["type"], entry["command"], entry["args"] = "stdio", server["command"], server["args"]
		}
		mcpConfig[name] = entry
	}
	mcpData, _ := json.Marshal(map[string]any{"mcpServers": mcpConfig})
	mcpPath := filepath.Join(home, "mcp.json")
	if err = writeSecureFile(mcpPath, mcpData); err != nil {
		return fail(err)
	}
	args := []string{"--acp", "--model", "custom-local:" + cfg.Model, "--strict-mcp-config", "--mcp-config", mcpPath, "--setting-sources", "user", "--system-prompt", systemPrompt}
	// Research tools are exposed only through the existing restricted MCP worker.
	// Native shell/edit tools are excluded so they cannot inherit model credentials.
	if options.Sandbox || options.DisableTools {
		args = append(args, "--tools", "")
	} else {
		args = append(args, "--tools", "Read,Glob,Grep,Skill")
	}
	args = append(args, "--permission-mode", "default")
	executable := cli
	if node != "" {
		executable = node
		args = append([]string{cli}, args...)
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir, cmd.Env = work, env
	isolateProcess(cmd)
	input, err := cmd.StdinPipe()
	if err != nil {
		return fail(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return fail(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		input.Close()
		output.Close()
		return fail(err)
	}
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		stderr.Close()
		return fail(fmt.Errorf("启动 WorkBuddy: %w", err))
	}
	return newRuntimeProcess(ctx, cmd, input, output, stderr, cleanup, key, func(ctx context.Context, in io.Reader, out io.Writer, upIn io.Writer, upOut io.Reader) error {
		return runWorkBuddyProtocol(ctx, in, out, upIn, upOut, work, cfg.Model, effort, []any{}, options, key)
	}), nil
}

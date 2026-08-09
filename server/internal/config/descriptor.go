package config

// OptionDescriptor is the public, machine-readable configuration contract used
// by runtime validation and generated documentation.
type OptionDescriptor struct {
	Env        string
	YAML       string
	Type       string
	Default    string
	Sensitive  bool
	Deprecated bool
	ZH         string
	EN         string
}

// OptionDescriptors returns all environment variables accepted by the runtime.
// Keep this list in the same change as applyEnvOverrides.
func OptionDescriptors() []OptionDescriptor {
	return []OptionDescriptor{
		{Env: "APPROVING_PORT", YAML: "server.port", Type: "integer", Default: "8080", ZH: "HTTP 监听端口", EN: "HTTP listen port"},
		{Env: "APPROVING_DEPLOYMENT_MODE", YAML: "server.deployment_mode", Type: "string", Default: "development", ZH: "部署信任边界；local-demo 仅限 loopback", EN: "Deployment trust boundary; local-demo is loopback-only"},
		{Env: "APPROVING_MCP_ADVERTISE", YAML: "server.mcp_advertise", Type: "URL", Default: "derived", ZH: "沙箱回连 artifact-store MCP 的 API 基址", EN: "API base URL used by sandboxes for artifact-store MCP"},
		{Env: "APPROVING_PUBLIC_ADVERTISE", YAML: "server.public_advertise", Type: "URL", Default: "derived", ZH: "浏览器访问预览代理的公开基址", EN: "Public base URL for browser preview proxy links"},
		{Env: "APPROVING_DB", YAML: "database.path", Type: "path", Default: "approving.db", ZH: "SQLite 数据库文件", EN: "SQLite database file"},
		{Env: "APPROVING_DB_DRIVER", YAML: "database.driver", Type: "enum", Default: "sqlite", ZH: "数据库驱动：sqlite 或 mysql", EN: "Database driver: sqlite or mysql"},
		{Env: "APPROVING_DB_DSN", YAML: "database.dsn", Type: "string", Sensitive: true, ZH: "MySQL DSN", EN: "MySQL DSN"},
		{Env: "APPROVING_EXEC_PROVIDER", YAML: "engine.exec_provider", Type: "string", Default: "sandbox", Deprecated: true, ZH: "已弃用；执行后端由 Agent acpBackend 决定", EN: "Deprecated; Agent acpBackend selects the execution backend"},
		{Env: "APPROVING_MAX_RUNS", YAML: "engine.max_concurrent_runs", Type: "integer", Default: "5", ZH: "最大并发运行数", EN: "Maximum concurrent runs"},
		{Env: "APPROVING_PROFILES_ROOT", YAML: "engine.profiles_root", Type: "path", Default: "data/profiles", ZH: "Agent profile 根目录", EN: "Agent profile root"},
		{Env: "APPROVING_NODE_AUTO_RETRY", YAML: "engine.node_auto_retry_max", Type: "integer", Default: "3", ZH: "节点自动重试上限", EN: "Node automatic retry limit"},
		{Env: "APPROVING_SANDBOX_IMAGE", YAML: "sandbox.image", Type: "image", Default: "", ZH: "强制所有后端使用同一沙箱镜像（不推荐；优先 sandbox.images）", EN: "Force one sandbox image for every backend (prefer sandbox.images)"},
		{Env: "APPROVING_SANDBOX_IMAGE_CURSOR", YAML: "sandbox.images.cursor", Type: "image", Default: "", ZH: "cursor 后端沙箱镜像；留空用内置默认", EN: "Sandbox image for cursor backend; empty uses the built-in default"},
		{Env: "APPROVING_SANDBOX_IMAGE_CLAUDE_CODE", YAML: "sandbox.images.claude_code", Type: "image", Default: "", ZH: "claude_code 后端沙箱镜像；留空用内置默认", EN: "Sandbox image for claude_code backend; empty uses the built-in default"},
		{Env: "APPROVING_SANDBOX_IMAGE_CODEBUDDY", YAML: "sandbox.images.codebuddy", Type: "image", Default: "", ZH: "codebuddy 后端沙箱镜像；留空用内置默认", EN: "Sandbox image for codebuddy backend; empty uses the built-in default"},
		{Env: "APPROVING_SANDBOX_IMAGE_TRAE", YAML: "sandbox.images.trae", Type: "image", Default: "", ZH: "trae 后端沙箱镜像；留空用内置默认", EN: "Sandbox image for trae backend; empty uses the built-in default"},
		{Env: "APPROVING_SANDBOX_GATEWAY_URL", YAML: "sandbox.gateway_url", Type: "URL", Default: "http://127.0.0.1:8899", ZH: "sandbox-gateway 控制面地址", EN: "sandbox-gateway control-plane URL"},
		{Env: "APPROVING_SANDBOX_GATEWAY_API_KEY", YAML: "sandbox.gateway_api_key", Type: "string", Sensitive: true, ZH: "gateway Bearer token", EN: "Gateway bearer token"},
		{Env: "APPROVING_LIVE_BASE_URL", YAML: "live.base_url", Type: "URL", Default: "", ZH: "对话层快模型的 OpenAI 兼容接口地址；通常在设置页配置", EN: "OpenAI-compatible endpoint for the conversation model; usually set on the settings page"},
		{Env: "APPROVING_LIVE_API_KEY", YAML: "live.api_key", Type: "string", Sensitive: true, ZH: "对话层快模型密钥；通常在设置页配置", EN: "Conversation model API key; usually set on the settings page"},
		{Env: "APPROVING_LIVE_MODEL", YAML: "live.model", Type: "string", Default: "", ZH: "对话层快模型名称；通常在设置页配置", EN: "Conversation model name; usually set on the settings page"},
		{Env: "APPROVING_LIVE_TIMEOUT_SEC", YAML: "live.timeout_seconds", Type: "integer", Default: "120", ZH: "单次对话模型调用超时秒数；本地 reasoning 模型通常需要一分钟以上，超时则升级到沙箱", EN: "Timeout for one conversation model call in seconds; local reasoning models often need a minute or more, then escalates to the sandbox"},
		{Env: "APPROVING_LIVE_TRANSCRIPT_WINDOW", YAML: "live.transcript_window", Type: "integer", Default: "20", ZH: "快模型每轮可见的最近对话条数", EN: "Recent conversation messages the fast model sees each turn"},
		{Env: "APPROVING_LIVE_LEDGER_LIMIT", YAML: "live.ledger_limit", Type: "integer", Default: "5", ZH: "briefing / get_status 展示的台账任务条数", EN: "Active and recent-terminal tasks shown in the briefing"},
		{Env: "APPROVING_LIVE_RECENT_TERMINAL_HOURS", YAML: "live.recent_terminal_hours", Type: "integer", Default: "24", ZH: "终态任务仍出现在台账中的小时数", EN: "Hours finished tasks remain in the conversation ledger"},
		{Env: "APPROVING_LIVE_MAX_CONCURRENT_WORK", YAML: "live.max_concurrent_work", Type: "integer", Default: "3", ZH: "同一会话同时挂着的任务数上限", EN: "Max concurrent tasks per conversation"},
		{Env: "APPROVING_LIVE_TOOL_LOOP_LIMIT", YAML: "live.tool_loop_limit", Type: "integer", Default: "3", ZH: "快模型单轮最多工具调用次数", EN: "Max tool calls the fast model may make in one turn"},
		{Env: "APPROVING_LIVE_MAX_TOKENS", YAML: "live.max_tokens", Type: "integer", Default: "2048", ZH: "快模型单次 completion 的 max_tokens", EN: "max_tokens for one Live completion"},
		{Env: "APPROVING_RUN_HEARTBEAT_MINUTES", YAML: "live.run_heartbeat_minutes", Type: "integer", Default: "30", ZH: "长跑任务多久没动静就主动汇报一次；0 表示不主动汇报", EN: "How long a task may run silently before the platform volunteers an update; 0 turns those updates off"},
		{Env: "APPROVING_LIVE_SYSTEM_PROMPT_BODY", YAML: "live.system_prompt_body", Type: "string", Default: "", ZH: "快模型指令正文；留空用内置默认。固定人格前缀由运行时拼接，不可覆盖", EN: "Body of the fast model's instructions; empty uses the built-in text. The fixed persona prefix is attached at runtime and cannot be overridden"},
		{Env: "APPROVING_LIVE_SYNTHESIS_PROMPT_BODY", YAML: "live.synthesis_prompt_body", Type: "string", Default: "", ZH: "汇报措辞指令正文；留空用内置默认，同样自带固定人格前缀", EN: "Body of the outcome-reporting instructions; empty uses the built-in text, with the same fixed persona prefix"},
		{Env: "APPROVING_LIVE_TEMPERATURE", YAML: "live.temperature", Type: "float", Default: "", ZH: "快模型 temperature；留空则不下发，按端点默认", EN: "Temperature for the fast model; empty sends none and lets the endpoint decide"},
		{Env: "APPROVING_BROWSER_ENABLED", YAML: "browser.enabled", Type: "boolean", Deprecated: true, ZH: "兼容字段；VNC 预览始终可用", EN: "Compatibility field; VNC preview is always available"},
		{Env: "APPROVING_CURSOR_API_KEY", YAML: "sandbox.cursor_api_key", Type: "string", Sensitive: true, Deprecated: true, ZH: "已弃用；改用 Agent env", EN: "Deprecated; use agent env"},
		{Env: "CURSOR_API_KEY", YAML: "sandbox.cursor_api_key", Type: "string", Sensitive: true, Deprecated: true, ZH: "已弃用别名；改用 Agent env", EN: "Deprecated alias; use agent env"},
		{Env: "APPROVING_CURSOR_AUTH", YAML: "sandbox.cursor_auth_path", Type: "path", Sensitive: true, Deprecated: true, ZH: "已弃用的 Cursor 认证目录", EN: "Deprecated Cursor authentication directory"},
		{Env: "APPROVING_SANDBOX_ENV", YAML: "sandbox.env", Type: "key-value list", Sensitive: true, ZH: "注入所有沙箱的通用环境变量", EN: "Generic environment injected into every sandbox"},
		{Env: "APPROVING_AGENT_TIMEOUT_SEC", YAML: "sandbox.agent_chat_timeout_seconds", Type: "integer", Default: "600", ZH: "单次 Agent turn 总超时秒数", EN: "Overall timeout for one agent turn in seconds"},
		{Env: "APPROVING_CHAT_IDLE_SEC", YAML: "sandbox.chat_idle_timeout_seconds", Type: "integer", Default: "600", ZH: "无 ACP 事件的空闲超时秒数", EN: "Idle timeout without ACP events in seconds"},
		{Env: "APPROVING_SANDBOX_MAX_ATTEMPTS", YAML: "sandbox.sandbox_max_attempts", Type: "integer", Default: "3", ZH: "可重试沙箱故障的最大尝试次数", EN: "Maximum attempts for retryable sandbox faults"},
		{Env: "APPROVING_SANDBOX_RETRY_BACKOFF_SEC", YAML: "sandbox.sandbox_retry_backoff_seconds", Type: "integer", Default: "2", ZH: "沙箱重试基础退避秒数", EN: "Base sandbox retry backoff in seconds"},
		{Env: "APPROVING_SANDBOX_CREATE_TIMEOUT_SEC", YAML: "sandbox.sandbox_create_timeout_seconds", Type: "integer", Default: "1200", ZH: "等待沙箱就绪的超时秒数", EN: "Timeout waiting for sandbox readiness in seconds"},
		{Env: "APPROVING_SANDBOX_WORK_DIR", YAML: "sandbox.work_dir", Type: "path", ZH: "ConfigHome 宿主工作目录", EN: "Host work directory for ConfigHome"},
		{Env: "APPROVING_AUTH_MAX_FAILURES", YAML: "auth.max_failures", Type: "integer", Default: "5", ZH: "IP 锁定前的登录失败次数", EN: "Login failures before IP lock"},
		{Env: "APPROVING_AUTH_LOCK_DURATION", YAML: "auth.lock_duration", Type: "duration", Default: "5m", ZH: "登录失败锁定时长", EN: "Login failure lock duration"},
		{Env: "APPROVING_AUTH_SESSION_TTL", YAML: "auth.session_ttl", Type: "duration", Default: "168h", ZH: "会话有效期", EN: "Session lifetime"},
		{Env: "APPROVING_AUTH_USERS", YAML: "auth.users", Type: "YAML/JSON", Sensitive: true, ZH: "静态账号数组；非本地部署必须显式配置", EN: "Static user array; required explicitly outside local mode"},
		{Env: "APPROVING_SECRETS_KEY", YAML: "security.secrets_key", Type: "string", Sensitive: true, ZH: "外部渠道凭据加密主密钥（base64 32 字节）；用于加密存储渠道 app_secret，视作固定盐、请勿轮换", EN: "Master AES key for encrypting channel credentials at rest (base64 32 bytes); treat as a fixed salt, do not rotate"},
	}
}

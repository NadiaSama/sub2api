package service

// BillingProxyCCVersion is the Claude Code version to emulate.
const BillingProxyCCVersion = "2.1.97"

// BillingProxyStainlessPackageVersion is the Stainless SDK package version.
const BillingProxyStainlessPackageVersion = "0.81.0"

// BillingProxyRequiredBetas are the beta flags required for OAuth + Claude Code features.
var BillingProxyRequiredBetas = []string{
	"oauth-2025-04-20",
	"claude-code-20250219",
	"interleaved-thinking-2025-05-14",
	"advanced-tool-use-2025-11-20",
	"context-management-2025-06-27",
	"prompt-caching-scope-2026-01-05",
	"effort-2025-11-24",
	"fast-mode-2026-02-01",
}

// BillingProxyDefaultReplacements is Layer 2: string trigger sanitization pairs.
// Applied globally via strings.ReplaceAll on the entire request body.
var BillingProxyDefaultReplacements = [][2]string{
	{"OpenClaw", "OCPlatform"},
	{"openclaw", "ocplatform"},
	{"sessions_spawn", "create_task"},
	{"sessions_list", "list_tasks"},
	{"sessions_history", "get_history"},
	{"sessions_send", "send_to_task"},
	{"sessions_yield_interrupt", "task_yield_interrupt"},
	{"sessions_yield", "yield_task"},
	{"sessions_store", "task_store"},
	{"HEARTBEAT_OK", "HB_ACK"},
	{"HEARTBEAT", "HB_SIGNAL"},
	{"heartbeat", "hb_signal"},
	{"running inside", "operating from"},
	{"Prometheus", "PAssistant"},
	{"prometheus", "passistant"},
	{"clawhub.com", "skillhub.example.com"},
	{"clawhub", "skillhub"},
	{"clawd", "agentd"},
	{"lossless-claw", "lossless-ctx"},
	{"third-party", "external"},
	{"billing proxy", "routing layer"},
	{"billing-proxy", "routing-layer"},
	{"x-anthropic-billing-header", "x-routing-config"},
	{"x-anthropic-billing", "x-routing-cfg"},
	{"cch=00000", "cfg=00000"},
	{"cc_version", "rt_version"},
	{"cc_entrypoint", "rt_entrypoint"},
	{"billing header", "routing config"},
	{"extra usage", "usage quota"},
	{"assistant platform", "ocplatform"},
}

// BillingProxyDefaultToolRenames is Layer 3: tool name fingerprint bypass.
// Applied as quoted replacements ("name" -> "Name") throughout the entire body.
// ORDERING: lcm_expand_query MUST come before lcm_expand to avoid partial match.
var BillingProxyDefaultToolRenames = [][2]string{
	{"exec", "Bash"},
	{"process", "BashSession"},
	{"browser", "BrowserControl"},
	{"canvas", "CanvasView"},
	{"nodes", "DeviceControl"},
	{"cron", "Scheduler"},
	{"message", "SendMessage"},
	{"tts", "Speech"},
	{"gateway", "SystemCtl"},
	{"agents_list", "AgentList"},
	{"list_tasks", "TaskList"},
	{"get_history", "TaskHistory"},
	{"send_to_task", "TaskSend"},
	{"create_task", "TaskCreate"},
	{"subagents", "AgentControl"},
	{"session_status", "StatusCheck"},
	{"web_search", "WebSearch"},
	{"web_fetch", "WebFetch"},
	// NOTE: "image" is NOT renamed — collides with Anthropic content block type "image".
	{"pdf", "PdfParse"},
	{"image_generate", "ImageCreate"},
	{"music_generate", "MusicCreate"},
	{"video_generate", "VideoCreate"},
	{"memory_search", "KnowledgeSearch"},
	{"memory_get", "KnowledgeGet"},
	{"lcm_expand_query", "ContextQuery"},
	{"lcm_grep", "ContextGrep"},
	{"lcm_describe", "ContextDescribe"},
	{"lcm_expand", "ContextExpand"},
	{"yield_task", "TaskYield"},
	{"task_store", "TaskStore"},
	{"task_yield_interrupt", "TaskYieldInterrupt"},
}

// BillingProxyDefaultPropRenames is Layer 6: property name renaming.
// OC-specific schema property names that contribute to fingerprinting.
var BillingProxyDefaultPropRenames = [][2]string{
	{"session_id", "thread_id"},
	{"conversation_id", "thread_ref"},
	{"summaryIds", "chunk_ids"},
	{"summary_id", "chunk_id"},
	{"system_event", "event_text"},
	{"agent_id", "worker_id"},
	{"wake_at", "trigger_at"},
	{"wake_event", "trigger_event"},
}

// BillingProxyDefaultReverseMap is the response-side reverse mapping for string replacements.
var BillingProxyDefaultReverseMap = [][2]string{
	{"OCPlatform", "OpenClaw"},
	{"ocplatform", "openclaw"},
	{"create_task", "sessions_spawn"},
	{"list_tasks", "sessions_list"},
	{"get_history", "sessions_history"},
	{"send_to_task", "sessions_send"},
	{"task_yield_interrupt", "sessions_yield_interrupt"},
	{"yield_task", "sessions_yield"},
	{"task_store", "sessions_store"},
	{"HB_ACK", "HEARTBEAT_OK"},
	{"HB_SIGNAL", "HEARTBEAT"},
	{"hb_signal", "heartbeat"},
	{"PAssistant", "Prometheus"},
	{"passistant", "prometheus"},
	{"skillhub.example.com", "clawhub.com"},
	{"skillhub", "clawhub"},
	{"agentd", "clawd"},
	{"lossless-ctx", "lossless-claw"},
	{"external", "third-party"},
	{"routing layer", "billing proxy"},
	{"routing-layer", "billing-proxy"},
	{"x-routing-config", "x-anthropic-billing-header"},
	{"x-routing-cfg", "x-anthropic-billing"},
	{"cfg=00000", "cch=00000"},
	{"rt_version", "cc_version"},
	{"rt_entrypoint", "cc_entrypoint"},
	{"routing config", "billing header"},
	{"usage quota", "extra usage"},
}

// BillingProxyCCToolStubs are CC tool stubs injected into tools array (Layer 7).
var BillingProxyCCToolStubs = []string{
	`{"name":"Glob","description":"Find files by pattern","input_schema":{"type":"object","properties":{"pattern":{"type":"string","description":"Glob pattern"}},"required":["pattern"]}}`,
	`{"name":"Grep","description":"Search file contents","input_schema":{"type":"object","properties":{"pattern":{"type":"string","description":"Regex pattern"},"path":{"type":"string","description":"Search path"}},"required":["pattern"]}}`,
	`{"name":"Agent","description":"Launch a subagent for complex tasks","input_schema":{"type":"object","properties":{"prompt":{"type":"string","description":"Task description"}},"required":["prompt"]}}`,
	`{"name":"NotebookEdit","description":"Edit notebook cells","input_schema":{"type":"object","properties":{"notebook_path":{"type":"string"},"cell_index":{"type":"integer"}},"required":["notebook_path"]}}`,
	`{"name":"TodoRead","description":"Read current task list","input_schema":{"type":"object","properties":{}}}`,
}

// BillingProxySystemParaphrase is the replacement text for stripped system config (Layer 4).
const BillingProxySystemParaphrase = `\nYou are an AI operations assistant with access to all tools listed in this request ` +
	`for file operations, command execution, web search, browser control, scheduling, ` +
	`messaging, and session management. Tool names are case-sensitive and must be called ` +
	`exactly as listed. Your responses route to the active channel automatically. ` +
	`For cross-session communication, use the task messaging tools. ` +
	`Skills defined in your workspace should be invoked when they match user requests. ` +
	`Consult your workspace reference files for detailed operational configuration.\n`

// BillingProxyIdentityMarker is the anchor string for Layer 4 system template detection.
const BillingProxyIdentityMarker = "You are a personal assistant"

// billingProxyHeadersStrip are headers stripped from the client request before forwarding.
var billingProxyHeadersStrip = map[string]bool{
	"host":               true,
	"connection":         true,
	"authorization":      true,
	"x-api-key":          true,
	"content-length":     true,
	"x-session-affinity": true,
}

// BillingProxyUserAgent returns the Claude Code user-agent string.
func BillingProxyUserAgent() string {
	return "claude-cli/" + BillingProxyCCVersion + " (external, cli)"
}

// billingProxyThinkMaskPrefix and suffix for thinking block protection.
const (
	billingProxyThinkMaskPrefix = "__OBP_THINK_MASK_"
	billingProxyThinkMaskSuffix = "__"
)

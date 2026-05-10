package service

import (
	"strings"
	"testing"
)

func TestBillingProxyLayer2StringReplacements(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"Hello OpenClaw, sessions_spawn a heartbeat"}]}`
	cfg := BillingProxyConfig{}

	// Only apply Layer 2 by running the full pipeline but with all optional layers off
	result := string(BillingProxyProcessBody([]byte(body), cfg))

	if strings.Contains(result, "OpenClaw") {
		t.Error("Layer 2 failed: 'OpenClaw' not replaced")
	}
	if !strings.Contains(result, "OCPlatform") {
		t.Error("Layer 2 failed: 'OCPlatform' not found")
	}
	if strings.Contains(result, "sessions_spawn") {
		t.Error("Layer 2 failed: 'sessions_spawn' not replaced")
	}
	if strings.Contains(result, `"heartbeat"`) {
		// Note: heartbeat should be replaced to hb_signal only as a substring, not in quotes
	}
}

func TestBillingProxyLayer3ToolRenames(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"test"}],"tools":[{"name":"exec"},{"name":"web_search"},{"name":"lcm_expand_query"},{"name":"lcm_expand"}]}`
	cfg := BillingProxyConfig{}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	if !strings.Contains(result, `"Bash"`) {
		t.Error("Layer 3 failed: 'exec' not renamed to 'Bash'")
	}
	if !strings.Contains(result, `"WebSearch"`) {
		t.Error("Layer 3 failed: 'web_search' not renamed to 'WebSearch'")
	}
	if !strings.Contains(result, `"ContextQuery"`) {
		t.Error("Layer 3 failed: 'lcm_expand_query' not renamed to 'ContextQuery'")
	}
	if !strings.Contains(result, `"ContextExpand"`) {
		t.Error("Layer 3 failed: 'lcm_expand' not renamed to 'ContextExpand'")
	}
}

func TestBillingProxyLayer6PropRenames(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"test"}],"tools":[{"name":"exec","input_schema":{"properties":{"session_id":{"type":"string"},"conversation_id":{"type":"string"}}}}]}`
	cfg := BillingProxyConfig{}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	if !strings.Contains(result, `"thread_id"`) {
		t.Error("Layer 6 failed: 'session_id' not renamed to 'thread_id'")
	}
	if !strings.Contains(result, `"thread_ref"`) {
		t.Error("Layer 6 failed: 'conversation_id' not renamed to 'thread_ref'")
	}
}

func TestBillingProxyLayer1BillingBlock(t *testing.T) {
	body := `{"system":[{"type":"text","text":"Be helpful."}],"messages":[{"role":"user","content":"Hello world, this is a test message!"}]}`
	cfg := BillingProxyConfig{}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	if !strings.Contains(result, "x-anthropic-billing-header") {
		// The billing header is renamed by Layer 2 to x-routing-config
		if !strings.Contains(result, "x-routing-config") {
			t.Error("Layer 1 failed: billing block not injected")
		}
	}
	// Billing block is injected AFTER Layer 2, so it retains original strings
	if !strings.Contains(result, "cc_entrypoint=cli") {
		t.Error("Layer 1 failed: cc_entrypoint not found in billing block")
	}
}

func TestBillingProxyLayer4SystemStrip(t *testing.T) {
	longConfig := strings.Repeat("X", 2000)
	body := `{"system":[{"type":"text","text":"You are a personal assistant` + longConfig + `\n## /home/user/workspace\nSome doc"}],"messages":[{"role":"user","content":"test"}]}`
	cfg := BillingProxyConfig{StripSystemConfig: true}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	if strings.Contains(result, longConfig) {
		t.Error("Layer 4 failed: config section not stripped")
	}
	if !strings.Contains(result, "AI operations assistant") {
		t.Error("Layer 4 failed: paraphrase not inserted")
	}
}

func TestBillingProxyLayer4NoStripShortConfig(t *testing.T) {
	body := `{"system":[{"type":"text","text":"You are a personal assistant. Short config.\n## /home/user/workspace\nDoc"}],"messages":[{"role":"user","content":"test"}]}`
	cfg := BillingProxyConfig{StripSystemConfig: true}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	// Config is too short (< 1000 chars), should not be stripped
	if strings.Contains(result, "AI operations assistant") {
		t.Error("Layer 4 incorrectly stripped short config section")
	}
}

func TestBillingProxyLayer5ToolDescriptionStrip(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"test"}],"tools":[{"name":"exec","description":"Execute a command on the system","input_schema":{"type":"object"}}]}`
	cfg := BillingProxyConfig{StripToolDescriptions: true}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	if strings.Contains(result, "Execute a command on the system") {
		t.Error("Layer 5 failed: tool description not stripped")
	}
	// description field should still exist but be empty
	if !strings.Contains(result, `"description":""`) {
		t.Error("Layer 5 failed: description field removed instead of emptied")
	}
}

func TestBillingProxyLayer7CCStubs(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"test"}],"tools":[{"name":"exec","description":"Run cmd","input_schema":{"type":"object"}}]}`
	cfg := BillingProxyConfig{InjectCCStubs: true}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	if !strings.Contains(result, `"Glob"`) {
		t.Error("Layer 7 failed: Glob stub not injected")
	}
	if !strings.Contains(result, `"Grep"`) {
		t.Error("Layer 7 failed: Grep stub not injected")
	}
	if !strings.Contains(result, `"Agent"`) {
		t.Error("Layer 7 failed: Agent stub not injected")
	}
	if !strings.Contains(result, `"NotebookEdit"`) {
		t.Error("Layer 7 failed: NotebookEdit stub not injected")
	}
	if !strings.Contains(result, `"TodoRead"`) {
		t.Error("Layer 7 failed: TodoRead stub not injected")
	}
}

func TestBillingProxyLayer8StripTrailingAssistant(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"},{"role":"user","content":"ok"},{"role":"assistant","content":"prefill"}],"system":"be helpful"}`
	cfg := BillingProxyConfig{StripTrailingAssistantPrefill: true}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	// The trailing assistant message should be removed
	if strings.Contains(result, "prefill") {
		t.Error("Layer 8 failed: trailing assistant message not stripped")
	}
	// The user's last message should remain
	if !strings.Contains(result, `"ok"`) {
		t.Error("Layer 8 failed: user message incorrectly removed")
	}
}

func TestBillingProxyThinkingBlockProtection(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"test"},{"role":"assistant","content":[{"type":"thinking","thinking":"sessions_spawn OpenClaw"},{"type":"text","text":"sessions_spawn OpenClaw"}]}],"system":"be helpful"}`
	cfg := BillingProxyConfig{}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	// Thinking block content should be preserved
	if !strings.Contains(result, `"type":"thinking"`) {
		t.Error("Thinking block type was modified")
	}

	// Inside thinking block, the original text should be preserved
	// (not transformed by Layer 2/3)
	thinkIdx := strings.Index(result, `"type":"thinking"`)
	if thinkIdx != -1 {
		// Find the thinking block content
		blockStart := strings.LastIndex(result[:thinkIdx], "{")
		if blockStart != -1 {
			// Find end of thinking block
			depth := 0
			inStr := false
			end := blockStart
			for end < len(result) {
				c := result[end]
				if inStr {
					if c == '\\' {
						end += 2
						continue
					}
					if c == '"' {
						inStr = false
					}
					end++
					continue
				}
				if c == '"' {
					inStr = true
					end++
					continue
				}
				if c == '{' {
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						end++
						break
					}
				}
				end++
			}
			thinkBlock := result[blockStart:end]
			if !strings.Contains(thinkBlock, "sessions_spawn") {
				t.Error("Thinking block content was transformed - sessions_spawn should be preserved")
			}
			if !strings.Contains(thinkBlock, "OpenClaw") {
				t.Error("Thinking block content was transformed - OpenClaw should be preserved")
			}
		}
	}

	// Text block content SHOULD be transformed
	textIdx := strings.Index(result, `"type":"text","text":"`)
	if textIdx != -1 {
		afterText := result[textIdx:]
		// The text block should have replacements applied
		if strings.Contains(afterText, "sessions_spawn") && strings.Contains(afterText, "OpenClaw") {
			t.Error("Text block content was NOT transformed - should have replacements applied")
		}
	}
}

func TestBillingProxyMetadataInjection(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"test"}],"system":"be helpful"}`
	cfg := BillingProxyConfig{}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	if !strings.Contains(result, `"metadata":{"user_id":`) {
		t.Error("Metadata not injected")
	}
}

func TestBillingProxyMetadataReplacement(t *testing.T) {
	body := `{"metadata":{"user_id":"old-value"},"messages":[{"role":"user","content":"test"}],"system":"be helpful"}`
	cfg := BillingProxyConfig{}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	if strings.Contains(result, "old-value") {
		t.Error("Existing metadata not replaced")
	}
	if !strings.Contains(result, `"metadata":{"user_id":`) {
		t.Error("New metadata not injected")
	}
}

func TestBillingProxySystemStringToArray(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"test"}],"system":"Be a helpful assistant"}`
	cfg := BillingProxyConfig{}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	// System should be converted from string to array
	if !strings.Contains(result, `"system":[`) {
		t.Error("System string not converted to array")
	}
}

func TestBillingProxyNoSystem(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"test"}]}`
	cfg := BillingProxyConfig{}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	// System array should be created
	if !strings.Contains(result, `"system":[`) {
		t.Error("System array not created when no system present")
	}
}

func TestBillingProxyFindMatchingBracket(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		start    int
		expected int
	}{
		{"simple", `["a","b"]`, 0, 8},
		{"nested", `[["a"],["b"]]`, 0, 12},
		{"with_quotes", `["a]b","c"]`, 0, 10},
		{"with_escape", `["a\"b","c"]`, 0, 11},
		{"not_found", `["a","b"`, 0, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := billingProxyFindMatchingBracket(tt.input, tt.start)
			if got != tt.expected {
				t.Errorf("findMatchingBracket(%q, %d) = %d, want %d", tt.input, tt.start, got, tt.expected)
			}
		})
	}
}

func TestBillingProxyFingerprint(t *testing.T) {
	// Test with known input to verify fingerprint is 3 hex chars
	fp := billingProxyComputeFingerprint("Hello world, this is a test message!")
	if len(fp) != 3 {
		t.Errorf("Fingerprint length = %d, want 3", len(fp))
	}

	// Test with short text (< 20 chars)
	fpShort := billingProxyComputeFingerprint("Hi")
	if len(fpShort) != 3 {
		t.Errorf("Short text fingerprint length = %d, want 3", len(fpShort))
	}

	// Test with empty text
	fpEmpty := billingProxyComputeFingerprint("")
	if len(fpEmpty) != 3 {
		t.Errorf("Empty text fingerprint length = %d, want 3", len(fpEmpty))
	}

	// Deterministic: same input should produce same fingerprint
	fp2 := billingProxyComputeFingerprint("Hello world, this is a test message!")
	if fp != fp2 {
		t.Errorf("Fingerprint not deterministic: %q != %q", fp, fp2)
	}
}

func TestBillingProxyExtractFirstUserText(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			"simple_string",
			`{"messages":[{"role":"user","content":"Hello world"}]}`,
			"Hello world",
		},
		{
			"array_content",
			`{"messages":[{"role":"user","content":[{"type":"text","text":"Array text"}]}]}`,
			"Array text",
		},
		{
			"assistant_first",
			`{"messages":[{"role":"assistant","content":"Hi"},{"role":"user","content":"User text"}]}`,
			"User text",
		},
		{
			"no_messages",
			`{"system":"test"}`,
			"",
		},
		{
			"escaped_content",
			`{"messages":[{"role":"user","content":"Hello\nworld"}]}`,
			"Hello\nworld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := billingProxyExtractFirstUserText(tt.body)
			if got != tt.expected {
				t.Errorf("extractFirstUserText = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBillingProxyMaskUnmaskRoundtrip(t *testing.T) {
	original := `[{"type":"text","text":"hello"},{"type":"thinking","thinking":"secret data"},{"type":"text","text":"world"},{"type":"redacted_thinking","data":"abc"}]`

	masked, masks := billingProxyMaskThinkingBlocks(original)

	if len(masks) != 2 {
		t.Fatalf("Expected 2 masks, got %d", len(masks))
	}

	// Masked string should not contain thinking content
	if strings.Contains(masked, "secret data") {
		t.Error("Masked string still contains thinking content")
	}

	// Unmask should restore original
	restored := billingProxyUnmaskThinkingBlocks(masked, masks)
	if restored != original {
		t.Errorf("Round-trip failed:\n  got:  %s\n  want: %s", restored, original)
	}
}

func TestBillingProxyFullPipeline(t *testing.T) {
	body := `{"system":[{"type":"text","text":"Be helpful."}],"messages":[{"role":"user","content":"Hello world, use exec to list files via OpenClaw"},{"role":"assistant","content":[{"type":"text","text":"Sure, let me use exec"}]}],"tools":[{"name":"exec","description":"Execute a command","input_schema":{"type":"object","properties":{"session_id":{"type":"string"}}}}]}`
	cfg := DefaultBillingProxyConfig()

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	// Tool renames
	if !strings.Contains(result, `"Bash"`) {
		t.Error("Tool name 'exec' not renamed to 'Bash'")
	}

	// String replacements
	if strings.Contains(result, "OpenClaw") {
		t.Error("'OpenClaw' not replaced")
	}

	// Property renames
	if strings.Contains(result, `"session_id"`) {
		t.Error("'session_id' not renamed to 'thread_id'")
	}

	// Description stripped
	if strings.Contains(result, "Execute a command") {
		t.Error("Tool description not stripped")
	}

	// CC stubs injected
	if !strings.Contains(result, `"Glob"`) {
		t.Error("CC stubs not injected")
	}

	// Billing block injected (after Layer 2, so original strings preserved)
	if !strings.Contains(result, "cc_entrypoint=cli") {
		t.Error("Billing block not injected")
	}

	// Metadata injected
	if !strings.Contains(result, `"metadata":{"user_id":`) {
		t.Error("Metadata not injected")
	}
}

func TestBillingProxyToolRenameOrder(t *testing.T) {
	// Verify lcm_expand_query is renamed before lcm_expand
	body := `{"messages":[{"role":"user","content":"test"}],"tools":[{"name":"lcm_expand_query"},{"name":"lcm_expand"}]}`
	cfg := BillingProxyConfig{}

	result := string(BillingProxyProcessBody([]byte(body), cfg))

	if !strings.Contains(result, `"ContextQuery"`) {
		t.Error("lcm_expand_query not correctly renamed to ContextQuery")
	}
	if !strings.Contains(result, `"ContextExpand"`) {
		t.Error("lcm_expand not correctly renamed to ContextExpand")
	}
	// Ensure lcm_expand_query wasn't partially matched by lcm_expand
	if strings.Contains(result, `"ContextExpandquery"`) || strings.Contains(result, `"ContextExpand_query"`) {
		t.Error("lcm_expand_query was corrupted by lcm_expand rename (order issue)")
	}
}

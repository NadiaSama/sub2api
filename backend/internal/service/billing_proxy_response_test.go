package service

import (
	"strings"
	"testing"
)

func TestBillingProxyReverseMapNonStream(t *testing.T) {
	// Simulate a response with renamed tool names and properties
	input := `{"type":"tool_use","name":"Bash","input":{"thread_id":"123","cmd":"ls"}}`
	result := string(BillingProxyReverseMapNonStream([]byte(input)))

	if !strings.Contains(result, `"exec"`) {
		t.Error("Tool name 'Bash' not reversed to 'exec'")
	}
	if !strings.Contains(result, `"session_id"`) {
		t.Error("Property 'thread_id' not reversed to 'session_id'")
	}
}

func TestBillingProxyReverseMapToolNamesEscaped(t *testing.T) {
	// SSE input_json_delta embeds tool args with escaped quotes
	input := `data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"name\":\"Bash\",\"cmd\":\"ls\"}"}}`
	result := string(BillingProxyReverseMapNonStream([]byte(input)))

	if !strings.Contains(result, `\"exec\"`) {
		t.Error("Escaped tool name '\\\"Bash\\\"' not reversed to '\\\"exec\\\"'")
	}
}

func TestBillingProxyReverseMapStringReplacements(t *testing.T) {
	input := `{"content":"OCPlatform is running from the skillhub"}`
	result := string(BillingProxyReverseMapNonStream([]byte(input)))

	if !strings.Contains(result, "OpenClaw") {
		t.Error("'OCPlatform' not reversed to 'OpenClaw'")
	}
}

func TestBillingProxyReverseMapThinkingBlockProtection(t *testing.T) {
	input := `{"content":[{"type":"thinking","thinking":"Bash OCPlatform"},{"type":"text","text":"Bash OCPlatform"}]}`
	result := string(BillingProxyReverseMapNonStream([]byte(input)))

	// Thinking block should be preserved unchanged
	thinkIdx := strings.Index(result, `"type":"thinking"`)
	if thinkIdx == -1 {
		t.Fatal("thinking block not found in result")
	}

	// Find the thinking block's content
	blockStart := strings.LastIndex(result[:thinkIdx], "{")
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
	if !strings.Contains(thinkBlock, "Bash") {
		t.Error("Thinking block content was modified - 'Bash' should be preserved")
	}
	if !strings.Contains(thinkBlock, "OCPlatform") {
		t.Error("Thinking block content was modified - 'OCPlatform' should be preserved")
	}

	// Text block should be reverse-mapped
	textIdx := strings.Index(result, `"type":"text"`)
	if textIdx == -1 {
		t.Fatal("text block not found in result")
	}
	textBlock := result[textIdx:]
	if strings.Contains(textBlock, "OCPlatform") {
		t.Error("Text block content was NOT reverse-mapped - 'OCPlatform' should become 'OpenClaw'")
	}
}

func TestBillingProxyReverseMapSSEEvent(t *testing.T) {
	thinking := false

	// Non-thinking event: should be reverse-mapped
	event1 := `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

`
	result1 := BillingProxyReverseMapSSEEvent(event1, &thinking)
	if thinking {
		t.Error("Non-thinking content_block_start should not set thinking=true")
	}
	_ = result1

	// Thinking content_block_start: should pass through unchanged
	event2 := `event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"thinking","thinking":""}}

`
	result2 := BillingProxyReverseMapSSEEvent(event2, &thinking)
	if !thinking {
		t.Error("Thinking content_block_start should set thinking=true")
	}
	if result2 != event2 {
		t.Error("Thinking content_block_start should pass through unchanged")
	}

	// Thinking delta: should pass through unchanged
	event3 := `event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"Bash OCPlatform"}}

`
	result3 := BillingProxyReverseMapSSEEvent(event3, &thinking)
	if result3 != event3 {
		t.Error("Thinking delta should pass through unchanged")
	}

	// Content block stop (thinking): should pass through, reset state
	event4 := `event: content_block_stop
data: {"type":"content_block_stop","index":1}

`
	result4 := BillingProxyReverseMapSSEEvent(event4, &thinking)
	if thinking {
		t.Error("content_block_stop should reset thinking=false")
	}
	if result4 != event4 {
		t.Error("Thinking block stop should pass through unchanged")
	}

	// Text delta: should be reverse-mapped
	event5 := `event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"Use Bash tool"}}

`
	result5 := BillingProxyReverseMapSSEEvent(event5, &thinking)
	if strings.Contains(result5, `"Bash"`) && !strings.Contains(result5, `"exec"`) {
		t.Error("Non-thinking text_delta should have tool names reversed")
	}
}

func TestBillingProxyReverseMapSSEThinkingState(t *testing.T) {
	thinking := false

	// Start a text block
	e1 := `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n"
	BillingProxyReverseMapSSEEvent(e1, &thinking)
	if thinking {
		t.Error("Text block should not set thinking=true")
	}

	// Text delta with "Bash" - should be reversed
	e2 := `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Using \"Bash\" now"}}` + "\n\n"
	r2 := BillingProxyReverseMapSSEEvent(e2, &thinking)
	if strings.Contains(r2, `"Bash"`) {
		t.Error("Text delta 'Bash' should be reversed to 'exec'")
	}

	// Stop text block
	e3 := `data: {"type":"content_block_stop","index":0}` + "\n\n"
	BillingProxyReverseMapSSEEvent(e3, &thinking)

	// Start thinking block
	e4 := `data: {"type":"content_block_start","index":1,"content_block":{"type":"thinking","thinking":""}}` + "\n\n"
	BillingProxyReverseMapSSEEvent(e4, &thinking)
	if !thinking {
		t.Error("Thinking block start should set thinking=true")
	}

	// Thinking delta with "Bash" - should NOT be reversed
	e5 := `data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"Using \"Bash\" now"}}` + "\n\n"
	r5 := BillingProxyReverseMapSSEEvent(e5, &thinking)
	if r5 != e5 {
		t.Error("Thinking delta should pass through unchanged")
	}

	// Stop thinking block
	e6 := `data: {"type":"content_block_stop","index":1}` + "\n\n"
	BillingProxyReverseMapSSEEvent(e6, &thinking)
	if thinking {
		t.Error("After thinking block stop, thinking should be false")
	}
}

func TestBillingProxyReverseMapAllToolNames(t *testing.T) {
	// Some tool "originals" are themselves Layer 2 forward-mapped strings,
	// so after full reverse (tool reverse + string reverse), they become the
	// Layer 2 original. Build the expected result by chaining both reverses.
	reverseStringMap := make(map[string]string)
	for _, pair := range BillingProxyDefaultReverseMap {
		reverseStringMap[pair[0]] = pair[1]
	}

	for _, pair := range BillingProxyDefaultToolRenames {
		orig, renamed := pair[0], pair[1]
		input := `{"name":"` + renamed + `"}`
		result := string(BillingProxyReverseMapNonStream([]byte(input)))

		// After tool reverse: orig. After string reverse: possibly further mapped.
		expected := orig
		if rev, ok := reverseStringMap[orig]; ok {
			expected = rev
		}
		if !strings.Contains(result, `"`+expected+`"`) {
			t.Errorf("Tool name %q: expected %q in result, got %s", renamed, expected, result)
		}
	}
}

func TestBillingProxyReverseMapAllPropNames(t *testing.T) {
	// Test all property rename pairs reverse correctly
	for _, pair := range BillingProxyDefaultPropRenames {
		orig, renamed := pair[0], pair[1]
		input := `{"` + renamed + `":"value"}`
		result := string(BillingProxyReverseMapNonStream([]byte(input)))
		if !strings.Contains(result, `"` + orig + `"`) {
			t.Errorf("Property name %q not reversed to %q", renamed, orig)
		}
	}
}

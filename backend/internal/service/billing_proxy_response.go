package service

import (
	"strings"
)

// BillingProxyReverseMapNonStream applies reverse mapping to a non-streaming response body.
// Thinking blocks are protected via mask/unmask to preserve byte-equality.
func BillingProxyReverseMapNonStream(body []byte) []byte {
	s := string(body)

	// Mask thinking blocks so reverseMap can't mutate them
	masked, masks := billingProxyMaskThinkingBlocks(s)
	reversed := billingProxyReverseMap(masked)
	result := billingProxyUnmaskThinkingBlocks(reversed, masks)

	return []byte(result)
}

// BillingProxyReverseMapSSEEvent applies reverse mapping to a single SSE event.
// It tracks thinking block state across events to avoid modifying thinking content.
//
// currentBlockIsThinking should be initialized to false and passed across events.
func BillingProxyReverseMapSSEEvent(event string, currentBlockIsThinking *bool) string {
	// Find the data: line
	dataIdx := -1
	if strings.HasPrefix(event, "data: ") {
		dataIdx = 0
	} else {
		idx := strings.Index(event, "\ndata: ")
		if idx != -1 {
			dataIdx = idx + 1
		}
	}

	if dataIdx == -1 {
		return billingProxyReverseMap(event)
	}

	// Extract the data payload
	dataLineEnd := strings.Index(event[dataIdx+6:], "\n")
	var dataStr string
	if dataLineEnd == -1 {
		dataStr = event[dataIdx+6:]
	} else {
		dataStr = event[dataIdx+6 : dataIdx+6+dataLineEnd]
	}

	// Check for content_block_start events
	if strings.Contains(dataStr, `"type":"content_block_start"`) {
		if strings.Contains(dataStr, `"content_block":{"type":"thinking"`) ||
			strings.Contains(dataStr, `"content_block":{"type":"redacted_thinking"`) {
			*currentBlockIsThinking = true
			return event // pass through unchanged
		}
		*currentBlockIsThinking = false
		return billingProxyReverseMap(event)
	}

	// Check for content_block_stop events
	if strings.Contains(dataStr, `"type":"content_block_stop"`) {
		wasThinking := *currentBlockIsThinking
		*currentBlockIsThinking = false
		if wasThinking {
			return event
		}
		return billingProxyReverseMap(event)
	}

	// Inside a thinking block: pass through unchanged
	if *currentBlockIsThinking {
		return event
	}

	return billingProxyReverseMap(event)
}

// billingProxyReverseMap applies the three-phase reverse mapping to text.
// Phase 1: Reverse tool names (both plain and escaped quote forms)
// Phase 2: Reverse property names (both plain and escaped quote forms)
// Phase 3: Reverse string replacements
func billingProxyReverseMap(text string) string {
	r := text

	// Reverse tool names first (more specific patterns)
	// Handle BOTH plain ("Name") AND escaped (\"Name\") forms for SSE compatibility
	for _, pair := range BillingProxyDefaultToolRenames {
		r = strings.ReplaceAll(r, `"`+pair[1]+`"`, `"`+pair[0]+`"`)
		r = strings.ReplaceAll(r, `\"`+pair[1]+`\"`, `\"`+pair[0]+`\"`)
	}

	// Reverse property names — same dual handling
	for _, pair := range BillingProxyDefaultPropRenames {
		r = strings.ReplaceAll(r, `"`+pair[1]+`"`, `"`+pair[0]+`"`)
		r = strings.ReplaceAll(r, `\"`+pair[1]+`\"`, `\"`+pair[0]+`\"`)
	}

	// Reverse string replacements
	for _, pair := range BillingProxyDefaultReverseMap {
		r = strings.ReplaceAll(r, pair[0], pair[1])
	}

	return r
}

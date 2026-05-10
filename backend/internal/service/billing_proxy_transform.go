package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// BillingProxyConfig holds toggle switches for optional transform layers.
type BillingProxyConfig struct {
	StripSystemConfig             bool // Layer 4: strip OC config section from system prompt
	StripToolDescriptions         bool // Layer 5: strip tool descriptions
	InjectCCStubs                 bool // Layer 7: inject CC tool stubs
	StripTrailingAssistantPrefill bool // Layer 8: remove trailing assistant messages
}

// DefaultBillingProxyConfig returns a config with all layers enabled (matching proxy.js defaults).
func DefaultBillingProxyConfig() BillingProxyConfig {
	return BillingProxyConfig{
		StripSystemConfig:             true,
		StripToolDescriptions:         true,
		InjectCCStubs:                 true,
		StripTrailingAssistantPrefill: true,
	}
}

// BillingProxyProcessBody applies the full 8-layer transformation pipeline to the request body.
// This matches proxy.js processBody() exactly.
func BillingProxyProcessBody(body []byte, cfg BillingProxyConfig) []byte {
	m := string(body)

	// Mask thinking/redacted_thinking blocks to protect from transform corruption
	masked, masks := billingProxyMaskThinkingBlocks(m)
	m = masked

	// Layer 2: String trigger sanitization (global replacement)
	for _, pair := range BillingProxyDefaultReplacements {
		m = strings.ReplaceAll(m, pair[0], pair[1])
	}

	// Layer 3: Tool name fingerprint bypass (quoted replacement)
	for _, pair := range BillingProxyDefaultToolRenames {
		m = strings.ReplaceAll(m, `"`+pair[0]+`"`, `"`+pair[1]+`"`)
	}

	// Layer 6: Property name renaming (quoted replacement)
	for _, pair := range BillingProxyDefaultPropRenames {
		m = strings.ReplaceAll(m, `"`+pair[0]+`"`, `"`+pair[1]+`"`)
	}

	// Layer 4: System prompt template bypass
	if cfg.StripSystemConfig {
		m = billingProxyStripSystemConfig(m)
	}

	// Layer 5: Tool description stripping + Layer 7: CC Tool Stubs injection
	if cfg.StripToolDescriptions {
		m = billingProxyStripToolDescriptions(m, cfg.InjectCCStubs)
	} else if cfg.InjectCCStubs {
		m = billingProxyInjectCCStubs(m)
	}

	// Layer 1: Billing header injection (dynamic fingerprint per request)
	m = billingProxyInjectBillingBlock(m)

	// Metadata injection: device_id + session_id matching real CC format
	m = billingProxyInjectMetadata(m)

	// Layer 8: Strip trailing assistant prefill
	if cfg.StripTrailingAssistantPrefill {
		m = billingProxyStripTrailingAssistant(m)
	}

	// Unmask thinking blocks
	m = billingProxyUnmaskThinkingBlocks(m, masks)

	return []byte(m)
}

// billingProxyStripSystemConfig implements Layer 4: strip OC config section from system prompt.
func billingProxyStripSystemConfig(m string) string {
	// Anchor search to the system array so we don't match conversation history
	sysArrayStart := strings.Index(m, `"system":[`)
	searchFrom := 0
	if sysArrayStart != -1 {
		searchFrom = sysArrayStart
	}

	configStart := strings.Index(m[searchFrom:], BillingProxyIdentityMarker)
	if configStart == -1 {
		return m
	}
	configStart += searchFrom

	stripFrom := configStart
	if stripFrom >= 2 && m[stripFrom-2] == '\\' && m[stripFrom-1] == 'n' {
		stripFrom -= 2
	}

	// Find end of config: first workspace doc header (filesystem path pattern)
	afterMarker := configStart + len(BillingProxyIdentityMarker)
	configEnd := strings.Index(m[afterMarker:], `\n## /`)
	if configEnd == -1 {
		configEnd = strings.Index(m[afterMarker:], `\n## C:\\`)
		if configEnd == -1 {
			return m
		}
	}
	boundary := afterMarker + configEnd

	strippedLen := boundary - stripFrom
	if strippedLen > 1000 {
		m = m[:stripFrom] + BillingProxySystemParaphrase + m[boundary:]
	}
	return m
}

// billingProxyStripToolDescriptions implements Layer 5: strip tool descriptions.
func billingProxyStripToolDescriptions(m string, injectStubs bool) string {
	toolsIdx := strings.Index(m, `"tools":[`)
	if toolsIdx == -1 {
		return m
	}

	toolsArrayStart := toolsIdx + len(`"tools":`)
	toolsEndIdx := billingProxyFindMatchingBracket(m, toolsArrayStart)
	if toolsEndIdx == -1 {
		return m
	}

	section := m[toolsIdx : toolsEndIdx+1]

	// Strip description values
	from := 0
	for {
		d := strings.Index(section[from:], `"description":"`)
		if d == -1 {
			break
		}
		d += from
		vs := d + len(`"description":"`)
		i := vs
		for i < len(section) {
			if section[i] == '\\' && i+1 < len(section) {
				i += 2
				continue
			}
			if section[i] == '"' {
				break
			}
			i++
		}
		section = section[:vs] + section[i:]
		from = vs + 1
	}

	// Inject CC tool stubs
	if injectStubs {
		insertAt := len(`"tools":[`)
		section = section[:insertAt] + strings.Join(BillingProxyCCToolStubs, ",") + "," + section[insertAt:]
	}

	m = m[:toolsIdx] + section + m[toolsEndIdx+1:]
	return m
}

// billingProxyInjectCCStubs injects CC tool stubs without stripping descriptions.
func billingProxyInjectCCStubs(m string) string {
	toolsIdx := strings.Index(m, `"tools":[`)
	if toolsIdx == -1 {
		return m
	}
	insertAt := toolsIdx + len(`"tools":[`)
	return m[:insertAt] + strings.Join(BillingProxyCCToolStubs, ",") + "," + m[insertAt:]
}

// billingProxyInjectBillingBlock implements Layer 1: billing header injection.
func billingProxyInjectBillingBlock(m string) string {
	billingBlock := billingProxyBuildBillingBlock(m)

	sysArrayIdx := strings.Index(m, `"system":[`)
	if sysArrayIdx != -1 {
		insertAt := sysArrayIdx + len(`"system":[`)
		return m[:insertAt] + billingBlock + "," + m[insertAt:]
	}

	// Handle "system":"string" format
	sysStrIdx := strings.Index(m, `"system":"`)
	if sysStrIdx != -1 {
		i := sysStrIdx + len(`"system":"`)
		for i < len(m) {
			if m[i] == '\\' {
				i += 2
				continue
			}
			if m[i] == '"' {
				break
			}
			i++
		}
		sysEnd := i + 1
		originalSysStr := m[sysStrIdx+len(`"system":`):sysEnd]
		return m[:sysStrIdx] +
			`"system":[` + billingBlock + `,{"type":"text","text":` + originalSysStr + `}]` +
			m[sysEnd:]
	}

	// No system field: create one
	return `{"system":[` + billingBlock + `],` + m[1:]
}

// billingProxyBuildBillingBlock builds the billing attribution block JSON string.
func billingProxyBuildBillingBlock(bodyStr string) string {
	firstText := billingProxyExtractFirstUserText(bodyStr)
	fp := billingProxyComputeFingerprint(firstText)
	ccVersion := BillingProxyCCVersion + "." + fp
	return `{"type":"text","text":"x-anthropic-billing-header: cc_version=` + ccVersion + `; cc_entrypoint=cli; cch=00000;"}`
}

// billingProxyExtractFirstUserText extracts the first user message text using string scanning.
// Matches proxy.js extractFirstUserText behavior.
func billingProxyExtractFirstUserText(bodyStr string) string {
	msgsIdx := strings.Index(bodyStr, `"messages":[`)
	if msgsIdx == -1 {
		return ""
	}
	userIdx := strings.Index(bodyStr[msgsIdx:], `"role":"user"`)
	if userIdx == -1 {
		return ""
	}
	userIdx += msgsIdx

	contentIdx := strings.Index(bodyStr[userIdx:], `"content"`)
	if contentIdx == -1 || contentIdx > 500 {
		return ""
	}
	contentIdx += userIdx

	// After "content" there's a colon, possibly spaces, then the value
	afterContent := contentIdx + len(`"content"`) + 1 // skip ':'
	if afterContent >= len(bodyStr) {
		return ""
	}

	ch := bodyStr[afterContent]
	if ch == '"' {
		// Simple string content: "content":"text here"
		textStart := afterContent + 1
		end := textStart
		for end < len(bodyStr) {
			if bodyStr[end] == '\\' {
				end += 2
				continue
			}
			if bodyStr[end] == '"' {
				break
			}
			end++
		}
		return billingProxyUnescapeJSON(bodyStr[textStart:end])
	}

	// Array content: find first text block
	textIdx := strings.Index(bodyStr[contentIdx:], `"text":"`)
	if textIdx == -1 || textIdx > 2000 {
		return ""
	}
	textStart := contentIdx + textIdx + len(`"text":"`)
	end := textStart
	for end < len(bodyStr) {
		if bodyStr[end] == '\\' {
			end += 2
			continue
		}
		if bodyStr[end] == '"' {
			break
		}
		end++
	}
	maxEnd := textStart + 50
	if end > maxEnd {
		end = maxEnd
	}
	return billingProxyUnescapeJSON(bodyStr[textStart:end])
}

// billingProxyUnescapeJSON decodes basic JSON escape sequences for fingerprint extraction.
func billingProxyUnescapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

// billingProxyComputeFingerprint computes the 3-char SHA256 fingerprint.
// Algorithm: SHA256(salt + chars_at_indices + version)[:3] — matches proxy.js and CC CLI.
func billingProxyComputeFingerprint(firstUserText string) string {
	indices := []int{4, 7, 20}
	chars := make([]byte, 0, 3)
	for _, i := range indices {
		if i < len(firstUserText) {
			chars = append(chars, firstUserText[i])
		} else {
			chars = append(chars, '0')
		}
	}
	sum := sha256.Sum256([]byte(fingerprintSalt + string(chars) + BillingProxyCCVersion))
	return hex.EncodeToString(sum[:])[:3]
}

// billingProxyInjectMetadata injects device_id + session_id into metadata.
func billingProxyInjectMetadata(m string) string {
	deviceID := billingProxyDeviceID()
	sessionID := billingProxySessionID()
	metaValue, _ := json.Marshal(map[string]string{
		"device_id":  deviceID,
		"session_id": sessionID,
	})
	metaJson := `"metadata":{"user_id":` + strconv.Quote(string(metaValue)) + `}`

	existingMeta := strings.Index(m, `"metadata":{`)
	if existingMeta != -1 {
		// Find end of existing metadata object
		depth := 0
		mi := existingMeta + len(`"metadata":`)
		for ; mi < len(m); mi++ {
			if m[mi] == '{' {
				depth++
			} else if m[mi] == '}' {
				depth--
				if depth == 0 {
					mi++
					break
				}
			}
		}
		return m[:existingMeta] + metaJson + m[mi:]
	}

	// Insert after opening brace
	return "{" + metaJson + "," + m[1:]
}

// billingProxyStripTrailingAssistant implements Layer 8: strip trailing assistant prefill.
func billingProxyStripTrailingAssistant(m string) string {
	msgsIdx := strings.Index(m, `"messages":[`)
	if msgsIdx == -1 {
		return m
	}

	arrayStart := msgsIdx + len(`"messages":[`)

	// Forward-scan to find all top-level message objects
	type msgPos struct {
		start, end int
	}
	var positions []msgPos
	depth := 0
	inStr := false
	objStart := -1

	for i := arrayStart; i < len(m); i++ {
		c := m[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			continue
		}
		if c == '{' {
			if depth == 0 {
				objStart = i
			}
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 && objStart != -1 {
				positions = append(positions, msgPos{start: objStart, end: i})
				objStart = -1
			}
		} else if c == ']' && depth == 0 {
			break
		}
	}

	// Pop trailing assistant messages
	popped := 0
	for len(positions) > 0 {
		last := positions[len(positions)-1]
		obj := m[last.start : last.end+1]
		if !strings.Contains(obj, `"role":"assistant"`) {
			break
		}

		stripFrom := last.start
		for i := last.start - 1; i >= arrayStart; i-- {
			if m[i] == ',' {
				stripFrom = i
				break
			}
			if m[i] != ' ' && m[i] != '\n' && m[i] != '\r' && m[i] != '\t' {
				break
			}
		}
		m = m[:stripFrom] + m[last.end+1:]
		positions = positions[:len(positions)-1]
		popped++
	}

	return m
}

// billingProxyFindMatchingBracket finds the matching closing bracket for '[' at position start.
// String-aware: skips brackets inside JSON string values.
func billingProxyFindMatchingBracket(str string, start int) int {
	d := 0
	inStr := false
	for i := start; i < len(str); i++ {
		c := str[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			continue
		}
		if c == '[' {
			d++
		} else if c == ']' {
			d--
			if d == 0 {
				return i
			}
		}
	}
	return -1
}

// billingProxyMaskThinkingBlocks masks thinking/redacted_thinking content blocks
// with unique placeholders to protect them from transform corruption.
func billingProxyMaskThinkingBlocks(m string) (string, []string) {
	patterns := []string{`{"type":"thinking"`, `{"type":"redacted_thinking"`}
	var masks []string
	var out strings.Builder
	i := 0

	for i < len(m) {
		nextIdx := -1
		for _, p := range patterns {
			idx := strings.Index(m[i:], p)
			if idx != -1 {
				absIdx := i + idx
				if nextIdx == -1 || absIdx < nextIdx {
					nextIdx = absIdx
				}
			}
		}
		if nextIdx == -1 {
			out.WriteString(m[i:])
			break
		}

		out.WriteString(m[i:nextIdx])

		// String-aware brace scan to find end of thinking block
		depth := 0
		inStr := false
		j := nextIdx
		for j < len(m) {
			c := m[j]
			if inStr {
				if c == '\\' {
					j += 2
					continue
				}
				if c == '"' {
					inStr = false
				}
				j++
				continue
			}
			if c == '"' {
				inStr = true
				j++
				continue
			}
			if c == '{' {
				depth++
				j++
				continue
			}
			if c == '}' {
				depth--
				j++
				if depth == 0 {
					break
				}
				continue
			}
			j++
		}

		if depth != 0 {
			// Malformed / truncated — bail without masking the rest
			out.WriteString(m[nextIdx:])
			return out.String(), masks
		}

		masks = append(masks, m[nextIdx:j])
		out.WriteString(billingProxyThinkMaskPrefix)
		out.WriteString(fmt.Sprintf("%d", len(masks)-1))
		out.WriteString(billingProxyThinkMaskSuffix)
		i = j
	}

	return out.String(), masks
}

// billingProxyUnmaskThinkingBlocks restores masked thinking blocks.
func billingProxyUnmaskThinkingBlocks(m string, masks []string) string {
	for i, mask := range masks {
		placeholder := billingProxyThinkMaskPrefix + fmt.Sprintf("%d", i) + billingProxyThinkMaskSuffix
		m = strings.ReplaceAll(m, placeholder, mask)
	}
	return m
}

// billingProxyDeviceID returns a persistent per-process device ID (64 hex chars).
var billingProxyDeviceIDValue string

func billingProxyDeviceID() string {
	if billingProxyDeviceIDValue == "" {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		billingProxyDeviceIDValue = hex.EncodeToString(b)
	}
	return billingProxyDeviceIDValue
}

// billingProxySessionID returns a persistent per-process session UUID.
var billingProxySessionIDValue string

func billingProxySessionID() string {
	if billingProxySessionIDValue == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		// Format as UUID v4
		b[6] = (b[6] & 0x0f) | 0x40
		b[8] = (b[8] & 0x3f) | 0x80
		billingProxySessionIDValue = fmt.Sprintf("%x-%x-%x-%x-%x",
			b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	}
	return billingProxySessionIDValue
}

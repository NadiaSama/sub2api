package service

import (
	"bytes"
	"context"
	"net/http"
	"runtime"
	"strings"
)

const billingProxyModeKey = "billing_proxy_mode"
const billingProxyThinkingStateKey = "billing_proxy_thinking_state"

// getBillingProxyConfigFromAccount builds BillingProxyConfig from account Extra fields.
func getBillingProxyConfigFromAccount(account *Account) BillingProxyConfig {
	return BillingProxyConfig{
		StripSystemConfig:             account.GetExtraBool("strip_system_config", true),
		StripToolDescriptions:         account.GetExtraBool("strip_tool_descriptions", true),
		InjectCCStubs:                 account.GetExtraBool("inject_cc_stubs", true),
		StripTrailingAssistantPrefill: account.GetExtraBool("strip_trailing_assistant_prefill", true),
	}
}

// isBillingProxyMode checks if the current request is in billing_proxy mode.
func isBillingProxyMode(c interface{ Get(string) (any, bool) }) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(billingProxyModeKey)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// billingProxyGetThinkingState retrieves or creates the thinking state tracker from gin context.
func billingProxyGetThinkingState(c interface {
	Get(string) (any, bool)
	Set(string, any)
}) *bool {
	if v, ok := c.Get(billingProxyThinkingStateKey); ok {
		if state, ok := v.(*bool); ok {
			return state
		}
	}
	state := new(bool)
	c.Set(billingProxyThinkingStateKey, state)
	return state
}

// reverseIfBillingProxy applies billing proxy reverse mapping to a response chunk.
// For non-billing-proxy requests, falls through to reverseToolNamesIfPresent.
// For streaming events, it tracks thinking block state across calls.
func reverseIfBillingProxy(c interface {
	Get(string) (any, bool)
	Set(string, any)
}, chunk []byte, isStreaming bool) []byte {
	if !isBillingProxyMode(c) {
		return reverseToolNamesIfPresent(c, chunk)
	}

	if isStreaming {
		state := billingProxyGetThinkingState(c)
		return []byte(BillingProxyReverseMapSSEEvent(string(chunk), state))
	}
	return BillingProxyReverseMapNonStream(chunk)
}

// buildUpstreamRequestBillingProxy constructs the HTTP request for billing_proxy accounts.
// It strips client headers and injects CC-style headers matching proxy.js behavior.
func (s *GatewayService) buildUpstreamRequestBillingProxy(ctx context.Context, c interface {
	Get(string) (any, bool)
}, account *Account, body []byte, token, modelID string) (*http.Request, error) {
	targetURL := claudeAPIURL

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Set authorization
	setHeaderRaw(req.Header, "authorization", "Bearer "+token)

	// Set content type
	setHeaderRaw(req.Header, "content-type", "application/json")
	setHeaderRaw(req.Header, "accept-encoding", "identity")
	setHeaderRaw(req.Header, "anthropic-version", "2023-06-01")

	// Inject CC identity headers (matching proxy.js getStainlessHeaders)
	setHeaderRaw(req.Header, "user-agent", BillingProxyUserAgent())
	setHeaderRaw(req.Header, "x-app", "cli")
	setHeaderRaw(req.Header, "x-claude-code-session-id", billingProxySessionID())
	setHeaderRaw(req.Header, "x-stainless-arch", billingProxyArch())
	setHeaderRaw(req.Header, "x-stainless-lang", "js")
	setHeaderRaw(req.Header, "x-stainless-os", billingProxyOS())
	setHeaderRaw(req.Header, "x-stainless-package-version", BillingProxyStainlessPackageVersion)
	setHeaderRaw(req.Header, "x-stainless-runtime", "node")
	setHeaderRaw(req.Header, "x-stainless-runtime-version", "v22.11.0")
	setHeaderRaw(req.Header, "x-stainless-retry-count", "0")
	setHeaderRaw(req.Header, "x-stainless-timeout", "600")
	setHeaderRaw(req.Header, "anthropic-dangerous-direct-browser-access", "true")

	// Set anthropic-beta with all required betas
	setHeaderRaw(req.Header, "anthropic-beta", strings.Join(BillingProxyRequiredBetas, ","))

	return req, nil
}

// billingProxyOS returns the OS name matching proxy.js convention.
func billingProxyOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

// billingProxyArch returns the architecture matching proxy.js convention.
func billingProxyArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

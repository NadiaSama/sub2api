package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newCLIProxyAccount 构造一个用于测试的 CliProxyAPI 账号。
func newCLIProxyAccount() *Account {
	return &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeCLIProxy,
		Credentials: map[string]any{
			"base_url": "http://localhost:7861",
			"api_key":  "sk-cliproxy-upstream",
		},
	}
}

// newAPIKeyPassthroughAccount 构造一个用于回归对比的 Anthropic APIKey passthrough 账号。
// build 函数只看 IsCLIProxy() 来 bifurcate，因此 APIKey 路径只需 Type == AccountTypeAPIKey。
func newAPIKeyPassthroughAccount() *Account {
	return &Account{
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-anthropic"},
		Extra:       map[string]any{"anthropic_passthrough": true},
	}
}

// newTestGatewayService 构造一个允许 http base_url 的 GatewayService。
func newTestGatewayService() *GatewayService {
	return &GatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{
					Enabled:           false,
					AllowInsecureHTTP: true,
				},
			},
		},
	}
}

func newGinCtxWithRequest(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c
}

// TestCLIProxyHeaderPassthrough_BusinessHeaders 验证：业务/应用层 Header 全透传。
// 包括白名单外（X-Custom-*）和模拟未来 Claude Code 新增的 Header。
func TestCLIProxyHeaderPassthrough_BusinessHeaders(t *testing.T) {
	c := newGinCtxWithRequest(t)
	c.Request.Header.Set("Authorization", "Bearer sub2api-key")
	c.Request.Header.Set("X-Api-Key", "sub2api-x-api-key")
	c.Request.Header.Set("X-Goog-Api-Key", "sub2api-goog-key")
	c.Request.Header.Set("Cookie", "sid=abc")
	c.Request.Header.Set("Anthropic-Beta", "client-beta")
	c.Request.Header.Set("Anthropic-Version", "2024-01-01")
	c.Request.Header.Set("Accept-Encoding", "gzip")
	c.Request.Header.Set("X-Stainless-Package-Version", "0.74.0")
	c.Request.Header.Set("X-Custom-Trace", "trace-1")
	c.Request.Header.Set("X-Future-Claude-Code-Header", "future-value")

	svc := newTestGatewayService()
	account := newCLIProxyAccount()

	req, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, []byte(`{}`), "sk-cliproxy-upstream",
	)
	require.NoError(t, err)

	require.Equal(t, "sk-cliproxy-upstream", getHeaderRaw(req.Header, "x-api-key"))
	require.Equal(t, "", getHeaderRaw(req.Header, "authorization"))
	require.Equal(t, "", getHeaderRaw(req.Header, "x-goog-api-key"))
	require.Equal(t, "", getHeaderRaw(req.Header, "cookie"))

	require.Equal(t, "client-beta", getHeaderRaw(req.Header, "anthropic-beta"),
		"Anthropic-Beta 不被合并、不补 oauth/interleaved")
	require.Equal(t, "2024-01-01", getHeaderRaw(req.Header, "anthropic-version"),
		"客户端带的 Anthropic-Version 不被覆盖")
	require.Equal(t, "gzip", getHeaderRaw(req.Header, "accept-encoding"),
		"Accept-Encoding 保留，不再 strip")
	require.Equal(t, "0.74.0", getHeaderRaw(req.Header, "x-stainless-package-version"))
	require.Equal(t, "trace-1", getHeaderRaw(req.Header, "x-custom-trace"),
		"白名单外的自定义 Header 必须透传")
	require.Equal(t, "future-value", getHeaderRaw(req.Header, "x-future-claude-code-header"),
		"未知 Header 也必须透传（这是去白名单的核心动机）")
}

// TestCLIProxyHeaderPassthrough_NoDefaultsInjected 验证：入站不带 Content-Type / Anthropic-Version
// 时，outbound 也不应该被 Sub2API 补齐。
func TestCLIProxyHeaderPassthrough_NoDefaultsInjected(t *testing.T) {
	c := newGinCtxWithRequest(t)
	// 故意不设置 Content-Type / Anthropic-Version。
	c.Request.Header.Set("X-Marker", "no-defaults")

	svc := newTestGatewayService()
	account := newCLIProxyAccount()

	req, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, []byte(`{}`), "sk-cliproxy-upstream",
	)
	require.NoError(t, err)

	require.Equal(t, "", getHeaderRaw(req.Header, "content-type"),
		"CLIProxy 路径不再兜底 content-type")
	require.Equal(t, "", getHeaderRaw(req.Header, "anthropic-version"),
		"CLIProxy 路径不再兜底 anthropic-version")
	require.Equal(t, "no-defaults", getHeaderRaw(req.Header, "x-marker"))
}

// TestCLIProxyHeaderPassthrough_AuthLeakProtection 验证：仅入站 Authorization
// （map key 为小写 "authorization"）也必须彻底清除——防回归 raw-header 删除陷阱。
func TestCLIProxyHeaderPassthrough_AuthLeakProtection(t *testing.T) {
	c := newGinCtxWithRequest(t)
	c.Request.Header.Set("Authorization", "Bearer should-be-deleted")
	// 故意不设置 x-api-key——只让 Authorization 走 raw-header 路径。

	svc := newTestGatewayService()
	account := newCLIProxyAccount()

	req, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, []byte(`{}`), "sk-cliproxy-upstream",
	)
	require.NoError(t, err)

	// 直接访问 map，绕过 canonical 化——专门覆盖 reviewer 指出的陷阱。
	require.Empty(t, req.Header["Authorization"], "canonical 形式不应存在")
	require.Empty(t, req.Header["authorization"], "raw 小写形式同样不应存在（这正是 delHeaderRaw 防的回归）")
	require.Equal(t, "", req.Header.Get("Authorization"))
	require.Equal(t, "sk-cliproxy-upstream", getHeaderRaw(req.Header, "x-api-key"))
}

// TestCLIProxyHeaderPassthrough_HopByHopStrip 验证：RFC 7230 §6.1 hop-by-hop
// Header 全部 strip，且 Connection value 列出的动态 hop-by-hop 字段也被 strip。
func TestCLIProxyHeaderPassthrough_HopByHopStrip(t *testing.T) {
	c := newGinCtxWithRequest(t)
	c.Request.Header.Set("Connection", "close, X-Custom-Hop")
	c.Request.Header.Set("Keep-Alive", "timeout=5")
	c.Request.Header.Set("Proxy-Authenticate", "Basic realm=foo")
	c.Request.Header.Set("Proxy-Authorization", "Basic xxx")
	c.Request.Header.Set("Proxy-Connection", "close")
	c.Request.Header.Set("TE", "trailers")
	c.Request.Header.Set("Trailer", "Expires")
	c.Request.Header.Set("Transfer-Encoding", "chunked")
	c.Request.Header.Set("Upgrade", "websocket")
	c.Request.Header.Set("X-Custom-Hop", "should-be-stripped")
	c.Request.Header.Set("X-Stay", "kept")

	svc := newTestGatewayService()
	account := newCLIProxyAccount()

	req, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, []byte(`{}`), "sk-cliproxy-upstream",
	)
	require.NoError(t, err)

	for _, k := range []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Proxy-Connection", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
	} {
		require.Empty(t, req.Header.Values(k), "hop-by-hop %q 必须 strip", k)
	}
	require.Empty(t, req.Header.Values("X-Custom-Hop"),
		"Connection value 列出的动态 hop-by-hop 字段必须 strip")
	require.Equal(t, "kept", req.Header.Get("X-Stay"),
		"非 hop-by-hop 业务 Header 不受影响")
}

// TestAPIKeyPassthrough_RegressionStillWhitelisted 回归：普通 Anthropic APIKey
// passthrough 账号必须保持原有白名单 + 兜底语义，不能被 CLIProxy 路径污染。
func TestAPIKeyPassthrough_RegressionStillWhitelisted(t *testing.T) {
	c := newGinCtxWithRequest(t)
	c.Request.Header.Set("Authorization", "Bearer sub2api-key")
	c.Request.Header.Set("Cookie", "sid=abc")
	c.Request.Header.Set("X-Custom-Trace", "trace-1")
	c.Request.Header.Set("X-Stainless-Package-Version", "0.74.0")
	// 故意不设 anthropic-version / content-type——验证兜底仍生效。

	svc := newTestGatewayService()
	account := newAPIKeyPassthroughAccount()

	req, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, []byte(`{}`), "sk-anthropic",
	)
	require.NoError(t, err)

	require.Equal(t, "sk-anthropic", getHeaderRaw(req.Header, "x-api-key"))
	require.Equal(t, "", getHeaderRaw(req.Header, "authorization"))
	require.Equal(t, "", getHeaderRaw(req.Header, "cookie"))
	require.Equal(t, "", getHeaderRaw(req.Header, "x-custom-trace"),
		"APIKey passthrough 路径必须维持白名单——X-Custom-* 不透传")
	require.Equal(t, "0.74.0", getHeaderRaw(req.Header, "x-stainless-package-version"),
		"白名单内的 stainless Header 仍透传")
	require.Equal(t, "application/json", getHeaderRaw(req.Header, "content-type"),
		"APIKey passthrough 缺失 content-type 时补齐")
	require.Equal(t, "2023-06-01", getHeaderRaw(req.Header, "anthropic-version"),
		"APIKey passthrough 缺失 anthropic-version 时补齐")
}

// TestCLIProxyHeaderPassthrough_CountTokensParity 验证 count_tokens 路径与
// /v1/messages 行为一致——这正是 V1.2 mapping bug 教训的同类陷阱。
func TestCLIProxyHeaderPassthrough_CountTokensParity(t *testing.T) {
	c := newGinCtxWithRequest(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
	c.Request.Header.Set("Authorization", "Bearer sub2api-key")
	c.Request.Header.Set("Cookie", "sid=abc")
	c.Request.Header.Set("X-Custom-Trace", "ct-1")
	c.Request.Header.Set("X-Future-Header", "future-ct")
	c.Request.Header.Set("Connection", "close")

	svc := newTestGatewayService()
	account := newCLIProxyAccount()

	req, err := svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, []byte(`{}`), "sk-cliproxy-upstream",
	)
	require.NoError(t, err)

	require.Equal(t, "http://localhost:7861/v1/messages/count_tokens?beta=true", req.URL.String())
	require.Equal(t, "sk-cliproxy-upstream", getHeaderRaw(req.Header, "x-api-key"))
	require.Equal(t, "", getHeaderRaw(req.Header, "authorization"))
	require.Equal(t, "", getHeaderRaw(req.Header, "cookie"),
		"count_tokens 也必须删除 Cookie，避免跨边界泄漏会话凭据")
	require.Equal(t, "ct-1", getHeaderRaw(req.Header, "x-custom-trace"))
	require.Equal(t, "future-ct", getHeaderRaw(req.Header, "x-future-header"))
	require.Empty(t, req.Header.Values("Connection"), "hop-by-hop 必须 strip")
	require.Equal(t, "", getHeaderRaw(req.Header, "content-type"),
		"count_tokens 同样不再兜底 content-type")
	require.Equal(t, "", getHeaderRaw(req.Header, "anthropic-version"),
		"count_tokens 同样不再兜底 anthropic-version")
}

// TestAPIKeyPassthrough_CountTokensRegression 回归：count_tokens 路径下，APIKey
// passthrough 账号必须维持白名单 + 兜底语义。
func TestAPIKeyPassthrough_CountTokensRegression(t *testing.T) {
	c := newGinCtxWithRequest(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
	c.Request.Header.Set("Authorization", "Bearer sub2api-key")
	c.Request.Header.Set("Cookie", "sid=abc")
	c.Request.Header.Set("X-Custom-Trace", "ct-1")

	svc := newTestGatewayService()
	account := newAPIKeyPassthroughAccount()

	req, err := svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, []byte(`{}`), "sk-anthropic",
	)
	require.NoError(t, err)

	require.Equal(t, "sk-anthropic", getHeaderRaw(req.Header, "x-api-key"))
	require.Equal(t, "", getHeaderRaw(req.Header, "cookie"))
	require.Equal(t, "", getHeaderRaw(req.Header, "x-custom-trace"),
		"APIKey passthrough count_tokens 维持白名单")
	require.Equal(t, "application/json", getHeaderRaw(req.Header, "content-type"))
	require.Equal(t, "2023-06-01", getHeaderRaw(req.Header, "anthropic-version"))
}

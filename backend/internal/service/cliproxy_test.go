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

func TestAccount_IsCLIProxy(t *testing.T) {
	t.Run("Anthropic cliproxy 命中", func(t *testing.T) {
		a := &Account{Platform: PlatformAnthropic, Type: AccountTypeCLIProxy}
		require.True(t, a.IsCLIProxy())
	})

	t.Run("非 Anthropic 不命中", func(t *testing.T) {
		a := &Account{Platform: PlatformOpenAI, Type: AccountTypeCLIProxy}
		require.False(t, a.IsCLIProxy())
	})

	t.Run("Anthropic 其他类型不命中", func(t *testing.T) {
		a := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
		require.False(t, a.IsCLIProxy())
	})

	t.Run("nil receiver 不 panic 且返回 false", func(t *testing.T) {
		var a *Account
		require.False(t, a.IsCLIProxy())
	})
}

func TestAccount_GetBaseURL_CLIProxy(t *testing.T) {
	t.Run("配置了 base_url 直接返回", func(t *testing.T) {
		a := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeCLIProxy,
			Credentials: map[string]any{
				"base_url": "http://localhost:7861",
				"api_key":  "sk-cliproxy",
			},
		}
		require.Equal(t, "http://localhost:7861", a.GetBaseURL())
	})

	t.Run("未配置 base_url 返回空（不回退 anthropic.com）", func(t *testing.T) {
		a := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeCLIProxy,
			Credentials: map[string]any{
				"api_key": "sk-cliproxy",
			},
		}
		require.Equal(t, "", a.GetBaseURL(), "cliproxy 必须显式配置 base_url，无默认值")
	})
}

func TestGatewayService_GetAccessToken_CLIProxy(t *testing.T) {
	svc := &GatewayService{}

	t.Run("正常返回 api_key + apikey 类型", func(t *testing.T) {
		a := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeCLIProxy,
			Credentials: map[string]any{
				"api_key": "sk-cliproxy-token",
			},
		}
		token, tokenType, err := svc.GetAccessToken(context.Background(), a)
		require.NoError(t, err)
		require.Equal(t, "sk-cliproxy-token", token)
		require.Equal(t, "apikey", tokenType,
			"必须返回 apikey 才能命中 forwardAnthropicAPIKeyPassthroughWithInput 的 tokenType 校验")
	})

	t.Run("缺 api_key 报错", func(t *testing.T) {
		a := &Account{
			Platform:    PlatformAnthropic,
			Type:        AccountTypeCLIProxy,
			Credentials: map[string]any{},
		}
		_, _, err := svc.GetAccessToken(context.Background(), a)
		require.Error(t, err)
	})
}

func TestGatewayService_BuildUpstreamRequest_CLIProxyAPIPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{
					Enabled:           false,
					AllowInsecureHTTP: true,
				},
			},
		},
	}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeCLIProxy,
		Credentials: map[string]any{
			"base_url": "http://localhost:7861",
			"api_key":  "sk-cliproxy",
		},
	}

	req, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, []byte(`{"model":"claude-sonnet-4-5"}`), "sk-cliproxy",
	)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:7861/v1/messages?beta=true", req.URL.String(),
		"必须把上游 URL 重写到 cliproxy 端点")
	require.Equal(t, "sk-cliproxy", getHeaderRaw(req.Header, "x-api-key"),
		"必须用 cliproxy 配置的 api_key 而不是入站 key")
	require.Equal(t, "", getHeaderRaw(req.Header, "authorization"),
		"入站 Authorization 应被清掉，避免泄漏给 CLIProxyAPI")
	require.Equal(t, "2023-06-01", getHeaderRaw(req.Header, "anthropic-version"),
		"应补齐 anthropic-version")
}

// 兜底：cliproxy 账号未配置 base_url 时，必须直接报错，
// 不能像 apikey 那样回退到真实的 api.anthropic.com（否则会把 cliproxy 的内部 token
// 发到 Anthropic 官方端点，既不工作又可能泄漏凭据）。
func TestGatewayService_BuildUpstreamRequest_CLIProxyMissingBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{cfg: &config.Config{}}
	account := &Account{
		ID:       42,
		Name:     "cliproxy-without-base",
		Platform: PlatformAnthropic,
		Type:     AccountTypeCLIProxy,
		Credentials: map[string]any{
			"api_key": "sk-cliproxy",
		},
	}

	_, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, []byte(`{"model":"claude-sonnet-4-5"}`), "sk-cliproxy",
	)
	require.Error(t, err, "cliproxy 没配 base_url 必须报错，不能回退到 api.anthropic.com")
	require.Contains(t, err.Error(), "base_url")

	_, err = svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, []byte(`{"model":"claude-sonnet-4-5"}`), "sk-cliproxy",
	)
	require.Error(t, err, "count_tokens 同样必须拦截")
	require.Contains(t, err.Error(), "base_url")
}

// 回归：普通 apikey 账号空 base_url 仍然要回退到 claudeAPIURL，
// 不能被上面的 cliproxy 守卫连带误伤。
func TestGatewayService_BuildUpstreamRequest_APIKeyMissingBaseURLStillFallsBack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{cfg: &config.Config{}}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-anthropic",
		},
	}

	req, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, []byte(`{}`), "sk-anthropic",
	)
	require.NoError(t, err)
	require.Equal(t, claudeAPIURL, req.URL.String(),
		"apikey 类型未配 base_url 应继续回退到 anthropic 官方")
}

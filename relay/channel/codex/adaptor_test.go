package codex

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCodexOAuthKey = `{"access_token":"access-token","account_id":"account-id"}`

func TestSetupRequestHeaderDoesNotInventOriginatorForGenericClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("User-Agent", "curl/8.0")

	headers := http.Header{}
	err := (&Adaptor{}).SetupRequestHeader(ctx, &headers, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: testCodexOAuthKey},
	})

	require.NoError(t, err)
	assert.Empty(t, headers.Get("originator"))
	assert.Equal(t, "Bearer access-token", headers.Get("Authorization"))
	assert.Equal(t, "account-id", headers.Get("chatgpt-account-id"))
}

func TestSetupRequestHeaderPreservesClientSuppliedOfficialCodexOriginator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
	ctx.Request.Header.Set("originator", "codex_cli_rs")

	headers := http.Header{}
	err := (&Adaptor{}).SetupRequestHeader(ctx, &headers, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: testCodexOAuthKey},
	})

	require.NoError(t, err)
	assert.Equal(t, "codex_cli_rs", headers.Get("originator"))
}

func TestSetupRequestHeaderDoesNotInventOriginatorFromUserAgentOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")

	headers := http.Header{}
	err := (&Adaptor{}).SetupRequestHeader(ctx, &headers, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: testCodexOAuthKey},
	})

	require.NoError(t, err)
	assert.Empty(t, headers.Get("originator"))
}

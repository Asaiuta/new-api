package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clientRestrictionTestContext(method string, path string, body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx
}

func withClientRestrictionSetting(t *testing.T, mutate func(*operation_setting.ClientRestrictionSetting)) {
	t.Helper()
	cfg := operation_setting.GetClientRestrictionSetting()
	original := *cfg
	mutate(cfg)
	t.Cleanup(func() {
		*cfg = original
	})
}

func TestCheckClientRestrictionAllowsOfficialCodexClient(t *testing.T) {
	withClientRestrictionSetting(t, func(cfg *operation_setting.ClientRestrictionSetting) {
		cfg.Enabled = true
		cfg.Mode = operation_setting.ClientRestrictionModeCodex
		cfg.AutoPassThroughHeadersForMatchedClients = true
		cfg.AutoPassThroughBodyForMatchedClients = true
	})

	ctx := clientRestrictionTestContext(http.MethodPost, "/v1/responses", `{"model":"gpt-5"}`)
	ctx.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0 (Windows 10.0.19045; x86_64) terminal")

	result := CheckClientRestriction(ctx, "default")

	require.True(t, result.Enabled)
	require.True(t, result.Allowed)
	assert.Equal(t, ReasonCodexOfficialClientMatched, result.Reason)
	assert.True(t, result.PassThroughBody)
	assert.Contains(t, result.PassHeaders, "Originator")
	assert.Contains(t, result.PassHeaders, "User-Agent")
}

func TestCheckClientRestrictionRejectsGenericClientWhenEnabled(t *testing.T) {
	withClientRestrictionSetting(t, func(cfg *operation_setting.ClientRestrictionSetting) {
		cfg.Enabled = true
		cfg.Mode = operation_setting.ClientRestrictionModeCodexOrClaude
	})

	ctx := clientRestrictionTestContext(http.MethodPost, "/v1/responses", `{"model":"gpt-5"}`)
	ctx.Request.Header.Set("User-Agent", "curl/8.0")

	result := CheckClientRestriction(ctx, "default")

	require.True(t, result.Enabled)
	require.False(t, result.Allowed)
	assert.Equal(t, ReasonClientRestrictionNotMatched, result.Reason)
}

func TestCheckClientRestrictionSkipsUnconfiguredGroup(t *testing.T) {
	withClientRestrictionSetting(t, func(cfg *operation_setting.ClientRestrictionSetting) {
		cfg.Enabled = true
		cfg.ApplyOnlyToGroups = []string{"codex-only"}
	})

	ctx := clientRestrictionTestContext(http.MethodPost, "/v1/responses", `{"model":"gpt-5"}`)

	result := CheckClientRestriction(ctx, "default")

	require.False(t, result.Enabled)
	require.True(t, result.Allowed)
	assert.Equal(t, ReasonClientRestrictionGroupSkipped, result.Reason)
}

func TestCheckClientRestrictionAllowsClaudeCodeMessagesRequest(t *testing.T) {
	withClientRestrictionSetting(t, func(cfg *operation_setting.ClientRestrictionSetting) {
		cfg.Enabled = true
		cfg.Mode = operation_setting.ClientRestrictionModeClaudeCode
		cfg.AutoPassThroughHeadersForMatchedClients = true
	})

	body := `{
		"model": "claude-sonnet-4-5",
		"max_tokens": 1024,
		"system": [{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."}],
		"messages": [{"role":"user","content":"hi"}],
		"metadata": {"user_id": "user_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_account__session_123e4567-e89b-12d3-a456-426614174000"}
	}`
	ctx := clientRestrictionTestContext(http.MethodPost, "/v1/messages", body)
	ctx.Request.Header.Set("User-Agent", "claude-cli/2.1.80")
	ctx.Request.Header.Set("X-App", "claude-code")
	ctx.Request.Header.Set("anthropic-beta", "token-efficient-tools-2025-02-19")
	ctx.Request.Header.Set("anthropic-version", "2023-06-01")

	result := CheckClientRestriction(ctx, "default")

	require.True(t, result.Enabled)
	require.True(t, result.Allowed)
	assert.Equal(t, ReasonClaudeCodeClientMatched, result.Reason)
	assert.Contains(t, result.PassHeaders, "Anthropic-Beta")
	assert.Contains(t, result.PassHeaders, "X-App")
}

func TestCheckClientRestrictionDoesNotTreatClaudeUAAsResponsesClient(t *testing.T) {
	withClientRestrictionSetting(t, func(cfg *operation_setting.ClientRestrictionSetting) {
		cfg.Enabled = true
		cfg.Mode = operation_setting.ClientRestrictionModeClaudeCode
	})

	ctx := clientRestrictionTestContext(http.MethodPost, "/v1/responses", `{"model":"gpt-5"}`)
	ctx.Request.Header.Set("User-Agent", "claude-cli/2.1.80")

	result := CheckClientRestriction(ctx, "default")

	require.True(t, result.Enabled)
	require.False(t, result.Allowed)
}

func TestCheckClientRestrictionRejectsClaudeCodeWithInvalidMetadataUserID(t *testing.T) {
	withClientRestrictionSetting(t, func(cfg *operation_setting.ClientRestrictionSetting) {
		cfg.Enabled = true
		cfg.Mode = operation_setting.ClientRestrictionModeClaudeCode
	})

	body := `{
		"model": "claude-sonnet-4-5",
		"max_tokens": 1024,
		"system": [{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."}],
		"messages": [{"role":"user","content":"hi"}],
		"metadata": {"user_id": "not-a-claude-code-metadata-id"}
	}`
	ctx := clientRestrictionTestContext(http.MethodPost, "/v1/messages", body)
	ctx.Request.Header.Set("User-Agent", "claude-cli/2.1.80")
	ctx.Request.Header.Set("X-App", "claude-code")
	ctx.Request.Header.Set("anthropic-beta", "token-efficient-tools-2025-02-19")
	ctx.Request.Header.Set("anthropic-version", "2023-06-01")

	result := CheckClientRestriction(ctx, "default")

	require.True(t, result.Enabled)
	require.False(t, result.Allowed)
}

func TestApplyMatchedClientPassThroughHeaderOverrideKeepsChannelOverride(t *testing.T) {
	ctx := clientRestrictionTestContext(http.MethodPost, "/v1/responses", `{"model":"gpt-5"}`)
	ctx.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
	ctx.Request.Header.Set("Originator", "codex_cli_rs")
	ctx.Request.Header.Set("Authorization", "Bearer client-secret")
	SetClientRestrictionResult(ctx, ClientRestrictionResult{
		Allowed:     true,
		PassHeaders: []string{"User-Agent", "Originator", "Authorization"},
	})

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]interface{}{
				"X-Static": "channel-value",
			},
		},
	}

	applied := ApplyMatchedClientPassThroughHeaderOverride(ctx, info)

	require.True(t, applied)
	require.True(t, info.UseRuntimeHeadersOverride)
	assert.Equal(t, "channel-value", info.RuntimeHeadersOverride["x-static"])
	assert.Equal(t, "codex_cli_rs/0.98.0", info.RuntimeHeadersOverride["user-agent"])
	assert.Equal(t, "codex_cli_rs", info.RuntimeHeadersOverride["originator"])
	_, hasAuthorization := info.RuntimeHeadersOverride["authorization"]
	assert.False(t, hasAuthorization)
}

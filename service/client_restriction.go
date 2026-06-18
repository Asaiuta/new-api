package service

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	ClientKindCodex      = "codex"
	ClientKindClaudeCode = "claude_code"

	ReasonClientRestrictionDisabled     = "client_restriction_disabled"
	ReasonClientRestrictionGroupSkipped = "client_restriction_group_skipped"
	ReasonCodexOfficialClientMatched    = "codex_official_client_matched"
	ReasonClaudeCodeClientMatched       = "claude_code_client_matched"
	ReasonClaudeCodeCodexPluginMatched  = "claude_code_codex_plugin_matched"
	ReasonClientRestrictionNotMatched   = "client_restriction_not_matched"
)

const (
	claudeCodeBillingHeaderPrefix = "x-anthropic-billing-header"
	claudeCodeCLIEntrypointMarker = "cc_entrypoint=cli"
)

const ginKeyClientRestrictionResult = "client_restriction_result"

var (
	codexOfficialUserAgentPrefixes = []string{
		"codex_cli_rs/",
		"codex_vscode/",
		"codex_app/",
		"codex_chatgpt_desktop/",
		"codex_atlas/",
		"codex_exec/",
		"codex_sdk_ts/",
		"codex ",
	}
	codexOfficialOriginatorPrefixes = []string{
		"codex_",
		"codex ",
	}
	claudeCodeUAPattern                = regexp.MustCompile(`(?i)^claude-cli/\d+\.\d+\.\d+`)
	legacyClaudeCodeMetadataUserIDExpr = regexp.MustCompile(`^user_([a-fA-F0-9]{64})_account_([a-fA-F0-9-]*)_session_([a-fA-F0-9-]{36})$`)
)

var clientPassThroughSkipHeaders = map[string]struct{}{
	"authorization":         {},
	"cookie":                {},
	"host":                  {},
	"content-length":        {},
	"accept-encoding":       {},
	"connection":            {},
	"keep-alive":            {},
	"proxy-authenticate":    {},
	"proxy-authorization":   {},
	"te":                    {},
	"trailer":               {},
	"transfer-encoding":     {},
	"upgrade":               {},
	"x-api-key":             {},
	"x-goog-api-key":        {},
	"sec-websocket-key":     {},
	"sec-websocket-version": {},
}

var codexMatchedClientPassThroughHeaders = []string{
	"Accept-Language",
	"Conversation_ID",
	"OpenAI-Beta",
	"Originator",
	"Session_id",
	"User-Agent",
	"X-Codex-Beta-Features",
	"X-Codex-Turn-Metadata",
	"X-Codex-Turn-State",
}

var claudeCodeMatchedClientPassThroughHeaders = []string{
	"Anthropic-Beta",
	"Anthropic-Dangerous-Direct-Browser-Access",
	"Anthropic-Version",
	"User-Agent",
	"X-App",
	"X-Stainless-Arch",
	"X-Stainless-Lang",
	"X-Stainless-Os",
	"X-Stainless-Package-Version",
	"X-Stainless-Retry-Count",
	"X-Stainless-Runtime",
	"X-Stainless-Runtime-Version",
	"X-Stainless-Timeout",
}

type ClientRestrictionResult struct {
	Enabled         bool
	Allowed         bool
	Reason          string
	MatchedKinds    []string
	PassThroughBody bool
	PassHeaders     []string
}

func CheckClientRestriction(c *gin.Context, usingGroup string) ClientRestrictionResult {
	cfg := operation_setting.GetClientRestrictionSetting()
	if cfg == nil || !cfg.Enabled {
		return ClientRestrictionResult{Enabled: false, Allowed: true, Reason: ReasonClientRestrictionDisabled}
	}
	if !clientRestrictionAppliesToGroup(cfg, usingGroup) {
		return ClientRestrictionResult{Enabled: false, Allowed: true, Reason: ReasonClientRestrictionGroupSkipped}
	}

	detection := DetectRequestClient(c)
	mode := strings.TrimSpace(cfg.Mode)
	if mode == "" {
		mode = operation_setting.ClientRestrictionModeCodexOrClaude
	}

	codexMatched := detection.CodexOfficial
	claudeCodeMatched := detection.ClaudeCode
	claudeCodeCodexPluginMatched := false
	if cfg.AllowClaudeCodeCodexPlugin && isClaudeCodeCodexPluginRequest(c) {
		codexMatched = true
		claudeCodeCodexPluginMatched = true
	}

	allowed := false
	switch mode {
	case operation_setting.ClientRestrictionModeCodex:
		allowed = codexMatched
	case operation_setting.ClientRestrictionModeClaudeCode:
		allowed = claudeCodeMatched
	case operation_setting.ClientRestrictionModeCodexAndClaude:
		allowed = codexMatched && claudeCodeMatched
	case operation_setting.ClientRestrictionModeCodexOrClaude:
		allowed = codexMatched || claudeCodeMatched
	default:
		allowed = codexMatched || claudeCodeMatched
	}

	result := ClientRestrictionResult{
		Enabled: true,
		Allowed: allowed,
		Reason:  ReasonClientRestrictionNotMatched,
	}
	if detection.CodexOfficial {
		result.MatchedKinds = append(result.MatchedKinds, ClientKindCodex)
		result.Reason = ReasonCodexOfficialClientMatched
	}
	if detection.ClaudeCode {
		result.MatchedKinds = append(result.MatchedKinds, ClientKindClaudeCode)
		result.Reason = ReasonClaudeCodeClientMatched
	}
	if claudeCodeCodexPluginMatched {
		result.MatchedKinds = append(result.MatchedKinds, ClientKindCodex)
		result.Reason = ReasonClaudeCodeCodexPluginMatched
	}

	if allowed {
		result.PassThroughBody = cfg.AutoPassThroughBodyForMatchedClients
		if cfg.AutoPassThroughHeadersForMatchedClients {
			result.PassHeaders = buildClientPassThroughHeaders(codexMatched, claudeCodeMatched)
		}
	}

	return result
}

func SetClientRestrictionResult(c *gin.Context, result ClientRestrictionResult) {
	if c == nil {
		return
	}
	c.Set(ginKeyClientRestrictionResult, result)
}

func GetClientRestrictionResult(c *gin.Context) (ClientRestrictionResult, bool) {
	if c == nil {
		return ClientRestrictionResult{}, false
	}
	raw, ok := c.Get(ginKeyClientRestrictionResult)
	if !ok {
		return ClientRestrictionResult{}, false
	}
	result, ok := raw.(ClientRestrictionResult)
	return result, ok
}

func ShouldPassThroughBodyForMatchedClient(c *gin.Context) bool {
	result, ok := GetClientRestrictionResult(c)
	return ok && result.Allowed && result.PassThroughBody
}

func ApplyMatchedClientPassThroughHeaderOverride(c *gin.Context, info *relaycommon.RelayInfo) bool {
	result, ok := GetClientRestrictionResult(c)
	if !ok || !result.Allowed || len(result.PassHeaders) == 0 || info == nil {
		return false
	}

	runtimeHeaders := make(map[string]interface{})
	for key, value := range relaycommon.GetEffectiveHeaderOverride(info) {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" {
			continue
		}
		runtimeHeaders[normalized] = value
	}

	applied := false
	for _, headerName := range result.PassHeaders {
		normalized := strings.ToLower(strings.TrimSpace(headerName))
		if normalized == "" || shouldSkipClientPassThroughHeader(normalized) {
			continue
		}
		value := ""
		if c != nil && c.Request != nil {
			value = strings.TrimSpace(c.Request.Header.Get(headerName))
		}
		if value == "" {
			continue
		}
		if existing, ok := runtimeHeaders[normalized]; ok && strings.TrimSpace(fmt.Sprintf("%v", existing)) != "" {
			continue
		}
		runtimeHeaders[normalized] = value
		applied = true
	}

	if !applied {
		return false
	}
	info.RuntimeHeadersOverride = runtimeHeaders
	info.UseRuntimeHeadersOverride = true
	return true
}

func shouldSkipClientPassThroughHeader(headerName string) bool {
	headerName = strings.ToLower(strings.TrimSpace(headerName))
	if headerName == "" {
		return true
	}
	_, ok := clientPassThroughSkipHeaders[headerName]
	return ok
}

func AbortForClientRestriction(c *gin.Context, result ClientRestrictionResult) {
	message := "This endpoint is restricted to real Codex or Claude Code clients"
	code := types.ErrorCodeAccessDenied
	if result.Reason != "" {
		message = fmt.Sprintf("%s: %s", message, result.Reason)
	}
	c.JSON(http.StatusForbidden, gin.H{
		"error": gin.H{
			"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
			"type":    "new_api_error",
			"code":    string(code),
		},
	})
	c.Abort()
}

func buildClientPassThroughHeaders(codexMatched bool, claudeCodeMatched bool) []string {
	headers := make([]string, 0, len(codexMatchedClientPassThroughHeaders)+len(claudeCodeMatchedClientPassThroughHeaders))
	if codexMatched {
		headers = append(headers, codexMatchedClientPassThroughHeaders...)
	}
	if claudeCodeMatched {
		headers = append(headers, claudeCodeMatchedClientPassThroughHeaders...)
	}
	return uniqueNonEmptyStrings(headers)
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func clientRestrictionAppliesToGroup(cfg *operation_setting.ClientRestrictionSetting, usingGroup string) bool {
	if cfg == nil || len(cfg.ApplyOnlyToGroups) == 0 {
		return true
	}
	usingGroup = strings.TrimSpace(usingGroup)
	for _, group := range cfg.ApplyOnlyToGroups {
		if strings.EqualFold(strings.TrimSpace(group), usingGroup) {
			return true
		}
	}
	return false
}

type RequestClientDetection struct {
	CodexOfficial bool
	ClaudeCode    bool
}

func DetectRequestClient(c *gin.Context) RequestClientDetection {
	ua := ""
	originator := ""
	if c != nil && c.Request != nil {
		ua = c.Request.Header.Get("User-Agent")
		originator = c.Request.Header.Get("originator")
	}
	return RequestClientDetection{
		CodexOfficial: isCodexOfficialClientByHeaders(ua, originator),
		ClaudeCode:    isClaudeCodeRequest(c),
	}
}

func IsCodexOfficialClientByHeaders(userAgent string, originator string) bool {
	return isCodexOfficialClientByHeaders(userAgent, originator)
}

func isCodexOfficialClientByHeaders(userAgent string, originator string) bool {
	return matchNormalizedPrefix(userAgent, codexOfficialUserAgentPrefixes) ||
		matchNormalizedPrefix(originator, codexOfficialOriginatorPrefixes)
}

func matchNormalizedPrefix(value string, prefixes []string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, prefix := range prefixes {
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(value, prefix) || strings.Contains(value, prefix) {
			return true
		}
	}
	return false
}

func isClaudeCodeCodexPluginRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	originator := strings.ToLower(strings.TrimSpace(c.Request.Header.Get("originator")))
	ua := strings.ToLower(strings.TrimSpace(c.Request.Header.Get("User-Agent")))
	return originator == "claude code" && strings.Contains(ua, "claude code/")
}

func isClaudeCodeRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	ua := c.Request.Header.Get("User-Agent")
	if !claudeCodeUAPattern.MatchString(ua) {
		return false
	}
	path := ""
	if c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	if !strings.Contains(path, "messages") {
		return false
	}
	if strings.HasSuffix(path, "/messages/count_tokens") {
		return true
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return false
	}
	body, err := storage.Bytes()
	if err != nil || len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	return hasClaudeCodeBodySignals(body, c.Request.Header)
}

func hasClaudeCodeBodySignals(body []byte, headers http.Header) bool {
	if strings.TrimSpace(headers.Get("X-App")) == "" ||
		strings.TrimSpace(headers.Get("anthropic-beta")) == "" ||
		strings.TrimSpace(headers.Get("anthropic-version")) == "" {
		return false
	}
	if !gjson.GetBytes(body, "model").Exists() {
		return false
	}
	metadataUserID := gjson.GetBytes(body, "metadata.user_id")
	if !metadataUserID.Exists() || metadataUserID.Type != gjson.String || !isClaudeCodeMetadataUserID(metadataUserID.String()) {
		return false
	}
	return hasClaudeCodeSystemPromptSignal(body)
}

func isClaudeCodeMetadataUserID(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "{") {
		if !gjson.Valid(raw) {
			return false
		}
		return strings.TrimSpace(gjson.Get(raw, "device_id").String()) != "" &&
			strings.TrimSpace(gjson.Get(raw, "session_id").String()) != ""
	}
	return legacyClaudeCodeMetadataUserIDExpr.MatchString(raw)
}

func hasClaudeCodeSystemPromptSignal(body []byte) bool {
	system := gjson.GetBytes(body, "system")
	if !system.Exists() {
		return false
	}
	if system.Type == gjson.String {
		return isClaudeCodeSystemText(system.String())
	}
	if !system.IsArray() {
		return false
	}
	found := false
	system.ForEach(func(_, item gjson.Result) bool {
		text := gjson.Get(item.Raw, "text")
		if text.Type == gjson.String && isClaudeCodeSystemText(text.String()) {
			found = true
			return false
		}
		return true
	})
	return found
}

func isClaudeCodeSystemText(text string) bool {
	if strings.HasPrefix(text, claudeCodeBillingHeaderPrefix) &&
		strings.Contains(text, claudeCodeCLIEntrypointMarker) {
		return true
	}
	normalized := strings.Join(strings.Fields(text), " ")
	return strings.Contains(normalized, "You are Claude Code, Anthropic's official CLI for Claude.") ||
		strings.Contains(normalized, "You are a Claude agent, built on Anthropic's Claude Agent SDK.") ||
		strings.Contains(normalized, "You are Claude Code, Anthropic's official CLI for Claude, running within the Claude Agent SDK.") ||
		strings.Contains(normalized, "You are an interactive CLI tool that helps users")
}

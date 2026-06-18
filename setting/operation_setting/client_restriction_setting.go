package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

const (
	ClientRestrictionModeOff            = ""
	ClientRestrictionModeCodex          = "codex"
	ClientRestrictionModeClaudeCode     = "claude_code"
	ClientRestrictionModeCodexOrClaude  = "codex_or_claude_code"
	ClientRestrictionModeCodexAndClaude = "codex_and_claude_code"
)

type ClientRestrictionSetting struct {
	// Enabled turns on NewAPI ingress-side client restriction for relay requests.
	Enabled bool `json:"enabled"`
	// Mode controls which real client signatures are allowed.
	// Supported values: codex, claude_code, codex_or_claude_code, codex_and_claude_code.
	Mode string `json:"mode"`
	// ApplyOnlyToGroups limits enforcement to selected using groups. Empty means all groups.
	ApplyOnlyToGroups []string `json:"apply_only_to_groups,omitempty"`
	// AllowClaudeCodeCodexPlugin allows Claude Code's Codex plugin signature for Codex-mode checks.
	AllowClaudeCodeCodexPlugin bool `json:"allow_claude_code_codex_plugin"`
	// AutoPassThroughBodyForMatchedClients reuses the original client JSON body for matched clients.
	// Keep this disabled unless upstream compatibility requires exact request bodies.
	AutoPassThroughBodyForMatchedClients bool `json:"auto_pass_through_body_for_matched_clients"`
	// AutoPassThroughHeadersForMatchedClients forwards the matched real client's own low-risk headers.
	AutoPassThroughHeadersForMatchedClients bool `json:"auto_pass_through_headers_for_matched_clients"`
}

var clientRestrictionSetting = ClientRestrictionSetting{
	Enabled:                                 false,
	Mode:                                    ClientRestrictionModeCodexOrClaude,
	ApplyOnlyToGroups:                       nil,
	AllowClaudeCodeCodexPlugin:              true,
	AutoPassThroughBodyForMatchedClients:    false,
	AutoPassThroughHeadersForMatchedClients: true,
}

func init() {
	config.GlobalConfig.Register("client_restriction_setting", &clientRestrictionSetting)
}

func GetClientRestrictionSetting() *ClientRestrictionSetting {
	return &clientRestrictionSetting
}

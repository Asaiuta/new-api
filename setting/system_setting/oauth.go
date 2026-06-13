package system_setting

import "github.com/QuantumNous/new-api/setting/config"

type OAuthSettings struct {
	DisableUserUnbind bool `json:"disable_user_unbind"`
}

var defaultOAuthSettings = OAuthSettings{}

func init() {
	config.GlobalConfig.Register("oauth", &defaultOAuthSettings)
}

func GetOAuthSettings() *OAuthSettings {
	return &defaultOAuthSettings
}

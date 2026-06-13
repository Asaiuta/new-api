package system_setting

import "github.com/QuantumNous/new-api/setting/config"

type LogSettings struct {
	ForceRecordIp bool `json:"force_record_ip"`
}

var defaultLogSettings = LogSettings{}

func init() {
	config.GlobalConfig.Register("log", &defaultLogSettings)
}

func GetLogSettings() *LogSettings {
	return &defaultLogSettings
}

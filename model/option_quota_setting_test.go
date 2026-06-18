package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestQuotaSettingFastPreConsumeEstimateSyncsRuntimeFlag(t *testing.T) {
	oldOptionMap := common.OptionMap
	oldFast := constant.FastPreConsumeEstimate
	oldSetting := operation_setting.GetQuotaSetting().FastPreConsumeEstimate

	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		common.OptionMap = oldOptionMap
		constant.FastPreConsumeEstimate = oldFast
		operation_setting.GetQuotaSetting().FastPreConsumeEstimate = oldSetting
	})

	err := updateOptionMap("quota_setting.fast_pre_consume_estimate", "true")
	require.NoError(t, err)
	require.True(t, constant.FastPreConsumeEstimate)
	require.True(t, operation_setting.GetQuotaSetting().FastPreConsumeEstimate)
	require.Equal(t, "true", common.OptionMap["quota_setting.fast_pre_consume_estimate"])

	err = updateOptionMap("quota_setting.fast_pre_consume_estimate", "false")
	require.NoError(t, err)
	require.False(t, constant.FastPreConsumeEstimate)
	require.False(t, operation_setting.GetQuotaSetting().FastPreConsumeEstimate)
	require.Equal(t, "false", common.OptionMap["quota_setting.fast_pre_consume_estimate"])
}

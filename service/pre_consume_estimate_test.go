package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newEstimateTestContext(model string) (*gin.Context, *relaycommon.RelayInfo) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, model)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: model,
		},
		RelayFormat: types.RelayFormatOpenAI,
	}
	return c, info
}

// TestFastPreConsumeEstimateUsesEstimationForOpenAIModel verifies that when the
// FastPreConsumeEstimate flag is enabled, the pre-consume token estimation for an
// OpenAI model uses the fast estimator instead of the tiktoken encoder.
func TestFastPreConsumeEstimateUsesEstimationForOpenAIModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitTokenEncoders()

	oldCount := constant.CountToken
	oldFast := constant.FastPreConsumeEstimate
	constant.CountToken = true
	t.Cleanup(func() {
		constant.CountToken = oldCount
		constant.FastPreConsumeEstimate = oldFast
	})

	const model = "gpt-4o"
	text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 200)
	meta := &types.TokenCountMeta{
		TokenType:   types.TokenTypeTokenizer,
		CombineText: text,
	}

	// EstimateRequestToken adds a fixed OpenAI format overhead of +3 here
	// (no tools/messages/names in meta), so the difference between the two paths
	// is entirely in the text-token component.
	const openAIFormatOverhead = 3

	// Flag off -> tiktoken exact count path.
	constant.FastPreConsumeEstimate = false
	c1, info1 := newEstimateTestContext(model)
	exactTokens, err := EstimateRequestToken(c1, meta, info1)
	require.NoError(t, err)

	// Flag on -> fast estimate path.
	constant.FastPreConsumeEstimate = true
	c2, info2 := newEstimateTestContext(model)
	estimateTokens, err := EstimateRequestToken(c2, meta, info2)
	require.NoError(t, err)

	exactText := CountTextToken(text, model)
	estimateText := EstimateTokenByModel(model, text)

	require.Equal(t, exactText+openAIFormatOverhead, exactTokens,
		"flag-off path must use tiktoken exact count for the text component")
	require.Equal(t, estimateText+openAIFormatOverhead, estimateTokens,
		"flag-on path must use the fast estimator for the text component")

	// The two paths must actually differ for a non-trivial OpenAI prompt,
	// otherwise the flag would be a no-op.
	require.NotEqual(t, exactTokens, estimateTokens)
}

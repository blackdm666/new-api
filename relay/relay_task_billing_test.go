package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTaskBillingRatiosRequiresSecondsForPerSecondFixedPrice(t *testing.T) {
	savedModes := billing_setting.GetBillingModeCopy()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"billing_setting.billing_mode": mustMarshalTaskBillingTest(t, savedModes),
		}))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"video-model":"per_second","request-model":"per_request"}`,
	}))

	priceData := hosttypes.PriceData{UsePrice: true}
	assert.Error(t, validateTaskBillingRatios("video-model", priceData))
	assert.NoError(t, validateTaskBillingRatios("request-model", priceData))
	assert.Error(t, validateTaskBillingRatios("video-model", hosttypes.PriceData{}))
	assert.Error(t, validateTaskBillingRatios("request-model", hosttypes.PriceData{}))

	priceData.AddOtherRatio("seconds", 10)
	assert.NoError(t, validateTaskBillingRatios("video-model", priceData))
}

func TestEnsureTaskBillingRatiosUsesStandardDurationFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{Duration: 7})

	savedModes := billing_setting.GetBillingModeCopy()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"billing_setting.billing_mode": mustMarshalTaskBillingTest(t, savedModes),
		}))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"video-model":"per_second"}`,
	}))

	priceData := hosttypes.PriceData{UsePrice: true}
	require.NoError(t, ensureTaskBillingRatios(c, "video-model", &priceData))
	assert.Equal(t, 7.0, priceData.OtherRatios()["seconds"])
}

func TestMergeTaskBillingRatiosPreservesRemixAndEstimatedValues(t *testing.T) {
	priceData := hosttypes.PriceData{UsePrice: true}
	mergeTaskBillingRatios(&priceData, map[string]float64{"seconds": 4})
	mergeTaskBillingRatios(&priceData, map[string]float64{"resolution": 1.5})

	assert.Equal(t, map[string]float64{
		"seconds":    4,
		"resolution": 1.5,
	}, priceData.OtherRatios())
}

func mustMarshalTaskBillingTest(t *testing.T, value any) string {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

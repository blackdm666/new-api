package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestBuildAudioQuotaInfoPreservesFixedPrice(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	priceData := types.PriceData{
		UsePrice:   true,
		ModelPrice: 0.4,
		GroupRatioInfo: types.GroupRatioInfo{
			GroupRatio: 0.3,
		},
	}
	info := buildAudioQuotaInfo(
		TokenDetails{TextTokens: 8},
		TokenDetails{AudioTokens: 27},
		"gemini-3.1-tts",
		priceData,
	)

	require.Equal(t, 0.4, info.ModelPrice)
	require.True(t, info.UsePrice)
	quota, clamp := calculateAudioQuota(info)
	require.Nil(t, clamp)
	require.Equal(t, 60_000, quota)
}

func TestBuildAudioQuotaInfoPreservesTokenRatios(t *testing.T) {
	priceData := types.PriceData{
		ModelRatio: 2,
		GroupRatioInfo: types.GroupRatioInfo{
			GroupRatio: 0.5,
		},
	}
	info := buildAudioQuotaInfo(
		TokenDetails{TextTokens: 8},
		TokenDetails{AudioTokens: 27},
		"token-priced-audio",
		priceData,
	)

	require.False(t, info.UsePrice)
	require.Equal(t, 2.0, info.ModelRatio)
	require.Equal(t, 0.5, info.GroupRatio)
}

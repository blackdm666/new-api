package middleware

import "testing"

func TestNormalizeResponsesCompactModel(t *testing.T) {
	tests := []struct {
		name        string
		requestPath string
		modelName   string
		want        string
	}{
		{
			name:        "legacy compact alias is normalized",
			requestPath: "/v1/responses/compact",
			modelName:   "gpt-5.4-openai-compact",
			want:        "gpt-5.4",
		},
		{
			name:        "base model stays unchanged",
			requestPath: "/v1/responses/compact",
			modelName:   "gpt-5.4",
			want:        "gpt-5.4",
		},
		{
			name:        "legacy alias remains invalid outside compact endpoint",
			requestPath: "/v1/responses",
			modelName:   "gpt-5.4-openai-compact",
			want:        "gpt-5.4-openai-compact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeResponsesCompactModel(tt.requestPath, tt.modelName); got != tt.want {
				t.Fatalf("normalizeResponsesCompactModel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTokenModelLimitAllowsResponsesCompactMigration(t *testing.T) {
	legacyOnly := map[string]bool{"gpt-5.4-openai-compact": true}
	baseOnly := map[string]bool{"gpt-5.4": true}

	if !tokenModelLimitAllowsRequest(legacyOnly, "gpt-5.4", "/v1/responses/compact") {
		t.Fatal("legacy compact permission should allow the base model on the compact endpoint")
	}
	if tokenModelLimitAllowsRequest(legacyOnly, "gpt-5.4", "/v1/responses") {
		t.Fatal("legacy compact permission must not allow the base model outside the compact endpoint")
	}
	if !tokenModelLimitAllowsRequest(baseOnly, "gpt-5.4", "/v1/responses/compact") {
		t.Fatal("base model permission should allow the compact endpoint")
	}
}

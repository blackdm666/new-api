package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveMappedModelName(t *testing.T) {
	tests := []struct {
		name       string
		origin     string
		mapping    string
		wantModel  string
		wantMapped bool
		wantErr    string
	}{
		{name: "no mapping", origin: "model-a", mapping: "{}", wantModel: "model-a"},
		{name: "direct", origin: "sales", mapping: `{"sales":"upstream"}`, wantModel: "upstream", wantMapped: true},
		{name: "chain", origin: "sales", mapping: `{"sales":"internal","internal":"upstream"}`, wantModel: "upstream", wantMapped: true},
		{name: "origin self mapping", origin: "sales", mapping: `{"sales":"sales"}`, wantModel: "sales"},
		{name: "chain tail self mapping", origin: "sales", mapping: `{"sales":"upstream","upstream":"upstream"}`, wantModel: "upstream", wantMapped: true},
		{name: "cycle", origin: "sales", mapping: `{"sales":"internal","internal":"sales"}`, wantModel: "sales", wantErr: "model_mapping_contains_cycle"},
		{name: "invalid json", origin: "sales", mapping: `{`, wantModel: "sales", wantErr: "unmarshal_model_mapping_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotModel, gotMapped, err := ResolveMappedModelName(tt.origin, tt.mapping)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantModel, gotModel)
			assert.Equal(t, tt.wantMapped, gotMapped)
		})
	}
}

package operation_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
)

func TestCyberPolicyCodesAreNeverRetried(t *testing.T) {
	for _, code := range []types.ErrorCode{
		"session_blocked_by_cyber_policy",
		"cyber_policy",
		"cyber_session_identity_required",
		"cyber_session_identity_conflict",
		"cyber_session_identity_invalid",
		"conversation_cyber_locked",
		"request_policy_violation",
	} {
		assert.True(t, IsAlwaysSkipRetryCode(code), string(code))
	}
	assert.True(t, IsAlwaysSkipRetryCode(types.ErrorCodeBadResponseBody), "upstream entry must survive")
	assert.False(t, IsAlwaysSkipRetryCode("unknown_error"))
}

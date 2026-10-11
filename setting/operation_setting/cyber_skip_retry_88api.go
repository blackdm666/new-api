package operation_setting

import "github.com/QuantumNous/new-api/relaykit/types"

// Upstream content-policy refusals are never retried. A retry can carry the
// same flagged conversation to another channel (the other sub2api key, or a
// cross-group fallback for auto tokens) where the session block does not
// apply, and send the flagged content to another upstream account.
var cyberSkipRetryCodes = []types.ErrorCode{
	"session_blocked_by_cyber_policy", // sub2api local session block (403)
	"cyber_policy",                    // OpenAI cyber_policy refusal
	"cyber_session_identity_required", // sub2api strict session identity gate
	"cyber_session_identity_conflict",
	"cyber_session_identity_invalid",
	"conversation_cyber_locked", // codex2api conversation lock
	"request_policy_violation",  // codex2api prompt filter
}

func init() {
	for _, code := range cyberSkipRetryCodes {
		alwaysSkipRetryCodes[code] = struct{}{}
	}
}

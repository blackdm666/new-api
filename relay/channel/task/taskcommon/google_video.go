package taskcommon

import (
	"fmt"
	"strings"
)

// GoogleVideoFailureReason preserves Google's safety-filter explanation when
// a completed Veo operation contains no playable video. A terminal response
// without media must be a failure; otherwise finalization repeatedly retries
// video preparation while leaving the task at its previous 50% state.
func GoogleVideoFailureReason(filteredCount int, reasons []string) string {
	cleanReasons := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		if reason = strings.TrimSpace(reason); reason != "" {
			cleanReasons = append(cleanReasons, reason)
		}
	}
	if len(cleanReasons) > 0 {
		return strings.Join(cleanReasons, "; ")
	}
	if filteredCount > 0 {
		return fmt.Sprintf("Google filtered %d generated video(s) under its usage guidelines", filteredCount)
	}
	return "Google video generation completed without video output"
}

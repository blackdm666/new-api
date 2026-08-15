package service

import "strings"

func parseNotificationEmails(raw string) []string {
	fields := strings.FieldsFunc(strings.TrimSpace(raw), func(r rune) bool {
		return r == ';' || r == ',' || r == ' ' || r == '\n' || r == '\r' || r == '\t'
	})
	result := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		email := strings.TrimSpace(field)
		if email == "" {
			continue
		}
		if _, exists := seen[email]; exists {
			continue
		}
		seen[email] = struct{}{}
		result = append(result, email)
	}
	return result
}

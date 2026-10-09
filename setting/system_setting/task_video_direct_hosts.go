package system_setting

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	TaskVideoDirectHostsOptionKey = "TaskVideoDirectHosts"
	TaskVideoDirectHostsEnv       = "TASK_VIDEO_DIRECT_HOSTS"

	maxTaskVideoDirectHostsLength  = 8192
	maxTaskVideoDirectHostsEntries = 256
)

// TaskVideoDirectHosts returns the operator-maintained list of official media
// hosts whose anonymous video URLs may be delivered without archiving. A
// persisted option, including an intentionally empty one, wins over the
// TASK_VIDEO_DIRECT_HOSTS environment variable, which only seeds instances
// that have never saved the option.
func TaskVideoDirectHosts() string {
	common.OptionMapRWMutex.RLock()
	value, configured := common.OptionMap[TaskVideoDirectHostsOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if configured {
		return value
	}
	return common.GetEnvOrDefaultString(TaskVideoDirectHostsEnv, "")
}

func splitTaskVideoDirectHosts(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}

func normalizeTaskVideoDirectHost(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}

// TaskVideoDirectHostAllowed is the single allow-list decision for every
// direct video delivery path. Matching is case-insensitive and ignores a
// trailing dot. "*.example.com" matches subdomains at any depth but not
// example.com itself; other entries match exactly. A bare "*" never matches.
func TaskVideoDirectHostAllowed(host string) bool {
	host = normalizeTaskVideoDirectHost(host)
	if host == "" {
		return false
	}
	for _, pattern := range splitTaskVideoDirectHosts(TaskVideoDirectHosts()) {
		pattern = normalizeTaskVideoDirectHost(pattern)
		if pattern == "" || pattern == "*" {
			continue
		}
		if suffix, wildcard := strings.CutPrefix(pattern, "*."); wildcard {
			if suffix != "" && host != suffix && strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if host == pattern {
			return true
		}
	}
	return false
}

// ValidateTaskVideoDirectHosts rejects administrator input the matcher would
// silently ignore or that would trust far more than one official host.
func ValidateTaskVideoDirectHosts(value string) error {
	if len(value) > maxTaskVideoDirectHostsLength {
		return fmt.Errorf("%s must not exceed %d characters", TaskVideoDirectHostsOptionKey, maxTaskVideoDirectHostsLength)
	}
	entries := splitTaskVideoDirectHosts(value)
	if len(entries) > maxTaskVideoDirectHostsEntries {
		return fmt.Errorf("%s must not contain more than %d hosts", TaskVideoDirectHostsOptionKey, maxTaskVideoDirectHostsEntries)
	}
	for _, entry := range entries {
		host := normalizeTaskVideoDirectHost(entry)
		if suffix, wildcard := strings.CutPrefix(host, "*."); wildcard {
			host = suffix
		}
		if err := validateTaskVideoDirectHostName(host); err != nil {
			return fmt.Errorf("invalid video direct host %q: %w", entry, err)
		}
	}
	return nil
}

func validateTaskVideoDirectHostName(host string) error {
	if host == "" || strings.Contains(host, "*") {
		return errors.New("only exact hosts or a leading *. wildcard are allowed")
	}
	if net.ParseIP(host) != nil {
		return errors.New("IP addresses are not allowed")
	}
	if len(host) > 253 {
		return errors.New("host is too long")
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return errors.New("host must contain a registrable domain")
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("host labels must be 1-63 characters and must not start or end with '-'")
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return errors.New("only host names without scheme, port or path are allowed")
			}
		}
	}
	if strings.Trim(labels[len(labels)-1], "0123456789") == "" {
		return errors.New("top-level label must not be numeric")
	}
	return nil
}

package service

import (
	"net/url"
	"strconv"
	"strings"
	"time"
)

// A redirect is only useful if the client can still start the download.
const taskVideoDirectExpirySkew = time.Minute

const presignedDateLayout = "20060102T150405Z"

// taskVideoURLExpiry reads the expiry a pre-signed object URL states about
// itself. It never contacts the host. ok is false when the URL carries no
// recognizable or well-formed expiry, so unsigned links keep their behavior.
func taskVideoURLExpiry(parsed *url.URL) (time.Time, bool) {
	if parsed == nil {
		return time.Time{}, false
	}
	// url.Query drops pairs containing ';', which COS uses unescaped in q-sign-time.
	query := map[string]string{}
	for _, pair := range strings.Split(parsed.RawQuery, "&") {
		rawKey, rawValue, _ := strings.Cut(pair, "=")
		key, keyErr := url.QueryUnescape(rawKey)
		value, valueErr := url.QueryUnescape(rawValue)
		if keyErr != nil || valueErr != nil || key == "" {
			continue
		}
		if _, seen := query[strings.ToLower(key)]; !seen {
			query[strings.ToLower(key)] = strings.TrimSpace(value)
		}
	}
	// SigV4-style signatures: Volcengine TOS, S3/R2, GCS and OSS V4.
	for _, prefix := range []string{"x-tos-", "x-amz-", "x-goog-", "x-oss-"} {
		date, hasDate := query[prefix+"date"]
		seconds, hasSeconds := query[prefix+"expires"]
		if !hasDate || !hasSeconds {
			continue
		}
		signed, err := time.Parse(presignedDateLayout, date)
		lifetime, convErr := strconv.ParseInt(seconds, 10, 64)
		if err != nil || convErr != nil || lifetime <= 0 {
			return time.Time{}, false
		}
		return signed.Add(time.Duration(lifetime) * time.Second), true
	}
	// Tencent COS: q-sign-time=<start>;<end>.
	if window, ok := query["q-sign-time"]; ok {
		parts := strings.Split(window, ";")
		if len(parts) != 2 {
			return time.Time{}, false
		}
		end, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end <= 0 {
			return time.Time{}, false
		}
		return time.Unix(end, 0), true
	}
	// Aliyun OSS V1 and S3 SigV2: an absolute Expires next to a signature.
	// A bare Expires without a signature is not trusted as an expiry.
	if expires, ok := query["expires"]; ok {
		if _, signed := query["signature"]; !signed {
			return time.Time{}, false
		}
		end, err := strconv.ParseInt(expires, 10, 64)
		if err != nil || end <= 0 {
			return time.Time{}, false
		}
		return time.Unix(end, 0), true
	}
	return time.Time{}, false
}

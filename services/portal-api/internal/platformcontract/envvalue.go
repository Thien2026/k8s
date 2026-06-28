package platformcontract

import (
	"fmt"
	"strings"
)

// ContainsLocalhostRef — giá trị trỏ máy local (không dùng trên prod deploy).
func ContainsLocalhostRef(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return false
	}
	return strings.Contains(v, "localhost") ||
		strings.Contains(v, "127.0.0.1") ||
		strings.Contains(v, "0.0.0.0") ||
		strings.HasPrefix(v, "http://127.") ||
		strings.HasPrefix(v, "http://0.")
}

// ValidateProdEnvValue — prod không chấp nhận localhost / loopback trong env.
func ValidateProdEnvValue(env, key, value string) error {
	if strings.ToLower(strings.TrimSpace(env)) != "prod" {
		return nil
	}
	if ContainsLocalhostRef(value) {
		return fmt.Errorf("%s: prod không được dùng localhost/127.0.0.1 — dùng /api hoặc domain thật", key)
	}
	return nil
}

package utils

import (
	"strings"
)

// ParseCookieString parses a raw Cookie header string into a map.
func ParseCookieString(cookieStr string) map[string]string {
	cookies := make(map[string]string)
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, "="); idx > 0 {
			key := strings.TrimSpace(part[:idx])
			value := strings.TrimSpace(part[idx+1:])
			cookies[key] = value
		}
	}
	return cookies
}

// HasRequiredCookies checks if cookies represent an authenticated session.
func HasRequiredCookies(cookies map[string]string) bool {
	_, hasTK := cookies["_m_h5_tk"]
	_, hasUNB := cookies["unb"]
	return hasTK && hasUNB
}

// BuildCookieHeader builds a Cookie header string from a map.
func BuildCookieHeader(cookies map[string]string) string {
	parts := make([]string, 0, len(cookies))
	for k, v := range cookies {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

// ExtractBrowserCookies attempts to extract cookies from installed browsers.
// Note: Go doesn't have a direct equivalent of browser-cookie3.
// This is a stub that returns nil - users should use --cookie flag instead.
func ExtractBrowserCookies(browser string) map[string]string {
	// Browser cookie extraction requires platform-specific code
	// (reading Chrome SQLite DB, decrypting with Keychain on macOS, etc.)
	// For now, recommend using --cookie flag to provide cookies manually.
	return nil
}

package httpapi

import (
	"net/mail"
	"strings"
)

// normalizeEmail parses and returns a lowercased bare address.
// Display-name forms like `Alice <a@b.com>` are rejected.
func normalizeEmail(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		return "", false
	}
	if strings.TrimSpace(addr.Name) != "" {
		return "", false
	}
	email := strings.ToLower(strings.TrimSpace(addr.Address))
	if email == "" {
		return "", false
	}
	return email, true
}

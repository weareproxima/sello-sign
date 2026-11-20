package config

import (
	"strings"
	"testing"
)

func TestEmail(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x", "SIGN_SECRETS_KEY": "k"}
	c, err := FromEnv(env(base))
	if err != nil {
		t.Fatal(err)
	}
	// Without a key the notices go to the log, from the panel's sender.
	if c.ResendAPIKey != "" || c.EmailFrom != "Sello Sign <noreply@sign.sello.tax>" {
		t.Fatalf("defaults: %q %q", c.ResendAPIKey, c.EmailFrom)
	}
	base["RESEND_API_KEY"], base["EMAIL_FROM"] = " re_123 ", "Firmas <avisos@example.com>"
	if c, err = FromEnv(env(base)); err != nil || c.ResendAPIKey != "re_123" || c.EmailFrom != "Firmas <avisos@example.com>" {
		t.Fatalf("set: %q %q %v", c.ResendAPIKey, c.EmailFrom, err)
	}
	base["EMAIL_FROM"] = "not an address"
	if _, err := FromEnv(env(base)); err == nil || !strings.Contains(err.Error(), "EMAIL_FROM") || strings.Contains(err.Error(), "not an address") {
		t.Fatalf("invalid: %v", err)
	}
}

// Package config reads the API's configuration from the environment
// (AGENTS.md, «Variables»). Errors name the variable, never its value.
package config

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
)

// DefaultCountries are the countries offered when SIGN_COUNTRIES is unset:
// those already tested against their authority.
var DefaultCountries = []string{"SV", "DO", "CR", "PA", "GT"}

// Config is the API's.
type Config struct {
	Addr        string
	DatabaseURL string
	RedisURL    string
	SecretsKey  string
	// SecretsKeyPrevious opens what an older key sealed, while it rotates.
	SecretsKeyPrevious string
	PanelURL           string
	APIURL             string
	Countries          []string
	Cloud              bool
	PlatformWorkspace  string
	BillingURL         string
	BillingToken       string
	BillingPushToken   string
	// CORSOrigins may call the API from a browser: the panel's origin and
	// CORS_ORIGINS.
	CORSOrigins []string
	// RequestsPerMinute and LogRetentionDays are the install's limits for
	// the spaces without a plan (SIGN_RATE_LIMIT_PER_MINUTE,
	// SIGN_LOG_RETENTION_DAYS); nil is none. In the cloud, billing pushes
	// each space's.
	RequestsPerMinute *int
	LogRetentionDays  *int
	// ResendAPIKey sends the notices through Resend (RESEND_API_KEY); ""
	// writes them to the log. EmailFrom is their sender (EMAIL_FROM). The
	// same variables as the panel's.
	ResendAPIKey string
	EmailFrom    string
	LogFormat    string
	LogLevel     string
}

// DefaultEmailFrom is the notices' sender when EMAIL_FROM is unset: the
// panel's.
const DefaultEmailFrom = "Sello Sign <noreply@sign.sello.tax>"

// Billing says the cloud talks to Sello's billing.
func (c Config) Billing() bool {
	return c.Cloud && c.BillingURL != "" && c.BillingToken != "" && c.BillingPushToken != ""
}

// FromEnv reads the configuration from getenv.
func FromEnv(getenv func(string) string) (Config, error) {
	get := func(k, def string) string {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
		return def
	}
	c := Config{
		Addr:               get("ADDR", "127.0.0.1:4441"),
		DatabaseURL:        get("DATABASE_URL", ""),
		RedisURL:           get("REDIS_URL", ""),
		SecretsKey:         get("SIGN_SECRETS_KEY", ""),
		SecretsKeyPrevious: get("SIGN_SECRETS_KEY_PREVIOUS", ""),
		PanelURL:           strings.TrimRight(get("PANEL_URL", "http://127.0.0.1:4440"), "/"),
		APIURL:             strings.TrimRight(get("API_URL", "http://127.0.0.1:4441"), "/"),
		PlatformWorkspace:  get("SIGN_PLATFORM_WORKSPACE_ID", ""),
		BillingURL:         strings.TrimRight(get("BILLING_URL", ""), "/"),
		BillingToken:       get("BILLING_TOKEN", ""),
		BillingPushToken:   get("BILLING_PUSH_TOKEN", ""),
		ResendAPIKey:       get("RESEND_API_KEY", ""),
		EmailFrom:          get("EMAIL_FROM", DefaultEmailFrom),
		LogFormat:          get("LOG_FORMAT", "json"),
		LogLevel:           get("LOG_LEVEL", "info"),
	}
	switch strings.ToLower(get("CLOUD", "false")) {
	case "true", "1", "yes":
		c.Cloud = true
	case "false", "0", "no":
	default:
		return Config{}, errors.New("config: CLOUD must be true or false")
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("config: DATABASE_URL is required")
	}
	for _, f := range []struct {
		name string
		dst  **int
	}{{"SIGN_RATE_LIMIT_PER_MINUTE", &c.RequestsPerMinute}, {"SIGN_LOG_RETENTION_DAYS", &c.LogRetentionDays}} {
		v := get(f.name, "")
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1_000_000 {
			return Config{}, fmt.Errorf("config: %s must be a whole number from 1 to 1000000, or empty for none", f.name)
		}
		*f.dst = &n
	}
	if c.SecretsKey == "" {
		return Config{}, errors.New("config: SIGN_SECRETS_KEY is required (32 bytes in base64: openssl rand -base64 32)")
	}
	for name, v := range map[string]string{"PANEL_URL": c.PanelURL, "API_URL": c.APIURL} {
		if u, err := url.Parse(v); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Config{}, fmt.Errorf("config: %s must be an http(s) URL", name)
		}
	}
	if _, err := mail.ParseAddress(c.EmailFrom); err != nil {
		return Config{}, errors.New("config: EMAIL_FROM must be an address, like " + DefaultEmailFrom)
	}
	countries := get("SIGN_COUNTRIES", strings.Join(DefaultCountries, ","))
	for _, cc := range strings.Split(countries, ",") {
		cc = strings.ToUpper(strings.TrimSpace(cc))
		if cc == "" {
			continue
		}
		if len(cc) != 2 {
			return Config{}, errors.New("config: SIGN_COUNTRIES must be ISO 3166-1 alpha-2 codes separated by commas")
		}
		if !slices.Contains(c.Countries, cc) {
			c.Countries = append(c.Countries, cc)
		}
	}
	c.CORSOrigins = []string{origin(c.PanelURL)}
	for _, o := range strings.Split(get("CORS_ORIGINS", ""), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			c.CORSOrigins = append(c.CORSOrigins, o)
		}
	}
	if c.Cloud {
		if c.PlatformWorkspace == "" {
			return Config{}, errors.New("config: in the cloud, SIGN_PLATFORM_WORKSPACE_ID is required")
		}
		set := 0
		for _, v := range []string{c.BillingURL, c.BillingToken, c.BillingPushToken} {
			if v != "" {
				set++
			}
		}
		if set != 0 && set != 3 {
			return Config{}, errors.New("config: BILLING_URL, BILLING_TOKEN and BILLING_PUSH_TOKEN go together")
		}
		if c.BillingToken != "" && c.BillingToken == c.BillingPushToken {
			return Config{}, errors.New("config: BILLING_TOKEN and BILLING_PUSH_TOKEN must differ: one per direction")
		}
	}
	return c, nil
}

// FromOS reads the configuration from the process's environment.
func FromOS() (Config, error) { return FromEnv(os.Getenv) }

func origin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

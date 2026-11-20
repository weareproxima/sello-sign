package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestFromEnv(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x", "SIGN_SECRETS_KEY": "k"}
	c, err := FromEnv(env(base))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != "127.0.0.1:4441" || strings.Join(c.Countries, ",") != "SV,DO,CR,PA,GT" || c.Cloud {
		t.Fatalf("defaults: %+v", c)
	}
	if c.CORSOrigins[0] != "http://127.0.0.1:4440" {
		t.Fatalf("cors: %v", c.CORSOrigins)
	}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for kk, vv := range base {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	c, err = FromEnv(env(with("SIGN_COUNTRIES", " sv, cr ,sv")))
	if err != nil || strings.Join(c.Countries, ",") != "SV,CR" {
		t.Fatalf("countries: %v %v", c.Countries, err)
	}
	for _, m := range []map[string]string{
		with("DATABASE_URL", ""),
		with("SIGN_SECRETS_KEY", ""),
		with("CLOUD", "maybe"),
		with("CLOUD", "true"), // without the platform's space
		with("PANEL_URL", "ftp://x"),
		with("SIGN_COUNTRIES", "SLV"),
	} {
		if _, err := FromEnv(env(m)); err == nil {
			t.Errorf("accepted %v", m)
		}
	}
	cloud := with("CLOUD", "true")
	cloud["SIGN_PLATFORM_WORKSPACE_ID"] = "w"
	cloud["BILLING_URL"] = "http://b"
	if _, err := FromEnv(env(cloud)); err == nil {
		t.Error("accepted half of billing")
	}
	cloud["BILLING_TOKEN"], cloud["BILLING_PUSH_TOKEN"] = "same", "same"
	if _, err := FromEnv(env(cloud)); err == nil {
		t.Error("accepted one token for both directions")
	}
	cloud["BILLING_PUSH_TOKEN"] = "other"
	if c, err := FromEnv(env(cloud)); err != nil || !c.Billing() {
		t.Errorf("cloud: %v %v", c.Billing(), err)
	}
}

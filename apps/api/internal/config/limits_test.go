package config

import "testing"

func TestInstallLimits(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x", "SIGN_SECRETS_KEY": "k"}
	c, err := FromEnv(env(base))
	if err != nil || c.RequestsPerMinute != nil || c.LogRetentionDays != nil {
		t.Fatalf("without them, no limits: %+v %v", c, err)
	}
	base["SIGN_RATE_LIMIT_PER_MINUTE"], base["SIGN_LOG_RETENTION_DAYS"] = "300", " 90 "
	c, err = FromEnv(env(base))
	if err != nil || *c.RequestsPerMinute != 300 || *c.LogRetentionDays != 90 {
		t.Fatalf("limits: %+v %v", c, err)
	}
	for _, bad := range []string{"0", "-1", "ten", "1.5"} {
		base["SIGN_LOG_RETENTION_DAYS"] = bad
		if _, err := FromEnv(env(base)); err == nil {
			t.Fatalf("SIGN_LOG_RETENTION_DAYS=%q was taken", bad)
		}
	}
}

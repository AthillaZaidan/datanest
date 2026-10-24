package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

var validKey = base64.StdEncoding.EncodeToString(make([]byte, 32))

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func minimal() map[string]string {
	return map[string]string{
		"PLATFORM_DSN":    "postgres://localhost/platform",
		"CREDENTIALS_KEY": validKey,
	}
}

func TestDefaults(t *testing.T) {
	cfg, err := FromLookup(env(minimal()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AppPort != "8080" {
		t.Errorf("AppPort = %q, want 8080", cfg.AppPort)
	}
	if cfg.CookieSecure {
		t.Error("CookieSecure should default to false")
	}
	want := SessionConfig{
		IdleTimeout:      30 * time.Minute,
		AbsoluteTimeout:  8 * time.Hour,
		RotationInterval: 15 * time.Minute,
		GracePeriod:      30 * time.Second,
		CleanupInterval:  time.Minute,
	}
	if cfg.Session != want {
		t.Errorf("Session = %+v, want %+v", cfg.Session, want)
	}
	if cfg.Target.MigrationExecTimeout != 2*time.Minute {
		t.Errorf("MigrationExecTimeout = %v, want 2m", cfg.Target.MigrationExecTimeout)
	}
	if len(cfg.CredentialsKey) != 32 {
		t.Errorf("CredentialsKey length = %d, want 32", len(cfg.CredentialsKey))
	}
}

func TestOverrides(t *testing.T) {
	m := minimal()
	m["COOKIE_SECURE"] = "true"
	m["SESSION_IDLE_TIMEOUT"] = "2m"
	m["SESSION_ABSOLUTE_TIMEOUT"] = "10m"
	m["SESSION_ROTATION_INTERVAL"] = "30s"
	m["SESSION_GRACE_PERIOD"] = "10s"

	cfg, err := FromLookup(env(m))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.CookieSecure {
		t.Error("CookieSecure should be true")
	}
	if cfg.Session.IdleTimeout != 2*time.Minute || cfg.Session.GracePeriod != 10*time.Second {
		t.Errorf("Session overrides not applied: %+v", cfg.Session)
	}
}

func TestInvalid(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr string
	}{
		{"missing dsn", func(m map[string]string) { delete(m, "PLATFORM_DSN") }, "PLATFORM_DSN is required"},
		{"missing key", func(m map[string]string) { delete(m, "CREDENTIALS_KEY") }, "CREDENTIALS_KEY is required"},
		{"key not base64", func(m map[string]string) { m["CREDENTIALS_KEY"] = "not base64!" }, "must be 32 bytes"},
		{"key wrong length", func(m map[string]string) {
			m["CREDENTIALS_KEY"] = base64.StdEncoding.EncodeToString(make([]byte, 16))
		}, "must be 32 bytes"},
		{"bad duration", func(m map[string]string) { m["TARGET_QUERY_TIMEOUT"] = "soon" }, "TARGET_QUERY_TIMEOUT"},
		{"negative duration", func(m map[string]string) { m["MIGRATION_EXEC_TIMEOUT"] = "-1s" }, "MIGRATION_EXEC_TIMEOUT"},
		{"bad bool", func(m map[string]string) { m["COOKIE_SECURE"] = "maybe" }, "COOKIE_SECURE"},
		{"grace not below rotation", func(m map[string]string) { m["SESSION_GRACE_PERIOD"] = "20m" }, "session timeouts"},
		{"idle not below absolute", func(m map[string]string) { m["SESSION_IDLE_TIMEOUT"] = "9h" }, "session timeouts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := minimal()
			tt.mutate(m)
			_, err := FromLookup(env(m))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestKeyNotEchoedInError(t *testing.T) {
	m := minimal()
	m["CREDENTIALS_KEY"] = "c2VjcmV0LXZhbHVl"
	_, err := FromLookup(env(m))
	if err == nil || strings.Contains(err.Error(), m["CREDENTIALS_KEY"]) {
		t.Fatalf("error must not contain the key value: %v", err)
	}
}

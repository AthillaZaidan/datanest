package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func lookupWith(extra map[string]string) func(string) (string, bool) {
	env := map[string]string{
		"PLATFORM_DSN":    "postgres://x",
		"CREDENTIALS_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
	}
	for k, v := range extra {
		env[k] = v
	}
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}

func TestLogAndTraceSettingsDefaultAndValidate(t *testing.T) {
	cfg, err := FromLookup(lookupWith(nil))
	if err != nil || cfg.LogLevel != "info" || cfg.TraceExporter != "console" {
		t.Fatalf("defaults = %+v, %v", cfg, err)
	}
	cfg, err = FromLookup(lookupWith(map[string]string{"LOG_LEVEL": "debug", "TRACE_EXPORTER": "none"}))
	if err != nil || cfg.LogLevel != "debug" || cfg.TraceExporter != "none" {
		t.Fatalf("explicit = %+v, %v", cfg, err)
	}
	if _, err := FromLookup(lookupWith(map[string]string{"LOG_LEVEL": "loud"})); err == nil || !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Errorf("bad level: %v", err)
	}
	if _, err := FromLookup(lookupWith(map[string]string{"TRACE_EXPORTER": "jaeger"})); err == nil || !strings.Contains(err.Error(), "TRACE_EXPORTER") {
		t.Errorf("bad exporter: %v", err)
	}
}

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

func TestAMQPURLIsOptional(t *testing.T) {
	cfg, err := FromLookup(env(minimal()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AMQPURL != "" {
		t.Errorf("AMQPURL = %q, want empty by default", cfg.AMQPURL)
	}

	m := minimal()
	m["AMQP_URL"] = "amqp://user:pass@broker:5672/"
	cfg, err = FromLookup(env(m))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AMQPURL != "amqp://user:pass@broker:5672/" {
		t.Errorf("AMQPURL = %q", cfg.AMQPURL)
	}
}

func TestWorkerDefaults(t *testing.T) {
	cfg, err := FromLookup(env(minimal()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := WorkerConfig{Concurrency: 4, MaxAttempts: 3, LeaseTTL: 30 * time.Second, HeartbeatInterval: 10 * time.Second, SweepInterval: 5 * time.Second, RepublishAfter: 30 * time.Second, MetricsAddr: ":9101"}
	if cfg.Worker != want {
		t.Errorf("worker config = %+v, want %+v", cfg.Worker, want)
	}
}

func TestWorkerSettingsAreValidated(t *testing.T) {
	cases := []struct {
		name string
		set  map[string]string
		want string
	}{
		{"zero concurrency", map[string]string{"WORKER_CONCURRENCY": "0"}, "WORKER_CONCURRENCY"},
		{"not a number", map[string]string{"WORKER_MAX_ATTEMPTS": "many"}, "WORKER_MAX_ATTEMPTS"},
		{"heartbeat not shorter than the lease", map[string]string{"WORKER_HEARTBEAT": "30s", "WORKER_LEASE": "30s"}, "WORKER_HEARTBEAT must be shorter"},
		{"bad duration", map[string]string{"WORKER_SWEEP_INTERVAL": "soon"}, "WORKER_SWEEP_INTERVAL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := minimal()
			for k, v := range c.set {
				m[k] = v
			}
			if _, err := FromLookup(env(m)); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestWorkerSettingsAreRead(t *testing.T) {
	m := minimal()
	m["WORKER_CONCURRENCY"] = "9"
	m["WORKER_LEASE"] = "1m"
	m["WORKER_HEARTBEAT"] = "20s"
	cfg, err := FromLookup(env(m))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worker.Concurrency != 9 || cfg.Worker.LeaseTTL != time.Minute || cfg.Worker.HeartbeatInterval != 20*time.Second {
		t.Errorf("worker config = %+v", cfg.Worker)
	}
}

func TestObservabilityDefaults(t *testing.T) {
	cfg, err := FromLookup(env(minimal()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MetricsAddr != ":9100" || cfg.Worker.MetricsAddr != ":9101" {
		t.Errorf("metrics addresses = %q, %q; want :9100 and :9101", cfg.MetricsAddr, cfg.Worker.MetricsAddr)
	}
	if cfg.OTLPEndpoint != "" || cfg.TraceExporter != "console" {
		t.Errorf("OTLPEndpoint = %q, TraceExporter = %q; want no OTLP export and console spans by default", cfg.OTLPEndpoint, cfg.TraceExporter)
	}
}

func TestObservabilitySettingsAreRead(t *testing.T) {
	m := minimal()
	m["METRICS_ADDR"] = "127.0.0.1:9200"
	m["WORKER_METRICS_ADDR"] = "off"
	m["OTLP_ENDPOINT"] = "http://localhost:4318"
	m["TRACE_EXPORTER"] = "none"
	cfg, err := FromLookup(env(m))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MetricsAddr != "127.0.0.1:9200" || cfg.Worker.MetricsAddr != "" {
		t.Errorf("metrics addresses = %q, %q; want 127.0.0.1:9200 and off", cfg.MetricsAddr, cfg.Worker.MetricsAddr)
	}
	if cfg.OTLPEndpoint != "http://localhost:4318" || cfg.TraceExporter != "none" {
		t.Errorf("OTLPEndpoint = %q, TraceExporter = %q", cfg.OTLPEndpoint, cfg.TraceExporter)
	}
}

func TestObservabilitySettingsAreValidated(t *testing.T) {
	cases := []struct {
		name string
		set  map[string]string
		want string
	}{
		{"metrics address without a port", map[string]string{"METRICS_ADDR": "localhost"}, "METRICS_ADDR"},
		{"worker metrics address without a port", map[string]string{"WORKER_METRICS_ADDR": "9101"}, "WORKER_METRICS_ADDR"},
		{"endpoint without a scheme", map[string]string{"OTLP_ENDPOINT": "localhost:4318"}, "OTLP_ENDPOINT"},
		{"endpoint with credentials", map[string]string{"OTLP_ENDPOINT": "http://user:secret@localhost:4318"}, "OTLP_ENDPOINT"},
		{"unknown trace exporter", map[string]string{"TRACE_EXPORTER": "jaeger"}, "TRACE_EXPORTER"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := minimal()
			for k, v := range c.set {
				m[k] = v
			}
			_, err := FromLookup(env(m))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to mention %q", err, c.want)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Errorf("error echoes the credential: %v", err)
			}
		})
	}
}

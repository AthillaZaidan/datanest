// Package config loads the server configuration from the environment.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppPort       string
	AllowedOrigin string
	CookieSecure  bool
	// Built SPA served by the backend, relative to where the server runs (backend/).
	FrontendDir string

	PlatformDSN string

	AMQPURL string

	// Seed the first Platform Admin when none exists.
	AdminUsername string
	AdminPassword string

	// AES-256 key for target DB passwords.
	CredentialsKey []byte

	Session SessionConfig
	Target  TargetConfig
	Worker  WorkerConfig
}

type WorkerConfig struct {
	Concurrency       int
	MaxAttempts       int
	LeaseTTL          time.Duration
	HeartbeatInterval time.Duration
	SweepInterval     time.Duration
	RepublishAfter    time.Duration
}

// Invariant: grace < rotation < idle < absolute.
type SessionConfig struct {
	IdleTimeout      time.Duration
	AbsoluteTimeout  time.Duration
	RotationInterval time.Duration
	GracePeriod      time.Duration
	CleanupInterval  time.Duration
}

type TargetConfig struct {
	QueryTimeout              time.Duration
	MigrationLockTimeout      time.Duration
	MigrationStatementTimeout time.Duration
	MigrationExecTimeout      time.Duration
}

func Load() (*Config, error) {
	return FromLookup(os.LookupEnv)
}

// FromLookup lets tests supply their own environment.
func FromLookup(lookup func(string) (string, bool)) (*Config, error) {
	r := reader{lookup: lookup}

	cfg := &Config{
		AppPort:       r.str("APP_PORT", "8080"),
		AllowedOrigin: r.str("ALLOWED_ORIGIN", "http://localhost:8080"),
		CookieSecure:  r.boolean("COOKIE_SECURE", false),
		FrontendDir:   r.str("FRONTEND_DIR", "../frontend/dist"),
		PlatformDSN:   r.str("PLATFORM_DSN", ""),
		AMQPURL:       r.str("AMQP_URL", ""),
		AdminUsername: r.str("ADMIN_USERNAME", ""),
		AdminPassword: r.str("ADMIN_PASSWORD", ""),
		Session: SessionConfig{
			IdleTimeout:      r.duration("SESSION_IDLE_TIMEOUT", 30*time.Minute),
			AbsoluteTimeout:  r.duration("SESSION_ABSOLUTE_TIMEOUT", 8*time.Hour),
			RotationInterval: r.duration("SESSION_ROTATION_INTERVAL", 15*time.Minute),
			GracePeriod:      r.duration("SESSION_GRACE_PERIOD", 30*time.Second),
			CleanupInterval:  r.duration("SESSION_CLEANUP_INTERVAL", time.Minute),
		},
		Target: TargetConfig{
			QueryTimeout:              r.duration("TARGET_QUERY_TIMEOUT", 5*time.Second),
			MigrationLockTimeout:      r.duration("MIGRATION_LOCK_TIMEOUT", 5*time.Second),
			MigrationStatementTimeout: r.duration("MIGRATION_STATEMENT_TIMEOUT", 60*time.Second),
			MigrationExecTimeout:      r.duration("MIGRATION_EXEC_TIMEOUT", 2*time.Minute),
		},
	}
	cfg.Worker = WorkerConfig{
		Concurrency:       r.integer("WORKER_CONCURRENCY", 4),
		MaxAttempts:       r.integer("WORKER_MAX_ATTEMPTS", 3),
		LeaseTTL:          r.duration("WORKER_LEASE", 30*time.Second),
		HeartbeatInterval: r.duration("WORKER_HEARTBEAT", 10*time.Second),
		SweepInterval:     r.duration("WORKER_SWEEP_INTERVAL", 5*time.Second),
		RepublishAfter:    r.duration("WORKER_REPUBLISH_AFTER", 30*time.Second),
	}
	cfg.CredentialsKey = r.credentialsKey("CREDENTIALS_KEY")

	if cfg.PlatformDSN == "" {
		r.fail("PLATFORM_DSN is required")
	}
	s := cfg.Session
	if !(s.GracePeriod < s.RotationInterval && s.RotationInterval < s.IdleTimeout && s.IdleTimeout < s.AbsoluteTimeout) {
		r.fail("session timeouts must satisfy SESSION_GRACE_PERIOD < SESSION_ROTATION_INTERVAL < SESSION_IDLE_TIMEOUT < SESSION_ABSOLUTE_TIMEOUT")
	}

	if cfg.Worker.HeartbeatInterval >= cfg.Worker.LeaseTTL {
		r.fail("WORKER_HEARTBEAT must be shorter than WORKER_LEASE")
	}

	if err := errors.Join(r.errs...); err != nil {
		return nil, err
	}
	return cfg, nil
}

type reader struct {
	lookup func(string) (string, bool)
	errs   []error
}

func (r *reader) fail(format string, args ...any) {
	r.errs = append(r.errs, fmt.Errorf(format, args...))
}

func (r *reader) str(key, def string) string {
	if v, ok := r.lookup(key); ok && v != "" {
		return v
	}
	return def
}

func (r *reader) boolean(key string, def bool) bool {
	v, ok := r.lookup(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		r.fail("%s: invalid boolean %q", key, v)
		return def
	}
	return b
}

func (r *reader) integer(key string, def int) int {
	v, ok := r.lookup(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		r.fail("%s: must be a positive integer, got %q", key, v)
		return def
	}
	return n
}

func (r *reader) duration(key string, def time.Duration) time.Duration {
	v, ok := r.lookup(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		r.fail("%s: invalid duration %q", key, v)
		return def
	}
	return d
}

func (r *reader) credentialsKey(key string) []byte {
	v, _ := r.lookup(key)
	if v == "" {
		r.fail("%s is required (generate with: openssl rand -base64 32)", key)
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil || len(b) != 32 {
		r.fail("%s must be 32 bytes encoded as base64", key)
		return nil
	}
	return b
}

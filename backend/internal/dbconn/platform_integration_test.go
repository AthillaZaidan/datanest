//go:build integration

package dbconn

import (
	"os"
	"testing"
)

func TestOpenPlatformCapsTheConnectionPool(t *testing.T) {
	dsn := os.Getenv("TEST_PLATFORM_DSN")
	if dsn == "" {
		t.Skip("TEST_PLATFORM_DSN not set")
	}
	db, err := OpenPlatform(dsn, 3)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if got := sqlDB.Stats().MaxOpenConnections; got != 3 {
		t.Errorf("max open connections = %d, want 3", got)
	}
	if err := db.Exec("SELECT 1").Error; err != nil {
		t.Errorf("query through the capped pool: %v", err)
	}
}

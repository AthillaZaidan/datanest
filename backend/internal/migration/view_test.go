package migration

import (
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func TestUserLabel(t *testing.T) {
	id := uint(3)
	if got := userLabel(&id, "budi"); got != "budi" {
		t.Errorf("existing user = %q", got)
	}
	if got := userLabel(nil, "budi"); got != "budi (deleted)" {
		t.Errorf("deleted user = %q", got)
	}
	if got := userLabel(nil, ""); got != "" {
		t.Errorf("no user = %q", got)
	}
}

func TestBaseOutdated(t *testing.T) {
	two := 2
	tests := []struct {
		name    string
		status  models.MigrationStatus
		base    *int
		current int
		want    bool
	}{
		{"draft up to date", models.MigrationDraft, &two, 2, false},
		{"draft behind", models.MigrationDraft, &two, 3, true},
		{"draft on empty db", models.MigrationDraft, nil, 0, false},
		{"draft written before first migration", models.MigrationDraft, nil, 1, true},
		{"executed never outdated", models.MigrationSuccess, &two, 5, false},
	}
	for _, tt := range tests {
		v := toView(&models.Migration{Status: tt.status, BaseVersion: tt.base}, tt.current)
		if v.BaseOutdated != tt.want {
			t.Errorf("%s: BaseOutdated = %v, want %v", tt.name, v.BaseOutdated, tt.want)
		}
	}
}

func TestValidateSQL(t *testing.T) {
	if validateSQL("  \n\t") == nil {
		t.Error("blank SQL must be rejected")
	}
	if validateSQL("SELECT 1") != nil {
		t.Error("non-blank SQL must pass")
	}
}

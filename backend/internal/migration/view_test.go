package migration

import (
	"errors"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"

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

func TestRecipeValidate(t *testing.T) {
	tests := []struct {
		name    string
		r       Recipe
		invalid []string
	}{
		{"blank up", Recipe{SQLUp: "  \n\t", SQLDown: "x"}, []string{"sqlUp"}},
		{"reversible draft may lack down for now", Recipe{SQLUp: "SELECT 1"}, nil},
		{"irreversible needs a reason", Recipe{SQLUp: "SELECT 1", Irreversible: true}, []string{"irreversibleReason"}},
		{"irreversible with a reason", Recipe{SQLUp: "SELECT 1", Irreversible: true, IrreversibleReason: "data hilang"}, nil},
	}
	for _, tt := range tests {
		err := tt.r.validate()
		var e *apperr.Error
		if len(tt.invalid) == 0 {
			if err != nil {
				t.Errorf("%s: %v", tt.name, err)
			}
			continue
		}
		if !errors.As(err, &e) || e.Code != "VALIDATION_ERROR" {
			t.Errorf("%s: err = %v, want validation error", tt.name, err)
			continue
		}
		details, _ := e.Details.(map[string]string)
		for _, f := range tt.invalid {
			if details[f] == "" {
				t.Errorf("%s: details = %v, want %s", tt.name, details, f)
			}
		}
	}
}

func TestRecipeValidateNormalisesTheDownSQL(t *testing.T) {
	r := Recipe{SQLUp: "DROP TABLE t", SQLDown: "CREATE TABLE t ()", Irreversible: true, IrreversibleReason: "data hilang"}
	if err := r.validate(); err != nil || r.SQLDown != "" {
		t.Errorf("irreversible keeps down SQL %q (%v)", r.SQLDown, err)
	}
	r = Recipe{SQLUp: "SELECT 1", SQLDown: "SELECT 2", IrreversibleReason: "stale"}
	if err := r.validate(); err != nil || r.IrreversibleReason != "" {
		t.Errorf("reversible keeps reason %q (%v)", r.IrreversibleReason, err)
	}
}

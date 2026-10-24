package authz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestRequiredLevel(t *testing.T) {
	tests := map[Action]Level{
		ViewDatabase:       View,
		ViewSchema:         View,
		ViewData:           View,
		ViewMigrations:     View,
		CreateMigration:    Edit,
		UpdateMigration:    Edit,
		DeleteMigration:    Edit,
		DuplicateMigration: Edit,
		ExecuteMigration:   Edit,
	}
	for action, want := range tests {
		got, ok := RequiredLevel(action)
		if !ok || got != want {
			t.Errorf("RequiredLevel(%s) = %v, %v; want %v, true", action, got, ok, want)
		}
	}
	if len(required) != len(tests) {
		t.Errorf("required has %d actions, test covers %d", len(required), len(tests))
	}
}

func TestUnknownActionDenied(t *testing.T) {
	if _, ok := RequiredLevel("drop_everything"); ok {
		t.Fatal("unknown action must not have a required level")
	}
	ok, err := New(nil).Can(context.Background(), 1, 1, "drop_everything")
	if ok || err != nil {
		t.Fatalf("Can(unknown) = %v, %v; want false, nil", ok, err)
	}
}

func TestCanDeniesOnLookupError(t *testing.T) {
	ok, err := New(nil).Can(context.Background(), 1, 1, ViewSchema)
	if ok || !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Can = %v, %v; want false, ErrNotImplemented", ok, err)
	}
}

func TestLevelOrdering(t *testing.T) {
	if !(None < View && View < Edit) {
		t.Fatal("levels must be ordered NONE < VIEW < EDIT")
	}
}

func TestLevelJSON(t *testing.T) {
	b, err := json.Marshal(struct{ Level Level }{Edit})
	if err != nil || string(b) != `{"Level":"EDIT"}` {
		t.Fatalf("marshal = %s, %v", b, err)
	}

	var v struct{ Level Level }
	if err := json.Unmarshal([]byte(`{"Level":"VIEW"}`), &v); err != nil || v.Level != View {
		t.Fatalf("unmarshal VIEW = %v, %v", v.Level, err)
	}
	for _, bad := range []string{`"NONE"`, `"view"`, `"ADMIN"`} {
		if err := json.Unmarshal([]byte(`{"Level":`+bad+`}`), &v); err == nil {
			t.Errorf("unmarshal %s should fail", bad)
		}
	}
}

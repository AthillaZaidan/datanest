package models

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func parse(t *testing.T, model any) *schema.Schema {
	t.Helper()
	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse %T: %v", model, err)
	}
	return s
}

func TestAllModelsParse(t *testing.T) {
	for _, m := range All() {
		parse(t, m)
	}
}

func TestNoSoftDelete(t *testing.T) {
	for _, m := range All() {
		s := parse(t, m)
		if s.LookUpField("DeletedAt") != nil {
			t.Errorf("%s must not have DeletedAt (soft delete is forbidden)", s.Name)
		}
	}
}

func TestTableNames(t *testing.T) {
	want := map[any]string{
		&User{}:               "users",
		&Role{}:               "roles",
		&UserRole{}:           "user_roles",
		&Group{}:              "groups",
		&GroupMember{}:        "group_members",
		&DatabaseConnection{}: "database_connections",
		&Grant{}:              "grants",
		&Migration{}:          "migrations",
	}
	for m, name := range want {
		if got := parse(t, m).Table; got != name {
			t.Errorf("%T table = %q, want %q", m, got, name)
		}
	}
}

func TestCompositePrimaryKeys(t *testing.T) {
	tests := []struct {
		model any
		keys  []string
	}{
		{&UserRole{}, []string{"user_id", "role_id"}},
		{&GroupMember{}, []string{"group_id", "user_id"}},
		{&Grant{}, []string{"group_id", "database_id"}},
	}
	for _, tt := range tests {
		s := parse(t, tt.model)
		if len(s.PrimaryFieldDBNames) != len(tt.keys) {
			t.Fatalf("%s primary key = %v, want %v", s.Name, s.PrimaryFieldDBNames, tt.keys)
		}
		for i, k := range tt.keys {
			if s.PrimaryFieldDBNames[i] != k {
				t.Errorf("%s primary key = %v, want %v", s.Name, s.PrimaryFieldDBNames, tt.keys)
			}
		}
	}
}

func TestDeleteBehaviour(t *testing.T) {
	tests := []struct {
		model    any
		relation string
		onDelete string
	}{
		{&UserRole{}, "User", "CASCADE"},
		{&UserRole{}, "Role", "CASCADE"},
		{&GroupMember{}, "Group", "CASCADE"},
		{&GroupMember{}, "User", "CASCADE"},
		{&Grant{}, "Group", "CASCADE"},
		{&Grant{}, "Database", "CASCADE"},
		{&Migration{}, "Database", "CASCADE"},
		{&Migration{}, "CreatedBy", "SET NULL"},
		{&Migration{}, "UpdatedBy", "SET NULL"},
		{&Migration{}, "ExecutedBy", "SET NULL"},
	}
	for _, tt := range tests {
		s := parse(t, tt.model)
		rel, ok := s.Relationships.Relations[tt.relation]
		if !ok {
			t.Fatalf("%s has no relation %s", s.Name, tt.relation)
		}
		c := rel.ParseConstraint()
		if c == nil || c.OnDelete != tt.onDelete {
			t.Errorf("%s.%s ON DELETE = %v, want %s", s.Name, tt.relation, c, tt.onDelete)
		}
	}
}

func TestSecretFieldsHiddenFromJSON(t *testing.T) {
	for model, field := range map[any]string{&User{}: "PasswordHash", &DatabaseConnection{}: "PasswordEnc"} {
		f := parse(t, model).LookUpField(field)
		if f == nil || f.StructField.Tag.Get("json") != "-" {
			t.Errorf("%T.%s must have json:\"-\"", model, field)
		}
	}
}

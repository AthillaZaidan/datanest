//go:build integration

package admin_test

import (
	"context"
	"errors"
	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
	"testing"
)

type sessions struct{ rotate, revoke map[uint]int }

func (s *sessions) MarkNeedsRotation(u uint) { s.rotate[u]++ }
func (s *sessions) RevokeAllForUser(u uint)  { s.revoke[u]++ }
func TestAdminLifecycle(t *testing.T) {
	db := testutil.PlatformDB(t)
	ctx := context.Background()
	a := testutil.CreateAdmin(t, db, "admin")
	u := testutil.CreateUser(t, db, "user")
	ss := &sessions{map[uint]int{}, map[uint]int{}}
	s := admin.New(db, ss)
	if _, e := s.Users(ctx, u.ID); !errors.Is(e, apperr.ErrForbidden) {
		t.Fatal(e)
	}
	created, e := s.CreateUser(ctx, a.ID, "newuser", "password123")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CreateUser(ctx, a.ID, "newuser", "password123"); !errors.Is(e, apperr.ErrUsernameTaken) {
		t.Fatal(e)
	}
	g, e := s.CreateGroup(ctx, a.ID, "dev", "description")
	if e != nil {
		t.Fatal(e)
	}
	empty := ""
	if _, e = s.UpdateGroup(ctx, a.ID, g.ID, admin.GroupPatch{Description: &empty}); e != nil {
		t.Fatal(e)
	}
	if e = s.Member(ctx, a.ID, g.ID, created.ID, true); e != nil {
		t.Fatal(e)
	}
	if e = s.Member(ctx, a.ID, g.ID, created.ID, true); e != nil {
		t.Fatal(e)
	}
	if ss.rotate[created.ID] != 1 {
		t.Fatal("membership not idempotent")
	}
	d := testutil.CreateDatabaseConnection(t, db, "target")
	if e = s.Grant(ctx, a.ID, g.ID, d.ID, "VIEW", true); e != nil {
		t.Fatal(e)
	}
	if e = s.Grant(ctx, a.ID, g.ID, d.ID, "EDIT", true); e != nil {
		t.Fatal(e)
	}
	var n int64
	db.Model(&models.Grant{}).Count(&n)
	if n != 1 {
		t.Fatal("grant duplicated")
	}
	if ss.rotate[created.ID] != 3 {
		t.Fatal("grant rotation")
	}
	if e = s.Role(ctx, a.ID, created.ID, models.RolePlatformAdmin, true); e != nil {
		t.Fatal(e)
	}
	if ss.rotate[created.ID] != 4 {
		t.Fatal("role rotation")
	}
	if e = s.Role(ctx, a.ID, created.ID, models.RolePlatformAdmin, false); e != nil {
		t.Fatal(e)
	}
	if ss.revoke[created.ID] != 1 {
		t.Fatal("role revoke")
	}
	version := 1
	m := models.Migration{DatabaseID: d.ID, Version: &version, SQL: "SELECT 1", Status: models.MigrationSuccess, CreatedByID: &created.ID, CreatedByUsername: created.Username, ExecutedByID: &created.ID, ExecutedByUsername: created.Username}
	if e = db.Create(&m).Error; e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteUser(ctx, a.ID, created.ID); e != nil {
		t.Fatal(e)
	}
	if ss.revoke[created.ID] != 2 {
		t.Fatal("delete revoke")
	}
	if e = db.First(&m, m.ID).Error; e != nil {
		t.Fatal(e)
	}
	if m.ExecutedByID != nil || m.ExecutedByUsername != "newuser" {
		t.Fatal("history lost")
	}
	if e = s.Member(ctx, a.ID, g.ID, u.ID, true); e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteGroup(ctx, a.ID, g.ID); e != nil {
		t.Fatal(e)
	}
	if l, e := authz.New(db).EffectiveLevel(ctx, u.ID, d.ID); e != nil || l != authz.None {
		t.Fatal("access remains", e)
	}
}

func TestLastAdminIsKept(t *testing.T) {
	db := testutil.PlatformDB(t)
	ctx := context.Background()
	a := testutil.CreateAdmin(t, db, "admin")
	s := admin.New(db, &sessions{map[uint]int{}, map[uint]int{}})
	if e := s.Role(ctx, a.ID, a.ID, models.RolePlatformAdmin, false); !errors.Is(e, apperr.ErrLastAdmin) {
		t.Fatalf("demote last admin: %v", e)
	}
	if e := s.DeleteUser(ctx, a.ID, a.ID); !errors.Is(e, apperr.ErrLastAdmin) {
		t.Fatalf("delete last admin: %v", e)
	}
	b := testutil.CreateAdmin(t, db, "second")
	if e := s.Role(ctx, a.ID, a.ID, models.RolePlatformAdmin, false); e != nil {
		t.Fatal(e)
	}
	if e := s.DeleteUser(ctx, b.ID, b.ID); !errors.Is(e, apperr.ErrLastAdmin) {
		t.Fatalf("delete remaining admin: %v", e)
	}
}

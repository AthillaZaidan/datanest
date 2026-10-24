//go:build integration

package authz_test

import (
	"context"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
	"testing"
)

func TestEffectiveAccess(t *testing.T) {
	db := testutil.PlatformDB(t)
	ctx := context.Background()
	s := authz.New(db)
	u := testutil.CreateUser(t, db, "user")
	a := testutil.CreateAdmin(t, db, "admin")
	d := testutil.CreateDatabaseConnection(t, db, "db")
	g1 := testutil.CreateGroup(t, db, "view")
	g2 := testutil.CreateGroup(t, db, "edit")
	testutil.AddMember(t, db, g1.ID, u.ID)
	testutil.GrantAccess(t, db, g1.ID, d.ID, authz.View)
	if l, e := s.EffectiveLevel(ctx, u.ID, d.ID); e != nil || l != authz.View {
		t.Fatalf("view %v %v", l, e)
	}
	testutil.AddMember(t, db, g2.ID, u.ID)
	testutil.GrantAccess(t, db, g2.ID, d.ID, authz.Edit)
	if l, e := s.EffectiveLevel(ctx, u.ID, d.ID); e != nil || l != authz.Edit {
		t.Fatalf("max %v %v", l, e)
	}
	if l, e := s.EffectiveLevel(ctx, a.ID, d.ID); e != nil || l != authz.None {
		t.Fatalf("admin bypass %v %v", l, e)
	}
	for _, act := range []authz.Action{authz.ViewData, authz.ExecuteMigration} {
		if ok, e := s.Can(ctx, u.ID, d.ID, act); e != nil || !ok {
			t.Fatal(act, e)
		}
	}
}

//go:build integration

package dbconn

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

// The database list and detail show the active position, so rolling back the only
// migration shows 0 even though that migration once succeeded.
func TestCurrentVersionFollowsTheActivePosition(t *testing.T) {
	db := testutil.PlatformDB(t)
	ctx := context.Background()
	c, _ := NewCipher(make([]byte, 32))
	p := NewPools(db, c, 200*time.Millisecond)
	defer p.Close()
	s := New(db, c, p, authz.New(db), 200*time.Millisecond)

	u := testutil.CreateUser(t, db, "user")
	stranger := testutil.CreateUser(t, db, "stranger")
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	g := testutil.CreateGroup(t, db, "view")
	testutil.AddMember(t, db, g.ID, u.ID)
	testutil.GrantAccess(t, db, g.ID, conn.ID, authz.View)

	m := testutil.CreateMigration(t, db, conn.ID)
	if err := db.Model(&m).Update("version", 1).Error; err != nil {
		t.Fatal(err)
	}
	run := testutil.CreateExecution(t, db, models.MigrationExecution{MigrationID: m.ID, DatabaseID: conn.ID, Status: models.ExecSuccess})

	current := func() (int, int) {
		t.Helper()
		list, err := s.ListForUser(ctx, u.ID)
		if err != nil || len(list) != 1 {
			t.Fatalf("list = %+v, %v", list, err)
		}
		detail, err := s.GetForUser(ctx, u.ID, conn.ID)
		if err != nil {
			t.Fatal(err)
		}
		return list[0].CurrentVersion, detail.CurrentVersion
	}

	if l, d := current(); l != 1 || d != 1 {
		t.Fatalf("after run: list %d detail %d, want 1", l, d)
	}
	testutil.CreateExecution(t, db, models.MigrationExecution{MigrationID: m.ID, DatabaseID: conn.ID,
		Kind: models.ExecKindRollback, Status: models.ExecSuccess, RevertsExecutionID: &run.ID})
	if l, d := current(); l != 0 || d != 0 {
		t.Fatalf("after rolling back v1: list %d detail %d, want 0", l, d)
	}

	if _, err := s.GetForUser(ctx, stranger.ID, conn.ID); !errors.Is(err, apperr.ErrForbidden) {
		t.Fatalf("user without a grant = %v, want forbidden", err)
	}
	if list, err := s.ListForUser(ctx, stranger.ID); err != nil || len(list) != 0 {
		t.Fatalf("stranger list = %+v, %v", list, err)
	}
}

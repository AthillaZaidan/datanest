//go:build integration

package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

type leaderEnv struct {
	ctx         context.Context
	db          *gorm.DB
	svc         *admin.Service
	admin, sari models.User
	outsider    models.User
	group       models.Group
}

func newLeaderEnv(t *testing.T) leaderEnv {
	t.Helper()
	db := testutil.PlatformDB(t)
	e := leaderEnv{ctx: context.Background(), db: db}
	e.admin = testutil.CreateAdmin(t, db, "admin")
	e.sari = testutil.CreateUser(t, db, "sari")
	e.outsider = testutil.CreateUser(t, db, "outsider")
	e.group = testutil.CreateGroup(t, db, "Tim A")
	testutil.AddMember(t, db, e.group.ID, e.sari.ID)
	e.svc = admin.New(db, &sessions{map[uint]int{}, map[uint]int{}})
	return e
}

func TestOnlyAnAdminSetsALeaderWhoIsAMember(t *testing.T) {
	e := newLeaderEnv(t)

	if _, err := e.svc.SetLeader(e.ctx, e.sari.ID, e.group.ID, e.sari.ID); !errors.Is(err, apperr.ErrForbidden) {
		t.Errorf("non-admin = %v, want FORBIDDEN", err)
	}
	if _, err := e.svc.SetLeader(e.ctx, e.admin.ID, e.group.ID, e.outsider.ID); !errors.Is(err, apperr.ErrValidation) {
		t.Errorf("non-member = %v, want VALIDATION_ERROR", err)
	}
	if _, err := e.svc.SetLeader(e.ctx, e.admin.ID, 987654, e.sari.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("missing group = %v, want NOT_FOUND", err)
	}
	got, err := e.svc.SetLeader(e.ctx, e.admin.ID, e.group.ID, e.sari.ID)
	if err != nil || got.LeaderID == nil || *got.LeaderID != e.sari.ID || got.LeaderUsername != "sari" {
		t.Fatalf("SetLeader = %+v, %v", got, err)
	}
	views, err := e.svc.Groups(e.ctx, e.admin.ID)
	if err != nil || len(views) != 1 || views[0].LeaderUsername != "sari" {
		t.Errorf("Groups = %+v, %v", views, err)
	}
}

func TestAGroupHasOneLeaderAndAPersonCanLeadManyGroups(t *testing.T) {
	e := newLeaderEnv(t)
	joko := testutil.CreateUser(t, e.db, "joko")
	testutil.AddMember(t, e.db, e.group.ID, joko.ID)

	if _, err := e.svc.SetLeader(e.ctx, e.admin.ID, e.group.ID, e.sari.ID); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.SetLeader(e.ctx, e.admin.ID, e.group.ID, joko.ID)
	if err != nil || *got.LeaderID != joko.ID {
		t.Fatalf("replace leader = %+v, %v", got, err)
	}

	other := testutil.CreateGroup(t, e.db, "Tim B")
	testutil.AddMember(t, e.db, other.ID, e.sari.ID)
	if _, err := e.svc.SetLeader(e.ctx, e.admin.ID, other.ID, e.sari.ID); err != nil {
		t.Errorf("one person leading a second group: %v", err)
	}
}

func TestLeaderLeavesWhenRemovedFromTheGroupOrCleared(t *testing.T) {
	e := newLeaderEnv(t)
	e.svc.SetLeader(e.ctx, e.admin.ID, e.group.ID, e.sari.ID)

	if err := e.svc.Member(e.ctx, e.admin.ID, e.group.ID, e.sari.ID, false); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.Group(e.ctx, e.admin.ID, e.group.ID)
	if err != nil || got.LeaderID != nil {
		t.Fatalf("after leaving the group: %+v, %v; a non-member cannot stay leader", got, err)
	}

	e.svc.Member(e.ctx, e.admin.ID, e.group.ID, e.sari.ID, true)
	e.svc.SetLeader(e.ctx, e.admin.ID, e.group.ID, e.sari.ID)
	got, err = e.svc.ClearLeader(e.ctx, e.admin.ID, e.group.ID)
	if err != nil || got.LeaderID != nil || got.LeaderUsername != "" {
		t.Fatalf("ClearLeader = %+v, %v", got, err)
	}
	if _, err := e.svc.ClearLeader(e.ctx, e.sari.ID, e.group.ID); !errors.Is(err, apperr.ErrForbidden) {
		t.Errorf("non-admin clear = %v", err)
	}
	if _, err := e.svc.ClearLeader(e.ctx, e.admin.ID, 987654); !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("clear on a missing group = %v", err)
	}
}

func TestDeletingTheLeaderUserLeavesTheGroupWithoutLeader(t *testing.T) {
	e := newLeaderEnv(t)
	e.svc.SetLeader(e.ctx, e.admin.ID, e.group.ID, e.sari.ID)
	if err := e.svc.DeleteUser(e.ctx, e.admin.ID, e.sari.ID); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.Group(e.ctx, e.admin.ID, e.group.ID)
	if err != nil || got.LeaderID != nil {
		t.Errorf("after deleting the leader: %+v, %v", got, err)
	}
}

func TestLeaderAndGrantChangesAreLogged(t *testing.T) {
	e := newLeaderEnv(t)
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(obs.NewHandler(&buf, slog.LevelInfo)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	e.svc.SetLeader(e.ctx, e.admin.ID, e.group.ID, e.sari.ID)
	conn := testutil.CreateDatabaseConnection(t, e.db, "toko")
	if err := e.svc.Grant(e.ctx, e.admin.ID, e.group.ID, conn.ID, authz.View.String(), true); err != nil {
		t.Fatal(err)
	}

	events := map[string]map[string]any{}
	dec := json.NewDecoder(&buf)
	for dec.More() {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			t.Fatal(err)
		}
		events[line[obs.AttrEvent].(string)] = line
	}
	leader, grant := events[obs.EventLeaderChanged], events[obs.EventGrantChanged]
	if leader == nil || leader[obs.AttrActorID] != float64(e.admin.ID) || leader[obs.AttrGroupID] != float64(e.group.ID) || leader[obs.AttrLeader] != float64(e.sari.ID) {
		t.Errorf("leader.changed = %v", leader)
	}
	if grant == nil || grant[obs.AttrActorID] != float64(e.admin.ID) || grant[obs.AttrDatabaseID] != float64(conn.ID) {
		t.Errorf("grant.changed = %v", grant)
	}
}

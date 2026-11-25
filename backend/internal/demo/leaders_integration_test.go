//go:build integration

package demo

import (
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/approval"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func TestSeedGivesEveryGroupALeader(t *testing.T) {
	db := testutil.PlatformDB(t)
	cipher, err := dbconn.NewCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // the seed is idempotent
		if err := Seed(db, cipher); err != nil {
			t.Fatal(err)
		}
	}

	var conns []models.DatabaseConnection
	db.Find(&conns)
	if len(conns) == 0 {
		t.Fatal("no demo databases")
	}
	for _, c := range conns {
		approvers, blocking, err := approval.Approvers(t.Context(), db, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(blocking) != 0 {
			t.Errorf("%s: groups without a leader: %+v", c.Name, blocking)
		}
		names := map[string]bool{}
		for _, a := range approvers {
			names[a.Username] = true
		}
		if !names["developer"] || !names["reviewer"] || len(names) != 2 {
			t.Errorf("%s: approvers = %+v, want developer and reviewer", c.Name, approvers)
		}
	}
}

package dbconn

import (
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func openLazy(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", "postgres://nobody@127.0.0.1:1/none")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(5)
	return db
}

func TestStatsListsEveryOpenTargetPoolByName(t *testing.T) {
	p := NewPools(nil, nil, 0)
	p.entries[1], p.names[1] = openLazy(t), "toko"
	p.entries[2], p.names[2] = openLazy(t), "kampus"
	defer p.Close()

	got := map[string]int{}
	for _, s := range p.Stats() {
		if s.Pool != "target" {
			t.Errorf("pool label = %q, want target", s.Pool)
		}
		got[s.Database] = s.Stats.MaxOpenConnections
	}
	if len(got) != 2 || got["toko"] != 5 || got["kampus"] != 5 {
		t.Errorf("stats = %v, want toko and kampus with max 5", got)
	}
}

func TestStatsForgetsAnInvalidatedPool(t *testing.T) {
	p := NewPools(nil, nil, 0)
	p.entries[1], p.names[1] = openLazy(t), "toko"
	defer p.Close()

	p.Invalidate(1)

	if got := p.Stats(); len(got) != 0 {
		t.Errorf("stats after Invalidate = %+v, want none", got)
	}
}

func TestStatsIsEmptyAfterClose(t *testing.T) {
	p := NewPools(nil, nil, 0)
	p.entries[1], p.names[1] = openLazy(t), "toko"

	p.Close()

	if got := p.Stats(); len(got) != 0 {
		t.Errorf("stats after Close = %+v, want none", got)
	}
}

//go:build integration

package migration_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

type levels map[uint]authz.Level

func (l levels) Can(_ context.Context, userID, _ uint, a authz.Action) (bool, error) {
	need, ok := authz.RequiredLevel(a)
	return ok && l[userID] >= need, nil
}

type explainEnv struct {
	svc            *migration.Service
	dbID           uint
	viewer, editor uint
	count          func(string) int
}

func setupExplain(t *testing.T) explainEnv {
	t.Helper()
	platform := testutil.PlatformDB(t)
	target := testutil.TargetDB(t)
	if _, err := target.Exec(`CREATE TABLE items (id int PRIMARY KEY, qty int NOT NULL);
		INSERT INTO items SELECT g, g % 7 FROM generate_series(1, 500) g; ANALYZE items;`); err != nil {
		t.Fatal(err)
	}
	viewer := testutil.CreateUser(t, platform, "viewer")
	editor := testutil.CreateUser(t, platform, "editor")
	conn := testutil.CreateDatabaseConnection(t, platform, "toko")
	svc := migration.New(platform, testutil.StaticProvider{DB: target}, levels{viewer.ID: authz.View, editor.ID: authz.Edit},
		migration.Options{LockTimeout: time.Second, StatementTimeout: 5 * time.Second, ExecTimeout: 10 * time.Second})
	count := func(q string) int {
		var n int
		if err := target.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	return explainEnv{svc: svc, dbID: conn.ID, viewer: viewer.ID, editor: editor.ID, count: count}
}

func (e explainEnv) draft(t *testing.T, sql string) uint {
	t.Helper()
	m, err := e.svc.Create(context.Background(), e.editor, e.dbID, sql)
	if err != nil {
		t.Fatal(err)
	}
	return m.ID
}

const explainSQL = `CREATE TABLE extra (id int PRIMARY KEY);
INSERT INTO extra SELECT id FROM items;
SELECT * FROM items WHERE id = 5;`

func TestExplainPlansEveryStatementAndRollsBack(t *testing.T) {
	e := setupExplain(t)
	plans, err := e.svc.Explain(context.Background(), e.viewer, e.draft(t, explainSQL), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 {
		t.Fatalf("plans = %d", len(plans))
	}
	if plans[0].Explained || !strings.Contains(plans[0].Note, "tidak dapat dianalisis") {
		t.Fatalf("DDL statement: %+v", plans[0])
	}
	if !plans[1].Explained || plans[1].Nodes[0].NodeType != "ModifyTable" {
		t.Fatalf("insert into a table created earlier in the migration: %+v", plans[1])
	}
	scan := plans[2].Nodes[0]
	if scan.NodeType != "Index Scan" || scan.Index != "items_pkey" || scan.Relation != "items" || scan.TotalCost <= 0 || scan.PlanRows != 1 {
		t.Fatalf("select plan: %+v", scan)
	}
	if scan.ActualRows != nil || plans[2].ExecutionMs != nil {
		t.Fatal("plain EXPLAIN must not report actual figures")
	}
	if e.count(`SELECT count(*) FROM pg_tables WHERE tablename = 'extra'`) != 0 {
		t.Fatal("explain left the DDL applied")
	}
}

func TestExplainAnalyzeNeedsEditAndChangesNothing(t *testing.T) {
	e := setupExplain(t)
	id := e.draft(t, explainSQL+"\nDELETE FROM items WHERE qty = 0;")
	if _, err := e.svc.Explain(context.Background(), e.viewer, id, true); !errors.Is(err, apperr.ErrForbidden) {
		t.Fatalf("viewer analyze: %v", err)
	}
	plans, err := e.svc.Explain(context.Background(), e.editor, id, true)
	if err != nil {
		t.Fatal(err)
	}
	sel := plans[2]
	if sel.Nodes[0].ActualRows == nil || *sel.Nodes[0].ActualRows != 1 || sel.ExecutionMs == nil {
		t.Fatalf("analyze figures missing: %+v", sel)
	}
	if del := plans[3].Nodes; len(del) < 2 || del[1].NodeType != "Seq Scan" || *del[1].ActualRows == 0 {
		t.Fatalf("delete plan: %+v", del)
	}
	if e.count(`SELECT count(*) FROM items`) != 500 || e.count(`SELECT count(*) FROM pg_tables WHERE tablename = 'extra'`) != 0 {
		t.Fatal("explain analyze changed the database")
	}
}

func TestExplainRejectsTransactionControl(t *testing.T) {
	e := setupExplain(t)
	id := e.draft(t, "SELECT 1; COMMIT;")
	if _, err := e.svc.Explain(context.Background(), e.editor, id, false); !errors.Is(err, apperr.ErrTransactionControlNotAllowed) {
		t.Fatalf("got %v", err)
	}
}

func TestExplainStopsAtFailingStatement(t *testing.T) {
	e := setupExplain(t)
	plans, err := e.svc.Explain(context.Background(), e.editor, e.draft(t, "SELECT * FROM missing; SELECT 1;"), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || !strings.Contains(plans[0].Error, `relation "missing" does not exist`) || plans[1].Explained || !strings.Contains(plans[1].Note, "Dilewati") {
		t.Fatalf("%+v", plans)
	}
}

func TestExplainUnknownMigration(t *testing.T) {
	e := setupExplain(t)
	if _, err := e.svc.Explain(context.Background(), e.editor, 9999, false); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

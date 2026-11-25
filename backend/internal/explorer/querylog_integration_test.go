//go:build integration

package explorer

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func queryLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for dec.More() {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			t.Fatal(err)
		}
		if line[obs.AttrEvent] == obs.EventExplorerQuery {
			out = append(out, line)
		}
	}
	return out
}

func TestEveryExplorerQueryIsLoggedWithItsSQLAndDuration(t *testing.T) {
	platform := testutil.PlatformDB(t)
	target := testutil.TargetDB(t)
	if _, err := target.Exec(`CREATE TABLE products (id int PRIMARY KEY, name text); INSERT INTO products VALUES (1, 'a'), (2, 'b')`); err != nil {
		t.Fatal(err)
	}
	user := testutil.CreateUser(t, platform, "viewer")
	conn := testutil.CreateDatabaseConnection(t, platform, "toko")
	svc := New(platform, testutil.StaticProvider{DB: target}, allowAll{}, 5*time.Second)

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(obs.NewHandler(&buf, slog.LevelDebug)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	if _, err := svc.GetRows(context.Background(), user.ID, conn.ID, "public", "products", 1, 10); err != nil {
		t.Fatal(err)
	}
	lines := queryLines(t, &buf)
	if len(lines) < 2 {
		t.Fatalf("explorer.query lines = %d, want one per query (count and page)", len(lines))
	}
	var sawCount, sawPage bool
	for _, l := range lines {
		q, _ := l[obs.AttrQuery].(string)
		sawCount = sawCount || strings.Contains(q, "count(*)")
		if strings.Contains(q, "LIMIT") {
			sawPage = true
			if params, _ := l[obs.AttrParams].([]any); len(params) != 2 || params[0] != float64(10) || params[1] != float64(0) {
				t.Errorf("page query params = %v, want [10 0]", l[obs.AttrParams])
			}
		}
		if l[obs.AttrActorID] != float64(user.ID) || l[obs.AttrDatabaseID] != float64(conn.ID) ||
			l[obs.AttrAction] != string(authz.ViewData) || l[obs.AttrStatus] != "ok" || l[obs.AttrDurationMS] == nil {
			t.Errorf("line = %v", l)
		}
	}
	if !sawCount || !sawPage {
		t.Errorf("count query seen %v, page query seen %v", sawCount, sawPage)
	}
}

func TestAFailingExplorerQueryIsLoggedAsAnError(t *testing.T) {
	platform := testutil.PlatformDB(t)
	target := testutil.TargetDB(t)
	user := testutil.CreateUser(t, platform, "viewer")
	conn := testutil.CreateDatabaseConnection(t, platform, "toko")
	svc := New(platform, testutil.StaticProvider{DB: target}, allowAll{}, 5*time.Second)

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(obs.NewHandler(&buf, slog.LevelDebug)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	_ = svc.readOnly(context.Background(), user.ID, conn.ID, authz.ViewData, func(ctx context.Context, tx *queryTx) error {
		_, err := tx.QueryContext(ctx, `SELECT * FROM missing WHERE name = 'secret'`)
		return err
	})
	lines := queryLines(t, &buf)
	if len(lines) != 1 || lines[0][obs.AttrStatus] != "error" || lines[0][obs.AttrError] == nil {
		t.Fatalf("lines = %v, want one error line", lines)
	}
	if q, _ := lines[0][obs.AttrQuery].(string); strings.Contains(q, "secret") {
		t.Errorf("a literal reached the log: %q", q)
	}
}

//go:build integration

package explorer

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

type allowAll struct{}

func (allowAll) Can(context.Context, uint, uint, authz.Action) (bool, error) { return true, nil }

func TestReadOnlyRejectsWrites(t *testing.T) {
	platform := testutil.PlatformDB(t)
	target := testutil.TargetDB(t)
	if _, err := target.Exec(`CREATE TABLE products (id int)`); err != nil {
		t.Fatal(err)
	}
	user := testutil.CreateUser(t, platform, "viewer")
	conn := testutil.CreateDatabaseConnection(t, platform, "toko")
	svc := New(platform, testutil.StaticProvider{DB: target}, allowAll{}, 5*time.Second)

	err := svc.readOnly(context.Background(), user.ID, conn.ID, authz.ViewData, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO products VALUES (1)`)
		return err
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "25006" { // read_only_sql_transaction
		t.Fatalf("write in explorer transaction = %v, want SQLSTATE 25006", err)
	}

	var n int
	target.QueryRow(`SELECT count(*) FROM products`).Scan(&n)
	if n != 0 {
		t.Fatalf("products has %d rows, want 0", n)
	}
}

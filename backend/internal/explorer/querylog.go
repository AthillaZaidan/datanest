package explorer

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/obs"
)

// queryTx logs every query it runs as an explorer.query line.
type queryTx struct {
	*sql.Tx
	attrs []any
}

func (t *queryTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	started := time.Now()
	rows, err := t.Tx.QueryContext(ctx, query, args...)
	t.log(ctx, query, args, started, err)
	return rows, err
}

func (t *queryTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	started := time.Now()
	row := t.Tx.QueryRowContext(ctx, query, args...)
	t.log(ctx, query, args, started, row.Err())
	return row
}

func (t *queryTx) log(ctx context.Context, query string, args []any, started time.Time, err error) {
	attrs := append(append([]any{}, t.attrs...), obs.AttrQuery, query, obs.AttrDurationMS, time.Since(started).Milliseconds())
	if len(args) > 0 {
		attrs = append(attrs, obs.AttrParams, args)
	}
	if err != nil {
		slog.WarnContext(ctx, obs.EventExplorerQuery, append(attrs, obs.AttrStatus, "error", obs.AttrError, err.Error())...)
		return
	}
	slog.InfoContext(ctx, obs.EventExplorerQuery, append(attrs, obs.AttrStatus, "ok")...)
}

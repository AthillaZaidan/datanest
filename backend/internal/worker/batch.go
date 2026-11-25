package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

type span struct{ from, to string }

func tableIdent(name string) pgx.Identifier {
	if i := strings.Index(name, "."); i > 0 {
		return pgx.Identifier{name[:i], name[i+1:]}
	}
	return pgx.Identifier{"public", name}
}

func totals(entries []migration.LedgerEntry) (int, int64) {
	var rows int64
	for _, e := range entries {
		rows += e.RowCount
	}
	return len(entries), rows
}

func parseBound(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, permanent("LEDGER_RANGE_INVALID", "Rentang di ledger bukan bilangan bulat: %q", s)
	}
	return n, nil
}

func checkBatchColumn(ctx context.Context, db *sql.DB, table pgx.Identifier, column string) error {
	var dataType string
	var notNull, unique bool
	err := db.QueryRowContext(ctx, `
		SELECT format_type(a.atttypid, a.atttypmod), a.attnotnull,
			EXISTS (
				SELECT 1 FROM pg_index i
				WHERE i.indrelid = a.attrelid AND i.indisunique AND i.indnatts = 1
					AND i.indkey[0] = a.attnum AND i.indpred IS NULL
			)
		FROM pg_attribute a
		WHERE a.attrelid = to_regclass($1) AND a.attname = $2 AND a.attnum > 0 AND NOT a.attisdropped`,
		table.Sanitize(), column,
	).Scan(&dataType, &notNull, &unique)
	if errors.Is(err, sql.ErrNoRows) {
		return permanent("BATCH_COLUMN_NOT_FOUND", "Kolom %s tidak ditemukan di tabel %s.", column, strings.Join(table, "."))
	}
	if err != nil {
		return err
	}
	switch dataType {
	case "smallint", "integer", "bigint":
	default:
		return permanent("BATCH_COLUMN_TYPE", "Kolom batch harus bertipe bilangan bulat (smallint, integer, bigint), bukan %s.", dataType)
	}
	if !notNull || !unique {
		return permanent("BATCH_COLUMN_NOT_UNIQUE", "Kolom batch harus NOT NULL dan unik (primary key atau unique index tunggal).")
	}
	return nil
}

func (w *Worker) chain(ctx context.Context, id uint) ([]uint, error) {
	var ids []uint
	cur := &id
	for i := 0; cur != nil && i < 100; i++ {
		ids = append(ids, *cur)
		e, err := w.store.Get(ctx, *cur)
		if errors.Is(err, execution.ErrNotFound) {
			break
		}
		if err != nil {
			return nil, err
		}
		cur = e.ContinuesExecutionID
	}
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids, nil
}

func (w *Worker) chainSpans(ctx context.Context, db *sql.DB, id uint) ([]span, error) {
	ids, err := w.chain(ctx, id)
	if err != nil {
		return nil, err
	}
	var out []span
	for _, cid := range ids {
		entries, err := migration.LookupExecution(ctx, db, cid)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.RangeFrom != nil && entry.RangeTo != nil {
				out = append(out, span{*entry.RangeFrom, *entry.RangeTo})
			}
		}
	}
	return out, nil
}

func (w *Worker) batchScope(ctx context.Context, db *sql.DB, cl *Claimed, table, column string) (scope int64, ok bool, err error) {
	e := &cl.Exec
	if e.ScopeMax != nil {
		n, err := parseBound(*e.ScopeMax)
		return n, true, err
	}
	var estimated int64
	if e.ContinuesExecutionID != nil {
		parentIDs, err := w.chain(ctx, *e.ContinuesExecutionID)
		if err != nil {
			return 0, false, err
		}
		for i := len(parentIDs) - 1; i >= 0; i-- {
			parent, err := w.store.Get(ctx, parentIDs[i])
			if err != nil {
				return 0, false, err
			}
			if parent.ScopeMax != nil {
				n, err := parseBound(*parent.ScopeMax)
				if err != nil {
					return 0, false, err
				}
				if parent.EstimatedRows != nil {
					estimated = *parent.EstimatedRows
				}
				return n, true, w.store.SetScope(context.WithoutCancel(ctx), e.ID, cl.Owner, *parent.ScopeMax, estimated)
			}
		}
	}
	var max sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT max(`+column+`) FROM `+table).Scan(&max); err != nil {
		return 0, false, err
	}
	if !max.Valid {
		return 0, false, nil
	}
	if err := db.QueryRowContext(ctx, `SELECT GREATEST(reltuples, 0)::bigint FROM pg_class WHERE oid = to_regclass($1)`, table).Scan(&estimated); err != nil {
		return 0, false, err
	}
	if estimated == 0 {
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&estimated); err != nil {
			return 0, false, err
		}
	}
	return max.Int64, true, w.store.SetScope(context.WithoutCancel(ctx), e.ID, cl.Owner, strconv.FormatInt(max.Int64, 10), estimated)
}

func (w *Worker) startCursor(ctx context.Context, db *sql.DB, e *models.MigrationExecution, own []migration.LedgerEntry, table, column string) (cursor int64, ok bool, err error) {
	if len(own) > 0 {
		last := own[len(own)-1]
		if last.RangeTo == nil {
			return 0, false, permanent("LEDGER_RANGE_INVALID", "Baris ledger batch %d tidak punya rentang.", last.BatchNo)
		}
		n, err := parseBound(*last.RangeTo)
		return n, true, err
	}
	if e.ContinuesExecutionID != nil {
		ids, err := w.chain(ctx, *e.ContinuesExecutionID)
		if err != nil {
			return 0, false, err
		}
		for i := len(ids) - 1; i >= 0; i-- {
			entries, err := migration.LookupExecution(ctx, db, ids[i])
			if err != nil {
				return 0, false, err
			}
			if len(entries) > 0 && entries[len(entries)-1].RangeTo != nil {
				n, err := parseBound(*entries[len(entries)-1].RangeTo)
				return n, true, err
			}
		}
	}
	var min sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT min(`+column+`) FROM `+table).Scan(&min); err != nil {
		return 0, false, err
	}
	if !min.Valid {
		return 0, false, nil
	}
	if min.Int64 == math.MinInt64 {
		return 0, false, permanent("BATCH_COLUMN_RANGE", "Nilai terkecil kolom batch di luar jangkauan.")
	}
	return min.Int64 - 1, true, nil
}

func (w *Worker) runBatches(ctx context.Context, db *sql.DB, cl *Claimed, own []migration.LedgerEntry) (int, int64, error) {
	e := &cl.Exec
	wctx := context.WithoutCancel(ctx)
	batches, rows := totals(own)
	if e.BatchSize == nil || *e.BatchSize < 1 {
		return batches, rows, permanent("BATCH_SIZE_INVALID", "Ukuran batch harus lebih dari 0.")
	}
	size := *e.BatchSize
	ident := tableIdent(e.BatchTable)
	if e.BatchTable == "" || e.BatchColumn == "" {
		return batches, rows, permanent("BATCH_CONFIG_MISSING", "Mode batch butuh tabel dan kolom batch.")
	}
	if err := checkBatchColumn(ctx, db, ident, e.BatchColumn); err != nil {
		return batches, rows, err
	}
	table, column := ident.Sanitize(), pgx.Identifier{e.BatchColumn}.Sanitize()

	scope, hasScope, err := w.batchScope(ctx, db, cl, table, column)
	if err != nil {
		return batches, rows, err
	}
	cursor, hasCursor, err := w.startCursor(ctx, db, e, own, table, column)
	if err != nil {
		return batches, rows, err
	}
	if !hasScope || !hasCursor {
		return batches, rows, nil
	}

	nextUpper := fmt.Sprintf(`SELECT max(c) FROM (SELECT %[2]s AS c FROM %[1]s WHERE %[2]s > $1 AND %[2]s <= $2 ORDER BY %[2]s LIMIT $3) s`, table, column)
	for {
		if err := ctx.Err(); err != nil {
			return batches, rows, err
		}
		var upper sql.NullInt64
		if err := db.QueryRowContext(ctx, nextUpper, cursor, scope, size).Scan(&upper); err != nil {
			return batches, rows, err
		}
		if !upper.Valid {
			return batches, rows, nil
		}
		from, to := strconv.FormatInt(cursor, 10), strconv.FormatInt(upper.Int64, 10)
		affected, err := w.applyTx(ctx, db, cl, batches+1, &from, &to, e.SQLExecuted, cursor, upper.Int64)
		if err != nil {
			return batches, rows, err
		}
		batches++
		rows += affected
		cursor = upper.Int64
		w.store.SetProgress(wctx, e.ID, cl.Owner, batches, rows, to)
	}
}

func (w *Worker) runUndo(ctx context.Context, db *sql.DB, cl *Claimed, own []migration.LedgerEntry) (int, int64, error) {
	e := &cl.Exec
	wctx := context.WithoutCancel(ctx)
	batches, rows := totals(own)
	if e.RevertsExecutionID == nil {
		return batches, rows, permanent("ROLLBACK_TARGET_MISSING", "Rollback per batch butuh eksekusi yang dibatalkan.")
	}
	applied, err := w.chainSpans(ctx, db, *e.RevertsExecutionID)
	if err != nil {
		return batches, rows, err
	}

	undone := map[span]bool{}
	for _, entry := range own {
		if entry.RangeFrom != nil && entry.RangeTo != nil {
			undone[span{*entry.RangeFrom, *entry.RangeTo}] = true
		}
	}
	if e.ContinuesExecutionID != nil {
		previous, err := w.chainSpans(ctx, db, *e.ContinuesExecutionID)
		if err != nil {
			return batches, rows, err
		}
		for _, s := range previous {
			undone[s] = true
		}
	}

	for i := len(applied) - 1; i >= 0; i-- {
		s := applied[i]
		if undone[s] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return batches, rows, err
		}
		from, err := parseBound(s.from)
		if err != nil {
			return batches, rows, err
		}
		to, err := parseBound(s.to)
		if err != nil {
			return batches, rows, err
		}
		fromText, toText := s.from, s.to
		affected, err := w.applyTx(ctx, db, cl, batches+1, &fromText, &toText, e.SQLExecuted, from, to)
		if err != nil {
			return batches, rows, err
		}
		batches++
		rows += affected
		w.store.SetProgress(wctx, e.ID, cl.Owner, batches, rows, toText)
	}
	return batches, rows, nil
}

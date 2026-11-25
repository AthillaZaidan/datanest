package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/targeterr"
)

const MaxBatchSize = 1_000_000

type BatchColumnError struct {
	Code    string
	Message string
}

func (e *BatchColumnError) Error() string { return e.Message }

func BatchTable(name string) pgx.Identifier {
	if i := strings.Index(name, "."); i > 0 {
		return pgx.Identifier{name[:i], name[i+1:]}
	}
	return pgx.Identifier{"public", name}
}

func CheckBatchColumn(ctx context.Context, db *sql.DB, table, column string) error {
	ident := BatchTable(table)
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
		ident.Sanitize(), column,
	).Scan(&dataType, &notNull, &unique)
	if errors.Is(err, sql.ErrNoRows) {
		return &BatchColumnError{"BATCH_COLUMN_NOT_FOUND", fmt.Sprintf("Kolom %s tidak ditemukan di tabel %s.", column, strings.Join(ident, "."))}
	}
	if err != nil {
		return err
	}
	switch dataType {
	case "smallint", "integer", "bigint":
	default:
		return &BatchColumnError{"BATCH_COLUMN_TYPE", fmt.Sprintf("Kolom batch harus bertipe bilangan bulat (smallint, integer, bigint), bukan %s.", dataType)}
	}
	if !notNull || !unique {
		return &BatchColumnError{"BATCH_COLUMN_NOT_UNIQUE", "Kolom batch harus NOT NULL dan unik (primary key atau unique index tunggal)."}
	}
	return nil
}

func batched(m *models.Migration, kind models.ExecutionKind) bool {
	return m.Mode == models.ExecModeBatch && (kind == models.ExecKindRun || kind == models.ExecKindForward)
}

func checkBatchSize(m *models.Migration, kind models.ExecutionKind, size *int) error {
	if !batched(m, kind) {
		if size != nil {
			return apperr.ErrValidation.WithDetails(map[string]string{"batchSize": "hanya untuk run atau forward migration per-batch"})
		}
		return nil
	}
	if size == nil || *size < 1 || *size > MaxBatchSize {
		return apperr.ErrValidation.WithDetails(map[string]string{"batchSize": fmt.Sprintf("wajib diisi, 1 sampai %d", MaxBatchSize)})
	}
	return nil
}

func (s *Service) checkBatchTarget(ctx context.Context, m *models.Migration) error {
	db, err := s.targets.Get(ctx, m.DatabaseID)
	if err != nil {
		return targeterr.Map(err)
	}
	err = CheckBatchColumn(ctx, db, m.BatchTable, m.BatchColumn)
	var problem *BatchColumnError
	if errors.As(err, &problem) {
		return apperr.ErrValidation.WithMessage(problem.Message).WithDetails(map[string]string{"batchColumn": problem.Message})
	}
	return targeterr.Map(err)
}

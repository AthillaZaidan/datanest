package explorer

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
)

const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

type Rows struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Page      int      `json:"page"`
	PageSize  int      `json:"pageSize"`
	TotalRows int64    `json:"totalRows"`
}

// ClampPage corrects out-of-range values instead of rejecting them.
func ClampPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return page, pageSize
}

func (s *Service) GetRows(ctx context.Context, userID, databaseID uint, schema, table string, page, pageSize int) (*Rows, error) {
	page, pageSize = ClampPage(page, pageSize)
	out := &Rows{Page: page, PageSize: pageSize, Columns: []string{}, Rows: [][]any{}}

	err := s.readOnly(ctx, userID, databaseID, authz.ViewData, func(ctx context.Context, tx *sql.Tx) error {
		oid, err := tableOID(ctx, tx, schema, table)
		if err != nil {
			return err
		}
		// Safe: schema and table were just verified against the catalog.
		name := pgx.Identifier{schema, table}.Sanitize()

		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM `+name).Scan(&out.TotalRows); err != nil {
			return err
		}

		order, err := orderBy(ctx, tx, oid)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT * FROM `+name+` ORDER BY `+order+` LIMIT $1 OFFSET $2`,
			pageSize, (page-1)*pageSize)
		if err != nil {
			return err
		}
		defer rows.Close()

		types, err := rows.ColumnTypes()
		if err != nil {
			return err
		}
		for _, t := range types {
			out.Columns = append(out.Columns, t.Name())
		}
		for rows.Next() {
			vals := make([]any, len(types))
			ptrs := make([]any, len(types))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return err
			}
			for i, v := range vals {
				vals[i] = jsonValue(types[i].DatabaseTypeName(), v)
			}
			out.Rows = append(out.Rows, vals)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// orderBy gives a stable order: the primary key, or ctid for tables without one.
func orderBy(ctx context.Context, tx *sql.Tx, oid uint32) (string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT a.attname FROM pg_index i
		JOIN unnest(i.indkey::int2[]) WITH ORDINALITY k(attnum, ord) ON true
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
		WHERE i.indrelid = $1 AND i.indisprimary
		ORDER BY k.ord`, oid)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return "", err
		}
		cols = append(cols, pgx.Identifier{c}.Sanitize())
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(cols) == 0 {
		return "ctid", nil
	}
	return strings.Join(cols, ", "), nil
}

// jsonValue makes driver values JSON-friendly: bytea as \x hex, json as raw JSON, other bytes as text.
func jsonValue(dbType string, v any) any {
	b, ok := v.([]byte)
	if !ok {
		return v
	}
	switch dbType {
	case "BYTEA":
		return `\x` + hex.EncodeToString(b)
	case "JSON", "JSONB":
		if json.Valid(b) {
			return json.RawMessage(b)
		}
	}
	return string(b)
}

// ParseInt returns def when s is empty or not a number.
func ParseInt(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

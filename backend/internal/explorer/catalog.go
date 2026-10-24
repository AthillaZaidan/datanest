package explorer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
)

// hiddenTable is the platform's migration ledger in public; users never see it.
const hiddenTable = "_platform_migrations"

type Schema struct {
	Name string `json:"name"`
}

type Table struct {
	Name string `json:"name"`
}

type Column struct {
	Name     string  `json:"name"`
	DataType string  `json:"dataType"`
	Nullable bool    `json:"nullable"`
	Default  *string `json:"default"`
}

type ForeignKeyRef struct {
	Schema  string   `json:"schema"`
	Table   string   `json:"table"`
	Columns []string `json:"columns"`
}

type Constraint struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Columns    []string       `json:"columns"`
	Definition string         `json:"definition"`
	References *ForeignKeyRef `json:"references,omitempty"`
}

type Index struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	Unique     bool   `json:"unique"`
	Primary    bool   `json:"primary"`
}

type TableDetail struct {
	Schema      string       `json:"schema"`
	Name        string       `json:"name"`
	Columns     []Column     `json:"columns"`
	Constraints []Constraint `json:"constraints"`
	Indexes     []Index      `json:"indexes"`
}

const userSchemaFilter = `nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
	AND nspname NOT LIKE 'pg_temp_%' AND nspname NOT LIKE 'pg_toast_temp_%'`

var (
	errSchemaNotFound = apperr.ErrNotFound.WithMessage("Schema tidak ditemukan.")
	errTableNotFound  = apperr.ErrNotFound.WithMessage("Table tidak ditemukan.")
)

func (s *Service) ListSchemas(ctx context.Context, userID, databaseID uint) ([]Schema, error) {
	out := []Schema{}
	err := s.readOnly(ctx, userID, databaseID, authz.ViewSchema, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT nspname FROM pg_namespace WHERE `+userSchemaFilter+` ORDER BY nspname`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sc Schema
			if err := rows.Scan(&sc.Name); err != nil {
				return err
			}
			out = append(out, sc)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) ListTables(ctx context.Context, userID, databaseID uint, schema string) ([]Table, error) {
	out := []Table{}
	err := s.readOnly(ctx, userID, databaseID, authz.ViewSchema, func(ctx context.Context, tx *sql.Tx) error {
		var exists bool
		err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE `+userSchemaFilter+` AND nspname = $1)`, schema).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return errSchemaNotFound
		}

		rows, err := tx.QueryContext(ctx, `
			SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = $1 AND c.relkind IN ('r', 'p') AND NOT (n.nspname = 'public' AND c.relname = $2)
			ORDER BY c.relname`, schema, hiddenTable)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t Table
			if err := rows.Scan(&t.Name); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) GetTable(ctx context.Context, userID, databaseID uint, schema, table string) (*TableDetail, error) {
	detail := &TableDetail{Schema: schema, Name: table}
	err := s.readOnly(ctx, userID, databaseID, authz.ViewSchema, func(ctx context.Context, tx *sql.Tx) error {
		oid, err := tableOID(ctx, tx, schema, table)
		if err != nil {
			return err
		}
		if detail.Columns, err = columns(ctx, tx, oid); err != nil {
			return err
		}
		if detail.Constraints, err = constraints(ctx, tx, oid); err != nil {
			return err
		}
		detail.Indexes, err = indexes(ctx, tx, oid)
		return err
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

// tableOID resolves a user-visible table; unknown names are 404, never interpolated into SQL.
func tableOID(ctx context.Context, tx *sql.Tx, schema, table string) (uint32, error) {
	var oid uint32
	err := tx.QueryRowContext(ctx, `
		SELECT c.oid FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind IN ('r', 'p')
			AND `+userSchemaFilter+` AND NOT (n.nspname = 'public' AND c.relname = $3)`,
		schema, table, hiddenTable).Scan(&oid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errTableNotFound
	}
	return oid, err
}

func columns(ctx context.Context, tx *sql.Tx, oid uint32) ([]Column, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT a.attname, format_type(a.atttypid, a.atttypmod), NOT a.attnotnull, pg_get_expr(d.adbin, d.adrelid)
		FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`, oid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Column{}
	for rows.Next() {
		var c Column
		var def sql.NullString
		if err := rows.Scan(&c.Name, &c.DataType, &c.Nullable, &def); err != nil {
			return nil, err
		}
		if def.Valid {
			c.Default = &def.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

var constraintTypes = map[string]string{
	"p": "PRIMARY KEY",
	"f": "FOREIGN KEY",
	"u": "UNIQUE",
	"c": "CHECK",
	"x": "EXCLUDE",
	"n": "NOT NULL",
	"t": "TRIGGER",
}

func constraints(ctx context.Context, tx *sql.Tx, oid uint32) ([]Constraint, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT con.conname, con.contype, pg_get_constraintdef(con.oid, true),
			(SELECT COALESCE(json_agg(a.attname ORDER BY k.ord), '[]')
				FROM unnest(con.conkey) WITH ORDINALITY k(attnum, ord)
				JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.attnum),
			fn.nspname, fc.relname,
			(SELECT COALESCE(json_agg(a.attname ORDER BY k.ord), '[]')
				FROM unnest(con.confkey) WITH ORDINALITY k(attnum, ord)
				JOIN pg_attribute a ON a.attrelid = con.confrelid AND a.attnum = k.attnum)
		FROM pg_constraint con
		LEFT JOIN pg_class fc ON fc.oid = con.confrelid
		LEFT JOIN pg_namespace fn ON fn.oid = fc.relnamespace
		WHERE con.conrelid = $1
		ORDER BY con.contype, con.conname`, oid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Constraint{}
	for rows.Next() {
		var c Constraint
		var contype, cols, refCols string
		var refSchema, refTable sql.NullString
		if err := rows.Scan(&c.Name, &contype, &c.Definition, &cols, &refSchema, &refTable, &refCols); err != nil {
			return nil, err
		}
		c.Type = constraintTypes[contype]
		if c.Type == "" {
			c.Type = contype
		}
		if err := json.Unmarshal([]byte(cols), &c.Columns); err != nil {
			return nil, err
		}
		if refTable.Valid {
			ref := &ForeignKeyRef{Schema: refSchema.String, Table: refTable.String}
			if err := json.Unmarshal([]byte(refCols), &ref.Columns); err != nil {
				return nil, err
			}
			c.References = ref
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func indexes(ctx context.Context, tx *sql.Tx, oid uint32) ([]Index, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT ic.relname, pg_get_indexdef(i.indexrelid), i.indisunique, i.indisprimary
		FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid
		WHERE i.indrelid = $1
		ORDER BY ic.relname`, oid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Index{}
	for rows.Next() {
		var ix Index
		if err := rows.Scan(&ix.Name, &ix.Definition, &ix.Unique, &ix.Primary); err != nil {
			return nil, err
		}
		out = append(out, ix)
	}
	return out, rows.Err()
}

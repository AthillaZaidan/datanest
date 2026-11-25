package migration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/migration/sqlguard"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/targeterr"
)

// StatementPlan is the analysis of one statement of a migration.
type StatementPlan struct {
	SQL         string     `json:"sql"`
	Explained   bool       `json:"explained"`
	Note        string     `json:"note,omitempty"`
	Error       string     `json:"error,omitempty"`
	Nodes       []PlanNode `json:"nodes"`
	PlanningMs  *float64   `json:"planningMs"`
	ExecutionMs *float64   `json:"executionMs"`
}

// PlanNode is one node of a PostgreSQL plan, flattened in display order.
type PlanNode struct {
	Depth       int      `json:"depth"`
	NodeType    string   `json:"nodeType"`
	Operation   string   `json:"operation,omitempty"`
	Relation    string   `json:"relation,omitempty"`
	Index       string   `json:"index,omitempty"`
	JoinType    string   `json:"joinType,omitempty"`
	Strategy    string   `json:"strategy,omitempty"`
	StartupCost float64  `json:"startupCost"`
	TotalCost   float64  `json:"totalCost"`
	PlanRows    float64  `json:"planRows"`
	ActualRows  *float64 `json:"actualRows"`
	ActualMs    *float64 `json:"actualMs"`
	Loops       *float64 `json:"loops"`
}

// Statements PostgreSQL can EXPLAIN; everything else (DDL, utility commands) is only run.
var explainable = map[string]bool{"select": true, "insert": true, "update": true, "delete": true, "with": true, "values": true, "table": true, "merge": true}

const (
	noteNotExplainable = "Statement ini tidak dapat dianalisis dengan EXPLAIN (misalnya DDL). Statement dijalankan di dalam transaction agar statement berikutnya dapat dianalisis, lalu di-rollback."
	noteSkipped        = "Dilewati karena statement sebelumnya gagal."
)

// Explain plans every statement of a migration inside one transaction that is always rolled back.
// Statements that cannot be explained are executed so later statements see their effect.
// ANALYZE executes the statements too, so it needs the same access as running the migration.
func (s *Service) Explain(ctx context.Context, userID, migrationID uint, analyze bool) ([]StatementPlan, error) {
	action := authz.ViewMigrations
	if analyze {
		action = authz.ExecuteMigration
	}
	m, err := s.loadAuthorized(ctx, userID, migrationID, action)
	if err != nil {
		return nil, err
	}
	if m.Mode == models.ExecModeBatch {
		return nil, apperr.ErrValidation.WithMessage("Analisis execution plan tidak tersedia untuk migration per-batch.")
	}
	if err := sqlguard.Check(m.SQL); err != nil {
		return nil, err
	}
	db, err := s.targets.Get(ctx, m.DatabaseID)
	if err != nil {
		return nil, targeterr.Map(err)
	}
	ctx, cancel := context.WithTimeout(ctx, s.opts.ExecTimeout)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, targeterr.Map(err)
	}
	defer tx.Rollback()
	for _, q := range s.limits() {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return nil, targeterr.Map(err)
		}
	}

	prefix := "EXPLAIN (FORMAT JSON) "
	if analyze {
		prefix = "EXPLAIN (ANALYZE, FORMAT JSON) "
	}
	var plans []StatementPlan
	failed := false
	for _, stmt := range sqlguard.Split(m.SQL) {
		p := StatementPlan{SQL: stmt}
		switch {
		case failed:
			p.Note = noteSkipped
		case !explainable[sqlguard.FirstWord(stmt)]:
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				p.Error, failed = errorText(err), true
			} else {
				p.Note = noteNotExplainable
			}
		default:
			var raw []byte
			if err := tx.QueryRowContext(ctx, prefix+stmt).Scan(&raw); err != nil {
				p.Error, failed = errorText(err), true
			} else if err := p.parse(raw); err != nil {
				return nil, err
			}
		}
		plans = append(plans, p)
	}
	return plans, nil
}

func errorText(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return "ERROR: " + pg.Message + " (SQLSTATE " + pg.Code + ")"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Analisis melewati batas waktu."
	}
	return "Statement tidak dapat dianalisis."
}

type rawNode struct {
	NodeType    string    `json:"Node Type"`
	Operation   string    `json:"Operation"`
	Relation    string    `json:"Relation Name"`
	Index       string    `json:"Index Name"`
	JoinType    string    `json:"Join Type"`
	Strategy    string    `json:"Strategy"`
	StartupCost float64   `json:"Startup Cost"`
	TotalCost   float64   `json:"Total Cost"`
	PlanRows    float64   `json:"Plan Rows"`
	ActualRows  *float64  `json:"Actual Rows"`
	ActualMs    *float64  `json:"Actual Total Time"`
	Loops       *float64  `json:"Actual Loops"`
	Plans       []rawNode `json:"Plans"`
}

func (p *StatementPlan) parse(raw []byte) error {
	var out []struct {
		Plan        rawNode  `json:"Plan"`
		PlanningMs  *float64 `json:"Planning Time"`
		ExecutionMs *float64 `json:"Execution Time"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out) == 0 {
		return errors.New("migration: unexpected EXPLAIN output")
	}
	p.Explained, p.PlanningMs, p.ExecutionMs = true, out[0].PlanningMs, out[0].ExecutionMs
	var walk func(n rawNode, depth int)
	walk = func(n rawNode, depth int) {
		p.Nodes = append(p.Nodes, PlanNode{Depth: depth, NodeType: n.NodeType, Operation: n.Operation, Relation: n.Relation, Index: n.Index,
			JoinType: n.JoinType, Strategy: n.Strategy, StartupCost: n.StartupCost, TotalCost: n.TotalCost, PlanRows: n.PlanRows,
			ActualRows: n.ActualRows, ActualMs: n.ActualMs, Loops: n.Loops})
		for _, c := range n.Plans {
			walk(c, depth+1)
		}
	}
	walk(out[0].Plan, 0)
	return nil
}

func (s *Service) limits() []string {
	out := []string{"SET LOCAL standard_conforming_strings = on"}
	if s.opts.LockTimeout > 0 {
		out = append(out, fmt.Sprintf("SET LOCAL lock_timeout = %d", s.opts.LockTimeout.Milliseconds()))
	}
	if s.opts.StatementTimeout > 0 {
		out = append(out, fmt.Sprintf("SET LOCAL statement_timeout = %d", s.opts.StatementTimeout.Milliseconds()))
	}
	return out
}

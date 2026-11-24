//go:build integration

package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func TestGetExecutionReturnsStatusProgressAndAttempts(t *testing.T) {
	platform := testutil.PlatformDB(t)
	viewer := testutil.CreateUser(t, platform, "viewer")
	outsider := testutil.CreateUser(t, platform, "outsider")
	conn := testutil.CreateDatabaseConnection(t, platform, "toko")
	size := 10
	scope, cursor, estimated := "95", "40", int64(95)
	started := time.Now()
	x := testutil.CreateExecution(t, platform, models.MigrationExecution{
		DatabaseID: conn.ID, Status: models.ExecPartial, Mode: models.ExecModeBatch, BatchSize: &size,
		SQLExecuted: "UPDATE items SET qty = qty + 1 WHERE id > $1 AND id <= $2", CommittedBatches: 4, CommittedRows: 40,
		LastCursor: &cursor, ScopeMax: &scope, EstimatedRows: &estimated, ErrorCode: "22012", ErrorMessage: "division by zero",
		RequestedByUsername: "developer", StartedAt: &started,
	})
	platform.Create(&models.ExecutionAttempt{ExecutionID: x.ID, Number: 1, Status: models.AttemptFailed, StartedAt: started, ErrorMessage: "division by zero"})
	svc := execution.NewService(platform, fakeAuthz{viewer.ID: authz.View, outsider.ID: authz.None})
	router := func(userID uint) http.Handler {
		return NewRouter(Deps{RequireAuth: testutil.AsUser(userID), Executions: svc})
	}

	var got struct {
		ID       uint   `json:"id"`
		Status   string `json:"status"`
		Mode     string `json:"mode"`
		Progress *struct {
			CommittedBatches int     `json:"committedBatches"`
			CommittedRows    int64   `json:"committedRows"`
			LastCursor       *string `json:"lastCursor"`
			ScopeMax         *string `json:"scopeMax"`
			EstimatedRows    *int64  `json:"estimatedRows"`
		} `json:"progress"`
		Attempts []struct {
			Number       int    `json:"number"`
			Status       string `json:"status"`
			ErrorMessage string `json:"errorMessage"`
		} `json:"attempts"`
		ErrorCode   string        `json:"errorCode"`
		SQLExecuted *string       `json:"sqlExecuted"`
		RequestedBy string        `json:"requestedBy"`
		BatchSize   *int          `json:"batchSize"`
		Approvers   []interface{} `json:"approvers"`
	}
	if code := getJSON(t, router(viewer.ID), "/api/executions/"+itoa(x.ID), &got); code != 200 {
		t.Fatalf("status = %d", code)
	}
	if got.ID != x.ID || got.Status != "partial" || got.Mode != "batch" || got.ErrorCode != "22012" || got.RequestedBy != "developer" ||
		got.BatchSize == nil || *got.BatchSize != 10 || got.SQLExecuted == nil || got.Approvers == nil || len(got.Approvers) != 0 {
		t.Errorf("body = %+v", got)
	}
	if got.Progress == nil || got.Progress.CommittedBatches != 4 || got.Progress.CommittedRows != 40 ||
		*got.Progress.LastCursor != "40" || *got.Progress.ScopeMax != "95" || *got.Progress.EstimatedRows != 95 {
		t.Errorf("progress = %+v", got.Progress)
	}
	if len(got.Attempts) != 1 || got.Attempts[0].Status != "failed" || got.Attempts[0].ErrorMessage != "division by zero" {
		t.Errorf("attempts = %+v", got.Attempts)
	}

	if code, ec := errorCode(t, router(outsider.ID), "/api/executions/"+itoa(x.ID)); code != 403 || ec != "FORBIDDEN" {
		t.Errorf("outsider = %d %s, want 403 FORBIDDEN", code, ec)
	}
	if code, ec := errorCode(t, router(viewer.ID), "/api/executions/987654"); code != 404 || ec != "NOT_FOUND" {
		t.Errorf("missing = %d %s, want 404 NOT_FOUND", code, ec)
	}
	if code, _ := errorCode(t, router(viewer.ID), "/api/executions/abc"); code != 400 && code != 404 {
		t.Errorf("malformed id = %d", code)
	}
}

func TestANormalModeExecutionHasNoProgressBlock(t *testing.T) {
	platform := testutil.PlatformDB(t)
	viewer := testutil.CreateUser(t, platform, "viewer")
	conn := testutil.CreateDatabaseConnection(t, platform, "toko")
	x := testutil.CreateExecution(t, platform, models.MigrationExecution{DatabaseID: conn.ID})
	svc := execution.NewService(platform, fakeAuthz{viewer.ID: authz.View})

	var got struct {
		Progress *struct{} `json:"progress"`
		Attempts []any     `json:"attempts"`
	}
	h := NewRouter(Deps{RequireAuth: testutil.AsUser(viewer.ID), Executions: svc})
	if code := getJSON(t, h, "/api/executions/"+itoa(x.ID), &got); code != 200 {
		t.Fatalf("status = %d", code)
	}
	if got.Progress != nil || got.Attempts == nil {
		t.Errorf("progress = %v, attempts = %v; want no progress and an empty attempts list", got.Progress, got.Attempts)
	}
}

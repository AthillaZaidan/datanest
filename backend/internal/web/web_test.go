package web

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func v(n int) *int { return &n }

func TestBuildRailMarksCurrentAndFailed(t *testing.T) {
	r := buildRail(&migration.ListResult{CurrentVersion: 2, Migrations: []migration.View{
		{ID: 3, Version: v(3), Status: models.MigrationFailed},
		{ID: 1, Version: v(1), Status: models.MigrationSuccess},
		{ID: 2, Version: v(2), Status: models.MigrationSuccess},
		{ID: 4, Status: models.MigrationDraft},
	}})
	if len(r.Steps) != 3 || r.Drafts != 1 || r.Current != 2 {
		t.Fatalf("%+v", r)
	}
	if r.Steps[0].Version != 1 || r.Steps[1].Label != "versi saat ini" || r.Steps[2].Label != "gagal" {
		t.Fatalf("order or marks wrong: %+v", r.Steps)
	}
}

func TestBuildRailCapsSteps(t *testing.T) {
	var ms []migration.View
	for i := 1; i <= railSteps+4; i++ {
		ms = append(ms, migration.View{ID: uint(i), Version: v(i), Status: models.MigrationSuccess})
	}
	r := buildRail(&migration.ListResult{CurrentVersion: railSteps + 4, Migrations: ms})
	if len(r.Steps) != railSteps || r.Hidden != 4 || r.Steps[0].Version != 5 {
		t.Fatalf("hidden %d, first %d", r.Hidden, r.Steps[0].Version)
	}
}

func TestFindingReportsLatestRun(t *testing.T) {
	done := time.Now()
	if got := finding(nil); got != "Belum ada migration yang dijalankan." {
		t.Fatal(got)
	}
	got := finding([]migration.View{
		{Version: v(1), Status: models.MigrationSuccess, ExecutedBy: "ayu", FinishedAt: &done},
		{Version: v(2), Status: models.MigrationFailed, ErrorMessage: "ERROR: relation \"x\" does not exist\nLINE 1", FinishedAt: &done},
		{Status: models.MigrationDraft},
	})
	if !strings.HasPrefix(got, `v2 gagal baru saja: ERROR: relation "x" does not exist.`) || !strings.HasSuffix(got, "1 draft menunggu dijalankan.") {
		t.Fatal(got)
	}
}

func TestWithQueryEscapesAndAppends(t *testing.T) {
	if got := withQuery("/admin/users", "err", "a b&c"); got != "/admin/users?err=a+b%26c" {
		t.Fatal(got)
	}
	if got := withQuery("/admin/groups?group=1", "err", "x"); got != "/admin/groups?group=1&err=x" {
		t.Fatal(got)
	}
}

func TestCellAndNumbers(t *testing.T) {
	if cell(json.RawMessage(`{"a":1}`)) != `{"a":1}` || cell(int64(7)) != "7" || cell("teks") != "teks" {
		t.Fatal("cell formatting")
	}
	if !isNumber(int64(1)) || !isNumber(2.5) || isNumber("3") || isNumber(true) {
		t.Fatal("number detection")
	}
}

func TestFormatTime(t *testing.T) {
	ts := time.Date(2026, 10, 25, 9, 5, 0, 0, time.Local)
	if got := formatTime(ts); got != "25 Okt 2026, 09.05" {
		t.Fatal(got)
	}
	var none *time.Time
	if formatTime(none) != "-" || duration(&ts, nil) != "-" {
		t.Fatal("missing times must print a dash")
	}
	later := ts.Add(1500 * time.Millisecond)
	if duration(&ts, &later) != "1.5 s" {
		t.Fatal(duration(&ts, &later))
	}
}

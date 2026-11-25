//go:build integration

package execution

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/dblock"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func setup(t *testing.T) (*Store, *gorm.DB, models.DatabaseConnection) {
	t.Helper()
	db := testutil.PlatformDB(t)
	return NewStore(db), db, testutil.CreateDatabaseConnection(t, db, "toko")
}

func newExec(t *testing.T, db *gorm.DB, databaseID uint, e models.MigrationExecution) models.MigrationExecution {
	t.Helper()
	e.DatabaseID = databaseID
	return testutil.CreateExecution(t, db, e)
}

func holdLock(t *testing.T, s *Store, databaseID uint, owner string) {
	t.Helper()
	err := s.db.Exec(`
		INSERT INTO database_locks (database_id, execution_id, owner, token, lease_until)
		VALUES (?, 0, ?, 1, now() + interval '1 minute')
		ON CONFLICT (database_id) DO UPDATE SET owner = EXCLUDED.owner, lease_until = EXCLUDED.lease_until`,
		databaseID, owner).Error
	if err != nil {
		t.Fatal(err)
	}
}

func claimAs(t *testing.T, s *Store, id uint, from models.ExecutionStatus, owner string) (*models.MigrationExecution, error) {
	t.Helper()
	e, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	holdLock(t, s, e.DatabaseID, owner)
	return s.Claim(context.Background(), id, from, owner)
}

func statusOf(t *testing.T, s *Store, id uint) models.ExecutionStatus {
	t.Helper()
	e, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return e.Status
}

func TestClaimHasOneWinnerAndCountsTheAttempt(t *testing.T) {
	s, db, conn := setup(t)
	e := newExec(t, db, conn.ID, models.MigrationExecution{})

	first, err := claimAs(t, s, e.ID, models.ExecQueued, "w1")
	if err != nil || first == nil {
		t.Fatalf("first claim = %v, %v", first, err)
	}
	if first.Status != models.ExecRunning || first.WorkerID != "w1" || first.Attempts != 1 || first.StartedAt == nil {
		t.Errorf("claimed row = %+v", first)
	}
	if second, err := claimAs(t, s, e.ID, models.ExecQueued, "w2"); err != nil || second != nil {
		t.Fatalf("second claim = %v, %v; want nothing", second, err)
	}
}

func runSeqOf(t *testing.T, s *Store, id uint) int64 {
	t.Helper()
	e, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if e.RunSeq == nil {
		t.Fatalf("execution %d has no run_seq", id)
	}
	return *e.RunSeq
}

func TestClaimNumbersRunsPerDatabaseInClaimOrder(t *testing.T) {
	s, db, conn := setup(t)
	other := testutil.CreateDatabaseConnection(t, db, "gudang")
	first := newExec(t, db, conn.ID, models.MigrationExecution{})
	second := newExec(t, db, conn.ID, models.MigrationExecution{})
	elsewhere := newExec(t, db, other.ID, models.MigrationExecution{})

	if got, _ := claimAs(t, s, second.ID, models.ExecQueued, "w"); got == nil || got.RunSeq == nil || *got.RunSeq != 1 {
		t.Fatalf("first claim = %+v", got)
	}
	claimAs(t, s, first.ID, models.ExecQueued, "w")
	claimAs(t, s, elsewhere.ID, models.ExecQueued, "w")

	if got := runSeqOf(t, s, second.ID); got != 1 {
		t.Errorf("run_seq of the first claimed = %d, want 1", got)
	}
	if got := runSeqOf(t, s, first.ID); got != 2 {
		t.Errorf("run_seq of the second claimed = %d, want 2", got)
	}
	if got := runSeqOf(t, s, elsewhere.ID); got != 1 {
		t.Errorf("run_seq on another database = %d, want 1", got)
	}
}

func TestClaimOfARequeuedExecutionMovesItAfterTheOneThatRanBetween(t *testing.T) {
	s, db, conn := setup(t)
	ctx := context.Background()
	retried := newExec(t, db, conn.ID, models.MigrationExecution{})
	between := newExec(t, db, conn.ID, models.MigrationExecution{})

	claimAs(t, s, retried.ID, models.ExecQueued, "w")
	if ok, err := s.Requeue(ctx, retried.ID, "w", 0, "", ""); err != nil || !ok {
		t.Fatalf("requeue = %v, %v", ok, err)
	}
	claimAs(t, s, between.ID, models.ExecQueued, "w")
	if got, _ := claimAs(t, s, retried.ID, models.ExecQueued, "w"); got == nil {
		t.Fatal("reclaim failed")
	}

	if before, after := runSeqOf(t, s, between.ID), runSeqOf(t, s, retried.ID); after <= before {
		t.Errorf("retried run_seq %d is not after the one that ran in between (%d)", after, before)
	}
}

func TestClaimNeedsALiveLockOfItsOwner(t *testing.T) {
	s, db, conn := setup(t)
	e := newExec(t, db, conn.ID, models.MigrationExecution{})
	ctx := context.Background()

	refused := func(step string) {
		t.Helper()
		if got, err := s.Claim(ctx, e.ID, models.ExecQueued, "w1"); err != nil || got != nil {
			t.Fatalf("%s: claim = %v, %v; want nothing", step, got, err)
		}
	}
	refused("no lock")
	holdLock(t, s, conn.ID, "w2")
	refused("the lock of another owner")
	other := testutil.CreateDatabaseConnection(t, db, "gudang")
	holdLock(t, s, other.ID, "w1")
	refused("a lock on another database")
	holdLock(t, s, conn.ID, "w1")
	db.Exec(`UPDATE database_locks SET lease_until = now() - interval '1 second' WHERE database_id = ?`, conn.ID)
	refused("an expired lease")

	if got, err := claimAs(t, s, e.ID, models.ExecQueued, "w1"); err != nil || got == nil {
		t.Fatalf("claim with the lock = %v, %v", got, err)
	}
}

func TestClaimClearsTheReasonOfTheLastWait(t *testing.T) {
	s, db, conn := setup(t)
	e := newExec(t, db, conn.ID, models.MigrationExecution{ErrorCode: "HELD", ErrorMessage: "Ditahan"})

	got, err := claimAs(t, s, e.ID, models.ExecQueued, "w")
	if err != nil || got == nil {
		t.Fatalf("claim = %v, %v", got, err)
	}
	if got.ErrorCode != "" || got.ErrorMessage != "" {
		t.Errorf("a running execution still shows %q %q", got.ErrorCode, got.ErrorMessage)
	}
}

func TestHoldDelaysOnlyAQueuedExecution(t *testing.T) {
	s, db, conn := setup(t)
	ctx := context.Background()
	now := time.Now()
	queued := newExec(t, db, conn.ID, models.MigrationExecution{PublishedAt: &now})
	if err := s.Hold(ctx, queued.ID, time.Minute, "Ditahan oleh E9"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, queued.ID)
	if got.Status != models.ExecQueued || got.Attempts != 0 || got.ErrorCode != "HELD" || got.ErrorMessage != "Ditahan oleh E9" || got.PublishedAt != nil {
		t.Errorf("row = %+v", got)
	}
	var wait float64
	db.Raw(`SELECT extract(epoch FROM not_before - now()) FROM migration_executions WHERE id = ?`, queued.ID).Scan(&wait)
	if wait < 50 || wait > 60 {
		t.Errorf("held for %.1f seconds, want about 60", wait)
	}

	for _, status := range []models.ExecutionStatus{models.ExecUncertain, models.ExecRunning, models.ExecSuccess, models.ExecPartial} {
		other := newExec(t, db, conn.ID, models.MigrationExecution{Status: status})
		if err := s.Hold(ctx, other.ID, time.Minute, "Ditahan"); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.Get(ctx, other.ID); got.ErrorCode != "" || got.NotBefore != nil {
			t.Errorf("a %s execution was held: %+v", status, got)
		}
	}
}

func TestBeginSQLCountsStartsForTheOwnerOfARunningClaim(t *testing.T) {
	s, db, conn := setup(t)
	ctx := context.Background()
	e := newExec(t, db, conn.ID, models.MigrationExecution{})
	if _, ok, err := s.BeginSQL(ctx, e.ID, ""); err != nil || ok {
		t.Fatalf("a queued execution started SQL: %v, %v", ok, err)
	}
	claimAs(t, s, e.ID, models.ExecQueued, "owner")
	if _, ok, err := s.BeginSQL(ctx, e.ID, "zombie"); err != nil || ok {
		t.Fatalf("a stranger started SQL: %v, %v", ok, err)
	}
	for want := 1; want <= 2; want++ {
		if runs, ok, err := s.BeginSQL(ctx, e.ID, "owner"); err != nil || !ok || runs != want {
			t.Fatalf("start %d = %d, %v, %v", want, runs, ok, err)
		}
	}
	s.Finish(ctx, e.ID, "owner", models.ExecSuccess, "", "", 1, 1)
	if _, ok, err := s.BeginSQL(ctx, e.ID, "owner"); err != nil || ok {
		t.Fatalf("a finished execution started SQL: %v, %v", ok, err)
	}
	if got, _ := s.Get(ctx, e.ID); got.SQLRuns != 2 {
		t.Errorf("sql_runs = %d, want 2", got.SQLRuns)
	}
}

func TestOwnsIsTrueOnlyForTheOwnerOfARunningClaim(t *testing.T) {
	s, db, conn := setup(t)
	ctx := context.Background()
	e := newExec(t, db, conn.ID, models.MigrationExecution{})
	claimAs(t, s, e.ID, models.ExecQueued, "owner")
	check := func(step, owner string, want bool) {
		t.Helper()
		if got, err := s.Owns(ctx, e.ID, owner); err != nil || got != want {
			t.Errorf("%s: owns = %v, %v, want %v", step, got, err, want)
		}
	}
	check("owner", "owner", true)
	check("stranger", "zombie", false)
	s.MarkUncertain(ctx, e.ID, "owner", 0, "", "")
	check("no longer running", "owner", false)
}

func TestClaimRespectsNotBefore(t *testing.T) {
	s, db, conn := setup(t)
	e := newExec(t, db, conn.ID, models.MigrationExecution{})
	ctx := context.Background()

	if err := s.Postpone(ctx, e.ID, time.Minute); err != nil {
		t.Fatal(err)
	}
	if got, _ := claimAs(t, s, e.ID, models.ExecQueued, "w1"); got != nil {
		t.Fatal("claimed an execution that is postponed")
	}
	db.Exec(`UPDATE migration_executions SET not_before = now() - interval '1 second' WHERE id = ?`, e.ID)
	if got, _ := claimAs(t, s, e.ID, models.ExecQueued, "w1"); got == nil {
		t.Fatal("could not claim after not_before passed")
	}
}

func TestClaimOnlyFromTheExpectedStatus(t *testing.T) {
	s, db, conn := setup(t)
	done := newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecSuccess})
	if got, _ := claimAs(t, s, done.ID, models.ExecQueued, "w1"); got != nil {
		t.Fatal("claimed a finished execution")
	}
	uncertain := newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecUncertain})
	if got, _ := claimAs(t, s, uncertain.ID, models.ExecQueued, "w1"); got != nil {
		t.Fatal("a queued claim took an uncertain execution")
	}
	if got, _ := claimAs(t, s, uncertain.ID, models.ExecUncertain, "w1"); got == nil {
		t.Fatal("recovery claim of an uncertain execution failed")
	}
}

func TestWritesFromAWorkerThatNoLongerOwnsTheExecutionAreIgnored(t *testing.T) {
	s, db, conn := setup(t)
	e := newExec(t, db, conn.ID, models.MigrationExecution{})
	ctx := context.Background()
	claimAs(t, s, e.ID, models.ExecQueued, "owner")

	if ok, _ := s.Finish(ctx, e.ID, "zombie", models.ExecSuccess, "", "", 1, 1); ok {
		t.Error("finish by a stranger was accepted")
	}
	if ok, _ := s.SetProgress(ctx, e.ID, "zombie", 5, 5, "5"); ok {
		t.Error("progress by a stranger was accepted")
	}
	if ok, _ := s.Requeue(ctx, e.ID, "zombie", 0, "", ""); ok {
		t.Error("requeue by a stranger was accepted")
	}
	if ok, _ := s.MarkUncertain(ctx, e.ID, "zombie", 0, "", ""); ok {
		t.Error("uncertain by a stranger was accepted")
	}
	if ok, _ := s.Finish(ctx, e.ID, "owner", models.ExecSuccess, "", "", 1, 1); !ok {
		t.Error("finish by the owner was rejected")
	}
	if got := statusOf(t, s, e.ID); got != models.ExecSuccess {
		t.Errorf("status = %s", got)
	}
	if ok, _ := s.Finish(ctx, e.ID, "owner", models.ExecFailed, "", "", 0, 0); ok {
		t.Error("a finished execution was finished again")
	}
}

func TestScopeIsSetOnlyOnce(t *testing.T) {
	s, db, conn := setup(t)
	e := newExec(t, db, conn.ID, models.MigrationExecution{Mode: models.ExecModeBatch})
	ctx := context.Background()
	claimAs(t, s, e.ID, models.ExecQueued, "w")

	s.SetScope(ctx, e.ID, "w", "100", 100)
	s.SetScope(ctx, e.ID, "w", "999", 999)
	got, _ := s.Get(ctx, e.ID)
	if got.ScopeMax == nil || *got.ScopeMax != "100" || *got.EstimatedRows != 100 {
		t.Errorf("scope = %v / %v, want 100 / 100", got.ScopeMax, got.EstimatedRows)
	}
}

func TestAttemptsAreRecordedPerNumber(t *testing.T) {
	s, db, conn := setup(t)
	e := newExec(t, db, conn.ID, models.MigrationExecution{})
	ctx := context.Background()
	claimed, _ := claimAs(t, s, e.ID, models.ExecQueued, "w")

	if err := s.StartAttempt(ctx, claimed, "w"); err != nil {
		t.Fatal(err)
	}
	if err := s.StartAttempt(ctx, claimed, "w"); err == nil {
		t.Error("the same attempt number was recorded twice")
	}
	s.FinishAttempt(ctx, e.ID, claimed.Attempts, models.AttemptFailed, "40P01", "deadlock")
	list, _ := s.Attempts(ctx, e.ID)
	if len(list) != 1 || list[0].Status != models.AttemptFailed || list[0].ErrorCode != "40P01" || list[0].FinishedAt == nil || list[0].SQLExecuted != "SELECT 1" {
		t.Errorf("attempts = %+v", list)
	}
}

func blockerIDs(t *testing.T, s *Store, e *models.MigrationExecution) map[uint]bool {
	t.Helper()
	list, err := s.Blockers(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	out := map[uint]bool{}
	for _, b := range list {
		out[b.ID] = true
	}
	return out
}

func TestBlockers(t *testing.T) {
	s, db, conn := setup(t)
	other := testutil.CreateDatabaseConnection(t, db, "kampus")

	subject := newExec(t, db, conn.ID, models.MigrationExecution{})
	running := newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecRunning})
	uncertain := newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecUncertain})
	partial := newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecPartial})
	newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecSuccess})
	newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecFailed})
	newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecQueued})
	newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecPendingReview})
	newExec(t, db, other.ID, models.MigrationExecution{Status: models.ExecRunning})

	got := blockerIDs(t, s, &subject)
	if len(got) != 3 || !got[running.ID] || !got[uncertain.ID] || !got[partial.ID] {
		t.Errorf("blockers = %v, want running, uncertain and partial of this database only", got)
	}
}

func TestAPartialExecutionDoesNotBlockItsOwnContinuation(t *testing.T) {
	s, db, conn := setup(t)
	partial := newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecPartial})
	continuation := newExec(t, db, conn.ID, models.MigrationExecution{Kind: models.ExecKindForward, ContinuesExecutionID: &partial.ID})

	if got := blockerIDs(t, s, &continuation); len(got) != 0 {
		t.Errorf("the continuation is blocked by %v", got)
	}
	stranger := newExec(t, db, conn.ID, models.MigrationExecution{})
	if got := blockerIDs(t, s, &stranger); !got[partial.ID] {
		t.Error("an unrelated execution must still be blocked by the partial one")
	}
}

func TestARollbackDoesNotBlockOnThePartialExecutionItReverts(t *testing.T) {
	s, db, conn := setup(t)
	partial := newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecPartial})
	rollback := newExec(t, db, conn.ID, models.MigrationExecution{Kind: models.ExecKindRollback, RevertsExecutionID: &partial.ID})

	if got := blockerIDs(t, s, &rollback); len(got) != 0 {
		t.Errorf("the rollback is blocked by %v", got)
	}
}

func TestAPartialExecutionStopsBlockingOnceSomethingSettledIt(t *testing.T) {
	cases := []struct {
		name         string
		settledBy    models.ExecutionStatus
		wantBlocking bool
	}{
		{"continuation succeeded", models.ExecSuccess, false},
		{"continuation left its own partial state", models.ExecPartial, false},
		{"continuation failed before committing anything", models.ExecFailed, true},
		{"continuation is only queued", models.ExecQueued, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, db, conn := setup(t)
			partial := newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecPartial})
			newExec(t, db, conn.ID, models.MigrationExecution{Kind: models.ExecKindForward, ContinuesExecutionID: &partial.ID, Status: c.settledBy})
			next := newExec(t, db, conn.ID, models.MigrationExecution{})

			if got := blockerIDs(t, s, &next)[partial.ID]; got != c.wantBlocking {
				t.Errorf("partial blocks = %v, want %v", got, c.wantBlocking)
			}
		})
	}
}

func TestAnExecutionWaitingForARetryHoldsOnlyFreshOnes(t *testing.T) {
	s, db, conn := setup(t)
	other := testutil.CreateDatabaseConnection(t, db, "kampus")
	retrying := newExec(t, db, conn.ID, models.MigrationExecution{Attempts: 1})
	second := newExec(t, db, conn.ID, models.MigrationExecution{Attempts: 2})
	fresh := newExec(t, db, conn.ID, models.MigrationExecution{})
	elsewhere := newExec(t, db, other.ID, models.MigrationExecution{})

	if got := blockerIDs(t, s, &fresh); len(got) != 2 || !got[retrying.ID] || !got[second.ID] {
		t.Errorf("a fresh execution is held by %v, want both retries", got)
	}
	if got := blockerIDs(t, s, &elsewhere); len(got) != 0 {
		t.Errorf("an execution on another database is held by %v", got)
	}
	for _, e := range []models.MigrationExecution{retrying, second} {
		if got := blockerIDs(t, s, &e); len(got) != 0 {
			t.Errorf("a retrying execution is held by %v", got)
		}
	}
}

func partialAt(t *testing.T, s *Store, databaseID uint, irreversible bool, runSeq int64, finished time.Time) models.MigrationExecution {
	t.Helper()
	x := newExec(t, s.db, databaseID, models.MigrationExecution{
		Status: models.ExecPartial, RunSeq: &runSeq, CreatedAt: finished.Add(-time.Hour), FinishedAt: &finished})
	if err := s.db.Model(&models.Migration{}).Where("id = ?", x.MigrationID).Update("irreversible", irreversible).Error; err != nil {
		t.Fatal(err)
	}
	return x
}

func TestAnIrreversiblePartialLetsARunCreatedAfterItThrough(t *testing.T) {
	finished := time.Now().Add(-time.Hour)
	before, after := finished.Add(-time.Minute), finished.Add(time.Minute)
	cases := []struct {
		name         string
		irreversible bool
		kind         models.ExecutionKind
		created      time.Time
		want         bool
	}{
		{"run created after", true, models.ExecKindRun, after, false},
		{"run created before", true, models.ExecKindRun, before, true},
		{"forward of another migration", true, models.ExecKindForward, after, true},
		{"rollback of another migration", true, models.ExecKindRollback, after, true},
		{"run after a reversible partial", false, models.ExecKindRun, after, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, db, conn := setup(t)
			partial := partialAt(t, s, conn.ID, c.irreversible, 1, finished)
			e := newExec(t, db, conn.ID, models.MigrationExecution{Kind: c.kind, CreatedAt: c.created})
			if got := blockerIDs(t, s, &e)[partial.ID]; got != c.want {
				t.Errorf("held = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAPartialWithoutAFinishTimeHoldsEverything(t *testing.T) {
	s, db, conn := setup(t)
	partial := partialAt(t, s, conn.ID, true, 1, time.Now())
	db.Exec(`UPDATE migration_executions SET finished_at = NULL WHERE id = ?`, partial.ID)
	e := newExec(t, db, conn.ID, models.MigrationExecution{})
	if !blockerIDs(t, s, &e)[partial.ID] {
		t.Error("a partial execution without a finish time held nothing")
	}
}

func TestALaterRunOfAnotherMigrationSettlesAPartial(t *testing.T) {
	finished := time.Now().Add(-time.Hour)
	later := finished.Add(time.Minute)
	cases := []struct {
		name   string
		status models.ExecutionStatus
		kind   models.ExecutionKind
		seq    int64
		want   bool
	}{
		{"run succeeded", models.ExecSuccess, models.ExecKindRun, 2, false},
		{"run ended partial", models.ExecPartial, models.ExecKindRun, 2, false},
		{"run failed", models.ExecFailed, models.ExecKindRun, 2, true},
		{"run is only queued", models.ExecQueued, models.ExecKindRun, 2, true},
		{"run before the partial", models.ExecSuccess, models.ExecKindRun, 0, true},
		{"forward succeeded", models.ExecSuccess, models.ExecKindForward, 2, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, db, conn := setup(t)
			partial := partialAt(t, s, conn.ID, true, 1, finished)
			corrective := newExec(t, db, conn.ID, models.MigrationExecution{
				Status: c.status, Kind: c.kind, RunSeq: &c.seq, CreatedAt: later, FinishedAt: &later})
			waiting := []models.MigrationExecution{
				newExec(t, db, conn.ID, models.MigrationExecution{CreatedAt: finished.Add(-time.Minute)}),
				newExec(t, db, conn.ID, models.MigrationExecution{Kind: models.ExecKindForward}),
				newExec(t, db, conn.ID, models.MigrationExecution{Kind: models.ExecKindRollback}),
			}
			for _, e := range waiting {
				if got := blockerIDs(t, s, &e)[partial.ID]; got != c.want {
					t.Errorf("%s created %s: held = %v, want %v", e.Kind, e.CreatedAt, got, c.want)
				}
			}
			if c.status == models.ExecPartial {
				next := newExec(t, db, conn.ID, models.MigrationExecution{Kind: models.ExecKindForward, ContinuesExecutionID: &corrective.ID})
				if got := blockerIDs(t, s, &next); len(got) != 0 {
					t.Errorf("the continuation of the corrective run is held by %v", got)
				}
			}
		})
	}
}

func TestARunOnAnotherDatabaseSettlesNothing(t *testing.T) {
	s, db, conn := setup(t)
	other := testutil.CreateDatabaseConnection(t, db, "kampus")
	partial := partialAt(t, s, conn.ID, true, 1, time.Now().Add(-time.Hour))
	seq := int64(2)
	newExec(t, db, other.ID, models.MigrationExecution{Status: models.ExecSuccess, RunSeq: &seq})
	e := newExec(t, db, conn.ID, models.MigrationExecution{Kind: models.ExecKindForward})
	if !blockerIDs(t, s, &e)[partial.ID] {
		t.Error("a run on another database settled the partial execution")
	}
}

func TestDueForPublish(t *testing.T) {
	s, db, conn := setup(t)
	ctx := context.Background()
	fresh := newExec(t, db, conn.ID, models.MigrationExecution{})
	recent := newExec(t, db, conn.ID, models.MigrationExecution{})
	stale := newExec(t, db, conn.ID, models.MigrationExecution{})
	later := newExec(t, db, conn.ID, models.MigrationExecution{})
	uncertain := newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecUncertain})
	newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecApproved})
	newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecRunning})
	db.Exec(`UPDATE migration_executions SET published_at = now() WHERE id = ?`, recent.ID)
	db.Exec(`UPDATE migration_executions SET published_at = now() - interval '2 minutes' WHERE id = ?`, stale.ID)
	db.Exec(`UPDATE migration_executions SET not_before = now() + interval '1 minute' WHERE id = ?`, later.ID)

	due, err := s.DueForPublish(ctx, 30*time.Second, 10)
	if err != nil {
		t.Fatal(err)
	}
	got := map[uint]bool{}
	for _, d := range due {
		got[d.ID] = true
	}
	if len(got) != 3 || !got[fresh.ID] || !got[stale.ID] || !got[uncertain.ID] {
		t.Errorf("due = %v, want never-published, stale and uncertain only", got)
	}
}

func TestDueForPublishSkipsAnExecutionWhoseLockIsAlive(t *testing.T) {
	s, db, conn := setup(t)
	e := newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecUncertain})
	locks := dblock.New(db)
	if _, ok, _ := locks.Acquire(context.Background(), conn.ID, e.ID, "someone", time.Minute); !ok {
		t.Fatal("setup lock failed")
	}
	if due, _ := s.DueForPublish(context.Background(), time.Second, 10); len(due) != 0 {
		t.Errorf("published an execution that a worker is still holding: %v", due)
	}
}

func TestMarkDeadRunningOnlyTouchesRunningExecutionsWithoutALiveLock(t *testing.T) {
	s, db, conn := setup(t)
	other := testutil.CreateDatabaseConnection(t, db, "kampus")
	ctx := context.Background()
	locks := dblock.New(db)

	alive := newExec(t, db, conn.ID, models.MigrationExecution{Status: models.ExecRunning})
	dead := newExec(t, db, other.ID, models.MigrationExecution{Status: models.ExecRunning})
	queued := newExec(t, db, conn.ID, models.MigrationExecution{})
	if _, ok, _ := locks.Acquire(ctx, conn.ID, alive.ID, "w1", time.Minute); !ok {
		t.Fatal("setup lock failed")
	}
	if _, ok, _ := locks.Acquire(ctx, other.ID, dead.ID, "w2", time.Minute); !ok {
		t.Fatal("setup lock failed")
	}
	db.Exec(`UPDATE database_locks SET lease_until = now() - interval '1 second' WHERE database_id = ?`, other.ID)

	ids, err := s.MarkDeadRunning(ctx)
	if err != nil || len(ids) != 1 || ids[0] != dead.ID {
		t.Fatalf("dead = %v, err %v; want only %d", ids, err, dead.ID)
	}
	if got := statusOf(t, s, dead.ID); got != models.ExecUncertain {
		t.Errorf("dead execution is %s, want uncertain", got)
	}
	if statusOf(t, s, alive.ID) != models.ExecRunning || statusOf(t, s, queued.ID) != models.ExecQueued {
		t.Error("a healthy execution was touched")
	}
	row, _ := s.Get(ctx, dead.ID)
	if row.ErrorCode != "WORKER_LOST" || row.PublishedAt != nil {
		t.Errorf("row = %+v", row)
	}
}

//go:build integration

package approval

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/queue"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

type fakePublisher struct {
	mu   sync.Mutex
	jobs []queue.Job
	err  error
}

func (p *fakePublisher) Publish(_ context.Context, j queue.Job) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.jobs = append(p.jobs, j)
	return nil
}

func (p *fakePublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.jobs)
}

// toko is granted to Tim A (leader sari, VIEW), Tim B (leader joko, VIEW) and Tim Dev (leader dev, EDIT); dev asked for one execution.
type env struct {
	db                        *gorm.DB
	svc                       *Service
	pub                       *fakePublisher
	dev, sari, joko, outsider models.User
	groupA, groupB, groupDev  models.Group
	conn                      models.DatabaseConnection
	exec                      models.MigrationExecution
}

func setup(t *testing.T) *env {
	t.Helper()
	db := testutil.PlatformDB(t)
	e := &env{db: db, pub: &fakePublisher{}}
	e.dev = testutil.CreateUser(t, db, "dev")
	e.sari = testutil.CreateUser(t, db, "sari")
	e.joko = testutil.CreateUser(t, db, "joko")
	e.outsider = testutil.CreateUser(t, db, "outsider")
	e.conn = testutil.CreateDatabaseConnection(t, db, "toko")

	e.groupA = testutil.CreateGroup(t, db, "Tim A")
	e.groupB = testutil.CreateGroup(t, db, "Tim B")
	e.groupDev = testutil.CreateGroup(t, db, "Tim Dev")
	testutil.AddMember(t, db, e.groupA.ID, e.sari.ID)
	testutil.AddMember(t, db, e.groupB.ID, e.joko.ID)
	testutil.AddMember(t, db, e.groupDev.ID, e.dev.ID)
	testutil.GrantAccess(t, db, e.groupA.ID, e.conn.ID, authz.View)
	testutil.GrantAccess(t, db, e.groupB.ID, e.conn.ID, authz.View)
	testutil.GrantAccess(t, db, e.groupDev.ID, e.conn.ID, authz.Edit)
	e.setLeader(t, e.groupA, &e.sari)
	e.setLeader(t, e.groupB, &e.joko)
	e.setLeader(t, e.groupDev, &e.dev)

	e.svc = NewService(db, authz.New(db))
	e.svc.Publisher = e.pub
	e.exec = e.newExecution(t)
	return e
}

func (e *env) setLeader(t *testing.T, g models.Group, u *models.User) {
	t.Helper()
	var id any
	if u != nil {
		id = u.ID
	}
	if err := e.db.Model(&models.Group{}).Where("id = ?", g.ID).Update("leader_id", id).Error; err != nil {
		t.Fatal(err)
	}
}

func (e *env) newExecution(t *testing.T) models.MigrationExecution {
	t.Helper()
	return testutil.CreateExecution(t, e.db, models.MigrationExecution{
		DatabaseID: e.conn.ID, Status: models.ExecPendingReview, RequestedByID: &e.dev.ID, RequestedByUsername: "dev",
	})
}

func (e *env) row(t *testing.T) models.MigrationExecution {
	t.Helper()
	var x models.MigrationExecution
	if err := e.db.First(&x, e.exec.ID).Error; err != nil {
		t.Fatal(err)
	}
	return x
}

func (e *env) status(t *testing.T) models.ExecutionStatus { return e.row(t).Status }

func (e *env) setStatus(t *testing.T, s models.ExecutionStatus) {
	t.Helper()
	if err := e.db.Model(&models.MigrationExecution{}).Where("id = ?", e.exec.ID).Update("status", s).Error; err != nil {
		t.Fatal(err)
	}
}

func (e *env) approvalCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	e.db.Model(&models.MigrationApproval{}).Where("execution_id = ?", e.exec.ID).Count(&n)
	return n
}

func code(err error) string {
	var a *apperr.Error
	if errors.As(err, &a) {
		return a.Code
	}
	if err == nil {
		return ""
	}
	return "other: " + err.Error()
}

func TestAllLeadersMustApproveThenTheExecutionIsQueuedOnce(t *testing.T) {
	e := setup(t)
	ctx := obs.WithCorrelationID(context.Background(), "corr-1")

	got, err := e.svc.Approve(ctx, e.sari.ID, e.exec.ID)
	if err != nil || got.Status != models.ExecPendingReview {
		t.Fatalf("first approval: %v, %+v", err, got)
	}
	if _, err := e.svc.Approve(ctx, e.joko.ID, e.exec.ID); err != nil || e.status(t) != models.ExecPendingReview {
		t.Fatalf("second approval: %v, status %s", err, e.status(t))
	}
	if e.pub.count() != 0 {
		t.Fatal("published before everyone approved")
	}
	got, err = e.svc.Approve(ctx, e.dev.ID, e.exec.ID)
	if err != nil || got.Status != models.ExecQueued {
		t.Fatalf("last approval: %v, %+v", err, got)
	}
	x := e.row(t)
	if x.Status != models.ExecQueued || x.QueuedAt == nil || x.CorrelationID != "corr-1" || x.PublishedAt == nil {
		t.Errorf("row = status %s queued_at %v corr %q published %v", x.Status, x.QueuedAt, x.CorrelationID, x.PublishedAt)
	}
	if e.pub.count() != 1 || e.pub.jobs[0].ExecutionID != e.exec.ID || e.pub.jobs[0].CorrelationID != "corr-1" {
		t.Errorf("jobs = %+v", e.pub.jobs)
	}
}

func TestViewLevelLeaderAndSelfApprovalAreAllowed(t *testing.T) {
	e := setup(t)
	if _, err := e.svc.Approve(context.Background(), e.sari.ID, e.exec.ID); err != nil {
		t.Fatalf("VIEW leader: %v", err)
	}
	if _, err := e.svc.Approve(context.Background(), e.dev.ID, e.exec.ID); err != nil {
		t.Fatalf("self approval: %v", err)
	}
}

func TestALeaderWithViewLevelCannotCreateOrExecute(t *testing.T) {
	e := setup(t)
	az := authz.New(e.db)
	for action, want := range map[authz.Action]bool{
		authz.ApproveMigration: true, authz.ViewMigrations: true,
		authz.CreateMigration: false, authz.UpdateMigration: false, authz.ExecuteMigration: false,
	} {
		got, err := az.Can(context.Background(), e.sari.ID, e.conn.ID, action)
		if err != nil || got != want {
			t.Errorf("leader with VIEW: Can(%s) = %v, %v; want %v", action, got, err, want)
		}
	}
}

func TestNonLeadersAreRefused(t *testing.T) {
	e := setup(t)
	e.setLeader(t, e.groupDev, &e.sari) // dev keeps EDIT but leads nothing
	ctx := context.Background()

	if _, err := e.svc.Approve(ctx, e.dev.ID, e.exec.ID); code(err) != "NOT_AN_APPROVER" {
		t.Errorf("EDIT non-leader = %s, want NOT_AN_APPROVER", code(err))
	}
	if _, err := e.svc.Approve(ctx, e.outsider.ID, e.exec.ID); code(err) != "FORBIDDEN" {
		t.Errorf("outsider = %s, want FORBIDDEN", code(err))
	}
	if _, err := e.svc.Reject(ctx, e.dev.ID, e.exec.ID, ""); code(err) != "NOT_AN_APPROVER" {
		t.Errorf("non-leader reject = %s", code(err))
	}
	if n := e.approvalCount(t); n != 0 || e.status(t) != models.ExecPendingReview {
		t.Errorf("refused calls left %d approvals, status %s", n, e.status(t))
	}
	if _, err := e.svc.Approve(ctx, e.sari.ID, 987654); code(err) != "NOT_FOUND" {
		t.Errorf("missing execution = %s", code(err))
	}
}

func TestGroupWithoutLeaderBlocksApproval(t *testing.T) {
	e := setup(t)
	e.setLeader(t, e.groupB, nil)
	_, err := e.svc.Approve(context.Background(), e.sari.ID, e.exec.ID)
	if code(err) != "GROUP_WITHOUT_LEADER" {
		t.Fatalf("err = %s, want GROUP_WITHOUT_LEADER", code(err))
	}
	var a *apperr.Error
	errors.As(err, &a)
	if !strings.Contains(a.Message, "Tim B") {
		t.Errorf("message does not name the group: %q", a.Message)
	}
	if e.approvalCount(t) != 0 {
		t.Error("an approval was recorded although the group blocks")
	}
	e.setLeader(t, e.groupB, &e.joko)
	if _, err := e.svc.Approve(context.Background(), e.sari.ID, e.exec.ID); err != nil {
		t.Fatalf("after the leader is back: %v", err)
	}
}

func TestApproverListIsLiveAndApprovalsStayWithThePerson(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.svc.Approve(ctx, e.sari.ID, e.exec.ID)

	tia := testutil.CreateUser(t, e.db, "tia")
	testutil.AddMember(t, e.db, e.groupA.ID, tia.ID)
	e.setLeader(t, e.groupA, &tia)
	e.svc.Approve(ctx, e.joko.ID, e.exec.ID)
	e.svc.Approve(ctx, e.dev.ID, e.exec.ID)
	if e.status(t) != models.ExecPendingReview {
		t.Fatalf("status = %s: sari's approval was counted for tia", e.status(t))
	}
	if _, err := e.svc.Approve(ctx, tia.ID, e.exec.ID); err != nil || e.status(t) != models.ExecQueued {
		t.Fatalf("tia approves: %v, status %s", err, e.status(t))
	}
}

func TestAPersonLeadingTwoGroupsCountsOnce(t *testing.T) {
	e := setup(t)
	e.setLeader(t, e.groupB, &e.sari) // sari leads A and B: the list is sari, dev
	ctx := context.Background()
	e.svc.Approve(ctx, e.sari.ID, e.exec.ID)
	if _, err := e.svc.Approve(ctx, e.dev.ID, e.exec.ID); err != nil || e.status(t) != models.ExecQueued {
		t.Fatalf("status = %s, err %v", e.status(t), err)
	}
}

func TestPromoteRechecksTheListAndFallsBack(t *testing.T) {
	e := setup(t)
	for _, u := range []models.User{e.sari, e.joko, e.dev} {
		e.db.Create(&models.MigrationApproval{ExecutionID: e.exec.ID, UserID: &u.ID, Username: u.Username})
	}
	e.setStatus(t, models.ExecApproved)

	late := testutil.CreateUser(t, e.db, "late")
	g := testutil.CreateGroup(t, e.db, "Tim Late")
	testutil.AddMember(t, e.db, g.ID, late.ID)
	testutil.GrantAccess(t, e.db, g.ID, e.conn.ID, authz.View)
	e.setLeader(t, g, &late)

	got, err := e.svc.Promote(context.Background(), e.exec.ID)
	if err != nil || got.Status != models.ExecPendingReview {
		t.Fatalf("promote = %+v, %v; want back to pending_review", got, err)
	}
	if e.pub.count() != 0 {
		t.Error("published although the list was incomplete")
	}
}

func TestConcurrentApprovalsQueueExactlyOnce(t *testing.T) {
	for round := 0; round < 5; round++ {
		e := setup(t)
		e.setLeader(t, e.groupDev, &e.sari) // approvers are now only sari and joko

		var wg sync.WaitGroup
		var failures atomic.Int32
		for _, u := range []models.User{e.sari, e.joko} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := e.svc.Approve(context.Background(), u.ID, e.exec.ID); err != nil {
					failures.Add(1)
				}
			}()
		}
		wg.Wait()
		if failures.Load() != 0 {
			t.Fatalf("round %d: %d approvals failed", round, failures.Load())
		}
		if e.status(t) != models.ExecQueued {
			t.Fatalf("round %d: status = %s, want queued", round, e.status(t))
		}
		if e.pub.count() != 1 {
			t.Fatalf("round %d: published %d times, want exactly 1", round, e.pub.count())
		}
	}
}

func TestRejectAndCancel(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	if _, err := e.svc.Cancel(ctx, e.sari.ID, e.exec.ID); code(err) != "FORBIDDEN" {
		t.Errorf("cancel by another person = %s, want FORBIDDEN", code(err))
	}
	got, err := e.svc.Reject(ctx, e.sari.ID, e.exec.ID, "terlalu berisiko")
	if err != nil || got.Status != models.ExecRejected {
		t.Fatalf("reject: %v, %+v", err, got)
	}
	x := e.row(t)
	if x.FinishedAt == nil || !strings.Contains(x.ErrorMessage, "terlalu berisiko") {
		t.Errorf("rejected row: finished %v message %q", x.FinishedAt, x.ErrorMessage)
	}
	if _, err := e.svc.Approve(ctx, e.joko.ID, e.exec.ID); code(err) != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("approve after reject = %s", code(err))
	}

	e.exec = e.newExecution(t)
	got, err = e.svc.Cancel(ctx, e.dev.ID, e.exec.ID)
	if err != nil || got.Status != models.ExecCanceled {
		t.Fatalf("cancel: %v, %+v", err, got)
	}
	if _, err := e.svc.Cancel(ctx, e.dev.ID, e.exec.ID); code(err) != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("second cancel = %s", code(err))
	}

	e.exec = e.newExecution(t)
	e.setStatus(t, models.ExecQueued)
	if _, err := e.svc.Reject(ctx, e.sari.ID, e.exec.ID, ""); code(err) != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("reject when queued = %s", code(err))
	}
	if _, err := e.svc.Cancel(ctx, e.dev.ID, e.exec.ID); code(err) != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("cancel when queued = %s", code(err))
	}
}

func TestWithdrawRevokesWithoutDeleting(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.svc.Withdraw(ctx, e.sari.ID, e.exec.ID); code(err) != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("withdraw without approving = %s", code(err))
	}
	e.svc.Approve(ctx, e.sari.ID, e.exec.ID)
	got, err := e.svc.Withdraw(ctx, e.sari.ID, e.exec.ID)
	if err != nil || got.Status != models.ExecPendingReview {
		t.Fatalf("withdraw: %v, %+v", err, got)
	}
	var rows []models.MigrationApproval
	e.db.Where("execution_id = ?", e.exec.ID).Find(&rows)
	if len(rows) != 1 || rows[0].RevokedAt == nil {
		t.Fatalf("approval rows = %+v, want one revoked row kept as history", rows)
	}
	if _, err := e.svc.Approve(ctx, e.sari.ID, e.exec.ID); err != nil {
		t.Fatalf("approve again: %v", err)
	}
	e.db.Where("execution_id = ?", e.exec.ID).Find(&rows)
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2", len(rows))
	}

	e.setStatus(t, models.ExecApproved)
	got, err = e.svc.Withdraw(ctx, e.sari.ID, e.exec.ID)
	if err != nil || got.Status != models.ExecPendingReview {
		t.Fatalf("withdraw from approved: %v, %+v", err, got)
	}

	e.svc.Approve(ctx, e.sari.ID, e.exec.ID)
	e.setStatus(t, models.ExecQueued)
	if _, err := e.svc.Withdraw(ctx, e.sari.ID, e.exec.ID); code(err) != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("withdraw when queued = %s", code(err))
	}
}

func TestApprovingTwiceIsAConflict(t *testing.T) {
	e := setup(t)
	e.svc.Approve(context.Background(), e.sari.ID, e.exec.ID)
	if _, err := e.svc.Approve(context.Background(), e.sari.ID, e.exec.ID); code(err) != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("second approval = %s", code(err))
	}
}

func TestPublishFailureLeavesTheExecutionQueuedForTheSweeper(t *testing.T) {
	e := setup(t)
	e.pub.err = errors.New("broker down")
	for _, u := range []models.User{e.sari, e.joko, e.dev} {
		if _, err := e.svc.Approve(context.Background(), u.ID, e.exec.ID); err != nil {
			t.Fatal(err)
		}
	}
	x := e.row(t)
	if x.Status != models.ExecQueued || x.PublishedAt != nil {
		t.Errorf("status %s published_at %v: want queued and unpublished so the sweeper picks it up", x.Status, x.PublishedAt)
	}
}

func TestPromoteStaleFinishesApprovedExecutions(t *testing.T) {
	e := setup(t)
	for _, u := range []models.User{e.sari, e.joko, e.dev} {
		e.db.Create(&models.MigrationApproval{ExecutionID: e.exec.ID, UserID: &u.ID, Username: u.Username})
	}
	e.db.Exec(`UPDATE migration_executions SET status = 'approved', updated_at = now() - interval '1 minute' WHERE id = ?`, e.exec.ID)
	n, err := e.svc.PromoteStale(context.Background(), 5*time.Second)
	if err != nil || n != 1 || e.status(t) != models.ExecQueued {
		t.Fatalf("PromoteStale = %d, %v; status %s", n, err, e.status(t))
	}
}

func TestPendingListsOnlyWhatTheLeaderMayReview(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	other := testutil.CreateDatabaseConnection(t, e.db, "kampus")
	testutil.CreateExecution(t, e.db, models.MigrationExecution{DatabaseID: other.ID, Status: models.ExecPendingReview, RequestedByUsername: "x"})
	testutil.CreateExecution(t, e.db, models.MigrationExecution{DatabaseID: e.conn.ID, Status: models.ExecSuccess, RequestedByUsername: "x"})

	items, err := e.svc.Pending(ctx, e.sari.ID)
	if err != nil || len(items) != 1 || items[0].ExecutionID != e.exec.ID {
		t.Fatalf("queue of sari = %+v, %v", items, err)
	}
	if items[0].DatabaseName != "toko" || items[0].RequestedBy != "dev" || items[0].Required != 3 || items[0].Approved != 0 || items[0].ApprovedByMe {
		t.Errorf("item = %+v", items[0])
	}
	e.svc.Approve(ctx, e.sari.ID, e.exec.ID)
	items, _ = e.svc.Pending(ctx, e.sari.ID)
	if len(items) != 1 || !items[0].ApprovedByMe || items[0].Approved != 1 {
		t.Errorf("after approving: %+v", items)
	}
	if items, _ := e.svc.Pending(ctx, e.outsider.ID); len(items) != 0 {
		t.Errorf("queue of outsider = %+v, want empty", items)
	}
}

func TestDescribeShowsApproversAndPermissions(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.svc.Approve(ctx, e.sari.ID, e.exec.ID)
	x := e.row(t)

	info, err := Describe(ctx, e.db, &x, e.joko.ID)
	if err != nil || len(info.Approvers) != 3 {
		t.Fatalf("describe: %+v, %v", info, err)
	}
	approved := map[string]bool{}
	for _, a := range info.Approvers {
		approved[a.Username] = a.Approved
	}
	if !approved["sari"] || approved["joko"] || approved["dev"] {
		t.Errorf("approved = %v", approved)
	}
	if p := info.Permissions; !p.CanApprove || !p.CanReject || p.CanCancel || p.CanWithdraw {
		t.Errorf("joko permissions = %+v", p)
	}
	info, _ = Describe(ctx, e.db, &x, e.sari.ID)
	if p := info.Permissions; p.CanApprove || !p.CanWithdraw {
		t.Errorf("sari permissions = %+v", p)
	}
	info, _ = Describe(ctx, e.db, &x, e.dev.ID)
	if !info.Permissions.CanCancel {
		t.Errorf("requester cannot cancel: %+v", info.Permissions)
	}
	info, _ = Describe(ctx, e.db, &x, e.outsider.ID)
	if info.Permissions != (Permissions{}) {
		t.Errorf("outsider permissions = %+v", info.Permissions)
	}

	e.setStatus(t, models.ExecQueued)
	x = e.row(t)
	info, _ = Describe(ctx, e.db, &x, e.joko.ID)
	if len(info.Approvers) != 1 || info.Approvers[0].Username != "sari" || info.Permissions != (Permissions{}) {
		t.Errorf("after queueing: %+v", info)
	}
}

func TestBlockedGroupsAreReported(t *testing.T) {
	e := setup(t)
	e.setLeader(t, e.groupB, nil)
	x := e.row(t)
	info, err := Describe(context.Background(), e.db, &x, e.sari.ID)
	if err != nil || len(info.BlockedGroups) != 1 || info.BlockedGroups[0].Name != "Tim B" || info.Permissions.CanApprove {
		t.Errorf("describe = %+v, %v", info, err)
	}
}

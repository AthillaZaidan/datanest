//go:build integration

package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/approval"
	"github.com/AthillaZaidan/datanest/backend/internal/auth"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

type approvalEnv struct {
	db                        *gorm.DB
	admin, dev, sari, outside models.User
	group                     models.Group
	exec                      models.MigrationExecution
	approvals                 *approval.Service
	executions                *execution.Service
	adminSvc                  *admin.Service
}

func newApprovalEnv(t *testing.T) approvalEnv {
	t.Helper()
	db := testutil.PlatformDB(t)
	e := approvalEnv{db: db}
	e.admin = testutil.CreateAdmin(t, db, "admin")
	e.dev = testutil.CreateUser(t, db, "dev")
	e.sari = testutil.CreateUser(t, db, "sari")
	e.outside = testutil.CreateUser(t, db, "outside")
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	e.group = testutil.CreateGroup(t, db, "Tim A")
	testutil.AddMember(t, db, e.group.ID, e.dev.ID)
	testutil.AddMember(t, db, e.group.ID, e.sari.ID)
	testutil.GrantAccess(t, db, e.group.ID, conn.ID, authz.Edit)
	e.exec = testutil.CreateExecution(t, db, models.MigrationExecution{
		DatabaseID: conn.ID, Status: models.ExecPendingReview, RequestedByID: &e.dev.ID, RequestedByUsername: "dev",
	})
	az := authz.New(db)
	e.approvals = approval.NewService(db, az)
	e.executions = execution.NewService(db, az)
	e.adminSvc = admin.New(db, fakeSessions{})
	return e
}

func (e approvalEnv) as(u models.User) http.Handler {
	as := &auth.Service{DB: e.db}
	return NewRouter(Deps{
		RequireAuth: testutil.AsUser(u.ID), RequireAdmin: as.RequireAdmin,
		Admin: e.adminSvc, Executions: e.executions, Approvals: e.approvals,
	})
}

func (e approvalEnv) path(action string) string {
	return "/api/executions/" + itoa(e.exec.ID) + "/" + action
}

func (e approvalEnv) setLeader(t *testing.T, u models.User) {
	t.Helper()
	r := call(e.as(e.admin), "PUT", "/api/admin/groups/"+itoa(e.group.ID)+"/leader", map[string]uint{"userId": u.ID})
	if r.Code != 200 {
		t.Fatalf("set leader = %d %s", r.Code, r.Body)
	}
}

func TestAdminSetsAndClearsTheLeaderOverHTTP(t *testing.T) {
	e := newApprovalEnv(t)
	base := "/api/admin/groups/" + itoa(e.group.ID) + "/leader"

	if r := call(e.as(e.dev), "PUT", base, map[string]uint{"userId": e.sari.ID}); r.Code != 403 {
		t.Errorf("non-admin PUT = %d, want 403", r.Code)
	}
	if r := call(e.as(e.admin), "PUT", base, map[string]uint{"userId": e.outside.ID}); r.Code != 400 {
		t.Errorf("non-member PUT = %d, want 400", r.Code)
	}
	if r := call(e.as(e.admin), "PUT", base, map[string]uint{}); r.Code != 400 {
		t.Errorf("missing userId = %d, want 400", r.Code)
	}
	r := call(e.as(e.admin), "PUT", base, map[string]uint{"userId": e.sari.ID})
	var g struct {
		LeaderID       *uint  `json:"leaderId"`
		LeaderUsername string `json:"leaderUsername"`
	}
	json.Unmarshal(r.Body, &g)
	if r.Code != 200 || g.LeaderID == nil || *g.LeaderID != e.sari.ID || g.LeaderUsername != "sari" {
		t.Fatalf("PUT = %d %s", r.Code, r.Body)
	}
	if r := call(e.as(e.admin), "DELETE", base, nil); r.Code != 200 {
		t.Errorf("DELETE = %d %s", r.Code, r.Body)
	}
	if r := call(e.as(e.dev), "DELETE", base, nil); r.Code != 403 {
		t.Errorf("non-admin DELETE = %d, want 403", r.Code)
	}
}

func TestApproveOverHTTPQueuesAndShowsPermissions(t *testing.T) {
	e := newApprovalEnv(t)
	e.setLeader(t, e.sari)

	var before execution.View
	if r := call(e.as(e.sari), "GET", "/api/executions/"+itoa(e.exec.ID), nil); r.Code != 200 {
		t.Fatalf("GET = %d %s", r.Code, r.Body)
	} else {
		json.Unmarshal(r.Body, &before)
	}
	if !before.Permissions.CanApprove || !before.Permissions.CanReject || before.Permissions.CanCancel || len(before.Approvers) != 1 {
		t.Errorf("sari sees %+v approvers %+v", before.Permissions, before.Approvers)
	}

	r := call(e.as(e.sari), "POST", e.path("approve"), nil)
	var v execution.View
	json.Unmarshal(r.Body, &v)
	if r.Code != 200 || v.Status != "queued" || len(v.Approvers) != 1 || !v.Approvers[0].Approved {
		t.Fatalf("approve = %d %s", r.Code, r.Body)
	}
	if r := call(e.as(e.sari), "POST", e.path("approve"), nil); r.Code != 409 || r.errCode() != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("second approve = %d %s", r.Code, r.errCode())
	}
}

func TestApprovalErrorsOverHTTP(t *testing.T) {
	e := newApprovalEnv(t)

	if r := call(e.as(e.sari), "POST", e.path("approve"), nil); r.Code != 403 || r.errCode() != "NOT_AN_APPROVER" {
		t.Errorf("approve without any leader = %d %s, want 403 NOT_AN_APPROVER", r.Code, r.errCode())
	}
	var v execution.View
	json.Unmarshal(call(e.as(e.sari), "GET", "/api/executions/"+itoa(e.exec.ID), nil).Body, &v)
	if len(v.BlockedGroups) != 1 || v.BlockedGroups[0].Name != "Tim A" || v.Permissions.CanApprove {
		t.Errorf("blocked = %+v permissions = %+v", v.BlockedGroups, v.Permissions)
	}

	other := testutil.CreateGroup(t, e.db, "Tim B")
	testutil.AddMember(t, e.db, other.ID, e.outside.ID)
	testutil.GrantAccess(t, e.db, other.ID, e.exec.DatabaseID, authz.View)
	e.db.Model(&models.Group{}).Where("id = ?", other.ID).Update("leader_id", e.outside.ID)
	if r := call(e.as(e.outside), "POST", e.path("approve"), nil); r.Code != 409 || r.errCode() != "GROUP_WITHOUT_LEADER" {
		t.Errorf("approve while Tim A has no leader = %d %s, want 409 GROUP_WITHOUT_LEADER", r.Code, r.errCode())
	}
	e.db.Model(&models.Group{}).Where("id = ?", other.ID).Update("leader_id", nil)
	e.db.Exec(`DELETE FROM grants WHERE group_id = ?`, other.ID)
	e.setLeader(t, e.sari)
	if r := call(e.as(e.dev), "POST", e.path("approve"), nil); r.Code != 403 || r.errCode() != "NOT_AN_APPROVER" {
		t.Errorf("EDIT non-leader = %d %s, want 403 NOT_AN_APPROVER", r.Code, r.errCode())
	}
	if r := call(e.as(e.outside), "POST", e.path("approve"), nil); r.Code != 403 || r.errCode() != "FORBIDDEN" {
		t.Errorf("outsider = %d %s, want 403 FORBIDDEN", r.Code, r.errCode())
	}
	if r := call(e.as(e.sari), "POST", "/api/executions/987654/approve", nil); r.Code != 404 {
		t.Errorf("missing execution = %d, want 404", r.Code)
	}
}

func TestRejectCancelAndWithdrawOverHTTP(t *testing.T) {
	e := newApprovalEnv(t)
	e.setLeader(t, e.sari)

	if r := call(e.as(e.sari), "POST", e.path("withdraw"), nil); r.Code != 409 {
		t.Errorf("withdraw before approving = %d, want 409", r.Code)
	}
	if r := call(e.as(e.sari), "POST", e.path("reject"), map[string]string{"reason": "belum siap"}); r.Code != 200 || statusOf(r) != "rejected" {
		t.Errorf("reject = %d %s", r.Code, r.Body)
	}

	e.exec = testutil.CreateExecution(t, e.db, models.MigrationExecution{
		DatabaseID: e.exec.DatabaseID, Status: models.ExecPendingReview, RequestedByID: &e.dev.ID, RequestedByUsername: "dev",
	})
	if r := call(e.as(e.sari), "POST", e.path("cancel"), nil); r.Code != 403 {
		t.Errorf("cancel by a leader who is not the requester = %d, want 403", r.Code)
	}
	if r := call(e.as(e.dev), "POST", e.path("cancel"), nil); r.Code != 200 || statusOf(r) != "canceled" {
		t.Errorf("cancel = %d %s", r.Code, r.Body)
	}

	e.exec = testutil.CreateExecution(t, e.db, models.MigrationExecution{
		DatabaseID: e.exec.DatabaseID, Status: models.ExecPendingReview, RequestedByUsername: "dev",
	})
	if r := call(e.as(e.sari), "POST", e.path("reject"), nil); r.Code != 200 || statusOf(r) != "rejected" {
		t.Errorf("reject without body = %d %s", r.Code, r.Body)
	}
}

func statusOf(r resp) string {
	var v struct {
		Status string `json:"status"`
	}
	json.Unmarshal(r.Body, &v)
	return v.Status
}

func TestPendingQueueOverHTTP(t *testing.T) {
	e := newApprovalEnv(t)
	e.setLeader(t, e.sari)

	var items []approval.PendingItem
	r := call(e.as(e.sari), "GET", "/api/approvals/pending", nil)
	json.Unmarshal(r.Body, &items)
	if r.Code != 200 || len(items) != 1 || items[0].ExecutionID != e.exec.ID || items[0].Required != 1 {
		t.Fatalf("pending = %d %s", r.Code, r.Body)
	}
	r = call(e.as(e.outside), "GET", "/api/approvals/pending", nil)
	if r.Code != 200 || string(r.Body) == "" || len(r.Body) < 2 || string(r.Body[:2]) != "[]" {
		t.Errorf("outsider queue = %d %s, want an empty list", r.Code, r.Body)
	}
}

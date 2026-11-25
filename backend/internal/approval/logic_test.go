package approval

import (
	"errors"
	"strings"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

var twoLeaders = []Person{{UserID: 1, Username: "sari"}, {UserID: 2, Username: "joko"}}

func TestCompleteNeedsEveryLiveApproverAndNoBlockingGroup(t *testing.T) {
	cases := []struct {
		name      string
		approvers []Person
		blocking  []GroupRef
		approved  map[uint]bool
		want      bool
	}{
		{"all approved", twoLeaders, nil, map[uint]bool{1: true, 2: true}, true},
		{"one missing", twoLeaders, nil, map[uint]bool{1: true}, false},
		{"nobody approved", twoLeaders, nil, map[uint]bool{}, false},
		{"approval of someone who left the list is ignored", twoLeaders, nil, map[uint]bool{1: true, 3: true}, false},
		{"extra approval from outside the list does no harm", twoLeaders, nil, map[uint]bool{1: true, 2: true, 3: true}, true},
		{"a group without a leader blocks even when all approved", twoLeaders, []GroupRef{{ID: 9, Name: "Tim X"}}, map[uint]bool{1: true, 2: true}, false},
		{"no approvers at all is never complete", nil, nil, map[uint]bool{}, false},
	}
	for _, c := range cases {
		if got := complete(c.approvers, c.blocking, c.approved); got != c.want {
			t.Errorf("%s: complete = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsApprover(t *testing.T) {
	if !isApprover(twoLeaders, 2) || isApprover(twoLeaders, 3) || isApprover(nil, 1) {
		t.Error("isApprover does not match the list")
	}
}

func TestReviewIsOpenOnlyBeforeQueueing(t *testing.T) {
	want := map[models.ExecutionStatus]bool{models.ExecPendingReview: true, models.ExecApproved: true}
	for _, s := range []models.ExecutionStatus{
		models.ExecPendingReview, models.ExecApproved, models.ExecQueued, models.ExecRunning, models.ExecSuccess,
		models.ExecFailed, models.ExecPartial, models.ExecUncertain, models.ExecRejected, models.ExecCanceled,
	} {
		if open(s) != want[s] {
			t.Errorf("open(%s) = %v", s, open(s))
		}
	}
}

func TestGroupsWithoutLeaderErrorNamesThem(t *testing.T) {
	err := groupsWithoutLeader([]GroupRef{{ID: 1, Name: "Tim A"}, {ID: 2, Name: "Tim B"}})
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != "GROUP_WITHOUT_LEADER" || e.Status != 409 {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(e.Message, "Tim A, Tim B") {
		t.Errorf("message = %q", e.Message)
	}
}

// Package approval implements the review step of an execution: approver list, approvals, reject, cancel and the move to queued.
package approval

import (
	"context"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

type Person struct {
	UserID   uint   `json:"userId"`
	Username string `json:"username"`
}

type GroupRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// Approvers is computed live from group leaders. Groups with a grant but no leader are returned as blocking.
func Approvers(ctx context.Context, db *gorm.DB, databaseID uint) (approvers []Person, blocking []GroupRef, err error) {
	approvers = []Person{}
	blocking = []GroupRef{}
	err = db.WithContext(ctx).Raw(`
		SELECT DISTINCT u.id AS user_id, u.username
		FROM groups g
		JOIN grants gr ON gr.group_id = g.id
		JOIN users u ON u.id = g.leader_id
		WHERE gr.database_id = ?
		ORDER BY u.id`, databaseID).Scan(&approvers).Error
	if err != nil {
		return nil, nil, err
	}
	err = db.WithContext(ctx).Raw(`
		SELECT g.id, g.name
		FROM groups g
		JOIN grants gr ON gr.group_id = g.id
		WHERE gr.database_id = ? AND g.leader_id IS NULL
		ORDER BY g.id`, databaseID).Scan(&blocking).Error
	if err != nil {
		return nil, nil, err
	}
	return approvers, blocking, nil
}

func liveApprovals(ctx context.Context, db *gorm.DB, executionID uint) (map[uint]bool, error) {
	var ids []uint
	err := db.WithContext(ctx).Model(&models.MigrationApproval{}).
		Where("execution_id = ? AND revoked_at IS NULL AND user_id IS NOT NULL", executionID).
		Pluck("user_id", &ids).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// complete ignores approvals of people who are no longer in the list.
func complete(approvers []Person, blocking []GroupRef, approved map[uint]bool) bool {
	if len(blocking) > 0 || len(approvers) == 0 {
		return false
	}
	for _, p := range approvers {
		if !approved[p.UserID] {
			return false
		}
	}
	return true
}

func isApprover(approvers []Person, userID uint) bool {
	for _, p := range approvers {
		if p.UserID == userID {
			return true
		}
	}
	return false
}

func open(s models.ExecutionStatus) bool {
	return s == models.ExecPendingReview || s == models.ExecApproved
}

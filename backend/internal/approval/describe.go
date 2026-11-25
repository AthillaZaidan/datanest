package approval

import (
	"context"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

type ApproverState struct {
	UserID   uint   `json:"userId"`
	Username string `json:"username"`
	Approved bool   `json:"approved"`
}

// Permissions only shape the UI; every action is checked again by its service.
type Permissions struct {
	CanApprove  bool `json:"canApprove"`
	CanReject   bool `json:"canReject"`
	CanCancel   bool `json:"canCancel"`
	CanWithdraw bool `json:"canWithdraw"`
}

type Info struct {
	Approvers     []ApproverState
	BlockedGroups []GroupRef
	Permissions   Permissions
}

func Describe(ctx context.Context, db *gorm.DB, e *models.MigrationExecution, viewerID uint) (Info, error) {
	info := Info{Approvers: []ApproverState{}, BlockedGroups: []GroupRef{}}
	approved, err := liveApprovals(ctx, db, e.ID)
	if err != nil {
		return info, err
	}
	if !open(e.Status) {
		var rows []models.MigrationApproval
		err := db.WithContext(ctx).Where("execution_id = ? AND revoked_at IS NULL", e.ID).Order("id").Find(&rows).Error
		if err != nil {
			return info, err
		}
		for _, r := range rows {
			st := ApproverState{Username: r.Username, Approved: true}
			if r.UserID != nil {
				st.UserID = *r.UserID
			}
			info.Approvers = append(info.Approvers, st)
		}
		return info, nil
	}

	approvers, blocking, err := Approvers(ctx, db, e.DatabaseID)
	if err != nil {
		return info, err
	}
	info.BlockedGroups = blocking
	for _, p := range approvers {
		info.Approvers = append(info.Approvers, ApproverState{UserID: p.UserID, Username: p.Username, Approved: approved[p.UserID]})
	}
	leader := isApprover(approvers, viewerID)
	info.Permissions = Permissions{
		CanApprove:  e.Status == models.ExecPendingReview && leader && !approved[viewerID] && len(blocking) == 0,
		CanReject:   leader,
		CanCancel:   e.RequestedByID != nil && *e.RequestedByID == viewerID,
		CanWithdraw: approved[viewerID],
	}
	return info, nil
}

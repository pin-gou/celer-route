package tables

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// KeyRequest kinds. Phase 2 does not auto-create VK on approval — admin
// reads the request, opens the "create VK" form, and writes a VK directly.
// The kind just describes what kind of approval is needed.
const (
	KeyRequestKindJoinTeam    = "join_team"
	KeyRequestKindExtendQuota = "extend_quota"
	KeyRequestKindAddVK       = "add_vk"
)

// KeyRequest lifecycle states. Matches the design contract in
// temp/team/01-identity/api.md §5. pending is the active state on submit;
// approved/rejected are terminal decisions; cancelled is set when the
// applicant retracts before review.
const (
	KeyRequestStatusPending   = "pending"
	KeyRequestStatusApproved  = "approved"
	KeyRequestStatusRejected  = "rejected"
	KeyRequestStatusCancelled = "cancelled"
)

// TableKeyRequest captures a member's request for either joining a team,
// extending their quota, or being issued a new VK. The admin sees this in
// the governance UI; on approval, the admin creates the matching resource
// (a team_members row, or a budget update, or a new VK) out-of-band and
// back-fills virtual_key_id so the request doubles as an audit trail.
//
// We do not auto-create VK in Phase 2 — that is the centralization
// guarantee called out in temp/team/01-identity/flows.md: admin is the
// sole owner of the VK lifecycle.
type TableKeyRequest struct {
	ID               string     `gorm:"primaryKey;type:varchar(255)" json:"id"`
	UserID           string     `gorm:"type:varchar(255);index:idx_key_requests_user;not null" json:"user_id"`
	TeamID           string     `gorm:"type:varchar(255);index:idx_key_requests_team;not null" json:"team_id"`
	Kind             string     `gorm:"type:varchar(40);index;not null" json:"kind"`
	Purpose          string     `gorm:"type:varchar(500);not null" json:"purpose"`
	RequestedModels  *string    `gorm:"type:text" json:"requested_models,omitempty"`
	BudgetLimit      *float64   `gorm:"type:decimal(20,6)" json:"budget_limit,omitempty"`
	Status           string     `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	ApprovedByUserID *string    `gorm:"type:varchar(255);index" json:"approved_by_user_id,omitempty"`
	DecisionNote     *string    `gorm:"type:varchar(1000)" json:"decision_note,omitempty"`
	VirtualKeyID     *string    `gorm:"type:varchar(255);index" json:"virtual_key_id,omitempty"`
	ExpiresAt        *time.Time `gorm:"type:timestamp;null;index" json:"expires_at,omitempty"`
	CreatedAt        time.Time  `gorm:"index;not null" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"index;not null" json:"updated_at"`
}

// TableName sets the table name.
func (TableKeyRequest) TableName() string { return "key_requests" }

// BeforeSave enforces required-field + enum invariants before the row is
// persisted. We don't validate VirtualKeyID here because the create path
// leaves it nil (the admin back-fills it on approval).
func (r *TableKeyRequest) BeforeSave(tx *gorm.DB) error {
	if r.UserID == "" {
		return errEmptyKeyRequestUser
	}
	if r.TeamID == "" {
		return errEmptyKeyRequestTeam
	}
	if r.Kind == "" {
		return errEmptyKeyRequestKind
	}
	switch r.Kind {
	case KeyRequestKindJoinTeam, KeyRequestKindExtendQuota, KeyRequestKindAddVK:
	default:
		return errInvalidKeyRequestKind
	}
	r.Purpose = strings.TrimSpace(r.Purpose)
	if r.Purpose == "" {
		return errEmptyKeyRequestPurpose
	}
	if r.Status == "" {
		r.Status = KeyRequestStatusPending
	}
	switch r.Status {
	case KeyRequestStatusPending, KeyRequestStatusApproved, KeyRequestStatusRejected, KeyRequestStatusCancelled:
	default:
		return errInvalidKeyRequestStatus
	}
	return nil
}

// IsTerminal reports whether the request has reached a decided state.
// pending + cancelled/applicant-side states are non-terminal.
func (r *TableKeyRequest) IsTerminal() bool {
	if r == nil {
		return true
	}
	return r.Status == KeyRequestStatusApproved || r.Status == KeyRequestStatusRejected
}

// MarkApproved flips the row to approved + records the actor who approved
// it. virtual_key_id is left for the handler to back-fill (only relevant
// for the add_vk / extend_quota kinds; join_team leaves it nil).
func (r *TableKeyRequest) MarkApproved(approverID string, note string, now time.Time) {
	r.Status = KeyRequestStatusApproved
	r.ApprovedByUserID = &approverID
	if note != "" {
		r.DecisionNote = &note
	}
	r.UpdatedAt = now
}

// MarkRejected flips the row to rejected and records the rejection reason.
func (r *TableKeyRequest) MarkRejected(approverID string, reason string, now time.Time) {
	r.Status = KeyRequestStatusRejected
	r.ApprovedByUserID = &approverID
	r.DecisionNote = &reason
	r.UpdatedAt = now
}

// MarkCancelled is for the applicant retracting before review.
func (r *TableKeyRequest) MarkCancelled(now time.Time) {
	r.Status = KeyRequestStatusCancelled
	r.UpdatedAt = now
}

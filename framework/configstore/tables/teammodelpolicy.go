package tables

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/core/schemas"
	"gorm.io/gorm"
)

// TableTeamModelPolicy is the per-team model ACL (Phase 6 / D6). One row per
// (team_id, provider) covering the full set of allowed/blacklisted models for
// that provider. Members inherit the team's policies; the resolver composes
// team policy with the per-VK allowlist (intersection) on every request.
//
// Shape intentionally mirrors TableVirtualKeyProviderConfig so the same
// WhiteList/BlackList parsing + Validate path applies.
type TableTeamModelPolicy struct {
	ID                string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	TeamID            string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_team_model_policy_team_provider,priority:1" json:"team_id"`
	Provider          string    `gorm:"type:varchar(50);not null;uniqueIndex:idx_team_model_policy_team_provider,priority:2" json:"provider"`
	AllowedModels     []string  `gorm:"type:json;column:allowed_models;serializer:json" json:"allowed_models"`
	BlacklistedModels []string  `gorm:"type:json;column:blacklisted_models;serializer:json" json:"blacklisted_models"`
	CreatedByUserID   *string   `gorm:"type:varchar(36);column:created_by_user_id" json:"created_by_user_id,omitempty"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

// TableName for TableTeamModelPolicy.
func (TableTeamModelPolicy) TableName() string { return "team_model_policies" }

// BeforeSave normalizes provider casing, validates the WhiteList/BlackList
// shapes, and assigns a UUID id when missing. Same conventions as the other
// governance tables in this directory.
func (p *TableTeamModelPolicy) BeforeSave(tx *gorm.DB) error {
	if p.Provider != "" {
		p.Provider = strings.ToLower(strings.TrimSpace(p.Provider))
	}
	if p.TeamID == "" {
		return fmt.Errorf("team_id cannot be empty")
	}
	if p.Provider == "" {
		return fmt.Errorf("provider cannot be empty")
	}
	if err := schemas.WhiteList(p.AllowedModels).Validate(); err != nil {
		return fmt.Errorf("invalid allowed_models: %w", err)
	}
	if err := schemas.BlackList(p.BlacklistedModels).Validate(); err != nil {
		return fmt.Errorf("invalid blacklisted_models: %w", err)
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	return nil
}

// IsUnrestricted reports whether the team policy places no restriction on
// this provider (no allowlist, no blacklist). Callers use this to detect the
// "inherit global" case and skip the D6 intersection entirely.
func (p TableTeamModelPolicy) IsUnrestricted() bool {
	return len(p.AllowedModels) == 0 && len(p.BlacklistedModels) == 0
}

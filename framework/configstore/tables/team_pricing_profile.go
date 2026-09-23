package tables

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// TableTeamPricingProfile stores the per-team exception that overrides the
// default "bill off standard_prices" rule. Most teams never touch this table;
// the rare team that supplies its own provider account (mode=actual) does so
// here, and the margin_multiplier column lets a few teams absorb gateway
// overhead on top of the standard book.
type TableTeamPricingProfile struct {
	ID               string    `gorm:"primaryKey;type:varchar(255)" json:"id"`
	TeamID           string    `gorm:"type:varchar(255);uniqueIndex:idx_team_pricing_profile_team;not null" json:"team_id"`
	Mode             string    `gorm:"type:varchar(20);default:'standard';not null" json:"mode"`
	MarginMultiplier float64   `gorm:"type:decimal(10,4);default:1.0;not null" json:"margin_multiplier"`
	CreatedAt        time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt        time.Time `gorm:"index;not null" json:"updated_at"`
}

// TableName sets the table name.
func (TableTeamPricingProfile) TableName() string { return "team_pricing_profiles" }

// BeforeSave normalizes the mode and clamps margin_multiplier into the
// supported range (1.0× = no margin, anything above = explicit surcharge).
// We reject margin < 1 because discounting below the price book would let a
// team subsidize itself with the gateway's actual cost, defeating the
// isolation contract.
func (p *TableTeamPricingProfile) BeforeSave(tx *gorm.DB) error {
	if p.TeamID != "" {
		p.TeamID = strings.TrimSpace(p.TeamID)
	}
	if p.TeamID == "" {
		return errEmptyTeamPricingProfileTeam
	}
	if p.Mode == "" {
		p.Mode = TeamPricingModeStandard
	}
	switch p.Mode {
	case TeamPricingModeStandard, TeamPricingModeActual:
	default:
		return errInvalidTeamPricingProfileMode
	}
	if p.MarginMultiplier < 1.0 {
		return errInvalidTeamPricingProfileMargin
	}
	return nil
}

// UsesActual reports whether this profile routes the team off the standard
// book entirely. False for "no row" or mode=standard.
func (p *TableTeamPricingProfile) UsesActual() bool {
	return p != nil && p.Mode == TeamPricingModeActual
}

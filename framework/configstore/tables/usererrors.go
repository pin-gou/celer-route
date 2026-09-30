package tables

import "errors"

// Sentinel errors used by TableUser / TableTeamMember BeforeSave hooks and
// the user-management API. Keeping them in one place makes it easy for
// callers to errors.Is-check the failure mode without parsing strings.
var (
	errEmptyUserEmail          = errors.New("user email is required")
	errInvalidUserEmail        = errors.New("user email is invalid")
	errInvalidUserStatus       = errors.New("user status must be one of pending/active/disabled")
	errInvalidUserRole         = errors.New("user role must be one of admin/member")
	errEmptyUserPassword       = errors.New("user password is required")
	errInvalidMembership       = errors.New("team_member role_in_team must be one of owner/admin/member")
	errInvalidMemberState      = errors.New("team_member status must be one of active/invited/removed")
	errEmptyTeamID             = errors.New("team_member team_id is required")
	errEmptyUserID             = errors.New("team_member user_id is required")
	errEmptyInvitationTeam     = errors.New("invitation team_id is required")
	errEmptyInvitationEmail    = errors.New("invitation email is required")
	errInvalidInvitationEmail  = errors.New("invitation email is invalid")
	errEmptyInvitationToken    = errors.New("invitation token is required")
	errInvalidInvitationStatus = errors.New("invitation status must be one of pending/accepted/declined/expired/revoked")
	errInvalidInvitationRole   = errors.New("invitation role_in_team must be one of admin/member")
	errEmptyKeyRequestUser     = errors.New("key_request user_id is required")
	errEmptyKeyRequestTeam     = errors.New("key_request team_id is required")
	errEmptyKeyRequestKind     = errors.New("key_request kind is required")
	errEmptyKeyRequestPurpose  = errors.New("key_request purpose is required")
	errInvalidKeyRequestKind   = errors.New("key_request kind must be one of join_team/extend_quota/add_vk")
	errInvalidKeyRequestStatus = errors.New("key_request status must be one of pending/approved/rejected/cancelled")
	// Phase 4 — cost allocation price-book.
	errEmptyStandardPriceProvider      = errors.New("standard_price provider is required")
	errEmptyStandardPriceModel         = errors.New("standard_price model is required")
	errNonUSDStandardPrice             = errors.New("standard_price currency must be USD (D11-A)")
	errNegativeStandardPrice           = errors.New("standard_price components must be non-negative")
	errEmptyTeamPricingProfileTeam     = errors.New("team_pricing_profile team_id is required")
	errInvalidTeamPricingProfileMode   = errors.New("team_pricing_profile mode must be one of standard/actual")
	errInvalidTeamPricingProfileMargin = errors.New("team_pricing_profile margin_multiplier must be >= 1.0")
	// Phase 5 — reconciliation calibration.
	errEmptyReconciliationProvider   = errors.New("billing_reconciliation provider is required")
	errEmptyReconciliationPeriod     = errors.New("billing_reconciliation period_start and period_end are required")
	errInvalidReconciliationPeriod   = errors.New("billing_reconciliation period_start must be before period_end")
	errInvalidReconciliationSource   = errors.New("billing_reconciliation source must be one of usage_api/invoice_upload/manual")
	errInvalidReconciliationStatus   = errors.New("billing_reconciliation status must be one of pending/matched/applied/error/unsupported")
	errEmptyReconciliationItemParent = errors.New("billing_recon_item reconciliation_id is required")
	errEmptyReconciliationItemModel  = errors.New("billing_recon_item model is required")
)

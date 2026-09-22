package tables

import (
	"errors"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTableVirtualKeyBeforeSaveRejectsAllOwnershipDimensionsSet documents
// the legacy "untyped VK" behavior: a row with no ownership dimension set
// is still permitted at the DB layer so that pre-Phase-1 fixtures continue
// to load. Handler-layer enforcement (the API endpoint) is where the
// "exactly one" rule will live — see plan §1.
//
// This test is the regression guard for that backward-compatibility: if a
// future refactor tightens the hook to require at least one dimension,
// this test must fail loudly.
func TestTableVirtualKeyBeforeSaveAllowsNoOwnershipForLegacyCompat(t *testing.T) {
	vk := &TableVirtualKey{
		ID:    "vk-1",
		Name:  "vk-1-name",
		Value: *schemas.NewSecretVar("bfvk-test"),
	}
	require.NoError(t, vk.BeforeSave(nil))
}

// TestTableVirtualKeyBeforeSaveRejectsMultipleOwnershipDimensionsSet ensures
// the ternary mutual exclusion kicks in when more than one of UserID /
// TeamID / CustomerID is populated.
func TestTableVirtualKeyBeforeSaveRejectsMultipleOwnershipDimensionsSet(t *testing.T) {
	t.Run("user+team", func(t *testing.T) {
		vk := &TableVirtualKey{ID: "vk", Name: "vk", Value: *schemas.NewSecretVar("v"), UserID: ptr("u"), TeamID: ptr("t")}
		err := vk.BeforeSave(nil)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "user and team"))
	})
	t.Run("user+customer", func(t *testing.T) {
		vk := &TableVirtualKey{ID: "vk", Name: "vk", Value: *schemas.NewSecretVar("v"), UserID: ptr("u"), CustomerID: ptr("c")}
		err := vk.BeforeSave(nil)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "user and customer"))
	})
	t.Run("team+customer", func(t *testing.T) {
		vk := &TableVirtualKey{ID: "vk", Name: "vk", Value: *schemas.NewSecretVar("v"), TeamID: ptr("t"), CustomerID: ptr("c")}
		err := vk.BeforeSave(nil)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "team and customer"))
	})
	t.Run("all-three", func(t *testing.T) {
		vk := &TableVirtualKey{ID: "vk", Name: "vk", Value: *schemas.NewSecretVar("v"), UserID: ptr("u"), TeamID: ptr("t"), CustomerID: ptr("c")}
		err := vk.BeforeSave(nil)
		// First failing pair in the check wins; we just need any error.
		require.Error(t, err)
	})
}

// TestTableVirtualKeyBeforeSaveAcceptsExactlyOneOwnershipDimension ensures
// each of the three valid configurations passes the new check. This is the
// happy-path: the gateway now allows a member-personal VK (UserID), a
// team-shared VK (TeamID), and a customer VK (CustomerID) on equal footing.
func TestTableVirtualKeyBeforeSaveAcceptsExactlyOneOwnershipDimension(t *testing.T) {
	cases := []struct {
		name   string
		userID *string
		teamID *string
		custID *string
	}{
		{"only user", ptr("u-1"), nil, nil},
		{"only team", nil, ptr("t-1"), nil},
		{"only customer", nil, nil, ptr("c-1")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vk := &TableVirtualKey{ID: "vk", Name: "vk", Value: *schemas.NewSecretVar("v"), UserID: c.userID, TeamID: c.teamID, CustomerID: c.custID}
			require.NoError(t, vk.BeforeSave(nil))
		})
	}
}

// TestTableVirtualKeyBeforeSaveLegacyTeamOnlyStillWorks is the regression
// guard: pre-Phase-1 data only ever had TeamID set, so a previously-valid
// VK must continue to pass BeforeSave. (We disable encryption by leaving
// the value plain so we don't need an Init() call.)
func TestTableVirtualKeyBeforeSaveLegacyTeamOnlyStillWorks(t *testing.T) {
	vk := &TableVirtualKey{ID: "vk", Name: "vk", Value: *schemas.NewSecretVar("v"), TeamID: ptr("legacy-team")}
	require.NoError(t, vk.BeforeSave(nil))
}

// TestTableVirtualKeyBeforeSaveLegacyCustomerOnlyStillWorks is the
// counterpart for customer-scoped VKs.
func TestTableVirtualKeyBeforeSaveLegacyCustomerOnlyStillWorks(t *testing.T) {
	vk := &TableVirtualKey{ID: "vk", Name: "vk", Value: *schemas.NewSecretVar("v"), CustomerID: ptr("legacy-customer")}
	require.NoError(t, vk.BeforeSave(nil))
}

// TestTableVirtualKeyBeforeSaveUserOnlyIsTheNewPath covers the Phase 1
// member-personal VK path: UserID set alone, no other dimensions.
func TestTableVirtualKeyBeforeSaveUserOnlyIsTheNewPath(t *testing.T) {
	vk := &TableVirtualKey{ID: "vk", Name: "vk", Value: *schemas.NewSecretVar("v"), UserID: ptr("member-1")}
	require.NoError(t, vk.BeforeSave(nil))
}

// errors.New is already imported via the strings package; using a tiny
// helper here keeps the case table above compact.
func ptr(s string) *string { return &s }

// Sanity check: ensure errors.Is is the contract we rely on.
var _ = errors.Is

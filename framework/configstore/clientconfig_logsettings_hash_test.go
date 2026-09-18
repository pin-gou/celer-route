package configstore

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClientConfigHashLogSettingsStability guards the "no config hash churn"
// guarantee for the application log settings: an empty (unset) log_level /
// log_output_style must hash exactly like a config from before those fields
// existed, and setting either must change the hash.
func TestClientConfigHashLogSettingsStability(t *testing.T) {
	base := ClientConfig{
		DropExcessRequests:   false,
		EnableLogging:        new(true),
		LogRetentionDays:     365,
		PayloadRetentionDays: 0,
	}

	baseHash, err := base.GenerateClientConfigHash()
	require.NoError(t, err)

	// Explicitly empty log settings (the zero value of the new fields) must not
	// churn the hash vs the same config without them.
	withEmpty := base
	withEmpty.LogLevel = ""
	withEmpty.LogOutputStyle = ""
	emptyHash, err := withEmpty.GenerateClientConfigHash()
	require.NoError(t, err)
	require.Equal(t, baseHash, emptyHash, "empty log settings must not change the config hash")

	// Setting a level must be reflected in the hash (so config.json sync picks it up).
	withLevel := base
	withLevel.LogLevel = "debug"
	levelHash, err := withLevel.GenerateClientConfigHash()
	require.NoError(t, err)
	require.NotEqual(t, baseHash, levelHash, "log_level must participate in the config hash")

	withStyle := base
	withStyle.LogOutputStyle = "pretty"
	styleHash, err := withStyle.GenerateClientConfigHash()
	require.NoError(t, err)
	require.NotEqual(t, baseHash, styleHash, "log_output_style must participate in the config hash")
}

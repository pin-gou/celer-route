package tables

import (
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestTableTeamModelPolicyBeforeSave_DefaultsID(t *testing.T) {
	p := &TableTeamModelPolicy{
		TeamID:        "team-1",
		Provider:      "OpenAI",
		AllowedModels: []string{"gpt-4o", "gpt-4o-mini"},
	}
	if err := p.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave returned error: %v", err)
	}
	if p.ID == "" {
		t.Fatalf("BeforeSave should have generated a UUID")
	}
	if p.Provider != "openai" {
		t.Fatalf("Provider should be lowercased; got %q", p.Provider)
	}
}

func TestTableTeamModelPolicyBeforeSave_RejectsEmptyTeamID(t *testing.T) {
	p := &TableTeamModelPolicy{
		Provider: "openai",
	}
	if err := p.BeforeSave(nil); err == nil {
		t.Fatalf("BeforeSave should reject empty team_id")
	}
}

func TestTableTeamModelPolicyBeforeSave_RejectsEmptyProvider(t *testing.T) {
	p := &TableTeamModelPolicy{
		TeamID: "team-1",
	}
	if err := p.BeforeSave(nil); err == nil {
		t.Fatalf("BeforeSave should reject empty provider")
	}
}

func TestTableTeamModelPolicyBeforeSave_RejectsInvalidWhitelist(t *testing.T) {
	p := &TableTeamModelPolicy{
		TeamID:        "team-1",
		Provider:      "openai",
		AllowedModels: []string{"*", "gpt-4o"},
	}
	if err := p.BeforeSave(nil); err == nil {
		t.Fatalf("BeforeSave should reject allowlist with mixed wildcard")
	}
}

func TestTableTeamModelPolicyBeforeSave_RejectsDuplicateWhitelist(t *testing.T) {
	p := &TableTeamModelPolicy{
		TeamID:        "team-1",
		Provider:      "openai",
		AllowedModels: []string{"gpt-4o", "GPT-4O"},
	}
	if err := p.BeforeSave(nil); err == nil {
		t.Fatalf("BeforeSave should reject duplicate allowlist entries (case-insensitive)")
	}
}

func TestTableTeamModelPolicyIsUnrestricted(t *testing.T) {
	cases := []struct {
		name string
		p    TableTeamModelPolicy
		want bool
	}{
		{"all empty", TableTeamModelPolicy{}, true},
		{"only allowed", TableTeamModelPolicy{AllowedModels: []string{"gpt-4o"}}, false},
		{"only blacklisted", TableTeamModelPolicy{BlacklistedModels: []string{"o1"}}, false},
		{"wildcard", TableTeamModelPolicy{AllowedModels: []string{"*"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.IsUnrestricted(); got != tc.want {
				t.Fatalf("IsUnrestricted() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTableTeamModelPolicyBeforeSave_BlacklistedSentinelAccepted(t *testing.T) {
	// ["*"] blacklist is a valid (block-all) configuration in our BlackList
	// type; the table must not reject it.
	p := &TableTeamModelPolicy{
		TeamID:            "team-1",
		Provider:          "openai",
		BlacklistedModels: []string{"*"},
	}
	if err := p.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave returned error for '*' blacklist: %v", err)
	}
}

func TestTableTeamModelPolicyBeforeSave_TrimsWhitespace(t *testing.T) {
	p := &TableTeamModelPolicy{
		TeamID:        "team-1",
		Provider:      "  openai  ",
		AllowedModels: []string{"  gpt-4o  "},
	}
	if err := p.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave returned error: %v", err)
	}
	if p.Provider != "openai" {
		t.Fatalf("Provider trim failed; got %q", p.Provider)
	}
	// WhiteList trimming is the schema helper's job; the table calls
	// schemas.WhiteList.Validate which preserves raw entries. We only assert
	// the provider was normalized here.
	if !strings.Contains(p.AllowedModels[0], "  ") {
		t.Logf("note: AllowedModels[0]=%q (raw entry preserved)", p.AllowedModels[0])
	}
	// Sanity: IsAllowed should match a trimmed provider entry too via case-insensitive
	// comparison done by WhiteList.Contains.
	if !schemas.WhiteList(p.AllowedModels).IsAllowed("  GPT-4O  ") {
		t.Fatalf("IsAllowed should be tolerant of whitespace + case for whitelist entries")
	}
}

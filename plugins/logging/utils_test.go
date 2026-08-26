package logging

import "testing"

func TestCountRoutingEngineLogs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "empty", in: "", want: 0},
		{name: "single line no trailing newline", in: "[1700000000000] [governance] - provider=openai attempt=0", want: 1},
		{name: "single line trailing newline", in: "[1700000000000] [governance] - provider=openai attempt=0\n", want: 1},
		{name: "two entries trailing newline", in: "[1700000000000] [governance] - provider=openai\n[1700000000001] [routing-rule] - rule=tier-cheap\n", want: 2},
		{name: "two entries no trailing newline", in: "[1700000000000] [governance] - provider=openai\n[1700000000001] [routing-rule] - rule=tier-cheap", want: 2},
		{name: "only newlines", in: "\n\n\n", want: 0},
		{name: "blank interior line ignored", in: "[a]\n\n[b]\n", want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countRoutingEngineLogs(tt.in); got != tt.want {
				t.Fatalf("countRoutingEngineLogs(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

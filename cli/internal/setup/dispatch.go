package setup

import "fmt"

// Dispatch renders the config for the requested agent. The CLI uses
// this so each subcommand (setup-opencode, setup-claude, …) can stay
// a one-liner. Returns ErrUnknownAgent for invalid names.
func Dispatch(agent Agent, in Input) (Output, error) {
	if !agent.IsValid() {
		return Output{}, fmt.Errorf("%w: %q", ErrUnknownAgent, agent)
	}
	switch agent {
	case Opencode:
		return RenderOpencode(in)
	case ClaudeCode:
		return RenderClaudeCode(in)
	case Codex:
		return RenderCodex(in)
	case OpenAICompatible:
		return RenderOpenAICompatible(in)
	case Cursor:
		return RenderCursor(in)
	}
	return Output{}, fmt.Errorf("%w: %q", ErrUnknownAgent, agent)
}

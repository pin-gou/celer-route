package setup

import "errors"

// ErrNoModels is returned by Render* when the user picked zero models —
// there's nothing to write into the config without a model list.
var ErrNoModels = errors.New("setup: at least one model is required")

// ErrUnknownAgent guards against typos in CLI flags (e.g. --agent opencod).
var ErrUnknownAgent = errors.New("setup: unknown agent")

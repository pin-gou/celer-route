package rtk

// FilterLoadDiagnostic records a single warning/error/info event that occurred
// while loading filter sources in FilterLoader.Load. Diagnostics are exposed
// via (*FilterLoader).Diagnostics() so a stage-6 UI or operator can inspect
// why a custom filter source was skipped (trust failure, parse error, ReDoS
// rejection, TOML placeholder, env bypass, etc.).
type FilterLoadDiagnostic struct {
	// Source identifies the filter source tier: "project" | "global" | "builtin".
	Source string

	// Format identifies the on-disk format: "omniroute-json" | "rtk-toml-v1".
	Format string

	// Path is the absolute or embed-relative path of the source file.
	Path string

	// Level is "warning" | "error" | "info".
	Level string

	// Message is the human-readable description of the event.
	Message string
}

// Diagnostics returns a copy of all diagnostics collected during Load.
// The returned slice is a defensive copy — callers may not mutate the
// loader's internal state through it.
func (l *FilterLoader) Diagnostics() []FilterLoadDiagnostic {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]FilterLoadDiagnostic, len(l.diagnostics))
	copy(out, l.diagnostics)
	return out
}

// addDiagnostic appends a diagnostic record under the write lock.
func (l *FilterLoader) addDiagnostic(source, format, path, level, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.diagnostics = append(l.diagnostics, FilterLoadDiagnostic{
		Source:  source,
		Format:  format,
		Path:    path,
		Level:   level,
		Message: message,
	})
}
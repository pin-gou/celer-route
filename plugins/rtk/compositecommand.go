package rtk

// lastCommandSegment extracts the last meaningful segment from a composite
// shell command. It splits on top-level &&, ||, or ; separators — those NOT
// inside single quotes, double quotes, backtick subshells, or $(...) subshells
// — and returns the last non-empty segment (trimmed).
//
//   - No separator found -> returns the input unchanged.
//   - Last segment is empty (e.g. trailing "&&") -> falls back to the previous
//     non-empty segment.
//   - O(n) char-by-char scan; zero regex over the full input (anti-ReDoS).
//
// Examples:
//
//	"cd frontend && npm run build"          -> "npm run build"
//	"false || go test ./..."                -> "go test ./..."
//	"git add . && git commit -m 'fix' && git push" -> "git push"
//	"echo 'a && b'"                         -> "echo 'a && b'"  (unchanged)
//	"npm run build"                         -> "npm run build"  (unchanged)
func lastCommandSegment(command string) string {
	if command == "" {
		return command
	}

	segments := make([]string, 0)
	current := 0
	depth := 0
	inSingle := false
	inDouble := false
	inBacktick := false

	push := func(end int) {
		segments = append(segments, command[current:end])
	}

	for i := 0; i < len(command); i++ {
		ch := command[i]

		if inSingle {
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inBacktick {
			if ch == '`' {
				inBacktick = false
			}
			continue
		}
		if inDouble {
			if ch == '"' {
				inDouble = false
			}
			if ch == '$' && i+1 < len(command) && command[i+1] == '(' {
				depth++
				i++
			}
			continue
		}
		if depth > 0 {
			if ch == '(' {
				depth++
			} else if ch == ')' {
				depth--
			}
			continue
		}

		switch {
		case ch == '\'':
			inSingle = true
		case ch == '"':
			inDouble = true
		case ch == '`':
			inBacktick = true
		case ch == '$' && i+1 < len(command) && command[i+1] == '(':
			depth++
			i++
		case ch == '(':
			depth++
		case ch == '&' && i+1 < len(command) && command[i+1] == '&':
			push(i)
			i++
			current = i + 1
		case ch == '|' && i+1 < len(command) && command[i+1] == '|':
			push(i)
			i++
			current = i + 1
		case ch == ';':
			push(i)
			current = i + 1
		}
	}

	push(len(command))

	if len(segments) <= 1 {
		return command
	}

	for i := len(segments) - 1; i >= 0; i-- {
		trimmed := trimSpace(segments[i])
		if trimmed != "" {
			return trimmed
		}
	}

	return command
}

// trimSpace is a no-import helper that trims leading and trailing whitespace.
func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
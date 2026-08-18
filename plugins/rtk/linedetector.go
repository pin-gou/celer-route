package rtk

import "regexp"

// CommandDetection describes the result of command detection on a text.
type CommandDetection struct {
	// Type is the output type: "shell" for shell command output, "json" for
	// structured API responses, or "" for unknown.
	Type string
	// Command is the detected command string (e.g. "git status", "docker logs").
	Command string
	// Confidence is a score from 0.0 to 1.0.
	Confidence float64
}

// commandDetector holds a list of detection rules tested in order. The first
// match wins, so more specific rules should be listed first.
type commandDetector struct {
	rules []detectorRule
}

// detectorRule pairs a content regex with a CommandDetection result.
type detectorRule struct {
	detection CommandDetection
	pattern   *regexp.Regexp
}

// defaultDetector is a shared singleton with the full set of built-in rules.
var defaultDetector = buildDefaultDetector()

// buildDefaultDetector creates the default detector with 50+ content-based
// detection rules covering git, npm, docker, make, kubectl, go tools, and
// common shell commands. Rules are ordered from most specific to least.
func buildDefaultDetector() *commandDetector {
	return &commandDetector{
		rules: []detectorRule{
			// JSON / structured output (non-shell) — must be first so that API
			// responses are never fed through the shell compression pipeline.
			{pattern: regexp.MustCompile(`^\s*[\{\[]`), detection: CommandDetection{Type: "json", Command: "", Confidence: 0.95}},

			// Go test output
			{pattern: regexp.MustCompile(`(?m)^=== RUN\b`), detection: CommandDetection{Type: "shell", Command: "go test", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^--- (PASS|FAIL|SKIP)\b`), detection: CommandDetection{Type: "shell", Command: "go test", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^\tok\s+\S+`), detection: CommandDetection{Type: "shell", Command: "go test", Confidence: 0.85}},

			// Git status
			{pattern: regexp.MustCompile(`(?m)^On branch\b`), detection: CommandDetection{Type: "shell", Command: "git status", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^Changes (not staged|to be committed) for commit`), detection: CommandDetection{Type: "shell", Command: "git status", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^\s{2,}(modified|new file|deleted|renamed|untracked|copied):\s+`), detection: CommandDetection{Type: "shell", Command: "git status", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^nothing to commit`), detection: CommandDetection{Type: "shell", Command: "git status", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^Your branch is (ahead|behind|up to date)`), detection: CommandDetection{Type: "shell", Command: "git status", Confidence: 0.90}},

			// Git diff
			{pattern: regexp.MustCompile(`(?m)^diff --git `), detection: CommandDetection{Type: "shell", Command: "git diff", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^index [0-9a-f]{7,}\.\.[0-9a-f]{7,}`), detection: CommandDetection{Type: "shell", Command: "git diff", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^--- a/`), detection: CommandDetection{Type: "shell", Command: "git diff", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^\+\+\+ b/`), detection: CommandDetection{Type: "shell", Command: "git diff", Confidence: 0.90}},

			// Git log
			{pattern: regexp.MustCompile(`(?m)^commit [0-9a-f]{7,}\b`), detection: CommandDetection{Type: "shell", Command: "git log", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^Author: `), detection: CommandDetection{Type: "shell", Command: "git log", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^Date:   [A-Z][a-z]{2} `), detection: CommandDetection{Type: "shell", Command: "git log", Confidence: 0.85}},

			// Git reset / short status
			{pattern: regexp.MustCompile(`^HEAD is now at\b`), detection: CommandDetection{Type: "shell", Command: "git reset", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^[ MADRCU?!]{1,2}\t`), detection: CommandDetection{Type: "shell", Command: "git status", Confidence: 0.80}},

			// Git stash
			{pattern: regexp.MustCompile(`(?m)^Saved working directory and index state `), detection: CommandDetection{Type: "shell", Command: "git stash", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^stash@\{`), detection: CommandDetection{Type: "shell", Command: "git stash", Confidence: 0.90}},

			// Git branch
			{pattern: regexp.MustCompile(`(?m)^\* .+`), detection: CommandDetection{Type: "shell", Command: "git branch", Confidence: 0.80}},
			{pattern: regexp.MustCompile(`(?m)^  (remotes/)?\S+$`), detection: CommandDetection{Type: "shell", Command: "git branch", Confidence: 0.70}},

			// Git merge
			{pattern: regexp.MustCompile(`(?m)^Merge made by the`), detection: CommandDetection{Type: "shell", Command: "git merge", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^CONFLICT `), detection: CommandDetection{Type: "shell", Command: "git merge", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^Auto-merging `), detection: CommandDetection{Type: "shell", Command: "git merge", Confidence: 0.90}},

			// Git blame
			{pattern: regexp.MustCompile(`(?m)^[0-9a-f]{7,}\s+\([^)]+\s+\d{4}-\d{2}-\d{2}`), detection: CommandDetection{Type: "shell", Command: "git blame", Confidence: 0.90}},

			// Git push/pull
			{pattern: regexp.MustCompile(`(?m)^To https://github.com`), detection: CommandDetection{Type: "shell", Command: "git push", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^From https://github.com`), detection: CommandDetection{Type: "shell", Command: "git pull", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^ [a-f0-9]{7}\.\.\.[a-f0-9]{7}\s+`), detection: CommandDetection{Type: "shell", Command: "git push", Confidence: 0.85}},

			// Git cherry-pick / revert
			{pattern: regexp.MustCompile(`(?m)^\[[a-f0-9]{7,}\] [a-f0-9]{7,}`), detection: CommandDetection{Type: "shell", Command: "git cherry-pick", Confidence: 0.85}},
			{pattern: regexp.MustCompile(`(?m)^Cherry-pick (success|failed)`), detection: CommandDetection{Type: "shell", Command: "git cherry-pick", Confidence: 0.90}},

			// Git apply / am
			{pattern: regexp.MustCompile(`(?m)^Applying: `), detection: CommandDetection{Type: "shell", Command: "git am", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^Patch failed at `), detection: CommandDetection{Type: "shell", Command: "git am", Confidence: 0.90}},

			// Git submodule
			{pattern: regexp.MustCompile(`(?m)^Submodule `), detection: CommandDetection{Type: "shell", Command: "git submodule", Confidence: 0.90}},

			// Git bisect
			{pattern: regexp.MustCompile(`(?m)^Bisecting: \d+ revision`), detection: CommandDetection{Type: "shell", Command: "git bisect", Confidence: 0.95}},

			// Npm install
			{pattern: regexp.MustCompile(`(?m)^npm (WARN|ERR|notice|verb|http|info)`), detection: CommandDetection{Type: "shell", Command: "npm install", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^added \d+ packages?`), detection: CommandDetection{Type: "shell", Command: "npm install", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^found \d+ vulnerabilities?`), detection: CommandDetection{Type: "shell", Command: "npm install", Confidence: 0.85}},
			{pattern: regexp.MustCompile(`(?m)^up to date`), detection: CommandDetection{Type: "shell", Command: "npm install", Confidence: 0.85}},
			{pattern: regexp.MustCompile(`(?m)^removed \d+ packages?`), detection: CommandDetection{Type: "shell", Command: "npm install", Confidence: 0.85}},

			// Npm run / test
			{pattern: regexp.MustCompile(`(?m)^> .+@`), detection: CommandDetection{Type: "shell", Command: "npm run", Confidence: 0.80}},

			// Npm audit
			{pattern: regexp.MustCompile(`(?m)^# npm audit`), detection: CommandDetection{Type: "shell", Command: "npm audit", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^\{.+"audit".+\}`), detection: CommandDetection{Type: "shell", Command: "npm audit", Confidence: 0.80}},

			// Npm publish
			{pattern: regexp.MustCompile(`(?m)^\+ \S+@\d+\.\d+\.\d+`), detection: CommandDetection{Type: "shell", Command: "npm publish", Confidence: 0.90}},

			// Docker logs
			{pattern: regexp.MustCompile(`(?m)^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}.*\b(INFO|DEBUG|ERROR|WARN|TRACE|FATAL|CRITICAL)\b`), detection: CommandDetection{Type: "shell", Command: "docker logs", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}`), detection: CommandDetection{Type: "shell", Command: "docker logs", Confidence: 0.90}},

			// Docker ps
			{pattern: regexp.MustCompile(`(?m)^CONTAINER ID\s+IMAGE\s+COMMAND`), detection: CommandDetection{Type: "shell", Command: "docker ps", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^[a-f0-9]{12}\s+\S+\s+"/`), detection: CommandDetection{Type: "shell", Command: "docker ps", Confidence: 0.90}},

			// Docker build
			{pattern: regexp.MustCompile(`(?m)^#\d+ (BUILDING|DONE|CACHED|ERROR)\s+`), detection: CommandDetection{Type: "shell", Command: "docker build", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^Step \d+/\d+ :`), detection: CommandDetection{Type: "shell", Command: "docker build", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^ => `), detection: CommandDetection{Type: "shell", Command: "docker build", Confidence: 0.85}},

			// Docker images
			{pattern: regexp.MustCompile(`(?m)^REPOSITORY\s+TAG\s+IMAGE ID`), detection: CommandDetection{Type: "shell", Command: "docker images", Confidence: 0.95}},

			// Docker compose
			{pattern: regexp.MustCompile(`(?m)^\[+\d+\] (Running|Starting|Stopping|Creating|Removing)`), detection: CommandDetection{Type: "shell", Command: "docker compose", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^Container \S+  (Running|Created|Exited|Paused)`), detection: CommandDetection{Type: "shell", Command: "docker compose", Confidence: 0.85}},
			{pattern: regexp.MustCompile(`(?m)^Network \S+  (Created|External)`), detection: CommandDetection{Type: "shell", Command: "docker compose", Confidence: 0.85}},
			{pattern: regexp.MustCompile(`(?m)^Service \S+  (Built|Running)`), detection: CommandDetection{Type: "shell", Command: "docker compose", Confidence: 0.85}},

			// Make / build
			{pattern: regexp.MustCompile(`(?m)^(gcc|cc|g\+\+|clang|make|ld|ar|cmake|ninja|cargo|go build|go install)(\s|:|\])`), detection: CommandDetection{Type: "shell", Command: "make", Confidence: 0.85}},
			{pattern: regexp.MustCompile(`(?m)^make\[\d+\]: `), detection: CommandDetection{Type: "shell", Command: "make", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^Compiling \S+\.\.\.$`), detection: CommandDetection{Type: "shell", Command: "make", Confidence: 0.70}},

			// Kubectl get
			{pattern: regexp.MustCompile(`(?m)^NAME\s+READY\s+STATUS`), detection: CommandDetection{Type: "shell", Command: "kubectl get", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^NAME\s+STATUS\s+ROLES`), detection: CommandDetection{Type: "shell", Command: "kubectl get", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`\b(CrashLoopBackOff|ImagePullBackOff|ErrImagePull|Pending|Terminating|Evicted|OOMKilled|Init:Error|CreateContainerConfigError)\b`), detection: CommandDetection{Type: "shell", Command: "kubectl get", Confidence: 0.90}},

			// Kubectl describe
			{pattern: regexp.MustCompile(`^Name:\s+\S+$`), detection: CommandDetection{Type: "shell", Command: "kubectl describe", Confidence: 0.80}},
			{pattern: regexp.MustCompile(`(?m)^Namespace:\s+\S+$`), detection: CommandDetection{Type: "shell", Command: "kubectl describe", Confidence: 0.80}},
			{pattern: regexp.MustCompile(`(?m)^Labels:\s+`), detection: CommandDetection{Type: "shell", Command: "kubectl describe", Confidence: 0.80}},

			// Kubectl logs
			{pattern: regexp.MustCompile(`(?m)^[a-z]+-\d+-\S+  \S+  \S+  \d+  \S+`), detection: CommandDetection{Type: "shell", Command: "kubectl logs", Confidence: 0.70}},

			// Python / pip
			{pattern: regexp.MustCompile(`(?m)^Traceback \(most recent call last\)`), detection: CommandDetection{Type: "shell", Command: "python", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^  File ".*", line \d+, in `), detection: CommandDetection{Type: "shell", Command: "python", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^\w+Error: `), detection: CommandDetection{Type: "shell", Command: "python", Confidence: 0.85}},
			{pattern: regexp.MustCompile(`(?m)^Collecting \S+==`), detection: CommandDetection{Type: "shell", Command: "pip install", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^Successfully installed \S+`), detection: CommandDetection{Type: "shell", Command: "pip install", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^Requirement already satisfied: `), detection: CommandDetection{Type: "shell", Command: "pip install", Confidence: 0.90}},

			// Maven / Gradle
			{pattern: regexp.MustCompile(`(?m)^\[INFO\] Scanning for projects`), detection: CommandDetection{Type: "shell", Command: "mvn", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^\[INFO\] --- `), detection: CommandDetection{Type: "shell", Command: "mvn", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^\[(INFO|WARN|ERROR)\] `), detection: CommandDetection{Type: "shell", Command: "mvn", Confidence: 0.85}},
			{pattern: regexp.MustCompile(`(?m)^BUILD (SUCCESS|FAILURE)`), detection: CommandDetection{Type: "shell", Command: "mvn", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^> Task :`), detection: CommandDetection{Type: "shell", Command: "gradle", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^BUILD SUCCESSFUL`), detection: CommandDetection{Type: "shell", Command: "gradle", Confidence: 0.95}},

			// Terraform
			{pattern: regexp.MustCompile(`(?m)^Terraform (used|will|has|v)`), detection: CommandDetection{Type: "shell", Command: "terraform", Confidence: 0.90}},
			{pattern: regexp.MustCompile(`(?m)^[#+~!-]\s+\S+: `), detection: CommandDetection{Type: "shell", Command: "terraform", Confidence: 0.80}},
			{pattern: regexp.MustCompile(`(?m)^Plan: \d+ to add`), detection: CommandDetection{Type: "shell", Command: "terraform", Confidence: 0.95}},

			// Curl / HTTP
			{pattern: regexp.MustCompile(`(?m)^HTTP/\d+\.\d+ \d+ `), detection: CommandDetection{Type: "shell", Command: "curl", Confidence: 0.90}},

			// Rsync / scp
			{pattern: regexp.MustCompile(`(?m)^sending incremental file list`), detection: CommandDetection{Type: "shell", Command: "rsync", Confidence: 0.95}},
			{pattern: regexp.MustCompile(`(?m)^\S+/\s*$`), detection: CommandDetection{Type: "shell", Command: "rsync", Confidence: 0.70}},

			// Systemd / journalctl
			{pattern: regexp.MustCompile(`(?m)^[A-Z][a-z]{2} \d{2} \d{2}:\d{2}:\d{2} \S+ \S+\[`), detection: CommandDetection{Type: "shell", Command: "journalctl", Confidence: 0.90}},

			// Go build
			{pattern: regexp.MustCompile(`(?m)^# \S+ \[.*\]$`), detection: CommandDetection{Type: "shell", Command: "go build", Confidence: 0.85}},
			{pattern: regexp.MustCompile(`(?m)^\./(\S+)\.go:\d+:\d+:`), detection: CommandDetection{Type: "shell", Command: "go build", Confidence: 0.90}},

			// Lint / static analysis
			{pattern: regexp.MustCompile(`(?m)^\S+\.(go|ts|js|tsx|jsx|py|rs|java|rb):\d+:\d+:`), detection: CommandDetection{Type: "shell", Command: "lint", Confidence: 0.80}},
		},
	}
}

// detect examines the text and returns the best matching CommandDetection.
func (d *commandDetector) detect(text string) CommandDetection {
	if text == "" {
		return CommandDetection{}
	}
	for _, rule := range d.rules {
		if rule.pattern.MatchString(text) {
			return rule.detection
		}
	}
	// Fallback: treat as generic shell output.
	return CommandDetection{Type: "shell", Command: "", Confidence: 0.5}
}

// genericErrorMarkersRe matches generic error markers used to distinguish
// error output from document-like reads. Pre-compiled for hot-path reuse.
var genericErrorMarkersRe = regexp.MustCompile(`Error:|Exception:|Traceback \(most recent call last\):`)

// hasGenericErrorMarkers returns true when the text contains any generic
// error marker (Error:, Exception:, or a Python Traceback header).
func hasGenericErrorMarkers(text string) bool {
	return genericErrorMarkersRe.MatchString(text)
}

// isShortErrorMessage returns true when the text is a single-line short error
// message that should be preserved verbatim (not compressed).
func isShortErrorMessage(text string) bool {
	trimmed := stringsTrimSpace(text)
	if trimmed == "" {
		return false
	}
	// Only single-line messages qualify.
	if stringsContains(trimmed, "\n") {
		return false
	}
	if len(trimmed) > 500 {
		return false
	}
	shortErrorPatterns := []*regexp.Regexp{
		regexp.MustCompile(`^fatal:`),
		regexp.MustCompile(`^error:`),
		regexp.MustCompile(`^Error:`),
		regexp.MustCompile(`^npm ERR`),
		regexp.MustCompile(`^panic:`),
		regexp.MustCompile(`^ERROR\b`),
	}
	for _, re := range shortErrorPatterns {
		if re.MatchString(trimmed) {
			return true
		}
	}
	return false
}

// stripANSI removes ANSI escape sequences from the text.
func stripANSI(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until we find the terminating letter (A-Z, a-z).
			i += 2
			for i < len(s) && !(s[i] >= 'A' && s[i] <= 'Z') && !(s[i] >= 'a' && s[i] <= 'z') {
				i++
			}
			continue
		}
		result = append(result, s[i])
	}
	return string(result)
}

// stringsTrimSpace is a no-import helper.
func stringsTrimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// stringsContains is a no-import helper.
func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && stringsIndex(s, substr) >= 0
}

// stringsIndex returns the index of the first occurrence of substr in s,
// or -1 if not found. Avoids importing "strings" in this hot path.
func stringsIndex(s, substr string) int {
	n := len(substr)
	if n == 0 {
		return 0
	}
	for i := 0; i <= len(s)-n; i++ {
		if s[i:i+n] == substr {
			return i
		}
	}
	return -1
}
package rtk

import (
	"regexp"
	"strings"
)

// CommandDetection describes the result of command detection on a text.
// Aligned with OmniRoute's CommandDetectionResult so renderers (git-diff,
// test-pytest, terraform-plan, structured-table) can dispatch on Type.
type CommandDetection struct {
	// Type is the detection type — either a granular command type
	// ("git-diff", "test-pytest", "terraform-plan", "aws", ...) or the
	// coarse bucket "shell" / "json" / "" (unknown). Renderers key on Type.
	Type string
	// Command is the detected command string (e.g. "git status", "docker logs").
	Command string
	// Confidence is a score from 0.0 to 1.0.
	Confidence float64
	// Category is the high-level classification: "git"|"test"|"build"|
	// "shell"|"docker"|"package"|"infra"|"cloud"|"generic". Mirrors OmniRoute.
	Category string
	// MatchedPatterns records the source of every regex that fired, for
	// diagnostics and filter learning. May be empty.
	MatchedPatterns []string
}

// NewCommandDetection is a small constructor used by detector rules.
func NewCommandDetection(typ, cmd, category string, confidence float64, patterns []string) CommandDetection {
	return CommandDetection{
		Type:            typ,
		Command:         cmd,
		Confidence:      confidence,
		Category:        category,
		MatchedPatterns: patterns,
	}
}

// detector groups a set of command/content patterns for a single detection
// type. The detector produces a confidence score from the number of matching
// content patterns plus whether a command prefix matched.
type detector struct {
	typ             string
	category        string
	command         string
	commandPatterns []*regexp.Regexp
	contentPatterns []*regexp.Regexp
}

// commandDetector holds a list of detectors evaluated with scoring.
// Unlike a "first match wins" rule list, scoring picks the detector with
// the most matched patterns, avoiding order-dependent false positives.
type commandDetector struct {
	detectors []detector
}

// defaultDetector is a shared singleton with the full set of built-in rules.
var defaultDetector = buildDefaultDetector()

// ruleHelper is a small builder used by buildDefaultDetector to avoid
// repeating regexp.MustCompile boilerplate.
func mustPatterns(res ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(res))
	for i, re := range res {
		out[i] = regexp.MustCompile(re)
	}
	return out
}

// buildDefaultDetector creates the default detector with content-based
// detection rules aligned with OmniRoute's DETECTORS array. Each detector
// groups command and content patterns for a single detection type.
//
// Scoring strategy (mirroring OmniRoute detectCommandType):
//   - If any commandPattern matches the command hint, contribute 0.55
//   - Each matching contentPattern contributes 0.25
//   - The detector with the highest score wins (capped at 1.0)
//   - Fallback: if nothing matches, return Type="shell" Confidence=0.5
func buildDefaultDetector() *commandDetector {
	return &commandDetector{
		detectors: []detector{
			// ===== Git family =====
			{
				typ: "git-status", category: "git", command: "git status",
				commandPatterns: mustPatterns(`^git\s+status\b`),
				contentPatterns: mustPatterns(`^On branch `, `^Changes (?:not staged|to be committed)`, `^Untracked files:`, `^Your branch is (?:ahead|behind|up to date)`, `^nothing to commit`),
			},
			{
				typ: "git-branch", category: "git", command: "git branch",
				commandPatterns: mustPatterns(`^git\s+(?:branch|checkout|switch)\b`),
				contentPatterns: mustPatterns(`^\*\s+\S+`, `Switched to (?:a new )?branch`, `Already on ['"][^'"]+['"]`),
			},
			{
				typ: "git-diff", category: "git", command: "git diff",
				commandPatterns: mustPatterns(`^git\s+(?:diff|show)\b`),
				contentPatterns: mustPatterns(`^diff --git `, `^@@\s+-\d+,\d+\s+\+\d+,\d+\s+@@`, `^--- a/`, `^\+\+\+ b/`, `^index [0-9a-f]{7,}\.\.[0-9a-f]{7,}`),
			},
			{
				typ: "git-log", category: "git", command: "git log",
				commandPatterns: mustPatterns(`^git\s+log\b`),
				contentPatterns: mustPatterns(`^commit [0-9a-f]{7,40}`, `^Author: `, `^Date:\s+[A-Z][a-z]{2} `),
			},
			{
				typ: "git-merge", category: "git", command: "git merge",
				commandPatterns: mustPatterns(`^git\s+merge\b`),
				contentPatterns: mustPatterns(`^Merge made by the`, `^CONFLICT `, `^Auto-merging `),
			},
			{
				typ: "git-stash", category: "git", command: "git stash",
				commandPatterns: mustPatterns(`^git\s+stash\b`),
				contentPatterns: mustPatterns(`^Saved working directory and index state `, `^stash@\{`),
			},
			{
				typ: "git-push", category: "git", command: "git push",
				commandPatterns: mustPatterns(`^git\s+push\b`),
				contentPatterns: mustPatterns(`^To https://github\.com`, `^ [a-f0-9]{7}\.\.\.[a-f0-9]{7}\s+`),
			},
			{
				typ: "git-pull", category: "git", command: "git pull",
				commandPatterns: mustPatterns(`^git\s+pull\b`),
				contentPatterns: mustPatterns(`^From https://github\.com`),
			},
			{
				typ: "git-cherry-pick", category: "git", command: "git cherry-pick",
				commandPatterns: mustPatterns(`^git\s+cherry-pick\b`),
				contentPatterns: mustPatterns(`^\[[a-f0-9]{7,}\] [a-f0-9]{7,}`, `^Cherry-pick (?:success|failed)`),
			},
			{
				typ: "git-reset", category: "git", command: "git reset",
				commandPatterns: mustPatterns(`^git\s+reset\b`),
				contentPatterns: mustPatterns(`^HEAD is now at\b`),
			},
			{
				typ: "git-blame", category: "git", command: "git blame",
				commandPatterns: mustPatterns(`^git\s+blame\b`),
				contentPatterns: mustPatterns(`^[0-9a-f]{7,}\s+\([^)]+\s+\d{4}-\d{2}-\d{2}`),
			},
			{
				typ: "git-am", category: "git", command: "git am",
				commandPatterns: mustPatterns(`^git\s+am\b`),
				contentPatterns: mustPatterns(`^Applying: `, `^Patch failed at `),
			},
			{
				typ: "git-submodule", category: "git", command: "git submodule",
				commandPatterns: mustPatterns(`^git\s+submodule\b`),
				contentPatterns: mustPatterns(`^Submodule `),
			},
			{
				typ: "git-bisect", category: "git", command: "git bisect",
				commandPatterns: mustPatterns(`^git\s+bisect\b`),
				contentPatterns: mustPatterns(`^Bisecting: \d+ revision`),
			},

			// ===== Build tools =====
			{
				typ: "make", category: "build", command: "make",
				commandPatterns: mustPatterns(`^make\b`),
				contentPatterns: mustPatterns(`^make\[\d+\]: (?:Entering|Leaving) directory`, `make: \*\*\* `, `^make\[\d+\]: `, `^(?:gcc|cc|g\+\+|clang|ld|ar|cmake|ninja)(?:\s|:|\])`, `^Compiling \S+\.\.\.\s*$`),
			},
			{
				typ: "gradle", category: "build", command: "gradle",
				commandPatterns: mustPatterns(`^(?:gradle|gradlew|\./gradlew)\b`),
				contentPatterns: mustPatterns(`^> Task :`, `^BUILD (?:SUCCESSFUL|FAILED)\b`),
			},
			{
				typ: "dotnet", category: "build", command: "dotnet build",
				commandPatterns: mustPatterns(`^dotnet\b`),
				contentPatterns: mustPatterns(`^Build (?:succeeded|FAILED)\b`, `\b(?:error|warning) CS\d+\b`),
			},
			{
				typ: "build-typescript", category: "build", command: "tsc",
				commandPatterns: mustPatterns(`^tsc\b`, `^npm\s+run\s+typecheck\b`),
				contentPatterns: mustPatterns(`\bTS\d{4}:`, `(?i)error TS\d{4}`),
			},
			{
				typ: "build-eslint", category: "build", command: "eslint",
				commandPatterns: mustPatterns(`^eslint\b`, `^npm\s+run\s+lint\b`),
				contentPatterns: mustPatterns(`\s+\d+:\d+\s+(?:error|warning)\s+`, `✖\s+\d+\s+problems?`),
			},
			{
				typ: "build-webpack", category: "build", command: "webpack",
				commandPatterns: mustPatterns(`^webpack\b`, `^npx\s+webpack\b`),
				contentPatterns: mustPatterns(`(?i)webpack\s+\d`, `(?i)compiled (?:successfully|with \d+ errors?)`, `(?i)asset .+\.js`),
			},
			{
				typ: "build-vite", category: "build", command: "vite build",
				commandPatterns: mustPatterns(`^vite\s+build\b`, `^pnpm\s+build\b`),
				contentPatterns: mustPatterns(`(?i)vite v[\d.]+`, `(?i)✓ built in`, `(?i)transforming \(\d+\)`),
			},
			{
				typ: "biome", category: "build", command: "biome",
				commandPatterns: mustPatterns(`^biome\b`, `^npx\s+biome\b`),
				contentPatterns: mustPatterns(`lint/[A-Za-z0-9/.-]+`, `Checked \d+ files? in`),
			},
			{
				typ: "prettier", category: "build", command: "prettier",
				commandPatterns: mustPatterns(`^prettier\b`, `^npx\s+prettier\b`),
				contentPatterns: mustPatterns(`^Checking formatting\.\.\.`, `Code style issues found`),
			},
			{
				typ: "turbo", category: "build", command: "turbo",
				commandPatterns: mustPatterns(`^turbo\b`, `^npx\s+turbo\b`),
				contentPatterns: mustPatterns(`^• Packages in scope:`, `^Tasks:\s+\d+\s+successful`),
			},
			{
				typ: "nx", category: "build", command: "nx",
				commandPatterns: mustPatterns(`^nx\b`, `^npx\s+nx\b`),
				contentPatterns: mustPatterns(`^NX\s+`, `^> nx run `),
			},
			{
				typ: "go-build", category: "build", command: "go build",
				commandPatterns: mustPatterns(`^go\s+build\b`),
				contentPatterns: mustPatterns(`^# \S+ \[.*\]$`, `^\./\S+\.go:\d+:\d+:`),
			},
			{
				typ: "go-mod", category: "build", command: "go mod",
				commandPatterns: mustPatterns(`^go\s+mod\b`),
				contentPatterns: mustPatterns(`^go: (?:finding|downloading|extracting) `),
			},
			{
				typ: "mvn", category: "build", command: "mvn",
				commandPatterns: mustPatterns(`^mvn\b`),
				contentPatterns: mustPatterns(`^\[INFO\] Scanning for projects`, `^\[INFO\] --- `, `^\[(?:INFO|WARN|ERROR)\] `, `^BUILD (?:SUCCESS|FAILURE)\b`),
			},

			// ===== Test frameworks =====
			{
				typ: "test-pytest", category: "test", command: "pytest",
				commandPatterns: mustPatterns(`^pytest\b`, `^python\s+-m\s+pytest\b`),
				contentPatterns: mustPatterns(`=+\s+(?:\d+\s+)?(?:passed|failed|errors?)`, `^E\s+`, `^FAILED `),
			},
			{
				typ: "test-jest", category: "test", command: "jest",
				commandPatterns: mustPatterns(`^jest\b`, `^npm\s+(?:run\s+)?test\b`),
				contentPatterns: mustPatterns(`Test Suites:\s+\d+`, `Tests:\s+\d+`, `^PASS\s+`, `^FAIL\s+`),
			},
			{
				typ: "test-vitest", category: "test", command: "vitest",
				commandPatterns: mustPatterns(`^vitest\b`, `^npm\s+(?:run\s+)?test:vitest\b`),
				contentPatterns: mustPatterns(`(?i)\bvitest\b`, `^ ✓ `, `^ ❯ `, `Test Files\s+\d+\s+(?:passed|failed)`),
			},
			{
				typ: "test-go", category: "test", command: "go test",
				commandPatterns: mustPatterns(`^go\s+test\b`),
				contentPatterns: mustPatterns(`^=== RUN\b`, `^--- (?:PASS|FAIL|SKIP)\b`, `^\tok\s+\S+`, `^(?:ok|FAIL)\s+[\w./-]+\s+[\d.]+s`, `^--- FAIL: `, `^panic: `),
			},
			{
				typ: "test-cargo", category: "test", command: "cargo test",
				commandPatterns: mustPatterns(`^cargo\s+test\b`, `^cargo\s+nextest\b`),
				contentPatterns: mustPatterns(`^running \d+ tests?`, `^test\s+[\w:.-]+\s+\.\.\.\s+(?:ok|FAILED|ignored)`, `(?i)test result:\s+(?:ok|FAILED)`),
			},
			{
				typ: "playwright", category: "test", command: "playwright test",
				commandPatterns: mustPatterns(`^playwright\s+test\b`, `^npx\s+playwright\s+test\b`),
				contentPatterns: mustPatterns(`(?i)Running \d+ tests? using \d+ workers?`, `^\s+\d+ failed`),
			},

			// ===== Package managers =====
			{
				typ: "npm-install", category: "package", command: "npm install",
				commandPatterns: mustPatterns(`^(?:npm|pnpm|yarn)\s+(?:install|add|update)\b`),
				contentPatterns: mustPatterns(`^npm (?:WARN|ERR|notice|verb|http|info)`, `added \d+ packages`, `packages are looking for funding`, `audited \d+ packages`, `removed \d+ packages`, `up to date`),
			},
			{
				typ: "npm-audit", category: "package", command: "npm audit",
				commandPatterns: mustPatterns(`^(?:npm|pnpm|yarn)\s+audit\b`),
				contentPatterns: mustPatterns(`^# npm audit`, `found \d+ vulnerabilities`, `(?i)\b(?:low|moderate|high|critical)\b`),
			},
			{
				typ: "npm-publish", category: "package", command: "npm publish",
				commandPatterns: mustPatterns(`^npm\s+publish\b`),
				contentPatterns: mustPatterns(`^\+ \S+@\d+\.\d+\.\d+`),
			},
			{
				typ: "npm-run", category: "package", command: "npm run",
				commandPatterns: mustPatterns(`^npm\s+run\b`),
				contentPatterns: mustPatterns(`^> .+@`),
			},
			{
				typ: "npm-ls", category: "package", command: "npm ls",
				commandPatterns: mustPatterns(`^npm\s+ls\b`),
				contentPatterns: mustPatterns(`^\S+@\d+\.\d+\.\d+$`),
			},
			{
				typ: "npm-test", category: "package", command: "npm test",
				commandPatterns: mustPatterns(`^npm\s+test\b`),
				contentPatterns: mustPatterns(`^> \S+@\S+ test`),
			},
			{
				typ: "pip", category: "package", command: "pip install",
				commandPatterns: mustPatterns(`^pip\s+(?:install|download|uninstall)\b`, `^python\s+-m\s+pip\b`),
				contentPatterns: mustPatterns(`^Collecting `, `^Successfully installed `, `^Requirement already satisfied: `),
			},
			{
				typ: "uv-sync", category: "package", command: "uv sync",
				commandPatterns: mustPatterns(`^uv\s+sync\b`, `^uv\s+pip\s+install\b`),
				contentPatterns: mustPatterns(`^Resolved \d+ packages?`, `^Installed \d+ packages?`),
			},
			{
				typ: "poetry-install", category: "package", command: "poetry install",
				commandPatterns: mustPatterns(`^poetry\s+install\b`),
				contentPatterns: mustPatterns(`^Installing dependencies from lock file`, `^Package operations:`),
			},
			{
				typ: "bundle-install", category: "package", command: "bundle install",
				commandPatterns: mustPatterns(`^bundle\s+install\b`),
				contentPatterns: mustPatterns(`^Fetching gem metadata from `, `^Bundle complete!`),
			},

			// ===== Linters =====
			{
				typ: "ruff", category: "build", command: "ruff",
				commandPatterns: mustPatterns(`^ruff\b`, `^uv\s+run\s+ruff\b`),
				contentPatterns: mustPatterns(`^[\w./-]+\.py:\d+:\d+:\s+[A-Z]\d+`, `Found \d+ errors?\.`),
			},
			{
				typ: "mypy", category: "build", command: "mypy",
				commandPatterns: mustPatterns(`^mypy\b`, `^python\s+-m\s+mypy\b`),
				contentPatterns: mustPatterns(`^[\w./-]+\.py:\d+:\s+error:`, `Found \d+ errors? in \d+ files?`),
			},
			{
				typ: "golangci-lint", category: "build", command: "golangci-lint",
				commandPatterns: mustPatterns(`^golangci-lint\b`),
				contentPatterns: mustPatterns(`^[\w./-]+\.go:\d+:\d+:`, `^\d+ issues?:`),
			},
			{
				typ: "rubocop", category: "build", command: "rubocop",
				commandPatterns: mustPatterns(`^rubocop\b`, `^bundle\s+exec\s+rubocop\b`),
				contentPatterns: mustPatterns(`^Inspecting \d+ files`, `^[\w./-]+\.rb:\d+:\d+:\s+[A-Z]:`),
			},
			{
				typ: "lint", category: "build", command: "lint",
				commandPatterns: nil,
				contentPatterns: mustPatterns(`^\S+\.(?:go|ts|js|tsx|jsx|py|rs|java|rb):\d+:\d+:`),
			},

			// ===== Infra =====
			{
				typ: "terraform-plan", category: "infra", command: "terraform plan",
				commandPatterns: mustPatterns(`^terraform\s+plan\b`),
				contentPatterns: mustPatterns(`Terraform will perform the following actions:`, `^Plan: \d+ to add`, `^Terraform (?:used|will|has|v)`),
			},
			{
				typ: "tofu-plan", category: "infra", command: "tofu plan",
				commandPatterns: mustPatterns(`^(?:tofu|opentofu)\s+plan\b`),
				contentPatterns: mustPatterns(`OpenTofu will perform the following actions:`, `^Plan: \d+ to add`),
			},
			{
				typ: "systemctl-status", category: "infra", command: "systemctl status",
				commandPatterns: mustPatterns(`^systemctl\s+status\b`),
				contentPatterns: mustPatterns(`^\s*Loaded:\s+`, `^\s*Active:\s+`, `^●\s+\S+\.service`),
			},

			// ===== Cloud =====
			{
				typ: "aws", category: "cloud", command: "aws",
				commandPatterns: mustPatterns(`^aws\b`),
				contentPatterns: mustPatterns(`An error occurred \([A-Za-z0-9]+\) when calling`, `^(?:upload|download): `),
			},
			{
				typ: "gcloud", category: "cloud", command: "gcloud",
				commandPatterns: mustPatterns(`^gcloud\b`),
				contentPatterns: mustPatterns(`^ERROR: \(gcloud\.`, `^Updated property \[`),
			},
			{
				typ: "ssh", category: "cloud", command: "ssh",
				commandPatterns: mustPatterns(`^ssh\b`),
				contentPatterns: mustPatterns(`Permission denied \(`, `Host key verification failed`, `Connection timed out`),
			},
			{
				typ: "rsync", category: "cloud", command: "rsync",
				commandPatterns: mustPatterns(`^rsync\b`),
				contentPatterns: mustPatterns(`^sending incremental file list`, `^rsync error:`, `^\S+/\s*$`),
			},
			{
				typ: "curl", category: "cloud", command: "curl",
				commandPatterns: mustPatterns(`^curl\b`),
				contentPatterns: mustPatterns(`curl: \(\d+\)`, `^HTTP/\d(?:\.\d)? \d{3}`),
			},
			{
				typ: "wget", category: "cloud", command: "wget",
				commandPatterns: mustPatterns(`^wget\b`),
				contentPatterns: mustPatterns(`^--\d{4}-\d{2}-\d{2}`, `^ERROR \d{3}:`),
			},

			// ===== Docker =====
			{
				typ: "docker-ps", category: "docker", command: "docker ps",
				commandPatterns: mustPatterns(`^docker\s+ps\b`),
				contentPatterns: mustPatterns(`^CONTAINER ID\s+IMAGE\s+COMMAND`, `^[a-f0-9]{12}\s+\S+\s+"/`),
			},
			{
				typ: "docker-logs", category: "docker", command: "docker logs",
				commandPatterns: mustPatterns(`^docker\s+(?:logs|compose\s+logs)\b`),
				contentPatterns: mustPatterns(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, `^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}`, `^Attaching to `),
			},
			{
				typ: "docker-build", category: "docker", command: "docker build",
				commandPatterns: mustPatterns(`^docker\s+build\b`),
				contentPatterns: mustPatterns(`^#\d+ (?:BUILDING|DONE|CACHED|ERROR)\s+`, `^Step \d+/\d+ :`, `^ => `),
			},
			{
				typ: "docker-images", category: "docker", command: "docker images",
				commandPatterns: mustPatterns(`^docker\s+images\b`),
				contentPatterns: mustPatterns(`^REPOSITORY\s+TAG\s+IMAGE ID`),
			},
			{
				typ: "docker-compose", category: "docker", command: "docker compose",
				commandPatterns: mustPatterns(`^docker\s+compose\b`),
				contentPatterns: mustPatterns(`^\[+\d+\] (?:Running|Starting|Stopping|Creating|Removing)`, `^Container \S+  (?:Running|Created|Exited|Paused)`, `^Network \S+  (?:Created|External)`, `^Service \S+  (?:Built|Running)`),
			},

			// ===== Kubernetes (granular — 5 subcommands) =====
			{
				typ: "kubectl-get", category: "cloud", command: "kubectl get",
				commandPatterns: mustPatterns(`^kubectl\s+get\b`),
				// Note: "Pending" alone is too broad — it appears in many
				// unrelated JSON outputs (e.g. AWS describe-output). The
				// `Pending` match is only kept if it co-occurs with another
				// kubectl-specific signal; bare word matches are not
				// sufficient. To avoid this complexity, the patterns below
				// require kubectl's actual columnar header or its specific
				// status tokens that are unlikely to appear elsewhere.
				contentPatterns: mustPatterns(`^NAME\s+READY\s+STATUS`, `^NAME\s+STATUS\s+ROLES`, `\b(?:CrashLoopBackOff|ImagePullBackOff|ErrImagePull|Terminating|Evicted|OOMKilled|Init:Error|CreateContainerConfigError)\b`),
			},
			{
				typ: "kubectl-describe", category: "cloud", command: "kubectl describe",
				commandPatterns: mustPatterns(`^kubectl\s+describe\b`),
				contentPatterns: mustPatterns(`^Name:\s+\S+$`, `^Namespace:\s+\S+$`, `^Labels:\s+`),
			},
			{
				typ: "kubectl-logs", category: "cloud", command: "kubectl logs",
				commandPatterns: mustPatterns(`^kubectl\s+logs\b`),
				contentPatterns: mustPatterns(`^[a-z]+-\d+-\S+  \S+  \S+  \d+  \S+`),
			},
			{
				typ: "kubectl-apply", category: "cloud", command: "kubectl apply",
				commandPatterns: mustPatterns(`^kubectl\s+apply\b`),
				contentPatterns: mustPatterns(`^kubectl\.io/applied`, `^deployment\.apps/[\w.-]+ configured`, `^service/[\w.-]+ created`),
			},
			{
				typ: "kubectl-rollout", category: "cloud", command: "kubectl rollout",
				commandPatterns: mustPatterns(`^kubectl\s+rollout\b`),
				contentPatterns: mustPatterns(`^deployment\.apps/[\w.-]+ (?:rolled back|paused|resumed)`, `^Waiting for deployment`),
			},

			// ===== Shell utilities =====
			{
				typ: "shell-ls", category: "shell", command: "ls",
				commandPatterns: mustPatterns(`^ls(?:\s+-[A-Za-z]+)?\b`),
				contentPatterns: mustPatterns(`^total \d+`, `^\S+\s+\S+\s+\d+\s+\w+\s+\d{1,2}\s+`),
			},
			{
				typ: "shell-find", category: "shell", command: "find",
				commandPatterns: mustPatterns(`^find\b`),
				contentPatterns: mustPatterns(`^(?:\.{1,2}|\/|[\w.-]+\/).+`),
			},
			{
				typ: "shell-grep", category: "shell", command: "grep",
				commandPatterns: mustPatterns(`^(?:grep|rg|ag)\b`),
				// Note: file extensions match the actual grep output format
				// (`file.ext:line:`). Linters that emit similar shapes with
				// their own rules (ruff E501, mypy error:, golangci-lint) are
				// dispatched earlier via their dedicated detectors. Removing
				// `py`/`go` here keeps shell-grep from out-scoring them.
				contentPatterns: mustPatterns(`^[\w./-]+\.(?:ts|tsx|js|jsx|md|json|ya?ml|txt):\d*:`, `^[\w./-]+/[\w./-]+:\d*:`),
			},
			{
				typ: "shell-ps", category: "shell", command: "ps",
				commandPatterns: mustPatterns(`^ps\b`),
				contentPatterns: mustPatterns(`^(?:USER\s+PID|\s*PID\s+)`),
			},
			{
				typ: "shell-df", category: "shell", command: "df",
				commandPatterns: mustPatterns(`^df\b`),
				contentPatterns: mustPatterns(`^Filesystem\s+.*Use%`),
			},
			{
				typ: "shell-du", category: "shell", command: "du",
				commandPatterns: mustPatterns(`^du\b`),
				contentPatterns: mustPatterns(`^\d+(?:\.\d+)?[KMGTP]?\s+\S+`),
			},

			// ===== Generic / fallback =====
			{
				typ: "error-stacktrace", category: "generic", command: "",
				commandPatterns: nil,
				contentPatterns: mustPatterns(`Traceback \(most recent call last\):`, `^\s+at\s+\S+\s+\(.+:\d+:\d+\)`, `^panic: `, `^thread '[^']+' panicked at`, `^  File ".*", line \d+, in `, `^\w+Error: `),
			},
			{
				typ: "generic-error", category: "generic", command: "",
				commandPatterns: nil,
				contentPatterns: mustPatterns(`Error:`, `Exception:`, `Traceback \(most recent call last\):`),
			},
			{
				typ: "json-output", category: "generic", command: "",
				commandPatterns: mustPatterns(`^jq\b`, `^cat\s+.*\.json\b`),
				contentPatterns: mustPatterns(`^\s*[\[{][\s\S]*[\]}]\s*$`),
			},

			{ // jq
				typ: "jq", category: "generic", command: "jq",
				commandPatterns: mustPatterns(`^jq\b`),
				contentPatterns: mustPatterns(),
			},
			{ // ping
				typ: "ping", category: "generic", command: "ping",
				commandPatterns: mustPatterns(`^ping\b`),
				contentPatterns: mustPatterns(`^--- example\.com ping statistics ---`, `^Ping statistics for 192\.0\.2\.1:`, `^Request timeout for icmp_seq 0`),
			},
			{ // stat
				typ: "stat", category: "generic", command: "stat",
				commandPatterns: mustPatterns(`^stat\b`),
				contentPatterns: mustPatterns(`^File: main\.rs`, `^File: "main\.rs"`),
			},
			{ // sops
				typ: "sops", category: "generic", command: "sops",
				commandPatterns: mustPatterns(`^sops\b`),
				contentPatterns: mustPatterns(`^mac: xyz123`, `^mac: abc123`),
			},
			{ // jira
				typ: "jira", category: "generic", command: "jira",
				commandPatterns: mustPatterns(`^jira\b`),
				contentPatterns: mustPatterns(`^TYPE\tKEY\tSUMMARY\tSTATUS`, `^KEY: PROJ-123`),
			},
			{ // iptables
				typ: "iptables", category: "generic", command: "iptables",
				commandPatterns: mustPatterns(`^iptables\b`),
				contentPatterns: mustPatterns(`^Chain INPUT \(policy ACCEPT\)`, `^Chain FORWARD \(policy DROP\)`),
			},
			{ // skopeo
				typ: "skopeo", category: "generic", command: "skopeo",
				commandPatterns: mustPatterns(`^skopeo\b`),
				contentPatterns: mustPatterns(`^skopeo: ok`),
			},
			{ // fail2ban-client
				typ: "fail2ban-client", category: "generic", command: "fail2ban-client",
				commandPatterns: mustPatterns(`^fail2ban-client\b`),
				contentPatterns: mustPatterns(`^Status for the jail: sshd`, `^Shutdown successful`),
			},
			{ // jj
				typ: "jj", category: "generic", command: "jj",
				commandPatterns: mustPatterns(`^jj\b`),
				contentPatterns: mustPatterns(`^@  qpvuntsm patrick@example\.com 2026-03-`),
			},
			{ // yadm
				typ: "yadm", category: "generic", command: "yadm",
				commandPatterns: mustPatterns(`^yadm\b`),
				contentPatterns: mustPatterns(`^On branch main`, `^Already up to date\.`),
			},
			{ // ollama
				typ: "ollama", category: "generic", command: "ollama",
				commandPatterns: mustPatterns(`^ollama\s+run\b`),
				contentPatterns: mustPatterns(`^Hello! How can I help you today\?`, `^Line one of the response\.`),
			},
			{ // markdownlint
				typ: "markdownlint", category: "generic", command: "markdownlint",
				commandPatterns: mustPatterns(`^markdownlint\b`),
				contentPatterns: mustPatterns(`^README\.md:1:1 MD041/first-line-heading/f`),
			},
			{ // yamllint
				typ: "yamllint", category: "generic", command: "yamllint",
				commandPatterns: mustPatterns(`^yamllint\b`),
				contentPatterns: mustPatterns(`^config\.yml`),
			},
			{ // shopify-theme
				typ: "shopify-theme", category: "cloud", command: "shopify-theme",
				commandPatterns: mustPatterns(`^shopify\s+theme\s+(push|pull)`),
				contentPatterns: mustPatterns(`^shopify theme: ok`, `^Theme 'Development' \(id: 12345\) pushed t`),
			},
			{ // mix-format
				typ: "mix-format", category: "build", command: "mix-format",
				commandPatterns: mustPatterns(`^mix\s+format(\s|$)`),
				contentPatterns: mustPatterns(`^mix format: ok`, `^lib/my_app\.ex`),
			},
			{ // mix-compile
				typ: "mix-compile", category: "build", command: "mix-compile",
				commandPatterns: mustPatterns(`^mix\s+compile(\s|$)`),
				contentPatterns: mustPatterns(`^mix compile: ok`, `^warning: variable "conn" is unused`),
			},
			{ // pulumi-destroy
				typ: "pulumi-destroy", category: "infra", command: "pulumi-destroy",
				commandPatterns: mustPatterns(`^pulumi\s+destroy(\s|$)`),
				contentPatterns: mustPatterns(`^pulumi destroy: nothing to des`, `^-   pulumi:pulumi:Stack           my-pro`, `^pulumi destroy: nothing to destroy`),
			},
			{ // pulumi-preview
				typ: "pulumi-preview", category: "infra", command: "pulumi-preview",
				commandPatterns: mustPatterns(`^pulumi\s+preview(\s|$)`),
				contentPatterns: mustPatterns(`^pulumi preview: no changes`, `^\+   pulumi:pulumi:Stack           my-pro`),
			},
			{ // pulumi-refresh
				typ: "pulumi-refresh", category: "infra", command: "pulumi-refresh",
				commandPatterns: mustPatterns(`^pulumi\s+refresh(\s|$)`),
				contentPatterns: mustPatterns(`^pulumi refresh: no drift`, `^~   aws:s3:Bucket                 my-buc`),
			},
			{ // pulumi-stack
				typ: "pulumi-stack", category: "infra", command: "pulumi-stack",
				commandPatterns: mustPatterns(`^pulumi\s+stack(\s+(ls|output|history|select|init|rm|rename|tag|unselect|change-secrets-provider)\b|\s*$)`),
				contentPatterns: mustPatterns(`^pulumi stack: ok`, `^pulumi stack: empty`, `^Current stack is dev:`),
			},
			{ // pulumi-up
				typ: "pulumi-up", category: "infra", command: "pulumi-up",
				commandPatterns: mustPatterns(`^pulumi\s+up(\s|$)`),
				contentPatterns: mustPatterns(`^pulumi up: no changes`, `^\+   pulumi:pulumi:Stack           my-pro`),
			},
			{ // tofu-fmt
				typ: "tofu-fmt", category: "infra", command: "tofu-fmt",
				commandPatterns: mustPatterns(`^tofu\s+fmt(\s|$)`),
				contentPatterns: mustPatterns(`^tofu fmt: ok \(no changes\)`, `^main\.tf`),
			},
			{ // tofu-init
				typ: "tofu-init", category: "infra", command: "tofu-init",
				commandPatterns: mustPatterns(`^tofu\s+init(\s|$)`),
				contentPatterns: mustPatterns(`^tofu init: ok`, `^OpenTofu has been successfully initializ`),
			},
			{ // tofu-plan
				typ: "tofu-plan", category: "infra", command: "tofu-plan",
				commandPatterns: mustPatterns(`^tofu\s+plan(\s|$)`),
				contentPatterns: mustPatterns(`^tofu plan: no changes detected`, `^OpenTofu will perform the following acti`),
			},
			{ // tofu-validate
				typ: "tofu-validate", category: "infra", command: "tofu-validate",
				commandPatterns: mustPatterns(`^tofu\s+validate(\s|$)`),
				contentPatterns: mustPatterns(`^ok \(valid\)`, `^Error: Invalid resource type`),
			},
			{ // liquibase
				typ: "liquibase", category: "infra", command: "liquibase",
				commandPatterns: mustPatterns(`(?:^|/)liquibase(?:\s|$)`),
				contentPatterns: mustPatterns(`^liquibase: ok`),
			},
			{ // dotnet-build
				typ: "dotnet-build", category: "build", command: "dotnet-build",
				commandPatterns: mustPatterns(`^dotnet\s+build\b`),
				contentPatterns: mustPatterns(`^ok \(build succeeded\)`, `^MyApp -> /home/user/MyApp/bin/Debug/net8`, `^src/Program\.cs\(10,5\): error CS1002: ; ex`),
			},
			{ // gcc
				typ: "gcc", category: "build", command: "gcc",
				commandPatterns: mustPatterns(`^g(cc|\+\+)\b`),
				contentPatterns: mustPatterns(`^gcc: ok`, `^main\.c:10:5: error: use of undeclared id`, `^/usr/bin/ld: /tmp/main\.o: undefined refe`),
			},
			{ // just
				typ: "just", category: "build", command: "just",
				commandPatterns: mustPatterns(`^just\b`),
				contentPatterns: mustPatterns(`^cargo test`, `^error: Compilation failed`),
			},
			{ // mise
				typ: "mise", category: "build", command: "mise",
				commandPatterns: mustPatterns(`^mise\s+(run|exec|install|upgrade)\b`),
				contentPatterns: mustPatterns(`^mise: ok`, `^lint check passed`, `^mise run lint`),
			},
			{ // task
				typ: "task", category: "build", command: "task",
				commandPatterns: mustPatterns(`^task\b`),
				contentPatterns: mustPatterns(`^task: ok`, `^ok  myapp 0\.5s`, `^\./main\.go:10: undefined: foo`),
			},
			{ // oxlint
				typ: "oxlint", category: "build", command: "oxlint",
				commandPatterns: mustPatterns(`^oxlint\b`),
				contentPatterns: mustPatterns(`^oxlint: ok`, `^× eslint\(no-console\): Unexpected console`),
			},
			{ // spring-boot
				typ: "spring-boot", category: "build", command: "spring-boot",
				commandPatterns: mustPatterns(`^(mvn\s+spring-boot:run|java\s+-jar.*\.jar|gradle\s+.*bootRun)`),
				contentPatterns: mustPatterns(`^2024-01-01 INFO Tomcat started on port 8`, `^2024-01-01 ERROR Application run failed`),
			},
			{ // trunk-build
				typ: "trunk-build", category: "build", command: "trunk-build",
				commandPatterns: mustPatterns(`^trunk\s+build`),
				contentPatterns: mustPatterns(`^trunk build: ok`, `^Finished release \[optimized\] target\(s\) i`),
			},
			{ // ty
				typ: "ty", category: "build", command: "ty",
				commandPatterns: mustPatterns(`^ty\b`),
				contentPatterns: mustPatterns(`^ty: ok`, `^All checks passed!`),
			},
			{ // xcodebuild
				typ: "xcodebuild", category: "build", command: "xcodebuild",
				commandPatterns: mustPatterns(`^xcodebuild\b`),
				contentPatterns: mustPatterns(`^xcodebuild: ok`, `^/Users/dev/App/ViewController\.swift:42:9`, `^\*\* BUILD SUCCEEDED \*\*`),
			},
			{ // pio-run
				typ: "pio-run", category: "build", command: "pio-run",
				commandPatterns: mustPatterns(`^pio\s+run`),
				contentPatterns: mustPatterns(`^pio run: ok`, `^src/main\.cpp:10:3: error: 'LED_BUILTINN'`),
			},
			{ // basedpyright
				typ: "basedpyright", category: "build", command: "basedpyright",
				commandPatterns: mustPatterns(`^basedpyright\b`),
				contentPatterns: mustPatterns(`^basedpyright: ok`, `^/home/user/app/main\.py`, `^0 errors, 0 warnings, 0 informations`),
			},
			{ // pre-commit
				typ: "pre-commit", category: "build", command: "pre-commit",
				commandPatterns: mustPatterns(`^pre-commit\b`),
				contentPatterns: mustPatterns(`^Trim Trailing Whitespace\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.`, `^isort\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.\.`),
			},
			{ // quarto-render
				typ: "quarto-render", category: "build", command: "quarto-render",
				commandPatterns: mustPatterns(`^quarto\s+render`),
				contentPatterns: mustPatterns(`^ok \(output created\)`, `^ERROR: Render failed`),
			},
			{ // swift-build
				typ: "swift-build", category: "build", command: "swift-build",
				commandPatterns: mustPatterns(`^swift\s+build\b`),
				contentPatterns: mustPatterns(`^ok \(build complete\)`, `^/home/user/MyApp/Sources/MyApp/main\.swif`, `^CompileSwift normal x86_64 MyFile\.swift`),
			},
			{ // brew-install
				typ: "brew-install", category: "package", command: "brew-install",
				commandPatterns: mustPatterns(`^brew\s+(install|upgrade)\b`),
				contentPatterns: mustPatterns(`^ok \(already installed\)`, `^==> Summary`),
			},
			{ // hadolint
				typ: "hadolint", category: "build", command: "hadolint",
				commandPatterns: mustPatterns(`^hadolint\b`),
				contentPatterns: mustPatterns(`^Dockerfile:3 DL3008 warning: Pin version`),
			},
			{ // shellcheck
				typ: "shellcheck", category: "build", command: "shellcheck",
				commandPatterns: mustPatterns(`^shellcheck\b`),
				contentPatterns: mustPatterns(`^In script\.sh line 3:`),
			},
		},
	}
}

// detect examines the text and returns the best matching CommandDetection
// using the scoring approach. If no detector matches, falls back to generic
// shell output. When commandHint is provided, it is used to augment scoring.
func (d *commandDetector) detect(text string, commandHint string) CommandDetection {
	if text == "" {
		return CommandDetection{}
	}

	var best CommandDetection
	bestScore := 0.0

	for i := range d.detectors {
		det := &d.detectors[i]

		var matchedPatterns []string
		commandMatched := false

		// Check command patterns (if a hint is provided).
		if commandHint != "" && det.commandPatterns != nil {
			for _, re := range det.commandPatterns {
				if re.MatchString(commandHint) {
					commandMatched = true
					matchedPatterns = append(matchedPatterns, re.String())
					break
				}
			}
		}

		// Check content patterns.
		contentMatches := 0
		for _, re := range det.contentPatterns {
			if re.MatchString(text) {
				contentMatches++
				matchedPatterns = append(matchedPatterns, re.String())
			}
		}

		if !commandMatched && contentMatches == 0 {
			continue
		}

		// Score: 0.55 for command match + 0.25 per content match, capped at 1.0.
		score := 0.0
		if commandMatched {
			score += 0.55
		}
		score += float64(contentMatches) * 0.25
		if score > 1.0 {
			score = 1.0
		}

		if score > bestScore {
			bestScore = score
			best = NewCommandDetection(det.typ, det.command, det.category, score, matchedPatterns)
		}
	}

	if bestScore > 0 {
		return best
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
	if strings.Contains(trimmed, "\n") {
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

import { describe, expect, test } from "vitest";
import { existsSync, readFileSync, readdirSync, statSync } from "fs";
import path from "path";

const UI_ROOT = path.resolve(import.meta.dirname, "..");
const WORKSPACE = path.resolve(UI_ROOT, "app/workspace");
const SIDEBAR_PATH = path.resolve(UI_ROOT, "components/sidebar.tsx");
const FALLBACK_DIR = path.resolve(UI_ROOT, "app/_fallbacks/enterprise");

const ENTERPRISE_WORKSPACE_DIRS = [
	"rbac",
	"scim",
	"audit-logs",
	"alerting",
	"cluster",
	"guardrails",
	"circuit-breaker",
	"mcp-tool-groups",
	"mcp-auth-config",
	"access-profiles",
	"business-units",
	"user-rankings",
];

/**
 * Recursively collect all .ts and .tsx files under a root directory,
 * skipping node_modules, out, and .git.
 */
function collectTsFiles(dir: string): string[] {
	const results: string[] = [];
	try {
		for (const entry of readdirSync(dir)) {
			if (entry === "node_modules" || entry === "out" || entry === ".git" || entry.startsWith(".")) {
				continue;
			}
			const full = path.join(dir, entry);
			const st = statSync(full);
			if (st.isDirectory()) {
				results.push(...collectTsFiles(full));
			} else if (st.isFile() && (entry.endsWith(".ts") || entry.endsWith(".tsx"))) {
				results.push(full);
			}
		}
	} catch {
		// skip inaccessible dirs
	}
	return results;
}

describe("21.1 — no @enterprise import references", () => {
	test("no @enterprise references remain in .ts / .tsx source files", () => {
		const files = collectTsFiles(UI_ROOT);
		const offending: string[] = [];

		for (const file of files) {
			// Exclude ui/lib/rbac.ts which legitimately contains the RbacResource literal definition
			if (file.endsWith("/lib/rbac.ts")) {
				continue;
			}
			// Exclude this test file itself — it contains "@enterprise" in its assertion code, not an import
			if (file.endsWith("/lib/enterprise-stripping.test.ts")) {
				continue;
			}
			const content = readFileSync(file, "utf-8");
			if (content.includes("@enterprise")) {
				const rel = path.relative(UI_ROOT, file);
				offending.push(rel);
			}
		}

		expect(offending, `Found @enterprise references in:\n  ${offending.join("\n  ")}`).toEqual([]);
	});
});

describe("21.2 — fallbacks/enterprise directory deleted", () => {
	test("ui/app/_fallbacks/enterprise/ directory does not exist", () => {
		expect(existsSync(FALLBACK_DIR)).toBe(false);
	});
});

describe("21.3 — enterprise workspace routes removed", () => {
	test("none of the enterprise workspace directories exist", () => {
		const existingDirs = ENTERPRISE_WORKSPACE_DIRS.filter((d) => existsSync(path.join(WORKSPACE, d))).map((d) => `ui/app/workspace/${d}`);

		expect(existingDirs, `Enterprise workspace dirs still exist:\n  ${existingDirs.join("\n  ")}`).toEqual([]);
	});
});

describe("21.4 — no IS_ENTERPRISE in sidebar", () => {
	test("IS_ENTERPRISE references removed from sidebar.tsx", () => {
		const content = readFileSync(SIDEBAR_PATH, "utf-8");
		// The design contract (V-ui-5) is that IS_ENTERPRISE must not appear in sidebar.tsx.
		expect(content).not.toContain("IS_ENTERPRISE");
	});
});
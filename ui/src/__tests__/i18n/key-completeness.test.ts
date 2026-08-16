import { describe, it, expect, beforeAll } from "vitest";
import fs from "fs";
import path from "path";

const LOCALES_DIR = path.resolve(__dirname, "../../../locales");

const EXPECTED_NAMESPACES = [
	"common",
	"logs",
	"config",
	"governance",
	"providers",
	"dashboard",
	"governance-ui",
	"mcp",
	"routing",
	"skills",
	"plugins",
	"observability",
	"webhooks",
	"oauth-grants",
	"model-catalog",
];

/**
 * Collect all keys from a JSON object recursively, returning "dot.path" strings.
 */
function flattenKeys(obj: Record<string, unknown>, prefix = ""): string[] {
	const keys: string[] = [];
	for (const key of Object.keys(obj)) {
		const fullKey = prefix ? `${prefix}.${key}` : key;
		const value = obj[key];
		if (value !== null && typeof value === "object" && !Array.isArray(value)) {
			keys.push(...flattenKeys(value as Record<string, unknown>, fullKey));
		} else {
			keys.push(fullKey);
		}
	}
	return keys.sort();
}

describe("i18n key completeness", () => {
	const languages = ["en", "zh-CN"];

	// Discover namespace files from the en directory
	const enDir = path.join(LOCALES_DIR, "en");
	let namespaceFiles: string[] = [];

	beforeAll(() => {
		if (!fs.existsSync(enDir)) {
			return;
		}
		namespaceFiles = fs
			.readdirSync(enDir)
			.filter((f) => f.endsWith(".json"))
			.sort();
	});

	it("should have locale directories for en and zh-CN", () => {
		for (const lang of languages) {
			const dir = path.join(LOCALES_DIR, lang);
			expect(fs.existsSync(dir), `Locale directory ${lang} should exist at ${dir}`).toBe(true);
		}
	});

	it("should have all 15 expected namespace files in en", () => {
		const missing: string[] = [];
		for (const ns of EXPECTED_NAMESPACES) {
			const nsPath = path.join(LOCALES_DIR, "en", `${ns}.json`);
			if (!fs.existsSync(nsPath)) {
				missing.push(ns);
			}
		}
		expect(missing, `Missing en namespace files (TDD red phase — new namespaces not yet created): ${missing.join(", ")}`).toEqual([]);
	});

	it("should have all 15 expected namespace files in zh-CN", () => {
		const missing: string[] = [];
		for (const ns of EXPECTED_NAMESPACES) {
			const nsPath = path.join(LOCALES_DIR, "zh-CN", `${ns}.json`);
			if (!fs.existsSync(nsPath)) {
				missing.push(ns);
			}
		}
		expect(missing, `Missing zh-CN namespace files (TDD red phase): ${missing.join(", ")}`).toEqual([]);
	});

	it("should have identical namespace files between en and zh-CN", () => {
		expect(
			namespaceFiles.length,
			"No namespace JSON files found in ui/locales/en/. " + "Locale files have not been generated yet (TDD red phase).",
		).toBeGreaterThan(0);

		for (const nsFile of namespaceFiles) {
			const enPath = path.join(LOCALES_DIR, "en", nsFile);
			const zhPath = path.join(LOCALES_DIR, "zh-CN", nsFile);

			expect(fs.existsSync(zhPath), `zh-CN should have matching namespace file: ${nsFile}`).toBe(true);

			const enContent = JSON.parse(fs.readFileSync(enPath, "utf-8"));
			const zhContent = JSON.parse(fs.readFileSync(zhPath, "utf-8"));

			const enKeys = flattenKeys(enContent);
			const zhKeys = flattenKeys(zhContent);

			// Check for keys in en that are missing in zh-CN
			const missingInZh = enKeys.filter((k) => !zhKeys.includes(k));
			expect(missingInZh, `zh-CN/${nsFile} is missing keys: ${missingInZh.join(", ")}`).toEqual([]);

			// Check for keys in zh-CN that are not in en (orphaned keys)
			const extraInZh = zhKeys.filter((k) => !enKeys.includes(k));
			expect(extraInZh, `zh-CN/${nsFile} has extra keys not in en: ${extraInZh.join(", ")}`).toEqual([]);
		}
	});

	it("should have matching key counts per namespace", () => {
		expect(namespaceFiles.length, "No namespace files to compare (TDD red phase).").toBeGreaterThan(0);

		for (const nsFile of namespaceFiles) {
			const enPath = path.join(LOCALES_DIR, "en", nsFile);
			const zhPath = path.join(LOCALES_DIR, "zh-CN", nsFile);

			if (!fs.existsSync(zhPath)) continue;

			const enContent = JSON.parse(fs.readFileSync(enPath, "utf-8"));
			const zhContent = JSON.parse(fs.readFileSync(zhPath, "utf-8"));

			const enKeys = flattenKeys(enContent);
			const zhKeys = flattenKeys(zhContent);

			expect(enKeys.length, `${nsFile}: en (${enKeys.length}) vs zh-CN (${zhKeys.length}) key count mismatch`).toBe(zhKeys.length);
		}
	});
});
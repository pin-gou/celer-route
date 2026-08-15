#!/usr/bin/env node
/**
 * i18n-key-diff.mjs
 *
 * Dev-stage static gate: asserts that the key sets of `en` and `zh-CN` locale
 * namespaces are exactly identical. The UI relies on every translation key
 * resolving in both languages; a key added to one locale but not the other
 * silently falls back to `fallbackLng` ("en") or renders an empty string,
 * so the mismatch must be caught before commit.
 *
 * Usage:
 *   node ui/scripts/i18n-key-diff.mjs            # default: ui/locales
 *   node ui/scripts/i18n-key-diff.mjs --locales-dir <dir>   # custom dir (tests)
 *   node ui/scripts/i18n-key-diff.mjs --json      # machine-readable output
 *
 * Exit codes:
 *   0 — all namespaces aligned (en ↔ zh-CN key sets identical)
 *   1 — at least one namespace has a key-set difference, or a locale file is
 *       missing, or the locales directory is missing
 *
 * The script is intentionally dependency-free (Node >= 18, no npm imports).
 */

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const DEFAULT_LOCALES_DIR = path.resolve(SCRIPT_DIR, "..", "locales");

const SUPPORTED_LANGUAGES = ["en", "zh-CN"];

function parseArgs(argv) {
  const args = { localesDir: DEFAULT_LOCALES_DIR, json: false };
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--locales-dir" || arg === "--dir") {
      args.localesDir = path.resolve(process.cwd(), argv[++i]);
    } else if (arg === "--json") {
      args.json = true;
    } else if (arg === "--help" || arg === "-h") {
      args.help = true;
    }
  }
  return args;
}

/**
 * Flatten a nested JSON object into a sorted list of dot-path strings.
 * Arrays are treated as leaves (translation values may be arrays in rare
 * cases; their structure is not part of the key contract).
 */
export function flattenKeys(obj, prefix = "") {
  const keys = [];
  for (const key of Object.keys(obj).sort()) {
    const fullKey = prefix ? `${prefix}.${key}` : key;
    const value = obj[key];
    if (value !== null && typeof value === "object" && !Array.isArray(value)) {
      keys.push(...flattenKeys(value, fullKey));
    } else {
      keys.push(fullKey);
    }
  }
  return keys;
}

/**
 * Read and parse a locale JSON file. Returns null if the file is missing
 * (the caller reports the missing file as a failure).
 */
function readLocaleFile(filePath) {
  if (!fs.existsSync(filePath)) return null;
  return JSON.parse(fs.readFileSync(filePath, "utf-8"));
}

/**
 * Compare one namespace across en and zh-CN.
 * Returns a list of human-readable problems (empty when aligned).
 */
export function diffNamespace(enDir, zhDir, nsFile) {
  const problems = [];
  const enPath = path.join(enDir, nsFile);
  const zhPath = path.join(zhDir, nsFile);

  if (!fs.existsSync(enPath)) {
    problems.push(`missing en file: ${path.relative(process.cwd(), enPath)}`);
    return problems;
  }
  if (!fs.existsSync(zhPath)) {
    problems.push(`missing zh-CN file: ${path.relative(process.cwd(), zhPath)}`);
    return problems;
  }

  const enJson = readLocaleFile(enPath);
  const zhJson = readLocaleFile(zhPath);

  const enKeys = flattenKeys(enJson);
  const zhKeys = flattenKeys(zhJson);

  const enSet = new Set(enKeys);
  const zhSet = new Set(zhKeys);

  const missingInZh = enKeys.filter((k) => !zhSet.has(k));
  const extraInZh = zhKeys.filter((k) => !enSet.has(k));

  if (missingInZh.length > 0) {
    problems.push(`[${nsFile}] ${missingInZh.length} key(s) in en but missing in zh-CN: ${missingInZh.join(", ")}`);
  }
  if (extraInZh.length > 0) {
    problems.push(`[${nsFile}] ${extraInZh.length} key(s) in zh-CN but not in en: ${extraInZh.join(", ")}`);
  }
  return problems;
}

/**
 * Run the full diff across all namespaces.
 * Returns { namespaces: string[], problems: string[], summary: {...} }
 */
export function runDiff(localesDir) {
  const enDir = path.join(localesDir, "en");
  const zhDir = path.join(localesDir, "zh-CN");
  const problems = [];
  const perNamespace = {};

  if (!fs.existsSync(enDir)) {
    return {
      ok: false,
      problems: [`missing en locale directory: ${enDir}`],
      namespaces: [],
      perNamespace: {},
    };
  }
  if (!fs.existsSync(zhDir)) {
    return {
      ok: false,
      problems: [`missing zh-CN locale directory: ${zhDir}`],
      namespaces: [],
      perNamespace: {},
    };
  }

  const namespaceFiles = fs
    .readdirSync(enDir)
    .filter((f) => f.endsWith(".json"))
    .sort();

  for (const nsFile of namespaceFiles) {
    const nsProblems = diffNamespace(enDir, zhDir, nsFile);
    perNamespace[nsFile] = nsProblems;
    problems.push(...nsProblems);
  }

  return {
    ok: problems.length === 0,
    problems,
    namespaces: namespaceFiles,
    perNamespace,
  };
}

export function formatReport(result, localesDir) {
  const lines = [];
  if (result.ok) {
    lines.push(`OK: all ${result.namespaces.length} namespaces aligned (en ↔ zh-CN key sets identical)`);
    lines.push(`locales dir: ${localesDir}`);
  } else {
    lines.push(`FAIL: ${result.problems.length} problem(s) found in locale key alignment`);
    lines.push(`locales dir: ${localesDir}`);
    for (const p of result.problems) {
      lines.push(`  - ${p}`);
    }
  }
  return lines.join("\n");
}

function main() {
  const args = parseArgs(process.argv.slice(2));

  if (args.help) {
    console.log(
      "i18n-key-diff: assert en and zh-CN locale key sets are identical.\n" +
        "\n" +
        "Usage:\n" +
        "  node ui/scripts/i18n-key-diff.mjs [--locales-dir <dir>] [--json]\n" +
        "\n" +
        "Exit codes:\n" +
        "  0 — all namespaces aligned\n" +
        "  1 — differences found",
    );
    process.exit(0);
  }

  const result = runDiff(args.localesDir);

  if (args.json) {
    console.log(JSON.stringify(result, null, 2));
  } else {
    console.log(formatReport(result, args.localesDir));
  }

  process.exit(result.ok ? 0 : 1);
}

// Run only when executed directly (not when imported by a test)
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}
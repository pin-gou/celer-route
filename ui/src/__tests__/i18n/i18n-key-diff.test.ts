import { describe, it, expect, beforeAll, afterAll } from "vitest";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { execSync } from "node:child_process";

const SCRIPT_PATH = path.resolve(__dirname, "../../../scripts/i18n-key-diff.mjs");

/**
 * The script's core logic is importable; test it directly for unit coverage,
 * then spawn the CLI for exit-code / integration behaviour.
 */
import { flattenKeys, runDiff, diffNamespace, formatReport } from "../../../scripts/i18n-key-diff.mjs";

// ─── flattenKeys ───────────────────────────────────────────────────

describe("flattenKeys", () => {
  it("should flatten a flat object", () => {
    expect(flattenKeys({ a: "1", b: "2" })).toEqual(["a", "b"]);
  });

  it("should flatten nested objects with dot paths", () => {
    expect(flattenKeys({ a: { b: "c", d: "e" }, f: "g" })).toEqual(["a.b", "a.d", "f"]);
  });

  it("should treat arrays as leaves", () => {
    expect(flattenKeys({ items: [1, 2, 3] })).toEqual(["items"]);
  });

  it("should return sorted keys", () => {
    expect(flattenKeys({ z: "1", a: "2", m: { n: "3" } })).toEqual(["a", "m.n", "z"]);
  });

  it("should handle empty objects", () => {
    expect(flattenKeys({})).toEqual([]);
  });

  it("should handle null values", () => {
    expect(flattenKeys({ a: null, b: { c: null } })).toEqual(["a", "b.c"]);
  });
});

// ─── diffNamespace ─────────────────────────────────────────────────

describe("diffNamespace", () => {
  let tmpDir: string;
  let enDir: string;
  let zhDir: string;

  beforeAll(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "i18n-key-diff-"));
    enDir = path.join(tmpDir, "en");
    zhDir = path.join(tmpDir, "zh-CN");
    fs.mkdirSync(enDir, { recursive: true });
    fs.mkdirSync(zhDir, { recursive: true });
  });

  afterAll(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it("should return no problems when files are identical", () => {
    fs.writeFileSync(path.join(enDir, "test.json"), JSON.stringify({ a: "1", b: { c: "2" } }));
    fs.writeFileSync(path.join(zhDir, "test.json"), JSON.stringify({ a: "1", b: { c: "2" } }));
    expect(diffNamespace(enDir, zhDir, "test.json")).toEqual([]);
  });

  it("should report missing keys in zh-CN", () => {
    fs.writeFileSync(path.join(enDir, "extra.json"), JSON.stringify({ a: "1", b: "2" }));
    fs.writeFileSync(path.join(zhDir, "extra.json"), JSON.stringify({ a: "1" }));
    const problems = diffNamespace(enDir, zhDir, "extra.json");
    expect(problems.length).toBeGreaterThan(0);
    expect(problems.some((p) => p.includes("extra.json") && p.includes("missing in zh-CN"))).toBe(true);
    // Clean up
    fs.unlinkSync(path.join(enDir, "extra.json"));
    fs.unlinkSync(path.join(zhDir, "extra.json"));
  });

  it("should report extra keys in zh-CN", () => {
    fs.writeFileSync(path.join(enDir, "extra2.json"), JSON.stringify({ a: "1" }));
    fs.writeFileSync(path.join(zhDir, "extra2.json"), JSON.stringify({ a: "1", b: "2" }));
    const problems = diffNamespace(enDir, zhDir, "extra2.json");
    expect(problems.length).toBeGreaterThan(0);
    expect(problems.some((p) => p.includes("extra2.json") && p.includes("not in en"))).toBe(true);
    fs.unlinkSync(path.join(enDir, "extra2.json"));
    fs.unlinkSync(path.join(zhDir, "extra2.json"));
  });

  it("should report missing zh-CN file", () => {
    fs.writeFileSync(path.join(enDir, "lonely.json"), JSON.stringify({ a: "1" }));
    const problems = diffNamespace(enDir, zhDir, "lonely.json");
    expect(problems.length).toBeGreaterThan(0);
    expect(problems.some((p) => p.includes("missing zh-CN"))).toBe(true);
    fs.unlinkSync(path.join(enDir, "lonely.json"));
  });

  it("should report missing en file", () => {
    fs.writeFileSync(path.join(zhDir, "lonely.json"), JSON.stringify({ a: "1" }));
    const problems = diffNamespace(enDir, zhDir, "lonely.json");
    expect(problems.length).toBeGreaterThan(0);
    expect(problems.some((p) => p.includes("missing en"))).toBe(true);
    fs.unlinkSync(path.join(zhDir, "lonely.json"));
  });
});

// ─── runDiff (integration via temp fixture) ────────────────────────

describe("runDiff", () => {
  let tmpDir: string;

  beforeAll(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "i18n-key-diff-run-"));
    const enDir = path.join(tmpDir, "en");
    const zhDir = path.join(tmpDir, "zh-CN");
    fs.mkdirSync(enDir, { recursive: true });
    fs.mkdirSync(zhDir, { recursive: true });

    // 3 namespaces, all aligned
    fs.writeFileSync(path.join(enDir, "common.json"), JSON.stringify({ a: "1", b: { c: "2" } }));
    fs.writeFileSync(path.join(zhDir, "common.json"), JSON.stringify({ a: "1", b: { c: "2" } }));
    fs.writeFileSync(path.join(enDir, "logs.json"), JSON.stringify({ x: "y" }));
    fs.writeFileSync(path.join(zhDir, "logs.json"), JSON.stringify({ x: "y" }));
    fs.writeFileSync(path.join(enDir, "config.json"), JSON.stringify({ m: "n" }));
    fs.writeFileSync(path.join(zhDir, "config.json"), JSON.stringify({ m: "n" }));
  });

  afterAll(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it("should report ok when all aligned", () => {
    const result = runDiff(tmpDir);
    expect(result.ok).toBe(true);
    expect(result.problems).toEqual([]);
    expect(result.namespaces).toEqual(["common.json", "config.json", "logs.json"]);
  });

  it("should report problems when a key is added to en but not zh-CN", () => {
    // Add a key to en only — simulates "新增 key 后脚本能正确检出差异（红）"
    const enDir = path.join(tmpDir, "en");
    const commonPath = path.join(enDir, "common.json");
    const original = JSON.parse(fs.readFileSync(commonPath, "utf-8"));
    // Temporarily add a key
    fs.writeFileSync(commonPath, JSON.stringify({ ...original, new_key_en: "new value" }));

    const result = runDiff(tmpDir);
    expect(result.ok).toBe(false);
    expect(result.problems.length).toBeGreaterThan(0);
    expect(result.problems.some((p) => p.includes("common.json") && p.includes("missing in zh-CN") && p.includes("new_key_en"))).toBe(true);

    // Restore
    fs.writeFileSync(commonPath, JSON.stringify(original));
  });

  it("should report missing en directory", () => {
    const result = runDiff("/tmp/nonexistent-i18n-dir");
    expect(result.ok).toBe(false);
    expect(result.problems.length).toBeGreaterThan(0);
  });
});

// ─── CLI spawn (exit code verification) ────────────────────────────

describe("CLI integration", () => {
  it("should exit 0 on the real locale files (already aligned)", () => {
    const result = execSync(`node "${SCRIPT_PATH}"`, { encoding: "utf-8" });
    expect(result).toContain("OK");
  });

  it("should exit 1 and report missing key when a diff is artificially introduced", () => {
    // Create a temp fixtures dir with a mismatch
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "i18n-key-diff-cli-"));
    const enDir = path.join(tmpDir, "en");
    const zhDir = path.join(tmpDir, "zh-CN");
    fs.mkdirSync(enDir, { recursive: true });
    fs.mkdirSync(zhDir, { recursive: true });

    fs.writeFileSync(path.join(enDir, "test.json"), JSON.stringify({ a: "1", b: "2" }));
    fs.writeFileSync(path.join(zhDir, "test.json"), JSON.stringify({ a: "1" }));

    let threw = false;
    try {
      execSync(`node "${SCRIPT_PATH}" --locales-dir "${tmpDir}"`, { encoding: "utf-8" });
    } catch (e: any) {
      threw = true;
      expect(e.status).toBe(1);
      expect(e.stdout).toContain("FAIL");
      expect(e.stdout).toContain("missing in zh-CN");
    }
    expect(threw).toBe(true);

    fs.rmSync(tmpDir, { recursive: true, force: true });
  });
});

// ─── formatReport ──────────────────────────────────────────────────

describe("formatReport", () => {
  it("should format a passing report", () => {
    const text = formatReport(
      { ok: true, problems: [], namespaces: ["a.json", "b.json"], perNamespace: {} },
      "/some/dir",
    );
    expect(text).toContain("OK");
    expect(text).toContain("2 namespaces");
  });

  it("should format a failing report", () => {
    const text = formatReport(
      { ok: false, problems: ["[x.json] 1 key(s) in en but missing in zh-CN: foo.bar"], namespaces: ["x.json"], perNamespace: {} },
      "/some/dir",
    );
    expect(text).toContain("FAIL");
    expect(text).toContain("foo.bar");
  });
});
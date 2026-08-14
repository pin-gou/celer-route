import { describe, it, expect } from "vitest";
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
];

describe("i18n namespace sanity", () => {
  const languages = ["en", "zh-CN"];

  it.each(languages)(
    "should have exactly 7 namespace files for %s",
    (lang) => {
      const langDir = path.join(LOCALES_DIR, lang);

      // This assertion will fail in the red phase (locale dirs don't exist yet)
      expect(
        fs.existsSync(langDir),
        `Locale directory for "${lang}" should exist at ${langDir}`
      ).toBe(true);

      const files = fs
        .readdirSync(langDir)
        .filter((f) => f.endsWith(".json"))
        .sort();

      expect(files.length).toBe(7);

      // Verify each expected namespace file exists
      for (const ns of EXPECTED_NAMESPACES) {
        expect(
          files.includes(`${ns}.json`),
          `Expected namespace file "${ns}.json" to exist in ${langDir}`
        ).toBe(true);
      }
    }
  );

  it("should have identical namespace sets between en and zh-CN", () => {
    const enDir = path.join(LOCALES_DIR, "en");
    const zhDir = path.join(LOCALES_DIR, "zh-CN");

    expect(
      fs.existsSync(enDir),
      `en locale directory should exist at ${enDir}`
    ).toBe(true);
    expect(
      fs.existsSync(zhDir),
      `zh-CN locale directory should exist at ${zhDir}`
    ).toBe(true);

    const enFiles = fs
      .readdirSync(enDir)
      .filter((f) => f.endsWith(".json"))
      .sort();
    const zhFiles = fs
      .readdirSync(zhDir)
      .filter((f) => f.endsWith(".json"))
      .sort();

    expect(enFiles).toEqual(zhFiles);
  });

  it("should only contain the expected 7 namespaces (no extra files)", () => {
    const enDir = path.join(LOCALES_DIR, "en");

    expect(
      fs.existsSync(enDir),
      `en locale directory should exist at ${enDir}`
    ).toBe(true);

    const files = fs
      .readdirSync(enDir)
      .filter((f) => f.endsWith(".json"))
      .map((f) => f.replace(/\.json$/, ""))
      .sort();

    expect(files).toEqual(EXPECTED_NAMESPACES.sort());
  });
});
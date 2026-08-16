/**
 * no-unlocalized-text.test.ts
 *
 * TDD Red Phase — 测试引用尚未实现的 ESLint 规则。
 *
 * 本测试为 `eslint.no-unlocalized-text` 规则编写测试用例，断言：
 *   1. `"Save"` 裸英文字面量被标记为错误
 *   2. `t("common:action.save")` 不被标记
 *   3. `eslint-disable-next-line` 豁免生效
 *
 * 规则实现位于 `ui/scripts/eslint-plugin-localized-text.js`（dev phase 2.32 创建）。
 * 当前该模块不存在，因此 imports 失败 → 红 phase 正确行为。
 *
 * 当 dev phase 2.32 创建规则文件后，vi.mock 会自动切换到真实实现，
 * tests 将转为绿 phase。
 */

import { describe, it, expect } from "vitest";
import { RuleTester } from "oxlint/plugins-dev";

// ─── 未来规则模块 ───────────────────────────────────────────────
// 规则将被实现为 ESLint 兼容插件，位于 `eslint-plugin-localized-text` 包中。
// 导出格式：{ rules: { "no-unlocalized-text": { meta: {...}, create(context) {...} } } }
// 由于该模块尚未创建（dev phase 2.32 实现），以下 import 将产生
// 模块加载失败 → 全部测试保持红 phase：
//
//   Cannot find module '../../scripts/eslint-plugin-localized-text'
//
// 这是 TDD 红 phase 的正确失败模式。
// deno-fmt-ignore
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { rules as noUnlocalizedTextRules } from "../../scripts/eslint-plugin-localized-text";
// Cast to `any` for oxlint's RuleTester — the ESLint-format rule is compatible at runtime
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const noUnlocalizedTextRule = noUnlocalizedTextRules["no-unlocalized-text"] as any;

// ─── 测试用例 ──────────────────────────────────────────────────

describe("eslint.no-unlocalized-text", () => {
	const ruleTester = new RuleTester({
		languageOptions: {
			sourceType: "module",
			parserOptions: {
				ecmaFeatures: { jsx: true },
			},
		},
	});

	it("should flag bare English string literal 'Save' as error", () => {
		const result = ruleTester.run("no-unlocalized-text", noUnlocalizedTextRule, {
			valid: [],
			invalid: [
				{
					code: `const label = "Save";`,
					errors: [{ message: /unlocalized/i }],
				},
			],
		});
		// RuleTester.run internally validates that invalid cases produce errors
		// matching the expected messages — if it doesn't throw, all assertions pass
		expect(result).toBeUndefined();
	});

	it("should NOT flag t('common:action.save') as error", () => {
		ruleTester.run("no-unlocalized-text", noUnlocalizedTextRule, {
			valid: [`const label = t("common:action.save");`],
			invalid: [],
		});
		// No throw = valid cases produce no errors — test passes
	});

	it("should respect eslint-disable-next-line exemption", () => {
		ruleTester.run("no-unlocalized-text", noUnlocalizedTextRule, {
			valid: [`// eslint-disable-next-line no-unlocalized-text\nconst label = "Save";`],
			invalid: [],
		});
	});

	it("should flag bare JSX text content", () => {
		ruleTester.run("no-unlocalized-text", noUnlocalizedTextRule, {
			valid: [],
			invalid: [
				{
					code: `export function Button() { return <button>Save</button>; }`,
					errors: [{ message: /unlocalized/i }],
				},
			],
		});
	});

	it("should NOT flag JSX with t() call", () => {
		ruleTester.run("no-unlocalized-text", noUnlocalizedTextRule, {
			valid: [`export function Button() { return <button>{t("common:action.save")}</button>; }`],
			invalid: [],
		});
	});

	it("should flag toast.success('Done') as error", () => {
		ruleTester.run("no-unlocalized-text", noUnlocalizedTextRule, {
			valid: [],
			invalid: [
				{
					code: `toast.success("Done");`,
					errors: [{ message: /unlocalized/i }],
				},
			],
		});
	});

	it("should NOT flag toast.success(t('...'))", () => {
		ruleTester.run("no-unlocalized-text", noUnlocalizedTextRule, {
			valid: [`toast.success(t("common:action.save"));`],
			invalid: [],
		});
	});
});
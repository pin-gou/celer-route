import { describe, it, expect } from "vitest";
import fs from "fs";
import path from "path";

/**
 * @file TDD Red Phase — home freeTier i18n 键值对齐扫描（dev.ui task 6.4）
 *
 * 契约来源：tasks.md 7.7 —— 在 ui/locales/en/home.json 和 ui/locales/zh-CN/home.json
 * 新增 home 命名空间（freeTier.* 段，约 25 键：title / updatedAt / applyNow /
 * configureNow / noBundles / retry / recentRoutingRules / configSuccess /
 * configFailed / alreadyConfigured / keylessNote 等）。
 *
 * 红 phase 说明：en/home.json 与 zh-CN/home.json 尚未创建（生产交付物，由 dev
 * phase task 7.7 生成），本测试在 fs.existsSync 断言处失败——这是预期的 TDD 红结果。
 */

const LOCALES_DIR = path.resolve(__dirname, "../../../locales");
const HOME_NAMESPACE = "home";
const FREE_TIER_PREFIX = "freeTier";
// tasks.md 7.7 列出的核心键（子集，dev 必须实现）
const CORE_FREE_TIER_KEYS = ["title", "updatedAt", "noBundles", "retry", "configSuccess", "keylessNote"];

/**
 * 递归收集 JSON 对象的 "dot.path" 键集合。
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

/**
 * 读取某语种 home.json 的 freeTier.* 扁平键集合。
 * 文件不存在 → 断言失败（TDD 红 phase）。
 */
function readFreeTierKeys(lang: string): string[] {
	const filePath = path.join(LOCALES_DIR, lang, `${HOME_NAMESPACE}.json`);
	expect(fs.existsSync(filePath), `${lang}/${HOME_NAMESPACE}.json 不存在（TDD 红 phase —— home.json 尚未创建）`).toBe(true);
	const data = JSON.parse(fs.readFileSync(filePath, "utf-8")) as Record<string, unknown>;
	const freeTier = (data[FREE_TIER_PREFIX] ?? {}) as Record<string, unknown>;
	return flattenKeys(freeTier);
}

describe("home freeTier i18n 键值对齐", () => {
	it("en 与 zh-CN 的 freeTier.* 键集合应完全一致", () => {
		const enKeys = readFreeTierKeys("en");
		const zhKeys = readFreeTierKeys("zh-CN");

		const missingInZh = enKeys.filter((k) => !zhKeys.includes(k));
		expect(missingInZh, `zh-CN home.json 缺失 freeTier 键: ${missingInZh.join(", ")}`).toEqual([]);

		const extraInZh = zhKeys.filter((k) => !enKeys.includes(k));
		expect(extraInZh, `zh-CN home.json 多余 freeTier 键: ${extraInZh.join(", ")}`).toEqual([]);
	});

	it("freeTier.* 段非空且包含核心键", () => {
		const enKeys = readFreeTierKeys("en");
		expect(enKeys.length, "en home.json freeTier 段为空（TDD 红 phase）").toBeGreaterThan(0);

		for (const core of CORE_FREE_TIER_KEYS) {
			expect(enKeys, `en home.json freeTier 段缺少核心键 ${core}`).toContain(core);
		}
	});
});
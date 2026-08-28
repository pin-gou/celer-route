// @vitest-environment jsdom
/**
 * @file TDD Red Phase — FreeTierRecommendationCard 组件测试（dev.ui task 6.1）
 *
 * 契约来源：design.md「2. freeTierRecommendationCard.tsx 主卡」+「3. bundleApplyCard.tsx」
 *   - 主卡 data-testid="home-free-tier-card"
 *   - 每个 bundle 一个子卡 data-testid={`home-free-tier-bundle-${bundle.id}`}
 *   - 空 bundles 数组 → 空状态卡 data-testid="home-free-tier-empty"（含重试按钮 data-testid="home-free-tier-retry"）
 *   - 网络错误（data 为空 / error 存在）→ 同上空状态 + 重试按钮
 *   - 卡片底部展示最近路由规则（前 3 条：name / use_count / last_used_at）→ V-ui-2
 *
 * 红 phase 说明：freeTierRecommendationCard.tsx / catalogApi.ts / hooks 目录均未创建，
 * 本文件在 import 阶段即失败（Failed to resolve import）——这是预期的 TDD 红结果。
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import type { QueryStatus } from "@reduxjs/toolkit/query";
import FreeTierRecommendationCard from "./freeTierRecommendationCard";
import { useGetBundlesQuery } from "@/lib/store/apis/catalogApi";
import { useRecentRoutingRulesQuery } from "../hooks/useRecentRoutingRulesQuery";

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string) => key,
		i18n: { language: "zh-CN" },
	}),
}));

vi.mock("@/lib/store/apis/catalogApi", () => ({
	useGetBundlesQuery: vi.fn(),
}));

vi.mock("../hooks/useRecentRoutingRulesQuery", () => ({
	useRecentRoutingRulesQuery: vi.fn(),
}));

const mockGetBundles = vi.mocked(useGetBundlesQuery);
const mockRecentRules = vi.mocked(useRecentRoutingRulesQuery);

// RecentRoutingRulesResponse 形状（design.md GET /api/logs/recent-routing-rules）
interface RecentRoutingRule {
	id: string;
	name: string;
	last_used_at: string;
	use_count: number;
}

// 构造一个类型完备的 useRecentRoutingRulesQuery 返回对象。UseQueryHookResult 要求
// 完整的 status 标志位 + refetch + 成功分支的 error/fulfilledTimeStamp/currentData，
// 缺一不可，否则 tsc 报错。布尔标志用字面量断言保持 precise 类型以匹配判别联合。
const completeRecentRules = (rules: RecentRoutingRule[] = []) => ({
	data: { rules },
	status: "fulfilled" as QueryStatus,
	isUninitialized: false as const,
	isLoading: false as const,
	isFetching: false as const,
	isSuccess: true as const,
	isError: false as const,
	error: undefined,
	fulfilledTimeStamp: 0,
	currentData: { rules },
	refetch: vi.fn(),
});

// design.md bundleEntry / bundleProviderEntry 形状的 fixture
const bundleCoding = {
	id: "coding",
	title: "编程开发",
	description: "代码补全与调试首选",
	providers: [
		{
			provider: "openai",
			models: ["gpt-4o-mini", "gpt-4.1"],
			apply_url: "https://platform.openai.com/signup",
			apply_steps: ["注册账号", "申请 API Key", "回到此处填入"],
			is_keyless: false,
			notes: "新用户首月 $5 免费额度",
		},
		{
			provider: "opencode",
			models: ["default"],
			apply_url: "",
			apply_steps: [],
			is_keyless: true,
			notes: "免 Key, 直接添加",
		},
	],
};

const bundleWriting = {
	id: "writing",
	title: "写作助手",
	description: "文案润色与翻译",
	providers: [],
};

// design.md GET /api/logs/recent-routing-rules 响应形状
const rulePgMaster = {
	id: "rr-uuid-1",
	name: "pg-master",
	last_used_at: "2026-08-28T07:45:12Z",
	use_count: 42,
};

const ruleHermes = {
	id: "rr-uuid-2",
	name: "hermes-default",
	last_used_at: "2026-08-28T07:30:00Z",
	use_count: 18,
};

beforeEach(() => {
	mockGetBundles.mockReset();
	mockRecentRules.mockReset();
	// 默认：无最近路由规则，避免未设置时 undefined 报错
	mockRecentRules.mockReturnValue(completeRecentRules());
});

describe("FreeTierRecommendationCard", () => {
	it("should render a bundle card per bundle with title/description when bundles load", () => {
		mockGetBundles.mockReturnValue({
			data: {
				bundles: [bundleCoding, bundleWriting],
				updated_at: "2026-08-28T08:00:00Z",
				version: "2026-08-28",
			},
			isSuccess: true,
			refetch: vi.fn(),
		});

		render(<FreeTierRecommendationCard />);

		expect(screen.getByTestId("home-free-tier-card"), "主卡应渲染").toBeTruthy();
		expect(screen.getByTestId("home-free-tier-bundle-coding"), "coding bundle 子卡应渲染").toBeTruthy();
		expect(screen.getByTestId("home-free-tier-bundle-writing"), "writing bundle 子卡应渲染").toBeTruthy();
		expect(screen.getByText("编程开发"), "bundle title 应渲染").toBeTruthy();
		expect(screen.getByText("代码补全与调试首选"), "bundle description 应渲染").toBeTruthy();
	});

	it("should show recent routing rules with use_count and last_used_at in the bundle footer (V-ui-2)", () => {
		mockGetBundles.mockReturnValue({
			data: { bundles: [bundleCoding], updated_at: null, version: null },
			isSuccess: true,
			refetch: vi.fn(),
		});
		mockRecentRules.mockReturnValue(completeRecentRules([rulePgMaster, ruleHermes]));

		render(<FreeTierRecommendationCard />);

		expect(screen.getByText("pg-master"), "最近路由规则名应显示").toBeTruthy();
		expect(screen.getByText(/42/), "use_count 应显示").toBeTruthy();
		expect(screen.getByText(/2026-08-28T07:45:12Z/), "last_used_at 应显示").toBeTruthy();
	});

	it("should render empty state with a working retry button when bundles array is empty (V-ui-4)", () => {
		const retry = vi.fn();
		mockGetBundles.mockReturnValue({
			data: { bundles: [], updated_at: null, version: null },
			isSuccess: true,
			refetch: retry,
		});

		render(<FreeTierRecommendationCard />);

		expect(screen.getByTestId("home-free-tier-empty"), "空状态卡应渲染").toBeTruthy();
		expect(screen.getByTestId("home-free-tier-retry"), "重试按钮应渲染").toBeTruthy();
		expect(screen.queryByTestId("home-free-tier-card"), "空 bundles 时不渲染主卡").toBeNull();

		fireEvent.click(screen.getByTestId("home-free-tier-retry"));
		expect(retry, "点击重试应触发 refetch").toHaveBeenCalledTimes(1);
	});

	it("should render empty state with a working retry button when the fetch fails (V-ui-4)", () => {
		const retry = vi.fn();
		mockGetBundles.mockReturnValue({
			data: undefined,
			error: { status: 500 },
			isSuccess: false,
			refetch: retry,
		});

		render(<FreeTierRecommendationCard />);

		expect(screen.getByTestId("home-free-tier-empty"), "网络错误时也应显示空状态卡").toBeTruthy();
		expect(screen.getByTestId("home-free-tier-retry"), "网络错误时重试按钮应渲染").toBeTruthy();

		fireEvent.click(screen.getByTestId("home-free-tier-retry"));
		expect(retry, "点击重试应触发 refetch").toHaveBeenCalledTimes(1);
	});
});
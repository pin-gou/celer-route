// @vitest-environment jsdom
/**
 * @file TDD Red Phase — FreeTierRecommendationCard 组件测试（dev.ui task 6.1）
 *
 * 契约来源：design.md「2. freeTierRecommendationCard.tsx 主卡」+「3. bundleApplyCard.tsx」
 *   - 主卡 data-testid="home-free-tier-card"
 *   - 每个 bundle 一个子卡 data-testid={`home-free-tier-bundle-${bundle.id}`}
 *   - 空 bundles 数组 → 空状态卡 data-testid="home-free-tier-empty"（含重试按钮 data-testid="home-free-tier-retry"）
 *   - 网络错误（data 为空 / error 存在）→ 同上空状态 + 重试按钮
 *
 * 红 phase 说明：freeTierRecommendationCard.tsx / catalogApi.ts / hooks 目录均未创建，
 * 本文件在 import 阶段即失败（Failed to resolve import）——这是预期的 TDD 红结果。
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import type { ReactNode } from "react";
import FreeTierRecommendationCard from "./freeTierRecommendationCard";
import { useGetBundlesQuery } from "@/lib/store/apis/catalogApi";
import { useGetProvidersQuery } from "@/lib/store/apis/providersApi";

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string) => key,
		i18n: { language: "zh-CN" },
	}),
}));

vi.mock("@/lib/store/apis/catalogApi", () => ({
	useGetBundlesQuery: vi.fn(),
}));

vi.mock("@/lib/store/apis/providersApi", () => ({
	useGetProvidersQuery: vi.fn(),
}));

// Link 脱离 RouterProvider 会因缺少路由上下文而抛错，用 <a> 桩替身以满足
// 已配置 provider 行跳转文案的断言（href 按 /workspace/providers/:id 拼装）。
vi.mock("@tanstack/react-router", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@tanstack/react-router")>();
	const StubLink = ({
		to,
		params,
		children,
		...anchor
	}: {
		to: string;
		params?: Record<string, string>;
		children: unknown;
		"data-testid"?: string;
		className?: string;
	}) => {
		const href = to.includes("$id") && params ? to.replace("$id", params.id) : to;
		return (
			<a href={href} {...anchor}>
				{children as ReactNode}
			</a>
		);
	};
	return { ...actual, Link: StubLink };
});

const mockGetBundles = vi.mocked(useGetBundlesQuery);
const mockGetProviders = vi.mocked(useGetProvidersQuery);

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
			free_valid_until: "2026-09-06",
		},
	],
};

const bundleWriting = {
	id: "writing",
	title: "写作助手",
	description: "文案润色与翻译",
	providers: [],
};

beforeEach(() => {
	mockGetBundles.mockReset();
	mockGetProviders.mockReset();
	// 默认无已配置 providers
	mockGetProviders.mockReturnValue({ data: [], isSuccess: true, refetch: vi.fn() });
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
		expect(screen.getByTestId("home-free-tier-valid-until-opencode"), "应渲染免费有效期徽章").toBeTruthy();
		expect(screen.getByText("编程开发"), "bundle title 应渲染").toBeTruthy();
		expect(screen.getByText("代码补全与调试首选"), "bundle description 应渲染").toBeTruthy();
	});

	it("should mark already-configured providers, link them to the detail page and sort them to the end", () => {
		mockGetBundles.mockReturnValue({
			data: { bundles: [bundleCoding], updated_at: null, version: null },
			isSuccess: true,
			refetch: vi.fn(),
		});
		// openai 未配置（保留"一键配置"），opencode 已配置（标注 + 跳转详情页）
		mockGetProviders.mockReturnValue({
			data: [{ name: "opencode" }],
			isSuccess: true,
			refetch: vi.fn(),
		});

		render(<FreeTierRecommendationCard />);

		const providerRows = screen.getAllByTestId(/home-free-tier-provider-coding-/);
		expect(providerRows.length, "应渲染两个 provider 行").toBe(2);
		expect(providerRows[0].textContent, "未配置的 openai 应排在前").toContain("openai");
		expect(providerRows[0].getAttribute("href"), "未配置行不是链接").toBeNull();
		expect(providerRows[1].textContent, "已配置的 opencode 应排在后").toContain("opencode");
		expect(providerRows[1].getAttribute("href"), "已配置行应链接到提供商详情页").toBe("/workspace/providers/opencode");
		expect(screen.getByTestId("home-free-tier-configured-opencode"), "已配置 provider 应带'已配置'标注").toBeTruthy();
		expect(screen.getByTestId("home-free-tier-status-opencode"), "已配置 provider 应显示健康状态点").toBeTruthy();
		expect(screen.queryByTestId("home-free-tier-configure-opencode"), "已配置 provider 不应显示一键配置按钮").toBeNull();
		expect(screen.queryByTestId("home-free-tier-configure-openai"), "未配置 provider 应保留一键配置按钮").toBeTruthy();
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

	it("should render unsupported providers greyed out with a hint and no configure button", () => {
		mockGetBundles.mockReturnValue({
			data: {
				bundles: [
					{
						...bundleCoding,
						providers: [
							{
								provider: "acme",
								models: ["acme-1"],
								apply_url: "https://acme.ai",
								apply_steps: [],
								is_keyless: false,
								notes: "",
								supported: false,
							},
						],
					},
				],
				updated_at: null,
				version: null,
			},
			isSuccess: true,
			refetch: vi.fn(),
		});

		render(<FreeTierRecommendationCard />);

		expect(screen.getByTestId("home-free-tier-provider-coding-acme"), "不支持的卡片仍应保留展示").toBeTruthy();
		expect(screen.getByTestId("home-free-tier-unsupported-acme"), "应渲染当前版本不支持提示").toBeTruthy();
		expect(screen.queryByTestId("home-free-tier-configure-acme"), "不应提供一键配置入口").toBeNull();
		expect(screen.getByTestId("home-free-tier-apply-acme"), "去申请外链应保留").toBeTruthy();
	});

	it("should show a protocol badge and keep the configure button for custom-fallback providers", () => {
		mockGetBundles.mockReturnValue({
			data: {
				bundles: [
					{
						...bundleCoding,
						providers: [
							{
								provider: "together",
								models: ["m1"],
								apply_url: "",
								apply_steps: [],
								is_keyless: false,
								notes: "",
								base_provider: "openai",
								base_url: "https://api.together.xyz/v1",
								supported: true,
							},
						],
					},
				],
				updated_at: null,
				version: null,
			},
			isSuccess: true,
			refetch: vi.fn(),
		});

		render(<FreeTierRecommendationCard />);

		expect(screen.getByTestId("home-free-tier-protocol-together"), "应渲染基于 openai 协议徽章").toBeTruthy();
		expect(screen.getByTestId("home-free-tier-configure-together"), "自定义兜底提供商仍应可一键配置").toBeTruthy();
	});
});
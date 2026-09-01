// @vitest-environment jsdom
/**
 * @file TDD Red Phase — FreeTierOneKeyConfigDialog 组件测试（dev.ui task 6.2）
 *
 * 契约来源：design.md「4. freeTierOneKeyConfigDialog.tsx 弹窗」
 *   - Props：{ open: boolean; provider: BundleProviderEntry | null; onOpenChange?: (open: boolean) => void }
 *   - API Key 输入框 data-testid="home-free-tier-key-input"，提交按钮 data-testid="home-free-tier-submit"
 *   - 提交流程：
 *       1) createProvider({ provider })（来自 catalogApi.useCreateProviderMutation）
 *       2) 抛 { status: 409 } → 已存在，继续创建 key；其他错误 → 重新抛出（不继续）
 *       3) 非 keyless 且填了 key → createKey({ provider, key })；keyless → 跳过 createKey
 *       4) 成功后 toast.success(t('freeTier.configSuccess')) + catalogApi.util.invalidateTags(['CatalogBundles'])
 *
 * 红 phase 说明：freeTierOneKeyConfigDialog.tsx / catalogApi.ts 均未创建，
 * 本文件在 import 阶段即失败（Failed to resolve import）——这是预期的 TDD 红结果。
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import FreeTierOneKeyConfigDialog from "./freeTierOneKeyConfigDialog";
import { DefaultNetworkConfig } from "@/lib/constants/config";
import { useOneClickCreateProviderMutation, useOneClickCreateProviderKeyMutation, catalogApi } from "@/lib/store/apis/catalogApi";

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string) => key,
		i18n: { language: "zh-CN" },
	}),
}));

vi.mock("sonner", () => ({
	toast: { success: vi.fn() },
}));

vi.mock("@/lib/store/apis/catalogApi", () => ({
	useOneClickCreateProviderMutation: vi.fn(),
	useOneClickCreateProviderKeyMutation: vi.fn(),
	catalogApi: { util: { invalidateTags: vi.fn() } },
}));

import { toast } from "sonner";

const mockCreateProvider = vi.mocked(useOneClickCreateProviderMutation);
const mockCreateKey = vi.mocked(useOneClickCreateProviderKeyMutation);
const mockInvalidateTags = vi.mocked(catalogApi.util.invalidateTags);
const mockToastSuccess = vi.mocked(toast.success);

// design.md bundleProviderEntry 形状的 fixture
const openaiProvider = {
	provider: "openai",
	models: ["gpt-4o-mini", "gpt-4.1"],
	apply_url: "https://platform.openai.com/signup",
	apply_steps: ["注册账号", "申请 API Key", "回到此处填入"],
	is_keyless: false,
	notes: "新用户首月 $5 免费额度",
};

const opencodeProvider = {
	provider: "opencode",
	models: ["default"],
	apply_url: "",
	apply_steps: [],
	is_keyless: true,
	notes: "免 Key，直接添加",
};

// 非内建提供商（服务端标注 supported + custom-provider 兜底字段）
const togetherProvider = {
	provider: "together",
	models: ["meta-llama/Llama-3.3-70B-Instruct-Turbo-Free"],
	apply_url: "https://api.together.ai/",
	apply_steps: ["注册账号"],
	is_keyless: false,
	notes: "开源模型免费额度",
	base_provider: "openai",
	base_url: "https://api.together.xyz/v1",
	supported: true,
};

// 非内建 + keyless（is_key_less 走 custom_provider_config）
const pollinationsProvider = {
	provider: "pollinations",
	models: ["openai"],
	apply_url: "",
	apply_steps: [],
	is_keyless: true,
	notes: "免 Key，直接添加",
	base_provider: "openai",
	base_url: "https://text.pollinations.ai/openai",
	supported: true,
};

// RTK Query mutation trigger mocks. The component awaits `trigger(args).unwrap()`,
// so each trigger must return a real Promise augmented with the action-creator
// fields (unwrap/reset/abort/arg/requestId) expected by MutationActionCreatorResult.
type ProviderTrigger = ReturnType<typeof useOneClickCreateProviderMutation>[0];
type ProviderKeyTrigger = ReturnType<typeof useOneClickCreateProviderKeyMutation>[0];
type ProviderState = ReturnType<typeof useOneClickCreateProviderMutation>[1];
type KeyState = ReturnType<typeof useOneClickCreateProviderKeyMutation>[1];

const triggerBase = (unwrap: () => Promise<unknown>) =>
	Object.assign(Promise.resolve({ data: null, error: undefined }), {
		arg: { endpointName: "mock", originalArgs: undefined },
		requestId: "mock-request-id",
		abort: () => {},
		unwrap,
		reset: () => {},
	});

const resolveTrigger = <T extends (...args: never[]) => unknown>(value: unknown) =>
	vi.fn(() => triggerBase(() => Promise.resolve(value))) as unknown as T;
const rejectTrigger = <T extends (...args: never[]) => unknown>(err: unknown) =>
	vi.fn(() => triggerBase(() => Promise.reject(err))) as unknown as T;

// UseMutationStateResult requires a `reset` callback alongside the status flags.
const resolvedState = { isLoading: false, reset: vi.fn() } as ProviderState;
const keyResolvedState = { isLoading: false, reset: vi.fn() } as KeyState;

beforeEach(() => {
	mockCreateProvider.mockReset();
	mockCreateKey.mockReset();
	mockInvalidateTags.mockReset();
	mockToastSuccess.mockReset();
});

describe("FreeTierOneKeyConfigDialog", () => {
	it("should create provider then key on success (POST providers 200 → POST keys 200)", async () => {
		const createProvider = resolveTrigger<ProviderTrigger>({ name: "openai" });
		const createKey = resolveTrigger<ProviderKeyTrigger>({ provider: "openai", key_id: "k-1" });
		mockCreateProvider.mockReturnValue([createProvider, resolvedState]);
		mockCreateKey.mockReturnValue([createKey, keyResolvedState]);

		render(<FreeTierOneKeyConfigDialog open provider={openaiProvider} onOpenChange={() => {}} />);

		fireEvent.change(screen.getByTestId("home-free-tier-key-input"), { target: { value: "sk-test-123" } });
		fireEvent.click(screen.getByTestId("home-free-tier-submit"));

		await waitFor(() => expect(createProvider).toHaveBeenCalledWith({ provider: "openai" }));
		await waitFor(() => expect(createKey).toHaveBeenCalledWith({ provider: "openai", key: "sk-test-123" }));
		expect(mockToastSuccess, "成功后应弹成功 toast").toHaveBeenCalledWith("freeTier.configSuccess");
		expect(mockInvalidateTags, "成功后应失效 CatalogBundles 缓存").toHaveBeenCalledWith(["CatalogBundles"]);
	});

	it("should continue to create key when provider already exists (POST providers 409)", async () => {
		const createProvider = rejectTrigger<ProviderTrigger>({ status: 409 });
		const createKey = resolveTrigger<ProviderKeyTrigger>({ provider: "openai", key_id: "k-2" });
		mockCreateProvider.mockReturnValue([createProvider, resolvedState]);
		mockCreateKey.mockReturnValue([createKey, keyResolvedState]);

		render(<FreeTierOneKeyConfigDialog open provider={openaiProvider} onOpenChange={() => {}} />);

		fireEvent.change(screen.getByTestId("home-free-tier-key-input"), { target: { value: "sk-test-456" } });
		fireEvent.click(screen.getByTestId("home-free-tier-submit"));

		await waitFor(() => expect(createProvider).toHaveBeenCalledWith({ provider: "openai" }));
		await waitFor(() => expect(createKey).toHaveBeenCalledWith({ provider: "openai", key: "sk-test-456" }));
		expect(mockToastSuccess, "409 后流程仍应成功").toHaveBeenCalledWith("freeTier.configSuccess");
	});

	it("should skip key creation for keyless providers", async () => {
		const createProvider = resolveTrigger<ProviderTrigger>({ name: "opencode" });
		const createKey = vi.fn();
		mockCreateProvider.mockReturnValue([createProvider, resolvedState]);
		mockCreateKey.mockReturnValue([createKey, keyResolvedState]);

		render(<FreeTierOneKeyConfigDialog open provider={opencodeProvider} onOpenChange={() => {}} />);

		fireEvent.click(screen.getByTestId("home-free-tier-submit"));

		await waitFor(() => expect(createProvider).toHaveBeenCalledWith({ provider: "opencode" }));
		expect(createKey, "keyless provider 不应调用 createKey").not.toHaveBeenCalled();
		expect(mockToastSuccess, "keyless 成功后也应弹成功 toast").toHaveBeenCalledWith("freeTier.configSuccess");
	});

	it("should create custom-fallback providers with custom_provider_config + network_config", async () => {
		const createProvider = resolveTrigger<ProviderTrigger>({ name: "together" });
		const createKey = resolveTrigger<ProviderKeyTrigger>({ provider: "together", key_id: "k-3" });
		mockCreateProvider.mockReturnValue([createProvider, resolvedState]);
		mockCreateKey.mockReturnValue([createKey, keyResolvedState]);

		render(<FreeTierOneKeyConfigDialog open provider={togetherProvider} onOpenChange={() => {}} />);

		expect(screen.getByTestId("home-free-tier-custom-hint"), "自定义提供商提示应渲染").toBeTruthy();

		fireEvent.change(screen.getByTestId("home-free-tier-key-input"), { target: { value: "sk-together" } });
		fireEvent.click(screen.getByTestId("home-free-tier-submit"));

		await waitFor(() =>
			expect(createProvider).toHaveBeenCalledWith({
				provider: "together",
				custom_provider_config: { base_provider_type: "openai", is_key_less: false },
				network_config: { ...DefaultNetworkConfig, base_url: "https://api.together.xyz/v1" },
			}),
		);
		await waitFor(() => expect(createKey).toHaveBeenCalledWith({ provider: "together", key: "sk-together" }));
		expect(mockToastSuccess, "自定义提供商成功后应弹成功 toast").toHaveBeenCalledWith("freeTier.configSuccess");
	});

	it("should skip key creation for keyless custom providers (is_key_less=true)", async () => {
		const createProvider = resolveTrigger<ProviderTrigger>({ name: "pollinations" });
		const createKey = vi.fn();
		mockCreateProvider.mockReturnValue([createProvider, resolvedState]);
		mockCreateKey.mockReturnValue([createKey, keyResolvedState]);

		render(<FreeTierOneKeyConfigDialog open provider={pollinationsProvider} onOpenChange={() => {}} />);

		fireEvent.click(screen.getByTestId("home-free-tier-submit"));

		await waitFor(() =>
			expect(createProvider).toHaveBeenCalledWith({
				provider: "pollinations",
				custom_provider_config: { base_provider_type: "openai", is_key_less: true },
				network_config: { ...DefaultNetworkConfig, base_url: "https://text.pollinations.ai/openai" },
			}),
		);
		expect(createKey, "keyless 自定义提供商不应调用 createKey").not.toHaveBeenCalled();
	});
});
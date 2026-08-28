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
import { useCreateProviderMutation, useCreateProviderKeyMutation, catalogApi } from "@/lib/store/apis/catalogApi";

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
	useCreateProviderMutation: vi.fn(),
	useCreateProviderKeyMutation: vi.fn(),
	catalogApi: { util: { invalidateTags: vi.fn() } },
}));

import { toast } from "sonner";

const mockCreateProvider = vi.mocked(useCreateProviderMutation);
const mockCreateKey = vi.mocked(useCreateProviderKeyMutation);
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
	notes: "免 Key, 直接添加",
};

// RTK Query mutation trigger mocks. The component awaits `trigger(args).unwrap()`,
// so each trigger must return a real Promise augmented with the action-creator
// fields (unwrap/reset/abort/arg/requestId) expected by MutationActionCreatorResult.
type ProviderTrigger = ReturnType<typeof useCreateProviderMutation>[0];
type ProviderKeyTrigger = ReturnType<typeof useCreateProviderKeyMutation>[0];
type ProviderState = ReturnType<typeof useCreateProviderMutation>[1];
type KeyState = ReturnType<typeof useCreateProviderKeyMutation>[1];

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
});
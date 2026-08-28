// @vitest-environment jsdom
/**
 * @file TDD Red Phase — useRecentRoutingRulesQuery hook 测试（dev.ui task 6.3）
 *
 * 契约来源：design.md「5. useRecentRoutingRulesQuery.ts」
 *   export const useRecentRoutingRulesQuery = (args: { limit?: number }) =>
 *     useGetRecentRoutingRulesQuery({ limit: args.limit ?? 100 });
 *
 *   - 未传 limit → 透传 { limit: 100 }（默认值）
 *   - 传 limit → 透传 { limit: N }
 *   - 返回结构 = useGetRecentRoutingRulesQuery 的返回值（rules 数组原样透传）
 *
 * 红 phase 说明：useRecentRoutingRulesQuery.ts / catalogApi.ts 均未创建，
 * 本文件在 import 阶段即失败（Failed to resolve import）——这是预期的 TDD 红结果。
 */
import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useRecentRoutingRulesQuery } from "./useRecentRoutingRulesQuery";
import { useGetRecentRoutingRulesQuery } from "@/lib/store/apis/catalogApi";

vi.mock("@/lib/store/apis/catalogApi", () => ({
	useGetRecentRoutingRulesQuery: vi.fn(),
}));

const mockGetRecentRoutingRules = vi.mocked(useGetRecentRoutingRulesQuery);

// design.md GET /api/logs/recent-routing-rules 响应形状
const rulesFixture = {
	rules: [
		{ id: "rr-uuid-1", name: "pg-master", last_used_at: "2026-08-28T07:45:12Z", use_count: 42 },
		{ id: "rr-uuid-2", name: "hermes-default", last_used_at: "2026-08-28T07:30:00Z", use_count: 18 },
	],
};

// UseQueryHookResult 要求 refetch（UseQuerySubscriptionResult），缺了 tsc 报错。
const queryResultFixture = { data: rulesFixture, isLoading: false, isFetching: false, refetch: vi.fn() };

describe("useRecentRoutingRulesQuery", () => {
	it("should default limit to 100 when not provided", () => {
		mockGetRecentRoutingRules.mockReturnValue(queryResultFixture);
		renderHook(() => useRecentRoutingRulesQuery({}));
		expect(mockGetRecentRoutingRules).toHaveBeenCalledWith({ limit: 100 });
	});

	it("should pass through an explicit limit", () => {
		mockGetRecentRoutingRules.mockReturnValue(queryResultFixture);
		renderHook(() => useRecentRoutingRulesQuery({ limit: 5 }));
		expect(mockGetRecentRoutingRules).toHaveBeenCalledWith({ limit: 5 });
	});

	it("should return the underlying query result structure (rules passthrough)", () => {
		mockGetRecentRoutingRules.mockReturnValue(queryResultFixture);
		const { result } = renderHook(() => useRecentRoutingRulesQuery({ limit: 100 }));
		expect(result.current).toEqual(queryResultFixture);
		expect(result.current.data!.rules[0]).toMatchObject({
			id: "rr-uuid-1",
			name: "pg-master",
			last_used_at: "2026-08-28T07:45:12Z",
			use_count: 42,
		});
	});
});
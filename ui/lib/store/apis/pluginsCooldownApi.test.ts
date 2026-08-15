// @vitest-environment jsdom
/**
 * @file TDD Red Phase — RTK Query cooldown API endpoint tests (dev.ui task 11.3)
 *
 * Contract (design.md "API 设计"):
 *   - pluginsApi extends with 3 endpoints:
 *       1. getCooldownState  → GET    /plugins/provider-cooldown/state
 *       2. getCooldownStats  → GET    /plugins/provider-cooldown/stats
 *       3. unfreezeCooldown  → DELETE /plugins/provider-cooldown/state/{provider}/{keyId}
 *   - Exported hooks: useGetCooldownStateQuery, useGetCooldownStatsQuery, useUnfreezeCooldownMutation
 *   - Response shapes:
 *       getCooldownState:  { state: Array<{ provider, keyId, expireAt, reason }> }
 *       getCooldownStats:  { stats: { markCount, suppressedCount, activeCount } }
 *       unfreezeCooldown:  { message, provider, keyId }
 *
 * In the TDD red phase these endpoints are not yet registered on pluginsApi —
 * the import of the new hooks will fail with "does not provide an export named",
 * and the endpoint definitions will be undefined. This is the expected TDD
 * red-phase result.
 */

import { beforeEach, describe, expect, it, vi, afterEach } from "vitest";
import { configureStore } from "@reduxjs/toolkit";

// ---------------------------------------------------------------------------
// Red phase: the following hooks are not yet exported from pluginsApi.
// In the TDD red phase this import will fail at load time.
// ---------------------------------------------------------------------------
import {
	pluginsApi,
	useGetCooldownStateQuery,
	useGetCooldownStatsQuery,
	useUnfreezeCooldownMutation,
} from "./pluginsApi";

// ---------------------------------------------------------------------------
// Types for response shape assertions (contract from design.md)
// ---------------------------------------------------------------------------

interface CooldownStateEntry {
	provider: string;
	keyId: string;
	expireAt: string;
	reason: string;
}

interface CooldownStats {
	markCount: number;
	suppressedCount: number;
	activeCount: number;
}

interface CooldownStateResponse {
	state: CooldownStateEntry[];
}

interface CooldownStatsResponse {
	stats: CooldownStats;
}

interface UnfreezeCooldownResponse {
	message: string;
	provider: string;
	keyId: string;
}

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

let fetchMock: ReturnType<typeof vi.fn>;

const jsonResponse = (body: unknown) =>
	new Response(JSON.stringify(body), {
		status: 200,
		headers: { "Content-Type": "application/json" },
	});

const stateBody: CooldownStateResponse = {
	state: [
		{ provider: "openai", keyId: "key-abc-123", expireAt: "2026-08-15T18:00:00Z", reason: "quota_exhausted" },
	],
};

const statsBody: CooldownStatsResponse = {
	stats: { markCount: 12, suppressedCount: 8, activeCount: 3 },
};

const unfreezeBody: UnfreezeCooldownResponse = {
	message: "key unfrozen",
	provider: "openai",
	keyId: "key-abc-123",
};

function createApiStore() {
	return configureStore({
		reducer: { [pluginsApi.reducerPath]: pluginsApi.reducer },
		middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(pluginsApi.middleware),
	});
}

function getRequestUrl(callIndex: number): string {
	const firstArg = fetchMock.mock.calls[callIndex][0];
	if (typeof firstArg === "string") return firstArg;
	if (firstArg && typeof firstArg === "object" && "url" in firstArg) return firstArg.url as string;
	return String(firstArg);
}

function getRequestMethod(callIndex: number): string | undefined {
	const firstArg = fetchMock.mock.calls[callIndex][0];
	if (typeof firstArg === "object" && firstArg && "method" in firstArg) return firstArg.method as string;
	return undefined;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("pluginsApi cooldown endpoint registration (task 11.3)", () => {
	beforeEach(() => {
		fetchMock = vi.fn();
		vi.stubGlobal("fetch", fetchMock);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	// -----------------------------------------------------------------------
	// Endpoint existence
	// -----------------------------------------------------------------------

	it("registers getCooldownState endpoint on pluginsApi", () => {
		expect(pluginsApi.endpoints.getCooldownState).toBeDefined();
	});

	it("registers getCooldownStats endpoint on pluginsApi", () => {
		expect(pluginsApi.endpoints.getCooldownStats).toBeDefined();
	});

	it("registers unfreezeCooldown endpoint on pluginsApi", () => {
		expect(pluginsApi.endpoints.unfreezeCooldown).toBeDefined();
	});

	// -----------------------------------------------------------------------
	// Hook exports
	// -----------------------------------------------------------------------

	it("exports useGetCooldownStateQuery hook", () => {
		expect(useGetCooldownStateQuery).toBeDefined();
		expect(typeof useGetCooldownStateQuery).toBe("function");
	});

	it("exports useGetCooldownStatsQuery hook", () => {
		expect(useGetCooldownStatsQuery).toBeDefined();
		expect(typeof useGetCooldownStatsQuery).toBe("function");
	});

	it("exports useUnfreezeCooldownMutation hook", () => {
		expect(useUnfreezeCooldownMutation).toBeDefined();
		expect(typeof useUnfreezeCooldownMutation).toBe("function");
	});

	// -----------------------------------------------------------------------
	// getCooldownState — request shape & response type
	// -----------------------------------------------------------------------

	it("getCooldownState issues GET /plugins/provider-cooldown/state", async () => {
		fetchMock.mockResolvedValue(jsonResponse(stateBody));
		const store = createApiStore();

		const data = (await store.dispatch(
			pluginsApi.endpoints.getCooldownState.initiate(undefined),
		).unwrap()) as CooldownStateResponse;

		expect(data.state).toHaveLength(1);
		expect(data.state[0]).toMatchObject({
			provider: "openai",
			keyId: "key-abc-123",
			reason: "quota_exhausted",
		});

		const url = getRequestUrl(0);
		expect(url).toContain("/plugins/provider-cooldown/state");

		const method = getRequestMethod(0);
		expect(method).toBe("GET");
	});

	// -----------------------------------------------------------------------
	// getCooldownStats — request shape & response type
	// -----------------------------------------------------------------------

	it("getCooldownStats issues GET /plugins/provider-cooldown/stats", async () => {
		fetchMock.mockResolvedValue(jsonResponse(statsBody));
		const store = createApiStore();

		const data = (await store.dispatch(
			pluginsApi.endpoints.getCooldownStats.initiate(undefined),
		).unwrap()) as CooldownStatsResponse;

		expect(data.stats).toMatchObject({
			markCount: 12,
			suppressedCount: 8,
			activeCount: 3,
		});

		const url = getRequestUrl(0);
		expect(url).toContain("/plugins/provider-cooldown/stats");

		const method = getRequestMethod(0);
		expect(method).toBe("GET");
	});

	// -----------------------------------------------------------------------
	// unfreezeCooldown — request shape & response type
	// -----------------------------------------------------------------------

	it("unfreezeCooldown issues DELETE /plugins/provider-cooldown/state/{provider}/{keyId}", async () => {
		fetchMock.mockResolvedValue(jsonResponse(unfreezeBody));
		const store = createApiStore();

		const result = (await store.dispatch(
			pluginsApi.endpoints.unfreezeCooldown.initiate({ provider: "openai", keyId: "key-abc-123" }),
		).unwrap()) as UnfreezeCooldownResponse;

		expect(result).toMatchObject({
			message: "key unfrozen",
			provider: "openai",
			keyId: "key-abc-123",
		});

		const url = getRequestUrl(0);
		expect(url).toContain("/plugins/provider-cooldown/state/openai/key-abc-123");

		const method = getRequestMethod(0);
		expect(method).toBe("DELETE");
	});
});
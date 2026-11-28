/**
 * Pins the client-side mirror of the Go dimension classifier.
 *
 * The vectors below are the same ones `plugins/providercooldown/dimension_test.go`
 * and the error-patterns handler test use, so a divergence between the
 * preview (this module) and the authoritative server-side decision fails here
 * instead of silently mislabelling a rule in the UI.
 */
import { describe, expect, it } from "vitest";
import { isTokenBudgetDimension, limiterDimensionOfMatch, matchLimiterDimension } from "@/lib/utils/limiterDimension";

describe("matchLimiterDimension", () => {
	const vectors: Array<[string, string]> = [
		// Provider codes observed in production.
		["endpointrpmexceeded rate_limit_error rpm exhausted", "rpm"],
		["endpointtpmexceeded rate_limit_error tpm exhausted", "tpm"],
		["modelaccountrpmratelimitexceeded", "rpm"],
		["modelaccounttpmratelimitexceeded", "tpm"],
		// Message forms.
		["tpm exhausted", "tpm"],
		["rps exhausted", "rpm"],
		["requestbursttoofast", "rpm"],
		["throttling.burstrate", "rpm"],
		["token rate limit exceeded", "tpm"],
		["tokens per minute budget reached", "tpm"],
		// Generic fallback.
		["rate limit exceeded", "rpm"],
		// The guard: a generic phrase next to a token signal must NOT become rpm.
		["rate limit exceeded, token budget exceeded", "unknown"],
		// Token patterns beat request patterns regardless of order in the body.
		["modelaccounttpmratelimitexceeded rpm rate limit reached", "tpm"],
		// No signal at all.
		["upstream boom", "unknown"],
	];

	for (const [haystack, want] of vectors) {
		it(`${haystack} → ${want}`, () => {
			expect(matchLimiterDimension(haystack)).toBe(want);
		});
	}
});

describe("limiterDimensionOfMatch", () => {
	it("uses the match's own predicates", () => {
		expect(limiterDimensionOfMatch({ code: ["EndpointRPMExceeded"] })).toBe("rpm");
		expect(limiterDimensionOfMatch({ code: ["EndpointTPMExceeded"] })).toBe("tpm");
		expect(limiterDimensionOfMatch({ type: ["rate_limit_error"], message_contains: ["tpm exhausted"] })).toBe("tpm");
	});

	it("returns unknown for a bare status-code catch-all", () => {
		expect(limiterDimensionOfMatch({})).toBe("unknown");
	});
});

describe("isTokenBudgetDimension", () => {
	it("excludes rpm samples from the token estimate", () => {
		expect(isTokenBudgetDimension("rpm")).toBe(false);
		expect(isTokenBudgetDimension("tpm")).toBe(true);
		expect(isTokenBudgetDimension("unknown")).toBe(true);
	});
});
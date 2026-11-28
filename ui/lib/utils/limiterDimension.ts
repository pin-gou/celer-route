/**
 * Client-side mirror of the provider-cooldown rate-limit dimension classifier.
 *
 * The authoritatATIVE decision is always made server-side
 * (`plugins/providercooldown/dimension.go`): at mark time from the live
 * provider error, and per error-pattern bucket via
 * `LimiterDimensionOfFields` (returned as `limiter_dimension` on
 * `/api/logs/error-patterns`). This module exists so the match editor can
 * PREVIEW what "automatic" would decide while the operator is typing, without
 * a network round-trip per keystroke.
 *
 * If the two ever disagree, the server wins — the preview is advisory.
 *
 * KEEP IN SYNC with `limiterDimensionPatterns` in
 * `plugins/providercooldown/dimension.go`. Both sides are pinned by tests
 * driven by the same vectors (that file's `TestLimiterDimension*` and
 * `limiterDimension.test.ts`), so a divergence fails a test rather than
 * silently mislabelling traffic.
 */

export type LimiterDimension = "rpm" | "tpm" | "unknown";

interface LimiterDimensionPattern {
	dimension: LimiterDimension;
	pattern: string;
	/** Banned substrings: the pattern is skipped when any is present. */
	notContains?: string[];
}

/** Ordered matcher table; first hit wins, so specific patterns come first. */
export const LIMITER_DIMENSION_PATTERNS: readonly LimiterDimensionPattern[] = [
	// Token-rate patterns (checked before request-rate so a body carrying a
	// generic rate-limit phrase plus a token signal resolves to tpm).
	{ dimension: "tpm", pattern: "endpointtpmexceeded" },
	{ dimension: "tpm", pattern: "modelaccounttpmratelimitexceeded" },
	{ dimension: "tpm", pattern: "tpm exhausted" },
	{ dimension: "tpm", pattern: "token rate limit" },
	{ dimension: "tpm", pattern: "tokens per minute" },

	// Request-rate patterns (rps folds into rpm: same recovery dynamics).
	{ dimension: "rpm", pattern: "endpointrpmexceeded" },
	{ dimension: "rpm", pattern: "modelaccountrpmratelimitexceeded" },
	{ dimension: "rpm", pattern: "rpm exhausted" },
	{ dimension: "rpm", pattern: "rps exhausted" },
	{ dimension: "rpm", pattern: "requestbursttoofast" },
	{ dimension: "rpm", pattern: "throttling.burstrate" },

	// Generic fallback: request-rate only when nothing token-related appears.
	{ dimension: "rpm", pattern: "rate limit exceeded", notContains: ["token", "tpm"] },
];

/** Applies the ordered table to an already lower-cased haystack. */
export function matchLimiterDimension(haystack: string): LimiterDimension {
	for (const p of LIMITER_DIMENSION_PATTERNS) {
		if (!haystack.includes(p.pattern)) continue;
		if (p.notContains?.some((banned) => haystack.includes(banned))) continue;
		return p.dimension;
	}
	return "unknown";
}

/**
 * Classifies a match's own predicates. `message_contains` holds the substrings
 * the error message must contain, so joining them is the closest stand-in for
 * the message itself — good enough for a preview, and the cases that matter
 * (`EndpointRPMExceeded`, `tpm exhausted`, …) are carried by `code`/`type`
 * verbatim.
 */
export function limiterDimensionOfMatch(match: { code?: string[]; type?: string[]; message_contains?: string[] }): LimiterDimension {
	const haystack = [...(match.code ?? []), ...(match.type ?? []), ...(match.message_contains ?? [])].join(" ").toLowerCase();
	return matchLimiterDimension(haystack);
}

/**
 * Same predicate the learner uses: an RPM rejection must never contribute to
 * the token-limit estimate, so a match pinned to rpm is excluded there too.
 */
export function isTokenBudgetDimension(dimension: LimiterDimension): boolean {
	return dimension !== "rpm";
}
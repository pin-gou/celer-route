// @vitest-environment jsdom
//
// Pins the D6 兜底 contract (plan.md §7) at the parse/submit boundary:
// when the admin types only blacklisted models into the form, the
// submit path prepends allowed_models: ["*"] so the resolver still
// passes everything except the blacklist (an empty allowed_models list
// is deny-by-default and would silently forbid every model of that
// provider — the opposite of what the admin almost certainly meant).
import { describe, it, expect } from "vitest";
import {
	parseCsv,
	d6Guard,
} from "@/app/workspace/governance/teams/$teamId/views/teamModelPoliciesSection";

describe("teamModelPoliciesSection helpers — D6 兜底", () => {
	it("parseCsv splits comma- and newline-separated values, drops blanks, dedupes case-insensitively", () => {
		const out = parseCsv("a, B, a\nc, ", "  b ,B, c\n\n");
		// dedupe is case-insensitive — the first-seen entry wins, so the
		// lowercase "b" beats "B" in the blacklisted column.
		expect(out.allowed_models).toEqual(["a", "B", "c"]);
		expect(out.blacklisted_models).toEqual(["b", "c"]);
	});

	it("d6Guard: blacklisted-only submission gets allowed_models auto-filled to [\"*\"]", () => {
		const guarded = d6Guard({ allowed_models: [], blacklisted_models: ["o1"] }, null);
		expect(guarded.allowed_models).toEqual(["*"]);
		expect(guarded.blacklisted_models).toEqual(["o1"]);
	});

	it("d6Guard: explicit allowed list is passed through unchanged", () => {
		const guarded = d6Guard({ allowed_models: ["gpt-4o-mini"], blacklisted_models: ["o1"] }, null);
		expect(guarded).toEqual({ allowed_models: ["gpt-4o-mini"], blacklisted_models: ["o1"] });
	});

	it("d6Guard: empty-everything submission stays empty (admin wants to clear)", () => {
		const guarded = d6Guard({ allowed_models: [], blacklisted_models: [] }, null);
		expect(guarded).toEqual({ allowed_models: [], blacklisted_models: [] });
	});

	it("d6Guard: blacklisted-only with no toast funcs still returns the safe body", () => {
		const guarded = d6Guard({ allowed_models: [], blacklisted_models: ["o1"] }, null);
		expect(guarded.allowed_models).toEqual(["*"]);
	});
});
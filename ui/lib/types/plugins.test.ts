// @vitest-environment jsdom
/**
 * @file TDD Red Phase — rtkConfigSchema (zod) validation logic (dev.ui task 11.2)
 *
 * Contract (design.md "API 设计"):
 *   rtkConfigSchema validates the RTK plugin config form:
 *   - pipeline: array of {id: string, config?: unknown} with default [{id:"rtk"}]
 *   - min_tokens_to_compress: non-negative integer with default 0
 *   - plus all existing RTK config fields (intensity, max_lines_per_result, etc.)
 *
 * In the TDD red phase, rtkConfigSchema is not yet exported from plugins.ts —
 * the import will fail with "does not provide an export named rtkConfigSchema".
 * This is the expected TDD red-phase result.
 */

import { describe, expect, it } from "vitest";

// ---------------------------------------------------------------------------
// Red phase: rtkConfigSchema is not yet exported from plugins.ts.
// In the TDD red phase this import will fail at load time.
// ---------------------------------------------------------------------------
import { rtkConfigSchema } from "./plugins";

// ---------------------------------------------------------------------------
// Type for the expected shape
// ---------------------------------------------------------------------------

interface RTKConfig {
	pipeline?: Array<{ id: string; config?: unknown }>;
	min_tokens_to_compress?: number;
	intensity?: string;
	max_lines_per_result?: number;
	max_chars_per_result?: number;
	dedup_threshold?: number;
	raw_output_retention?: string;
	enabled?: boolean;
}

// ---------------------------------------------------------------------------
// Helper: parse a config through the schema and return the parsed result
// ---------------------------------------------------------------------------

function parseConfig(input: unknown) {
	return rtkConfigSchema.parse(input);
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("rtkConfigSchema (task 11.2)", () => {
	// -----------------------------------------------------------------------
	// Schema existence
	// -----------------------------------------------------------------------

	it("exports rtkConfigSchema as a ZodObject", () => {
		expect(rtkConfigSchema).toBeDefined();
		expect(rtkConfigSchema.constructor.name).toBe("ZodObject");
	});

	// -----------------------------------------------------------------------
	// pipeline field
	// -----------------------------------------------------------------------

	it("accepts pipeline as an array of objects with id (string)", () => {
		const result = parseConfig({
			pipeline: [{ id: "rtk" }],
		});
		expect(result.pipeline).toHaveLength(1);
		expect(result.pipeline[0].id).toBe("rtk");
	});

	it("accepts pipeline with optional config field", () => {
		const result = parseConfig({
			pipeline: [{ id: "rtk", config: { some: "value" } }],
		});
		expect(result.pipeline).toHaveLength(1);
		expect(result.pipeline[0].id).toBe("rtk");
		expect(result.pipeline[0].config).toEqual({ some: "value" });
	});

	it("accepts pipeline with multiple engines", () => {
		const result = parseConfig({
			pipeline: [{ id: "rtk" }, { id: "llmlingua" }],
		});
		expect(result.pipeline).toHaveLength(2);
		expect(result.pipeline[1].id).toBe("llmlingua");
	});

	it('defaults pipeline to [{id:"rtk"}] when omitted', () => {
		const result = parseConfig({});
		expect(result.pipeline).toBeDefined();
		expect(result.pipeline).toHaveLength(1);
		expect(result.pipeline[0].id).toBe("rtk");
	});

	it("rejects pipeline when id is not a string", () => {
		expect(() =>
			parseConfig({
				pipeline: [{ id: 42 }],
			}),
		).toThrow();
	});

	it("rejects pipeline when pipeline is not an array", () => {
		expect(() =>
			parseConfig({
				pipeline: "not-an-array",
			}),
		).toThrow();
	});

	// -----------------------------------------------------------------------
	// min_tokens_to_compress field
	// -----------------------------------------------------------------------

	it("accepts min_tokens_to_compress as a non-negative integer", () => {
		const result = parseConfig({ min_tokens_to_compress: 500 });
		expect(result.min_tokens_to_compress).toBe(500);
	});

	it("accepts min_tokens_to_compress = 0 (no skip)", () => {
		const result = parseConfig({ min_tokens_to_compress: 0 });
		expect(result.min_tokens_to_compress).toBe(0);
	});

	it("defaults min_tokens_to_compress to 0 when omitted", () => {
		const result = parseConfig({});
		expect(result.min_tokens_to_compress).toBe(0);
	});

	it("rejects negative min_tokens_to_compress", () => {
		expect(() => parseConfig({ min_tokens_to_compress: -1 })).toThrow();
	});

	it("rejects non-integer min_tokens_to_compress", () => {
		expect(() => parseConfig({ min_tokens_to_compress: 3.14 })).toThrow();
	});

	it("rejects string min_tokens_to_compress", () => {
		expect(() => parseConfig({ min_tokens_to_compress: "500" })).toThrow();
	});

	// -----------------------------------------------------------------------
	// Combined valid config
	// -----------------------------------------------------------------------

	it("accepts a full valid config with all fields", () => {
		const result = parseConfig({
			pipeline: [{ id: "rtk" }],
			min_tokens_to_compress: 1000,
			intensity: "standard",
			max_lines_per_result: 50,
			max_chars_per_result: 2000,
			dedup_threshold: 5,
			raw_output_retention: "all",
			enabled: true,
		});
		expect(result.pipeline).toHaveLength(1);
		expect(result.min_tokens_to_compress).toBe(1000);
		expect(result.intensity).toBe("standard");
		expect(result.max_lines_per_result).toBe(50);
		expect(result.max_chars_per_result).toBe(2000);
		expect(result.dedup_threshold).toBe(5);
		expect(result.raw_output_retention).toBe("all");
		expect(result.enabled).toBe(true);
	});
});
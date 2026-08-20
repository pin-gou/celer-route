// @vitest-environment jsdom
/**
 * @file TDD Red Phase — compatibility + logging_headers validation tests (dev.ui task 11.3)
 *
 * Contract (design.md "数据模型" + "组件设计"):
 *   - compatConfigSchema:
 *       convert_text_to_chat:       boolean, default true
 *       convert_chat_to_responses:  boolean, default true
 *       should_drop_params:         boolean, default true
 *       should_convert_params:      boolean, default false
 *   - loggingConfigSchema:
 *       disable_content_logging:                     boolean (optional)
 *       retain_content_in_object_storage:            boolean (optional)
 *       allow_per_request_content_storage_override:  boolean (optional)
 *       logging_headers:                             string[] (optional, validated as array of strings)
 *
 * In the TDD red phase these schemas are not yet exported from plugins.ts —
 * the import will fail with "does not provide an export named ...".
 * This is the expected TDD red-phase result.
 */

import { describe, expect, it } from "vitest";

// ---------------------------------------------------------------------------
// Red phase: these schemas are not yet exported from plugins.ts.
// In the TDD red phase this import will fail at load time.
// ---------------------------------------------------------------------------
import { compatConfigSchema, loggingConfigSchema } from "../plugins";

// ---------------------------------------------------------------------------
// compatConfigSchema — compatibility field defaults
// ---------------------------------------------------------------------------

describe("compatConfigSchema — 4-field defaults (task 11.3)", () => {
	it("exports compatConfigSchema as a ZodObject", () => {
		expect(compatConfigSchema).toBeDefined();
		expect(compatConfigSchema.constructor.name).toBe("ZodObject");
	});

	// -----------------------------------------------------------------------
	// Default values: 3 fields true, 1 field false
	// -----------------------------------------------------------------------

	it("defaults convert_text_to_chat to true when omitted", () => {
		const result = compatConfigSchema.parse({});
		expect(result.convert_text_to_chat).toBe(true);
	});

	it("defaults convert_chat_to_responses to true when omitted", () => {
		const result = compatConfigSchema.parse({});
		expect(result.convert_chat_to_responses).toBe(true);
	});

	it("defaults should_drop_params to true when omitted", () => {
		const result = compatConfigSchema.parse({});
		expect(result.should_drop_params).toBe(true);
	});

	it("defaults should_convert_params to false when omitted", () => {
		const result = compatConfigSchema.parse({});
		expect(result.should_convert_params).toBe(false);
	});

	// -----------------------------------------------------------------------
	// Explicit values override defaults
	// -----------------------------------------------------------------------

	it("accepts explicit false for convert_text_to_chat", () => {
		const result = compatConfigSchema.parse({ convert_text_to_chat: false });
		expect(result.convert_text_to_chat).toBe(false);
	});

	it("accepts explicit true for should_convert_params", () => {
		const result = compatConfigSchema.parse({ should_convert_params: true });
		expect(result.should_convert_params).toBe(true);
	});

	it("accepts a config with all 4 fields set explicitly", () => {
		const result = compatConfigSchema.parse({
			convert_text_to_chat: false,
			convert_chat_to_responses: true,
			should_drop_params: false,
			should_convert_params: true,
		});
		expect(result.convert_text_to_chat).toBe(false);
		expect(result.convert_chat_to_responses).toBe(true);
		expect(result.should_drop_params).toBe(false);
		expect(result.should_convert_params).toBe(true);
	});

	// -----------------------------------------------------------------------
	// Type rejection
	// -----------------------------------------------------------------------

	it("rejects non-boolean values for convert_text_to_chat", () => {
		const result = compatConfigSchema.safeParse({ convert_text_to_chat: "true" });
		expect(result.success).toBe(false);
	});

	it("rejects non-boolean values for should_convert_params", () => {
		const result = compatConfigSchema.safeParse({ should_convert_params: 1 });
		expect(result.success).toBe(false);
	});

	it("strips unknown extra fields (non-strict object)", () => {
		const result = compatConfigSchema.parse({ convert_text_to_chat: true, unknown_field: "value" });
		expect(result.convert_text_to_chat).toBe(true);
		expect("unknown_field" in result).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// loggingConfigSchema — logging_headers array validation
// ---------------------------------------------------------------------------

describe("loggingConfigSchema — logging_headers array validation (task 11.3)", () => {
	it("exports loggingConfigSchema as a ZodObject", () => {
		expect(loggingConfigSchema).toBeDefined();
		expect(loggingConfigSchema.constructor.name).toBe("ZodObject");
	});

	// -----------------------------------------------------------------------
	// Happy path
	// -----------------------------------------------------------------------

	it("accepts logging_headers as an array of strings", () => {
		const result = loggingConfigSchema.safeParse({
			logging_headers: ["x-bf-vk", "x-bf-request-id"],
		});
		expect(result.success).toBe(true);
	});

	it("accepts logging_headers as an empty array", () => {
		const result = loggingConfigSchema.safeParse({ logging_headers: [] });
		expect(result.success).toBe(true);
	});

	it("accepts a config with all 4 logging fields", () => {
		const result = loggingConfigSchema.safeParse({
			disable_content_logging: false,
			retain_content_in_object_storage: true,
			allow_per_request_content_storage_override: false,
			logging_headers: ["x-bf-vk"],
		});
		expect(result.success).toBe(true);
	});

	// -----------------------------------------------------------------------
	// Invalid logging_headers
	// -----------------------------------------------------------------------

	it("rejects logging_headers when it is not an array", () => {
		const result = loggingConfigSchema.safeParse({ logging_headers: "x-bf-vk" });
		expect(result.success).toBe(false);
	});

	it("rejects logging_headers when it contains non-string elements", () => {
		const result = loggingConfigSchema.safeParse({ logging_headers: ["x-bf-vk", 42] });
		expect(result.success).toBe(false);
	});

	it("rejects logging_headers when it is null", () => {
		const result = loggingConfigSchema.safeParse({ logging_headers: null });
		expect(result.success).toBe(false);
	});

	// -----------------------------------------------------------------------
	// Other fields
	// -----------------------------------------------------------------------

	it("accepts disable_content_logging as a boolean", () => {
		const result = loggingConfigSchema.safeParse({ disable_content_logging: true });
		expect(result.success).toBe(true);
	});

	it("rejects disable_content_logging when it is not a boolean", () => {
		const result = loggingConfigSchema.safeParse({ disable_content_logging: "yes" });
		expect(result.success).toBe(false);
	});

	it("accepts retain_content_in_object_storage as a boolean", () => {
		const result = loggingConfigSchema.safeParse({ retain_content_in_object_storage: false });
		expect(result.success).toBe(true);
	});

	it("accepts allow_per_request_content_storage_override as a boolean", () => {
		const result = loggingConfigSchema.safeParse({ allow_per_request_content_storage_override: true });
		expect(result.success).toBe(true);
	});

	// -----------------------------------------------------------------------
	// Partial / empty config
	// -----------------------------------------------------------------------

	it("accepts an empty object (all fields optional)", () => {
		const result = loggingConfigSchema.safeParse({});
		expect(result.success).toBe(true);
	});

	it("accepts a single-field partial object", () => {
		const result = loggingConfigSchema.safeParse({ logging_headers: ["x-custom"] });
		expect(result.success).toBe(true);
	});
});
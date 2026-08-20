// @vitest-environment jsdom
/**
 * @file TDD Red Phase — 4 new Zod schema validation tests (dev.ui task 11.1)
 *
 * Contract (design.md "数据模型"):
 *   - loggingConfigSchema:
 *       disable_content_logging:                     boolean (optional)
 *       retain_content_in_object_storage:            boolean (optional)
 *       allow_per_request_content_storage_override:  boolean (optional)
 *       logging_headers:                             string[] (optional)
 *   - semanticCacheConfigSchema:
 *       provider:                       string (optional)
 *       embedding_model:                string (optional, required when provider set)
 *       dimension:                      number >= 1 (>= 2 when provider set)
 *       ttl:                            union<string, number> (optional)
 *       threshold:                      number 0-1 (optional)
 *       vector_store_namespace:         string (optional)
 *       default_cache_key:              string (optional)
 *       conversation_history_threshold: number >= 0 (optional)
 *       cache_by_model:                 boolean, default true
 *       cache_by_provider:              boolean, default true
 *       exclude_system_prompt:          boolean, default false
 *   - mockerConfigSchema:
 *       global_latency:     { min: string, max: string, type: "fixed"|"uniform" } (optional)
 *       rules:              array of Rule objects (optional)
 *       default_behavior:   enum<"passthrough"|"error"|"success"> (optional)
 *   - compatConfigSchema:
 *       convert_text_to_chat:       boolean, default true
 *       convert_chat_to_responses:  boolean, default true
 *       should_drop_params:         boolean, default true
 *       should_convert_params:      boolean, default false
 *
 * In the TDD red phase these 4 schemas are not yet exported from plugins.ts —
 * the import will fail with "does not provide an export named ...".
 * This is the expected TDD red-phase result.
 */

import { describe, expect, it } from "vitest";

// ---------------------------------------------------------------------------
// Red phase: these schemas are not yet exported from plugins.ts.
// In the TDD red phase this import will fail at load time.
// ---------------------------------------------------------------------------
import { loggingConfigSchema, semanticCacheConfigSchema, mockerConfigSchema, compatConfigSchema } from "@/lib/types/plugins";

// ---------------------------------------------------------------------------
// loggingConfigSchema tests
// ---------------------------------------------------------------------------

describe("loggingConfigSchema (task 11.1)", () => {
	it("exports loggingConfigSchema as a ZodObject", () => {
		expect(loggingConfigSchema).toBeDefined();
		expect(loggingConfigSchema.constructor.name).toBe("ZodObject");
	});

	it("accepts a valid config with all 4 fields", () => {
		const result = loggingConfigSchema.safeParse({
			disable_content_logging: false,
			retain_content_in_object_storage: true,
			allow_per_request_content_storage_override: false,
			logging_headers: ["x-bf-vk", "x-bf-request-id"],
		});
		expect(result.success).toBe(true);
	});

	it("accepts an empty object (all fields optional)", () => {
		const result = loggingConfigSchema.safeParse({});
		expect(result.success).toBe(true);
	});

	it("accepts a single-field partial object", () => {
		const result = loggingConfigSchema.safeParse({ disable_content_logging: true });
		expect(result.success).toBe(true);
	});

	it("rejects disable_content_logging when type is not boolean", () => {
		const result = loggingConfigSchema.safeParse({ disable_content_logging: "yes" });
		expect(result.success).toBe(false);
	});

	it("rejects logging_headers when it is not an array", () => {
		const result = loggingConfigSchema.safeParse({ logging_headers: "x-bf-vk" });
		expect(result.success).toBe(false);
	});

	it("accepts logging_headers as an empty array", () => {
		const result = loggingConfigSchema.safeParse({ logging_headers: [] });
		expect(result.success).toBe(true);
	});

	it("rejects retain_content_in_object_storage when type is not boolean", () => {
		const result = loggingConfigSchema.safeParse({ retain_content_in_object_storage: 1 });
		expect(result.success).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// semanticCacheConfigSchema tests
// ---------------------------------------------------------------------------

describe("semanticCacheConfigSchema (task 11.1)", () => {
	it("exports semanticCacheConfigSchema as a ZodObject", () => {
		expect(semanticCacheConfigSchema).toBeDefined();
		expect(semanticCacheConfigSchema.constructor.name).toBe("ZodObject");
	});

	it("accepts a valid config with all fields", () => {
		const result = semanticCacheConfigSchema.safeParse({
			provider: "openai",
			embedding_model: "text-embedding-3-small",
			dimension: 1536,
			ttl: "5m",
			threshold: 0.8,
			vector_store_namespace: "prod",
			default_cache_key: "default",
			conversation_history_threshold: 3,
			cache_by_model: true,
			cache_by_provider: true,
			exclude_system_prompt: false,
		});
		expect(result.success).toBe(true);
	});

	it("accepts an empty object (all fields optional)", () => {
		const result = semanticCacheConfigSchema.safeParse({});
		expect(result.success).toBe(true);
	});

	// -----------------------------------------------------------------------
	// dimension boundary values
	// -----------------------------------------------------------------------

	it("accepts dimension = 1 when provider is not set", () => {
		const result = semanticCacheConfigSchema.safeParse({ dimension: 1 });
		expect(result.success).toBe(true);
	});

	it("accepts dimension = 2 when provider is set", () => {
		const result = semanticCacheConfigSchema.safeParse({ provider: "openai", embedding_model: "text-embedding-3-small", dimension: 2 });
		expect(result.success).toBe(true);
	});

	it("rejects dimension = 1 when provider is set (>= 2 required)", () => {
		const result = semanticCacheConfigSchema.safeParse({ provider: "openai", embedding_model: "text-embedding-3-small", dimension: 1 });
		expect(result.success).toBe(false);
	});

	it("rejects dimension < 1", () => {
		const result = semanticCacheConfigSchema.safeParse({ dimension: 0 });
		expect(result.success).toBe(false);
	});

	it("rejects dimension when it is not an integer", () => {
		const result = semanticCacheConfigSchema.safeParse({ dimension: 1.5 });
		expect(result.success).toBe(false);
	});

	// -----------------------------------------------------------------------
	// provider — when set, embedding_model should be required
	// -----------------------------------------------------------------------

	it("accepts provider with embedding_model set", () => {
		const result = semanticCacheConfigSchema.safeParse({ provider: "openai", embedding_model: "text-embedding-3-small" });
		expect(result.success).toBe(true);
	});

	it("rejects provider with empty embedding_model", () => {
		// provider is set but embedding_model is empty string — should fail
		const result = semanticCacheConfigSchema.safeParse({ provider: "openai", embedding_model: "" });
		expect(result.success).toBe(false);
	});

	// -----------------------------------------------------------------------
	// threshold: 0-1 range
	// -----------------------------------------------------------------------

	it("accepts threshold = 0", () => {
		const result = semanticCacheConfigSchema.safeParse({ threshold: 0 });
		expect(result.success).toBe(true);
	});

	it("accepts threshold = 1", () => {
		const result = semanticCacheConfigSchema.safeParse({ threshold: 1 });
		expect(result.success).toBe(true);
	});

	it("rejects threshold > 1", () => {
		const result = semanticCacheConfigSchema.safeParse({ threshold: 1.5 });
		expect(result.success).toBe(false);
	});

	it("rejects threshold < 0", () => {
		const result = semanticCacheConfigSchema.safeParse({ threshold: -0.1 });
		expect(result.success).toBe(false);
	});

	// -----------------------------------------------------------------------
	// conversation_history_threshold >= 0
	// -----------------------------------------------------------------------

	it("accepts conversation_history_threshold = 0", () => {
		const result = semanticCacheConfigSchema.safeParse({ conversation_history_threshold: 0 });
		expect(result.success).toBe(true);
	});

	it("rejects conversation_history_threshold < 0", () => {
		const result = semanticCacheConfigSchema.safeParse({ conversation_history_threshold: -1 });
		expect(result.success).toBe(false);
	});

	// -----------------------------------------------------------------------
	// boolean defaults
	// -----------------------------------------------------------------------

	it("defaults cache_by_model to true when omitted", () => {
		const result = semanticCacheConfigSchema.parse({});
		expect(result.cache_by_model).toBe(true);
	});

	it("defaults cache_by_provider to true when omitted", () => {
		const result = semanticCacheConfigSchema.parse({});
		expect(result.cache_by_provider).toBe(true);
	});

	it("defaults exclude_system_prompt to false when omitted", () => {
		const result = semanticCacheConfigSchema.parse({});
		expect(result.exclude_system_prompt).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// mockerConfigSchema tests
// ---------------------------------------------------------------------------

describe("mockerConfigSchema (task 11.1)", () => {
	it("exports mockerConfigSchema as a ZodObject", () => {
		expect(mockerConfigSchema).toBeDefined();
		expect(mockerConfigSchema.constructor.name).toBe("ZodObject");
	});

	it("accepts a valid config with all fields", () => {
		const result = mockerConfigSchema.safeParse({
			global_latency: { min: "100ms", max: "500ms", type: "uniform" },
			rules: [
				{
					name: "gpt4-mock",
					conditions: { model: "gpt-4", provider: "openai" },
					responses: [{ status_code: 200, body: { choices: [{ message: { content: "mock" } }] } }],
					priority: 10,
					probability: 0.5,
				},
			],
			default_behavior: "passthrough",
		});
		expect(result.success).toBe(true);
	});

	it("accepts an empty object (all fields optional)", () => {
		const result = mockerConfigSchema.safeParse({});
		expect(result.success).toBe(true);
	});

	it("accepts default_behavior = 'error'", () => {
		const result = mockerConfigSchema.safeParse({ default_behavior: "error" });
		expect(result.success).toBe(true);
	});

	it("accepts default_behavior = 'success'", () => {
		const result = mockerConfigSchema.safeParse({ default_behavior: "success" });
		expect(result.success).toBe(true);
	});

	it("rejects default_behavior with an invalid enum value", () => {
		const result = mockerConfigSchema.safeParse({ default_behavior: "invalid" });
		expect(result.success).toBe(false);
	});

	it("rejects global_latency when it is not an object", () => {
		const result = mockerConfigSchema.safeParse({ global_latency: "fast" });
		expect(result.success).toBe(false);
	});

	it("rejects global_latency with invalid type enum", () => {
		const result = mockerConfigSchema.safeParse({ global_latency: { min: "100ms", max: "500ms", type: "unknown" } });
		expect(result.success).toBe(false);
	});

	it("accepts global_latency with only min and max (type defaults)", () => {
		const result = mockerConfigSchema.safeParse({ global_latency: { min: "100ms", max: "500ms" } });
		expect(result.success).toBe(true);
	});

	it("rejects rules when it is not an array", () => {
		const result = mockerConfigSchema.safeParse({ rules: "not-an-array" });
		expect(result.success).toBe(false);
	});

	it("rejects illegal JSON by rejecting non-object input", () => {
		// Parsing a JSON string instead of an object — treats the string as
		// the whole config value, which Zod should reject.
		const result = mockerConfigSchema.safeParse("invalid json content");
		expect(result.success).toBe(false);
	});

	it("rejects null input (illegal JSON equivalent)", () => {
		const result = mockerConfigSchema.safeParse(null);
		expect(result.success).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// compatConfigSchema tests
// ---------------------------------------------------------------------------

describe("compatConfigSchema (task 11.1)", () => {
	it("exports compatConfigSchema as a ZodObject", () => {
		expect(compatConfigSchema).toBeDefined();
		expect(compatConfigSchema.constructor.name).toBe("ZodObject");
	});

	it("accepts a valid config with all 4 fields", () => {
		const result = compatConfigSchema.safeParse({
			convert_text_to_chat: true,
			convert_chat_to_responses: false,
			should_drop_params: true,
			should_convert_params: false,
		});
		expect(result.success).toBe(true);
	});

	it("accepts an empty object (all fields optional)", () => {
		const result = compatConfigSchema.safeParse({});
		expect(result.success).toBe(true);
	});

	// -----------------------------------------------------------------------
	// Default values
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
	// Type rejection
	// -----------------------------------------------------------------------

	it("rejects convert_text_to_chat when type is not boolean", () => {
		const result = compatConfigSchema.safeParse({ convert_text_to_chat: "yes" });
		expect(result.success).toBe(false);
	});

	it("rejects convert_chat_to_responses when type is not boolean", () => {
		const result = compatConfigSchema.safeParse({ convert_chat_to_responses: 1 });
		expect(result.success).toBe(false);
	});

	it("rejects should_drop_params when type is not boolean", () => {
		const result = compatConfigSchema.safeParse({ should_drop_params: "true" });
		expect(result.success).toBe(false);
	});

	it("rejects should_convert_params when type is not boolean", () => {
		const result = compatConfigSchema.safeParse({ should_convert_params: 0 });
		expect(result.success).toBe(false);
	});

	it("strips unknown extra fields (non-strict object)", () => {
		const result = compatConfigSchema.parse({ convert_text_to_chat: true, unknown_field: "value" });
		expect(result.convert_text_to_chat).toBe(true);
		expect("unknown_field" in result).toBe(false);
	});
});
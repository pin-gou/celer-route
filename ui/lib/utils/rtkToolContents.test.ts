import { describe, expect, it } from "vitest";

import type { LogEntry } from "@/lib/types/logs";

import { extractCompressedToolContents } from "./rtkToolContents";

const hint = "[rtk\u2011recovery] celer\u2011route (LLM gateway) RTK compression can truncate tool_result blocks.";

function chatLog(input_history: unknown[], metadata?: Record<string, unknown>): LogEntry {
	return { input_history, metadata } as unknown as LogEntry;
}

function responsesLog(responses_input_history: unknown[], metadata?: Record<string, unknown>): LogEntry {
	return { responses_input_history, metadata } as unknown as LogEntry;
}

describe("extractCompressedToolContents", () => {
	it("numbers chat tool messages by their hint-free position (no hint, no offset)", () => {
		const log = chatLog([
			{ role: "system", content: "sys" },
			{ role: "user", content: "run tests" },
			{ role: "assistant", content: "" },
			{ role: "tool", content: "output A" },
		]);

		expect(extractCompressedToolContents(log)).toEqual([{ index: 3, content: "output A" }]);
	});

	it("subtracts the authoritative rtk_input_hint_offset when the hint shifts positions", () => {
		const log = chatLog(
			[
				{ role: "system", content: hint },
				{ role: "system", content: "sys" },
				{ role: "user", content: "run tests" },
				{ role: "assistant", content: "" },
				{ role: "tool", content: "output A" },
			],
			{ rtk_input_hint_offset: 1 },
		);

		// The tool sits at raw position 4; the authoritative (hint-free) index
		// recorded by the RTK pipeline is 3.
		expect(extractCompressedToolContents(log)).toEqual([{ index: 3, content: "output A" }]);
	});

	it("falls back to raw positions when the offset metadata is absent (legacy logs)", () => {
		const log = chatLog([
			{ role: "system", content: hint },
			{ role: "system", content: "sys" },
			{ role: "user", content: "run tests" },
			{ role: "assistant", content: "" },
			{ role: "tool", content: "output A" },
		]);

		// No offset recorded → 0 subtracted; the tool keeps its stored position.
		expect(extractCompressedToolContents(log)).toEqual([{ index: 4, content: "output A" }]);
	});

	it("numbers responses function_call_output items with the responses offset", () => {
		const log = responsesLog(
			[
				{ type: "message", role: "system", content: hint },
				{ type: "message", role: "user", content: "run tests" },
				{ type: "function_call_output", call_id: "call_1", output: "output B" },
			],
			{ rtk_responses_input_hint_offset: 1 },
		);

		expect(extractCompressedToolContents(log)).toEqual([{ index: 1, content: "output B" }]);
	});

	it("ignores non-tool messages and empty outputs", () => {
		const log = chatLog([
			{ role: "system", content: "sys" },
			{ role: "tool", content: "" },
			{ role: "user", content: "hi" },
			{ role: "tool", content: ["text not a string"] },
			{ role: "tool", content: [{ type: "text", text: "keep me" }] },
		]);

		expect(extractCompressedToolContents(log)).toEqual([{ index: 4, content: "keep me" }]);
	});
});
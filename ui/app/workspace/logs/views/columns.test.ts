import { describe, expect, it } from "vitest";

import type { LogEntry } from "@/lib/types/logs";

import { getMessage, truncateByWidth } from "./columns";

describe("getMessage", () => {
	it("returns EI realtime text from input history", () => {
		const log = {
			object: "realtime.turn",
			input_history: [
				{
					role: "user",
					content: [{ type: "text", text: "hello from the browser" }],
				},
			],
		} as unknown as LogEntry;

		expect(getMessage(log)).toBe("User: hello from the browser");
	});

	it("returns LM realtime text from output message", () => {
		const log = {
			object: "realtime.turn",
			input_history: [],
			responses_input_history: [],
			output_message: {
				role: "assistant",
				content: [{ type: "text", text: "hello from the model" }],
			},
		} as unknown as LogEntry;

		expect(getMessage(log)).toBe("Assistant: hello from the model");
	});

	it("returns split realtime text when both user and assistant are present", () => {
		const log = {
			object: "realtime.turn",
			input_history: [
				{
					role: "user",
					content: [{ type: "text", text: "who are you?" }],
				},
			],
			output_message: {
				role: "assistant",
				content: [{ type: "text", text: "I am the assistant." }],
			},
		} as unknown as LogEntry;

		expect(getMessage(log)).toBe("User: who are you?\nAssistant: I am the assistant.");
	});

	it("returns split realtime text including tool output", () => {
		const log = {
			object: "realtime.turn",
			input_history: [
				{
					role: "tool",
					content: [{ type: "text", text: '{"nextResponse":"tool result"}' }],
				},
				{
					role: "user",
					content: [{ type: "text", text: "who are you?" }],
				},
			],
			output_message: {
				role: "assistant",
				content: [{ type: "text", text: "I am the assistant." }],
			},
		} as unknown as LogEntry;

		expect(getMessage(log)).toBe('Tool Result: {"nextResponse":"tool result"}\nUser: who are you?\nAssistant: I am the assistant.');
	});

	it("returns realtime assistant tool calls from output message", () => {
		const log = {
			object: "realtime.turn",
			input_history: [
				{
					role: "user",
					content: [{ type: "text", text: "show me a pastel palette" }],
				},
			],
			output_message: {
				role: "assistant",
				tool_calls: [
					{
						function: {
							name: "display_color_palette",
							arguments: '{"theme":"pastel"}',
						},
					},
				],
			},
		} as unknown as LogEntry;

		expect(getMessage(log)).toBe('User: show me a pastel palette\nAssistant Tool Call: display_color_palette({"theme":"pastel"})');
	});

	it("returns the last user prompt, not a trailing system prompt, for chat logs", () => {
		const log = {
			object: "chat.completion",
			input_history: [
				{ role: "system", content: "You are a helpful assistant." },
				{ role: "user", content: "first question" },
				{ role: "assistant", content: "first answer" },
				{ role: "user", content: "the real last prompt" },
				{ role: "system", content: "<system-reminder>trailing injection</system-reminder>" },
			],
		} as unknown as LogEntry;

		expect(getMessage(log)).toBe("the real last prompt");
	});

	it("returns the first text block of the last user message, skipping a trailing <system-reminder> block", () => {
		// Anthropic providers inline mid-conversation system messages as a
		// trailing text block on the user turn. The last user message therefore
		// has two text blocks: the actual user prompt followed by
		// "<system-reminder>...". The message-column preview must show the user
		// prompt (the first non-empty text block), matching the SSE preview's
		// `activeEntryMessage` behavior on the Go side.
		const log = {
			object: "chat.completion",
			input_history: [
				{
					role: "user",
					content: [
						{ type: "text", text: "the real last prompt" },
						{ type: "text", text: "<system-reminder>injected by Anthropic</system-reminder>" },
					],
				},
			],
		} as unknown as LogEntry;

		expect(getMessage(log)).toBe("the real last prompt");
	});

	it("returns the first text block of the last responses user message, skipping a trailing <system-reminder> block", () => {
		const log = {
			object: "responses",
			input_history: [],
			responses_input_history: [
				{
					role: "user",
					content: [
						{ type: "input_text", text: "the real last prompt" },
						{ type: "input_text", text: "<system-reminder>injected by Anthropic</system-reminder>" },
					],
				},
			],
		} as unknown as LogEntry;

		expect(getMessage(log)).toBe("the real last prompt");
	});

	it("falls back to the last message when no user message exists", () => {
		const log = {
			object: "chat.completion",
			input_history: [
				{ role: "system", content: "setup" },
				{ role: "assistant", content: "only an assistant message" },
			],
		} as unknown as LogEntry;

		expect(getMessage(log)).toBe("only an assistant message");
	});

	it("falls back to content_summary when input history is empty", () => {
		const log = {
			object: "chat.completion",
			input_history: [],
			responses_input_history: [],
			content_summary: "summary preview",
		} as unknown as LogEntry;

		expect(getMessage(log)).toBe("summary preview");
	});

	it("returns the last user prompt from responses input history", () => {
		const log = {
			object: "responses",
			input_history: [],
			responses_input_history: [
				{ role: "user", content: [{ type: "input_text", text: "older request" }] },
				{ role: "assistant", content: [{ type: "input_text", text: "ack" }] },
				{ role: "user", content: [{ type: "input_text", text: "latest responses prompt" }] },
				{ role: "system", content: [{ type: "input_text", text: "trailing system" }] },
			],
		} as unknown as LogEntry;

		expect(getMessage(log)).toBe("latest responses prompt");
	});
});

describe("truncateByWidth", () => {
	it("does not truncate text within limit", () => {
		expect(truncateByWidth("你好世界", 25)).toBe("你好世界");
	});

	it("truncates pure Chinese text exceeding 25 chars", () => {
		expect(truncateByWidth("一二三四五六七八九十甲乙丙丁戊己庚辛壬癸天地玄黄宇宙", 25)).toBe(
			"一二三四五六七八九十甲乙丙丁戊己庚辛壬癸天地玄黄宇...",
		);
	});

	it("does not truncate exactly 25 Chinese chars", () => {
		expect(truncateByWidth("一二三四五六七八九十甲乙丙丁戊己庚辛壬癸天地玄黄宇", 25)).toBe(
			"一二三四五六七八九十甲乙丙丁戊己庚辛壬癸天地玄黄宇",
		);
	});

	it("does not truncate 37 English chars (37 × 2/3 = 24.67 < 25)", () => {
		const text = "a".repeat(37);
		expect(truncateByWidth(text, 25)).toBe(text);
	});

	it("truncates 38 English chars (38 × 2/3 = 25.33 > 25)", () => {
		expect(truncateByWidth("a".repeat(38), 25)).toBe("a".repeat(37) + "...");
	});

	it("does not truncate 10 Chinese + 22 English (10×3 + 22×2 = 74 < 75)", () => {
		const text = "一二三四五六七八九十" + "e".repeat(22);
		expect(truncateByWidth(text, 25)).toBe(text);
	});

	it("truncates 10 Chinese + 23 English (10×3 + 23×2 = 76 > 75)", () => {
		const text = "一二三四五六七八九十" + "e".repeat(23);
		expect(truncateByWidth(text, 25)).toBe("一二三四五六七八九十" + "e".repeat(22) + "...");
	});

	it("does not truncate at exact boundary 23 Chinese + 3 English (23×3 + 3×2 = 75)", () => {
		const text = "一".repeat(23) + "eee";
		expect(truncateByWidth(text, 25)).toBe(text);
	});

	it("returns empty string for empty input", () => {
		expect(truncateByWidth("", 25)).toBe("");
	});
});
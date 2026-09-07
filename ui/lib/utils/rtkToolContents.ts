import { LogEntry } from "@/lib/types/logs";

// extractCompressedToolContents walks a log entry's request body for the
// post-compression tool messages and indexes them by the same `index` value
// the RTK pipeline recorded on the pre-compression snapshot side. The diff
// view aligns the two sides on this key.
//
// Indices are canonicalised to the hint-free (client-sent) array: the RTK
// recovery hint is prepended at input[0] when RTK is enabled, which shifts
// every later position by one. Whether the persisted input_history actually
// contains that hint is decided by the pre-hook plugin order and by fallback
// inheritance — facts the logging plugin observes at capture time and records
// as the authoritative metadata offsets rtk_input_hint_offset /
// rtk_responses_input_hint_offset. This component subtracts those offsets
// instead of re-deriving them from message content, so numbering always agrees
// with the indices the RTK pipeline records (plugins/rtk compression.go).
//
// Coverage:
//   - Chat Completions: each tool-role message in input_history contributes
//     one entry keyed by its position in the array.
//   - Responses API: each function_call_output item in
//     responses_input_history contributes one entry keyed by its position.
//   - Anthropic-style tool_result blocks nested inside a user message use a
//     synthetic index (i*100+j) on the Go side; those don't currently have a
//     TS surface in ContentBlock, so they fall through and the diff view
//     falls back to the original text on both sides — matching the prior
//     behaviour when the compressed snapshot was missing.
export function extractCompressedToolContents(log: LogEntry): { index: number; content: string }[] {
	const items: { index: number; content: string }[] = [];
	const metadata = (log.metadata ?? {}) as Record<string, unknown>;

	const chatOffset = numberFromMetadata(metadata.rtk_input_hint_offset);
	if (Array.isArray(log.input_history)) {
		log.input_history.forEach((msg, i) => {
			if (msg?.role !== "tool") return;
			const text = readMessageContentAsText(msg.content);
			if (text === "") return;
			items.push({ index: i - chatOffset, content: text });
		});
	}

	const responsesOffset = numberFromMetadata(metadata.rtk_responses_input_hint_offset);
	if (Array.isArray(log.responses_input_history)) {
		log.responses_input_history.forEach((msg, i) => {
			if (msg?.type !== "function_call_output") return;
			const output = (msg as { output?: unknown }).output;
			if (typeof output === "string") {
				if (output !== "") items.push({ index: i - responsesOffset, content: output });
				return;
			}
			if (Array.isArray(output)) {
				const text = output
					.map((block) =>
						block && typeof block === "object" && "text" in block && typeof (block as { text?: unknown }).text === "string"
							? (block as { text: string }).text
							: "",
					)
					.filter((s) => s.length > 0)
					.join("");
				if (text !== "") items.push({ index: i - responsesOffset, content: text });
			}
		});
	}

	return items;
}

// numberFromMetadata reads an optional numeric metadata value; absent or
// non-numeric values resolve to 0 (no hint offset — the default for logs
// produced before the offset metadata existed, and for hint-free requests).
function numberFromMetadata(v: unknown): number {
	if (typeof v === "number" && Number.isFinite(v)) return v;
	if (typeof v === "string" && v.trim() !== "" && Number.isFinite(Number(v))) return Number(v);
	return 0;
}

function readMessageContentAsText(content: unknown): string {
	if (typeof content === "string") return content;
	if (Array.isArray(content)) {
		return content
			.map((block) => {
				if (!block || typeof block !== "object") return "";
				const text = (block as { text?: unknown }).text;
				return typeof text === "string" ? text : "";
			})
			.join("");
	}
	return "";
}
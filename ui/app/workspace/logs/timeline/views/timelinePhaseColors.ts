/**
 * @file Phase color/label registry for the per-log timeline detail view.
 *
 * Maps the timeline `phase` field produced by the backend aggregator
 * (transports/celer-route-http/handlers/logging.go:2957-3072) to a stable
 * display label and color pair. The five phases below are the only ones the
 * backend currently emits; anything else falls back to a neutral style and a
 * humanized version of the raw phase string so unknown phases still render.
 */

export type PhaseKey = "pre_llm" | "upstream_call" | "plugin_log" | "post_llm" | "key_attempt" | string;

export interface PhaseStyle {
	key: PhaseKey;
	labelKey: string;
	label: string;
	bar: string;
	chip: string;
	order: number;
}

const PHASE_ORDER: Record<string, number> = {
	pre_llm: 0,
	upstream_call: 1,
	key_attempt: 2,
	plugin_log: 3,
	post_llm: 4,
};

const PHASE_LABEL_KEY: Record<string, string> = {
	pre_llm: "pre",
	upstream_call: "upstream",
	plugin_log: "plugin",
	post_llm: "post",
	key_attempt: "keyAttempt",
};

const PHASE_BAR: Record<string, string> = {
	// pre-llm hook — neutral slate, short and near origin
	pre_llm: "bg-slate-400/70 dark:bg-slate-500/70",
	// upstream provider call — primary blue, the longest bar
	upstream_call: "bg-blue-500/80 dark:bg-blue-400/80",
	// arbitrary plugin log — purple
	plugin_log: "bg-violet-500/70 dark:bg-violet-400/70",
	// post-llm hook — slate, mirrored
	post_llm: "bg-slate-400/70 dark:bg-slate-500/70",
	// key attempt — amber
	key_attempt: "bg-amber-500/70 dark:bg-amber-400/70",
};

const PHASE_CHIP: Record<string, string> = {
	pre_llm: "bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-300",
	upstream_call: "bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300",
	plugin_log: "bg-violet-100 text-violet-700 dark:bg-violet-950 dark:text-violet-300",
	post_llm: "bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-300",
	key_attempt: "bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-300",
};

export function getPhaseStyle(phase: string, t: (key: string) => string): PhaseStyle {
	const labelKey = PHASE_LABEL_KEY[phase] ?? "unknown";
	const fallbackLabel = phase ? phase : t("timeline.detail.phase.unknown");
	const label = phase && PHASE_LABEL_KEY[phase] ? t(`timeline.detail.phase.${labelKey}`) : fallbackLabel;
	return {
		key: phase,
		labelKey,
		label,
		bar: PHASE_BAR[phase] ?? "bg-gray-400/70 dark:bg-gray-500/70",
		chip: PHASE_CHIP[phase] ?? "bg-gray-100 text-gray-700 dark:bg-gray-900 dark:text-gray-300",
		order: PHASE_ORDER[phase] ?? 99,
	};
}
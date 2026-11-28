/**
 * Shared time-window vocabulary for the provider cooldown tab.
 *
 * The window used to be owned by three independent components — the usage
 * card, the error-pattern "baseline" block and the error-sample browser —
 * each carrying its own copy of the 1h/24h option list. Nothing stopped the
 * page from showing two error lists for the same provider computed over
 * DIFFERENT windows at the same time, which reads as contradictory data
 * even though both were correct.
 *
 * One vocabulary, one owner: the tab picks the window, every consumer renders
 * it. The error-sample browser keeps a private fallback window only for the
 * contexts that embed it without a tab around it (the provider config sheet).
 */

export type CooldownWindowKey = "1h" | "24h";

export interface CooldownWindowOption {
	/** Stable key — also the value the /logs/error-patterns endpoint expects. */
	key: CooldownWindowKey;
	/** Window length in minutes, as the provider-usage endpoint expects. */
	minutes: number;
	/** data-testid of the segmented-control button. */
	testId: string;
}

export const COOLDOWN_WINDOW_OPTIONS: readonly CooldownWindowOption[] = [
	{ key: "1h", minutes: 60, testId: "providers2-cooldown-window-1h" },
	{ key: "24h", minutes: 1440, testId: "providers2-cooldown-window-24h" },
];

export const DEFAULT_COOLDOWN_WINDOW: CooldownWindowKey = "1h";

/** Window length in minutes for a window key. */
export function cooldownWindowMinutes(key: CooldownWindowKey): number {
	return COOLDOWN_WINDOW_OPTIONS.find((option) => option.key === key)?.minutes ?? COOLDOWN_WINDOW_OPTIONS[0].minutes;
}

/** i18n key of the window label, shared by every window control. */
export function cooldownWindowLabelKey(key: CooldownWindowKey): string {
	return `cooldownCapability.window.${key}`;
}

/**
 * Formats a fetch timestamp as a local wall-clock time for the "updated at"
 * affordance. Returns null when there is nothing to show (not fetched yet),
 * so callers can hide the affordance instead of rendering a fake time.
 */
export function formatUpdatedAt(timestampMs: number | undefined): string | null {
	if (!timestampMs || !Number.isFinite(timestampMs)) return null;
	const date = new Date(timestampMs);
	if (Number.isNaN(date.getTime())) return null;
	return date.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}
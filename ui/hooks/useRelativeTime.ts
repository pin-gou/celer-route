import i18n from "@/lib/i18n/config";
import { useEffect, useState } from "react";

/**
 * Locale-aware relative-time formatter.
 *
 * Replaces date-fns `formatDistanceToNow`, which always emits English output
 * regardless of the active i18n language. This hook uses the built-in
 * `Intl.RelativeTimeFormat` with the current i18n language, falling back to
 * "en" when the runtime doesn't have data for the requested locale.
 *
 * Updates automatically when:
 *   - the i18n language changes (e.g. user switches to zh-CN)
 *   - the dependent time changes (e.g. user revisits the sheet after minutes)
 *
 * The output mirrors `formatDistanceToNow(date, { addSuffix: true })`:
 *   - future date: "in 2 hours" / "2 小时后"
 *   - past date:   "2 hours ago" / "2 小时前"
 */
export function useRelativeTime(date: Date | string | null | undefined, options?: { addSuffix?: boolean }): string {
	const addSuffix = options?.addSuffix ?? true;
	useNow(60_000); // recompute once a minute so "1 minute ago" -> "2 minutes ago"
	const [, setLangVersion] = useState(0);
	const currentLang = i18n.language || "en";

	useEffect(() => {
		const handler = () => setLangVersion((v) => v + 1);
		i18n.on("languageChanged", handler);
		return () => {
			i18n.off("languageChanged", handler);
		};
	}, []);

	if (!date) return "";
	const target = typeof date === "string" ? new Date(date) : date;
	if (Number.isNaN(target.getTime())) return "";

	const locale = resolveLocale(currentLang);
	const diffMs = target.getTime() - Date.now();
	const absMs = Math.abs(diffMs);

	const { value, unit } = pickUnit(absMs);
	const formatter = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
	const raw = formatter.format(value, unit);

	if (!addSuffix) return raw;

	// When numeric="auto", future "in 1 day" / past "1 day ago" are produced
	// for ±1; for other magnitudes Intl already prepends "in"/"ago" in many
	// locales. We avoid double-prefixing by checking the raw string.
	const isPast = diffMs < 0;
	if (isPast && (raw.endsWith("ago") || /前$/.test(raw))) return raw;
	if (!isPast && (raw.startsWith("in ") || /后$/.test(raw))) return raw;

	// Fallback for locales without native suffix words in numeric mode.
	const numeric = new Intl.RelativeTimeFormat(locale, { numeric: "always" });
	const n = numeric.format(value, unit);
	return isPast ? `${n} ago` : `in ${n}`;
}

function pickUnit(absMs: number): { value: number; unit: Intl.RelativeTimeFormatUnit } {
	const seconds = Math.round(absMs / 1000);
	if (seconds < 60) return { value: -Math.sign(absMs) * seconds, unit: "second" };
	const minutes = Math.round(seconds / 60);
	if (minutes < 60) return { value: -Math.sign(absMs) * minutes, unit: "minute" };
	const hours = Math.round(minutes / 60);
	if (hours < 24) return { value: -Math.sign(absMs) * hours, unit: "hour" };
	const days = Math.round(hours / 24);
	if (days < 7) return { value: -Math.sign(absMs) * days, unit: "day" };
	const weeks = Math.round(days / 7);
	if (weeks < 4) return { value: -Math.sign(absMs) * weeks, unit: "week" };
	const months = Math.round(days / 30);
	if (months < 12) return { value: -Math.sign(absMs) * months, unit: "month" };
	const years = Math.round(days / 365);
	return { value: -Math.sign(absMs) * years, unit: "year" };
}

function resolveLocale(lang: string): string {
	// i18n uses "zh-CN"; Intl expects BCP-47. The base subtag is enough for
	// most browsers; if not, fall back to "en".
	if (typeof lang !== "string" || lang.length === 0) return "en";
	try {
		new Intl.RelativeTimeFormat(lang);
		return lang;
	} catch {
		return "en";
	}
}

/** Re-render every `intervalMs` so labels like "1 minute ago" stay fresh. */
function useNow(intervalMs: number): number {
	const [now, setNow] = useState(() => Date.now());
	useEffect(() => {
		const id = window.setInterval(() => setNow(Date.now()), intervalMs);
		return () => window.clearInterval(id);
	}, [intervalMs]);
	return now;
}
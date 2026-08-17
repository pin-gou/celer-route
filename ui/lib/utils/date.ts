import { format, formatDistanceToNow, isValid, Locale } from "date-fns";
import { enUS, zhCN } from "date-fns/locale";

/**
 * Converts a Date object to an RFC 3339 string with the local time zone offset.
 *
 * Example: 2025-11-19T12:23:19.421+05:30
 *
 * @param dateObj The Date object to convert (defaults to new Date() if null/undefined).
 * @returns The RFC 3339 formatted string with local offset.
 */
export function dateToRfc3339Local(dateObj?: Date): string {
	const now = dateObj instanceof Date ? dateObj : new Date();

	// Helper function to pad single digits with a leading zero
	const pad = (num: number): string => (num < 10 ? "0" + num : String(num));

	const Y = now.getFullYear();
	const M = pad(now.getMonth() + 1); // Month is 0-indexed (Jan=0)
	const D = pad(now.getDate());
	const H = pad(now.getHours());
	const m = pad(now.getMinutes());
	const S = pad(now.getSeconds());
	const ms = String(now.getMilliseconds()).padStart(3, "0");

	// getTimezoneOffset() returns the difference in minutes from UTC for the local time.
	// The result is positive for time zones west of Greenwich and negative for those east.
	// We negate it to get the standard ISO/RFC sign convention (+ for East, - for West).
	const timezoneOffsetMinutes = -now.getTimezoneOffset();
	const sign = timezoneOffsetMinutes >= 0 ? "+" : "-";
	const absoluteOffset = Math.abs(timezoneOffsetMinutes);
	const offsetHours = pad(Math.floor(absoluteOffset / 60));
	const offsetMinutes = pad(absoluteOffset % 60);
	const rfc3339Local = `${Y}-${M}-${D}T${H}:${m}:${S}.${ms}${sign}${offsetHours}:${offsetMinutes}`;
	return rfc3339Local;
}

/**
 * Picks the date-fns Locale that matches the current i18next language.
 *
 * Mirrors the project-wide convention in
 * `ui/app/workspace/logs/views/columns.tsx` and
 * `ui/components/ui/calendar.tsx` (`zh*` uses zhCN, everything else enUS).
 *
 * @param i18nLanguage The current `i18n.language` (e.g. "zh-CN", "en").
 * @returns The matching date-fns locale.
 */
export function getDateLocale(i18nLanguage?: string): Locale {
	return (i18nLanguage ?? "").startsWith("zh") ? zhCN : enUS;
}

/**
 * Formats a timestamp string or Date into a locale-aware absolute time
 * rendered in the user's local time zone.
 *
 * Returns `undefined` for invalid/missing inputs so callers can decide how
 * to fall back (placeholder string, omitting the field, etc.).
 *
 * Default pattern is `yyyy-MM-dd HH:mm:ss` — keeps the same wire date order
 * as ISO timestamps and matches the format requested for the cooldown panel.
 *
 * @param value ISO/RFC string, Date, or any value `new Date()` accepts.
 * @param pattern date-fns format pattern (default `yyyy-MM-dd HH:mm:ss`).
 * @param locale date-fns Locale (defaults to `getDateLocale("en")`).
 */
export function formatLocalDateTime(
	value: string | Date | null | undefined,
	pattern = "yyyy-MM-dd HH:mm:ss",
	locale: Locale = enUS,
): string | undefined {
	if (value === null || value === undefined || value === "") return undefined;
	const date = value instanceof Date ? value : new Date(value);
	if (!isValid(date)) return undefined;
	return format(date, pattern, { locale });
}

/**
 * Formats the gap between `value` and now as a human-readable phrase in the
 * user's current language (e.g. "1 hour 3 minutes", "1 小时 3 分钟").
 *
 * Returns `undefined` for invalid/missing inputs.
 *
 * @param value ISO/RFC string, Date, or any value `new Date()` accepts.
 * @param locale date-fns Locale (defaults to `getDateLocale("en")`).
 */
export function formatRelativeDistanceToNow(value: string | Date | null | undefined, locale: Locale = enUS): string | undefined {
	if (value === null || value === undefined || value === "") return undefined;
	const date = value instanceof Date ? value : new Date(value);
	if (!isValid(date)) return undefined;
	return formatDistanceToNow(date, { locale });
}
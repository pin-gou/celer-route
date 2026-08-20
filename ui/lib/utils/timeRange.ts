export const TIME_PERIODS = [
	{ label: "Last hour", value: "1h" },
	{ label: "Last 6 hours", value: "6h" },
	{ label: "Last 24 hours", value: "24h" },
	{ label: "Last 7 days", value: "7d" },
	{ label: "Last 30 days", value: "30d" },
	{ label: "Today", value: "today" },
	{ label: "Yesterday", value: "yesterday" },
];

export type TimePeriod = (typeof TIME_PERIODS)[number]["value"];

export function getTimePeriods(t: (key: string, opts?: Record<string, unknown>) => string): { label: string; value: string }[] {
	return [
		{ label: t("timePeriods.lastHour", { ns: "common" }), value: "1h" },
		{ label: t("timePeriods.last6Hours", { ns: "common" }), value: "6h" },
		{ label: t("timePeriods.last24Hours", { ns: "common" }), value: "24h" },
		{ label: t("timePeriods.last7Days", { ns: "common" }), value: "7d" },
		{ label: t("timePeriods.last30Days", { ns: "common" }), value: "30d" },
		{ label: t("timePeriods.today", { ns: "common" }), value: "today" },
		{ label: t("timePeriods.yesterday", { ns: "common" }), value: "yesterday" },
	];
}

/** Returns a fresh { from, to } Date pair for the given relative period string. */
export function getRangeForPeriod(period: string): { from: Date; to: Date } {
	const to = new Date();
	const from = new Date(to.getTime());
	switch (period) {
		case "1h":
			from.setHours(from.getHours() - 1);
			break;
		case "6h":
			from.setHours(from.getHours() - 6);
			break;
		case "24h":
			from.setHours(from.getHours() - 24);
			break;
		case "7d":
			from.setDate(from.getDate() - 7);
			break;
		case "30d":
			from.setDate(from.getDate() - 30);
			break;
		case "today":
			from.setHours(0, 0, 0, 0);
			to.setHours(23, 59, 59, 999);
			break;
		case "yesterday": {
			const y = new Date(to.getTime());
			y.setDate(y.getDate() - 1);
			y.setHours(0, 0, 0, 0);
			from.setTime(y.getTime());
			to.setDate(to.getDate() - 1);
			to.setHours(23, 59, 59, 999);
			break;
		}
		default:
			from.setHours(from.getHours() - 1);
	}
	return { from, to };
}

/** Returns unix timestamps (seconds) for the given relative period string. */
export function getUnixRangeForPeriod(period: string): { start: number; end: number } {
	const { from, to } = getRangeForPeriod(period);
	return {
		start: Math.floor(from.getTime() / 1000),
		end: Math.floor(to.getTime() / 1000),
	};
}
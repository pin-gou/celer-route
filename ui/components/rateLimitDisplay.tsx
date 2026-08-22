import { Progress } from "@/components/ui/progress";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { resetDurationLabels, supportsCalendarAlignment } from "@/lib/constants/governance";
import { cn } from "@/lib/utils";
import { formatCompactNumber } from "@/lib/utils/numbers";
import { useTranslation } from "react-i18next";

interface RateLimitShape {
	token_max_limit?: number | null;
	token_reset_duration?: string | null;
	token_current_usage?: number | null;
	request_max_limit?: number | null;
	request_reset_duration?: string | null;
	request_current_usage?: number | null;
}

interface RateLimitDisplayProps {
	rateLimits: RateLimitShape | null | undefined;
	/** Compact mode for narrow cells — still renders bars, just tighter */
	compact?: boolean;
	/** Render limit + reset period only (no usage bar). Use for template entities like access profiles. */
	limitOnly?: boolean;
	/** When true, alignable durations (day/week/month/year) get a "(calendar)" suffix to
	 * mirror the budget cell. Sourced from the owning VK's calendar_aligned flag. */
	calendarAligned?: boolean;
}

const formatResetDuration = (duration?: string | null, calendarAligned?: boolean, t?: (key: string) => string) => {
	if (!duration) return "";
	const label = resetDurationLabels[duration] || duration;
	return calendarAligned && supportsCalendarAlignment(duration) ? `${label} ${t?.("period.calendarSuffix")}` : label;
};

function LimitText({
	label,
	max,
	resetDuration,
	calendarAligned,
}: {
	label: string;
	max: number;
	resetDuration?: string | null;
	calendarAligned?: boolean;
}) {
	const { t } = useTranslation("governance-ui");
	return (
		<div className="flex items-center justify-between gap-4 text-xs">
			<span className="font-mono">
				{formatCompactNumber(max)} {label}
			</span>
			<span className="text-muted-foreground">{formatResetDuration(resetDuration, calendarAligned, t)}</span>
		</div>
	);
}

function Bar({
	label,
	current,
	max,
	resetDuration,
	compact,
	calendarAligned,
}: {
	label: string;
	current: number;
	max: number;
	resetDuration?: string | null;
	compact?: boolean;
	calendarAligned?: boolean;
}) {
	const { t } = useTranslation("governance-ui");
	const pct = max > 0 ? Math.min((current / max) * 100, 100) : 0;
	const isExhausted = max > 0 && current >= max;
	const barClass = isExhausted ? "[&>div]:bg-red-500/70" : pct > 80 ? "[&>div]:bg-amber-500/70" : "[&>div]:bg-emerald-500/70";

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<div className={cn("space-y-1.5", compact && "space-y-1")}>
					<div className="flex items-center justify-between gap-4 text-xs">
						<span className="font-medium">
							{formatCompactNumber(max)} {label}
						</span>
						<span className="text-muted-foreground">{formatResetDuration(resetDuration, calendarAligned, t)}</span>
					</div>
					<Progress value={pct} className={cn("bg-muted/70 dark:bg-muted/30 h-1", barClass)} />
				</div>
			</TooltipTrigger>
			<TooltipContent>
				<p className="font-medium">
					{current.toLocaleString()} / {max.toLocaleString()} {label}
				</p>
				{resetDuration ? (
					<p className="text-primary-foreground/80 text-xs">
						{t("period.resets", { period: formatResetDuration(resetDuration, calendarAligned, t) })}
					</p>
				) : null}
			</TooltipContent>
		</Tooltip>
	);
}

export function RateLimitDisplay({ rateLimits, compact, limitOnly, calendarAligned }: RateLimitDisplayProps) {
	const { t } = useTranslation("governance-ui");
	if (!rateLimits) {
		return <span className="text-muted-foreground text-sm">-</span>;
	}

	const hasTokens = rateLimits.token_max_limit != null && rateLimits.token_max_limit > 0;
	const hasRequests = rateLimits.request_max_limit != null && rateLimits.request_max_limit > 0;

	if (!hasTokens && !hasRequests) {
		return <span className="text-muted-foreground text-sm">-</span>;
	}

	return (
		<div className={cn("space-y-2.5 min-w-[160px]", compact && "space-y-2", limitOnly && "space-y-1")}>
			{hasTokens ? (
				limitOnly ? (
					<LimitText
						label={t("governance:rateLimit.tokens")}
						max={rateLimits.token_max_limit!}
						resetDuration={rateLimits.token_reset_duration}
						calendarAligned={calendarAligned}
					/>
				) : (
					<Bar
						label={t("governance:rateLimit.tokens")}
						current={rateLimits.token_current_usage ?? 0}
						max={rateLimits.token_max_limit!}
						resetDuration={rateLimits.token_reset_duration}
						compact={compact}
						calendarAligned={calendarAligned}
					/>
				)
			) : null}
			{hasRequests ? (
				limitOnly ? (
					<LimitText
						label={t("governance:rateLimit.requests")}
						max={rateLimits.request_max_limit!}
						resetDuration={rateLimits.request_reset_duration}
						calendarAligned={calendarAligned}
					/>
				) : (
					<Bar
						label={t("governance:rateLimit.requests")}
						current={rateLimits.request_current_usage ?? 0}
						max={rateLimits.request_max_limit!}
						resetDuration={rateLimits.request_reset_duration}
						compact={compact}
						calendarAligned={calendarAligned}
					/>
				)
			) : null}
		</div>
	);
}
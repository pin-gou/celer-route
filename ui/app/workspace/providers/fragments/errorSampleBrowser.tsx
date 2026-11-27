import React from "react";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
	COOLDOWN_WINDOW_OPTIONS,
	DEFAULT_COOLDOWN_WINDOW,
	cooldownWindowLabelKey,
	formatUpdatedAt,
	type CooldownWindowKey,
} from "@/components/providerCooldown/cooldownWindow";
import { useGetErrorPatternsQuery, type ErrorPattern } from "@/lib/store/apis/errorPatternsApi";
import { useTranslation } from "react-i18next";

interface ErrorSampleBrowserProps {
	provider: string;
	onApply: (pattern: ErrorPattern) => void;
	/**
	 * Controlled window. When the tab owns the window it passes it down and
	 * this component renders no selector of its own — two independent window
	 * pickers on the same page could show two error lists computed over
	 * different windows, which reads as contradictory data.
	 */
	window?: CooldownWindowKey;
	/** Hides the "no errors in this window" message (the tab shows one instead). */
	suppressEmptyState?: boolean;
}

/**
 * The provider's current error shapes, each one click away from becoming a
 * cooldown match.
 */
export function ErrorSampleBrowser({ provider, onApply, window, suppressEmptyState }: ErrorSampleBrowserProps) {
	const { t } = useTranslation("providers");
	const { t: tc } = useTranslation("common");
	// Uncontrolled fallback for the contexts that embed this browser without
	// a tab around it (the provider config sheet).
	const [localWindow, setLocalWindow] = React.useState<CooldownWindowKey>(DEFAULT_COOLDOWN_WINDOW);
	const isControlled = window !== undefined;
	const activeWindow = window ?? localWindow;

	const { data, isLoading, error, fulfilledTimeStamp } = useGetErrorPatternsQuery({
		provider,
		window: activeWindow,
		limit: 20,
	});

	const patterns = data?.patterns ?? [];
	const totalErrors = data?.total_errors ?? 0;
	const updatedAt = formatUpdatedAt(fulfilledTimeStamp);

	return (
		<div className="flex flex-col gap-3">
			{/* Provider label + window selector (only when uncontrolled) */}
			<div className="flex items-center gap-2">
				<div className="flex-1 truncate font-mono text-xs font-medium">{provider}</div>
				{isControlled ? (
					<span className="text-muted-foreground shrink-0 text-xs">{tc(cooldownWindowLabelKey(activeWindow))}</span>
				) : (
					<Select value={activeWindow} onValueChange={(value) => setLocalWindow(value as CooldownWindowKey)}>
						<SelectTrigger className="h-7 w-36 text-xs" data-testid="error-sample-browser-window-trigger">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{COOLDOWN_WINDOW_OPTIONS.map((option) => (
								<SelectItem key={option.key} value={option.key}>
									{tc(cooldownWindowLabelKey(option.key))}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				)}
			</div>

			{isLoading && <p className="text-muted-foreground py-4 text-center text-xs">{t("fragments.cooldownPolicy.errorSample.loading")}</p>}

			{error && <p className="text-destructive py-4 text-center text-xs">{t("fragments.cooldownPolicy.errorSample.error")}</p>}

			{!isLoading && !error && patterns.length === 0 && !suppressEmptyState && (
				<div className="py-4 text-center">
					<p className="text-muted-foreground text-xs">{t("fragments.cooldownPolicy.errorSample.noErrors")}</p>
					{/* The hint only makes sense while a wider window is still
					    available — on the 24h window it would be a lie. */}
					{activeWindow !== "24h" && (
						<p className="text-muted-foreground mt-1 text-xs">{t("fragments.cooldownPolicy.errorSample.tryChangingWindow")}</p>
					)}
				</div>
			)}

			{!isLoading && !error && patterns.length > 0 && (
				<div className="space-y-2">
					<div className="flex flex-wrap items-center justify-between gap-2">
						<p className="text-muted-foreground text-xs">{t("fragments.cooldownPolicy.errorSample.totalErrors", { count: totalErrors })}</p>
						{updatedAt && (
							<span data-testid="error-sample-updated-at" className="text-muted-foreground text-xs">
								{t("fragments.cooldownPolicy.errorSample.updatedAt", { time: updatedAt })}
							</span>
						)}
					</div>
					<div className="max-h-80 space-y-1 overflow-y-auto">
						{patterns.map((p) => (
							<div
								key={p.rank}
								data-testid={`error-sample-row-${p.rank}`}
								className="bg-muted/30 flex items-center justify-between rounded border p-2"
							>
								<div className="min-w-0 flex-1">
									<div className="flex items-center gap-1 text-xs">
										<span className="font-semibold">#{p.rank}</span>
										<span className="text-muted-foreground">{p.count}×</span>
										{p.status_code !== undefined && (
											<span className="text-muted-foreground font-mono">
												{p.status_code >= 100 && p.status_code <= 599 ? p.status_code : "—"}
											</span>
										)}
									</div>
									<div className="text-muted-foreground truncate font-mono text-xs">
										{p.error_type && <span>{p.error_type} </span>}
										{p.error_code && <span className="text-muted-foreground/70">{p.error_code} </span>}
										{p.sample_message && (
											<span className="text-muted-foreground/50 truncate">
												"{p.sample_message.slice(0, 80)}
												{p.sample_message.length > 80 ? "…" : ""}"
											</span>
										)}
									</div>
								</div>
								<Button
									type="button"
									variant="outline"
									size="sm"
									className="ml-2 shrink-0"
									onClick={() => onApply(p)}
									data-testid={`error-sample-apply-${p.rank}`}
								>
									{t("fragments.cooldownPolicy.errorSample.apply")}
								</Button>
							</div>
						))}
					</div>
				</div>
			)}
		</div>
	);
}
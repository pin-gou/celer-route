import React from "react";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useGetErrorPatternsQuery, type ErrorPattern } from "@/lib/store/apis/errorPatternsApi";
import { useTranslation } from "react-i18next";

interface ErrorSampleBrowserProps {
	provider: string;
	onApply: (pattern: ErrorPattern) => void;
}

const WINDOW_OPTIONS = ["1h", "24h"] as const;

export function ErrorSampleBrowser({ provider, onApply }: ErrorSampleBrowserProps) {
	const { t } = useTranslation("providers");
	// Default to the last 1h of errors — the most actionable window for
	// "what is failing right now". The user can widen to 24h via the selector.
	const [selectedWindow, setSelectedWindow] = React.useState<string>("1h");
	const { data, isLoading, error } = useGetErrorPatternsQuery({
		provider,
		window: selectedWindow,
		limit: 20,
	});

	const patterns = data?.patterns ?? [];
	const totalErrors = data?.total_errors ?? 0;

	return (
		<div className="flex flex-col gap-3">
			{/* Provider label + window selector */}
			<div className="flex items-center gap-2">
				<div className="flex-1 truncate font-mono text-xs font-medium">{provider}</div>
				<Select value={selectedWindow} onValueChange={setSelectedWindow}>
					<SelectTrigger className="h-7 w-36 text-xs" data-testid="error-sample-browser-window-trigger">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{WINDOW_OPTIONS.map((w) => (
							<SelectItem key={w} value={w}>
								{t(`fragments.cooldownPolicy.errorSample.windowOptions.${w}`)}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</div>

			{isLoading && <p className="text-muted-foreground py-4 text-center text-xs">{t("fragments.cooldownPolicy.errorSample.loading")}</p>}

			{error && <p className="text-destructive py-4 text-center text-xs">{t("fragments.cooldownPolicy.errorSample.error")}</p>}

			{!isLoading && !error && patterns.length === 0 && (
				<div className="py-4 text-center">
					<p className="text-muted-foreground text-xs">{t("fragments.cooldownPolicy.errorSample.noErrors")}</p>
					<p className="text-muted-foreground mt-1 text-xs">{t("fragments.cooldownPolicy.errorSample.tryChangingWindow")}</p>
				</div>
			)}

			{!isLoading && !error && patterns.length > 0 && (
				<div className="space-y-2">
					<p className="text-muted-foreground text-xs">{t("fragments.cooldownPolicy.errorSample.totalErrors", { count: totalErrors })}</p>
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
										{p.status_code !== undefined && <span className="text-muted-foreground font-mono">{p.status_code}</span>}
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
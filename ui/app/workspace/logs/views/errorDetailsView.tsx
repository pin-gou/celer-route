import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import type { BifrostError } from "@/lib/types/logs";
import { cn } from "@/lib/utils";
import { AlertCircle, ChevronDown, Clipboard } from "lucide-react";
import { useTranslation } from "react-i18next";

function CopyButton({ text, testId }: { text: string; testId?: string }) {
	const { t } = useTranslation("logs");
	const { copy } = useCopyToClipboard({ successMessage: t("detailView.copied") });
	return (
		<button
			type="button"
			onClick={(e) => {
				e.stopPropagation();
				copy(text);
			}}
			className="text-muted-foreground hover:bg-muted hover:text-foreground inline-flex h-6 w-6 items-center justify-center rounded-sm transition"
			aria-label={t("detailView.copied")}
			data-testid={testId}
		>
			<Clipboard className="h-3.5 w-3.5" />
		</button>
	);
}

export default function ErrorDetailsView({
	errorDetails,
	compact = false,
	className,
	testId,
}: {
	errorDetails: BifrostError;
	compact?: boolean;
	className?: string;
	testId?: string;
}) {
	const { t } = useTranslation("logs");
	const message = errorDetails?.error?.message;
	const detail = errorDetails?.error?.error;
	if (!message && detail == null) return null;

	return (
		<div
			data-testid={testId}
			className={cn(
				"rounded-sm border border-red-200 bg-red-50/70 dark:border-red-900 dark:bg-red-950/30",
				compact ? "p-3" : "p-5",
				className,
			)}
		>
			<div className="flex items-center gap-2 text-red-700 dark:text-red-400">
				<AlertCircle className="h-4 w-4 shrink-0" />
				<span className={cn("font-semibold", compact ? "text-[12px]" : "text-[12.5px]")}>{t("detailView.error")}</span>
				{message ? <CopyButton text={message} /> : null}
			</div>
			{message ? (
				<div
					className={cn(
						"break-words whitespace-pre-wrap text-red-700 dark:text-red-400",
						compact ? "mt-1.5 text-[12.5px] leading-relaxed" : "mt-2 text-[13px] leading-relaxed",
					)}
				>
					{message}
				</div>
			) : null}
			{detail != null ? (
				<details
					className={cn(
						"group rounded-sm border border-red-200/70 bg-white/40 dark:border-red-900/70 dark:bg-red-950/40",
						compact ? "mt-2" : "mt-3",
					)}
				>
					<summary className="flex cursor-pointer items-center justify-between px-3 py-2 text-[12px] text-red-700 hover:bg-red-50/80 dark:text-red-400 dark:hover:bg-red-950/60">
						<span className="font-medium">{t("detailView.details")}</span>
						<ChevronDown className="h-3.5 w-3.5 transition-transform group-open:rotate-180" />
					</summary>
					<div className="custom-scrollbar max-h-[400px] overflow-y-auto border-t border-red-200/70 px-3 py-2 font-mono text-[11.5px] leading-[1.6] break-words whitespace-pre-wrap text-red-900 dark:border-red-900/70 dark:text-red-300">
						{typeof detail === "string" ? detail : JSON.stringify(detail, null, 2)}
					</div>
				</details>
			) : null}
		</div>
	);
}
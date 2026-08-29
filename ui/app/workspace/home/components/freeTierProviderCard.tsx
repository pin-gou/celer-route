import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { BundleProviderEntry } from "@/lib/types/catalog";
import { cn } from "@/lib/utils";
import { Link } from "@tanstack/react-router";
import { ChevronRight, ExternalLink, KeyRound, MousePointerClick } from "lucide-react";
import { useTranslation } from "react-i18next";

interface Props {
	bundleId: string;
	provider: BundleProviderEntry;
	// True when the provider is already configured on this instance. Configured
	// rows render as a direct link to the provider detail page (no configure
	// button); unconfigured rows show apply/configure actions.
	configured: boolean;
	onConfigure: (provider: BundleProviderEntry) => void;
}

/**
 * Single free-tier provider card. Used inside the unified grid of the
 * free-tier recommendation card — one card per provider, no nested wrapping
 * cards. Rows in the same grid row align to the tallest card, so bundles
 * with very different provider counts no longer produce big empty cells.
 *
 * Layout:
 *   line 1: provider name (+ "已配置" badge for configured rows)
 *   line 2: model list (rendered as low-emphasis Badges)
 *   line 3: notes (free-form hint text)
 *   line 4 (right-aligned): "去申请" link + small "一键配置" button
 */
export default function FreeTierProviderCard({ bundleId, provider, configured, onConfigure }: Props) {
	const { t } = useTranslation("home");

	const testid = `home-free-tier-provider-${bundleId}-${provider.provider}`;

	// Shared model list — rendered as muted Badges so users can scan the
	// supported model names without the row becoming visually loud.
	const modelBadges = provider.models.length > 0 && (
		<div className="flex flex-wrap gap-1" data-testid={`home-free-tier-models-${bundleId}-${provider.provider}`}>
			{provider.models.map((model) => (
				<Badge key={model} variant="secondary" className="text-muted-foreground h-4 rounded-sm px-1.5 font-normal">
					{model}
				</Badge>
			))}
		</div>
	);

	if (configured) {
		return (
			<Link
				to="/workspace/providers/$id"
				params={{ id: provider.provider }}
				className="bg-muted/30 hover:bg-muted/40 flex h-full flex-col gap-2 rounded-md border border-dashed p-3 opacity-80 transition-colors hover:opacity-100"
				data-testid={testid}
			>
				<div className="flex items-center justify-between gap-2">
					<span className="text-muted-foreground text-sm font-medium capitalize">{provider.provider}</span>
					<Badge
						variant="secondary"
						className="text-muted-foreground h-4 px-1.5 text-[10px] font-normal"
						data-testid={`home-free-tier-configured-${provider.provider}`}
					>
						{t("freeTier.configured")}
					</Badge>
				</div>
				<div className="opacity-70">{modelBadges}</div>
				{provider.notes && <p className="text-muted-foreground/70 text-xs">{provider.notes}</p>}
				<div className="mt-auto flex justify-end">
					<span className="text-muted-foreground inline-flex items-center gap-1 text-xs font-medium">
						{t("freeTier.viewDetails")}
						<ChevronRight className="h-3.5 w-3.5" />
					</span>
				</div>
			</Link>
		);
	}

	return (
		<div
			className={cn(
				"flex h-full flex-col gap-2 rounded-md border p-3 shadow-sm transition-shadow hover:shadow-md",
				"border-orange-300/70 bg-gradient-to-br from-orange-50/70 via-amber-50/50 to-rose-50/60",
				"dark:border-orange-800/50 dark:from-orange-950/25 dark:via-amber-950/15 dark:to-rose-950/25",
			)}
			data-testid={testid}
		>
			<div className="text-sm font-medium capitalize">{provider.provider}</div>
			{modelBadges}
			{provider.notes && <p className="text-muted-foreground text-xs">{provider.notes}</p>}
			<div className="mt-auto flex items-center justify-end gap-2">
				{provider.apply_url && (
					<a
						href={provider.apply_url}
						target="_blank"
						rel="noreferrer"
						className="inline-flex items-center gap-1 text-xs font-medium text-orange-600 hover:text-orange-700 dark:text-orange-400 dark:hover:text-orange-300"
						data-testid={`home-free-tier-apply-${provider.provider}`}
					>
						<ExternalLink className="h-3 w-3" />
						{t("freeTier.applyNow")}
					</a>
				)}
				<Button
					type="button"
					className="h-6 gap-1 rounded-sm bg-orange-500 px-2 text-[11px] text-white hover:bg-orange-600 dark:bg-orange-500 dark:hover:bg-orange-400"
					dataTestId={`home-free-tier-configure-${provider.provider}`}
					onClick={() => onConfigure(provider)}
				>
					{provider.is_keyless ? <MousePointerClick className="h-3 w-3" /> : <KeyRound className="h-3 w-3" />}
					<span>{provider.is_keyless ? t("freeTier.keylessNote") : t("freeTier.configureNow")}</span>
				</Button>
			</div>
		</div>
	);
}
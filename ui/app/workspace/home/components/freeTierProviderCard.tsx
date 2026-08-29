import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { BundleProviderEntry } from "@/lib/types/catalog";
import { ModelProvider } from "@/lib/types/config";
import { cn } from "@/lib/utils";
import { Link } from "@tanstack/react-router";
import { ChevronRight, ExternalLink, Info, KeyRound, MousePointerClick } from "lucide-react";
import { useTranslation } from "react-i18next";
import { computeDotState, dotClass } from "./providerHealth";

interface Props {
	bundleId: string;
	provider: BundleProviderEntry;
	// True when the provider is already configured on this instance. Configured
	// rows render as a direct link to the provider detail page (no configure
	// button); unconfigured rows show apply/configure actions.
	configured: boolean;
	// Runtime provider object from GET /api/providers (present only when
	// configured) — drives the health dot on configured rows.
	runtime?: ModelProvider | null;
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
export default function FreeTierProviderCard({ bundleId, provider, configured, runtime, onConfigure }: Props) {
	const { t } = useTranslation("home");
	const { t: tc } = useTranslation("common");

	const testid = `home-free-tier-provider-${bundleId}-${provider.provider}`;

	// Present only for providers this gateway build does not ship natively —
	// the server keeps these fields solely for the custom-provider fallback.
	const customBadge = provider.base_provider && (
		<Badge
			variant="secondary"
			className="text-muted-foreground h-4 shrink-0 rounded-sm px-1.5 text-[10px] font-normal"
			data-testid={`home-free-tier-protocol-${provider.provider}`}
		>
			{t("freeTier.customProtocol", { protocol: provider.base_provider })}
		</Badge>
	);

	// Present only when the entry has a fixed end date (e.g. a limited-time
	// free model window). Rendered next to the provider name as an orange badge.
	const freeValidBadge = provider.free_valid_until && (
		<Badge
			variant="outline"
			className="h-4 shrink-0 rounded-sm border-orange-300/70 px-1.5 text-[10px] font-normal text-orange-700 dark:border-orange-700/50 dark:text-orange-400"
			data-testid={`home-free-tier-valid-until-${provider.provider}`}
		>
			{t("freeTier.validUntil", { date: provider.free_valid_until })}
		</Badge>
	);

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
		// Configured means usable — light-blue treatment (not greyed out), with
		// the same health dot as the "你的提供商" topology card.
		const dotState = runtime ? computeDotState(runtime) : "missing";
		return (
			<Link
				to="/workspace/providers/$id"
				params={{ id: provider.provider }}
				className="flex h-full flex-col gap-2 rounded-md border border-sky-200/70 bg-sky-50/60 p-3 transition-colors hover:bg-sky-100/60 dark:border-sky-900/40 dark:bg-sky-950/20 dark:hover:bg-sky-950/30"
				data-testid={testid}
			>
				<div className="flex items-center justify-between gap-2">
					<span className="flex min-w-0 items-center gap-1.5 text-sm font-medium capitalize">
						<span className="truncate">{provider.provider}</span>
						<span
							className={cn("inline-block h-2 w-2 shrink-0 rounded-full", dotClass[dotState])}
							title={tc(`home.providers.dotState.${dotState}`)}
							data-testid={`home-free-tier-status-${provider.provider}`}
						/>
					</span>
					<Badge
						className="h-4 shrink-0 border-sky-300/60 bg-sky-100/80 px-1.5 text-[10px] font-normal text-sky-700 dark:border-sky-800/50 dark:bg-sky-950/40 dark:text-sky-300"
						data-testid={`home-free-tier-configured-${provider.provider}`}
					>
						{t("freeTier.configured")}
					</Badge>
					{freeValidBadge}
				</div>
				{modelBadges}
				{provider.notes && <p className="text-muted-foreground text-xs">{provider.notes}</p>}
				<div className="mt-auto flex justify-end">
					<span className="inline-flex items-center gap-1 text-xs font-medium text-sky-700 dark:text-sky-300">
						{t("freeTier.viewDetails")}
						<ChevronRight className="h-3.5 w-3.5" />
					</span>
				</div>
			</Link>
		);
	}

	// The server flagged this entry as unconfigurable on this gateway build
	// (unknown provider, no usable custom-provider fallback). Keep the card
	// visible but greyed out — the apply link stays so users can still sign
	// up upstream while the one-click configure entry point is removed.
	if (provider.supported === false) {
		return (
			<div className="bg-muted/20 flex h-full flex-col gap-2 rounded-md border border-dashed p-3 opacity-60" data-testid={testid}>
				<div className="flex items-center justify-between gap-2">
					<span className="text-muted-foreground text-sm font-medium capitalize">{provider.provider}</span>
					{customBadge}
					{freeValidBadge}
				</div>
				{modelBadges && <div className="opacity-70">{modelBadges}</div>}
				{provider.notes && <p className="text-muted-foreground/70 text-xs">{provider.notes}</p>}
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
					<span
						className="text-muted-foreground inline-flex items-center gap-1 text-xs"
						data-testid={`home-free-tier-unsupported-${provider.provider}`}
					>
						<Info className="h-3 w-3" />
						{t("freeTier.unsupportedHint")}
					</span>
				</div>
			</div>
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
			<div className="flex items-center justify-between gap-2">
				<span className="text-sm font-medium capitalize">{provider.provider}</span>
				{customBadge}
				{freeValidBadge}
			</div>
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
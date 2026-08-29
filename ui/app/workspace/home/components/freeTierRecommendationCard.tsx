import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useGetBundlesQuery } from "@/lib/store/apis/catalogApi";
import { useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { BundleProviderEntry } from "@/lib/types/catalog";
import { format } from "date-fns";
import { Gift, RefreshCw } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import FreeTierBundleSection from "./freeTierBundleSection";
import FreeTierOneKeyConfigDialog from "./freeTierOneKeyConfigDialog";
import FreeTierProviderCard from "./freeTierProviderCard";

/**
 * Empty/degraded state for the free-tier recommendation card. Shown when the
 * bundle list is empty, or when the upstream fetch failed (the backend always
 * answers 200, so the UI treats "no/empty data" as the degraded signal). The
 * retry button re-triggers the bundles refetch.
 */
function EmptyStateCard({ onRetry }: { onRetry: () => void }) {
	const { t } = useTranslation("home");
	return (
		<Card className="bg-card gap-0 border py-0 shadow-sm" data-testid="home-free-tier-empty">
			<CardContent className="flex flex-col items-center justify-center gap-3 py-10 text-center">
				<Gift className="text-muted-foreground h-5 w-5 opacity-60" />
				<p className="text-muted-foreground max-w-md text-sm">{t("freeTier.noBundles")}</p>
				<button
					type="button"
					onClick={onRetry}
					data-testid="home-free-tier-retry"
					className="border-input bg-background hover:bg-accent hover:text-accent-foreground inline-flex h-8 items-center justify-center gap-1 rounded-md border px-3 text-xs font-medium"
				>
					<RefreshCw className="h-3.5 w-3.5" />
					{t("freeTier.retry")}
				</button>
			</CardContent>
		</Card>
	);
}

/**
 * Free-tier recommendation card for the home page.
 *
 * Fetches GET /api/catalog/bundles?lang=<current locale> and renders one
 * bundle section per entry. Each bundle is a titled section with a flat grid
 * of provider cards underneath — no nested wrapping cards, so bundles with
 * very different provider counts no longer produce uneven heights.
 *
 * Empty results or fetch errors degrade to the empty state with a retry
 * button (V-ui-4).
 */
export default function FreeTierRecommendationCard() {
	const { t, i18n } = useTranslation("home");
	const { data, error, isSuccess, refetch } = useGetBundlesQuery({ lang: i18n.language });
	const { data: providers } = useGetProvidersQuery();

	// Providers already configured on this instance — their bundle rows get a
	// "configured" marker, become direct links to the provider detail page and
	// are sorted to the end of each bundle.
	const configuredProviders = new Set((providers ?? []).map((p) => p.name.toLowerCase()));

	// Single shared dialog so a click in any bundle opens it at the top level.
	const [dialogState, setDialogState] = useState<{ open: boolean; bundleId: string | null; provider: BundleProviderEntry | null }>({
		open: false,
		bundleId: null,
		provider: null,
	});
	const openDialog = (bundleId: string, provider: BundleProviderEntry) => setDialogState({ open: true, bundleId, provider });
	const closeDialog = (open: boolean) => {
		if (!open) setDialogState((s) => ({ ...s, open: false }));
	};

	if (!isSuccess || error || !data || data.bundles.length === 0) {
		return <EmptyStateCard onRetry={refetch} />;
	}

	return (
		<Card
			className="gap-0 overflow-hidden border-orange-200/70 bg-gradient-to-br from-orange-50/60 via-amber-50/40 to-rose-50/60 py-0 shadow-sm dark:border-orange-900/40 dark:from-orange-950/20 dark:via-amber-950/15 dark:to-rose-950/20"
			data-testid="home-free-tier-card"
		>
			<CardHeader className="flex flex-row items-start justify-between gap-2 border-b border-orange-200/60 bg-gradient-to-r from-orange-100/70 via-amber-100/50 to-transparent px-6 py-3 dark:border-orange-900/40 dark:from-orange-950/40 dark:via-amber-950/30 dark:to-transparent">
				<div className="min-w-0 flex-1">
					<div className="flex items-center gap-2">
						<Gift className="h-4 w-4 shrink-0 text-orange-500 dark:text-orange-400" aria-hidden />
						<CardTitle className="text-sm font-semibold">{t("freeTier.title")}</CardTitle>
						<Badge
							variant="warning"
							className="h-5 px-2 py-0 text-[10px] font-semibold tracking-wide uppercase"
							data-testid="home-free-tier-promo-badge"
						>
							{t("freeTier.promoBadge")}
						</Badge>
					</div>
					{data.updated_at && (
						<p className="text-muted-foreground mt-1 text-xs">
							{t("freeTier.updatedAt", { at: format(new Date(data.updated_at), "yyyy-MM-dd HH:mm:ss") })}
						</p>
					)}
				</div>
			</CardHeader>

			<CardContent className="space-y-6 px-6 py-4">
				{data.bundles.map((bundle) => {
					// Stable partition: unconfigured providers first (original order
					// preserved), configured ones moved to the back.
					const sortedProviders = [...bundle.providers].sort(
						(a, b) => Number(configuredProviders.has(a.provider.toLowerCase())) - Number(configuredProviders.has(b.provider.toLowerCase())),
					);
					return (
						<FreeTierBundleSection key={bundle.id} bundle={bundle} providerCount={sortedProviders.length}>
							<div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
								{sortedProviders.map((p) => (
									<FreeTierProviderCard
										key={p.provider}
										bundleId={bundle.id}
										provider={p}
										configured={configuredProviders.has(p.provider.toLowerCase())}
										onConfigure={(provider) => openDialog(bundle.id, provider)}
									/>
								))}
							</div>
						</FreeTierBundleSection>
					);
				})}
			</CardContent>

			{/* Mounted only while open so the dialog's mutations (and their RTK
			    hooks) do not run until the user actually opens the dialog. */}
			{dialogState.open && <FreeTierOneKeyConfigDialog open provider={dialogState.provider} onOpenChange={closeDialog} />}
		</Card>
	);
}
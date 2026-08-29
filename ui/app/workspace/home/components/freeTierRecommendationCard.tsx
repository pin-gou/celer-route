import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useGetBundlesQuery } from "@/lib/store/apis/catalogApi";
import { useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { format } from "date-fns";
import { RefreshCw, Sparkles } from "lucide-react";
import { useTranslation } from "react-i18next";
import BundleApplyCard from "./bundleApplyCard";

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
				<Sparkles className="text-muted-foreground h-5 w-5 opacity-60" />
				<p className="text-muted-foreground max-w-md text-sm">{t("freeTier.noBundles")}</p>
				<Button size="sm" variant="outline" onClick={onRetry} dataTestId="home-free-tier-retry">
					<RefreshCw className="mr-1 h-4 w-4" />
					{t("freeTier.retry")}
				</Button>
			</CardContent>
		</Card>
	);
}

/**
 * Free-tier recommendation card for the home page.
 *
 * Fetches GET /api/catalog/bundles?lang=<current locale> and renders one
 * bundle card per entry. Empty results or fetch errors degrade to the empty
 * state with a retry button (V-ui-4).
 */
export default function FreeTierRecommendationCard() {
	const { t, i18n } = useTranslation("home");
	const { data, error, isSuccess, refetch } = useGetBundlesQuery({ lang: i18n.language });
	const { data: providers } = useGetProvidersQuery();

	// Providers already configured on this instance — their bundle rows get a
	// "configured" marker, become direct links to the provider detail page and
	// are sorted to the end of each bundle.
	const configuredProviders = new Set((providers ?? []).map((p) => p.name.toLowerCase()));

	if (!isSuccess || error || !data || data.bundles.length === 0) {
		return <EmptyStateCard onRetry={refetch} />;
	}

	return (
		<Card className="bg-card gap-0 border py-0 shadow-sm" data-testid="home-free-tier-card">
			<CardHeader className="border-b px-6 py-3">
				<CardTitle className="text-sm font-semibold">{t("freeTier.title")}</CardTitle>
				{data.updated_at && (
					<CardDescription className="text-xs">
						{t("freeTier.updatedAt", { at: format(new Date(data.updated_at), "yyyy-MM-dd HH:mm:ss") })}
					</CardDescription>
				)}
			</CardHeader>
			<CardContent className="grid grid-cols-1 gap-4 px-6 py-4 md:grid-cols-2 lg:grid-cols-3">
				{data.bundles.map((bundle) => (
					<BundleApplyCard key={bundle.id} bundle={bundle} configuredProviders={configuredProviders} />
				))}
			</CardContent>
		</Card>
	);
}
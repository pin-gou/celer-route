import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useGetBundlesQuery } from "@/lib/store/apis/catalogApi";
import { RefreshCw, Sparkles } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useRecentRoutingRulesQuery } from "../hooks/useRecentRoutingRulesQuery";
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
 * bundle card per entry, plus the top 3 recently used routing rules as a
 * "heat" footer (V-ui-2). Empty results or fetch errors degrade to the empty
 * state with a retry button (V-ui-4).
 */
export default function FreeTierRecommendationCard() {
	const { t, i18n } = useTranslation("home");
	const { data, error, isSuccess, refetch } = useGetBundlesQuery({ lang: i18n.language });
	const recent = useRecentRoutingRulesQuery({ limit: 100 });

	if (!isSuccess || error || !data || data.bundles.length === 0) {
		return <EmptyStateCard onRetry={refetch} />;
	}

	return (
		<Card className="bg-card gap-0 border py-0 shadow-sm" data-testid="home-free-tier-card">
			<CardHeader className="border-b px-6 py-3">
				<CardTitle className="text-sm font-semibold">{t("freeTier.title")}</CardTitle>
				{data.updated_at && <CardDescription className="text-xs">{t("freeTier.updatedAt", { at: data.updated_at })}</CardDescription>}
			</CardHeader>
			<CardContent className="grid grid-cols-1 gap-4 px-6 py-4 md:grid-cols-2 lg:grid-cols-3">
				{data.bundles.map((bundle) => (
					<BundleApplyCard key={bundle.id} bundle={bundle} recentRules={recent.data?.rules.slice(0, 3)} />
				))}
			</CardContent>
		</Card>
	);
}
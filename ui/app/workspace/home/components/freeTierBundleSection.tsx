import { BundleEntry } from "@/lib/types/catalog";
import { Gift } from "lucide-react";
import { ReactNode } from "react";
import { useTranslation } from "react-i18next";

interface Props {
	bundle: BundleEntry;
	// Total number of providers rendered in this section (after the configured
	// sort). Drives the count chip next to the bundle title.
	providerCount: number;
	// Provider grid (or any content) rendered below the section header.
	children: ReactNode;
}

/**
 * Bundle section for the free-tier recommendation grid. Mirrors the
 * `ProviderFamilyGroup` pattern from /workspace/providers: a titled heading
 * above a flat grid of provider cards, no nested wrapping card.
 *
 * Visual style stays consistent with the rest of the free-tier block (orange
 * gradient + gift icon + bundle description). The provider count is rendered
 * on the same row as the title, right-aligned and visually emphasised.
 */
export default function FreeTierBundleSection({ bundle, providerCount, children }: Props) {
	const { t } = useTranslation("home");
	return (
		<section data-testid={`home-free-tier-bundle-${bundle.id}`} className="flex flex-col gap-3">
			<header className="flex items-center justify-between gap-3 rounded-md border border-orange-200/50 bg-gradient-to-br from-orange-100/60 via-amber-50/40 to-rose-100/40 px-4 py-3 dark:border-orange-900/30 dark:from-orange-950/30 dark:via-amber-950/20 dark:to-rose-950/30">
				<div className="flex min-w-0 items-center gap-2">
					<Gift className="h-3.5 w-3.5 shrink-0 text-orange-500 dark:text-orange-400" aria-hidden />
					<h3 className="shrink-0 text-sm font-semibold">{bundle.title}</h3>
					<span className="text-muted-foreground text-xs">·</span>
					<span className="text-muted-foreground truncate text-xs">{bundle.description}</span>
				</div>
				<span
					data-testid={`home-free-tier-bundle-${bundle.id}-count`}
					className="shrink-0 rounded-full bg-white/70 px-2.5 py-0.5 text-xs font-medium text-orange-700 tabular-nums ring-1 ring-orange-200/60 backdrop-blur-sm ring-inset dark:bg-white/10 dark:text-orange-300 dark:ring-orange-800/50"
				>
					{t("freeTier.providerCount", { count: providerCount })}
				</span>
			</header>
			{children}
		</section>
	);
}
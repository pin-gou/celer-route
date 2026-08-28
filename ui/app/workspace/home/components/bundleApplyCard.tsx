import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { BundleEntry, BundleProviderEntry, RecentRoutingRule } from "@/lib/types/catalog";
import { ExternalLink, KeyRound, MousePointerClick } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import FreeTierOneKeyConfigDialog from "./freeTierOneKeyConfigDialog";

interface Props {
	bundle: BundleEntry;
	recentRules?: RecentRoutingRule[];
}

/**
 * Single free-tier bundle card: title, description, provider rows, and a
 * "recently used routing rules" heat footer (top N from the logs aggregate).
 */
export default function BundleApplyCard({ bundle, recentRules }: Props) {
	const { t } = useTranslation("home");
	const [dialogState, setDialogState] = useState<{ open: boolean; provider: BundleProviderEntry | null }>({
		open: false,
		provider: null,
	});

	return (
		<Card className="bg-card gap-0 border py-0 shadow-sm" data-testid={`home-free-tier-bundle-${bundle.id}`}>
			<CardHeader className="border-b px-6 py-3">
				<CardTitle className="text-sm font-semibold">{bundle.title}</CardTitle>
				<CardDescription className="text-xs">{bundle.description}</CardDescription>
			</CardHeader>

			<CardContent className="space-y-3 px-6 py-4">
				{bundle.providers.map((p) => (
					<div
						key={p.provider}
						className="bg-muted/20 rounded-md border p-3"
						data-testid={`home-free-tier-provider-${bundle.id}-${p.provider}`}
					>
						<div className="flex flex-wrap items-center justify-between gap-2">
							<div className="min-w-0 space-y-0.5">
								<div className="text-sm font-medium capitalize">{p.provider}</div>
								{p.models.length > 0 && <div className="text-muted-foreground truncate text-xs">{p.models.join(", ")}</div>}
							</div>
							<div className="flex shrink-0 items-center gap-2">
								{p.apply_url && (
									<a
										href={p.apply_url}
										target="_blank"
										rel="noreferrer"
										className="text-primary hover:text-primary/80 inline-flex items-center gap-1 text-xs font-medium"
										data-testid={`home-free-tier-apply-${p.provider}`}
									>
										<ExternalLink className="h-3.5 w-3.5" />
										{t("freeTier.applyNow")}
									</a>
								)}
								<Button
									size="sm"
									variant={p.is_keyless ? "default" : "outline"}
									dataTestId={`home-free-tier-configure-${p.provider}`}
									onClick={() => setDialogState({ open: true, provider: p })}
								>
									{p.is_keyless ? <MousePointerClick className="h-3.5 w-3.5" /> : <KeyRound className="h-3.5 w-3.5" />}
									{p.is_keyless ? t("freeTier.keylessNote") : t("freeTier.configureNow")}
								</Button>
							</div>
						</div>
						{p.notes && <p className="text-muted-foreground mt-2 text-xs">{p.notes}</p>}
					</div>
				))}
			</CardContent>

			{recentRules && recentRules.length > 0 && (
				<CardFooter className="border-t px-6 py-3">
					<div className="w-full space-y-1.5">
						<div className="text-muted-foreground text-xs font-medium">{t("freeTier.recentRoutingRules")}</div>
						<ul className="space-y-1">
							{recentRules.map((r) => (
								<li key={r.id}>
									<a
										href={`/workspace/routing-rules/${r.id}`}
										className="hover:bg-muted/40 flex items-center gap-2 rounded-sm px-1 py-0.5 text-xs"
										data-testid={`home-free-tier-recent-rule-${r.id}`}
									>
										<span className="font-medium">{r.name}</span>
										<span className="text-muted-foreground">{r.use_count}</span>
										<span className="text-muted-foreground ml-auto">{r.last_used_at}</span>
									</a>
								</li>
							))}
						</ul>
					</div>
				</CardFooter>
			)}

			{/* Mounted only while open so the dialog's mutations (and their RTK
			    hooks) do not run until the user actually opens the dialog. */}
			{dialogState.open && (
				<FreeTierOneKeyConfigDialog open provider={dialogState.provider} onOpenChange={(open) => setDialogState((s) => ({ ...s, open }))} />
			)}
		</Card>
	);
}
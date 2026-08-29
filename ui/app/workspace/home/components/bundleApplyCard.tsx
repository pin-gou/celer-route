import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { BundleEntry, BundleProviderEntry } from "@/lib/types/catalog";
import { Link } from "@tanstack/react-router";
import { ChevronRight, ExternalLink, KeyRound, MousePointerClick } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import FreeTierOneKeyConfigDialog from "./freeTierOneKeyConfigDialog";

interface Props {
	bundle: BundleEntry;
	// Names of providers already configured on this instance (lowercased).
	configuredProviders: ReadonlySet<string>;
}

/**
 * Single free-tier bundle card: title, description, and provider rows.
 *
 * Providers already configured on the instance are marked with a badge,
 * link straight to the provider detail page, and are sorted to the end of the
 * provider list.
 */
export default function BundleApplyCard({ bundle, configuredProviders }: Props) {
	const { t } = useTranslation("home");
	const [dialogState, setDialogState] = useState<{ open: boolean; provider: BundleProviderEntry | null }>({
		open: false,
		provider: null,
	});

	const isConfigured = (name: string) => configuredProviders.has(name.toLowerCase());

	// Stable partition: unconfigured providers first (original order preserved),
	// configured ones moved to the back.
	const providers = [...bundle.providers].sort((a, b) => Number(isConfigured(a.provider)) - Number(isConfigured(b.provider)));

	return (
		<Card className="bg-card gap-0 border py-0 shadow-sm" data-testid={`home-free-tier-bundle-${bundle.id}`}>
			<CardHeader className="border-b px-6 py-3">
				<CardTitle className="text-sm font-semibold">{bundle.title}</CardTitle>
				<CardDescription className="text-xs">{bundle.description}</CardDescription>
			</CardHeader>

			<CardContent className="space-y-3 px-6 py-4">
				{providers.map((p) =>
					isConfigured(p.provider) ? (
						<Link
							key={p.provider}
							to="/workspace/providers/$id"
							params={{ id: p.provider }}
							className="bg-muted/20 hover:bg-muted/40 flex flex-col gap-2 rounded-md border p-3 transition-colors"
							data-testid={`home-free-tier-provider-${bundle.id}-${p.provider}`}
						>
							<div className="flex flex-wrap items-center justify-between gap-2">
								<div className="min-w-0 space-y-0.5">
									<div className="flex items-center gap-2">
										<span className="text-sm font-medium capitalize">{p.provider}</span>
										<Badge
											variant="secondary"
											className="text-muted-foreground h-4 px-1.5 text-[10px] font-normal"
											data-testid={`home-free-tier-configured-${p.provider}`}
										>
											{t("freeTier.configured")}
										</Badge>
									</div>
									{p.models.length > 0 && <div className="text-muted-foreground truncate text-xs">{p.models.join(", ")}</div>}
								</div>
								<span className="text-muted-foreground inline-flex shrink-0 items-center gap-1 text-xs font-medium">
									{t("freeTier.viewDetails")}
									<ChevronRight className="h-3.5 w-3.5" />
								</span>
							</div>
							{p.notes && <p className="text-muted-foreground text-xs">{p.notes}</p>}
						</Link>
					) : (
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
					),
				)}
			</CardContent>

			{/* Mounted only while open so the dialog's mutations (and their RTK
		    hooks) do not run until the user actually opens the dialog. */}
			{dialogState.open && (
				<FreeTierOneKeyConfigDialog open provider={dialogState.provider} onOpenChange={(open) => setDialogState((s) => ({ ...s, open }))} />
			)}
		</Card>
	);
}
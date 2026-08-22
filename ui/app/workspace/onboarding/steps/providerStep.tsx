import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { RenderProviderIcon } from "@/lib/constants/icons";
import { useCreateProviderMutation, useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { AddProviderRequest } from "@/lib/types/config";
import { getProviderLabel } from "@/lib/constants/logs";
import { CheckCircle2, ExternalLink, Loader2, Sparkles } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

const OPENCODE = "opencode" as const;
const KNOWN_PROVIDERS_COUNT = 40;

interface Props {
	onProviderAdded?: () => void;
}

export default function ProviderStep({ onProviderAdded }: Props) {
	const { t } = useTranslation("onboarding");
	const { data: providers } = useGetProvidersQuery();
	const [createProvider, { isLoading: isCreating }] = useCreateProviderMutation();

	const hasOpenCode = useMemo(() => (providers ?? []).some((p) => p.name.toLowerCase() === OPENCODE), [providers]);

	const [added, setAdded] = useState(hasOpenCode);
	const [error, setError] = useState<string | null>(null);

	const handleAddOpenCode = async () => {
		setError(null);
		if (hasOpenCode) {
			setAdded(true);
			onProviderAdded?.();
			return;
		}
		try {
			const payload: AddProviderRequest = { provider: OPENCODE };
			await createProvider(payload).unwrap();
			setAdded(true);
			onProviderAdded?.();
			toast.success(t("providerCard.addedOpenCode"));
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			setError(msg);
			toast.error(msg);
		}
	};

	return (
		<div className="space-y-5">
			<div className="space-y-1 text-center sm:text-left">
				<h2 className="text-xl font-semibold tracking-tight">{t("step.provider")}</h2>
				<p className="text-muted-foreground text-sm">{t("providerDesc")}</p>
			</div>

			<Card className="border-primary/30 bg-primary/5 gap-0 py-0" data-testid="onboarding-provider-opencode-card">
				<div className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
					<div className="flex items-start gap-3">
						<RenderProviderIcon provider={OPENCODE} size={32} className="mt-0.5 shrink-0" />
						<div className="space-y-1">
							<div className="flex items-center gap-2">
								<span className="text-sm font-semibold">{getProviderLabel(OPENCODE)}</span>
								<span className="bg-primary/10 text-primary rounded-full px-2 py-0.5 text-[10px] font-medium">
									{t("providerCard.recommended")}
								</span>
							</div>
							<div className="flex items-center gap-1.5">
								<Sparkles className="text-primary h-3.5 w-3.5" />
								<span className="text-muted-foreground text-xs">{t("providerCard.keylessBadge")}</span>
							</div>
						</div>
					</div>
					<div className="flex items-center sm:shrink-0">
						<Button
							size="sm"
							variant={added ? "secondary" : "default"}
							onClick={() => void handleAddOpenCode()}
							disabled={isCreating || added}
							data-testid="onboarding-provider-add-opencode"
						>
							{isCreating ? (
								<Loader2 className="h-4 w-4 animate-spin" />
							) : added ? (
								<>
									<CheckCircle2 className="mr-1 h-4 w-4 text-emerald-500" />
									{t("providerCard.addedOpenCode")}
								</>
							) : (
								t("providerCard.addOpenCode")
							)}
						</Button>
					</div>
				</div>
				{error && (
					<div className="text-destructive border-t px-4 py-2 text-xs" role="alert">
						{error}
					</div>
				)}
			</Card>

			<div className="bg-muted/20 rounded-md border border-dashed p-4">
				<div className="space-y-2">
					<div className="text-sm font-medium">{t("providerCard.otherProvidersLabel")}</div>
					<p className="text-muted-foreground text-xs">{t("providerCard.otherProvidersDesc", { count: String(KNOWN_PROVIDERS_COUNT) })}</p>
					<Button size="sm" variant="outline" asChild>
						<Link to="/workspace/providers" data-testid="onboarding-provider-open-providers-page">
							<ExternalLink className="mr-1 h-4 w-4" />
							{t("providerCard.openProvidersPage")}
						</Link>
					</Button>
				</div>
			</div>
		</div>
	);
}
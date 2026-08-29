import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { DefaultNetworkConfig } from "@/lib/constants/config";
import { catalogApi, useCreateProviderKeyMutation, useCreateProviderMutation } from "@/lib/store/apis/catalogApi";
import { BundleProviderEntry } from "@/lib/types/catalog";
import { KnownProvider } from "@/lib/types/config";
import { ExternalLink, Info, KeyRound, ListChecks, Plus, Sparkles, X } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

interface Props {
	open: boolean;
	provider: BundleProviderEntry | null;
	onOpenChange?: (open: boolean) => void;
}

/**
 * One-click provider/key configuration dialog for a bundle row.
 *
 * Submission flow (design.md §4):
 *   1) POST /api/providers            — create the provider (409 → already exists, continue)
 *   2) POST /api/providers/{p}/keys   — add the API key (skipped for keyless providers)
 *   3) success toast + invalidate CatalogBundles so the catalog refetches
 */
export default function FreeTierOneKeyConfigDialog({ open, provider, onOpenChange }: Props) {
	const { t } = useTranslation("home");
	const [createProvider, { isLoading: isCreatingProvider }] = useCreateProviderMutation();
	const [createKey, { isLoading: isCreatingKey }] = useCreateProviderKeyMutation();
	const [apiKey, setApiKey] = useState("");

	const isKeyless = provider?.is_keyless ?? false;
	// The server keeps base_provider only on entries this build does not ship
	// natively — those are created through the custom-provider fallback.
	const isCustom = !!provider?.base_provider;
	const isSubmitting = isCreatingProvider || isCreatingKey;

	const handleSubmit = async () => {
		if (!provider) return;
		if (!isKeyless && !apiKey.trim()) return;

		try {
			try {
				await createProvider(
					isCustom
						? {
								provider: provider.provider,
								custom_provider_config: {
									base_provider_type: provider.base_provider as KnownProvider,
									is_key_less: isKeyless,
								},
								network_config: {
									...DefaultNetworkConfig,
									base_url: provider.base_url ?? "",
								},
							}
						: { provider: provider.provider },
				).unwrap();
			} catch (e) {
				if ((e as { status?: number })?.status === 409) {
					// Provider already configured — fall through and add the key.
				} else {
					throw e;
				}
			}
			if (!isKeyless && apiKey) {
				await createKey({ provider: provider.provider, key: apiKey }).unwrap();
			}
			toast.success(t("freeTier.configSuccess"));
			catalogApi.util.invalidateTags(["CatalogBundles"]);
			setApiKey("");
			onOpenChange?.(false);
		} catch {
			toast.error(t("freeTier.configFailed"));
		}
	};

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				data-testid="home-free-tier-config-dialog"
				// Wide dialog capped at min(80vw, 720px); since 80vw never
				// exceeds the viewport, no extra clamp is needed. overflow-x-hidden
				// is the final guard for inner content.
				className="overflow-x-hidden sm:max-w-[min(80vw,720px)]"
			>
				<DialogHeader>
					<DialogTitle className="capitalize">{provider?.provider}</DialogTitle>
					<DialogDescription className="break-words">{provider?.notes || t("freeTier.configureNow")}</DialogDescription>
					{isCustom && (
						<DialogDescription data-testid="home-free-tier-custom-hint" className="break-words text-orange-600/90 dark:text-orange-400/90">
							{t("freeTier.customProviderHint", { protocol: provider?.base_provider ?? "" })}
						</DialogDescription>
					)}
				</DialogHeader>

				{/* 1) Promo highlight: recommended models (the offer summary lives
				    in DialogDescription above via `provider.notes`). */}
				<div className="space-y-1.5 rounded-md border border-orange-200/60 bg-gradient-to-br from-orange-50/60 via-amber-50/40 to-rose-50/60 px-3 py-2 text-xs leading-relaxed dark:border-orange-900/40 dark:from-orange-950/20 dark:via-amber-950/15 dark:to-rose-950/20">
					{isKeyless ? (
						<div className="flex items-start gap-2">
							<Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-orange-500 dark:text-orange-400" aria-hidden />
							<span className="text-muted-foreground">{t("freeTier.applyHintKeyless")}</span>
						</div>
					) : (
						<div className="flex items-center gap-1.5">
							<Sparkles className="h-3.5 w-3.5 shrink-0 text-orange-500 dark:text-orange-400" aria-hidden />
							<span className="text-foreground/80 font-medium">{t("freeTier.models")}:</span>
							{provider?.models && provider.models.length > 0 ? (
								<span className="text-muted-foreground min-w-0 truncate">{provider.models.join(", ")}</span>
							) : (
								<span className="text-muted-foreground min-w-0 truncate">{provider?.provider ?? ""}</span>
							)}
						</div>
					)}
				</div>

				{/* 2) Apply steps (keyed providers only). Skipped when keyless or
				    when the catalog did not provide any steps. */}
				{!isKeyless && provider?.apply_steps && provider.apply_steps.length > 0 && (
					<div className="space-y-1.5">
						<div className="text-foreground/80 flex items-center gap-1.5 text-xs font-medium">
							<ListChecks className="text-muted-foreground h-3.5 w-3.5" aria-hidden />
							{t("freeTier.applySteps")}
						</div>
						<ol className="text-muted-foreground space-y-1 pl-5 text-xs leading-relaxed">
							{provider.apply_steps.map((step, idx) => (
								<li key={idx} className="list-decimal break-words">
									{step}
								</li>
							))}
						</ol>
					</div>
				)}

				{/* 3) "Apply" CTA — placed right above the API key input so the
				    journey reads: steps → apply link → paste key. Keyless providers
				    have no sign-up flow and skip this. */}
				{!isKeyless && provider?.apply_url && (
					<a
						href={provider.apply_url}
						target="_blank"
						rel="noreferrer"
						className="inline-flex w-full items-center justify-center gap-1 rounded-md border border-orange-200/60 bg-orange-50/60 px-3 py-1.5 text-xs font-medium text-orange-600 transition-colors hover:bg-orange-100/80 hover:text-orange-700 dark:border-orange-900/40 dark:bg-orange-950/30 dark:text-orange-400 dark:hover:bg-orange-950/50"
						data-testid="home-free-tier-apply-dialog"
					>
						<ExternalLink className="h-3.5 w-3.5" />
						{t("freeTier.applyNow")}
					</a>
				)}

				{/* 4) API key input (keyless providers skip this entirely). */}
				{isKeyless ? (
					<p className="text-muted-foreground text-sm">{t("freeTier.keylessNote")}</p>
				) : (
					<div className="space-y-2">
						<label htmlFor="home-free-tier-key-input" className="flex items-center gap-1.5 text-sm font-medium">
							<KeyRound className="text-muted-foreground h-3.5 w-3.5" aria-hidden />
							{t("freeTier.keyInputLabel")}
						</label>
						<Input
							id="home-free-tier-key-input"
							data-testid="home-free-tier-key-input"
							value={apiKey}
							onChange={(e) => setApiKey(e.target.value)}
							placeholder={t("freeTier.apiKeyPlaceholder", { provider: provider?.provider ?? "" })}
							autoComplete="off"
						/>
					</div>
				)}

				<DialogFooter className="gap-2 sm:justify-end">
					<Button
						type="button"
						variant="outline"
						onClick={() => onOpenChange?.(false)}
						disabled={isSubmitting}
						dataTestId="home-free-tier-cancel"
					>
						<X className="h-3.5 w-3.5" />
						{t("freeTier.cancel")}
					</Button>
					<Button
						type="button"
						className="bg-orange-500 text-white hover:bg-orange-600 dark:bg-orange-500 dark:hover:bg-orange-400"
						dataTestId="home-free-tier-submit"
						onClick={handleSubmit}
						isLoading={isSubmitting}
						disabled={!isKeyless && !apiKey.trim()}
					>
						<Plus className="h-3.5 w-3.5" />
						{t("freeTier.submit")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
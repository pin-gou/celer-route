import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { catalogApi, useCreateProviderKeyMutation, useCreateProviderMutation } from "@/lib/store/apis/catalogApi";
import { BundleProviderEntry } from "@/lib/types/catalog";
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
	const isSubmitting = isCreatingProvider || isCreatingKey;

	const handleSubmit = async () => {
		if (!provider) return;
		if (!isKeyless && !apiKey.trim()) return;

		try {
			try {
				await createProvider({ provider: provider.provider }).unwrap();
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
			<DialogContent data-testid="home-free-tier-config-dialog">
				<DialogHeader>
					<DialogTitle>{provider?.provider}</DialogTitle>
					<DialogDescription>{provider?.notes || t("freeTier.configureNow")}</DialogDescription>
				</DialogHeader>

				{isKeyless ? (
					<p className="text-muted-foreground text-sm">{t("freeTier.keylessNote")}</p>
				) : (
					<div className="space-y-2">
						<label htmlFor="home-free-tier-key-input" className="text-sm font-medium">
							{t("freeTier.keyInputLabel")}
						</label>
						<Input
							id="home-free-tier-key-input"
							data-testid="home-free-tier-key-input"
							value={apiKey}
							onChange={(e) => setApiKey(e.target.value)}
							placeholder={t("freeTier.apiKeyPlaceholder")}
							autoComplete="off"
						/>
					</div>
				)}

				<DialogFooter>
					<Button
						type="button"
						variant="outline"
						onClick={() => onOpenChange?.(false)}
						disabled={isSubmitting}
						dataTestId="home-free-tier-cancel"
					>
						{t("freeTier.cancel")}
					</Button>
					<Button
						type="button"
						dataTestId="home-free-tier-submit"
						onClick={handleSubmit}
						isLoading={isSubmitting}
						disabled={!isKeyless && !apiKey.trim()}
					>
						{t("freeTier.submit")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
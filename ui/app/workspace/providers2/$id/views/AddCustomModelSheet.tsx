import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { DottedSeparator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { getErrorMessage, useUpsertModelCatalogEntriesMutation } from "@/lib/store";
import { ModelProvider } from "@/lib/types/config";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useTranslation } from "react-i18next";
import { useState } from "react";
import { toast } from "sonner";

interface AddCustomModelSheetProps {
	provider: ModelProvider;
	onClose: () => void;
}

// AddCustomModelSheet registers a model that was never discovered by a
// provider key. It seeds a governance_model_pricing row (mode is part of the
// (model, provider, mode) natural key) so the model becomes resolvable, then
// drops the user back to the list.
export function AddCustomModelSheet({ provider, onClose }: AddCustomModelSheetProps) {
	const { t } = useTranslation("providers");
	const [isOpen, setIsOpen] = useState(true);
	const hasCreateAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Create);

	const MODE_OPTIONS = [
		{ value: "chat", label: t("providers2.addCustomModelSheet.modeOptions.chat") },
		{ value: "text", label: t("providers2.addCustomModelSheet.modeOptions.text") },
		{ value: "embedding", label: t("providers2.addCustomModelSheet.modeOptions.embedding") },
		{ value: "image", label: t("providers2.addCustomModelSheet.modeOptions.image") },
		{ value: "audio", label: t("providers2.addCustomModelSheet.modeOptions.audio") },
	];
	const [modelId, setModelId] = useState("");
	const [mode, setMode] = useState("chat");
	const [upsertEntries, { isLoading }] = useUpsertModelCatalogEntriesMutation();

	const handleClose = () => {
		setIsOpen(false);
		setTimeout(onClose, 150);
	};

	const handleSubmit = async () => {
		const id = modelId.trim();
		if (!id) {
			toast.error(t("providers2.addCustomModelSheet.toast.modelIdRequired"));
			return;
		}
		try {
			await upsertEntries([
				{
					model: id,
					provider: provider.name,
					mode,
				},
			]).unwrap();
			toast.success(t("providers2.addCustomModelSheet.toast.modelRegistered", { id }));
			handleClose();
		} catch (err) {
			toast.error(t("providers2.addCustomModelSheet.toast.failedToRegister"), { description: getErrorMessage(err) });
		}
	};

	return (
		<Sheet open={isOpen} onOpenChange={(open) => !open && handleClose()}>
			<SheetContent className="flex w-full flex-col overflow-x-hidden pt-4" data-testid="providers2-add-model-sheet">
				<SheetHeader className="flex flex-col items-start p-0 px-8 py-4" headerClassName="mb-0 sticky -top-4 bg-card z-10">
					<SheetTitle>{t("providers2.addCustomModelSheet.title")}</SheetTitle>
					<SheetDescription>
						{t("providers2.addCustomModelSheet.description")}
					</SheetDescription>
				</SheetHeader>

				<div className="flex h-full flex-col gap-6">
					<div className="grow space-y-4 px-8">
						<div>
							<Label className="text-sm font-medium">{t("providers2.addCustomModelSheet.providerLabel")}</Label>
							<div className="bg-muted/30 mt-2 rounded-sm border px-3 py-2 text-sm">{provider.name}</div>
						</div>

						<div>
							<Label className="text-sm font-medium">{t("providers2.addCustomModelSheet.modelIdLabel")}</Label>
							<Input
								data-testid="providers2-add-model-id"
								className="mt-2 font-mono"
								value={modelId}
								onChange={(e) => setModelId(e.target.value)}
								onKeyDown={(e) => e.key === "Enter" && handleSubmit()}
								placeholder={t("providers2.addCustomModelSheet.modelIdPlaceholder")}
							/>
							<p className="text-muted-foreground mt-1 text-xs">
								{t("providers2.addCustomModelSheet.modelIdHint")}
							</p>
						</div>

						<div>
							<Label className="text-sm font-medium">{t("providers2.addCustomModelSheet.modeLabel")}</Label>
							<select
								value={mode}
								onChange={(e) => setMode(e.target.value)}
								className="border-input mt-2 w-full rounded-md border bg-background px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-blue-500"
								data-testid="providers2-add-model-mode"
							>
								{MODE_OPTIONS.map((opt) => (
									<option key={opt.value} value={opt.value}>
										{opt.label}
									</option>
								))}
							</select>
						</div>

						<DottedSeparator />
					</div>

					<div className="bg-card sticky bottom-0 shrink-0 border-t px-8 py-4">
						<div className="flex items-center justify-end gap-3">
							{!hasCreateAccess && <p className="text-destructive text-sm">{t("providers2.addCustomModelSheet.noPermission")}</p>}
							<Button type="button" variant="outline" onClick={handleClose} data-testid="providers2-add-model-cancel">
								{t("providers2.addCustomModelSheet.cancel")}
							</Button>
							<Button
								type="button"
								onClick={handleSubmit}
								disabled={isLoading || !modelId.trim() || !hasCreateAccess}
								data-testid="providers2-add-model-submit"
							>
								{isLoading ? t("providers2.addCustomModelSheet.saving") : t("providers2.addCustomModelSheet.addModel")}
							</Button>
						</div>
					</div>
				</div>
			</SheetContent>
		</Sheet>
	);
}
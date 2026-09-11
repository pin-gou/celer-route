import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { getErrorMessage, ModelDetails, useRenameModelCatalogEntryMutation } from "@/lib/store";
import { ModelProvider } from "@/lib/types/config";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useTranslation } from "react-i18next";
import { useState } from "react";
import { toast } from "sonner";

interface RenameCustomModelSheetProps {
	provider: ModelProvider;
	model: ModelDetails;
	onClose: () => void;
}

// RenameCustomModelSheet renames a manually-added model's pricing rows. Only
// custom (is_custom) models reach here; the backend enforces that too.
export function RenameCustomModelSheet({ provider, model, onClose }: RenameCustomModelSheetProps) {
	const { t } = useTranslation("providers");
	const [isOpen, setIsOpen] = useState(true);
	const hasUpdateAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);
	const [newModel, setNewModel] = useState(model.name);
	const [renameEntry, { isLoading }] = useRenameModelCatalogEntryMutation();

	const handleClose = () => {
		setIsOpen(false);
		setTimeout(onClose, 150);
	};

	const handleSubmit = async () => {
		const id = newModel.trim();
		if (!id) {
			toast.error(t("providers2.renameCustomModelSheet.toast.modelNameRequired"));
			return;
		}
		if (id === model.name) {
			toast.error(t("providers2.renameCustomModelSheet.toast.sameName"));
			return;
		}
		try {
			await renameEntry({ model: model.name, provider: provider.name, new_model: id }).unwrap();
			toast.success(t("providers2.renameCustomModelSheet.toast.modelRenamed", { id }));
			handleClose();
		} catch (err) {
			toast.error(t("providers2.renameCustomModelSheet.toast.failedToRename"), { description: getErrorMessage(err) });
		}
	};

	return (
		<Sheet open={isOpen} onOpenChange={(open) => !open && handleClose()}>
			<SheetContent className="flex w-full flex-col overflow-x-hidden pt-4" data-testid="providers2-rename-model-sheet">
				<SheetHeader className="flex flex-col items-start p-0 px-8 py-4" headerClassName="mb-0 sticky -top-4 bg-card z-10">
					<SheetTitle>{t("providers2.renameCustomModelSheet.title")}</SheetTitle>
					<SheetDescription>{t("providers2.renameCustomModelSheet.description")}</SheetDescription>
				</SheetHeader>

				<div className="flex h-full flex-col gap-6">
					<div className="grow space-y-4 px-8">
						<div>
							<Label className="text-sm font-medium">{t("providers2.renameCustomModelSheet.providerLabel")}</Label>
							<div className="bg-muted/30 mt-2 rounded-sm border px-3 py-2 text-sm">{provider.name}</div>
						</div>

						<div>
							<Label className="text-sm font-medium">{t("providers2.renameCustomModelSheet.currentModelLabel")}</Label>
							<div className="bg-muted/30 mt-2 rounded-sm border px-3 py-2 font-mono text-sm">{model.name}</div>
						</div>

						<div>
							<Label className="text-sm font-medium">{t("providers2.renameCustomModelSheet.newModelLabel")}</Label>
							<Input
								data-testid="providers2-rename-model-id"
								className="mt-2 font-mono"
								value={newModel}
								onChange={(e) => setNewModel(e.target.value)}
								onKeyDown={(e) => e.key === "Enter" && handleSubmit()}
								placeholder={t("providers2.renameCustomModelSheet.newModelPlaceholder")}
							/>
							<p className="text-muted-foreground mt-1 text-xs">{t("providers2.renameCustomModelSheet.newModelHint")}</p>
						</div>
					</div>

					<div className="bg-card sticky bottom-0 shrink-0 border-t px-8 py-4">
						<div className="flex items-center justify-end gap-3">
							{!hasUpdateAccess && <p className="text-destructive text-sm">{t("providers2.renameCustomModelSheet.noPermission")}</p>}
							<Button type="button" variant="outline" onClick={handleClose} data-testid="providers2-rename-model-cancel">
								{t("providers2.renameCustomModelSheet.cancel")}
							</Button>
							<Button
								type="button"
								onClick={handleSubmit}
								disabled={isLoading || !newModel.trim() || !hasUpdateAccess}
								data-testid="providers2-rename-model-submit"
							>
								{isLoading ? t("providers2.renameCustomModelSheet.saving") : t("providers2.renameCustomModelSheet.rename")}
							</Button>
						</div>
					</div>
				</div>
			</SheetContent>
		</Sheet>
	);
}
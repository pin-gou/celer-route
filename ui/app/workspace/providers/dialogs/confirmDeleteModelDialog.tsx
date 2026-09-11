import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alertDialog";
import { getErrorMessage, ModelDetails, useDeleteModelCatalogEntryMutation } from "@/lib/store";
import { ModelProvider } from "@/lib/types/config";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

interface Props {
	show: boolean;
	model: ModelDetails;
	provider: ModelProvider;
	onCancel: () => void;
	onDelete: () => void;
}

// ConfirmDeleteModelDialog removes a manually-added model's pricing rows. Only
// custom (is_custom) models reach here; the backend enforces that too.
export default function ConfirmDeleteModelDialog({ show, model, provider, onCancel, onDelete }: Props) {
	const { t } = useTranslation("providers");
	const [deleteEntry, { isLoading: isDeleting }] = useDeleteModelCatalogEntryMutation();
	const hasDeleteAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Delete);

	const onDeleteHandler = () => {
		deleteEntry({ model: model.name, provider: provider.name })
			.unwrap()
			.then(() => {
				toast.success(t("providers2.modelsTab.toast.modelDeleted", { model: model.name }));
				onDelete();
			})
			.catch((err) => {
				toast.error(t("providers2.modelsTab.toast.failedToDeleteModel", { model: model.name }), {
					description: getErrorMessage(err),
				});
			});
	};

	return (
		<AlertDialog open={show}>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>{t("providers2.deleteModelConfirm.title")}</AlertDialogTitle>
					<AlertDialogDescription>
						{t("providers2.deleteModelConfirm.description", { model: model.name, provider: provider.name })}
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel onClick={onCancel}>{t("providers2.deleteModelConfirm.cancel")}</AlertDialogCancel>
					<AlertDialogAction onClick={onDeleteHandler} disabled={isDeleting || !hasDeleteAccess}>
						{isDeleting ? t("providers2.deleteModelConfirm.deleting") : t("providers2.deleteModelConfirm.confirm")}
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
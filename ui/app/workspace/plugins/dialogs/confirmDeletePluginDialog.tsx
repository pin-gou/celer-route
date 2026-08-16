import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
} from "@/components/ui/alertDialog";
import { getErrorMessage, useDeletePluginMutation } from "@/lib/store";
import { Plugin } from "@/lib/types/plugins";
import { AlertDialogTitle } from "@radix-ui/react-alert-dialog";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

interface Props {
	show: boolean;
	onCancel: () => void;
	onDelete: () => void;
	plugin: Plugin;
}

export default function ConfirmDeletePluginDialog({ show, onCancel, onDelete, plugin }: Props) {
	const { t } = useTranslation("plugins");
	const [deletePlugin, { isLoading: isDeletingPlugin }] = useDeletePluginMutation();

	const onDeleteHandler = () => {
		deletePlugin(plugin.name)
			.unwrap()
			.then(() => {
				onDelete();
			})
			.catch((err) => {
				toast.error(t("deleteDialog.errorToast"), {
					description: getErrorMessage(err),
				});
			});
	};

	return (
		<AlertDialog open={show}>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>{t("deleteDialog.title")}</AlertDialogTitle>
					<AlertDialogDescription>{t("deleteDialog.description", { name: plugin.name })}</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel onClick={onCancel}>{t("deleteDialog.cancel")}</AlertDialogCancel>
					<AlertDialogAction onClick={onDeleteHandler} disabled={isDeletingPlugin}>
						{isDeletingPlugin ? t("deleteDialog.deleting") : t("deleteDialog.delete")}
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
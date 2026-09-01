import Provider from "@/components/provider";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { ModelProvider } from "@/lib/types/config";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import ProviderKeyForm from "../views/providerKeyForm";

interface Props {
	show: boolean;
	onCancel: () => void;
	provider: ModelProvider;
	keyId: string | null;
	providerName?: string;
}

export default function AddNewKeySheet({ show, onCancel, provider, keyId, providerName }: Props) {
	const { t } = useTranslation("providers");
	const isEditing = keyId !== null;
	const resolvedProviderName = (providerName ?? provider.name).toLowerCase();
	const isVLLM = resolvedProviderName === "vllm";
	const isOllamaOrSGL = resolvedProviderName === "ollama" || resolvedProviderName === "sgl";
	const entityLabel = isVLLM ? "model" : isOllamaOrSGL ? "server" : "key";
	const titleKey = isEditing
		? entityLabel === "model"
			? "providers2.keyForm.sheet.editModelTitle"
			: entityLabel === "server"
				? "providers2.keyForm.sheet.editServerTitle"
				: "providers2.keyForm.sheet.editTitle"
		: entityLabel === "model"
			? "providers2.keyForm.sheet.addModelTitle"
			: entityLabel === "server"
				? "providers2.keyForm.sheet.addServerTitle"
				: "providers2.keyForm.sheet.addTitle";
	const toastKey = isEditing ? "providers2.keyForm.toast.keyUpdated" : "providers2.keyForm.toast.keyAdded";

	return (
		<Sheet
			open={show}
			onOpenChange={(open) => {
				if (!open) onCancel();
			}}
		>
			<SheetContent className="p-0 pt-4" data-testid="key-form" onInteractOutside={(e) => e.preventDefault()}>
				<SheetHeader className="flex flex-col items-start px-8 py-4" headerClassName="mb-0 sticky -top-4 bg-card z-10">
					<SheetTitle>
						<div className="font-lg flex items-center gap-2">
							<div className={"flex items-center"}>
								<Provider provider={provider.name} size={24} />:
							</div>
							{t(titleKey)}
						</div>
					</SheetTitle>
				</SheetHeader>
				<ProviderKeyForm
					provider={provider}
					keyId={keyId}
					onCancel={onCancel}
					onSave={() => {
						toast.success(t(toastKey));
						onCancel();
					}}
				/>
			</SheetContent>
		</Sheet>
	);
}
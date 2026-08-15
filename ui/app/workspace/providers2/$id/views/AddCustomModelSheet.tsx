import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { DottedSeparator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { getErrorMessage, useUpsertModelCatalogEntriesMutation } from "@/lib/store";
import { ModelProvider } from "@/lib/types/config";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useState } from "react";
import { toast } from "sonner";

interface AddCustomModelSheetProps {
	provider: ModelProvider;
	onClose: () => void;
}

const MODE_OPTIONS = [
	{ value: "chat", label: "Chat completions" },
	{ value: "text", label: "Text completions" },
	{ value: "embedding", label: "Embeddings" },
	{ value: "image", label: "Images" },
	{ value: "audio", label: "Audio" },
];

// AddCustomModelSheet registers a model that was never discovered by a
// provider key. It seeds a governance_model_pricing row (mode is part of the
// (model, provider, mode) natural key) so the model becomes resolvable, then
// drops the user back to the list.
export function AddCustomModelSheet({ provider, onClose }: AddCustomModelSheetProps) {
	const [isOpen, setIsOpen] = useState(true);
	const hasCreateAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Create);
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
			toast.error("Model ID is required");
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
			toast.success(`Registered model ${id}`);
			handleClose();
		} catch (err) {
			toast.error("Failed to register model", { description: getErrorMessage(err) });
		}
	};

	return (
		<Sheet open={isOpen} onOpenChange={(open) => !open && handleClose()}>
			<SheetContent className="flex w-full flex-col overflow-x-hidden pt-4" data-testid="providers2-add-model-sheet">
				<SheetHeader className="flex flex-col items-start p-0 px-8 py-4" headerClassName="mb-0 sticky -top-4 bg-card z-10">
					<SheetTitle>Add Custom Model</SheetTitle>
					<SheetDescription>
						Register a model ID that was not discovered by a provider key. The model is added to the pricing catalog so it becomes
						routable; set pricing and limits via Edit on the model row afterwards.
					</SheetDescription>
				</SheetHeader>

				<div className="flex h-full flex-col gap-6">
					<div className="grow space-y-4 px-8">
						<div>
							<Label className="text-sm font-medium">Provider</Label>
							<div className="bg-muted/30 mt-2 rounded-sm border px-3 py-2 text-sm">{provider.name}</div>
						</div>

						<div>
							<Label className="text-sm font-medium">Model ID</Label>
							<Input
								data-testid="providers2-add-model-id"
								className="mt-2 font-mono"
								value={modelId}
								onChange={(e) => setModelId(e.target.value)}
								onKeyDown={(e) => e.key === "Enter" && handleSubmit()}
								placeholder="e.g. my-selfhosted-model"
							/>
							<p className="text-muted-foreground mt-1 text-xs">
								The exact model ID your clients send in the `model` field. Must match what the provider API serves.
							</p>
						</div>

						<div>
							<Label className="text-sm font-medium">Mode</Label>
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
							{!hasCreateAccess && <p className="text-destructive text-sm">You don't have permission to perform this action</p>}
							<Button type="button" variant="outline" onClick={handleClose} data-testid="providers2-add-model-cancel">
								Cancel
							</Button>
							<Button
								type="button"
								onClick={handleSubmit}
								disabled={isLoading || !modelId.trim() || !hasCreateAccess}
								data-testid="providers2-add-model-submit"
							>
								{isLoading ? "Saving..." : "Add Model"}
							</Button>
						</div>
					</div>
				</div>
			</SheetContent>
		</Sheet>
	);
}
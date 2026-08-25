import { useGetProviderErrorCatalogQuery } from "@/lib/store/apis/providerErrorCatalogApi";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { PlusIcon, Trash2Icon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useState } from "react";

interface CooldownTypeSelectProps {
	label: string;
	provider: string;
	values: string[];
	field: "type" | "code";
	onChange: (values: string[]) => void;
}

export function CooldownTypeSelect({ label, provider, values, field, onChange }: CooldownTypeSelectProps) {
	const { t } = useTranslation("providers");
	const { data: catalog } = useGetProviderErrorCatalogQuery(provider, { skip: !provider });
	const options = catalog?.[field === "type" ? "types" : "codes"] ?? [];

	const addItem = () => {
		onChange([...values, ""]);
	};

	const updateItem = (index: number, value: string) => {
		const next = [...values];
		next[index] = value;
		onChange(next);
	};

	const removeItem = (index: number) => {
		onChange(values.filter((_, i) => i !== index));
	};

	return (
		<div className="space-y-1">
			<div className="flex items-center justify-between">
				<span className="text-xs font-medium">{label}</span>
				<Button type="button" variant="outline" size="sm" onClick={addItem} data-testid={`cooldown-${field}-add`}>
					<PlusIcon className="mr-1 h-3 w-3" />
					{t("fragments.cooldownPolicy.addItem")}
				</Button>
			</div>

			{values.length === 0 && <p className="text-muted-foreground text-xs">{t("fragments.cooldownPolicy.noItems")}</p>}

			{values.map((v, i) => {
				const isKnown = !v || options.includes(v);
				return (
					<div key={i} className="flex items-center gap-2">
						{isKnown ? (
							<Select value={v} onValueChange={(nv) => updateItem(i, nv)}>
								<SelectTrigger className="h-8 flex-1 text-xs" data-testid={`cooldown-${field}-select-${i}`}>
									<SelectValue placeholder={t("fragments.cooldownPolicy.typeSelect.selectPlaceholder")} />
								</SelectTrigger>
								<SelectContent>
									{options.map((opt) => (
										<SelectItem key={opt} value={opt}>
											{opt}
										</SelectItem>
									))}
									<SelectItem value="__custom__">{t("fragments.cooldownPolicy.typeSelect.custom")}</SelectItem>
								</SelectContent>
							</Select>
						) : (
							<div className="flex flex-1 items-center gap-1">
								<Input
									className="h-8 flex-1 text-xs"
									value={v}
									onChange={(e) => updateItem(i, e.target.value)}
									placeholder={t("fragments.cooldownPolicy.typeSelect.customPlaceholder")}
									data-testid={`cooldown-${field}-custom-input-${i}`}
								/>
								<Button
									type="button"
									variant="ghost"
									size="sm"
									onClick={() => updateItem(i, "")}
									data-testid={`cooldown-${field}-custom-reset-${i}`}
								>
									{t("fragments.cooldownPolicy.typeSelect.reset")}
								</Button>
							</div>
						)}
						<Button type="button" variant="ghost" size="sm" onClick={() => removeItem(i)} data-testid={`cooldown-${field}-remove-${i}`}>
							<Trash2Icon className="h-4 w-4" />
						</Button>
					</div>
				);
			})}
		</div>
	);
}
import type { ReactNode } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";

// Presentational primitives shared by the three cooldown capability field
// groups. They are deliberately context-free (no react-hook-form) so the same
// components can be embedded in the provider detail form.
//
// Numeric inputs follow the plugin page's existing behaviour: an empty string
// becomes `undefined`, everything else is parsed with Number.parseInt /
// Number.parseFloat and a NaN parse is ignored (leaving the last good value).

interface CooldownNumberFieldProps {
	label: string;
	testId: string;
	value: number | undefined;
	onChange: (value: number | undefined) => void;
	description?: string;
	placeholder?: string;
	min?: number;
	max?: number;
	step?: number;
	integer?: boolean;
	disabled?: boolean;
}

export function CooldownNumberField({
	label,
	testId,
	value,
	onChange,
	description,
	placeholder,
	min,
	max,
	step,
	integer = true,
	disabled,
}: CooldownNumberFieldProps) {
	return (
		<div className="space-y-1">
			<Label htmlFor={testId} className="text-xs">
				{label}
			</Label>
			{description && <p className="text-muted-foreground text-xs">{description}</p>}
			<Input
				id={testId}
				type="number"
				inputMode={integer ? "numeric" : "decimal"}
				min={min}
				max={max}
				step={step}
				data-testid={testId}
				placeholder={placeholder}
				value={value ?? ""}
				disabled={disabled}
				onChange={(event) => {
					const raw = event.target.value;
					if (raw === "") {
						onChange(undefined);
						return;
					}
					const parsed = integer ? Number.parseInt(raw, 10) : Number.parseFloat(raw);
					if (!Number.isNaN(parsed)) onChange(parsed);
				}}
			/>
		</div>
	);
}

interface CooldownSelectFieldProps<T extends string> {
	label: string;
	testId: string;
	value: T;
	onChange: (value: T) => void;
	options: ReadonlyArray<{ value: T; label: string }>;
	description?: string;
	placeholder?: string;
	disabled?: boolean;
}

export function CooldownSelectField<T extends string>({
	label,
	testId,
	value,
	onChange,
	options,
	description,
	placeholder,
	disabled,
}: CooldownSelectFieldProps<T>) {
	return (
		<div className="space-y-1">
			<Label className="text-xs">{label}</Label>
			{description && <p className="text-muted-foreground text-xs">{description}</p>}
			<Select value={value} onValueChange={(next) => onChange(next as T)} disabled={disabled}>
				<SelectTrigger className="w-full" data-testid={testId}>
					<SelectValue placeholder={placeholder} />
				</SelectTrigger>
				<SelectContent>
					{options.map((option) => (
						<SelectItem key={option.value} value={option.value}>
							{option.label}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
	);
}

interface CooldownSwitchRowProps {
	label: string;
	testId: string;
	checked: boolean;
	onCheckedChange: (checked: boolean) => void;
	description?: ReactNode;
	disabled?: boolean;
}

export function CooldownSwitchRow({ label, testId, checked, onCheckedChange, description, disabled }: CooldownSwitchRowProps) {
	return (
		<div className="flex flex-row items-start justify-between gap-3">
			<div className="space-y-0.5">
				<Label className="text-xs">{label}</Label>
				{description && <p className="text-muted-foreground text-xs">{description}</p>}
			</div>
			<Switch data-testid={testId} checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} />
		</div>
	);
}
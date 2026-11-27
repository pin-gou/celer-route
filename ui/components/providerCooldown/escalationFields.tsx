import type { CooldownEscalationConfig } from "@/lib/types/config";
import { useTranslation } from "react-i18next";
import { CooldownNumberField, CooldownSwitchRow } from "./fieldPrimitives";

interface EscalationFieldsProps {
	value: CooldownEscalationConfig;
	onChange: (value: CooldownEscalationConfig) => void;
	disabled?: boolean;
}

/**
 * Adaptive TTL escalation — the provider-level override of the plugin's
 * escalation section. Controlled: the owner owns the object, this component
 * only emits a new one.
 */
export function EscalationFields({ value, onChange, disabled }: EscalationFieldsProps) {
	const { t } = useTranslation("common");
	const enabled = value.enabled ?? false;

	return (
		<div className="space-y-3">
			<CooldownSwitchRow
				label={t("cooldownCapability.escalation.title")}
				description={t("cooldownCapability.escalation.description")}
				testId="providercooldown-escalation-enabled"
				checked={enabled}
				onCheckedChange={(checked) => onChange({ ...value, enabled: checked })}
				disabled={disabled}
			/>
			<CooldownNumberField
				label={t("cooldownCapability.escalation.maxMultiplierLabel")}
				description={t("cooldownCapability.escalation.maxMultiplierDescription")}
				placeholder={t("cooldownCapability.escalation.maxMultiplierPlaceholder")}
				testId="providercooldown-escalation-max-multiplier"
				value={value.max_multiplier}
				onChange={(maxMultiplier) => onChange({ ...value, max_multiplier: maxMultiplier })}
				min={1}
				max={16}
				step={1}
				disabled={disabled || !enabled}
			/>
		</div>
	);
}
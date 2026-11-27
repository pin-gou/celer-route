import type { CooldownCapacityConfig } from "@/lib/types/config";
import { useTranslation } from "react-i18next";
import { DEFAULT_CAPACITY_ACTION, DEFAULT_CAPACITY_SCOPE } from "./defaults";
import { CooldownNumberField, CooldownSelectField, CooldownSwitchRow } from "./fieldPrimitives";

type CapacityScope = NonNullable<CooldownCapacityConfig["scope"]>;
type CapacityAction = NonNullable<CooldownCapacityConfig["action"]>;

interface CapacityFieldsProps {
	value: CooldownCapacityConfig;
	onChange: (value: CooldownCapacityConfig) => void;
	disabled?: boolean;
}

/**
 * Retrospective capacity gate — the provider-level override of the plugin's
 * capacity section. Controlled: the owner owns the object.
 */
export function CapacityFields({ value, onChange, disabled }: CapacityFieldsProps) {
	const { t } = useTranslation("common");
	const enabled = value.enabled ?? false;

	const scopeOptions: ReadonlyArray<{ value: CapacityScope; label: string }> = [
		{ value: "provider", label: t("cooldownCapability.capacity.scopeProvider") },
		{ value: "provider_model", label: t("cooldownCapability.capacity.scopeProviderModel") },
		{ value: "key", label: t("cooldownCapability.capacity.scopeKey") },
	];
	const actionOptions: ReadonlyArray<{ value: CapacityAction; label: string }> = [
		{ value: "fallback", label: t("cooldownCapability.capacity.actionFallback") },
		{ value: "reject", label: t("cooldownCapability.capacity.actionReject") },
	];

	return (
		<div className="space-y-3">
			<CooldownSwitchRow
				label={t("cooldownCapability.capacity.title")}
				description={t("cooldownCapability.capacity.description")}
				testId="providercooldown-capacity-enabled"
				checked={enabled}
				onCheckedChange={(checked) => onChange({ ...value, enabled: checked })}
				disabled={disabled}
			/>

			<div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<CooldownNumberField
					label={t("cooldownCapability.capacity.tokenLimitLabel")}
					description={t("cooldownCapability.capacity.tokenLimitDescription")}
					placeholder={t("cooldownCapability.capacity.tokenLimitPlaceholder")}
					testId="providercooldown-capacity-token-limit"
					value={value.token_limit}
					onChange={(tokenLimit) => onChange({ ...value, token_limit: tokenLimit })}
					min={0}
					step={1}
					disabled={disabled || !enabled}
				/>
				<CooldownNumberField
					label={t("cooldownCapability.capacity.requestLimitLabel")}
					description={t("cooldownCapability.capacity.requestLimitDescription")}
					placeholder={t("cooldownCapability.capacity.requestLimitPlaceholder")}
					testId="providercooldown-capacity-request-limit"
					value={value.request_limit}
					onChange={(requestLimit) => onChange({ ...value, request_limit: requestLimit })}
					min={0}
					step={1}
					disabled={disabled || !enabled}
				/>
				<CooldownNumberField
					label={t("cooldownCapability.capacity.windowSecondsLabel")}
					description={t("cooldownCapability.capacity.windowSecondsDescription")}
					placeholder={t("cooldownCapability.capacity.windowSecondsPlaceholder")}
					testId="providercooldown-capacity-window-seconds"
					value={value.window_seconds}
					onChange={(windowSeconds) => onChange({ ...value, window_seconds: windowSeconds })}
					min={10}
					max={3600}
					step={1}
					disabled={disabled || !enabled}
				/>
				<CooldownNumberField
					label={t("cooldownCapability.capacity.headroomLabel")}
					description={t("cooldownCapability.capacity.headroomDescription")}
					placeholder={t("cooldownCapability.capacity.headroomPlaceholder")}
					testId="providercooldown-capacity-headroom"
					value={value.headroom}
					onChange={(headroom) => onChange({ ...value, headroom })}
					min={0.1}
					max={1}
					step={0.1}
					integer={false}
					disabled={disabled || !enabled}
				/>
				<CooldownSelectField
					label={t("cooldownCapability.capacity.scopeLabel")}
					description={t("cooldownCapability.capacity.scopeDescription")}
					placeholder={t("cooldownCapability.capacity.scopePlaceholder")}
					testId="providercooldown-capacity-scope-trigger"
					value={value.scope ?? DEFAULT_CAPACITY_SCOPE}
					onChange={(scope) => onChange({ ...value, scope })}
					options={scopeOptions}
					disabled={disabled || !enabled}
				/>
				<CooldownSelectField
					label={t("cooldownCapability.capacity.actionLabel")}
					description={t("cooldownCapability.capacity.actionDescription")}
					placeholder={t("cooldownCapability.capacity.actionPlaceholder")}
					testId="providercooldown-capacity-action-trigger"
					value={value.action ?? DEFAULT_CAPACITY_ACTION}
					onChange={(action) => onChange({ ...value, action })}
					options={actionOptions}
					disabled={disabled || !enabled}
				/>
			</div>
		</div>
	);
}
import type { CooldownCapacityConfig, CooldownCapacityKeyLimit } from "@/lib/types/config";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { useTranslation } from "react-i18next";
import { DEFAULT_CAPACITY_ACTION, DEFAULT_CAPACITY_SCOPE } from "./defaults";
import { CooldownNumberField, CooldownSelectField, CooldownSwitchRow } from "./fieldPrimitives";

type CapacityScope = NonNullable<CooldownCapacityConfig["scope"]>;
type CapacityAction = NonNullable<CooldownCapacityConfig["action"]>;

/** The minimum a key needs to be listed in the per-key override editor. */
export interface CapacityKeyOption {
	id: string;
	name: string;
}

interface CapacityFieldsProps {
	value: CooldownCapacityConfig;
	onChange: (value: CooldownCapacityConfig) => void;
	disabled?: boolean;
	/**
	 * The provider's configured keys. Used by the per-key override editor, which
	 * is only offered under `scope: "key"`.
	 */
	keys?: ReadonlyArray<CapacityKeyOption>;
}

/**
 * Retrospective capacity gate — the provider-level override of the plugin's
 * capacity section. Controlled: the owner owns the object.
 */
export function CapacityFields({ value, onChange, disabled, keys = [] }: CapacityFieldsProps) {
	const { t } = useTranslation("common");
	const enabled = value.enabled ?? false;
	const scope = value.scope ?? DEFAULT_CAPACITY_SCOPE;
	const keyLimits = value.key_limits ?? {};

	const setKeyLimit = (name: string, patch: Partial<CooldownCapacityKeyLimit>) => {
		const current = keyLimits[name] ?? {};
		const next: CooldownCapacityKeyLimit = { ...current, ...patch };
		// An undefined field means "inherit"; drop it instead of serializing it.
		if (next.token_limit === undefined) delete next.token_limit;
		if (next.request_limit === undefined) delete next.request_limit;
		const all: Record<string, CooldownCapacityKeyLimit> = { ...keyLimits };
		if (next.token_limit === undefined && next.request_limit === undefined) {
			// An entry with no limit would be rejected by the backend (and would
			// silently do nothing), so removing both fields removes the entry.
			delete all[name];
		} else {
			all[name] = next;
		}
		onChange({ ...value, key_limits: Object.keys(all).length > 0 ? all : undefined });
	};

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

			{scope === "key" && (
				<div className="space-y-2" data-testid="providercooldown-capacity-key-limits">
					<div className="space-y-1">
						<Label className="text-xs">{t("cooldownCapability.capacity.keyLimitsLabel")}</Label>
						<p className="text-muted-foreground text-xs">{t("cooldownCapability.capacity.keyLimitsDescription")}</p>
					</div>
					{keys.length === 0 ? (
						<p
							data-testid="providercooldown-capacity-key-limits-empty"
							className="text-muted-foreground rounded-md border border-dashed p-2.5 text-xs"
						>
							{t("cooldownCapability.capacity.keyLimitsEmpty")}
						</p>
					) : (
						<div className="space-y-2">
							{keys.map((key) => {
								const override = keyLimits[key.name];
								return (
									<div
										key={key.id || key.name}
										data-testid={`providercooldown-capacity-key-limit-${key.name}`}
										className="rounded-md border p-2.5"
									>
										<div className="mb-2 flex items-center justify-between gap-2">
											<span className="font-mono text-xs break-all">{key.name}</span>
											{override ? (
												<Button
													type="button"
													variant="ghost"
													size="sm"
													className="h-6 px-2 text-xs"
													data-testid={`providercooldown-capacity-key-limit-reset-${key.name}`}
													onClick={() => setKeyLimit(key.name, { token_limit: undefined, request_limit: undefined })}
													disabled={disabled}
												>
													{t("cooldownCapability.capacity.keyLimitReset")}
												</Button>
											) : (
												<span className="text-muted-foreground text-xs">{t("cooldownCapability.capacity.keyLimitInherited")}</span>
											)}
										</div>
										<div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
											<CooldownNumberField
												label={t("cooldownCapability.capacity.keyLimitTokenLabel")}
												placeholder={t("cooldownCapability.capacity.keyLimitPlaceholder")}
												testId={`providercooldown-capacity-key-limit-token-${key.name}`}
												value={override?.token_limit}
												onChange={(token_limit) => setKeyLimit(key.name, { token_limit })}
												min={0}
												step={1}
												disabled={disabled || !enabled}
											/>
											<CooldownNumberField
												label={t("cooldownCapability.capacity.keyLimitRequestLabel")}
												placeholder={t("cooldownCapability.capacity.keyLimitPlaceholder")}
												testId={`providercooldown-capacity-key-limit-request-${key.name}`}
												value={override?.request_limit}
												onChange={(request_limit) => setKeyLimit(key.name, { request_limit })}
												min={0}
												step={1}
												disabled={disabled || !enabled}
											/>
										</div>
									</div>
								);
							})}
						</div>
					)}
				</div>
			)}
		</div>
	);
}
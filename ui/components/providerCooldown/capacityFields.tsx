import type { CooldownCapacityConfig, CooldownCapacityKeyLimit, CooldownCapacityModelLimit } from "@/lib/types/config";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
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
	/**
	 * Every model the provider serves, in display order. Used to spell out the
	 * budget count for the selected unit and to populate the per-model override
	 * picker. An empty list simply hides the count/aggregate preview and
	 * disables adding model overrides.
	 */
	modelOptions?: ReadonlyArray<string>;
}

/**
 * Retrospective capacity gate — the provider-level override of the plugin's
 * capacity section. Controlled: the owner owns the object.
 *
 * The field order follows the decision an operator actually makes:
 *   1. what one budget covers (budget unit),
 *   2. how much each budget gets,
 *   3. how the window/headroom turn that into a block point,
 *   4. what happens when it blocks,
 *   5. optional per-key / per-model exceptions.
 *
 * Step 1 multiplies step 2 — a limit is per budget unit, not a total — so the
 * consequence is spelled out in between instead of being left to arithmetic.
 */
export function CapacityFields({ value, onChange, disabled, keys = [], modelOptions = [] }: CapacityFieldsProps) {
	const { t } = useTranslation("common");
	const enabled = value.enabled ?? false;
	const scope = value.scope ?? DEFAULT_CAPACITY_SCOPE;
	const keyLimits = value.key_limits ?? {};
	const fieldsDisabled = disabled || !enabled;

	const setKeyLimit = (name: string, patch: Partial<CooldownCapacityKeyLimit>) => {
		const current = keyLimits[name] ?? {};
		const next: CooldownCapacityKeyLimit = { ...current, ...patch };
		// An undefined field means "inherit"; drop it instead of serializing it.
		if (next.token_limit === undefined) delete next.token_limit;
		if (next.request_limit === undefined) delete next.request_limit;
		if (next.models === undefined) delete next.models;
		const hasModelOverrides = next.models !== undefined && Object.keys(next.models).length > 0;
		const all: Record<string, CooldownCapacityKeyLimit> = { ...keyLimits };
		if (next.token_limit === undefined && next.request_limit === undefined && !hasModelOverrides) {
			// An entry with no limit would be rejected by the backend (and would
			// silently do nothing), so removing both fields removes the entry.
			delete all[name];
		} else {
			all[name] = next;
		}
		onChange({ ...value, key_limits: Object.keys(all).length > 0 ? all : undefined });
	};

	const setModelLimit = (name: string, model: string, patch: Partial<CooldownCapacityModelLimit>) => {
		const keyEntry = keyLimits[name] ?? {};
		const models = { ...(keyEntry.models ?? {}) };
		const currentModel = models[model] ?? {};
		const nextModel: CooldownCapacityModelLimit = { ...currentModel, ...patch };
		if (nextModel.token_limit === undefined) delete nextModel.token_limit;
		if (nextModel.request_limit === undefined) delete nextModel.request_limit;
		// A model override with nothing set is a no-op the backend rejects;
		// removing both fields removes the row.
		if (nextModel.token_limit === undefined && nextModel.request_limit === undefined) {
			delete models[model];
		} else {
			models[model] = nextModel;
		}
		setKeyLimit(name, { models: Object.keys(models).length > 0 ? models : undefined });
	};

	/**
	 * A new per-model row starts from the credential's effective value, so the
	 * operator edits a meaningful baseline instead of an empty, invalid entry.
	 * Falls back to the provider-level value and then to an explicit 0.
	 */
	const addModelLimit = (name: string, available: ReadonlyArray<string>) => {
		const first = available[0];
		if (!first) return;
		const keyEntry = keyLimits[name] ?? {};
		const token = keyEntry.token_limit ?? value.token_limit;
		const request = keyEntry.request_limit ?? value.request_limit;
		const seeded: CooldownCapacityModelLimit = {};
		if (token !== undefined) seeded.token_limit = token;
		if (request !== undefined) seeded.request_limit = request;
		// Nothing to inherit (the gate has no limit yet) → start from an explicit
		// 0 so the row is valid and the operator edits a visible value.
		if (token === undefined && request === undefined) seeded.token_limit = 0;
		setKeyLimit(name, { models: { ...(keyEntry.models ?? {}), [first]: seeded } });
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

	// How many independent budgets the current unit produces. `null` means we
	// cannot tell yet (model list not loaded / no keys configured), in which
	// case the preview explains the formula instead of asserting a number.
	const modelCount = modelOptions.length;
	const unitCount = (() => {
		if (scope === "provider") return 1;
		if (modelCount === 0) return null;
		if (scope === "provider_model") return modelCount;
		if (keys.length === 0) return null;
		return keys.length * modelCount;
	})();

	const unitLabel =
		scope === "provider"
			? t("cooldownCapability.capacity.scopeUnitProvider")
			: scope === "provider_model"
				? modelCount > 0
					? t("cooldownCapability.capacity.scopeUnitProviderModel", { models: modelCount })
					: t("cooldownCapability.capacity.scopeUnitUnknown")
				: unitCount !== null
					? t("cooldownCapability.capacity.scopeUnitKey", { keys: keys.length, models: modelCount })
					: t("cooldownCapability.capacity.scopeUnitUnknown");

	const totalTokens = unitCount !== null && (value.token_limit ?? 0) > 0 ? (value.token_limit ?? 0) * unitCount : 0;
	const totalRequests = unitCount !== null && (value.request_limit ?? 0) > 0 ? (value.request_limit ?? 0) * unitCount : 0;
	const hasAnyLimit = (value.token_limit ?? 0) > 0 || (value.request_limit ?? 0) > 0 || Object.keys(keyLimits).length > 0;
	const showTotal = unitCount !== null && hasAnyLimit;
	const showExplosionWarning = unitCount !== null && unitCount >= 16;

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

			{/* 1. What one budget covers. Full width and first: every number
			    below is per budget, so the unit has to be read first. */}
			<CooldownSelectField
				label={t("cooldownCapability.capacity.scopeLabel")}
				description={t("cooldownCapability.capacity.scopeDescription")}
				placeholder={t("cooldownCapability.capacity.scopePlaceholder")}
				testId="providercooldown-capacity-scope-trigger"
				value={scope}
				onChange={(nextScope) => onChange({ ...value, scope: nextScope })}
				options={scopeOptions}
				disabled={fieldsDisabled}
			/>
			<div className="bg-muted/40 space-y-1 rounded-md border p-2.5" data-testid="providercooldown-capacity-scope-preview">
				<p className="text-xs">
					<span className="font-medium">
						{unitCount !== null ? t("cooldownCapability.capacity.scopePreview", { count: unitCount, unit: unitLabel }) : unitLabel}
					</span>
					<span className="text-muted-foreground ml-1.5">
						{t(
							`cooldownCapability.capacity.scopeHint${scope === "provider" ? "Provider" : scope === "provider_model" ? "ProviderModel" : "Key"}`,
						)}
					</span>
				</p>
				{showTotal && (
					<p className="text-muted-foreground text-xs" data-testid="providercooldown-capacity-scope-total">
						{totalTokens > 0 || totalRequests > 0
							? t("cooldownCapability.capacity.scopeTotal", {
									tokens: totalTokens > 0 ? totalTokens.toLocaleString("en-US") : t("cooldownCapability.capacity.noLimit"),
									requests: totalRequests > 0 ? totalRequests.toLocaleString("en-US") : t("cooldownCapability.capacity.noLimit"),
									window: value.window_seconds && value.window_seconds > 0 ? value.window_seconds : 60,
								})
							: t("cooldownCapability.capacity.scopeTotalNoLimit")}
					</p>
				)}
				{showExplosionWarning && (
					<p className="text-xs text-amber-600 dark:text-amber-500" data-testid="providercooldown-capacity-scope-warning">
						{t("cooldownCapability.capacity.scopeWarnMany", { count: unitCount })}
					</p>
				)}
			</div>

			{/* 2. The amount each budget gets. */}
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
					disabled={fieldsDisabled}
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
					disabled={fieldsDisabled}
				/>
			</div>

			{/* 3. When it blocks. */}
			<div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
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
					disabled={fieldsDisabled}
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
					disabled={fieldsDisabled}
				/>
			</div>

			{/* 4. What happens once it blocks. */}
			<CooldownSelectField
				label={t("cooldownCapability.capacity.actionLabel")}
				description={t("cooldownCapability.capacity.actionDescription")}
				placeholder={t("cooldownCapability.capacity.actionPlaceholder")}
				testId="providercooldown-capacity-action-trigger"
				value={value.action ?? DEFAULT_CAPACITY_ACTION}
				onChange={(action) => onChange({ ...value, action })}
				options={actionOptions}
				disabled={fieldsDisabled}
			/>

			{/* 5. Exceptions. Only the per-key unit carries a credential, so only
			    that unit offers this editor. */}
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
								const modelOverrides = override?.models ?? {};
								const usedModels = Object.keys(modelOverrides);
								const availableModels = modelOptions.filter((m) => !usedModels.includes(m));
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
													onClick={() => setKeyLimit(key.name, { token_limit: undefined, request_limit: undefined, models: undefined })}
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
												disabled={fieldsDisabled}
											/>
											<CooldownNumberField
												label={t("cooldownCapability.capacity.keyLimitRequestLabel")}
												placeholder={t("cooldownCapability.capacity.keyLimitPlaceholder")}
												testId={`providercooldown-capacity-key-limit-request-${key.name}`}
												value={override?.request_limit}
												onChange={(request_limit) => setKeyLimit(key.name, { request_limit })}
												min={0}
												step={1}
												disabled={fieldsDisabled}
											/>
										</div>

										{/* Most specific layer: one credential's quota may differ
										    per model (e.g. a different free-tier bucket per model). */}
										<div className="mt-2 border-t pt-2">
											<div className="flex items-center justify-between gap-2">
												<span className="text-muted-foreground text-xs">
													{t("cooldownCapability.capacity.keyModelOverrideLabel")}
													{usedModels.length > 0 ? ` · ${usedModels.length}` : ""}
												</span>
												<Button
													type="button"
													variant="outline"
													size="sm"
													className="h-6 px-2 text-xs"
													data-testid={`providercooldown-capacity-key-model-add-${key.name}`}
													onClick={() => addModelLimit(key.name, availableModels)}
													disabled={disabled || !enabled || availableModels.length === 0}
												>
													{t("cooldownCapability.capacity.keyModelAdd")}
												</Button>
											</div>
											{modelOptions.length === 0 && (
												<p className="text-muted-foreground mt-1 text-xs">{t("cooldownCapability.capacity.keyModelNoOptions")}</p>
											)}
											{usedModels.length > 0 && (
												<div className="mt-2 space-y-2">
													{usedModels.map((model) => {
														const modelEntry = modelOverrides[model] ?? {};
														const rowId = `${key.name}-${model}`;
														const rowModels = modelOptions.includes(model) ? modelOptions : [model, ...modelOptions];
														return (
															<div
																key={model}
																data-testid={`providercooldown-capacity-key-model-row-${rowId}`}
																className="bg-muted/30 space-y-2 rounded-md border border-dashed p-2"
															>
																<div className="flex items-center justify-between gap-2">
																	<Select
																		value={model}
																		onValueChange={(nextModel) => {
																			if (nextModel === model) return;
																			const rest = { ...modelOverrides };
																			delete rest[model];
																			rest[nextModel] = modelEntry;
																			setKeyLimit(key.name, { models: rest });
																		}}
																		disabled={disabled || !enabled}
																	>
																		<SelectTrigger
																			className="h-7 w-full max-w-[220px]"
																			data-testid={`providercooldown-capacity-key-model-select-${rowId}`}
																		>
																			<SelectValue />
																		</SelectTrigger>
																		<SelectContent>
																			{rowModels.map((m) => (
																				<SelectItem key={m} value={m} disabled={m !== model && usedModels.includes(m)}>
																					{m}
																				</SelectItem>
																			))}
																		</SelectContent>
																	</Select>
																	<Button
																		type="button"
																		variant="ghost"
																		size="sm"
																		className="h-6 px-2 text-xs"
																		data-testid={`providercooldown-capacity-key-model-remove-${rowId}`}
																		onClick={() => setModelLimit(key.name, model, { token_limit: undefined, request_limit: undefined })}
																		disabled={disabled}
																	>
																		{t("cooldownCapability.capacity.keyModelRemove")}
																	</Button>
																</div>
																<div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
																	<CooldownNumberField
																		label={t("cooldownCapability.capacity.keyLimitTokenLabel")}
																		placeholder={
																			override?.token_limit !== undefined
																				? String(override.token_limit)
																				: t("cooldownCapability.capacity.keyLimitPlaceholder")
																		}
																		testId={`providercooldown-capacity-key-model-token-${rowId}`}
																		value={modelEntry.token_limit}
																		onChange={(token_limit) => setModelLimit(key.name, model, { token_limit })}
																		min={0}
																		step={1}
																		disabled={fieldsDisabled}
																	/>
																	<CooldownNumberField
																		label={t("cooldownCapability.capacity.keyLimitRequestLabel")}
																		placeholder={
																			override?.request_limit !== undefined
																				? String(override.request_limit)
																				: t("cooldownCapability.capacity.keyLimitPlaceholder")
																		}
																		testId={`providercooldown-capacity-key-model-request-${rowId}`}
																		value={modelEntry.request_limit}
																		onChange={(request_limit) => setModelLimit(key.name, model, { request_limit })}
																		min={0}
																		step={1}
																		disabled={fieldsDisabled}
																	/>
																</div>
															</div>
														);
													})}
												</div>
											)}
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
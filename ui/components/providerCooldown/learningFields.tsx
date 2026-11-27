import type { CooldownLearningConfig } from "@/lib/types/config";
import { useTranslation } from "react-i18next";
import { CooldownNumberField, CooldownSwitchRow } from "./fieldPrimitives";

interface LearningFieldsProps {
	value: CooldownLearningConfig;
	onChange: (value: CooldownLearningConfig) => void;
	disabled?: boolean;
}

/**
 * Read-only limit learner — the provider-level override of the plugin's
 * learning section. Controlled: the owner owns the object.
 */
export function LearningFields({ value, onChange, disabled }: LearningFieldsProps) {
	const { t } = useTranslation("common");
	const enabled = value.enabled ?? false;

	return (
		<div className="space-y-3">
			<CooldownSwitchRow
				label={t("cooldownCapability.learning.title")}
				description={t("cooldownCapability.learning.description")}
				testId="providercooldown-learning-enabled"
				checked={enabled}
				onCheckedChange={(checked) => onChange({ ...value, enabled: checked })}
				disabled={disabled}
			/>

			<div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
				<CooldownNumberField
					label={t("cooldownCapability.learning.windowSecondsLabel")}
					description={t("cooldownCapability.learning.windowSecondsDescription")}
					placeholder={t("cooldownCapability.learning.windowSecondsPlaceholder")}
					testId="providercooldown-learning-window-seconds"
					value={value.window_seconds}
					onChange={(windowSeconds) => onChange({ ...value, window_seconds: windowSeconds })}
					min={10}
					max={3600}
					step={1}
					disabled={disabled || !enabled}
				/>
				<CooldownNumberField
					label={t("cooldownCapability.learning.maxSamplesLabel")}
					description={t("cooldownCapability.learning.maxSamplesDescription")}
					placeholder={t("cooldownCapability.learning.maxSamplesPlaceholder")}
					testId="providercooldown-learning-max-samples"
					value={value.max_samples}
					onChange={(maxSamples) => onChange({ ...value, max_samples: maxSamples })}
					min={5}
					max={500}
					step={1}
					disabled={disabled || !enabled}
				/>
				<CooldownNumberField
					label={t("cooldownCapability.learning.minSamplesLabel")}
					description={t("cooldownCapability.learning.minSamplesDescription")}
					placeholder={t("cooldownCapability.learning.minSamplesPlaceholder")}
					testId="providercooldown-learning-min-samples"
					value={value.min_samples}
					onChange={(minSamples) => onChange({ ...value, min_samples: minSamples })}
					min={1}
					max={100}
					step={1}
					disabled={disabled || !enabled}
				/>
			</div>
		</div>
	);
}
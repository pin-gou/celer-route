import { Button } from "@/components/ui/button";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { CooldownCapabilityMode } from "./defaults";

const MODES: readonly CooldownCapabilityMode[] = ["inherit", "custom", "disabled"];

interface CapabilityOverrideCardProps {
	/** Card container data-testid, e.g. `providercooldown-capacity-card`. */
	testId: string;
	title: string;
	description?: string;
	value: CooldownCapabilityMode;
	onChange: (mode: CooldownCapabilityMode) => void;
	/**
	 * Human-readable summary of the value the provider currently resolves to
	 * (provider override, else plugin default, else frontend constant). Always
	 * rendered in muted text so the operator can see what "inherit" means
	 * before switching.
	 */
	inheritedSummary: string;
	disabled?: boolean;
	/** The editable fields; rendered only in `custom` mode. */
	children?: ReactNode;
	/** Extra content rendered in every mode (e.g. learned-limit suggestions). */
	panel?: ReactNode;
}

/**
 * Tri-state shell for one per-provider cooldown capability:
 * inherit the plugin default / custom override / explicitly disabled.
 *
 * It is a pure presentational shell — the owner decides how each mode maps to
 * the emitted `cooldown_policy` section.
 */
export function CapabilityOverrideCard({
	testId,
	title,
	description,
	value: mode,
	onChange,
	inheritedSummary,
	disabled,
	children,
	panel,
}: CapabilityOverrideCardProps) {
	const { t } = useTranslation("common");

	return (
		<div data-testid={testId} className="space-y-3 rounded-md border p-3">
			<div className="flex flex-wrap items-start justify-between gap-2">
				<div className="min-w-0">
					<h5 className="text-sm font-medium">{title}</h5>
					{description && <p className="text-muted-foreground mt-1 text-xs">{description}</p>}
				</div>
				<div className="flex shrink-0 items-center gap-1" role="group" aria-label={title}>
					{MODES.map((option) => (
						<Button
							key={option}
							type="button"
							size="sm"
							variant={mode === option ? "default" : "outline"}
							className="h-7 px-2 text-xs"
							onClick={() => onChange(option)}
							disabled={disabled}
							data-testid={`${testId}-mode-${option}`}
						>
							{t(`cooldownCapability.mode.${option}`)}
						</Button>
					))}
				</div>
			</div>

			<p data-testid={`${testId}-effective-summary`} className="text-muted-foreground text-xs">
				{mode === "disabled"
					? t("cooldownCapability.summaryDisabled")
					: t(mode === "inherit" ? "cooldownCapability.summaryInheritedPrefix" : "cooldownCapability.summaryGlobalPrefix", {
							summary: inheritedSummary,
						})}
			</p>

			{mode === "custom" && children}

			{panel}
		</div>
	);
}
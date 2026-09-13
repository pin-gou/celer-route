"use client";

import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useTokenUnitPreference } from "@/lib/hooks/useTokenUnitPreference";
import { Repeat2 } from "lucide-react";
import { useTranslation } from "react-i18next";

/**
 * Button that toggles the token unit system between SI-style 千/兆/吉 and
 * Chinese 万/亿. The preference is persisted in localStorage and applies to
 * every token-count display across the dashboard, logs and timeline pages.
 */
export function TokenUnitToggle() {
	const { t } = useTranslation("dashboard");
	const [system, setSystem] = useTokenUnitPreference();
	const label = system === "si" ? t("tokenUnits.si") : t("tokenUnits.cn");
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					className="h-7 gap-1 px-1.5 text-xs"
					onClick={() => setSystem(system === "si" ? "cn" : "si")}
					aria-label={t("tokenUnits.tooltip")}
					data-testid="token-unit-toggle"
				>
					<Repeat2 className="h-3.5 w-3.5" />
					{label}
				</Button>
			</TooltipTrigger>
			<TooltipContent data-testid="token-unit-toggle-tooltip">{t("tokenUnits.tooltip")}</TooltipContent>
		</Tooltip>
	);
}
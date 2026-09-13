"use client";

import { useTokenUnitPreference } from "@/lib/hooks/useTokenUnitPreference";
import { getTokenUnitScale } from "@/lib/utils/tokenUnits";
import NumberFlow from "@number-flow/react";

interface TokenNumberProps {
	value: number;
	/** Render a space between the number and the unit suffix (e.g. "1.23 千"). */
	spaceBeforeUnit?: boolean;
	className?: string;
}

/**
 * Animated token-count display (wraps NumberFlow) that follows the user's
 * token unit preference. Subscribes to the preference so it re-renders when
 * the unit system is toggled.
 */
export function TokenNumber({ value, spaceBeforeUnit = true, className }: TokenNumberProps) {
	useTokenUnitPreference();
	const { divisor, suffix } = getTokenUnitScale(value);
	const scaled = divisor > 1;
	const prefix = suffix && spaceBeforeUnit ? " " : "";
	return (
		<NumberFlow
			className={className}
			value={scaled ? value / divisor : value}
			format={
				scaled
					? { minimumFractionDigits: 2, maximumFractionDigits: 2, useGrouping: true }
					: { minimumFractionDigits: 0, maximumFractionDigits: 0, useGrouping: true }
			}
			suffix={suffix ? prefix + suffix : ""}
		/>
	);
}
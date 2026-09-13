"use client";

import { useCallback, useSyncExternalStore } from "react";
import { getTokenUnitSystem, setTokenUnitSystem, subscribeTokenUnitSystem, type TokenUnitSystem } from "../utils/tokenUnits";

export { getTokenUnitSystem };

/**
 * Hook that reads the user's preferred token unit system (千兆吉 vs 万亿) and
 * persists it in localStorage. Components calling this hook re-render whenever
 * the preference changes, so the toggle button and every token display stay in
 * sync across dashboard, logs and timeline pages.
 *
 * Returns a `[system, setSystem]` tuple.
 */
export function useTokenUnitPreference(): [TokenUnitSystem, (system: TokenUnitSystem) => void] {
	const system = useSyncExternalStore(subscribeTokenUnitSystem, getTokenUnitSystem, getTokenUnitSystem);
	const setSystem = useCallback((next: TokenUnitSystem) => setTokenUnitSystem(next), []);
	return [system, setSystem];
}
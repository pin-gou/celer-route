export type TokenUnitSystem = "si" | "cn";

export const TOKEN_UNIT_STORAGE_KEY = "bifrost.tokenUnits";

/** Default unit system: SI-style 千/兆/吉 (10^3 / 10^6 / 10^9). */
export const DEFAULT_TOKEN_UNIT_SYSTEM: TokenUnitSystem = "si";

function readStoredSystem(): TokenUnitSystem {
	try {
		const stored = localStorage.getItem(TOKEN_UNIT_STORAGE_KEY);
		if (stored === "si" || stored === "cn") return stored;
	} catch {
		// localStorage unavailable (SSR, privacy mode, tests) — keep default.
	}
	return DEFAULT_TOKEN_UNIT_SYSTEM;
}

let currentSystem: TokenUnitSystem = readStoredSystem();

const listeners = new Set<() => void>();

function emitChange() {
	for (const listener of listeners) listener();
}

export function getTokenUnitSystem(): TokenUnitSystem {
	return currentSystem;
}

export function setTokenUnitSystem(system: TokenUnitSystem) {
	currentSystem = system;
	try {
		localStorage.setItem(TOKEN_UNIT_STORAGE_KEY, system);
	} catch {
		// localStorage unavailable — preference won't persist.
	}
	emitChange();
}

export function subscribeTokenUnitSystem(listener: () => void): () => void {
	listeners.add(listener);
	return () => {
		listeners.delete(listener);
	};
}

// Keep open tabs in sync when the preference changes elsewhere.
if (typeof window !== "undefined") {
	window.addEventListener("storage", (event) => {
		if (event.key !== TOKEN_UNIT_STORAGE_KEY) return;
		currentSystem = readStoredSystem();
		emitChange();
	});
}

export interface TokenUnitScale {
	divisor: number;
	suffix: string;
}

/** Resolve the scale (divisor + unit suffix) for a token count under the current unit system. */
export function getTokenUnitScale(value: number): TokenUnitScale {
	if (currentSystem === "cn") {
		if (value >= 1_0000_0000) return { divisor: 1_0000_0000, suffix: "亿" };
		if (value >= 1_0000) return { divisor: 1_0000, suffix: "万" };
		return { divisor: 1, suffix: "" };
	}
	if (value >= 1_000_000_000) return { divisor: 1_000_000_000, suffix: "吉" };
	if (value >= 1_000_000) return { divisor: 1_000_000, suffix: "兆" };
	if (value >= 1_000) return { divisor: 1_000, suffix: "千" };
	return { divisor: 1, suffix: "" };
}

/** Format a token count under the current unit system (e.g. "1.23 千", "1.50 万"). */
export function formatTokenCount(value: number, fractionDigits = 2): string {
	if (!Number.isFinite(value)) return "0";
	const { divisor, suffix } = getTokenUnitScale(value);
	if (divisor === 1) return value.toLocaleString("en-US");
	return `${(value / divisor).toLocaleString("en-US", {
		minimumFractionDigits: fractionDigits,
		maximumFractionDigits: fractionDigits,
	})} ${suffix}`;
}
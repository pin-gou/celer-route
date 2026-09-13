import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
	DEFAULT_TOKEN_UNIT_SYSTEM,
	getTokenUnitScale,
	getTokenUnitSystem,
	formatTokenCount,
	setTokenUnitSystem,
	TOKEN_UNIT_STORAGE_KEY,
} from "./tokenUnits";
import { formatTokensAdaptive } from "./numbers";

beforeEach(() => {
	localStorage.clear();
	setTokenUnitSystem(DEFAULT_TOKEN_UNIT_SYSTEM);
});

afterEach(() => {
	localStorage.clear();
});

describe("tokenUnits", () => {
	it("defaults to the SI-style 千/兆/吉 system", () => {
		expect(getTokenUnitSystem()).toBe("si");
	});

	it("persists the preference to localStorage", () => {
		setTokenUnitSystem("cn");
		expect(localStorage.getItem(TOKEN_UNIT_STORAGE_KEY)).toBe("cn");
		expect(getTokenUnitSystem()).toBe("cn");
	});

	it("ignores an invalid stored value", () => {
		localStorage.setItem(TOKEN_UNIT_STORAGE_KEY, "imperial");
		setTokenUnitSystem("si");
		expect(getTokenUnitSystem()).toBe("si");
	});

	describe("getTokenUnitScale", () => {
		it("maps si thresholds to 千/兆/吉", () => {
			expect(getTokenUnitScale(999)).toEqual({ divisor: 1, suffix: "" });
			expect(getTokenUnitScale(1_500)).toEqual({ divisor: 1_000, suffix: "千" });
			expect(getTokenUnitScale(2_000_000)).toEqual({ divisor: 1_000_000, suffix: "兆" });
			expect(getTokenUnitScale(3_000_000_000)).toEqual({ divisor: 1_000_000_000, suffix: "吉" });
		});

		it("maps cn thresholds to 万/亿", () => {
			setTokenUnitSystem("cn");
			expect(getTokenUnitScale(9_999)).toEqual({ divisor: 1, suffix: "" });
			expect(getTokenUnitScale(12_345)).toEqual({ divisor: 10_000, suffix: "万" });
			expect(getTokenUnitScale(123_456_789)).toEqual({ divisor: 100_000_000, suffix: "亿" });
		});
	});

	describe("formatTokenCount", () => {
		it("keeps raw counts grouped without a unit suffix", () => {
			expect(formatTokenCount(512)).toBe("512");
			expect(formatTokenCount(12_345)).toBe("12.35 千");
		});

		it("renders two decimal places by default", () => {
			expect(formatTokenCount(1_500)).toBe("1.50 千");
			expect(formatTokenCount(1_500_000)).toBe("1.50 兆");
		});

		it("renders 万/亿 under the cn system", () => {
			setTokenUnitSystem("cn");
			expect(formatTokenCount(45_678)).toBe("4.57 万");
			expect(formatTokenCount(123_456_789)).toBe("1.23 亿");
		});

		it("handles non-finite values", () => {
			expect(formatTokenCount(Number.NaN)).toBe("0");
			expect(formatTokenCount(Number.POSITIVE_INFINITY)).toBe("0");
		});
	});

	it("formatTokensAdaptive follows the current unit system", () => {
		expect(formatTokensAdaptive(2_000_000)).toBe("2.00 兆");
		setTokenUnitSystem("cn");
		expect(formatTokensAdaptive(2_000_000)).toBe("200.00 万");
	});
});
import { useTranslation } from "react-i18next";
import { useCallback, useEffect, useState } from "react";
import { SUPPORTED_LOCALES, type SupportedLocale } from "./config";

const STORAGE_KEY = "bifrost.locale";

function normalizeLocale(locale: string): SupportedLocale {
	// Normalize "en-US", "en-GB", "en" → "en"
	if (locale.startsWith("en")) return "en";
	// Normalize "zh-CN", "zh-Hans", "zh" → "zh-CN"
	// Use exact match + known valid variants to avoid matching corrupted
	// values like "zh-CN-invalid" that happen to start with "zh".
	if (locale === "zh-CN" || locale === "zh" || locale === "zh-Hans" || locale === "zh-Hans-CN") return "zh-CN";
	return "en";
}

export interface UseLocaleReturn {
	locale: SupportedLocale;
	setLocale: (locale: SupportedLocale) => void;
	availableLocales: readonly SupportedLocale[];
}

export function useLocale(): UseLocaleReturn {
	const { i18n } = useTranslation();

	const [locale, setLocaleState] = useState<SupportedLocale>(() => {
		try {
			const stored = localStorage.getItem(STORAGE_KEY);
			if (stored) {
				const normalized = normalizeLocale(stored);
				if (SUPPORTED_LOCALES.includes(normalized)) {
					return normalized;
				}
			}
		} catch {
			// localStorage corrupted or unavailable — fall through
		}
		// Fallback to navigator.language
		if (typeof navigator !== "undefined" && navigator.language) {
			return normalizeLocale(navigator.language);
		}
		return "en";
	});

	const setLocale = useCallback(
		(newLocale: SupportedLocale) => {
			try {
				localStorage.setItem(STORAGE_KEY, newLocale);
			} catch {
				// localStorage unavailable — silently ignore
			}
			setLocaleState(newLocale);
			i18n.changeLanguage(newLocale).catch(() => {
				// i18n changeLanguage failed — silently ignore
			});
		},
		[i18n],
	);

	// Listen for cross-tab storage events to sync locale
	useEffect(() => {
		const handleStorage = (e: StorageEvent) => {
			if (e.key === STORAGE_KEY && e.newValue) {
				const normalized = normalizeLocale(e.newValue);
				if (SUPPORTED_LOCALES.includes(normalized)) {
					setLocaleState(normalized);
					i18n.changeLanguage(normalized).catch(() => {});
				}
			}
		};
		if (typeof window !== "undefined") {
			window.addEventListener("storage", handleStorage);
			return () => window.removeEventListener("storage", handleStorage);
		}
	}, [i18n]);

	return {
		locale,
		setLocale,
		availableLocales: SUPPORTED_LOCALES,
	};
}
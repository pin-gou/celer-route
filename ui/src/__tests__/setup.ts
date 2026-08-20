// ResizeObserver polyfill for jsdom — Radix UI components (Form, Switch, etc.)
// use @radix-ui/react-use-size which internally calls ResizeObserver.
// jsdom does not provide it, so we define a minimal no-op stub.
if (typeof globalThis.ResizeObserver === "undefined") {
	globalThis.ResizeObserver = class ResizeObserver {
		observe() {}
		unobserve() {}
		disconnect() {}
	};
}

// Initialize i18n for the test environment so useTranslation() returns real locale
// values instead of raw keys. This matches the app's i18n setup without the
// LanguageDetector dependency (which fails in jsdom).
import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import plugins_en from "@/locales/en/plugins.json";

i18n.use(initReactI18next).init({
	resources: {
		en: { plugins: plugins_en },
	},
	fallbackLng: "en",
	ns: ["plugins"],
	defaultNS: "plugins",
	interpolation: {
		escapeValue: false,
	},
	react: {
		useSuspense: false,
	},
	returnObjects: false,
	lng: "en",
});
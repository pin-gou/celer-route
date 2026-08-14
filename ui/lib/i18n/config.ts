import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import LanguageDetector from "i18next-browser-languagedetector";

import common_en from "@/locales/en/common.json";
import logs_en from "@/locales/en/logs.json";
import config_en from "@/locales/en/config.json";
import governance_en from "@/locales/en/governance.json";
import providers_en from "@/locales/en/providers.json";
import dashboard_en from "@/locales/en/dashboard.json";
import governanceUi_en from "@/locales/en/governance-ui.json";

import common_zh from "@/locales/zh-CN/common.json";
import logs_zh from "@/locales/zh-CN/logs.json";
import config_zh from "@/locales/zh-CN/config.json";
import governance_zh from "@/locales/zh-CN/governance.json";
import providers_zh from "@/locales/zh-CN/providers.json";
import dashboard_zh from "@/locales/zh-CN/dashboard.json";
import governanceUi_zh from "@/locales/zh-CN/governance-ui.json";

const NS = ["common", "logs", "config", "governance", "providers", "dashboard", "governance-ui"] as const;

export const SUPPORTED_LOCALES = ["en", "zh-CN"] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

export const LOCALE_LABELS: Record<SupportedLocale, string> = {
  en: "English",
  "zh-CN": "简体中文",
};

export const DEFAULT_NS = "common";

export const resources = {
  en: {
    common: common_en,
    logs: logs_en,
    config: config_en,
    governance: governance_en,
    providers: providers_en,
    dashboard: dashboard_en,
    "governance-ui": governanceUi_en,
  },
  "zh-CN": {
    common: common_zh,
    logs: logs_zh,
    config: config_zh,
    governance: governance_zh,
    providers: providers_zh,
    dashboard: dashboard_zh,
    "governance-ui": governanceUi_zh,
  },
} as const;

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: "en",
    ns: NS,
    defaultNS: DEFAULT_NS,
    interpolation: {
      escapeValue: false, // React already escapes values
    },
    detection: {
      order: ["localStorage", "navigator", "htmlTag"],
      lookupLocalStorage: "bifrost.locale",
      caches: ["localStorage"],
    },
    react: {
      useSuspense: false,
    },
    returnObjects: false,
  });

export default i18n;
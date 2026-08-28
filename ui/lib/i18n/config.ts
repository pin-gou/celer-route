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
import mcp_en from "@/locales/en/mcp.json";
import routing_en from "@/locales/en/routing.json";
import skills_en from "@/locales/en/skills.json";
import plugins_en from "@/locales/en/plugins.json";
import observability_en from "@/locales/en/observability.json";
import webhooks_en from "@/locales/en/webhooks.json";
import oauthGrants_en from "@/locales/en/oauth-grants.json";
import modelCatalog_en from "@/locales/en/model-catalog.json";
import onboarding_en from "@/locales/en/onboarding.json";
import login_en from "@/locales/en/login.json";
import home_en from "@/locales/en/home.json";

import common_zh from "@/locales/zh-CN/common.json";
import logs_zh from "@/locales/zh-CN/logs.json";
import config_zh from "@/locales/zh-CN/config.json";
import governance_zh from "@/locales/zh-CN/governance.json";
import providers_zh from "@/locales/zh-CN/providers.json";
import dashboard_zh from "@/locales/zh-CN/dashboard.json";
import governanceUi_zh from "@/locales/zh-CN/governance-ui.json";
import mcp_zh from "@/locales/zh-CN/mcp.json";
import routing_zh from "@/locales/zh-CN/routing.json";
import skills_zh from "@/locales/zh-CN/skills.json";
import plugins_zh from "@/locales/zh-CN/plugins.json";
import observability_zh from "@/locales/zh-CN/observability.json";
import webhooks_zh from "@/locales/zh-CN/webhooks.json";
import oauthGrants_zh from "@/locales/zh-CN/oauth-grants.json";
import modelCatalog_zh from "@/locales/zh-CN/model-catalog.json";
import onboarding_zh from "@/locales/zh-CN/onboarding.json";
import login_zh from "@/locales/zh-CN/login.json";
import home_zh from "@/locales/zh-CN/home.json";

const NS = [
	"common",
	"logs",
	"config",
	"governance",
	"providers",
	"dashboard",
	"governance-ui",
	"mcp",
	"routing",
	"skills",
	"plugins",
	"observability",
	"webhooks",
	"oauth-grants",
	"model-catalog",
	"onboarding",
	"login",
	"home",
] as const;

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
		mcp: mcp_en,
		routing: routing_en,
		skills: skills_en,
		plugins: plugins_en,
		observability: observability_en,
		webhooks: webhooks_en,
		"oauth-grants": oauthGrants_en,
		"model-catalog": modelCatalog_en,
		onboarding: onboarding_en,
		login: login_en,
		home: home_en,
	},
	"zh-CN": {
		common: common_zh,
		logs: logs_zh,
		config: config_zh,
		governance: governance_zh,
		providers: providers_zh,
		dashboard: dashboard_zh,
		"governance-ui": governanceUi_zh,
		mcp: mcp_zh,
		routing: routing_zh,
		skills: skills_zh,
		plugins: plugins_zh,
		observability: observability_zh,
		webhooks: webhooks_zh,
		"oauth-grants": oauthGrants_zh,
		"model-catalog": modelCatalog_zh,
		onboarding: onboarding_zh,
		login: login_zh,
		home: home_zh,
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
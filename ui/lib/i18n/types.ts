// Auto-generated type definitions for i18n resources.
// This file provides compile-time type safety for t() calls by importing
// the JSON resource files directly. TypeScript resolves the JSON types at
// build time, so new keys added to the JSON files are automatically picked
// up. The Resources interface and KeysWithNamespace type are derived from
// the JSON imports, not manually maintained per-key.

import type common from "@/locales/en/common.json";
import type logs from "@/locales/en/logs.json";
import type config from "@/locales/en/config.json";
import type governance from "@/locales/en/governance.json";
import type providers from "@/locales/en/providers.json";
import type dashboard from "@/locales/en/dashboard.json";
import type governanceUi from "@/locales/en/governance-ui.json";
import type mcp from "@/locales/en/mcp.json";
import type routing from "@/locales/en/routing.json";
import type skills from "@/locales/en/skills.json";
import type plugins from "@/locales/en/plugins.json";
import type observability from "@/locales/en/observability.json";
import type webhooks from "@/locales/en/webhooks.json";
import type oauthGrants from "@/locales/en/oauth-grants.json";
import type modelCatalog from "@/locales/en/model-catalog.json";
import type onboarding from "@/locales/en/onboarding.json";

export interface Resources {
	common: typeof common;
	logs: typeof logs;
	config: typeof config;
	governance: typeof governance;
	providers: typeof providers;
	dashboard: typeof dashboard;
	"governance-ui": typeof governanceUi;
	mcp: typeof mcp;
	routing: typeof routing;
	skills: typeof skills;
	plugins: typeof plugins;
	observability: typeof observability;
	webhooks: typeof webhooks;
	"oauth-grants": typeof oauthGrants;
	"model-catalog": typeof modelCatalog;
	onboarding: typeof onboarding;
}

export type KeysWithNamespace = {
	[NS in keyof Resources]: `${NS}:${string}`;
}[keyof Resources];
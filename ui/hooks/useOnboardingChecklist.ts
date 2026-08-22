import { useGetCoreConfigQuery } from "@/lib/store";
import { useGetModelConfigsQuery, useGetVirtualKeysQuery } from "@/lib/store/apis/governanceApi";
import { useGetAllKeysQuery } from "@/lib/store/apis/providersApi";
import { useMemo } from "react";

export const METADATA_DISMISSED_KEY = "onboarding_dismissed";
export const METADATA_SKIPPED_KEY = "onboarding_skipped";
// "Remind me later" snoozes the widget by setting this cookie's expiry to the
// chosen date — once the browser drops the cookie, the widget is due again.
export const REMIND_LATER_COOKIE = "bifrost_onboarding_remind_at";
// Closing via X is a session-scoped hide, not a long-lived dismissal — no
// expiry, explicitly cleared on route change. Both cookies are shared
// constants (not just widget-local) so the sidebar's "resume setup" promo
// card can read the same hidden/snoozed state the floating widget uses.
export const HIDDEN_UNTIL_NAV_COOKIE = "bifrost_onboarding_hidden_until_nav";

export type OnboardingSection = "Security" | "Provider Setup" | "Everything Else";

export interface OnboardingStep {
	id: string;
	title: string;
	route: string;
	complete: boolean;
	section: OnboardingSection;
}

export const parseSkippedIds = (raw: unknown) => (Array.isArray(raw) ? raw.filter((id): id is string => typeof id === "string") : []);

const authValueSet = (secretVar: { value?: string; ref?: string; type?: string } | undefined) => {
	if (!secretVar) return false;
	return !!secretVar.value || !!secretVar.ref;
};

// Shared onboarding checklist state — both the floating OnboardingWidget and
// the sidebar's "resume setup" promo card read this, so they can't silently
// disagree on what counts as done. RTK Query dedupes the underlying
// core-config/keys/governance calls across both consumers automatically.
export function useOnboardingChecklist({ skip = false }: { skip?: boolean } = {}) {
	const { data: bifrostConfig } = useGetCoreConfigQuery({}, { skip });
	const isDismissedForAll = bifrostConfig?.metadata?.[METADATA_DISMISSED_KEY] === true;
	const shouldSkipChecklistQueries = skip || isDismissedForAll;

	const { data: allKeys } = useGetAllKeysQuery(undefined, { skip: shouldSkipChecklistQueries });
	const { data: modelConfigsResponse } = useGetModelConfigsQuery(undefined, {
		skip: shouldSkipChecklistQueries,
	});
	const { data: vksResponse } = useGetVirtualKeysQuery(undefined, {
		skip: shouldSkipChecklistQueries,
	});

	const checklistReady = bifrostConfig !== undefined && allKeys !== undefined;

	const skippedIds = useMemo<string[]>(() => {
		return parseSkippedIds(bifrostConfig?.metadata?.[METADATA_SKIPPED_KEY]);
	}, [bifrostConfig?.metadata]);

	const authConfig = bifrostConfig?.auth_config;
	const clientConfig = bifrostConfig?.client_config;

	// True when no admin account has ever been created on this instance. The very
	// first PUT /api/config that creates one must include the setup token the
	// operator configured via setup_token in config.json (or BIFROST_SETUP_TOKEN).
	// Drives the workspace/home → /workspace/onboarding redirect on first login.
	const isFirstTimeSetup = bifrostConfig !== undefined && !authConfig;

	const steps: OnboardingStep[] = useMemo(() => {
		// Order: 1) Security, 2) Provider Setup, 3) Everything Else.
		// Security comes first so admins lock down access before exposing keys.
		const allowedOrigins = clientConfig?.allowed_origins ?? [];
		const common: OnboardingStep[] = [
			{
				id: "cors",
				title: "Restrict CORS origins",
				route: "/workspace/config/security",
				section: "Security",
				complete: allowedOrigins.some((origin) => origin.trim().length > 0) && allowedOrigins.every((origin) => origin.trim() !== "*"),
			},
			{
				id: "dashboard-auth",
				title: "Set up dashboard auth",
				route: "/workspace/config/security",
				section: "Security",
				complete: !!authConfig?.is_enabled && authValueSet(authConfig?.admin_username) && authValueSet(authConfig?.admin_password),
			},
			{
				id: "enforce-inference-auth",
				title: "Enforce auth on inference",
				route: "/workspace/config/security",
				section: "Security",
				complete: !!clientConfig?.enforce_auth_on_inference,
			},
			{
				id: "provider-key",
				title: "Add a provider key",
				route: "/workspace/providers",
				section: "Provider Setup",
				complete: (allKeys?.length ?? 0) > 0,
			},
		];
		return common;
	}, [allKeys, clientConfig, authConfig]);

	return { bifrostConfig, steps, skippedIds, checklistReady, isDismissedForAll, isFirstTimeSetup };
}
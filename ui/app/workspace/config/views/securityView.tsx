import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { SecretVarInput } from "@/components/ui/secretVarInput";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage, useGetCoreConfigQuery, useUpdateCoreConfigMutation } from "@/lib/store";
import { AuthConfig, CoreConfig, DefaultCoreConfig } from "@/lib/types/config";
import { SecretVar } from "@/lib/types/schemas";
import { parseArrayFromText } from "@/lib/utils/array";
import { getPasswordPolicyFailures, validateOrigins } from "@/lib/utils/validation";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { AlertTriangle, ChevronDown, Globe, KeyRound, Network, ShieldCheck } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

export default function SecurityView() {
	const { t } = useTranslation("config");
	const hasSettingsUpdateAccess = useRbac(RbacResource.Settings, RbacOperation.Update);
	const { data: bifrostConfig } = useGetCoreConfigQuery({ fromDB: true });
	const config = bifrostConfig?.client_config;
	const [updateCoreConfig, { isLoading }] = useUpdateCoreConfigMutation();
	const [localConfig, setLocalConfig] = useState<CoreConfig>(DefaultCoreConfig);
	const showPasswordSection = true;
	const passwordInputRef = useRef<HTMLInputElement | HTMLTextAreaElement>(null);
	const passwordUnchangedRef = useRef(true);

	const [localValues, setLocalValues] = useState<{
		allowed_origins: string;
		allowed_headers: string;
		required_headers: string;
		whitelisted_routes: string;
	}>({
		allowed_origins: "",
		allowed_headers: "",
		required_headers: "",
		whitelisted_routes: "",
	});

	const [authConfig, setAuthConfig] = useState<AuthConfig>({
		admin_username: { value: "", ref: "" },
		admin_password: { value: "", ref: "" },
		is_enabled: false,
	});
	const [passwordError, setPasswordError] = useState("");
	// No admin account has ever been created on this instance yet. The HTTP API
	// cannot create the first admin without a setup_token, so we guide the
	// operator to the celer-route-admin CLI instead.
	const isFirstTimeSetup = !bifrostConfig?.auth_config;

	useEffect(() => {
		if (bifrostConfig && config) {
			setLocalConfig(config);
			setLocalValues({
				allowed_origins: config?.allowed_origins?.join(", ") || "",
				allowed_headers: config?.allowed_headers?.join(", ") || "",
				required_headers: config?.required_headers?.join(", ") || "",
				whitelisted_routes: config?.whitelisted_routes?.join(", ") || "",
			});
		}
		if (bifrostConfig?.auth_config) {
			passwordUnchangedRef.current = true;
			setAuthConfig(bifrostConfig.auth_config);
		}
	}, [config, bifrostConfig]);

	const hasChanges = useMemo(() => {
		if (!config) return false;
		const localOrigins = localConfig.allowed_origins?.slice().sort().join(",");
		const serverOrigins = config.allowed_origins?.slice().sort().join(",");
		const originsChanged = localOrigins !== serverOrigins;

		const localHeaders = localConfig.allowed_headers?.slice().sort().join(",");
		const serverHeaders = config.allowed_headers?.slice().sort().join(",");
		const headersChanged = localHeaders !== serverHeaders;

		const usernameChanged =
			authConfig.admin_username?.value !== bifrostConfig?.auth_config?.admin_username?.value ||
			authConfig.admin_username?.ref !== bifrostConfig?.auth_config?.admin_username?.ref ||
			authConfig.admin_username?.type !== bifrostConfig?.auth_config?.admin_username?.type;
		const passwordChanged =
			authConfig.admin_password?.value !== bifrostConfig?.auth_config?.admin_password?.value ||
			authConfig.admin_password?.ref !== bifrostConfig?.auth_config?.admin_password?.ref ||
			authConfig.admin_password?.type !== bifrostConfig?.auth_config?.admin_password?.type;
		// When no admin exists yet, auth changes cannot be persisted via HTTP
		// (the operator must use the CLI) so exclude them from the dirty check.
		const authChanged =
			!isFirstTimeSetup && showPasswordSection
				? authConfig.is_enabled !== bifrostConfig?.auth_config?.is_enabled || usernameChanged || passwordChanged
				: false;

		const localRequired = localConfig.required_headers?.slice().sort().join(",");
		const serverRequired = config.required_headers?.slice().sort().join(",");
		const requiredChanged = localRequired !== serverRequired;

		const localWhitelistedRoutes = localConfig.whitelisted_routes?.slice().sort().join(",");
		const serverWhitelistedRoutes = config.whitelisted_routes?.slice().sort().join(",");
		const whitelistedRoutesChanged = localWhitelistedRoutes !== serverWhitelistedRoutes;

		const enforceAuthOnInferenceChanged = localConfig.enforce_auth_on_inference !== config.enforce_auth_on_inference;
		const allowDirectKeysChanged = localConfig.allow_direct_keys !== config.allow_direct_keys;
		const dualCredentialConflictBehaviorChanged =
			(localConfig.dual_credential_conflict_behavior || "prefer_idp") !== (config.dual_credential_conflict_behavior || "prefer_idp");

		return (
			originsChanged ||
			headersChanged ||
			requiredChanged ||
			whitelistedRoutesChanged ||
			authChanged ||
			enforceAuthOnInferenceChanged ||
			allowDirectKeysChanged ||
			dualCredentialConflictBehaviorChanged
		);
	}, [config, localConfig, authConfig, bifrostConfig, showPasswordSection, isFirstTimeSetup]);

	const needsRestart = useMemo(() => {
		if (!config) return false;

		const localOrigins = localConfig.allowed_origins?.slice().sort().join(",");
		const serverOrigins = config.allowed_origins?.slice().sort().join(",");
		const originsChanged = localOrigins !== serverOrigins;

		const localHeaders = localConfig.allowed_headers?.slice().sort().join(",");
		const serverHeaders = config.allowed_headers?.slice().sort().join(",");
		const headersChanged = localHeaders !== serverHeaders;

		const enforceAuthOnInferenceChanged = localConfig.enforce_auth_on_inference !== config.enforce_auth_on_inference && false;

		return originsChanged || headersChanged || enforceAuthOnInferenceChanged;
	}, [config, localConfig]);

	const handleAllowedOriginsChange = useCallback((value: string) => {
		setLocalValues((prev) => ({ ...prev, allowed_origins: value }));
		setLocalConfig((prev) => ({ ...prev, allowed_origins: parseArrayFromText(value) }));
	}, []);

	const handleAllowedHeadersChange = useCallback((value: string) => {
		setLocalValues((prev) => ({ ...prev, allowed_headers: value }));
		setLocalConfig((prev) => ({ ...prev, allowed_headers: parseArrayFromText(value) }));
	}, []);

	const handleRequiredHeadersChange = useCallback((value: string) => {
		setLocalValues((prev) => ({ ...prev, required_headers: value }));
		setLocalConfig((prev) => ({ ...prev, required_headers: parseArrayFromText(value) }));
	}, []);

	const handleWhitelistedRoutesChange = useCallback((value: string) => {
		setLocalValues((prev) => ({ ...prev, whitelisted_routes: value }));
		setLocalConfig((prev) => ({ ...prev, whitelisted_routes: parseArrayFromText(value) }));
	}, []);

	const handleConfigChange = useCallback((field: keyof CoreConfig, value: boolean) => {
		setLocalConfig((prev) => ({ ...prev, [field]: value }));
	}, []);

	const handleAuthToggle = useCallback((checked: boolean) => {
		setAuthConfig((prev) => ({ ...prev, is_enabled: checked }));
	}, []);

	const handleAuthFieldChange = useCallback((field: "admin_username" | "admin_password", value: SecretVar) => {
		if (field === "admin_password") {
			passwordUnchangedRef.current = false;
			const passwordPolicyFailures = !value.ref && value.value ? getPasswordPolicyFailures(value.value, false) : [];
			setPasswordError(
				passwordPolicyFailures.length > 0 ? t("security.passwordPolicyError", { failures: passwordPolicyFailures.join(", ") }) : "",
			);
		}
		setAuthConfig((prev) => ({ ...prev, [field]: value }));
	}, []);

	const handleSave = useCallback(async () => {
		try {
			const validation = validateOrigins(localConfig.allowed_origins);

			if (!validation.isValid && localConfig.allowed_origins.length > 0) {
				toast.error(t("security.invalidOrigins", { origins: validation.invalidOrigins.join(", ") }));
				return;
			}
			const hasUsername = authConfig.admin_username?.value || authConfig.admin_username?.ref;
			const hasPassword = authConfig.admin_password?.value || authConfig.admin_password?.ref;
			const passwordPolicyFailures =
				showPasswordSection && authConfig.is_enabled && !authConfig.admin_password?.ref && authConfig.admin_password?.value
					? getPasswordPolicyFailures(authConfig.admin_password.value, passwordUnchangedRef.current)
					: [];

			if (passwordPolicyFailures.length > 0) {
				setPasswordError(t("security.passwordPolicyError", { failures: passwordPolicyFailures.join(", ") }));
				passwordInputRef.current?.scrollIntoView({ behavior: "smooth", block: "center" });
				passwordInputRef.current?.focus({ preventScroll: true });
				return;
			}
			setPasswordError("");

			// When no admin exists yet, the HTTP API cannot create one without a
			// setup_token, so skip auth_config entirely — the operator must use the
			// celer-route-admin CLI. Once the admin is created via CLI and the page
			// is refreshed, auth_config will be present and the normal management
			// path (including the save button) becomes available.
			await updateCoreConfig({
				...bifrostConfig!,
				client_config: localConfig,
				...(showPasswordSection && !isFirstTimeSetup
					? {
							auth_config: {
								...(authConfig.is_enabled && hasUsername && hasPassword ? authConfig : { ...authConfig, is_enabled: false }),
							},
						}
					: {}),
			}).unwrap();
			toast.success(t("security.toast.settingsUpdated"));
		} catch (error) {
			toast.error(getErrorMessage(error));
		}
	}, [bifrostConfig, localConfig, authConfig, showPasswordSection, updateCoreConfig, isFirstTimeSetup]);

	return (
		<div className="mx-auto w-full max-w-4xl space-y-6">
			<div>
				<h2 className="text-lg font-semibold tracking-tight">{t("security.pageTitle")}</h2>
				<p className="text-muted-foreground text-sm">{t("security.pageDescription")}</p>
			</div>

			<div className="space-y-6">
				{/* Password Protect the Dashboard */}
				<Card className="py-0">
					<CardHeader className="flex flex-row items-center gap-2 px-5 py-4">
						<ShieldCheck className="text-muted-foreground size-4" />
						<div>
							<h3 className="text-sm font-semibold tracking-tight">{t("security.groupDashboardAccess")}</h3>
							<p className="text-muted-foreground text-xs">{t("security.groupDashboardAccessDesc")}</p>
						</div>
					</CardHeader>
					{showPasswordSection && (
						<CardContent className="space-y-4 px-5 pb-5">
							<div className="flex items-center justify-between">
								<div className="space-y-0.5">
									<Label htmlFor="auth-enabled" className="text-sm font-medium">
										{t("security.passwordProtect")}
									</Label>
									<p className="text-muted-foreground text-sm">{t("security.passwordProtectDesc")}</p>
								</div>
								<Switch id="auth-enabled" checked={authConfig.is_enabled} onCheckedChange={handleAuthToggle} />
							</div>
							{!isFirstTimeSetup ? (
								<div className="space-y-4">
									<div className="space-y-2">
										<Label htmlFor="admin-username">{t("security.username")}</Label>
										<SecretVarInput
											id="admin-username"
											type="text"
											placeholder={t("security.usernamePlaceholder")}
											value={authConfig.admin_username}
											disabled={!authConfig.is_enabled}
											onChange={(value) => handleAuthFieldChange("admin_username", value)}
										/>
									</div>
									<div className="space-y-2">
										<Label htmlFor="admin-password">{t("security.password")}</Label>
										<SecretVarInput
											ref={passwordInputRef}
											id="admin-password"
											aria-invalid={!!passwordError}
											aria-describedby={passwordError ? "admin-password-error" : undefined}
											type="password"
											showPasswordToggle
											placeholder={t("security.passwordPlaceholder")}
											value={authConfig.admin_password}
											disabled={!authConfig.is_enabled}
											onChange={(value) => handleAuthFieldChange("admin_password", value)}
										/>
										<p className="text-muted-foreground text-xs">{t("security.passwordHint")}</p>
										{passwordError ? (
											<p id="admin-password-error" className="text-destructive text-xs" role="alert">
												{passwordError}
											</p>
										) : null}
									</div>
								</div>
							) : authConfig.is_enabled ? (
								<div className="rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-xs text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
									<p className="mb-1 font-semibold">{t("security.setupTokenCliTitle")}</p>
									<p className="mb-2">{t("security.setupTokenCliDesc")}</p>
									<code className="block rounded bg-amber-100 px-2 py-1 font-mono text-xs dark:bg-amber-900/40">
										{t("security.setupTokenCliCommand")}
									</code>
								</div>
							) : null}
						</CardContent>
					)}
				</Card>

				{/* Enable Auth on Inference / Allow Direct API Keys */}
				<Card className="py-0">
					<CardHeader className="flex flex-row items-center gap-2 px-5 py-4">
						<KeyRound className="text-muted-foreground size-4" />
						<div>
							<h3 className="text-sm font-semibold tracking-tight">{t("security.groupApiAccess")}</h3>
							<p className="text-muted-foreground text-xs">{t("security.groupApiAccessDesc")}</p>
						</div>
					</CardHeader>
					<CardContent className="space-y-4 px-5 pb-5">
						<div className="flex items-center justify-between space-x-2">
							<div className="space-y-0.5">
								<label htmlFor="enforce-auth-on-inference" className="text-sm font-medium">
									{t("security.enforceVirtualKeys")}
								</label>
								<p className="text-muted-foreground text-sm">{t("security.enforceVirtualKeysDesc")}</p>
							</div>
							<Switch
								id="enforce-auth-on-inference"
								data-testid="enforce-auth-on-inference-switch"
								checked={localConfig.enforce_auth_on_inference}
								onCheckedChange={(checked) => handleConfigChange("enforce_auth_on_inference", checked)}
							/>
						</div>
						<div className="flex items-center justify-between space-x-2">
							<div className="space-y-0.5">
								<label htmlFor="allow-direct-keys" className="text-sm font-medium">
									{t("security.allowDirectKeys")}
								</label>
								<p className="text-muted-foreground text-sm">{t("security.allowDirectKeysDesc")}</p>
							</div>
							<Switch
								id="allow-direct-keys"
								data-testid="security-allow-direct-keys-switch"
								checked={localConfig.allow_direct_keys}
								onCheckedChange={(checked) => handleConfigChange("allow_direct_keys", checked)}
							/>
						</div>
					</CardContent>
				</Card>

				{/* Allowed Origins / Headers / Required Headers */}
				<Card className="py-0">
					<Collapsible defaultOpen={true}>
						<CollapsibleTrigger asChild>
							<button
								type="button"
								className="hover:bg-muted/50 group flex w-full items-center gap-2 rounded-sm px-5 py-4 text-left"
								data-testid="security-cors-headers-collapsible-trigger"
							>
								<Globe className="text-muted-foreground size-4" />
								<div className="flex-1 text-left">
									<h3 className="text-sm font-semibold tracking-tight">{t("security.groupCorsHeaders")}</h3>
									<p className="text-muted-foreground text-xs">{t("security.groupCorsHeadersDesc")}</p>
								</div>
								<ChevronDown className="text-muted-foreground size-4 transition-transform group-data-[state=open]:rotate-180" />
							</button>
						</CollapsibleTrigger>
						<CollapsibleContent className="border-t px-5 py-4">
							<div className="space-y-3">
								{needsRestart && <RestartWarning t={t} />}
								<TextareaField
									id="allowed-origins"
									label={t("security.allowedOrigins")}
									description={t("security.allowedOriginsDesc")}
									placeholder={t("security.allowedOriginsPlaceholder")}
									value={localValues.allowed_origins}
									onChange={handleAllowedOriginsChange}
									example={t("security.allowedOriginsExample")}
								/>
								<TextareaField
									id="allowed-headers"
									label={t("security.allowedHeaders")}
									description={t("security.allowedHeadersDesc")}
									placeholder={t("security.allowedHeadersPlaceholder")}
									value={localValues.allowed_headers}
									onChange={handleAllowedHeadersChange}
									example={t("security.allowedHeadersExample")}
								/>
								<TextareaField
									id="required-headers"
									label={t("security.requiredHeaders")}
									description={t("security.requiredHeadersDesc")}
									placeholder={t("security.requiredHeadersPlaceholder")}
									value={localValues.required_headers}
									onChange={handleRequiredHeadersChange}
									testId="required-headers-textarea"
									example={t("security.requiredHeadersExample")}
								/>
							</div>
						</CollapsibleContent>
					</Collapsible>
				</Card>

				{/* Whitelisted Routes */}
				<Card className="py-0">
					<Collapsible defaultOpen={false}>
						<CollapsibleTrigger asChild>
							<button
								type="button"
								className="hover:bg-muted/50 group flex w-full items-center gap-2 rounded-sm px-5 py-4 text-left"
								data-testid="security-routes-collapsible-trigger"
							>
								<Network className="text-muted-foreground size-4" />
								<div className="flex-1 text-left">
									<h3 className="text-sm font-semibold tracking-tight">{t("security.groupRouteExceptions")}</h3>
									<p className="text-muted-foreground text-xs">{t("security.groupRouteExceptionsDesc")}</p>
								</div>
								<ChevronDown className="text-muted-foreground size-4 transition-transform group-data-[state=open]:rotate-180" />
							</button>
						</CollapsibleTrigger>
						<CollapsibleContent className="border-t px-5 py-4">
							<TextareaField
								id="whitelisted-routes"
								label={t("security.whitelistedRoutes")}
								description={t("security.whitelistedRoutesDesc")}
								placeholder={t("security.whitelistedRoutesPlaceholder")}
								value={localValues.whitelisted_routes}
								onChange={handleWhitelistedRoutesChange}
								testId="whitelisted-routes-textarea"
								example={t("security.whitelistedRoutesExample")}
							/>
						</CollapsibleContent>
					</Collapsible>
				</Card>
			</div>

			<div className="bg-card sticky bottom-0 flex justify-end py-2">
				<Button onClick={handleSave} disabled={!hasChanges || isLoading || !hasSettingsUpdateAccess}>
					{isLoading ? t("actions.saving") : t("actions.saveChanges")}
				</Button>
			</div>
		</div>
	);
}

const TextareaField = ({
	id,
	label,
	description,
	placeholder,
	value,
	onChange,
	example,
	testId,
}: {
	id: string;
	label: string;
	description: string;
	placeholder: string;
	value: string;
	onChange: (value: string) => void;
	example?: string;
	testId?: string;
}) => (
	<div className="space-y-2 rounded-sm border p-4">
		<div className="space-y-0.5">
			<label htmlFor={id} className="text-sm font-medium">
				{label}
			</label>
			<p className="text-muted-foreground text-sm">{description}</p>
		</div>
		<Textarea
			id={id}
			data-testid={testId}
			className="h-24"
			placeholder={placeholder}
			value={value}
			onChange={(e) => onChange(e.target.value)}
		/>
		{example ? <p className="text-muted-foreground text-xs">{example}</p> : null}
	</div>
);

const RestartWarning = ({ t }: { t: (key: string) => string }) => {
	return (
		<Alert variant="destructive" className="mt-2">
			<AlertTriangle className="h-4 w-4" />
			<AlertDescription>{t("security.restartWarning")}</AlertDescription>
		</Alert>
	);
};
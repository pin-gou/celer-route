import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SecretVarInput } from "@/components/ui/secretVarInput";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage, useGetCoreConfigQuery, useUpdateCoreConfigMutation } from "@/lib/store";
import { AuthConfig, CoreConfig, DefaultCoreConfig } from "@/lib/types/config";
import { SecretVar } from "@/lib/types/schemas";
import { parseArrayFromText } from "@/lib/utils/array";
import { getPasswordPolicyFailures, validateOrigins } from "@/lib/utils/validation";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { AlertTriangle, Loader2 } from "lucide-react";
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
	const [setupToken, setSetupToken] = useState("");
	const [setupTokenErrorMessage, setSetupTokenErrorMessage] = useState<string | null>(null);
	// No admin account has ever been created on this instance yet. The very first
	// PUT /api/config that creates one must include the setup token the operator
	// configured via setup_token in config.json (or BIFROST_SETUP_TOKEN), so this
	// field only needs to show up that once.
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
		const authChanged = showPasswordSection
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
	}, [config, localConfig, authConfig, bifrostConfig, showPasswordSection]);

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
			setPasswordError(passwordPolicyFailures.length > 0 ? t("security.passwordPolicyError", { failures: passwordPolicyFailures.join(", ") }) : "");
		}
		setAuthConfig((prev) => ({ ...prev, [field]: value }));
	}, []);

	const handleSave = useCallback(async () => {
		try {
			const validation = validateOrigins(localConfig.allowed_origins);

			if (!validation.isValid && localConfig.allowed_origins.length > 0) {
				toast.error(
					t("security.invalidOrigins", { origins: validation.invalidOrigins.join(", ") }),
				);
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
			if (isFirstTimeSetup && authConfig.is_enabled && !setupToken.trim()) {
				setSetupTokenErrorMessage(
					t("security.setupTokenError"),
				);
				return;
			}
			setPasswordError("");

			await updateCoreConfig({
				...bifrostConfig!,
				client_config: localConfig,
				...(showPasswordSection
					? {
							auth_config: {
								...(authConfig.is_enabled && hasUsername && hasPassword ? authConfig : { ...authConfig, is_enabled: false }),
								...(isFirstTimeSetup ? { setup_token: setupToken.trim() } : {}),
							},
						}
					: {}),
			}).unwrap();
			setSetupToken("");
			toast.success(t("security.toast.settingsUpdated"));
		} catch (error) {
			const message = getErrorMessage(error);
			if (isFirstTimeSetup && message.toLowerCase().includes("setup token")) {
				setSetupTokenErrorMessage(message);
			} else {
				toast.error(message);
			}
		}
	}, [bifrostConfig, localConfig, authConfig, showPasswordSection, updateCoreConfig, isFirstTimeSetup, setupToken]);

	return (
		<div className="mx-auto w-full max-w-4xl space-y-4">
			<div>
				<h2 className="text-lg font-semibold tracking-tight">{t("security.pageTitle")}</h2>
				<p className="text-muted-foreground text-sm">{t("security.pageDescription")}</p>
			</div>

			<div className="space-y-4">
				{/* Password Protect the Dashboard */}
				{showPasswordSection && (
					<div>
						<div className="space-y-4 rounded-sm border p-4">
							<div className="flex items-center justify-between">
								<div className="space-y-0.5">
									<Label htmlFor="auth-enabled" className="text-sm font-medium">
										{t("security.passwordProtect")} <Badge variant="secondary">{t("security.beta")}</Badge>
									</Label>
									<p className="text-muted-foreground text-sm">
										{t("security.passwordProtectDesc")}
									</p>
								</div>
								<Switch id="auth-enabled" checked={authConfig.is_enabled} onCheckedChange={handleAuthToggle} />
							</div>
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
										placeholder={t("security.passwordPlaceholder")}
										value={authConfig.admin_password}
										disabled={!authConfig.is_enabled}
										onChange={(value) => handleAuthFieldChange("admin_password", value)}
									/>
									<p className="text-muted-foreground text-xs">
										{t("security.passwordHint")}
									</p>
									{passwordError ? (
										<p id="admin-password-error" className="text-destructive text-xs" role="alert">
											{passwordError}
										</p>
									) : null}
								</div>
								{isFirstTimeSetup && authConfig.is_enabled ? (
									<div className="space-y-2">
										<Label htmlFor="setup-token">{t("security.setupToken")}</Label>
										<Input
											id="setup-token"
											data-testid="security-setup-token-input"
											type="password"
											autoComplete="off"
											placeholder={t("security.setupTokenPlaceholder")}
											value={setupToken}
											onChange={(e) => setSetupToken(e.target.value)}
										/>
										<p className="text-muted-foreground text-xs">
											{t("security.setupTokenDesc")}
										</p>
									</div>
								) : null}
							</div>
						</div>
					</div>
				)}
				{/* Enable Auth on Inference */}
				<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
					<div className="space-y-0.5">
						<label htmlFor="enforce-auth-on-inference" className="text-sm font-medium">
							{t("security.enforceVirtualKeys")}
						</label>
						<p className="text-muted-foreground text-sm">
							{t("security.enforceVirtualKeysDesc")}{" "}
							
						</p>
					</div>
					<Switch
						id="enforce-auth-on-inference"
						data-testid="enforce-auth-on-inference-switch"
						checked={localConfig.enforce_auth_on_inference}
						onCheckedChange={(checked) => handleConfigChange("enforce_auth_on_inference", checked)}
					/>
				</div>
				{/* Dual Credential Conflict Behavior */}
				{/* Allow Direct API Keys */}
				<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
					<div className="space-y-0.5">
						<label htmlFor="allow-direct-keys" className="text-sm font-medium">
							{t("security.allowDirectKeys")}
						</label>
						<p className="text-muted-foreground text-sm">
							{t("security.allowDirectKeysDesc")}
						</p>
					</div>
					<Switch
						id="allow-direct-keys"
						data-testid="security-allow-direct-keys-switch"
						checked={localConfig.allow_direct_keys}
						onCheckedChange={(checked) => handleConfigChange("allow_direct_keys", checked)}
					/>
				</div>
				{/* Allowed Origins */}
				{needsRestart && <RestartWarning t={t} />}
				<div>
					<div className="space-y-2 rounded-sm border p-4">
						<div className="space-y-0.5">
							<label htmlFor="allowed-origins" className="text-sm font-medium">
								{t("security.allowedOrigins")}
							</label>
							<p className="text-muted-foreground text-sm">
								{t("security.allowedOriginsDesc")}
							</p>
						</div>
						<Textarea
							id="allowed-origins"
							className="h-24"
							placeholder={t("security.allowedOriginsPlaceholder")}
							value={localValues.allowed_origins}
							onChange={(e) => handleAllowedOriginsChange(e.target.value)}
						/>
					</div>
				</div>
				{/* Allowed Headers */}
				<div>
					<div className="space-y-2 rounded-sm border p-4">
						<div className="space-y-0.5">
							<label htmlFor="allowed-headers" className="text-sm font-medium">
								{t("security.allowedHeaders")}
							</label>
							<p className="text-muted-foreground text-sm">{t("security.allowedHeadersDesc")}</p>
						</div>
						<Textarea
							id="allowed-headers"
							className="h-24"
							placeholder={t("security.allowedHeadersPlaceholder")}
							value={localValues.allowed_headers}
							onChange={(e) => handleAllowedHeadersChange(e.target.value)}
						/>
					</div>
				</div>
				{/* Required Headers */}
				<div>
					<div className="space-y-2 rounded-sm border p-4">
						<div className="space-y-0.5">
							<label htmlFor="required-headers" className="text-sm font-medium">
								{t("security.requiredHeaders")}
							</label>
							<p className="text-muted-foreground text-sm">
								{t("security.requiredHeadersDesc")}
							</p>
						</div>
						<Textarea
							id="required-headers"
							data-testid="required-headers-textarea"
							className="h-24"
							placeholder={t("security.requiredHeadersPlaceholder")}
							value={localValues.required_headers}
							onChange={(e) => handleRequiredHeadersChange(e.target.value)}
						/>
					</div>
				</div>
				{/* Whitelisted Routes */}
				<div>
					<div className="space-y-2 rounded-sm border p-4">
						<div className="space-y-0.5">
							<label htmlFor="whitelisted-routes" className="text-sm font-medium">
								{t("security.whitelistedRoutes")}
							</label>
							<p className="text-muted-foreground text-sm">
								{t("security.whitelistedRoutesDesc")}
							</p>
						</div>
						<Textarea
							id="whitelisted-routes"
							data-testid="whitelisted-routes-textarea"
							className="h-24"
							placeholder={t("security.whitelistedRoutesPlaceholder")}
							value={localValues.whitelisted_routes}
							onChange={(e) => handleWhitelistedRoutesChange(e.target.value)}
						/>
					</div>
				</div>
			</div>
			<div className="bg-card sticky bottom-0 flex justify-end py-2">
				<Button onClick={handleSave} disabled={!hasChanges || isLoading || !hasSettingsUpdateAccess}>
					{isLoading ? t("actions.saving") : t("actions.saveChanges")}
				</Button>
			</div>
			<Dialog open={!!setupTokenErrorMessage} onOpenChange={(open) => !open && setSetupTokenErrorMessage(null)}>
				<DialogContent data-testid="setup-token-error-dialog">
					<DialogHeader>
						<DialogTitle>{t("security.setupTokenRequired")}</DialogTitle>
						<DialogDescription>{setupTokenErrorMessage}</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button variant="outline" onClick={() => setSetupTokenErrorMessage(null)} data-testid="setup-token-error-close">
							{t("security.close")}
						</Button>
						
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}

const RestartWarning = ({ t }: { t: (key: string) => string }) => {
	return (
		<Alert variant="destructive" className="mt-2">
			<AlertTriangle className="h-4 w-4" />
			<AlertDescription>{t("security.restartWarning")}</AlertDescription>
		</Alert>
	);
};
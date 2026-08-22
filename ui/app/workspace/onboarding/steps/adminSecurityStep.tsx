import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { SecretVarInput } from "@/components/ui/secretVarInput";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage, useGetCoreConfigQuery, useUpdateCoreConfigMutation } from "@/lib/store";
import { AuthConfig, CoreConfig, DefaultCoreConfig } from "@/lib/types/config";
import { parseArrayFromText } from "@/lib/utils/array";
import { Info, Loader2, Save } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

const DefaultAuthConfig: AuthConfig = {
	admin_username: { value: "", ref: "" },
	admin_password: { value: "", ref: "" },
	is_enabled: false,
};

export default function AdminSecurityStep() {
	const { t } = useTranslation("onboarding");
	const { data: bifrostConfig } = useGetCoreConfigQuery({ fromDB: true });
	const [updateCoreConfig, { isLoading }] = useUpdateCoreConfigMutation();

	const [localConfig, setLocalConfig] = useState<CoreConfig>(DefaultCoreConfig);
	const [authConfig, setAuthConfig] = useState<AuthConfig>(DefaultAuthConfig);
	const [corsText, setCorsText] = useState("");
	const [submitted, setSubmitted] = useState(false);

	const isFirstTimeSetup = !bifrostConfig?.auth_config;

	useEffect(() => {
		if (!bifrostConfig) return;
		setLocalConfig(bifrostConfig.client_config);
		setCorsText((bifrostConfig.client_config.allowed_origins ?? []).join(", "));
		if (bifrostConfig.auth_config) {
			setAuthConfig(bifrostConfig.auth_config);
		}
	}, [bifrostConfig]);

	const handleSave = useCallback(async () => {
		setSubmitted(true);

		const username = authConfig.admin_username?.value?.trim() ?? "";
		const password = authConfig.admin_password?.value?.trim() ?? "";
		if (authConfig.is_enabled && (!username || !password)) {
			toast.error(t("adminUsername") + " / " + t("adminPassword"));
			return;
		}

		try {
			const nextConfig = {
				...bifrostConfig!,
				client_config: {
					...localConfig,
					allowed_origins: parseArrayFromText(corsText),
				},
				// When no admin exists yet, skip auth_config — the operator must
				// use the CLI to create the first admin account.
				auth_config: isFirstTimeSetup
					? undefined
					: {
							...authConfig,
							is_enabled: authConfig.is_enabled && !!username && !!password,
						},
			};
			await updateCoreConfig(nextConfig).unwrap();
			toast.success("Saved");
			setSubmitted(false);
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	}, [authConfig, bifrostConfig, corsText, isFirstTimeSetup, localConfig, t, updateCoreConfig]);

	const hasAuthConfig = !isFirstTimeSetup;

	return (
		<div className="space-y-6">
			<div className="space-y-1 text-center sm:text-left">
				<h2 className="text-xl font-semibold tracking-tight">{t("step.admin")}</h2>
				<p className="text-muted-foreground text-sm">{t("adminDesc")}</p>
			</div>

			<div className="bg-card space-y-4 rounded-md border p-4">
				<div className="flex items-start justify-between gap-4">
					<div className="space-y-1">
						<Label htmlFor="onb-auth-enabled" className="text-sm font-medium">
							{t("enableDashboardAuth")}
						</Label>
						<p className="text-muted-foreground text-xs">{t("enableDashboardAuthDesc")}</p>
					</div>
					<Switch
						id="onb-auth-enabled"
						checked={authConfig.is_enabled}
						onCheckedChange={(checked) => setAuthConfig((prev) => ({ ...prev, is_enabled: checked }))}
					/>
				</div>

				{hasAuthConfig ? (
					<div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
						<div className="space-y-2">
							<Label htmlFor="onb-admin-username">{t("adminUsername")}</Label>
							<SecretVarInput
								id="onb-admin-username"
								type="text"
								placeholder={t("adminUsernamePlaceholder")}
								value={authConfig.admin_username}
								disabled={!authConfig.is_enabled}
								onChange={(value) => setAuthConfig((prev) => ({ ...prev, admin_username: value }))}
							/>
						</div>
						<div className="space-y-2">
							<Label htmlFor="onb-admin-password">{t("adminPassword")}</Label>
							<SecretVarInput
								id="onb-admin-password"
								type="password"
								placeholder={t("adminPasswordPlaceholder")}
								value={authConfig.admin_password}
								disabled={!authConfig.is_enabled}
								onChange={(value) => setAuthConfig((prev) => ({ ...prev, admin_password: value }))}
							/>
						</div>
					</div>
				) : authConfig.is_enabled ? (
					<div className="rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-xs text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
						<p className="mb-1 font-semibold">{t("setupTokenCliTitle")}</p>
						<p className="mb-2">{t("setupTokenCliDesc")}</p>
						<code className="block rounded bg-amber-100 px-2 py-1 font-mono text-xs dark:bg-amber-900/40">{t("setupTokenCliCommand")}</code>
					</div>
				) : null}

				{!authConfig.is_enabled && (
					<div className="flex items-start gap-2 rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-xs text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
						<Info className="mt-0.5 h-4 w-4 shrink-0" />
						<span>{t("adminSkipHint")}</span>
					</div>
				)}
			</div>

			<div className="bg-card space-y-2 rounded-md border p-4">
				<div className="space-y-1">
					<Label htmlFor="onb-cors" className="text-sm font-medium">
						{t("corsOrigins")}
					</Label>
					<p className="text-muted-foreground text-xs">{t("corsOriginsDesc")}</p>
				</div>
				<Textarea
					id="onb-cors"
					rows={2}
					placeholder={t("corsOriginsPlaceholder")}
					value={corsText}
					onChange={(e) => setCorsText(e.target.value)}
				/>
			</div>

			<div className="bg-card space-y-2 rounded-md border p-4">
				<div className="flex items-start justify-between gap-4">
					<div className="space-y-1">
						<Label htmlFor="onb-enforce-auth" className="text-sm font-medium">
							{t("enforceAuthOnInference")}
						</Label>
						<p className="text-muted-foreground text-xs">{t("enforceAuthOnInferenceDesc")}</p>
					</div>
					<Switch
						id="onb-enforce-auth"
						checked={localConfig.enforce_auth_on_inference}
						onCheckedChange={(checked) => setLocalConfig((prev) => ({ ...prev, enforce_auth_on_inference: checked }))}
					/>
				</div>
			</div>

			{hasAuthConfig && (
				<div className="flex items-center justify-end gap-2">
					<Button
						size="sm"
						onClick={() => void handleSave()}
						disabled={isLoading || (submitted && isLoading)}
						data-testid="onboarding-admin-save"
					>
						{isLoading ? (
							<>
								<Loader2 className="mr-1 h-4 w-4 animate-spin" />
								Saving…
							</>
						) : (
							<>
								<Save className="mr-1 h-4 w-4" />
								{t("action.save", { ns: "common" })}
							</>
						)}
					</Button>
				</div>
			)}
		</div>
	);
}
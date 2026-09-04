import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage, useGetCoreConfigQuery, useUpdateCoreConfigMutation } from "@/lib/store";
import { CoreConfig, DefaultCoreConfig } from "@/lib/types/config";
import { parseArrayFromText } from "@/lib/utils/array";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { toast } from "sonner";

function SectionTitle({ children }: { children: React.ReactNode }) {
	return <h3 className="text-muted-foreground px-1 text-xs font-semibold tracking-wide uppercase">{children}</h3>;
}

export default function LoggingView() {
	const { t } = useTranslation("config");
	const hasSettingsUpdateAccess = useRbac(RbacResource.Settings, RbacOperation.Update);
	const { data: bifrostConfig } = useGetCoreConfigQuery({ fromDB: true });
	const config = bifrostConfig?.client_config;
	const [updateCoreConfig, { isLoading }] = useUpdateCoreConfigMutation();
	const [localConfig, setLocalConfig] = useState<CoreConfig>(DefaultCoreConfig);
	const [needsRestart, setNeedsRestart] = useState<boolean>(false);
	const [loggingHeadersText, setLoggingHeadersText] = useState<string>("");

	useEffect(() => {
		if (config) {
			setLocalConfig(config);
			setLoggingHeadersText(config.logging_headers?.join(", ") || "");
		}
	}, [config]);

	const hasChanges = useMemo(() => {
		if (!config) return false;
		return (
			localConfig.enable_logging !== config.enable_logging ||
			localConfig.disable_content_logging !== config.disable_content_logging ||
			localConfig.retain_content_in_object_storage !== config.retain_content_in_object_storage ||
			localConfig.allow_per_request_content_storage_override !== config.allow_per_request_content_storage_override ||
			localConfig.allow_per_request_raw_override !== config.allow_per_request_raw_override ||
			localConfig.log_retention_days !== config.log_retention_days ||
			localConfig.payload_retention_days !== config.payload_retention_days ||
			localConfig.hide_deleted_virtual_keys_in_filters !== config.hide_deleted_virtual_keys_in_filters ||
			JSON.stringify(localConfig.logging_headers || []) !== JSON.stringify(config.logging_headers || [])
		);
	}, [config, localConfig]);

	const handleConfigChange = useCallback((field: keyof CoreConfig, value: boolean | number | string[]) => {
		setLocalConfig((prev) => ({ ...prev, [field]: value }));
		if (field === "enable_logging") {
			setNeedsRestart(true);
		}
	}, []);

	const handleLoggingHeadersChange = useCallback((value: string) => {
		setLoggingHeadersText(value);
		setLocalConfig((prev) => ({ ...prev, logging_headers: parseArrayFromText(value) }));
	}, []);

	const handleSave = useCallback(async () => {
		if (!bifrostConfig) {
			toast.error(t("toast.configNotLoaded"));
			return;
		}

		if (localConfig.log_retention_days < 1) {
			toast.error(t("toast.logRetentionDaysMin"));
			return;
		}

		try {
			await updateCoreConfig({ ...bifrostConfig, client_config: localConfig }).unwrap();
			toast.success(t("toast.loggingUpdated"));
		} catch (error) {
			toast.error(getErrorMessage(error));
		}
	}, [bifrostConfig, localConfig, updateCoreConfig, t]);

	const loggingEnabled = localConfig.enable_logging && bifrostConfig?.is_logs_connected;
	const contentLoggingOn = !localConfig.disable_content_logging;
	const objectStorageConnected = bifrostConfig?.is_object_storage_connected === true;
	const showBackupSection =
		loggingEnabled &&
		(localConfig.disable_content_logging || localConfig.allow_per_request_content_storage_override) &&
		objectStorageConnected;

	return (
		<div className="mx-auto w-full max-w-4xl space-y-4 py-6">
			<div>
				<h2 className="text-lg font-semibold tracking-tight">{t("page.logsSettings")}</h2>
				<p className="text-muted-foreground text-sm">{t("descriptions.logsSettings")}</p>
			</div>

			<div className="space-y-6">
				{/* Basic */}
				<section className="space-y-4">
					<SectionTitle>{t("logging.section.basic")}</SectionTitle>
					<div>
						<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
							<div className="space-y-0.5">
								<label htmlFor="enable-logging" className="text-sm font-medium">
									{t("logging.enableLogs")}
								</label>
								<p className="text-muted-foreground text-sm">
									{t("logging.enableLogsDesc")}
									{!bifrostConfig?.is_logs_connected && (
										<span className="text-destructive font-medium"> {t("logging.requiresLogsStore")}</span>
									)}
								</p>
							</div>
							<Switch
								id="enable-logging"
								size="md"
								checked={localConfig.enable_logging && bifrostConfig?.is_logs_connected}
								disabled={!bifrostConfig?.is_logs_connected}
								onCheckedChange={(checked) => {
									if (bifrostConfig?.is_logs_connected) {
										handleConfigChange("enable_logging", checked);
									}
								}}
							/>
						</div>
						{needsRestart && <RestartWarning />}
					</div>

					{loggingEnabled && (
						<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
							<div className="space-y-0.5">
								<label htmlFor="record-content-logging" className="text-sm font-medium">
									{t("logging.recordContent")}
								</label>
								<p className="text-muted-foreground text-sm">{t("logging.recordContentDesc")}</p>
							</div>
							<Switch
								id="record-content-logging"
								data-testid="workspace-record-content-logging-switch"
								size="md"
								checked={contentLoggingOn}
								onCheckedChange={(checked) => handleConfigChange("disable_content_logging", !checked)}
							/>
						</div>
					)}
				</section>

				{/* Content & Retention — only when content logging is on */}
				{loggingEnabled && contentLoggingOn && (
					<section className="space-y-4">
						<SectionTitle>{t("logging.section.retention")}</SectionTitle>
						<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
							<div className="space-y-0.5">
								<Label htmlFor="log-retention-days" className="text-sm font-medium">
									{t("logging.logRetentionDays")}
								</Label>
								<p className="text-muted-foreground text-sm">{t("logging.logRetentionDaysDesc")}</p>
							</div>
							<Input
								id="log-retention-days"
								type="number"
								min="1"
								value={localConfig.log_retention_days}
								onChange={(e) => {
									const value = parseInt(e.target.value) || 1;
									handleConfigChange("log_retention_days", Math.max(1, value));
								}}
								className="w-24"
							/>
						</div>

						<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
							<div className="space-y-0.5">
								<Label htmlFor="payload-retention-days" className="text-sm font-medium">
									{t("logging.payloadRetentionDays")}
								</Label>
								<p className="text-muted-foreground text-sm">{t("logging.payloadRetentionDaysDesc")}</p>
							</div>
							<Input
								id="payload-retention-days"
								data-testid="workspace-payload-retention-days-input"
								type="number"
								min="0"
								value={localConfig.payload_retention_days}
								onChange={(e) => {
									const value = parseInt(e.target.value) || 0;
									handleConfigChange("payload_retention_days", Math.max(0, value));
								}}
								className="w-24"
							/>
						</div>
					</section>
				)}

				{/* Data backup — only when content can be disabled (global or per-request) */}
				{showBackupSection && (
					<section className="space-y-4">
						<SectionTitle>{t("logging.section.backup")}</SectionTitle>
						<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
							<div className="space-y-0.5">
								<label htmlFor="retain-content-in-object-storage" className="text-sm font-medium">
									{t("logging.retainContentInObjectStorage")}
								</label>
								<p className="text-muted-foreground text-sm">
									<Trans
										i18nKey="logging.retainContentInObjectStorageDesc"
										ns="config"
										components={{
											1: <code className="text-xs" />,
										}}
									/>
									{!objectStorageConnected && <span className="text-destructive font-medium"> {t("logging.requiresObjectStorage")}</span>}
								</p>
							</div>
							<Switch
								id="retain-content-in-object-storage"
								data-testid="workspace-retain-content-in-object-storage-switch"
								size="md"
								checked={localConfig.retain_content_in_object_storage && objectStorageConnected}
								disabled={!objectStorageConnected}
								onCheckedChange={(checked) => {
									if (objectStorageConnected) {
										handleConfigChange("retain_content_in_object_storage", checked);
									}
								}}
							/>
						</div>
					</section>
				)}

				{/* Advanced — shown whenever logging is enabled */}
				{loggingEnabled && (
					<section className="space-y-4">
						<SectionTitle>{t("logging.section.advanced")}</SectionTitle>
						<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
							<div className="space-y-0.5">
								<label htmlFor="allow-per-request-content-storage-override" className="text-sm font-medium">
									{t("logging.allowPerRequestContentStorageOverride")}
								</label>
								<p className="text-muted-foreground text-sm">
									<Trans
										i18nKey="logging.allowPerRequestContentStorageOverrideDesc"
										ns="config"
										components={{
											1: <code className="text-xs" />,
											3: <code className="text-xs" />,
											5: <code className="text-xs" />,
											7: <code className="text-xs" />,
										}}
									/>
								</p>
							</div>
							<Switch
								id="allow-per-request-content-storage-override"
								data-testid="workspace-content-storage-override-switch"
								size="md"
								checked={localConfig.allow_per_request_content_storage_override}
								onCheckedChange={(checked) => handleConfigChange("allow_per_request_content_storage_override", checked)}
							/>
						</div>

						<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
							<div className="space-y-0.5">
								<label htmlFor="allow-per-request-raw-override" className="text-sm font-medium">
									{t("logging.allowPerRequestRawOverride")}
								</label>
								<p className="text-muted-foreground text-sm">
									<Trans
										i18nKey="logging.allowPerRequestRawOverrideDesc"
										ns="config"
										components={{
											1: <code className="text-xs" />,
											3: <code className="text-xs" />,
										}}
									/>
								</p>
							</div>
							<Switch
								id="allow-per-request-raw-override"
								data-testid="workspace-raw-override-switch"
								size="md"
								checked={localConfig.allow_per_request_raw_override}
								onCheckedChange={(checked) => handleConfigChange("allow_per_request_raw_override", checked)}
							/>
						</div>

						<div className="space-y-2 rounded-sm border p-4">
							<label htmlFor="logging-headers" className="text-sm font-medium">
								{t("logging.loggingHeaders")}
							</label>
							<p className="text-muted-foreground text-sm">
								<Trans
									i18nKey="logging.loggingHeadersDesc"
									ns="config"
									components={{
										1: <code className="text-xs" />,
										3: <code className="text-xs" />,
										5: <code className="text-xs" />,
										7: <code className="text-xs" />,
									}}
								/>
							</p>
							<Textarea
								id="logging-headers"
								data-testid="workspace-logging-headers-textarea"
								className="h-24"
								placeholder={t("logging.loggingHeadersPlaceholder")}
								value={loggingHeadersText}
								onChange={(e) => handleLoggingHeadersChange(e.target.value)}
							/>
						</div>
					</section>
				)}

				{/* Display — independent of logging */}
				<section className="space-y-4">
					<SectionTitle>{t("logging.section.display")}</SectionTitle>
					<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
						<div className="space-y-0.5">
							<label htmlFor="hide-deleted-virtual-keys-in-filters" className="text-sm font-medium">
								{t("logging.hideDeletedVirtualKeys")}
							</label>
							<p className="text-muted-foreground text-sm">{t("logging.hideDeletedVirtualKeysDesc")}</p>
						</div>
						<Switch
							id="hide-deleted-virtual-keys-in-filters"
							data-testid="hide-deleted-virtual-keys-in-filters-switch"
							size="md"
							checked={localConfig.hide_deleted_virtual_keys_in_filters}
							onCheckedChange={(checked) => handleConfigChange("hide_deleted_virtual_keys_in_filters", checked)}
						/>
					</div>
				</section>
			</div>

			<div className="flex justify-end pt-2">
				<Button onClick={handleSave} disabled={!hasChanges || isLoading || !hasSettingsUpdateAccess}>
					{isLoading ? t("logging.saving") : t("logging.saveChanges")}
				</Button>
			</div>
		</div>
	);
}

const RestartWarning = () => {
	const { t } = useTranslation("config");
	return <div className="text-muted-foreground mt-2 pl-4 text-xs font-semibold">{t("logging.restartWarning")}</div>;
};
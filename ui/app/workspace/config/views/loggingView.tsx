import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage, useGetCoreConfigQuery, useUpdateCoreConfigMutation } from "@/lib/store";
import { CoreConfig, DefaultCoreConfig } from "@/lib/types/config";
import { parseArrayFromText } from "@/lib/utils/array";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { parseAsStringLiteral, useQueryState } from "nuqs";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

const LOGGING_TABS = ["requests", "app"] as const;

// Radix Select treats an empty string as the placeholder state, so the "follow
// startup default" option needs a real value; mapped back to "" before saving.
const UNSET = "__unset__";

const LOG_LEVEL_OPTIONS = ["debug", "info", "warn", "error"] as const;
const LOG_STYLE_OPTIONS = ["json", "pretty"] as const;

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
	const [activeTab, setActiveTab] = useQueryState("tab", parseAsStringLiteral(LOGGING_TABS).withDefault("requests"));

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
			(localConfig.log_level || "") !== (config.log_level || "") ||
			(localConfig.log_output_style || "") !== (config.log_output_style || "") ||
			localConfig.dump_errors_in_console_logs !== config.dump_errors_in_console_logs ||
			JSON.stringify(localConfig.logging_headers || []) !== JSON.stringify(config.logging_headers || [])
		);
	}, [config, localConfig]);

	const handleConfigChange = useCallback((field: keyof CoreConfig, value: boolean | number | string | string[]) => {
		setLocalConfig((prev) => ({ ...prev, [field]: value }));
		if (field === "enable_logging") {
			setNeedsRestart(true);
		}
	}, []);

	const handleTabChange = (value: string) => {
		setActiveTab(value as (typeof LOGGING_TABS)[number]);
	};

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

	const logsConnected = bifrostConfig?.is_logs_connected === true;
	const loggingEnabled = localConfig.enable_logging && logsConnected;
	const contentLoggingOn = !localConfig.disable_content_logging;
	const objectStorageConnected = bifrostConfig?.is_object_storage_connected === true;
	// Backing up skipped content only makes sense when some requests can end up
	// without logged content (globally or per request).
	const showBackupSection =
		loggingEnabled && (localConfig.disable_content_logging || localConfig.allow_per_request_content_storage_override);

	// Effective application log settings: the persisted override when set,
	// otherwise the runtime value reported by the backend (startup default).
	const effectiveLogLevel = localConfig.log_level || bifrostConfig?.runtime_log_level || "info";
	const effectiveLogStyle = localConfig.log_output_style || bifrostConfig?.runtime_log_output_style || "json";
	const followsStartupDefault = !localConfig.log_level && !localConfig.log_output_style;

	return (
		<div className="mx-auto w-full max-w-7xl space-y-4">
			<div>
				<h2 className="text-lg font-semibold tracking-tight">{t("page.logsSettings")}</h2>
				<p className="text-muted-foreground text-sm">{t("descriptions.logsSettings")}</p>
			</div>

			<Tabs value={activeTab} onValueChange={handleTabChange} className="gap-4">
				<TabsList>
					<TabsTrigger value="requests" data-testid="logging-tab-requests">
						{t("logging.tabs.requests")}
					</TabsTrigger>
					<TabsTrigger value="app" data-testid="logging-tab-app">
						{t("logging.tabs.app")}
					</TabsTrigger>
				</TabsList>

				{/* Tab 1: Request Logs */}
				<TabsContent value="requests" className="space-y-6">
					{/* Intro: what request logs are and where they are stored */}
					<div className="bg-muted/40 rounded-sm border px-4 py-3 text-sm" data-testid="logging-intro-requests">
						<p>
							<span className="font-medium">{t("logging.introWhat")}: </span>
							{t("logging.introRequestsDesc")}
						</p>
						<p className="text-muted-foreground mt-1">
							<span className="font-medium">{t("logging.introWhere")}: </span>
							{t("logging.introRequestsStorage")}
						</p>
					</div>

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
										{!logsConnected && <span className="text-destructive font-medium"> {t("logging.requiresLogsStore")}</span>}
									</p>
								</div>
								<Switch
									id="enable-logging"
									size="md"
									checked={loggingEnabled}
									disabled={!logsConnected}
									onCheckedChange={(checked) => {
										if (logsConnected) {
											handleConfigChange("enable_logging", checked);
										}
									}}
								/>
							</div>
							{needsRestart && <RestartWarning />}
						</div>
					</section>

					{/* Content & Retention */}
					{loggingEnabled && (
						<section className="space-y-4">
							<SectionTitle>{t("logging.section.retention")}</SectionTitle>
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

							{contentLoggingOn && (
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
							)}
						</section>
					)}

					{/* Advanced (collapsible) */}
					{loggingEnabled && (
						<Accordion type="single" collapsible className="rounded-sm border px-4">
							<AccordionItem value="advanced" className="border-b-0">
								<AccordionTrigger data-testid="logging-advanced-trigger" className="text-sm font-medium">
									{t("logging.section.advanced")}
								</AccordionTrigger>
								<AccordionContent className="space-y-2 pt-2">
									<div className="flex items-center justify-between space-x-2 border-t p-4">
										<div className="space-y-0.5">
											<Label htmlFor="allow-per-request-content-storage-override" className="text-sm font-medium">
												{t("logging.allowPerRequestContentStorageOverride")}
											</Label>
											<p className="text-muted-foreground text-sm">{t("logging.allowPerRequestContentStorageOverrideDesc")}</p>
										</div>
										<Switch
											id="allow-per-request-content-storage-override"
											data-testid="workspace-content-storage-override-switch"
											size="md"
											checked={localConfig.allow_per_request_content_storage_override}
											onCheckedChange={(checked) => handleConfigChange("allow_per_request_content_storage_override", checked)}
										/>
									</div>

									<div className="flex items-center justify-between space-x-2 border-t p-4">
										<div className="space-y-0.5">
											<Label htmlFor="allow-per-request-raw-override" className="text-sm font-medium">
												{t("logging.allowPerRequestRawOverride")}
											</Label>
											<p className="text-muted-foreground text-sm">{t("logging.allowPerRequestRawOverrideDesc")}</p>
										</div>
										<Switch
											id="allow-per-request-raw-override"
											data-testid="workspace-raw-override-switch"
											size="md"
											checked={localConfig.allow_per_request_raw_override}
											onCheckedChange={(checked) => handleConfigChange("allow_per_request_raw_override", checked)}
										/>
									</div>

									{showBackupSection && (
										<div className="flex items-center justify-between space-x-2 border-t p-4">
											<div className="space-y-0.5">
												<Label htmlFor="retain-content-in-object-storage" className="text-sm font-medium">
													{t("logging.retainContentInObjectStorage")}
												</Label>
												<p className="text-muted-foreground text-sm">
													{t("logging.retainContentInObjectStorageDesc")}
													{!objectStorageConnected && (
														<span className="text-destructive font-medium"> {t("logging.requiresObjectStorage")}</span>
													)}
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
									)}

									<div className="space-y-2 border-t p-4">
										<label htmlFor="logging-headers" className="text-sm font-medium">
											{t("logging.loggingHeaders")}
										</label>
										<p className="text-muted-foreground text-sm">{t("logging.loggingHeadersDesc")}</p>
										<Textarea
											id="logging-headers"
											data-testid="workspace-logging-headers-textarea"
											className="h-20"
											placeholder={t("logging.loggingHeadersPlaceholder")}
											value={loggingHeadersText}
											onChange={(e) => handleLoggingHeadersChange(e.target.value)}
										/>
									</div>
								</AccordionContent>
							</AccordionItem>
						</Accordion>
					)}

					{/* Display */}
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
				</TabsContent>

				{/* Tab 2: Application Logs */}
				<TabsContent value="app" className="space-y-6">
					{/* Intro: what application logs are and where they are stored */}
					<div className="bg-muted/40 rounded-sm border px-4 py-3 text-sm" data-testid="logging-intro-app">
						<p>
							<span className="font-medium">{t("logging.introWhat")}: </span>
							{t("logging.introAppDesc")}
						</p>
						<p className="text-muted-foreground mt-1">
							<span className="font-medium">{t("logging.introWhere")}: </span>
							{t("logging.introAppStorage")}
						</p>
					</div>
					<section className="space-y-4">
						<SectionTitle>{t("logging.section.appEffective")}</SectionTitle>
						<div className="rounded-sm border p-4 text-sm">
							<span className="text-muted-foreground">{t("logging.appEffectiveLabel")}: </span>
							<code className="bg-muted rounded px-1" data-testid="logging-app-effective-level">
								{effectiveLogLevel}
							</code>
							<span className="mx-1">·</span>
							<code className="bg-muted rounded px-1" data-testid="logging-app-effective-format">
								{effectiveLogStyle}
							</code>
							{followsStartupDefault && <span className="text-muted-foreground ml-2 text-xs">({t("logging.appFromBootArgs")})</span>}
						</div>
					</section>

					<section className="space-y-4">
						<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
							<div className="space-y-0.5">
								<label htmlFor="app-log-level" className="text-sm font-medium">
									{t("logging.appLogLevel")}
								</label>
								<p className="text-muted-foreground text-sm">{t("logging.appLogLevelDesc")}</p>
							</div>
							<Select
								value={localConfig.log_level || UNSET}
								onValueChange={(value) => handleConfigChange("log_level", value === UNSET ? "" : value)}
								disabled={!hasSettingsUpdateAccess}
							>
								<SelectTrigger id="app-log-level" data-testid="logging-app-level-select" className="w-44">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value={UNSET}>{t("logging.appUnsetOption")}</SelectItem>
									{LOG_LEVEL_OPTIONS.map((option) => (
										<SelectItem key={option} value={option}>
											{option}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>

						<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
							<div className="space-y-0.5">
								<label htmlFor="app-log-output-style" className="text-sm font-medium">
									{t("logging.appLogOutputStyle")}
								</label>
								<p className="text-muted-foreground text-sm">{t("logging.appLogOutputStyleDesc")}</p>
							</div>
							<Select
								value={localConfig.log_output_style || UNSET}
								onValueChange={(value) => handleConfigChange("log_output_style", value === UNSET ? "" : value)}
								disabled={!hasSettingsUpdateAccess}
							>
								<SelectTrigger id="app-log-output-style" data-testid="logging-app-format-select" className="w-44">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value={UNSET}>{t("logging.appUnsetOption")}</SelectItem>
									{LOG_STYLE_OPTIONS.map((option) => (
										<SelectItem key={option} value={option}>
											{option}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>

						<div className="flex items-center justify-between space-x-2 rounded-sm border p-4">
							<div className="space-y-0.5">
								<label htmlFor="dump-errors-in-console-logs" className="text-sm font-medium">
									{t("logging.dumpErrors")}
								</label>
								<p className="text-muted-foreground text-sm">{t("logging.dumpErrorsDesc")}</p>
							</div>
							<Switch
								id="dump-errors-in-console-logs"
								data-testid="logging-app-dump-errors-switch"
								size="md"
								checked={localConfig.dump_errors_in_console_logs}
								onCheckedChange={(checked) => handleConfigChange("dump_errors_in_console_logs", checked)}
								disabled={!hasSettingsUpdateAccess}
							/>
						</div>
					</section>
				</TabsContent>
			</Tabs>

			<div className="bg-card sticky bottom-0 flex justify-end py-2">
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
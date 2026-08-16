import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getErrorMessage, useGetCoreConfigQuery, useGetDroppedRequestsQuery, useUpdateCoreConfigMutation } from "@/lib/store";
import { CoreConfig, DefaultCoreConfig, DefaultGlobalHeaderFilterConfig, GlobalHeaderFilterConfig } from "@/lib/types/config";
import { cn } from "@/lib/utils";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { Info, Plus, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation, Trans } from "react-i18next";
import { toast } from "sonner";
import UserAgentMappingsView from "./userAgentMappingsView";

// Security headers that cannot be configured in allowlist/denylist
// These headers are always blocked for security reasons regardless of configuration
const SECURITY_HEADERS = [
	"proxy-authorization",
	"cookie",
	"host",
	"content-length",
	"connection",
	"transfer-encoding",
	"x-api-key",
	"x-goog-api-key",
	"x-bf-api-key",
	"x-bf-vk",
];

// Helper to check if a header is a security header
function isSecurityHeader(header: string): boolean {
	const h = header.toLowerCase().trim();
	// Wildcard patterns are not literal security headers
	if (h.includes("*")) return false;
	return SECURITY_HEADERS.includes(h);
}

// Helper to compare header filter configs
function headerFilterConfigEqual(a?: GlobalHeaderFilterConfig, b?: GlobalHeaderFilterConfig): boolean {
	const aAllowlist = a?.allowlist || [];
	const bAllowlist = b?.allowlist || [];
	const aDenylist = a?.denylist || [];
	const bDenylist = b?.denylist || [];

	if (aAllowlist.length !== bAllowlist.length || aDenylist.length !== bDenylist.length) {
		return false;
	}

	return aAllowlist.every((v, i) => v === bAllowlist[i]) && aDenylist.every((v, i) => v === bDenylist[i]);
}

// Helper to compare large payload configs

export default function ClientSettingsView() {
	const { t } = useTranslation("config");
	const hasSettingsUpdateAccess = useRbac(RbacResource.Settings, RbacOperation.Update);
	const [droppedRequests, setDroppedRequests] = useState<number>(0);
	const { data: droppedRequestsData } = useGetDroppedRequestsQuery();
	const { data: bifrostConfig, isLoading: isCoreConfigLoading } = useGetCoreConfigQuery({ fromDB: true });
	const config = bifrostConfig?.client_config;
	const [updateCoreConfig, { isLoading: isSavingCoreConfig }] = useUpdateCoreConfigMutation();
	const [localConfig, setLocalConfig] = useState<CoreConfig>(DefaultCoreConfig);

	const isQueriesLoading = isCoreConfigLoading;
	const isLoading = isSavingCoreConfig;

	useEffect(() => {
		if (droppedRequestsData) {
			setDroppedRequests(droppedRequestsData.dropped_requests);
		}
	}, [droppedRequestsData]);

	useEffect(() => {
		if (config) {
			setLocalConfig({
				...config,
				header_filter_config: config.header_filter_config || DefaultGlobalHeaderFilterConfig,
			});
		}
	}, [config]);

	const hasCoreConfigChanges = useMemo(() => {
		if (!config) return false;
		return (
			localConfig.drop_excess_requests !== config.drop_excess_requests ||
			localConfig.disable_db_pings_in_health !== config.disable_db_pings_in_health ||
			localConfig.dump_errors_in_console_logs !== config.dump_errors_in_console_logs ||
			localConfig.async_job_result_ttl !== config.async_job_result_ttl ||
			!headerFilterConfigEqual(localConfig.header_filter_config, config.header_filter_config)
		);
	}, [config, localConfig]);

	const hasChanges = hasCoreConfigChanges;

	// Detect security headers in allowlist/denylist
	const invalidSecurityHeaders = useMemo(() => {
		const allowlist = localConfig.header_filter_config?.allowlist || [];
		const denylist = localConfig.header_filter_config?.denylist || [];
		const invalidInAllowlist = allowlist.filter((h) => h && isSecurityHeader(h));
		const invalidInDenylist = denylist.filter((h) => h && isSecurityHeader(h));
		return [...new Set([...invalidInAllowlist, ...invalidInDenylist])];
	}, [localConfig.header_filter_config]);

	const hasSecurityHeaderError = invalidSecurityHeaders.length > 0;

	const handleConfigChange = useCallback((field: keyof CoreConfig, value: boolean | number | string[] | GlobalHeaderFilterConfig) => {
		setLocalConfig((prev) => ({ ...prev, [field]: value }));
	}, []);

	const handleSave = useCallback(async () => {
		// Defense in depth - don't save if security headers are present
		if (hasSecurityHeaderError) {
			return;
		}

		let coreConfigSaved = false;

		// Save core config if changed
		if (hasCoreConfigChanges) {
			if (!bifrostConfig) {
				toast.error(t("clientSettings.configNotLoaded"));
				return;
			}
			// Clean up empty strings from header filter config
			const cleanedConfig = {
				...localConfig,
				header_filter_config: {
					allowlist: (localConfig.header_filter_config?.allowlist || []).filter((h) => h && h.trim().length > 0),
					denylist: (localConfig.header_filter_config?.denylist || []).filter((h) => h && h.trim().length > 0),
				},
			};

			try {
				await updateCoreConfig({ ...bifrostConfig!, client_config: cleanedConfig }).unwrap();
				coreConfigSaved = true;
			} catch (error) {
				toast.error(t("clientSettings.saveFailed", { error: getErrorMessage(error) }));
			}
		}

		if (coreConfigSaved) {
			toast.success(t("clientSettings.saved"));
		}
	}, [bifrostConfig, hasSecurityHeaderError, hasCoreConfigChanges, localConfig, t, updateCoreConfig]);

	// Header filter list handlers
	const handleAddAllowlistHeader = useCallback(() => {
		setLocalConfig((prev) => ({
			...prev,
			header_filter_config: {
				...prev.header_filter_config,
				allowlist: [...(prev.header_filter_config?.allowlist || []), ""],
			},
		}));
	}, []);

	const handleRemoveAllowlistHeader = useCallback((index: number) => {
		setLocalConfig((prev) => ({
			...prev,
			header_filter_config: {
				...prev.header_filter_config,
				allowlist: (prev.header_filter_config?.allowlist || []).filter((_, i) => i !== index),
			},
		}));
	}, []);

	const handleAllowlistChange = useCallback((index: number, value: string) => {
		const lowerValue = value.toLowerCase();
		setLocalConfig((prev) => ({
			...prev,
			header_filter_config: {
				...prev.header_filter_config,
				allowlist: (prev.header_filter_config?.allowlist || []).map((h, i) => (i === index ? lowerValue : h)),
			},
		}));
	}, []);

	const handleAddDenylistHeader = useCallback(() => {
		setLocalConfig((prev) => ({
			...prev,
			header_filter_config: {
				...prev.header_filter_config,
				denylist: [...(prev.header_filter_config?.denylist || []), ""],
			},
		}));
	}, []);

	const handleRemoveDenylistHeader = useCallback((index: number) => {
		setLocalConfig((prev) => ({
			...prev,
			header_filter_config: {
				...prev.header_filter_config,
				denylist: (prev.header_filter_config?.denylist || []).filter((_, i) => i !== index),
			},
		}));
	}, []);

	const handleDenylistChange = useCallback((index: number, value: string) => {
		const lowerValue = value.toLowerCase();
		setLocalConfig((prev) => ({
			...prev,
			header_filter_config: {
				...prev.header_filter_config,
				denylist: (prev.header_filter_config?.denylist || []).map((h, i) => (i === index ? lowerValue : h)),
			},
		}));
	}, []);

	return (
		<div className="mx-auto w-full max-w-4xl space-y-6">
			<div>
				<h2 className="text-lg font-semibold tracking-tight">{t("page.title")}</h2>
				<p className="text-muted-foreground text-sm">{t("page.description")}</p>
			</div>

			<div className="space-y-4">
{/* Drop Excess Requests */}
			<div className="flex items-center justify-between space-x-2">
				<div className="space-y-0.5">
					<label htmlFor="drop-excess-requests" className="text-sm font-medium">
						{t("clientSettings.dropExcessRequests")}
					</label>
					<p className="text-muted-foreground text-sm">
						{t("clientSettings.dropExcessRequestsDesc")}{" "}
						{localConfig.drop_excess_requests && droppedRequests > 0 ? (
							<span>
								<Trans
									t={t}
									i18nKey="clientSettings.droppedRequests"
									count={droppedRequests}
									components={{ 1: <b /> }}
								/>
							</span>
						) : (
							<></>
						)}
					</p>
				</div>
					<Switch
						id="drop-excess-requests"
						size="md"
						checked={localConfig.drop_excess_requests}
						onCheckedChange={(checked) => handleConfigChange("drop_excess_requests", checked)}
						disabled={!hasSettingsUpdateAccess}
					/>
				</div>

				{/* Disable DB Pings in Health */}
				<div className="flex items-center justify-between space-x-2">
					<div className="space-y-0.5">
						<label htmlFor="disable-db-pings-in-health" className="text-sm font-medium">
							{t("clientSettings.disableDbPings")}
						</label>
						<p className="text-muted-foreground text-sm">
							{t("clientSettings.disableDbPingsDesc")}
						</p>
					</div>
					<Switch
						id="disable-db-pings-in-health"
						size="md"
						checked={localConfig.disable_db_pings_in_health}
						onCheckedChange={(checked) => handleConfigChange("disable_db_pings_in_health", checked)}
						disabled={!hasSettingsUpdateAccess}
					/>
				</div>

				{/* Dump Errors in Console Logs */}
				<div className="flex items-center justify-between space-x-2">
					<div className="space-y-0.5">
						<label htmlFor="dump-errors-in-console-logs" className="text-sm font-medium">
							{t("clientSettings.dumpErrors")}
						</label>
						<p className="text-muted-foreground text-sm">
							{t("clientSettings.dumpErrorsDesc")}
						</p>
					</div>
					<Switch
						id="dump-errors-in-console-logs"
						data-testid="client-settings-dump-errors-switch"
						size="md"
						checked={localConfig.dump_errors_in_console_logs}
						onCheckedChange={(checked) => handleConfigChange("dump_errors_in_console_logs", checked)}
						disabled={!hasSettingsUpdateAccess}
					/>
				</div>
				{/* Async Job Result TTL */}
				<div className="flex items-center justify-between space-x-2">
					<div className="space-y-0.5">
						<label htmlFor="async-job-result-ttl" className="text-sm font-medium">
							{t("clientSettings.asyncJobResultTtl")}
						</label>
						<p className="text-muted-foreground text-sm">
							{t("clientSettings.asyncJobResultTtlDesc")}
						</p>
					</div>
					<Input
						id="async-job-result-ttl"
						type="number"
						min={1}
						className="w-32"
						value={localConfig.async_job_result_ttl}
						onChange={(e) => handleConfigChange("async_job_result_ttl", parseInt(e.target.value) || 0)}
						disabled={!hasSettingsUpdateAccess}
						data-testid="client-settings-async-job-result-ttl-input"
					/>
				</div>
			</div>

			<UserAgentMappingsView disabled={isLoading || !hasSettingsUpdateAccess} />

			{/* Header Filter Section */}
			<div className="space-y-4">
				<div>
					<h3 className="text-lg font-semibold tracking-tight">{t("clientSettings.headerForwarding")}</h3>
					<p className="text-muted-foreground text-sm">{t("clientSettings.headerForwardingDesc")}</p>
				</div>

				<Accordion type="multiple" className="w-full rounded-sm border px-4">
					<AccordionItem value="about-extra-headers">
						<AccordionTrigger>
							<span className="flex items-center gap-2">
								<Info className="h-4 w-4" />
								{t("clientSettings.aboutHeaderForwarding")}
							</span>
						</AccordionTrigger>
						<AccordionContent className="space-y-3">
							<div>
								<p className="mb-2 font-medium">{t("clientSettings.twoWaysToForward")}</p>
								<ul className="text-muted-foreground list-inside list-disc space-y-1 text-sm">
									<li>
										<span className="font-medium">{t("clientSettings.prefixedHeaders")}</span>{" "}
										<Trans
											t={t}
											i18nKey="clientSettings.prefixedHeadersDesc"
											components={{ 1: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />, 3: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />, 5: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" /> }}
										/>
									</li>
									<li>
										<span className="font-medium">{t("clientSettings.directHeaders")}</span>{" "}
										<Trans
											t={t}
											i18nKey="clientSettings.directHeadersDesc"
											components={{ 1: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" /> }}
										/>
									</li>
								</ul>
							</div>
							<div>
								<p className="mb-2 font-medium">{t("clientSettings.howAllowlistDenylistWork")}</p>
								<ul className="text-muted-foreground list-inside list-disc space-y-1 text-sm">
									<li>
										<span className="font-medium">{t("clientSettings.allowlistEmpty")}</span>{" "}
										<Trans
											t={t}
											i18nKey="clientSettings.allowlistEmptyDesc"
											components={{ 1: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" /> }}
										/>
									</li>
									<li>
										<span className="font-medium">{t("clientSettings.allowlistConfigured")}</span>{" "}
										{t("clientSettings.allowlistConfiguredDesc")}
									</li>
									<li>
										<span className="font-medium">{t("clientSettings.denylistRule")}</span>{" "}
										{t("clientSettings.denylistRuleDesc")}
									</li>
									<li>
										<span className="font-medium">{t("clientSettings.wildcards")}</span>{" "}
										<Trans
											t={t}
											i18nKey="clientSettings.wildcardsDesc"
											components={{ 1: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />, 3: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />, 5: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />, 7: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" /> }}
										/>
									</li>
								</ul>
							</div>
							<div>
								<p className="mb-2 font-medium">{t("clientSettings.important")}</p>
								<ul className="text-muted-foreground list-inside list-disc space-y-1 text-sm">
									<li>
										<Trans
											t={t}
											i18nKey="clientSettings.importantDesc1"
											components={{ 1: <span className="font-medium" />, 3: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" /> }}
										/>
									</li>
									<li>
										<Trans
											t={t}
											i18nKey="clientSettings.importantDesc2"
											components={{ 1: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />, 3: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />, 5: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" /> }}
										/>
									</li>
								</ul>
							</div>
						</AccordionContent>
					</AccordionItem>

					<AccordionItem value="security-note">
						<AccordionTrigger>
							<span className="flex items-center gap-2">
								<Info className="h-4 w-4" />
								{t("clientSettings.securityNote")}
							</span>
						</AccordionTrigger>
						<AccordionContent>
							<p className="text-sm">
								{t("clientSettings.securityNoteDesc")}
							</p>
							<p className="text-muted-foreground mt-1 font-mono text-xs">
								proxy-authorization, cookie, host, content-length, connection, transfer-encoding, x-api-key, x-goog-api-key, x-bf-api-key,
								x-bf-vk
							</p>
						</AccordionContent>
					</AccordionItem>
				</Accordion>

				{/* Allowlist Section */}
				<div className="space-y-3">
					<div className="space-y-1">
						<h4 className="text-sm font-medium">{t("clientSettings.allowlist")}</h4>
						<p className="text-muted-foreground text-xs">
							<Trans
								t={t}
								i18nKey="clientSettings.allowlistDesc"
								components={{ 1: <code className="bg-muted rounded px-1 font-mono" /> }}
							/>
						</p>
					</div>

					<div className="space-y-2">
						{(localConfig.header_filter_config?.allowlist || []).map((header, index) => (
							<div key={index} className="flex items-center gap-2">
								<Input
									placeholder={t("clientSettings.placeholderAllowlist")}
									data-testid="header-filter-allowlist-input"
									className={cn(
										"font-mono lowercase",
										isSecurityHeader(header) &&
											"border-destructive focus:border-destructive focus-visible:border-destructive focus-visible:ring-destructive/50",
									)}
									value={header}
									onChange={(e) => handleAllowlistChange(index, e.target.value)}
									disabled={!hasSettingsUpdateAccess}
								/>
								<Button
									type="button"
									variant="ghost"
									size="icon"
									onClick={() => handleRemoveAllowlistHeader(index)}
									className="text-muted-foreground hover:text-destructive"
									disabled={!hasSettingsUpdateAccess}
								>
									<X className="h-4 w-4" />
								</Button>
							</div>
						))}
						<Button type="button" variant="outline" size="sm" onClick={handleAddAllowlistHeader} disabled={!hasSettingsUpdateAccess}>
							<Plus className="mr-2 h-4 w-4" />
							{t("clientSettings.addHeader")}
						</Button>
					</div>
				</div>

				{/* Denylist Section */}
				<div className="space-y-3">
					<div className="space-y-1">
						<h4 className="text-sm font-medium">{t("clientSettings.denylist")}</h4>
						<p className="text-muted-foreground text-xs">
							<Trans
								t={t}
								i18nKey="clientSettings.denylistDesc"
								components={{ 1: <code className="bg-muted rounded px-1 font-mono" /> }}
							/>
						</p>
					</div>

					<div className="space-y-2">
						{(localConfig.header_filter_config?.denylist || []).map((header, index) => (
							<div key={index} className="flex items-center gap-2">
								<Input
									placeholder={t("clientSettings.placeholderDenylist")}
									data-testid="header-filter-denylist-input"
									className={cn(
										"font-mono lowercase",
										isSecurityHeader(header) &&
											"border-destructive focus:border-destructive focus-visible:border-destructive focus-visible:ring-destructive/50",
									)}
									value={header}
									onChange={(e) => handleDenylistChange(index, e.target.value)}
									disabled={!hasSettingsUpdateAccess}
								/>
								<Button
									type="button"
									variant="ghost"
									size="icon"
									onClick={() => handleRemoveDenylistHeader(index)}
									className="text-muted-foreground hover:text-destructive"
									disabled={!hasSettingsUpdateAccess}
								>
									<X className="h-4 w-4" />
								</Button>
							</div>
						))}
						<Button type="button" variant="outline" size="sm" onClick={handleAddDenylistHeader} disabled={!hasSettingsUpdateAccess}>
							<Plus className="mr-2 h-4 w-4" />
							{t("clientSettings.addHeader")}
						</Button>
					</div>
				</div>
			</div>

			<div className="flex justify-end pt-2">
				{hasSecurityHeaderError ? (
					<Tooltip>
						<TooltipTrigger asChild>
							<span>
								<Button disabled>{isLoading ? t("clientSettings.saving") : t("clientSettings.saveChanges")}</Button>
							</span>
						</TooltipTrigger>
						<TooltipContent>
							{t("clientSettings.removeSecurityHeaders", { count: invalidSecurityHeaders.length })}: {invalidSecurityHeaders.join(", ")}
						</TooltipContent>
					</Tooltip>
				) : (
					<Button onClick={handleSave} disabled={!hasChanges || isLoading || isQueriesLoading || !hasSettingsUpdateAccess}>
						{isLoading ? t("clientSettings.saving") : t("clientSettings.saveChanges")}
					</Button>
				)}
			</div>
		</div>
	);
}
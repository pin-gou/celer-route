import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetClose, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getErrorMessage, useGetCoreConfigQuery, useGetDroppedRequestsQuery, useUpdateCoreConfigMutation } from "@/lib/store";
import { CoreConfig, DefaultCoreConfig, DefaultGlobalHeaderFilterConfig, GlobalHeaderFilterConfig } from "@/lib/types/config";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { cn } from "@/lib/utils";
import { BookOpenText, Info, ShieldAlert, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation, Trans } from "react-i18next";
import { toast } from "sonner";
import UserAgentMappingsView from "./userAgentMappingsView";
import { SecretVarInput } from "@/components/ui/secretVarInput";

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

interface HeaderListCardProps {
	title: string;
	description: React.ReactNode;
	placeholder: string;
	addLabel: string;
	emptyHint: string;
	removeLabel: string;
	headers: string[];
	onAdd: (value: string) => void;
	onRemove: (index: number) => void;
	disabled: boolean;
	dataTestIdPrefix: string;
}

function HeaderListCard({
	title,
	description,
	placeholder,
	addLabel,
	emptyHint,
	removeLabel,
	headers,
	onAdd,
	onRemove,
	disabled,
	dataTestIdPrefix,
}: HeaderListCardProps) {
	const [draft, setDraft] = useState("");

	const submit = useCallback(
		(value: string) => {
			const normalized = value.toLowerCase().trim();
			if (!normalized) return;
			onAdd(normalized);
		},
		[onAdd],
	);

	const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
		if (e.key === "Enter") {
			e.preventDefault();
			submit(draft);
			setDraft("");
		} else if (e.key === "Backspace" && draft === "" && headers.length > 0) {
			onRemove(headers.length - 1);
		}
	};

	return (
		<Card>
			<CardHeader>
				<CardTitle className="text-base">{title}</CardTitle>
				<CardDescription>{description}</CardDescription>
			</CardHeader>
			<CardContent className="space-y-3">
				{headers.length === 0 ? (
					<p className="text-muted-foreground text-xs italic">{emptyHint}</p>
				) : (
					<div className="flex flex-wrap gap-2">
						{headers.map((header, index) => {
							const isSecurity = isSecurityHeader(header);
							return (
								<Badge
									key={`${dataTestIdPrefix}-${index}-${header}`}
									variant={isSecurity ? "destructive" : "secondary"}
									className={cn("font-mono text-xs", !disabled && "pr-1")}
								>
									{header}
									{!disabled && (
										<button
											type="button"
											onClick={() => onRemove(index)}
											className="hover:text-foreground ml-1.5 inline-flex items-center rounded-sm opacity-70 transition-opacity hover:opacity-100 focus:ring-1 focus:outline-none"
											aria-label={removeLabel}
											disabled={disabled}
										>
											<X className="h-3 w-3" />
										</button>
									)}
								</Badge>
							);
						})}
					</div>
				)}
				<div className="flex items-center gap-2">
					<Input
						value={draft}
						onChange={(e) => setDraft(e.target.value)}
						onKeyDown={handleKeyDown}
						onBlur={() => {
							if (draft) {
								submit(draft);
								setDraft("");
							}
						}}
						placeholder={placeholder}
						className="font-mono lowercase"
						disabled={disabled}
						data-testid={`${dataTestIdPrefix}-input`}
					/>
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={() => {
							submit(draft);
							setDraft("");
						}}
						disabled={disabled || !draft.trim()}
					>
						{addLabel}
					</Button>
				</div>
			</CardContent>
		</Card>
	);
}

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
	const handleAddAllowlistHeader = useCallback((value: string) => {
		setLocalConfig((prev) => {
			const current = prev.header_filter_config?.allowlist || [];
			if (current.includes(value)) return prev;
			return {
				...prev,
				header_filter_config: {
					...prev.header_filter_config,
					allowlist: [...current, value],
				},
			};
		});
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

	const handleAddDenylistHeader = useCallback((value: string) => {
		setLocalConfig((prev) => {
			const current = prev.header_filter_config?.denylist || [];
			if (current.includes(value)) return prev;
			return {
				...prev,
				header_filter_config: {
					...prev.header_filter_config,
					denylist: [...current, value],
				},
			};
		});
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

	return (
		<div className="mx-auto w-full max-w-4xl space-y-6" data-testid="client-settings-view">
			<div>
				<h2 className="text-lg font-semibold tracking-tight">{t("clientSettings.title")}</h2>
				<p className="text-muted-foreground text-sm">{t("clientSettings.description")}</p>
			</div>

			<Tabs defaultValue="common" className="gap-4">
				<TabsList>
					<TabsTrigger value="common" data-testid="client-settings-tab-common">
						{t("clientSettings.tabs.common")}
					</TabsTrigger>
					<TabsTrigger value="headerForwarding" data-testid="client-settings-tab-header-forwarding">
						{t("clientSettings.tabs.headerForwarding")}
					</TabsTrigger>
					<TabsTrigger value="appRecognition" data-testid="client-settings-tab-app-recognition">
						{t("clientSettings.tabs.appRecognition")}
					</TabsTrigger>
				</TabsList>

				{/* Tab 1: Common */}
				<TabsContent value="common" className="space-y-4">
					<Card>
						<CardHeader>
							<CardTitle className="text-base">{t("clientSettings.sections.trafficAndStability")}</CardTitle>
							<CardDescription>{t("clientSettings.sections.trafficAndStabilityDesc")}</CardDescription>
						</CardHeader>
						<CardContent className="space-y-5">
							{/* Drop Excess Requests */}
							<div className="flex items-start justify-between gap-4">
								<div className="space-y-0.5">
									<div className="flex items-center gap-1.5">
										<Label htmlFor="drop-excess-requests" className="text-sm font-medium">
											{t("clientSettings.dropExcessRequests")}
										</Label>
										<InfoTooltip description={t("clientSettings.dropExcessRequestsDesc")} />
									</div>
									<p className="text-muted-foreground text-sm">
										{t("clientSettings.dropExcessRequestsDesc")}{" "}
										{localConfig.drop_excess_requests && droppedRequests > 0 ? (
											<span>
												<Trans t={t} i18nKey="clientSettings.droppedRequests" count={droppedRequests} components={{ 1: <b /> }} />
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
							<div className="flex items-start justify-between gap-4">
								<div className="space-y-0.5">
									<div className="flex items-center gap-1.5">
										<Label htmlFor="disable-db-pings-in-health" className="text-sm font-medium">
											{t("clientSettings.disableDbPings")}
										</Label>
										<InfoTooltip description={t("clientSettings.disableDbPingsDesc")} />
									</div>
									<p className="text-muted-foreground text-sm">{t("clientSettings.disableDbPingsDesc")}</p>
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
							<div className="flex items-start justify-between gap-4">
								<div className="space-y-0.5">
									<div className="flex items-center gap-1.5">
										<Label htmlFor="dump-errors-in-console-logs" className="text-sm font-medium">
											{t("clientSettings.dumpErrors")}
										</Label>
										<InfoTooltip description={t("clientSettings.dumpErrorsDesc")} />
									</div>
									<p className="text-muted-foreground text-sm">{t("clientSettings.dumpErrorsDesc")}</p>
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
							<div className="flex items-start justify-between gap-4">
								<div className="space-y-0.5">
									<div className="flex items-center gap-1.5">
										<Label htmlFor="async-job-result-ttl" className="text-sm font-medium">
											{t("clientSettings.asyncJobResultTtl")}
										</Label>
										<InfoTooltip description={t("clientSettings.asyncJobResultTtlDesc")} />
									</div>
									<p className="text-muted-foreground text-sm">{t("clientSettings.asyncJobResultTtlDesc")}</p>
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
						</CardContent>
					</Card>

					<Card>
						<CardHeader>
							<CardTitle className="text-base">{t("clientSettings.sections.gatewayPublicUrl")}</CardTitle>
							<CardDescription>{t("clientSettings.sections.gatewayPublicUrlDesc")}</CardDescription>
						</CardHeader>
						<CardContent className="space-y-3">
							<div className="space-y-1.5">
								<div className="flex items-center gap-1.5">
									<Label htmlFor="celer-route-base-url" className="text-sm font-medium">
										{t("clientSettings.celerRouteBaseUrl")}
									</Label>
									<InfoTooltip description={t("clientSettings.celerRouteBaseUrlDesc")} />
								</div>
								<SecretVarInput
									id="celer-route-base-url"
									data-testid="celer-route-base-url-input"
									placeholder="https://celer-route.example.com or env.CELER_ROUTE_BASE_URL"
									value={localConfig.celer_route_base_url}
									onChange={(value) => setLocalConfig((prev) => ({ ...prev, celer_route_base_url: value }))}
									disabled={!hasSettingsUpdateAccess}
								/>
							</div>
						</CardContent>
					</Card>
				</TabsContent>

				{/* Tab 2: Header Forwarding */}
				<TabsContent value="headerForwarding" className="space-y-4">
					<div className="flex items-start justify-between gap-4">
						<p className="text-muted-foreground text-sm">{t("clientSettings.headerForwardingDesc")}</p>
						<HeaderForwardingHelpSheet />
					</div>

					<HeaderListCard
						title={t("clientSettings.sections.allowlist")}
						description={t("clientSettings.sections.allowlistDesc")}
						placeholder={`${t("clientSettings.help.addHeaderPlaceholder")} · ${t("clientSettings.help.allowlistExample")}`}
						addLabel={t("clientSettings.addHeader")}
						emptyHint={t("clientSettings.allowlistEmptyHint")}
						removeLabel={t("clientSettings.help.removeHeader")}
						headers={localConfig.header_filter_config?.allowlist || []}
						onAdd={handleAddAllowlistHeader}
						onRemove={handleRemoveAllowlistHeader}
						disabled={!hasSettingsUpdateAccess}
						dataTestIdPrefix="header-filter-allowlist"
					/>

					<HeaderListCard
						title={t("clientSettings.sections.denylist")}
						description={t("clientSettings.sections.denylistDesc")}
						placeholder={`${t("clientSettings.help.addHeaderPlaceholder")} · ${t("clientSettings.help.denylistExample")}`}
						addLabel={t("clientSettings.addHeader")}
						emptyHint={t("clientSettings.denylistEmptyHint")}
						removeLabel={t("clientSettings.help.removeHeader")}
						headers={localConfig.header_filter_config?.denylist || []}
						onAdd={handleAddDenylistHeader}
						onRemove={handleRemoveDenylistHeader}
						disabled={!hasSettingsUpdateAccess}
						dataTestIdPrefix="header-filter-denylist"
					/>

					<Alert variant="warning">
						<ShieldAlert className="h-4 w-4" />
						<AlertTitle>{t("clientSettings.sections.securityNote")}</AlertTitle>
						<AlertDescription>
							<p>{t("clientSettings.sections.securityNoteDesc")}</p>
							<p className="mt-1 font-mono text-xs break-all">{SECURITY_HEADERS.join(", ")}</p>
						</AlertDescription>
					</Alert>
				</TabsContent>

				{/* Tab 3: App Recognition (User Agent Mappings) */}
				<TabsContent value="appRecognition" className="space-y-4">
					<UserAgentMappingsView disabled={isLoading || !hasSettingsUpdateAccess} />
				</TabsContent>
			</Tabs>

			{/* Persistent save footer — applies to Common + Header Forwarding tabs.
			    App Recognition tab has its own internal save buttons inside UserAgentMappingsView. */}
			<div className="flex justify-end pt-2">
				<SaveButton
					hasChanges={hasChanges}
					isLoading={isLoading}
					isQueriesLoading={isQueriesLoading}
					hasAccess={hasSettingsUpdateAccess}
					hasSecurityHeaderError={hasSecurityHeaderError}
					invalidSecurityHeaders={invalidSecurityHeaders}
					onSave={handleSave}
				/>
			</div>
		</div>
	);
}

function InfoTooltip({ description }: { description: string }) {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					className="text-muted-foreground hover:text-foreground inline-flex items-center rounded-sm transition-colors focus:outline-none"
					aria-label="More info"
				>
					<Info className="h-3.5 w-3.5" />
				</button>
			</TooltipTrigger>
			<TooltipContent className="max-w-xs">{description}</TooltipContent>
		</Tooltip>
	);
}

function HeaderForwardingHelpSheet() {
	const { t } = useTranslation("config");
	return (
		<Sheet>
			<SheetTrigger asChild>
				<Button variant="outline" size="sm" type="button">
					<BookOpenText className="mr-2 h-4 w-4" />
					{t("clientSettings.help.openRules")}
				</Button>
			</SheetTrigger>
			<SheetContent side="right" className="gap-4 overflow-y-auto p-6">
				<SheetHeader>
					<SheetTitle>{t("clientSettings.help.helpDialogTitle")}</SheetTitle>
					<SheetDescription>{t("clientSettings.help.helpDialogDescription")}</SheetDescription>
				</SheetHeader>
				<div className="space-y-5 text-sm">
					<section className="space-y-2">
						<h4 className="font-medium">{t("clientSettings.twoWaysToForward")}</h4>
						<ul className="text-muted-foreground list-inside list-disc space-y-2">
							<li>
								<span className="text-foreground font-medium">{t("clientSettings.prefixedHeaders")}</span>{" "}
								<Trans
									t={t}
									i18nKey="clientSettings.prefixedHeadersDesc"
									components={{
										1: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />,
										3: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />,
										5: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />,
									}}
								/>
							</li>
							<li>
								<span className="text-foreground font-medium">{t("clientSettings.directHeaders")}</span>{" "}
								<Trans
									t={t}
									i18nKey="clientSettings.directHeadersDesc"
									components={{ 1: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" /> }}
								/>
							</li>
						</ul>
					</section>
					<section className="space-y-2">
						<h4 className="font-medium">{t("clientSettings.howAllowlistDenylistWork")}</h4>
						<ul className="text-muted-foreground list-inside list-disc space-y-2">
							<li>
								<span className="text-foreground font-medium">{t("clientSettings.allowlistEmpty")}</span>{" "}
								<Trans
									t={t}
									i18nKey="clientSettings.allowlistEmptyDesc"
									components={{ 1: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" /> }}
								/>
							</li>
							<li>
								<span className="text-foreground font-medium">{t("clientSettings.allowlistConfigured")}</span>{" "}
								{t("clientSettings.allowlistConfiguredDesc")}
							</li>
							<li>
								<span className="text-foreground font-medium">{t("clientSettings.denylistRule")}</span>{" "}
								{t("clientSettings.denylistRuleDesc")}
							</li>
							<li>
								<span className="text-foreground font-medium">{t("clientSettings.wildcards")}</span>{" "}
								<Trans
									t={t}
									i18nKey="clientSettings.wildcardsDesc"
									components={{
										1: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />,
										3: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />,
										5: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />,
										7: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />,
									}}
								/>
							</li>
						</ul>
					</section>
					<section className="space-y-2">
						<h4 className="font-medium">{t("clientSettings.important")}</h4>
						<ul className="text-muted-foreground list-inside list-disc space-y-2">
							<li>
								<Trans
									t={t}
									i18nKey="clientSettings.importantDesc1"
									components={{
										1: <span className="text-foreground font-medium" />,
										3: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />,
									}}
								/>
							</li>
							<li>
								<Trans
									t={t}
									i18nKey="clientSettings.importantDesc2"
									components={{
										1: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />,
										3: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />,
										5: <code className="bg-muted rounded px-1 py-0.5 font-mono text-xs" />,
									}}
								/>
							</li>
						</ul>
					</section>
				</div>
				<div className="mt-6 flex justify-end">
					<SheetClose asChild>
						<Button type="button" variant="default" size="sm">
							{t("clientSettings.help.close")}
						</Button>
					</SheetClose>
				</div>
			</SheetContent>
		</Sheet>
	);
}

interface SaveButtonProps {
	hasChanges: boolean;
	isLoading: boolean;
	isQueriesLoading: boolean;
	hasAccess: boolean;
	hasSecurityHeaderError: boolean;
	invalidSecurityHeaders: string[];
	onSave: () => void;
}

function SaveButton({
	hasChanges,
	isLoading,
	isQueriesLoading,
	hasAccess,
	hasSecurityHeaderError,
	invalidSecurityHeaders,
	onSave,
}: SaveButtonProps) {
	const { t } = useTranslation("config");
	const label = isLoading ? t("clientSettings.saving") : t("clientSettings.saveChanges");

	if (hasSecurityHeaderError) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<span>
						<Button disabled>{label}</Button>
					</span>
				</TooltipTrigger>
				<TooltipContent>
					{t("clientSettings.removeSecurityHeaders", { count: invalidSecurityHeaders.length })}: {invalidSecurityHeaders.join(", ")}
				</TooltipContent>
			</Tooltip>
		);
	}
	return (
		<Button onClick={onSave} disabled={!hasChanges || isLoading || isQueriesLoading || !hasAccess}>
			{label}
		</Button>
	);
}
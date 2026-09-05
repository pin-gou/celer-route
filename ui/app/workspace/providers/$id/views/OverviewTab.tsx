import { Button } from "@/components/ui/button";
import { ModelProvider } from "@/lib/types/config";
import { useTranslation } from "react-i18next";
import { ConfigSuggestions } from "./ConfigSuggestions";

export interface OverviewTabProps {
	provider: ModelProvider;
}

interface ReadOnlyCardProps {
	testId: string;
	title: string;
	manageLabel?: string;
	onManage?: () => void;
	children: React.ReactNode;
}

function ReadOnlyCard({ testId, title, manageLabel, onManage, children }: ReadOnlyCardProps) {
	return (
		<div data-testid={testId} className="rounded-lg border p-4">
			<div className="mb-3 flex items-center justify-between">
				<h3 className="text-sm font-medium">{title}</h3>
				{manageLabel && onManage && (
					<Button variant="outline" size="sm" onClick={onManage} className="text-xs" data-testid={`${testId}-manage`}>
						{manageLabel}
					</Button>
				)}
			</div>
			{children}
		</div>
	);
}

function CooldownPolicySummary({ provider }: { provider: ModelProvider }) {
	const { t } = useTranslation("providers");
	const policy = provider.cooldown_policy;
	const hasRateLimit = !!policy?.rate_limit && policy.rate_limit.enabled !== false;
	const hasQuota = !!policy?.quota && policy.quota.enabled !== false;
	const hasDisabledOnly = !!policy && !hasRateLimit && !hasQuota && (!!policy.rate_limit || !!policy.quota);
	if (!hasRateLimit && !hasQuota) {
		if (hasDisabledOnly) {
			return (
				<div className="text-muted-foreground space-y-1 text-xs">
					<p>{t("providers2.overview.cooldownPolicyAllDisabled")}</p>
					{policy && <p className="mt-1 text-xs italic">{t("providers2.overview.cooldownPolicyOverride")}</p>}
				</div>
			);
		}
		return <p className="text-muted-foreground text-xs">{t("providers2.overview.cooldownPolicyUsingDefault")}</p>;
	}
	return (
		<div className="text-muted-foreground space-y-1 text-xs">
			{hasRateLimit && (
				<div className="flex justify-between">
					<span>{t("providers2.overview.cooldownPolicyRateLimitTtl")}</span>
					<span className="font-mono">{policy!.rate_limit!.ttl_seconds}s</span>
				</div>
			)}
			{hasQuota && (
				<div className="flex justify-between">
					<span>{t("providers2.overview.cooldownPolicyQuotaTtl")}</span>
					<span className="font-mono">{policy!.quota!.ttl_seconds}s</span>
				</div>
			)}
			{policy && <p className="mt-2 text-xs italic">{t("providers2.overview.cooldownPolicyOverride")}</p>}
		</div>
	);
}

export function OverviewTab({ provider, onNavigateTab }: OverviewTabProps & { onNavigateTab?: (tabId: string) => void }) {
	const { t } = useTranslation("providers");
	const keysCount = provider.keys_count ?? 0;
	const modelsCount = provider.models_count ?? 0;
	const maxRetries = provider.network_config?.max_retries ?? 0;
	const retryInitial = provider.network_config?.retry_backoff_initial ?? 0;
	const retryMax = provider.network_config?.retry_backoff_max ?? 0;

	const goTo = (tabId: string) => () => onNavigateTab?.(tabId);

	return (
		<div data-testid="providers2-overview-tab" className="space-y-6">
			<ConfigSuggestions provider={provider} />

			<div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
				<ReadOnlyCard
					testId="providers2-overview-keys"
					title={t("providers2.tabs.keys")}
					manageLabel={t("providers2.overview.manage")}
					onManage={goTo("keys")}
				>
					<div className="text-muted-foreground space-y-1 text-xs">
						<div className="flex justify-between">
							<span>{t("providers2.overview.apiKeysCount")}</span>
							<span className="font-mono">{keysCount}</span>
						</div>
					</div>
				</ReadOnlyCard>

				<ReadOnlyCard
					testId="providers2-overview-models"
					title={t("providers2.tabs.models")}
					manageLabel={t("providers2.overview.manage")}
					onManage={goTo("models")}
				>
					<div className="text-muted-foreground space-y-1 text-xs">
						<div className="flex justify-between">
							<span>{t("providers2.overview.modelsCount")}</span>
							<span className="font-mono">{modelsCount}</span>
						</div>
					</div>
				</ReadOnlyCard>

				<ReadOnlyCard
					testId="providers2-overview-cooldown-policy"
					title={t("providers2.overview.cooldownPolicy")}
					manageLabel={t("providers2.overview.manage")}
					onManage={goTo("cooldown")}
				>
					<CooldownPolicySummary provider={provider} />
				</ReadOnlyCard>

				<ReadOnlyCard
					testId="providers2-overview-retry"
					title={t("providers2.overview.retryPolicy")}
					manageLabel={t("providers2.overview.manage")}
					onManage={goTo("network")}
				>
					<div className="text-muted-foreground space-y-1 text-xs">
						<div className="flex justify-between">
							<span>{t("providers2.overview.maxRetries")}</span>
							<span className="font-mono">{maxRetries}</span>
						</div>
						<div className="flex justify-between">
							<span>{t("providers2.overview.retryBackoff")}</span>
							<span className="font-mono">
								{retryInitial}ms / {retryMax}ms
							</span>
						</div>
					</div>
				</ReadOnlyCard>
			</div>
		</div>
	);
}
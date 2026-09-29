import { TestCommandTabs } from "@/components/testCommandPanel";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { writeClipboard } from "@/hooks/useCopyToClipboard";
import { useGetModelsQuery, useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { useGetVirtualKeysQuery } from "@/lib/store/apis/governanceApi";
import { useGetCoreConfigQuery } from "@/lib/store";
import { RenderProviderIcon } from "@/lib/constants/icons";
import { getProviderLabel } from "@/lib/constants/logs";
import { buildExamples, resolveEndpointUrl } from "@/lib/utils/testCommandSnippets";
import { Copy, KeyRound, Sparkles, Terminal } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

export interface EndpointPanelProps {
	endpointUrl?: string;
	/**
	 * `card` (default) wraps the panel in a Card with header — used by the
	 * Home page. `bare` renders just the selector + tabs without a card,
	 * so callers (e.g. the logs empty state) can drop it into any layout.
	 */
	variant?: "card" | "bare";
	/**
	 * Optional prefix used for `data-testid` attributes. Defaults to
	 * `home-endpoint-card` so existing Home-page E2E selectors keep working
	 * when `variant="card"`. When `variant="bare"`, defaults to
	 * `logs-empty-endpoint` — distinct from the card variant so the same
	 * component can be tested under two surfaces without collisions.
	 */
	testIdPrefix?: string;
}

/**
 * EndpointPanel — the "how to integrate with celer-route" panel.
 *
 * Solves the long-standing empty-state gap where the static code samples
 * hard-coded `gpt-4o-mini` / `claude-3-sonnet-20240229` and a `dummy-api-key`
 * placeholder. This component fetches the live provider / model / virtual-key
 * lists and feeds them into `buildExamples`, so the four tabs (curl / Python /
 * Node / Go) always show *the* model + API key the user actually has access
 * to. Used by:
 *   - Home page (`variant="card"`) — the "Your endpoint" card
 *   - Logs empty state (`variant="bare"`) — replaces the old hard-coded
 *     `emptyState.tsx` tabs panel
 */
export function EndpointPanel({ endpointUrl, variant = "card", testIdPrefix }: EndpointPanelProps) {
	const { t } = useTranslation("common");
	const { data: providers } = useGetProvidersQuery();
	const { data: bifrostConfig } = useGetCoreConfigQuery({});
	const url = endpointUrl ?? resolveEndpointUrl();

	const enforceAuth = !!bifrostConfig?.client_config?.enforce_auth_on_inference;
	const { data: vksResponse } = useGetVirtualKeysQuery(undefined, { skip: !enforceAuth });
	const vks = useMemo(() => vksResponse?.virtual_keys ?? [], [vksResponse]);

	const [selectedProvider, setSelectedProvider] = useState<string | null>(null);
	const [selectedModel, setSelectedModel] = useState("");
	const [selectedVkId, setSelectedVkId] = useState<string>("");

	const providerNames = useMemo(() => {
		const seen = new Set<string>();
		return (providers ?? [])
			.filter((p) => {
				if (seen.has(p.name)) return false;
				seen.add(p.name);
				return true;
			})
			.map((p) => p.name);
	}, [providers]);

	useEffect(() => {
		if (!selectedProvider && providerNames.length > 0) {
			setSelectedProvider(providerNames[0]);
		}
	}, [providerNames, selectedProvider]);

	useEffect(() => {
		if (!selectedVkId && vks.length > 0) setSelectedVkId(vks[0].id);
	}, [vks, selectedVkId]);

	const selectedVk = useMemo(() => vks.find((v) => v.id === selectedVkId), [vks, selectedVkId]);

	const { data: modelsData, isFetching: isFetchingModels } = useGetModelsQuery(
		{ provider: selectedProvider ?? "", limit: 200 },
		{ skip: !selectedProvider },
	);
	const models = useMemo(() => {
		const seen = new Set<string>();
		const list: string[] = [];
		for (const m of modelsData?.models ?? []) {
			if (!seen.has(m.name)) {
				seen.add(m.name);
				list.push(m.name);
			}
		}
		return list;
	}, [modelsData]);

	useEffect(() => {
		if (!selectedModel && models.length > 0) setSelectedModel(models[0]);
	}, [models, selectedModel]);

	const model = selectedModel || "gpt-4o-mini";
	const examples = useMemo(
		() => buildExamples(url, model, enforceAuth ? (selectedVk?.value ?? null) : null),
		[url, model, enforceAuth, selectedVk],
	);

	const tabs = useMemo(
		() =>
			[
				{ id: "curl", label: t("codeTabs.curl"), code: examples.curl },
				{ id: "python", label: t("codeTabs.python"), code: examples.python },
				{ id: "node", label: t("codeTabs.node"), code: examples.node },
				{ id: "go", label: t("codeTabs.go"), code: examples.go },
			].map((tabItem) => ({ ...tabItem, copySuccessMessage: t("integrationPanel.copied") })),
		[examples, t],
	);

	const handleCopy = async (text: string) => {
		if (await writeClipboard(text)) {
			toast.success(t("integrationPanel.copied"));
		} else {
			toast.error("Copy failed");
		}
	};

	// testId defaulting: preserve existing Home-page E2E selectors (`home-endpoint-*`)
	// for `variant="card"`, and give the logs empty state its own prefix so the two
	// surfaces can be tested independently.
	const tid = testIdPrefix ?? (variant === "card" ? "home-endpoint-card" : "logs-empty-endpoint");

	const body = (
		<>
			<div className="bg-muted/40 flex items-center gap-2 rounded-md border p-3 font-mono text-sm">
				<code className="flex-1 truncate" data-testid={`${tid}-url`}>
					{url}
				</code>
				<Button size="sm" variant="outline" onClick={() => void handleCopy(url)} data-testid={`${tid}-copy-url`}>
					<Copy className="mr-1 h-4 w-4" />
					{t("integrationPanel.copyLabel")}
				</Button>
			</div>

			<div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<div className="space-y-2">
					<Label htmlFor={`${tid}-provider`}>{t("integrationPanel.providerLabel")}</Label>
					<Select value={selectedProvider ?? ""} onValueChange={(v) => setSelectedProvider(v)}>
						<SelectTrigger id={`${tid}-provider`} className="w-full" data-testid={`${tid}-provider`}>
							<SelectValue placeholder={t("integrationPanel.providerPlaceholder")} />
						</SelectTrigger>
						<SelectContent>
							{providerNames.map((p) => (
								<SelectItem key={p} value={p} data-testid={`${tid}-provider-option-${p}`}>
									<span className="flex items-center gap-2">
										<RenderProviderIcon
											provider={p as Parameters<typeof RenderProviderIcon>[0]["provider"]}
											size={16}
											className="mt-0 shrink-0"
										/>
										<span className="truncate">{getProviderLabel(p)}</span>
									</span>
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>
				<div className="space-y-2">
					<Label htmlFor={`${tid}-model`}>{t("integrationPanel.modelLabel")}</Label>
					<Select value={selectedModel} onValueChange={(v) => setSelectedModel(v)} disabled={isFetchingModels}>
						<SelectTrigger id={`${tid}-model`} className="w-full" data-testid={`${tid}-model`}>
							<SelectValue
								placeholder={
									isFetchingModels ? "…" : models.length === 0 ? t("integrationPanel.modelEmpty") : t("integrationPanel.modelPlaceholder")
								}
							/>
						</SelectTrigger>
						<SelectContent>
							{models.length === 0 ? (
								<SelectItem value="__none__" disabled>
									—
								</SelectItem>
							) : (
								models.map((m) => (
									<SelectItem key={m} value={m} data-testid={`${tid}-model-option-${m}`}>
										{m}
									</SelectItem>
								))
							)}
						</SelectContent>
					</Select>
				</div>
			</div>

			{enforceAuth && (
				<div className="space-y-2">
					<Label htmlFor={`${tid}-vk`}>{t("integrationPanel.vkLabel")}</Label>
					<Select value={selectedVkId} onValueChange={(v) => setSelectedVkId(v)} disabled={vks.length === 0}>
						<SelectTrigger id={`${tid}-vk`} className="w-full" data-testid={`${tid}-vk`}>
							<SelectValue placeholder={vks.length === 0 ? t("integrationPanel.vkEmpty") : t("integrationPanel.vkPlaceholder")} />
						</SelectTrigger>
						<SelectContent>
							{vks.length === 0 ? (
								<SelectItem value="__none__" disabled>
									—
								</SelectItem>
							) : (
								vks.map((vk) => (
									<SelectItem key={vk.id} value={vk.id} data-testid={`${tid}-vk-option-${vk.name}`}>
										<span className="flex items-center gap-2">
											<KeyRound className="text-muted-foreground h-4 w-4 shrink-0" />
											<span className="truncate">{vk.name}</span>
										</span>
									</SelectItem>
								))
							)}
						</SelectContent>
					</Select>
				</div>
			)}

			<TestCommandTabs tabs={tabs} testIdPrefix={`${tid}-examples`} />

			<Link
				to="/workspace/agent-setup"
				className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 text-xs underline-offset-2 hover:underline"
				data-testid={`${tid}-agent-setup-link`}
			>
				<Terminal className="h-3.5 w-3.5" />
				{t("integrationPanel.agentSetupLink")}
			</Link>
		</>
	);

	if (variant === "bare") {
		return (
			<div className="dark:bg-card flex w-full flex-col items-center justify-center space-y-6 bg-white" data-testid={tid}>
				<div className="w-full max-w-3xl space-y-6 p-4">
					<div className="flex flex-row items-center gap-2">
						<div>
							<h3 className="text-lg font-semibold">{t("integrationPanel.bareTitle")}</h3>
							<p className="text-muted-foreground text-sm">{t("integrationPanel.bareSubtitle")}</p>
						</div>
					</div>
					<div className="space-y-4">{body}</div>
				</div>
			</div>
		);
	}

	return (
		<Card className="bg-card gap-0 border py-0" data-testid={tid}>
			<CardHeader className="flex flex-row items-start justify-between gap-3 border-b px-6 py-4">
				<div className="space-y-1">
					<CardTitle className="text-base font-semibold">{t("integrationPanel.title")}</CardTitle>
					<p className="text-muted-foreground text-xs">{t("integrationPanel.subtitle")}</p>
				</div>
				<Button asChild size="sm" className="shrink-0" data-testid={`${tid}-connect-client-cta`}>
					<Link to="/workspace/agent-setup">
						<Sparkles className="mr-1 h-4 w-4" />
						{t("integrationPanel.connectClientCta")}
					</Link>
				</Button>
			</CardHeader>
			<CardContent className="space-y-4 px-6 py-4">{body}</CardContent>
		</Card>
	);
}

export default EndpointPanel;
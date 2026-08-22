import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useGetModelsQuery, useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { useGetVirtualKeysQuery } from "@/lib/store/apis/governanceApi";
import { useGetCoreConfigQuery } from "@/lib/store";
import { RenderProviderIcon } from "@/lib/constants/icons";
import { getProviderLabel } from "@/lib/constants/logs";
import { Copy, KeyRound } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

interface Props {
	selectedProvider: string | null;
	onProviderChange: (provider: string | null) => void;
	selectedModel: string;
	onModelChange: (model: string) => void;
}

export default function DoneStep({ selectedProvider, onProviderChange, selectedModel, onModelChange }: Props) {
	const { t } = useTranslation("onboarding");
	const { data: providers } = useGetProvidersQuery();
	const { data: bifrostConfig } = useGetCoreConfigQuery({});
	const url = defaultEndpoint();
	const [tab, setTab] = useState("curl");

	const enforceAuth = !!bifrostConfig?.client_config?.enforce_auth_on_inference;
	const { data: vksResponse } = useGetVirtualKeysQuery(undefined, { skip: !enforceAuth });
	const vks = useMemo(() => vksResponse?.virtual_keys ?? [], [vksResponse]);

	const [selectedVkId, setSelectedVkId] = useState<string>("");

	useEffect(() => {
		if (!selectedVkId && vks.length > 0) setSelectedVkId(vks[0].id);
	}, [vks, selectedVkId]);

	const selectedVk = useMemo(() => vks.find((v) => v.id === selectedVkId), [vks, selectedVkId]);

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
			onProviderChange(providerNames[0]);
		}
	}, [providerNames, selectedProvider, onProviderChange]);

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
		if (!selectedModel && models.length > 0) onModelChange(models[0]);
	}, [models, selectedModel, onModelChange]);

	const model = selectedModel || "gpt-4o-mini";
	const examples = useMemo(
		() => buildExamples(url, model, enforceAuth ? (selectedVk?.value ?? null) : null),
		[url, model, enforceAuth, selectedVk],
	);

	const handleCopy = async (text: string) => {
		try {
			await navigator.clipboard.writeText(text);
			toast.success(t("copiedEndpoint"));
		} catch {
			toast.error("Copy failed");
		}
	};

	return (
		<div className="space-y-5">
			<div className="space-y-1 text-center sm:text-left">
				<h2 className="text-xl font-semibold tracking-tight">{t("step.done")}</h2>
				<p className="text-muted-foreground text-sm">{t("yourEndpoint")}</p>
			</div>

			<div className="bg-muted/40 flex items-center gap-2 rounded-md border p-3 font-mono text-sm">
				<code className="flex-1 truncate" data-testid="onboarding-endpoint">
					{url}
				</code>
				<Button size="sm" variant="outline" onClick={() => void handleCopy(url)} data-testid="onboarding-endpoint-copy">
					<Copy className="mr-1 h-4 w-4" />
					{t("copyEndpoint")}
				</Button>
			</div>

			<div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<div className="space-y-2">
					<Label htmlFor="onb-done-provider">{t("testProviderLabel")}</Label>
					<Select value={selectedProvider ?? ""} onValueChange={(v) => onProviderChange(v)}>
						<SelectTrigger id="onb-done-provider" className="w-full">
							<SelectValue placeholder={t("testProviderPlaceholder")} />
						</SelectTrigger>
						<SelectContent>
							{providerNames.map((p) => (
								<SelectItem key={p} value={p} data-testid={`onboarding-done-provider-option-${p}`}>
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
					<Label htmlFor="onb-done-model">{t("testModelLabel")}</Label>
					<Select value={selectedModel} onValueChange={(v) => onModelChange(v)} disabled={isFetchingModels}>
						<SelectTrigger id="onb-done-model" className="w-full">
							<SelectValue
								placeholder={isFetchingModels ? `${t("sending")}…` : models.length === 0 ? t("testModelEmpty") : t("testModelPlaceholder")}
							/>
						</SelectTrigger>
						<SelectContent>
							{models.length === 0 ? (
								<SelectItem value="__none__" disabled>
									—
								</SelectItem>
							) : (
								models.map((m) => (
									<SelectItem key={m} value={m} data-testid={`onboarding-done-model-option-${m}`}>
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
					<Label htmlFor="onb-done-vk">{t("testVkLabel")}</Label>
					<Select value={selectedVkId} onValueChange={(v) => setSelectedVkId(v)} disabled={vks.length === 0}>
						<SelectTrigger id="onb-done-vk" className="w-full">
							<SelectValue placeholder={vks.length === 0 ? t("testVkEmpty") : t("testVkPlaceholder")} />
						</SelectTrigger>
						<SelectContent>
							{vks.length === 0 ? (
								<SelectItem value="__none__" disabled>
									—
								</SelectItem>
							) : (
								vks.map((vk) => (
									<SelectItem key={vk.id} value={vk.id} data-testid={`onboarding-done-vk-option-${vk.name}`}>
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

			<Tabs value={tab} onValueChange={setTab}>
				<TabsList>
					<TabsTrigger value="curl">{t("codeTabs.curl")}</TabsTrigger>
					<TabsTrigger value="python">{t("codeTabs.python")}</TabsTrigger>
					<TabsTrigger value="node">{t("codeTabs.node")}</TabsTrigger>
					<TabsTrigger value="go">{t("codeTabs.go")}</TabsTrigger>
				</TabsList>
				<TabsContent value="curl">
					<CodeBlock code={examples.curl} onCopy={() => void handleCopy(examples.curl)} />
				</TabsContent>
				<TabsContent value="python">
					<CodeBlock code={examples.python} onCopy={() => void handleCopy(examples.python)} />
				</TabsContent>
				<TabsContent value="node">
					<CodeBlock code={examples.node} onCopy={() => void handleCopy(examples.node)} />
				</TabsContent>
				<TabsContent value="go">
					<CodeBlock code={examples.go} onCopy={() => void handleCopy(examples.go)} />
				</TabsContent>
			</Tabs>
		</div>
	);
}

function CodeBlock({ code, onCopy }: { code: string; onCopy: () => void }) {
	return (
		<div className="relative">
			<pre className="overflow-x-auto rounded-md border bg-zinc-950 px-4 py-3 text-xs text-zinc-100">
				<code>{code}</code>
			</pre>
			<button
				type="button"
				onClick={onCopy}
				className="absolute top-2 right-2 rounded p-1 text-zinc-300 hover:bg-zinc-800 hover:text-white"
				aria-label="Copy"
			>
				<Copy className="h-4 w-4" />
			</button>
		</div>
	);
}

function defaultEndpoint(): string {
	if (typeof window === "undefined") return "http://localhost:8080/v1";
	return `${window.location.origin}/v1`;
}

function buildExamples(baseUrl: string, model: string, vkValue: string | null) {
	const url = baseUrl.replace(/\/$/, "");

	const curlLines = [`curl ${url}/chat/completions \\`, `  -H "Content-Type: application/json" \\`];
	if (vkValue) {
		curlLines.push(`  -H "Authorization: Bearer ${vkValue}" \\`);
	}
	curlLines.push(`  -d '{`, `    "model": "${model}",`, `    "messages": [{"role": "user", "content": "Hello!"}]`, `  }'`);
	const curl = curlLines.join("\n");

	const apiKeyLine = vkValue ? `    api_key="${vkValue}",` : null;
	const nodeApiKey = vkValue ? `  apiKey: "${vkValue}",` : null;
	const goApiKey = vkValue ? `\t\toption.WithAPIKey("${vkValue}"),` : null;

	const python = `from openai import OpenAI

client = OpenAI(
    base_url="${url}",${apiKeyLine ? `\n${apiKeyLine}` : ""}
)

resp = client.chat.completions.create(
    model="${model}",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(resp.choices[0].message.content)`;

	const node = `import OpenAI from "openai";

const client = new OpenAI({${nodeApiKey ? `\n${nodeApiKey}` : ""}
  baseURL: "${url}",
});

const resp = await client.chat.completions.create({
  model: "${model}",
  messages: [{ role: "user", content: "Hello!" }],
});
console.log(resp.choices[0].message.content);`;

	const go = `package main

import (
	"context"
	"fmt"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func main() {
	client := openai.NewClient(
		option.WithBaseURL("${url}"),${goApiKey ? `\n${goApiKey}` : ""}
	)
	resp, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model: openai.F("${model}"),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Hello!"),
		}),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.Choices[0].Message.Content)
}`;

	return { curl, python, node, go };
}
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useGetModelsQuery, useGetProvidersQuery, useTestProviderModelMutation } from "@/lib/store/apis/providersApi";
import { RenderProviderIcon } from "@/lib/constants/icons";
import { getProviderLabel } from "@/lib/constants/logs";
import { CheckCircle2, Loader2, RefreshCw, Send, XCircle } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

type Status = "idle" | "sending" | "success" | "error";

interface Props {
	selectedProvider: string | null;
	onProviderChange: (provider: string | null) => void;
	selectedModel: string;
	onModelChange: (model: string) => void;
}

export default function TestStep({ selectedProvider, onProviderChange, selectedModel, onModelChange }: Props) {
	const { t } = useTranslation("onboarding");
	const { data: providers } = useGetProvidersQuery();
	const [testProviderModel] = useTestProviderModelMutation();

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

	const [prompt, setPrompt] = useState("Say hi in one sentence.");
	const [status, setStatus] = useState<Status>("idle");
	const [message, setMessage] = useState<string>("");

	useEffect(() => {
		if (!selectedProvider && providerNames.length > 0) {
			onProviderChange(providerNames[0]);
		}
	}, [providerNames, selectedProvider, onProviderChange]);

	// Clear the model whenever the provider changes — model belongs to a provider.
	useEffect(() => {
		onModelChange("");
		setStatus("idle");
		setMessage("");
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [selectedProvider]);

	// Fetch models for the selected provider.
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

	const canSend = !!selectedProvider && !!selectedModel.trim() && !!prompt.trim() && status !== "sending";

	const handleSend = async () => {
		if (!canSend || !selectedProvider || !selectedModel) return;
		setStatus("sending");
		setMessage("");
		try {
			const res = await testProviderModel({
				provider: selectedProvider,
				model: selectedModel,
			}).unwrap();
			if (res.success) {
				setStatus("success");
				setMessage(t("testSuccess", { ms: String(res.latency_ms ?? 0) }));
			} else {
				setStatus("error");
				setMessage(t("testFailed", { err: res.error ?? "Unknown error" }));
			}
		} catch (err) {
			setStatus("error");
			setMessage(t("testFailed", { err: err instanceof Error ? err.message : String(err) }));
		}
	};

	return (
		<div className="space-y-5">
			<div className="space-y-1 text-center sm:text-left">
				<h2 className="text-xl font-semibold tracking-tight">{t("step.test")}</h2>
				<p className="text-muted-foreground text-sm">{t("testDesc")}</p>
			</div>

			<div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<div className="space-y-2">
					<Label htmlFor="onb-test-provider">{t("testProviderLabel")}</Label>
					<Select value={selectedProvider ?? ""} onValueChange={(v) => onProviderChange(v)}>
						<SelectTrigger id="onb-test-provider" className="w-full">
							<SelectValue placeholder={t("testProviderPlaceholder")} />
						</SelectTrigger>
						<SelectContent>
							{providerNames.map((p) => (
								<SelectItem key={p} value={p} data-testid={`onboarding-test-provider-option-${p}`}>
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
					<Label htmlFor="onb-test-model">{t("testModelLabel")}</Label>
					<Select value={selectedModel} onValueChange={(v) => onModelChange(v)} disabled={isFetchingModels}>
						<SelectTrigger id="onb-test-model" className="w-full">
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
									<SelectItem key={m} value={m} data-testid={`onboarding-test-model-option-${m}`}>
										{m}
									</SelectItem>
								))
							)}
						</SelectContent>
					</Select>
				</div>
			</div>

			<div className="space-y-2">
				<Label htmlFor="onb-test-prompt">{t("testPromptLabel")}</Label>
				<Textarea
					id="onb-test-prompt"
					rows={3}
					placeholder={t("testPromptPlaceholder")}
					value={prompt}
					onChange={(e) => setPrompt(e.target.value)}
				/>
			</div>

			<div className="flex items-center justify-between">
				<Button size="sm" disabled={!canSend} onClick={() => void handleSend()} data-testid="onboarding-test-send">
					{status === "sending" ? (
						<>
							<Loader2 className="mr-1 h-4 w-4 animate-spin" />
							{t("sending")}
						</>
					) : status === "error" ? (
						<>
							<RefreshCw className="mr-1 h-4 w-4" />
							{t("testRetry")}
						</>
					) : (
						<>
							<Send className="mr-1 h-4 w-4" />
							{t("sendTest")}
						</>
					)}
				</Button>
				<div className="flex items-center gap-2 text-xs">
					{status === "success" && (
						<span className="flex items-center gap-1 text-emerald-600">
							<CheckCircle2 className="h-4 w-4" />
							{message}
						</span>
					)}
					{status === "error" && (
						<span className="text-destructive flex items-center gap-1">
							<XCircle className="h-4 w-4" />
							{message}
						</span>
					)}
				</div>
			</div>
		</div>
	);
}
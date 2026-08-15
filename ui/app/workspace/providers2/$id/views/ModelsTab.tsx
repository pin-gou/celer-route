import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getErrorMessage } from "@/lib/store";
import { useGetModelsQuery, useRefreshProviderModelsMutation, useTestProviderModelMutation } from "@/lib/store/apis/providersApi";
import { ModelProvider } from "@/lib/types/config";
import { CheckCircle2, RefreshCwIcon, XCircle } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

interface ModelsTabProps {
	provider: ModelProvider;
}

interface TestStatus {
	state: "idle" | "testing" | "ok" | "error";
	latencyMs?: number;
	message?: string;
}

export function ModelsTab({ provider }: ModelsTabProps) {
	const [showAll, setShowAll] = useState(false);
	const { data: modelsData, isLoading, refetch } = useGetModelsQuery({ provider: provider.name, limit: showAll ? 500 : 10 });
	const [refreshModels, { isLoading: isRefreshing }] = useRefreshProviderModelsMutation();
	const [testModel] = useTestProviderModelMutation();

	const [testingId, setTestingId] = useState<string | null>(null);
	const [testStatus, setTestStatus] = useState<Record<string, TestStatus>>({});
	const [testingAll, setTestingAll] = useState(false);
	const [testProgress, setTestProgress] = useState<{ done: number; total: number } | null>(null);

	const models = modelsData?.models ?? [];
	const total = modelsData?.total ?? 0;

	const handleSync = async () => {
		try {
			await refreshModels(provider.name).unwrap();
			toast.success(`Model refresh started for ${provider.name}`);
			refetch();
		} catch (err: any) {
			if (err?.status === 409) {
				toast.info(`Model refresh already running for ${provider.name}`);
			} else {
				toast.error(`Failed to refresh models for ${provider.name}`);
			}
		}
	};

	const handleTestModel = async (name: string) => {
		setTestingId(name);
		try {
			const result = await testModel({ provider: provider.name, model: name }).unwrap();
			if (result.success) {
				setTestStatus((prev) => ({
					...prev,
					[name]: {
						state: "ok",
						latencyMs: result.latency_ms,
						message: `OK in ${result.latency_ms}ms`,
					},
				}));
				toast.success(`Model ${name} replied in ${result.latency_ms}ms`);
			} else {
				setTestStatus((prev) => ({
					...prev,
					[name]: { state: "error", message: result.error || "Test failed" },
				}));
				toast.error(`Model ${name} test failed`, { description: result.error });
			}
		} catch (err) {
			setTestStatus((prev) => ({
				...prev,
				[name]: { state: "error", message: getErrorMessage(err) },
			}));
			toast.error(`Model ${name} test failed`, { description: getErrorMessage(err) });
		} finally {
			setTestingId(null);
		}
	};

	const handleTestAll = async () => {
		const targets = models.filter((m) => !m.is_deprecated);
		if (targets.length === 0) return;
		setTestingAll(true);
		setTestProgress({ done: 0, total: targets.length });
		let passed = 0;
		let failed = 0;
		for (let i = 0; i < targets.length; i++) {
			const model = targets[i];
			setTestingId(model.name);
			try {
				const result = await testModel({ provider: provider.name, model: model.name }).unwrap();
				if (result.success) {
					passed++;
					setTestStatus((prev) => ({
						...prev,
						[model.name]: { state: "ok", latencyMs: result.latency_ms, message: `OK in ${result.latency_ms}ms` },
					}));
				} else {
					failed++;
					setTestStatus((prev) => ({
						...prev,
						[model.name]: { state: "error", message: result.error || "Test failed" },
					}));
				}
			} catch (err) {
				failed++;
				setTestStatus((prev) => ({
					...prev,
					[model.name]: { state: "error", message: getErrorMessage(err) },
				}));
			}
			setTestProgress({ done: i + 1, total: targets.length });
		}
		setTestingId(null);
		setTestProgress(null);
		setTestingAll(false);
		toast.success(`Tested ${targets.length} models: ${passed} passed, ${failed} failed`);
	};

	return (
		<div data-testid="providers2-models-tab" className="rounded-lg border p-6">
			<div className="mb-4 flex items-center justify-between">
				<div>
					<h3 className="text-sm font-medium">Models</h3>
					<p className="text-muted-foreground mt-1 text-xs">
						{total} model{total !== 1 ? "s" : ""} associated with {provider.name}
					</p>
				</div>
				<div className="flex items-center gap-2">
					{models.some((m) => !m.is_deprecated) && (
						<Button
							variant="outline"
							size="sm"
							data-testid="providers2-models-test-all-btn"
							onClick={handleTestAll}
							disabled={testingAll}
							className="gap-1 text-xs"
						>
							<RefreshCwIcon className={`h-3 w-3 ${testingAll ? "animate-spin" : ""}`} />
							{testingAll ? "Testing..." : "Test All"}
						</Button>
					)}
					<Button
						variant="outline"
						size="sm"
						data-testid="providers2-models-sync-btn"
						onClick={handleSync}
						disabled={isRefreshing}
						className="gap-1 text-xs"
					>
						<RefreshCwIcon className={`h-3 w-3 ${isRefreshing ? "animate-spin" : ""}`} />
						{isRefreshing ? "Syncing..." : "Sync"}
					</Button>
				</div>
			</div>

			{testProgress && (
				<div className="mb-3 h-1.5 w-full overflow-hidden rounded-full bg-muted">
					<div
						className="h-full bg-primary transition-all"
						style={{ width: `${(testProgress.done / testProgress.total) * 100}%` }}
					/>
				</div>
			)}

			{isLoading ? (
				<div className="text-muted-foreground flex h-20 items-center justify-center text-xs">
					Loading models...
				</div>
			) : models.length === 0 ? (
				<div className="text-muted-foreground flex h-20 items-center justify-center text-xs">
					No models found for {provider.name}. Click Sync to discover models.
				</div>
			) : (
				<div className="overflow-x-auto">
					<table className="w-full text-left text-xs">
						<thead>
							<tr className="text-muted-foreground border-b">
								<th className="pb-2 pr-4 font-medium">Model Name</th>
								<th className="pb-2 pr-4 font-medium">Provider</th>
								<th className="pb-2 pr-4 font-medium">Status</th>
								<th className="pb-2 font-medium">Test</th>
							</tr>
						</thead>
						<tbody>
							{models.map((model) => {
								const status = testStatus[model.name];
								const isTesting = testingId === model.name;
								return (
									<tr key={`${model.provider}-${model.name}`} className="border-b last:border-0" data-testid={`providers2-models-row-${model.name}`}>
										<td className="py-2 pr-4 font-mono text-xs">{model.name}</td>
										<td className="py-2 pr-4 text-xs">{model.provider}</td>
										<td className="py-2 pr-4 text-xs">
											{model.is_deprecated ? (
												<span className="text-red-500">Deprecated</span>
											) : (
												<span className="text-green-600">Active</span>
											)}
											{status?.state === "ok" && (
												<span className="text-green-600 ml-2 whitespace-nowrap">
													<CheckCircle2 className="mr-0.5 inline h-3 w-3" />
													{status.latencyMs}ms
												</span>
											)}
											{status?.state === "error" && (
												<Tooltip>
													<TooltipTrigger asChild>
														<span className="text-red-500 ml-2 cursor-help whitespace-nowrap underline decoration-dotted">
															<XCircle className="mr-0.5 inline h-3 w-3" />
															Failed
														</span>
													</TooltipTrigger>
													<TooltipContent className="max-w-xs break-words">{status.message}</TooltipContent>
												</Tooltip>
											)}
										</td>
										<td className="py-2 text-xs">
											<Button
												variant="outline"
												size="sm"
												className="gap-1 text-xs"
												disabled={isTesting || model.is_deprecated || testingAll}
												onClick={() => handleTestModel(model.name)}
											>
												<RefreshCwIcon className={`h-3 w-3 ${isTesting ? "animate-spin" : ""}`} />
												{isTesting ? "Testing..." : "Test"}
											</Button>
										</td>
									</tr>
								);
							})}
						</tbody>
					</table>
				</div>
			)}

			{total > 10 && (
				<button
					data-testid="providers2-models-show-all"
					className="text-primary mt-3 text-xs underline"
					onClick={() => setShowAll(!showAll)}
				>
					{showAll ? "Show less" : `Show all ${total} models`}
				</button>
			)}
		</div>
	);
}
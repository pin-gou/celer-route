import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
	CheckCircle2,
	Copy,
	PencilIcon,
	PlusIcon,
	RefreshCwIcon,
	SearchIcon,
	XCircle,
} from "lucide-react";
import {
	getErrorMessage,
	ModelDetails,
	useGetModelDetailsQuery,
	useRefreshProviderModelsMutation,
	useTestProviderModelsMutation,
} from "@/lib/store";
import { ModelProvider } from "@/lib/types/config";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useDebouncedValue } from "@/hooks/useDebounce";
import { toast } from "sonner";
import AttributeSheet from "@/app/workspace/model-catalog/views/attributeSheet";
import { AddCustomModelSheet } from "./AddCustomModelSheet";

interface ModelsTabProps {
	provider: ModelProvider;
}

interface TestStatus {
	state: "idle" | "testing" | "ok" | "error";
	latencyMs?: number;
	message?: string;
}

type StatusFilter = "all" | "active" | "deprecated";

const DISPLAY_LIMIT = 500;

export function ModelsTab({ provider }: ModelsTabProps) {
	const hasUpdateAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Update);
	const hasCreateAccess = useRbac(RbacResource.ModelProvider, RbacOperation.Create);
	const [query, setQuery] = useState("");
	const debouncedQuery = useDebouncedValue(query, 300);
	const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");

	// Server-side substring search via the query param. When the user types
	// nothing, the server returns up to DISPLAY_LIMIT models for this provider.
	// Status filtering is client-side since the endpoint has no deprecated flag.
	const { data: modelsData, isLoading, refetch } = useGetModelDetailsQuery({
		provider: provider.name,
		query: debouncedQuery.trim() || undefined,
		limit: DISPLAY_LIMIT,
		unfiltered: true,
	});

	const [refreshModels, { isLoading: isRefreshing }] = useRefreshProviderModelsMutation();
	const [testModels] = useTestProviderModelsMutation();

	const [editingModel, setEditingModel] = useState<ModelDetails | null>(null);
	const [showAddModelSheet, setShowAddModelSheet] = useState(false);

	const models = modelsData?.models ?? [];
	const total = modelsData?.total ?? 0;
	const [testStatus, setTestStatus] = useState<Record<string, TestStatus>>({});
	const [testingAll, setTestingAll] = useState(false);
	const [testProgress, setTestProgress] = useState<{ done: number; total: number } | null>(null);

	const filteredModels = statusFilter === "all" ? models : models.filter((m) => (statusFilter === "deprecated" ? !!m.is_deprecated : !m.is_deprecated));

	const syncStatus = (modelNames: string[], results: { model: string; success: boolean; latency_ms?: number; error?: string }[]) => {
		setTestStatus((prev) => {
			const next = { ...prev };
			for (const model of modelNames) {
				next[model] = { state: "idle" };
			}
			for (const r of results) {
				next[r.model] = {
					state: r.success ? "ok" : "error",
					latencyMs: r.latency_ms,
					message: r.success ? `OK in ${r.latency_ms}ms` : r.error || "Test failed",
				};
			}
			return next;
		});
	};

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
		setTestStatus((prev) => ({ ...prev, [name]: { state: "testing" } }));
		try {
			const results = await testModels({ provider: provider.name, models: [name] }).unwrap();
			syncStatus([name], results.results);
		} catch (err) {
			setTestStatus((prev) => ({
				...prev,
				[name]: { state: "error", message: getErrorMessage(err) },
			}));
			toast.error(`Failed to test ${name}`, { description: getErrorMessage(err) });
		}
	};

	const handleTestAll = async () => {
		const targets = filteredModels.filter((m) => !m.is_deprecated);
		if (targets.length === 0) return;
		setTestingAll(true);
		setTestProgress({ done: 0, total: targets.length });
		try {
			const results = await testModels({
				provider: provider.name,
				models: targets.map((m) => m.name),
			}).unwrap();
			syncStatus(targets.map((m) => m.name), results.results);
			const passed = results.results.filter((r) => r.success).length;
			const failed = results.results.length - passed;
			toast.success(`Tested ${results.results.length} models: ${passed} passed, ${failed} failed`);
		} catch (err) {
			toast.error("Failed to test models", { description: getErrorMessage(err) });
		} finally {
			setTestProgress(null);
			setTestingAll(false);
		}
	};

	const statusOptions: { value: StatusFilter; label: string }[] = [
		{ value: "all", label: "All" },
		{ value: "active", label: "Active" },
		{ value: "deprecated", label: "Deprecated" },
	];

	return (
		<div data-testid="providers2-models-tab" className="rounded-lg border">
			{/* Toolbar */}
			<div className="flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
				<div className="flex flex-wrap items-center gap-2">
					{/* Search */}
					<div className="relative">
						<SearchIcon className="text-muted-foreground absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2" />
						<input
							data-testid="providers2-models-search"
							type="text"
							value={query}
							onChange={(e) => setQuery(e.target.value)}
							placeholder="Search models..."
							className="border-input w-52 rounded-md border py-1.5 pl-7 pr-3 text-xs outline-none focus:ring-1 focus:ring-blue-500"
						/>
					</div>
					{/* Status filter pills */}
					<div className="flex items-center gap-1">
						{statusOptions.map((opt) => (
							<button
								key={opt.value}
								onClick={() => setStatusFilter(opt.value)}
								className={`rounded-full px-2.5 py-1 text-xs font-medium transition-colors ${
									statusFilter === opt.value
										? "bg-primary text-primary-foreground"
										: "bg-muted text-muted-foreground hover:bg-muted/80"
								}`}
							>
								{opt.label}
							</button>
						))}
					</div>
				</div>
				<div className="flex items-center gap-2">
					{filteredModels.some((m) => !m.is_deprecated) && (
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
					{hasCreateAccess && (
						<Button
							size="sm"
							data-testid="providers2-models-add-btn"
							onClick={() => setShowAddModelSheet(true)}
							className="gap-1 text-xs"
						>
							<PlusIcon className="h-3 w-3" />
							Add model
						</Button>
					)}
				</div>
			</div>

			{/* Test progress */}
			{testProgress && (
				<div className="border-b px-4 py-2">
					<div className="mb-1 flex items-center justify-between text-xs text-muted-foreground">
						<span>Testing models...</span>
						<span>
							{testProgress.done} / {testProgress.total}
						</span>
					</div>
					<div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
						<div
							className="h-full bg-primary transition-all"
							style={{ width: `${(testProgress.done / testProgress.total) * 100}%` }}
						/>
					</div>
				</div>
			)}

			{isLoading ? (
				<div className="text-muted-foreground flex h-20 items-center justify-center text-xs">
					Loading models...
				</div>
			) : filteredModels.length === 0 ? (
				<div className="text-muted-foreground flex h-20 items-center justify-center text-xs">
					{query
						? `No models matching "${query}".`
						: `No models found for ${provider.name}. Click Sync or Add model to add one.`}
				</div>
			) : (
				<div className="overflow-x-auto">
					<table className="w-full text-left text-xs">
						<thead>
							<tr className="text-muted-foreground border-b">
								<th className="px-4 py-2 font-medium">Model Name</th>
								<th className="w-28 px-2 py-2 font-medium">Status</th>
								<th className="w-20 px-2 py-2 font-medium">Test</th>
								<th className="w-20 px-2 py-2 font-medium"></th>
							</tr>
						</thead>
						<tbody>
							{filteredModels.map((model) => {
								const status = testStatus[model.name];
								const isTesting = status?.state === "testing";
								return (
									<tr key={`${model.provider}-${model.name}`} className="border-b last:border-0 hover:bg-muted/30" data-testid={`providers2-models-row-${model.name}`}>
										<td className="max-w-[300px] truncate px-4 py-2 font-mono text-xs" title={model.name}>
											<span className="inline-flex items-center gap-1">
												<span className="truncate">{model.name}</span>
												<Tooltip>
													<TooltipTrigger asChild>
														<button
															onClick={() => navigator.clipboard?.writeText(`${model.provider}/${model.name}`)}
															className="text-muted-foreground hover:text-foreground rounded p-0.5 transition-colors"
														>
															<Copy className="h-3 w-3" />
														</button>
													</TooltipTrigger>
													<TooltipContent>Copy provider-qualified model ID</TooltipContent>
												</Tooltip>
											</span>
										</td>
										<td className="px-2 py-2">
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
										<td className="px-2 py-2">
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
										<td className="px-2 py-2">
											<Tooltip>
												<TooltipTrigger asChild>
													<button
														onClick={() => setEditingModel(model)}
														className="text-muted-foreground hover:text-foreground rounded p-1 transition-colors"
														disabled={!hasUpdateAccess}
														data-testid={`providers2-models-edit-${model.name}`}
													>
														<PencilIcon className="h-3.5 w-3.5" />
													</button>
												</TooltipTrigger>
												<TooltipContent>Edit model attributes</TooltipContent>
											</Tooltip>
										</td>
									</tr>
								);
							})}
						</tbody>
					</table>
				</div>
			)}

			{/* Info footer */}
			<div className="text-muted-foreground flex items-center justify-between border-t px-4 py-2 text-xs">
				<span>
					{filteredModels.length} of {total} model{total !== 1 ? "s" : ""}
					{query.trim() ? ` matching "${query.trim()}"` : ""}
				</span>
			</div>

			{editingModel && (
				<AttributeSheet
					model={{
						...editingModel,
						additional_attributes: editingModel.additional_attributes ?? {},
					}}
					onClose={() => {
						setEditingModel(null);
						refetch();
					}}
				/>
			)}
			{showAddModelSheet && (
				<AddCustomModelSheet
					provider={provider}
					onClose={() => {
						setShowAddModelSheet(false);
						refetch();
					}}
				/>
			)}
		</div>
	);
}
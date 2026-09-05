import { useTranslation } from "react-i18next";
import { Link } from "@tanstack/react-router";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useGetRtkRawOutputQuery } from "@/lib/store/apis/rtkAdminApi";

interface ScannedIndexEntry {
	index: number;
}

// CompressedItem is the post-compression tool message body for one message,
// indexed by the same `index` value the RTK pipeline recorded on the
// pre-compression side (input_history index for chat tool messages,
// responses_input_history index for function_call_output items). The log
// detail view builds this map once from the log entry's request body and
// passes it in; the diff view is otherwise read-only.
export interface CompressedItem {
	index: number;
	content: string;
}

interface Props {
	metadata: Record<string, unknown> | undefined;
	compressedItems?: CompressedItem[];
}

// RTKCompressionDiffView renders the diff between the pre-compression raw tool
// output and the post-compression message body. The pre-compression text is
// recovered on demand via `GET /api/context/rtk/raw-output/{id}?raw=1` using
// the `rtk_raw_output_id` metadata left by the RTK plugin's PostLLMHook. A
// `rtk_pipeline_scanned` metadata entry marks messages the pipeline evaluated
// but did not compress — the diff view surfaces those as "participated but
// not compressed" so operators can see the pipeline actually ran.
//
// The component is intentionally read-only: it never invokes side effects
// and never re-derives data. All four states (uncompressed, snapshot
// disabled, populated, failed-to-fetch) share the same outer wrapper so the
// layout doesn't jump when the operator toggles between tabs.
export default function RTKCompressionDiffView({ metadata, compressedItems }: Props) {
	const { t: tFn } = useTranslation("logs");

	const ratio = numberFromMetadata(metadata?.rtk_compression_ratio);
	const techniques = stringArrayFromMetadata(metadata?.rtk_techniques);
	const filterMatched = stringFromMetadata(metadata?.rtk_filter_matched);
	const rawOutputID = stringFromMetadata(metadata?.rtk_raw_output_id);
	const scannedIndices = numberArrayFromMetadata(metadata?.rtk_pipeline_scanned);

	// Empty / not-triggered state: no raw-output pointer AND no scanned indices.
	const compressedFlag = techniques.length > 0 || (scannedIndices.length ?? 0) > 0 || !!rawOutputID;
	if (!compressedFlag && ratio === null) {
		return (
			<div
				className="text-muted-foreground flex flex-col items-center justify-center gap-2 rounded-sm border border-dashed py-12 text-sm"
				data-testid="rtk-diff-uncompressed"
			>
				<div className="text-base font-medium">{tFn("detailView.rtkUncompressed")}</div>
				<div className="text-xs">{tFn("detailView.rtkNoSnapshots")}</div>
			</div>
		);
	}

	// No raw-output pointer: the pipeline ran (scanned or marked techniques)
	// but no compression fired (or retention is "never"). Show the post-
	// compression message bodies alongside a banner prompting the operator
	// to enable raw-output retention if they want the diff.
	if (!rawOutputID) {
		return (
			<div className="space-y-4" data-testid="rtk-diff-disabled">
				<RTKHeader ratio={ratio} techniques={techniques} filterMatched={filterMatched} />
				<Alert className="border-amber-300 bg-amber-50 dark:border-amber-600 dark:bg-amber-950">
					<AlertDescription className="text-amber-800 dark:text-amber-200">
						<span>{tFn("detailView.rtkSnapshotDisabled")}</span>
						<Link
							to="/workspace/plugins"
							search={{ plugin: "rtk" }}
							className="ml-1 text-blue-600 underline underline-offset-2 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
						>
							{tFn("detailView.rtkSnapshotGoToConfig")}
						</Link>
					</AlertDescription>
				</Alert>
				{compressedItems != null && compressedItems.length > 0 && (
					<div className="space-y-3">
						<p className="text-muted-foreground text-xs">{tFn("detailView.rtkCompressedOnlyHint")}</p>
						<div className="flex flex-col gap-4">
							{compressedItems.map((item) => (
								<div key={`comp-only-${item.index}`} className="rounded-sm border" data-testid={`rtk-compressed-only-${item.index}`}>
									<div className="bg-muted/20 rounded-t-sm border-b px-4 py-2 text-xs font-medium">
										{tFn("detailView.rtkMessageLabel", { index: item.index >= 0 ? item.index : 0 })}
									</div>
									<DiffPane label={tFn("detailView.rtkCompressedLabel")} content={item.content} side="compressed" />
								</div>
							))}
						</div>
					</div>
				)}
			</div>
		);
	}

	// Compressed path: fetch the raw-output file and split it across the
	// compressed message slots. The raw-output file may contain one tool
	// output or several joined by "\n\n" (matches the legacy snapshot.go
	// merged-mode separator), so we split on that marker first; any slot
	// without a match is left empty and a banner explains the heuristic.
	return <PopulatedDiff metadata={metadata} rawOutputID={rawOutputID} compressedItems={compressedItems} scannedIndices={scannedIndices} />;
}

interface PopulatedDiffProps {
	metadata: Record<string, unknown> | undefined;
	rawOutputID: string;
	compressedItems?: CompressedItem[];
	scannedIndices: ScannedIndexEntry[];
}

function PopulatedDiff({ metadata, rawOutputID, compressedItems, scannedIndices }: PopulatedDiffProps) {
	const { t: tFn } = useTranslation("logs");

	const ratio = numberFromMetadata(metadata?.rtk_compression_ratio);
	const techniques = stringArrayFromMetadata(metadata?.rtk_techniques);
	const filterMatched = stringFromMetadata(metadata?.rtk_filter_matched);

	const { data: rawText, isLoading, isError } = useGetRtkRawOutputQuery(rawOutputID);

	// Map the raw output text to the compressedItems slots. The Go snapshot
	// builder joined entries with "\n\n" when serialising merged mode; the
	// raw-output persistence layer writes one tool_result per call, but a
	// single tool call may have triggered multiple compression passes that
	// share one file. Splitting on "\n\n" matches the legacy wire shape.
	const originalByIndex = splitRawOutputByIndex(rawText, compressedItems);

	return (
		<div className="space-y-4" data-testid="rtk-diff-populated">
			<RTKHeader ratio={ratio} techniques={techniques} filterMatched={filterMatched} />
			{isError && (
				<Alert className="border-amber-300 bg-amber-50 dark:border-amber-600 dark:bg-amber-950" data-testid="rtk-diff-fetch-error-banner">
					<AlertDescription className="text-amber-800 dark:text-amber-200">{tFn("detailView.rtkRawOutputFetchError")}</AlertDescription>
				</Alert>
			)}
			{isLoading && (
				<Alert className="border-muted" data-testid="rtk-diff-fetch-loading-banner">
					<AlertDescription className="text-muted-foreground text-sm">{tFn("detailView.rtkRawOutputFetchLoading")}</AlertDescription>
				</Alert>
			)}
			<RTKSplitDiff originalByIndex={originalByIndex} compressedItems={compressedItems ?? []} scannedIndices={scannedIndices} />
		</div>
	);
}

function RTKHeader({ ratio, techniques, filterMatched }: { ratio: number | null; techniques: string[]; filterMatched: string | null }) {
	const { t } = useTranslation("logs");
	const ratioPct = ratio !== null ? `${(ratio * 100).toFixed(1)}%` : "—";
	return (
		<div className="bg-muted/30 flex flex-wrap items-center gap-3 rounded-sm border px-4 py-3 text-sm">
			<div className="flex items-center gap-2">
				<span className="text-muted-foreground text-xs font-medium">{t("detailView.rtk_compressionRatio").toUpperCase()}</span>
				<Badge variant="outline" className="font-mono text-xs">
					{ratioPct}
				</Badge>
			</div>
			{filterMatched && (
				<div className="flex items-center gap-2">
					<span className="text-muted-foreground text-xs font-medium">{t("detailView.rtk_filterMatched")}</span>
					<Badge variant="outline" className="font-mono text-xs">
						{filterMatched}
					</Badge>
				</div>
			)}
			{techniques.length > 0 && (
				<div className="flex flex-wrap items-center gap-1.5">
					<span className="text-muted-foreground text-xs font-medium">{t("detailView.rtk_techniques")}</span>
					{techniques.map((tech) => (
						<Badge key={tech} variant="secondary" className="font-mono text-[10px]">
							{tech}
						</Badge>
					))}
				</div>
			)}
		</div>
	);
}

function RTKSplitDiff({
	originalByIndex,
	compressedItems,
	scannedIndices,
}: {
	originalByIndex: Map<number, string>;
	compressedItems: CompressedItem[];
	scannedIndices: ScannedIndexEntry[];
}) {
	const { t } = useTranslation("logs");

	if (compressedItems.length === 0) {
		return (
			<div className="text-muted-foreground rounded-sm border border-dashed p-6 text-center text-sm">{t("detailView.rtkNoSnapshots")}</div>
		);
	}

	const scannedSet = new Set(scannedIndices.map((entry) => entry.index));

	return (
		<div className="flex flex-col gap-4">
			{compressedItems.map((comp) => {
				const origContent = originalByIndex.get(comp.index);
				const participated = scannedSet.has(comp.index);
				return (
					<div key={`msg-${comp.index}`} className="rounded-sm border" data-testid={`rtk-diff-message-${comp.index}`}>
						<div className="bg-muted/20 flex items-center justify-between rounded-t-sm border-b px-4 py-2 text-xs">
							<div className="flex items-center gap-2 font-medium">
								<span>{t("detailView.rtkMessageLabel", { index: comp.index >= 0 ? comp.index : 0 })}</span>
								{participated && (
									<Badge variant="outline" className="text-[10px]" data-testid={`rtk-diff-participated-${comp.index}`}>
										{t("detailView.rtkParticipated")}
									</Badge>
								)}
							</div>
						</div>
						<div className="grid grid-cols-1 gap-0 md:grid-cols-2">
							<DiffPane label={t("detailView.rtkOriginalLabel")} content={origContent ?? ""} side="original" />
							<DiffPane label={t("detailView.rtkCompressedLabel")} content={comp.content} side="compressed" />
						</div>
					</div>
				);
			})}
		</div>
	);
}

function DiffPane({ label, content, side }: { label: string; content: string; side: "original" | "compressed" }) {
	const sideClass =
		side === "original"
			? "border-border bg-muted/10"
			: "border-emerald-200/60 bg-emerald-50/40 dark:border-emerald-800/60 dark:bg-emerald-950/20";
	return (
		<div className={`flex min-h-0 flex-col border-l first:border-l-0 md:border-l ${sideClass}`}>
			<div className="text-muted-foreground border-b px-3 py-1.5 text-xs font-medium tracking-wide uppercase">{label}</div>
			<pre className="max-h-[60vh] overflow-auto px-3 py-2 font-mono text-[11px] leading-relaxed break-all whitespace-pre-wrap">
				{content}
			</pre>
		</div>
	);
}

function numberFromMetadata(v: unknown): number | null {
	if (typeof v === "number" && Number.isFinite(v)) return v;
	if (typeof v === "string") {
		const n = Number(v);
		return Number.isFinite(n) ? n : null;
	}
	return null;
}

function stringFromMetadata(v: unknown): string | null {
	if (typeof v === "string" && v.length > 0) return v;
	return null;
}

function stringArrayFromMetadata(v: unknown): string[] {
	if (!Array.isArray(v)) return [];
	return v.filter((entry) => typeof entry === "string") as string[];
}

function numberArrayFromMetadata(v: unknown): ScannedIndexEntry[] {
	if (!Array.isArray(v)) return [];
	return v.filter((entry): entry is number => typeof entry === "number" && Number.isFinite(entry)).map((index) => ({ index }));
}

// splitRawOutputByIndex partitions the raw-output file body across the
// compressedItems slots. The RTK plugin stores one raw-output file per
// actually-compressed tool_result, but several messages may be persisted in
// sequence joined by "\n\n" — the same separator the legacy snapshot.go
// merged-mode builder used. We split on that marker and assign the i-th
// chunk to compressedItems[i]. When the count doesn't match (e.g. only one
// compressed item but multiple chunks) we fall back to "all chunks to the
// first item, rest empty" so the operator sees something rather than a
// silent zero-row table.
function splitRawOutputByIndex(rawText: string | undefined, compressedItems?: CompressedItem[]): Map<number, string> {
	const out = new Map<number, string>();
	if (!rawText || compressedItems == null || compressedItems.length === 0) {
		return out;
	}

	const chunks = rawText
		.split(/\n\n/)
		.map((c) => c.trim())
		.filter((c) => c.length > 0);
	if (chunks.length === 0) {
		return out;
	}

	if (chunks.length === compressedItems.length) {
		compressedItems.forEach((item, idx) => out.set(item.index, chunks[idx]));
		return out;
	}

	// Mismatch — put everything in the first slot, leave the rest empty so
	// the operator still sees the original text rather than nothing.
	out.set(compressedItems[0].index, chunks.join("\n\n"));
	return out;
}
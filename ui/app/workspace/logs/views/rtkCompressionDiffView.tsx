import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Link } from "@tanstack/react-router";

interface SnapshotEntry {
	index: number;
	role?: string;
	name?: string;
	content: string;
	originalTokens?: number;
	compressedTokens?: number;
}

interface SnapshotPayload {
	mode?: string;
	truncated?: boolean;
	items?: SnapshotEntry[];
}

// CompressedItem is the post-compression tool message body for one snapshot
// entry, indexed by the same `index` value the RTK pipeline recorded on the
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

// RTKCompressionDiffView renders the snapshot diff from the log entry's
// rtk_original_snapshot metadata field and the compressed tool message
// bodies carried in the request itself. The component is intentionally
// read-only: it never invokes side effects and never re-derives data. All
// four states (uncompressed, snapshot disabled, truncated, populated) share
// the same outer wrapper so the layout doesn't jump when the operator
// toggles between tabs.
export default function RTKCompressionDiffView({ metadata, compressedItems }: Props) {
	const { t } = useTranslation("logs");

	const ratio = numberFromMetadata(metadata?.rtk_compression_ratio);
	const techniques = stringArrayFromMetadata(metadata?.rtk_techniques);
	const filterMatched = stringFromMetadata(metadata?.rtk_filter_matched);
	const mode = stringFromMetadata(metadata?.rtk_snapshot_mode) ?? "off";

	const original = parseSnapshotPayload(metadata?.rtk_original_snapshot);

	// Empty / not-triggered state: ratio absent AND no snapshot entries.
	const compressedFlag = techniques.length > 0 || (original.items?.length ?? 0) > 0;
	if (!compressedFlag && ratio === null) {
		return (
			<div
				className="text-muted-foreground flex flex-col items-center justify-center gap-2 rounded-sm border border-dashed py-12 text-sm"
				data-testid="rtk-diff-uncompressed"
			>
				<div className="text-base font-medium">{t("detailView.rtkUncompressed")}</div>
				<div className="text-xs">{t("detailView.rtkNoSnapshots")}</div>
			</div>
		);
	}

	// Snapshot disabled (config snapshot_mode=off) — show summary stats and
	// a prompt to enable snapshots in the RTK config page.
	if (mode === "off") {
		return (
			<div className="space-y-4" data-testid="rtk-diff-disabled">
				<RTKHeader ratio={ratio} techniques={techniques} filterMatched={filterMatched} />
				<Alert className="border-amber-300 bg-amber-50 dark:border-amber-600 dark:bg-amber-950">
					<AlertDescription className="text-amber-800 dark:text-amber-200">
						<span>{t("detailView.rtkSnapshotDisabled")}</span>
						<Link
							to="/workspace/plugins"
							search={{ plugin: "rtk" }}
							className="ml-1 text-blue-600 underline underline-offset-2 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
						>
							{t("detailView.rtkSnapshotGoToConfig")}
						</Link>
					</AlertDescription>
				</Alert>
			</div>
		);
	}

	const compressedByIndex = new Map<number, string>();
	for (const item of compressedItems ?? []) {
		compressedByIndex.set(item.index, item.content);
	}

	return (
		<div className="space-y-4" data-testid="rtk-diff-populated">
			<RTKHeader ratio={ratio} techniques={techniques} filterMatched={filterMatched} />
			{original.truncated && (
				<Alert className="border-amber-300 bg-amber-50 dark:border-amber-600 dark:bg-amber-950" data-testid="rtk-diff-truncated-banner">
					<AlertDescription className="text-amber-800 dark:text-amber-200">{t("detailView.rtkSnapshotTruncated")}</AlertDescription>
				</Alert>
			)}
			{mode === "merged" ? (
				<RTKMergedDiff original={original.items ?? []} compressedByIndex={compressedByIndex} />
			) : (
				<RTKSplitDiff original={original.items ?? []} compressedByIndex={compressedByIndex} />
			)}
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

function RTKSplitDiff({ original, compressedByIndex }: { original: SnapshotEntry[]; compressedByIndex: Map<number, string> }) {
	const { t } = useTranslation("logs");
	if (original.length === 0) {
		return (
			<div className="text-muted-foreground rounded-sm border border-dashed p-6 text-center text-sm">{t("detailView.rtkNoSnapshots")}</div>
		);
	}

	// Align compressed entries to originals by index. Entries that have no
	// compressed counterpart (compression was skipped for that message, or
	// the message lives in a TS type we don't surface — e.g. Anthropic
	// tool_result blocks) keep their original text on both sides.
	return (
		<div className="flex flex-col gap-4">
			{original.map((orig) => {
				const compContent = compressedByIndex.get(orig.index);
				const compressedTokens = compContent !== undefined ? estimateTokensLocal(compContent) : undefined;
				return (
					<div key={`msg-${orig.index}`} className="rounded-sm border" data-testid={`rtk-diff-message-${orig.index}`}>
						<div className="bg-muted/20 flex items-center justify-between rounded-t-sm border-b px-4 py-2 text-xs">
							<div className="flex items-center gap-2 font-medium">
								<span>{t("detailView.rtkMessageLabel", { index: orig.index >= 0 ? orig.index : 0 })}</span>
								{orig.name && (
									<Badge variant="outline" className="font-mono text-[10px]">
										{orig.name}
									</Badge>
								)}
								{orig.role && (
									<Badge variant="secondary" className="text-[10px]">
										{orig.role}
									</Badge>
								)}
							</div>
							{(orig.originalTokens != null || compressedTokens != null) && (
								<div className="text-muted-foreground flex items-center gap-2 font-mono text-[11px]">
									<span>{orig.originalTokens ?? "—"}</span>
									<span>→</span>
									<span>{compressedTokens ?? "—"}</span>
								</div>
							)}
						</div>
						<div className="grid grid-cols-1 gap-0 md:grid-cols-2">
							<DiffPane label={t("detailView.rtkOriginalLabel")} content={orig.content} side="original" />
							<DiffPane label={t("detailView.rtkCompressedLabel")} content={compContent ?? orig.content} side="compressed" />
						</div>
					</div>
				);
			})}
		</div>
	);
}

function RTKMergedDiff({ original, compressedByIndex }: { original: SnapshotEntry[]; compressedByIndex: Map<number, string> }) {
	const { t } = useTranslation("logs");
	if (original.length === 0) {
		return (
			<div className="text-muted-foreground rounded-sm border border-dashed p-6 text-center text-sm">{t("detailView.rtkNoSnapshots")}</div>
		);
	}
	// Merged mode collapses every original entry into one big diff. We
	// re-derive the compressed side by walking orig.items in order and
	// stitching each entry's compressed counterpart (or the original text
	// when no compressed counterpart exists) with the same \n\n separator
	// the Go snapshot builder used.
	const origText = original.map((entry) => entry.content).join("\n\n");
	const compParts: string[] = [];
	for (const entry of original) {
		compParts.push(compressedByIndex.get(entry.index) ?? entry.content);
	}
	const compText = compParts.join("\n\n") || origText;
	return (
		<div className="rounded-sm border">
			<div className="grid grid-cols-1 md:grid-cols-2">
				<DiffPane label={t("detailView.rtkOriginalLabel")} content={origText} side="original" />
				<DiffPane label={t("detailView.rtkCompressedLabel")} content={compText} side="compressed" />
			</div>
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

function parseSnapshotPayload(v: unknown): SnapshotPayload {
	if (v == null) return {};
	if (typeof v === "string") {
		try {
			const parsed = JSON.parse(v) as unknown;
			return normalisePayload(parsed);
		} catch {
			return {};
		}
	}
	if (typeof v === "object") {
		return normalisePayload(v);
	}
	return {};
}

function normalisePayload(v: unknown): SnapshotPayload {
	if (!v || typeof v !== "object") return {};
	const obj = v as SnapshotPayload;
	const items = Array.isArray(obj.items)
		? (obj.items.filter((entry): entry is SnapshotEntry => {
				if (!entry || typeof entry !== "object") return false;
				const e = entry as unknown as Record<string, unknown>;
				return typeof e.content === "string";
			}) as SnapshotEntry[])
		: [];
	return {
		mode: typeof obj.mode === "string" ? obj.mode : undefined,
		truncated: Boolean(obj.truncated),
		items,
	};
}

// estimateTokensLocal is a deliberately rough (chars/4) token estimator kept
// local to this view so the per-message "compressedTokens" badge remains
// visually consistent without importing the bifrost-side estimator. The Go
// pipeline uses the same chars/4 heuristic for snapshot bookkeeping, so the
// numbers will line up within rounding.
function estimateTokensLocal(text: string): number {
	return Math.ceil(text.length / 4);
}
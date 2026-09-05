import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";

const RTK_METADATA_KEYS = new Set<string>([
	"rtk_compression_ratio",
	"rtk_techniques",
	"rtk_filter_matched",
	"rtk_raw_output_id",
	"rtk_pipeline_scanned",
]);

interface Props {
	keyName: string;
	value: unknown;
}

// isRTKMetadataKey reports whether a given metadata key should be rendered
// with the RTK-specific badge styling instead of the generic label/value
// pair. Keeping the check in one place lets the metadata renderer in
// logDetailView stay declarative.
export function isRTKMetadataKey(key: string): boolean {
	return RTK_METADATA_KEYS.has(key);
}

// RTKMetadataBadge renders a single RTK observability field from the log
// entry's metadata. Each known key gets a dedicated visual treatment so the
// operator can scan a metadata block and immediately understand the
// compression outcome (ratio, filter matched, techniques fired, raw output
// pointer). Unknown rtk_* keys fall through to a flat badge.
export default function RTKMetadataBadge({ keyName, value }: Props) {
	const { t } = useTranslation("logs");

	switch (keyName) {
		case "rtk_compression_ratio": {
			const numeric = typeof value === "number" ? value : Number(value);
			const pct = Number.isFinite(numeric) ? (numeric * 100).toFixed(1) : "0.0";
			const tone =
				numeric >= 0.5
					? "bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200"
					: numeric >= 0.2
						? "bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200"
						: "bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-300";
			return (
				<div className="flex w-full flex-col gap-2">
					<div className="text-muted-foreground text-xs font-medium">
						{t("detailView.rtk_compressionRatio").toUpperCase().replace(/_/g, " ")}
					</div>
					<div>
						<Badge variant="outline" className={`${tone} font-mono text-xs`}>
							{pct}%
						</Badge>
					</div>
				</div>
			);
		}
		case "rtk_techniques": {
			const techniques = Array.isArray(value) ? value.map((v) => String(v)) : [];
			if (techniques.length === 0) return null;
			return (
				<div className="flex w-full flex-col gap-2">
					<div className="text-muted-foreground text-xs font-medium">{t("detailView.rtk_techniques").toUpperCase().replace(/_/g, " ")}</div>
					<div className="flex flex-wrap gap-1.5">
						{techniques.map((tech) => (
							<Badge key={tech} variant="secondary" className="font-mono text-[10px]">
								{tech}
							</Badge>
						))}
					</div>
				</div>
			);
		}
		case "rtk_filter_matched":
			return (
				<div className="flex w-full flex-col gap-2">
					<div className="text-muted-foreground text-xs font-medium">
						{t("detailView.rtk_filterMatched").toUpperCase().replace(/_/g, " ")}
					</div>
					<div>
						<Badge variant="outline" className="font-mono text-xs">
							{String(value)}
						</Badge>
					</div>
				</div>
			);
		case "rtk_raw_output_id": {
			const id = String(value);
			return (
				<div className="flex w-full flex-col gap-2">
					<div className="text-muted-foreground text-xs font-medium">RTK RAW OUTPUT</div>
					<div>
						<Link
							to="/workspace/plugins/rtk/raw-output"
							search={{ id }}
							data-testid={`rtk-raw-output-link-${id}`}
							className="text-blue-600 underline-offset-2 hover:underline dark:text-blue-400"
						>
							{t("detailView.rtk_rawOutputLink")}
						</Link>
					</div>
				</div>
			);
		}
		case "rtk_pipeline_scanned": {
			const indices = Array.isArray(value) ? value.map((v) => Number(v)).filter((n) => Number.isFinite(n)) : [];
			if (indices.length === 0) return null;
			return (
				<div className="flex w-full flex-col gap-2">
					<div className="text-muted-foreground text-xs font-medium">
						{t("detailView.rtk_pipelineScanned").toUpperCase().replace(/_/g, " ")}
					</div>
					<div className="flex flex-wrap gap-1.5">
						{indices.map((idx) => (
							<Badge key={idx} variant="secondary" className="font-mono text-[10px]">
								#{idx}
							</Badge>
						))}
					</div>
				</div>
			);
		}
		default:
			return null;
	}
}
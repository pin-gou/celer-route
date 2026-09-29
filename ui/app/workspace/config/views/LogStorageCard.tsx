import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useGetLogsStorageStatsQuery } from "@/lib/store";
import type { LogStorageStats } from "@/lib/types/logs";
import {
	Calendar,
	CloudUpload,
	Database,
	Eraser,
	EyeOff,
	FileText,
	HardDrive,
	Info,
	Layers,
	Loader2,
	RefreshCw,
	Trash2,
} from "lucide-react";
import { useTranslation } from "react-i18next";

interface Props {
	onOpenCleanup: () => void;
}

function formatBytes(bytes: number): string {
	if (!Number.isFinite(bytes) || bytes < 0) return "-";
	if (bytes < 1024) return bytes + " B";
	const units = ["KB", "MB", "GB", "TB"];
	let val = bytes / 1024;
	let i = 0;
	while (val >= 1024 && i < units.length - 1) {
		val /= 1024;
		i += 1;
	}
	return val.toFixed(val >= 100 ? 0 : val >= 10 ? 1 : 2) + " " + units[i];
}

function formatDate(iso?: string): string {
	if (!iso) return "-";
	try {
		return new Date(iso).toLocaleString();
	} catch {
		return iso;
	}
}

interface StatCardProps {
	title: string;
	value: React.ReactNode;
	subValue?: React.ReactNode;
	description?: string;
	icon: React.ReactNode;
}

function StatCard({ title, value, subValue, description, icon }: StatCardProps) {
	return (
		<Card className="py-4 shadow-none">
			<CardContent className="flex items-center justify-between px-4">
				<div className="w-full min-w-0">
					<div className="text-muted-foreground flex items-center gap-1 text-xs">
						{icon}
						<span className="truncate">{title}</span>
						{description && (
							<Tooltip>
								<TooltipTrigger asChild>
									<button
										type="button"
										aria-label={`${title} info`}
										data-testid={`logs-storage-info-${title.toLowerCase().replace(/\s+/g, "-")}`}
										className="inline-flex items-center"
									>
										<Info className="size-3 cursor-help" />
									</button>
								</TooltipTrigger>
								<TooltipContent className="max-w-80 text-left text-xs text-wrap whitespace-pre-line">{description}</TooltipContent>
							</Tooltip>
						)}
					</div>
					<div className="truncate font-mono text-xl font-medium sm:text-2xl">{value}</div>
					{subValue && <div className="truncate font-mono text-[10.5px] tabular-nums">{subValue}</div>}
				</div>
			</CardContent>
		</Card>
	);
}

function StatsBody({ stats }: { stats: LogStorageStats }) {
	const { t } = useTranslation("config");
	return (
		<div className="grid grid-cols-2 gap-4 md:grid-cols-4">
			<StatCard title={t("logging.storageTotalLogs")} value={stats.total_logs.toLocaleString()} icon={<Layers className="size-4" />} />
			<StatCard
				title={t("logging.storageSize")}
				value={formatBytes(stats.estimated_size_bytes)}
				description={stats.estimate_caveat}
				icon={<HardDrive className="size-4" />}
			/>
			<StatCard
				title={t("logging.storageStoreType")}
				value={<span className="font-mono">{stats.store_type}</span>}
				icon={<Database className="size-4" />}
			/>
			<StatCard
				title={t("logging.storageRange")}
				value={
					<span className="font-mono text-base sm:text-lg">
						{formatDate(stats.oldest_log_at)} → {formatDate(stats.newest_log_at)}
					</span>
				}
				icon={<Calendar className="size-4" />}
			/>
		</div>
	);
}

function AutoCleanupSection({ stats }: { stats: LogStorageStats }) {
	const { t } = useTranslation("config");
	const hasLastRun = !!stats.last_cleanup_at;
	const deleted = (stats.last_cleanup_deleted ?? 0) + (stats.last_cleanup_stripped ?? 0);
	return (
		<div className="rounded-sm border p-3">
			<div className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">{t("logging.storageAutoTitle")}</div>
			{hasLastRun ? (
				<div className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
					<dt className="text-muted-foreground">{t("logging.storageAutoLastRun")}</dt>
					<dd className="font-mono text-xs">{formatDate(stats.last_cleanup_at)}</dd>
					<dt className="text-muted-foreground">{t("logging.storageAutoDeleted")}</dt>
					<dd className="font-mono tabular-nums">{deleted.toLocaleString()}</dd>
					<dt className="text-muted-foreground">{t("logging.storageAutoDuration")}</dt>
					<dd className="font-mono tabular-nums">{((stats.last_cleanup_duration_ms ?? 0) / 1000).toFixed(1)}s</dd>
				</div>
			) : (
				<div className="text-muted-foreground mt-2 text-sm">{t("logging.storageAutoNever")}</div>
			)}
		</div>
	);
}

function PayloadBreakdownSection({ stats }: { stats: LogStorageStats }) {
	const { t } = useTranslation("config");
	const total = stats.total_logs;
	if (total === 0) {
		return (
			<div className="space-y-3">
				<h3 className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">{t("logging.storageBreakdownTitle")}</h3>
				<div className="text-muted-foreground text-sm">{t("logging.storageBreakdownNone")}</div>
			</div>
		);
	}
	const buckets = [
		{
			key: "with_payload",
			label: t("logging.storageBreakdownWithPayload"),
			hint: t("logging.storageBreakdownWithPayloadHint"),
			count: stats.logs_with_payload,
			size: stats.size_with_payload_bytes,
			icon: <FileText className="size-4" />,
		},
		{
			key: "stripped",
			label: t("logging.storageBreakdownStripped"),
			hint: t("logging.storageBreakdownStrippedHint"),
			count: stats.logs_stripped,
			size: 0,
			icon: <Eraser className="size-4" />,
		},
		{
			key: "offloaded",
			label: t("logging.storageBreakdownOffloaded"),
			hint: t("logging.storageBreakdownOffloadedHint"),
			count: stats.logs_offloaded,
			size: stats.size_offloaded_bytes,
			icon: <CloudUpload className="size-4" />,
		},
		{
			key: "hidden",
			label: t("logging.storageBreakdownHidden"),
			hint: t("logging.storageBreakdownHiddenHint"),
			count: stats.logs_hidden,
			size: 0,
			icon: <EyeOff className="size-4" />,
		},
	];
	return (
		<div className="space-y-3">
			<h3 className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">{t("logging.storageBreakdownTitle")}</h3>
			<div className="grid grid-cols-2 gap-4 md:grid-cols-4">
				{buckets.map((b) => (
					<StatCard
						key={b.key}
						title={b.label}
						value={`${b.count.toLocaleString()} 条`}
						subValue={b.size > 0 ? <span className="text-muted-foreground">≈ {formatBytes(b.size)}</span> : undefined}
						description={b.hint}
						icon={b.icon}
					/>
				))}
			</div>
		</div>
	);
}

export default function LogStorageCard({ onOpenCleanup }: Props) {
	const { t } = useTranslation("config");
	const { data, isFetching, refetch } = useGetLogsStorageStatsQuery(undefined, {
		pollingInterval: 30000,
	});

	return (
		<section className="space-y-4">
			<SectionTitle>{t("logging.storageTitle")}</SectionTitle>
			<p className="text-muted-foreground text-sm">{t("logging.storageSubtitleRequests")}</p>
			<div className="rounded-sm border p-4">
				{!data && isFetching && (
					<div className="text-muted-foreground flex items-center gap-2 text-sm">
						<Loader2 className="h-4 w-4 animate-spin" /> {t("logging.storageLoading")}
					</div>
				)}
				{data && (
					<div className="space-y-4">
						<div className="flex items-center justify-end gap-2">
							<Button size="sm" variant="outline" onClick={() => refetch()} disabled={isFetching} data-testid="logs-storage-refresh">
								{isFetching ? <Loader2 className="mr-1 h-3 w-3 animate-spin" /> : <RefreshCw className="mr-1 h-3 w-3" />}
								{t("logging.storageRefresh")}
							</Button>
							<Button size="sm" variant="outline" onClick={onOpenCleanup} data-testid="logs-storage-cleanup-open">
								<Trash2 className="mr-1 h-3 w-3" />
								{t("logging.storageManualCleanup")}
							</Button>
						</div>
						<StatsBody stats={data} />
						<PayloadBreakdownSection stats={data} />
						<AutoCleanupSection stats={data} />
						<p className="text-muted-foreground text-xs">{t("logging.storageEstimateCaveat")}</p>
					</div>
				)}
			</div>
		</section>
	);
}

function SectionTitle({ children }: { children: React.ReactNode }) {
	return <h3 className="text-muted-foreground px-1 text-xs font-semibold tracking-wide uppercase">{children}</h3>;
}
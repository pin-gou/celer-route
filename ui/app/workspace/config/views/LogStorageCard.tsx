import { Button } from "@/components/ui/button";
import { useGetLogsStorageStatsQuery } from "@/lib/store";
import type { LogStorageStats } from "@/lib/types/logs";
import { HardDrive, Loader2, RefreshCw, Trash2 } from "lucide-react";
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

function StatsBody({ stats }: { stats: LogStorageStats }) {
	const { t } = useTranslation("config");
	return (
		<dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
			<dt className="text-muted-foreground">{t("logging.storageTotalLogs")}</dt>
			<dd className="font-mono tabular-nums">{stats.total_logs.toLocaleString()}</dd>
			<dt className="text-muted-foreground">{t("logging.storageSize")}</dt>
			<dd className="font-mono tabular-nums">
				{formatBytes(stats.estimated_size_bytes)} <span className="text-muted-foreground text-xs">({t("logging.storageEstimate")})</span>
			</dd>
			<dt className="text-muted-foreground">{t("logging.storageStoreType")}</dt>
			<dd className="font-mono">{stats.store_type}</dd>
			<dt className="text-muted-foreground">{t("logging.storageRange")}</dt>
			<dd className="font-mono text-xs">
				{formatDate(stats.oldest_log_at)} → {formatDate(stats.newest_log_at)}
			</dd>
		</dl>
	);
}

function AutoCleanupSection({ stats }: { stats: LogStorageStats }) {
	const { t } = useTranslation("config");
	const hasLastRun = !!stats.last_auto_cleanup_at;
	return (
		<div className="rounded-sm border p-3">
			<div className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">{t("logging.storageAutoTitle")}</div>
			{hasLastRun ? (
				<div className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
					<dt className="text-muted-foreground">{t("logging.storageAutoLastRun")}</dt>
					<dd className="font-mono text-xs">{formatDate(stats.last_auto_cleanup_at)}</dd>
					<dt className="text-muted-foreground">{t("logging.storageAutoDeleted")}</dt>
					<dd className="font-mono tabular-nums">{stats.last_auto_cleanup_deleted?.toLocaleString() ?? 0}</dd>
					<dt className="text-muted-foreground">{t("logging.storageAutoDuration")}</dt>
					<dd className="font-mono tabular-nums">{((stats.last_auto_cleanup_duration_ms ?? 0) / 1000).toFixed(1)}s</dd>
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
			<div className="rounded-sm border p-3">
				<div className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">{t("logging.storageBreakdownTitle")}</div>
				<div className="text-muted-foreground mt-2 text-sm">{t("logging.storageBreakdownNone")}</div>
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
		},
		{
			key: "stripped",
			label: t("logging.storageBreakdownStripped"),
			hint: t("logging.storageBreakdownStrippedHint"),
			count: stats.logs_stripped,
			size: 0,
		},
		{
			key: "offloaded",
			label: t("logging.storageBreakdownOffloaded"),
			hint: t("logging.storageBreakdownOffloadedHint"),
			count: stats.logs_offloaded,
			size: stats.size_offloaded_bytes,
		},
		{
			key: "hidden",
			label: t("logging.storageBreakdownHidden"),
			hint: t("logging.storageBreakdownHiddenHint"),
			count: stats.logs_hidden,
			size: 0,
		},
	];
	return (
		<div className="rounded-sm border p-3">
			<div className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">{t("logging.storageBreakdownTitle")}</div>
			<div className="mt-2 space-y-2 text-sm">
				{buckets.map((b) => (
					<div key={b.key} className="flex items-start justify-between gap-2" data-testid={`logs-storage-bucket-${b.key}`}>
						<div className="min-w-0">
							<div className="font-medium">{b.label}</div>
							<div className="text-muted-foreground text-xs">{b.hint}</div>
						</div>
						<div className="text-right font-mono text-xs tabular-nums">
							<div>{b.count.toLocaleString()} 条</div>
							{b.size > 0 && <div className="text-muted-foreground">≈ {formatBytes(b.size)}</div>}
						</div>
					</div>
				))}
			</div>
			<p className="text-muted-foreground mt-2 text-xs">
				{t("logging.storageBreakdownStrippedHint")}: {formatBytes(stats.size_with_payload_bytes)} ·{" "}
				{t("logging.storageBreakdownOffloadedHint")}: {formatBytes(stats.size_offloaded_bytes)}
			</p>
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
						<div className="flex items-start justify-between gap-3">
							<div className="flex items-center gap-2 font-medium">
								<HardDrive className="h-4 w-4" />
								<span>{formatBytes(data.estimated_size_bytes)}</span>
								<span className="text-muted-foreground text-xs">
									· {data.total_logs.toLocaleString()} {t("logging.storageTotalLogs")}
								</span>
							</div>
							<div className="flex items-center gap-2">
								<Button size="sm" variant="outline" onClick={() => refetch()} disabled={isFetching} data-testid="logs-storage-refresh">
									{isFetching ? <Loader2 className="mr-1 h-3 w-3 animate-spin" /> : <RefreshCw className="mr-1 h-3 w-3" />}
									{t("logging.storageRefresh")}
								</Button>
								<Button size="sm" variant="outline" onClick={onOpenCleanup} data-testid="logs-storage-cleanup-open">
									<Trash2 className="mr-1 h-3 w-3" />
									{t("logging.storageManualCleanup")}
								</Button>
							</div>
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
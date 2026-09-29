"use client";

import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
	useCancelCleanupMutation,
	useGetCleanupStatusQuery,
	useLazyPreviewCleanupByFilterQuery,
	useStartCleanupMutation,
} from "@/lib/store";
import type { CleanupJobStatus, CleanupPreview, CleanupRequest, LogFilters } from "@/lib/types/logs";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { AlertTriangle, Info, Loader2, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

type Scope = "all" | "older_than" | "filter";

interface Props {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	filters?: LogFilters | null;
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

const SAFETY_ROW_THRESHOLD = 1_000_000;
const SAFETY_SIZE_BYTES = 5 * 1024 * 1024 * 1024;

export default function LogCleanupDialog({ open, onOpenChange, filters }: Props) {
	const { t } = useTranslation("config");
	const hasSettingsUpdate = useRbac(RbacResource.Settings, RbacOperation.Update);

	const [scope, setScope] = useState<Scope>("older_than");
	const [cutoff, setCutoff] = useState<string>(() => {
		const d = new Date();
		d.setDate(d.getDate() - 90);
		return d.toISOString().slice(0, 16);
	});
	const [stripPayloadsOnly, setStripPayloadsOnly] = useState(false);
	const [preview, setPreview] = useState<CleanupPreview | null>(null);
	const [previewError, setPreviewError] = useState<string | null>(null);
	const [confirmPhrase, setConfirmPhrase] = useState("");
	const [jobId, setJobId] = useState<string | null>(null);

	const [triggerPreview, { isFetching: previewLoading }] = useLazyPreviewCleanupByFilterQuery();
	const [startCleanup, { isLoading: starting }] = useStartCleanupMutation();
	const [cancelCleanup, { isLoading: cancelling }] = useCancelCleanupMutation();

	const { data: status } = useGetCleanupStatusQuery(jobId ? { id: jobId } : undefined, { skip: !jobId, pollingInterval: 1000 });

	useEffect(() => {
		if (open) {
			setPreview(null);
			setPreviewError(null);
			setConfirmPhrase("");
			setJobId(null);
		}
	}, [open]);

	useEffect(() => {
		if (!jobId || !status) return;
		if (status.status === "completed") {
			toast.success(t("logging.cleanupRunningDone") + (status.message ? " · " + status.message : ""));
			setJobId(null);
		} else if (status.status === "failed") {
			toast.error(t("logging.cleanupRunningFailed") + (status.last_error ? " · " + status.last_error : ""));
			setJobId(null);
		} else if (status.status === "cancelled") {
			toast.info(t("logging.cleanupRunningCancelled"));
			setJobId(null);
		}
	}, [jobId, status, t]);

	const buildRequest = useCallback((): CleanupRequest | null => {
		if (scope === "all") return { scope: "all", strip_payloads_only: false };
		if (scope === "older_than") {
			const ts = new Date(cutoff);
			if (Number.isNaN(ts.getTime())) return null;
			return { scope: "older_than", cutoff: ts.toISOString(), strip_payloads_only: stripPayloadsOnly };
		}
		if (!filters) return null;
		return { scope: "filter", filters, strip_payloads_only: stripPayloadsOnly };
	}, [scope, cutoff, filters, stripPayloadsOnly]);

	const onPreview = useCallback(async () => {
		setPreviewError(null);
		setPreview(null);
		const req = buildRequest();
		if (!req) {
			setPreviewError(t("logging.cleanupCutoff"));
			return;
		}
		try {
			const result = await triggerPreview(req).unwrap();
			setPreview(result);
		} catch (err) {
			setPreviewError(String((err as { data?: { error?: string } })?.data?.error ?? err));
		}
	}, [buildRequest, triggerPreview, t]);

	const inputsInvalid = useMemo(() => {
		if (scope === "older_than") return Number.isNaN(new Date(cutoff).getTime());
		if (scope === "filter") return !filters;
		return false;
	}, [scope, cutoff, filters]);

	const requiresExplicitConfirm =
		preview !== null && (preview.matched_logs > SAFETY_ROW_THRESHOLD || preview.estimated_size_bytes > SAFETY_SIZE_BYTES);

	const confirmOk = !requiresExplicitConfirm || confirmPhrase.trim() === "CLEAN";

	const onStart = useCallback(async () => {
		if (!preview) {
			await onPreview();
			return;
		}
		if (!confirmOk) return;
		const req = buildRequest();
		if (!req) return;
		try {
			const job = await startCleanup(req).unwrap();
			setJobId(job.id ?? null);
			setPreview(null);
			setConfirmPhrase("");
		} catch (err) {
			const e = err as { status?: number; data?: { error?: string } };
			if (e?.status === 409) {
				toast.error(t("logging.cleanupRunning"));
			} else {
				toast.error(String(e?.data?.error ?? err));
			}
		}
	}, [preview, confirmOk, buildRequest, startCleanup, onPreview, t]);

	const onCancel = useCallback(async () => {
		if (!jobId) return;
		try {
			await cancelCleanup({ id: jobId }).unwrap();
		} catch (err) {
			toast.error(String(err));
		}
	}, [jobId, cancelCleanup]);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="sm:max-w-xl" data-testid="logs-cleanup-dialog">
				<DialogHeader>
					<DialogTitle>{t("logging.cleanupDialogTitle")}</DialogTitle>
					<DialogDescription>{t("logging.cleanupDialogDesc")}</DialogDescription>
				</DialogHeader>

				{!jobId ? (
					<div className="space-y-4">
						<fieldset className="space-y-2">
							<legend className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
								{t("logging.cleanupScopeLegend")}
							</legend>
							<ScopeRadio value="older_than" current={scope} onChange={setScope} label={t("logging.cleanupScopeOlderThan")} />
							<ScopeRadio
								value="filter"
								current={scope}
								onChange={setScope}
								label={t("logging.cleanupScopeFilter")}
								description={t("logging.cleanupScopeFilterDesc")}
								disabled={!filters}
							/>
							<ScopeRadio
								value="all"
								current={scope}
								onChange={setScope}
								label={t("logging.cleanupScopeAll")}
								description={t("logging.cleanupScopeAllDesc")}
							/>
						</fieldset>

						{scope === "older_than" && (
							<div className="space-y-2">
								<Label htmlFor="cleanup-cutoff">{t("logging.cleanupCutoff")}</Label>
								<Input id="cleanup-cutoff" type="datetime-local" value={cutoff} onChange={(e) => setCutoff(e.target.value)} />
							</div>
						)}

						{scope !== "all" && (
							<label className="flex items-start gap-3 rounded-sm border p-3">
								<input
									type="checkbox"
									className="mt-1"
									checked={stripPayloadsOnly}
									onChange={(e) => setStripPayloadsOnly(e.target.checked)}
								/>
								<span className="text-sm">
									<span className="font-medium">{t("logging.cleanupStripPayloadsOnly")}</span>
									<span className="text-muted-foreground block">{t("logging.cleanupStripPayloadsOnlyDesc")}</span>
								</span>
							</label>
						)}

						<div className="rounded-sm border p-3">
							<div className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">{t("logging.cleanupPreview")}</div>
							{previewError && (
								<Alert variant="destructive" className="mt-2">
									<AlertTriangle className="h-4 w-4" />
									<AlertDescription>{previewError}</AlertDescription>
								</Alert>
							)}
							{!preview && !previewError && (
								<div className="text-muted-foreground mt-2 text-sm">
									<Info className="mr-1 inline h-3 w-3" />
									{t("logging.cleanupPreviewHint")}
								</div>
							)}
							{preview && (
								<div className="mt-2 space-y-1 text-sm">
									{preview.matched_logs === 0 ? (
										<div className="text-muted-foreground">{t("logging.cleanupPreviewEmpty")}</div>
									) : (
										<>
											<div>
												<span className="font-mono tabular-nums">{preview.matched_logs.toLocaleString()}</span>
												<span className="text-muted-foreground ml-1">{t("logging.cleanupPreviewMatched")}</span>
											</div>
											<div className="text-muted-foreground">
												{t("logging.cleanupPreviewSize")}:
												<span className="ml-1 font-mono tabular-nums">{formatBytes(preview.estimated_size_bytes)}</span>
												<span className="text-xs"> ({t("logging.storageEstimate")})</span>
											</div>
										</>
									)}
								</div>
							)}
							<Button size="sm" variant="outline" className="mt-2" onClick={onPreview} disabled={inputsInvalid || previewLoading}>
								{previewLoading ? <Loader2 className="mr-1 h-3 w-3 animate-spin" /> : null}
								{t("logging.cleanupPreview")}
							</Button>
						</div>

						{preview && preview.matched_logs > 0 && requiresExplicitConfirm && (
							<div className="space-y-2">
								<Alert variant="destructive">
									<AlertTriangle className="h-4 w-4" />
									<AlertDescription>{t("logging.cleanupErrTooLarge")}</AlertDescription>
								</Alert>
								<div className="space-y-1">
									<Label htmlFor="cleanup-confirm">{t("logging.cleanupConfirmPhrase")}</Label>
									<Input id="cleanup-confirm" value={confirmPhrase} onChange={(e) => setConfirmPhrase(e.target.value)} autoComplete="off" />
								</div>
							</div>
						)}
					</div>
				) : (
					<RunningPanel
						status={status ?? null}
						onCancel={onCancel}
						cancelling={cancelling}
						labels={{
							title: t("logging.cleanupRunning"),
							progress: t("logging.cleanupRunningProgress"),
							of: t("logging.cleanupRunningTotal"),
							cancel: t("logging.cleanupCancelRunning"),
						}}
					/>
				)}

				<DialogFooter>
					{!jobId ? (
						<>
							<Button variant="outline" onClick={() => onOpenChange(false)}>
								{t("logging.cleanupCancel")}
							</Button>
							<Button
								variant="destructive"
								onClick={onStart}
								disabled={!hasSettingsUpdate || starting || !preview || preview.matched_logs === 0 || !confirmOk}
								data-testid="logs-cleanup-start"
							>
								{starting ? <Loader2 className="mr-1 h-3 w-3 animate-spin" /> : <Trash2 className="mr-1 h-3 w-3" />}
								{t("logging.cleanupStart")}
							</Button>
						</>
					) : (
						<Button variant="outline" onClick={() => onOpenChange(false)}>
							{t("logging.cleanupCancel")}
						</Button>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

interface ScopeRadioProps {
	value: Scope;
	current: Scope;
	onChange: (v: Scope) => void;
	label: string;
	description?: string;
	disabled?: boolean;
}

function ScopeRadio({ value, current, onChange, label, description, disabled }: ScopeRadioProps) {
	return (
		<label className={"flex items-start gap-2 rounded-sm border p-3 text-sm" + (disabled ? " opacity-50" : "")}>
			<input
				type="radio"
				className="mt-1"
				checked={current === value}
				onChange={() => onChange(value)}
				disabled={disabled}
				name="cleanup-scope"
			/>
			<span>
				<span className="font-medium">{label}</span>
				{description && <span className="text-muted-foreground block">{description}</span>}
			</span>
		</label>
	);
}

interface RunningPanelProps {
	status: CleanupJobStatus | null;
	onCancel: () => void;
	cancelling: boolean;
	labels: {
		title: string;
		progress: string;
		of: string;
		cancel: string;
	};
}

function RunningPanel({ status, onCancel, cancelling, labels }: RunningPanelProps) {
	const pct = status && status.total > 0 ? Math.min(100, Math.round((status.processed / status.total) * 100)) : 0;
	return (
		<div className="space-y-3">
			<div className="text-sm font-medium">{labels.title}</div>
			<div className="bg-muted h-2 w-full overflow-hidden rounded">
				<div className="bg-primary h-full transition-all" style={{ width: pct + "%" }} data-testid="logs-cleanup-progress" />
			</div>
			<div className="text-muted-foreground text-xs tabular-nums">
				{labels.progress}: {status?.processed ?? 0} {labels.of} {status?.total ?? 0}
			</div>
			{status?.message && <div className="text-muted-foreground text-xs">{status.message}</div>}
			<div>
				<Button variant="outline" size="sm" onClick={onCancel} disabled={cancelling}>
					{labels.cancel}
				</Button>
			</div>
		</div>
	);
}
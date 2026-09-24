import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import type {
	AlertChannel,
	AlertChannelType,
	AlertComparison,
	AlertMetric,
	AlertRule,
	AlertRuleUpsertRequest,
	AlertRuleStatus,
} from "@/lib/store/apis/alertingApi";
import { Plus, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

const METRICS: AlertMetric[] = ["cost_per_minute", "cost_per_request", "request_error_rate", "tokens_per_minute", "budget_consumption"];
const COMPARISONS: AlertComparison[] = ["gte", "gt", "lte", "lt", "eq"];
const STATUSES: AlertRuleStatus[] = ["enabled", "disabled", "draft"];
const SCOPES: AlertRule["scope_type"][] = ["global", "team", "customer", "virtual_key"];
const CHANNEL_TYPES: AlertChannelType[] = ["webhook", "slack", "smtp"];

interface AlertRuleSheetProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	rule: AlertRule | null;
	onSubmit: (body: AlertRuleUpsertRequest) => Promise<void>;
	isSubmitting: boolean;
}

export function AlertRuleSheet({ open, onOpenChange, rule, onSubmit, isSubmitting }: AlertRuleSheetProps) {
	const { t } = useTranslation("alerting");
	const [name, setName] = useState("");
	const [scopeType, setScopeType] = useState<AlertRule["scope_type"]>("global");
	const [scopeID, setScopeID] = useState("");
	const [metric, setMetric] = useState<AlertMetric>("cost_per_minute");
	const [threshold, setThreshold] = useState<string>("0");
	const [comparison, setComparison] = useState<AlertComparison>("gte");
	const [cooldownMinutes, setCooldownMinutes] = useState<string>("15");
	const [status, setStatus] = useState<AlertRuleStatus>("enabled");
	const [channels, setChannels] = useState<AlertChannel[]>([{ type: "webhook", target: "" }]);

	useEffect(() => {
		if (rule) {
			setName(rule.name);
			setScopeType(rule.scope_type);
			setScopeID(rule.scope_id);
			setMetric(rule.metric);
			setThreshold(String(rule.threshold));
			setComparison(rule.comparison);
			setCooldownMinutes(String(rule.cooldown_minutes));
			setStatus(rule.status);
			setChannels(rule.channels.length ? rule.channels : [{ type: "webhook", target: "" }]);
		} else {
			setName("");
			setScopeType("global");
			setScopeID("");
			setMetric("cost_per_minute");
			setThreshold("0");
			setComparison("gte");
			setCooldownMinutes("15");
			setStatus("enabled");
			setChannels([{ type: "webhook", target: "" }]);
		}
	}, [rule, open]);

	const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();
		const thresholdNum = Number(threshold);
		const cooldownNum = Number(cooldownMinutes);
		if (!name.trim()) return;
		const cleanChannels = channels.filter((c) => c.target.trim() !== "" || c.webhook_id);
		if (cleanChannels.length === 0) {
			alert(t("validationOneChannel"));
			return;
		}
		await onSubmit({
			name: name.trim(),
			scope_type: scopeType,
			scope_id: scopeType === "global" ? "" : scopeID.trim(),
			metric,
			threshold: Number.isFinite(thresholdNum) ? thresholdNum : 0,
			comparison,
			cooldown_minutes: Number.isFinite(cooldownNum) ? cooldownNum : 15,
			status,
			channels: cleanChannels,
		});
	};

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="w-full sm:max-w-xl" data-testid="alerting-sheet">
				<SheetHeader>
					<SheetTitle>{rule ? t("editRule") : t("createRule")}</SheetTitle>
					<SheetDescription>{t("sheetDesc")}</SheetDescription>
				</SheetHeader>
				<form onSubmit={handleSubmit} className="space-y-4 px-4 pb-4">
					<div className="space-y-2">
						<Label htmlFor="alert-name">{t("field_name")}</Label>
						<Input id="alert-name" value={name} onChange={(e) => setName(e.target.value)} required data-testid="alerting-name" />
					</div>

					<div className="grid grid-cols-2 gap-3">
						<div className="space-y-2">
							<Label>{t("field_scope_type")}</Label>
							<Select value={scopeType} onValueChange={(v) => setScopeType(v as AlertRule["scope_type"])}>
								<SelectTrigger data-testid="alerting-scope-type">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{SCOPES.map((s) => (
										<SelectItem key={s} value={s}>
											{t(`scope_${s}`)}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
						{scopeType !== "global" && (
							<div className="space-y-2">
								<Label htmlFor="alert-scope-id">{t("field_scope_id")}</Label>
								<Input
									id="alert-scope-id"
									value={scopeID}
									onChange={(e) => setScopeID(e.target.value)}
									required
									placeholder="UUID"
									data-testid="alerting-scope-id"
								/>
							</div>
						)}
					</div>

					<div className="grid grid-cols-3 gap-3">
						<div className="col-span-2 space-y-2">
							<Label>{t("field_metric")}</Label>
							<Select value={metric} onValueChange={(v) => setMetric(v as AlertMetric)}>
								<SelectTrigger data-testid="alerting-metric">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{METRICS.map((m) => (
										<SelectItem key={m} value={m}>
											{t(`metric_${m}`)}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
						<div className="space-y-2">
							<Label>{t("field_status")}</Label>
							<Select value={status} onValueChange={(v) => setStatus(v as AlertRuleStatus)}>
								<SelectTrigger data-testid="alerting-status">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{STATUSES.map((s) => (
										<SelectItem key={s} value={s}>
											{t(`status_${s}`)}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
					</div>

					<div className="grid grid-cols-3 gap-3">
						<div className="space-y-2">
							<Label>{t("field_comparison")}</Label>
							<Select value={comparison} onValueChange={(v) => setComparison(v as AlertComparison)}>
								<SelectTrigger data-testid="alerting-comparison">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{COMPARISONS.map((c) => (
										<SelectItem key={c} value={c}>
											{c}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
						<div className="space-y-2">
							<Label htmlFor="alert-threshold">{t("field_threshold")}</Label>
							<Input
								id="alert-threshold"
								type="number"
								step="any"
								value={threshold}
								onChange={(e) => setThreshold(e.target.value)}
								required
								data-testid="alerting-threshold"
							/>
						</div>
						<div className="space-y-2">
							<Label htmlFor="alert-cooldown">{t("field_cooldown")}</Label>
							<Input
								id="alert-cooldown"
								type="number"
								min={1}
								value={cooldownMinutes}
								onChange={(e) => setCooldownMinutes(e.target.value)}
								required
								data-testid="alerting-cooldown"
							/>
						</div>
					</div>

					<div className="space-y-2">
						<div className="flex items-center justify-between">
							<Label>{t("field_channels")}</Label>
							<Button
								type="button"
								variant="ghost"
								size="sm"
								onClick={() => setChannels([...channels, { type: "webhook", target: "" }])}
								data-testid="alerting-add-channel"
							>
								<Plus className="mr-1 h-3 w-3" /> {t("addChannel")}
							</Button>
						</div>
						<div className="space-y-2">
							{channels.map((ch, idx) => (
								<div key={idx} className="grid grid-cols-[120px_1fr_auto] items-center gap-2" data-testid={`alerting-channel-${idx}`}>
									<Select value={ch.type} onValueChange={(v) => updateChannel(idx, { type: v as AlertChannelType })}>
										<SelectTrigger>
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											{CHANNEL_TYPES.map((ct) => (
												<SelectItem key={ct} value={ct}>
													{ct}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
									<Input
										placeholder={t("channelTargetPlaceholder")}
										value={ch.target}
										onChange={(e) => updateChannel(idx, { target: e.target.value })}
									/>
									<Button
										type="button"
										variant="ghost"
										size="icon"
										disabled={channels.length === 1}
										onClick={() => setChannels(channels.filter((_, i) => i !== idx))}
									>
										<Trash2 className="h-4 w-4" />
									</Button>
								</div>
							))}
						</div>
					</div>

					<SheetFooter className="px-0">
						<Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
							{t("cancel")}
						</Button>
						<Button type="submit" isLoading={isSubmitting} data-testid="alerting-submit">
							{rule ? t("save") : t("create")}
						</Button>
					</SheetFooter>
				</form>
			</SheetContent>
		</Sheet>
	);

	function updateChannel(idx: number, patch: Partial<AlertChannel>) {
		setChannels((prev) => prev.map((c, i) => (i === idx ? { ...c, ...patch } : c)));
	}
}
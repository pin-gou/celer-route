import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Pagination } from "@/components/ui/pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
	getErrorMessage,
	useGetAlertRuleQuery,
	useGetBudgetProjectionQuery,
	useListAlertEventsQuery,
	useTestAlertRuleMutation,
} from "@/lib/store";
import type { AlertEvent } from "@/lib/store/apis/alertingApi";
import { Link, useParams } from "@tanstack/react-router";
import { AlertTriangle, ArrowLeft, BellRing, Loader2, Send, TrendingUp } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

const PAGE_SIZE = 25;

function statusVariant(status: AlertEvent["status"]) {
	switch (status) {
		case "firing":
			return "destructive" as const;
		case "resolved":
			return "secondary" as const;
	}
}

function severityVariant(severity: AlertEvent["severity"]) {
	switch (severity) {
		case "critical":
			return "destructive" as const;
		case "warning":
			return "default" as const;
	}
}

export default function AlertRuleDetailView() {
	const { ruleId } = useParams({ from: "/workspace/alerting/$ruleId" });
	const { t } = useTranslation("alerting");
	const [offset, setOffset] = useState(0);
	const [statusFilter, setStatusFilter] = useState<"all" | AlertEvent["status"]>("all");

	const { data: rule, isLoading: ruleLoading, error: ruleError } = useGetAlertRuleQuery(ruleId);
	const { data: events, isLoading: eventsLoading } = useListAlertEventsQuery({
		rule_id: ruleId,
		limit: PAGE_SIZE,
		offset,
		status: statusFilter === "all" ? undefined : statusFilter,
	});
	// US17: budget_consumption rules attach to a specific budget_id. The
	// projection endpoint powers the "predicted exhaustion" card on the
	// rule detail page so the admin sees the budget's risk level + ETA
	// alongside the event history.
	const showBudgetProjection = rule?.metric === "budget_consumption";
	const { data: projection, isLoading: projectionLoading } = useGetBudgetProjectionQuery(ruleId, {
		skip: !showBudgetProjection,
	});

	const [testAlertRule, { isLoading: isTesting }] = useTestAlertRuleMutation();

	if (ruleLoading) {
		return (
			<div className="flex min-h-[40vh] items-center justify-center" data-testid="alerting-detail-loading">
				<Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
			</div>
		);
	}

	if (ruleError || !rule) {
		return (
			<div className="mx-auto max-w-3xl space-y-4 p-8" data-testid="alerting-detail-error">
				<h1 className="text-foreground text-xl font-semibold">{t("ruleNotFound")}</h1>
				<p className="text-muted-foreground text-sm">{getErrorMessage(ruleError)}</p>
				<Button asChild variant="outline">
					<Link to="/workspace/alerting">
						<ArrowLeft className="mr-2 h-4 w-4" /> {t("backToList")}
					</Link>
				</Button>
			</div>
		);
	}

	const eventsList = events?.events ?? [];
	const totalEvents = events?.total ?? 0;

	const onTest = async () => {
		try {
			await testAlertRule(rule.id).unwrap();
			toast.success(t("ruleTestSent", { name: rule.name }));
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	return (
		<div className="mx-auto max-w-5xl space-y-6 p-6" data-testid="alerting-detail">
			<header className="flex flex-wrap items-start justify-between gap-3">
				<div>
					<Button asChild variant="ghost" size="sm" className="mb-2">
						<Link to="/workspace/alerting">
							<ArrowLeft className="mr-1 h-3 w-3" /> {t("backToList")}
						</Link>
					</Button>
					<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold">
						<BellRing className="h-6 w-6" /> {rule.name}
					</h1>
					<p className="text-muted-foreground mt-1 font-mono text-xs">{rule.id}</p>
				</div>
				<div className="flex items-center gap-2">
					<Badge variant="outline">{t(`scope_${rule.scope_type}`)}</Badge>
					<Badge variant={rule.status === "enabled" ? "default" : "secondary"}>{t(`status_${rule.status}`)}</Badge>
					<Button variant="outline" size="sm" onClick={onTest} isLoading={isTesting} data-testid="alerting-detail-test">
						<Send className="mr-1 h-3 w-3" /> {t("action_test")}
					</Button>
				</div>
			</header>

			<div className="grid grid-cols-1 gap-4 md:grid-cols-3">
				<Card>
					<CardHeader>
						<CardTitle className="text-base">{t("detail_metric")}</CardTitle>
					</CardHeader>
					<CardContent className="space-y-2 text-sm">
						<div className="font-mono">{t(`metric_${rule.metric}`)}</div>
						<div className="text-muted-foreground text-xs">
							{rule.comparison} {rule.threshold}
						</div>
					</CardContent>
				</Card>
				<Card>
					<CardHeader>
						<CardTitle className="text-base">{t("detail_cooldown")}</CardTitle>
					</CardHeader>
					<CardContent className="text-sm">
						<div className="font-mono">{rule.cooldown_minutes} min</div>
					</CardContent>
				</Card>
				<Card>
					<CardHeader>
						<CardTitle className="text-base">{t("detail_channels")}</CardTitle>
					</CardHeader>
					<CardContent className="space-y-1 text-sm">
						{rule.channels.length === 0 ? (
							<p className="text-muted-foreground text-xs">{t("noChannels")}</p>
						) : (
							rule.channels.map((c, i) => (
								<div key={i} className="font-mono text-xs">
									{c.type} · {c.target || c.webhook_id || "—"}
								</div>
							))
						)}
					</CardContent>
				</Card>
			</div>

			{showBudgetProjection && (
				<Card data-testid="alerting-detail-budget-projection">
					<CardHeader>
						<CardTitle className="flex items-center gap-2 text-base">
							<TrendingUp className="h-4 w-4" />
							{t("budgetProjection.title")}
						</CardTitle>
						<CardDescription>{t("budgetProjection.subtitle")}</CardDescription>
					</CardHeader>
					<CardContent>
						{projectionLoading ? (
							<p className="text-muted-foreground text-sm">
								<Loader2 className="mr-1 inline h-3 w-3 animate-spin" />
								{t("loading")}
							</p>
						) : projection ? (
							<div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
								<div>
									<p className="text-muted-foreground text-xs">{t("budgetProjection.used")}</p>
									<p className="text-xl font-semibold">${projection.used_amount.toFixed(2)}</p>
									<p className="text-muted-foreground text-xs">
										{t("budgetProjection.max")} ${projection.max_amount.toFixed(2)}
									</p>
								</div>
								<div>
									<p className="text-muted-foreground text-xs">{t("budgetProjection.usagePercent")}</p>
									<p className="text-xl font-semibold">{projection.usage_percent.toFixed(1)}%</p>
									<div className="bg-muted mt-1 h-2 w-full overflow-hidden rounded">
										<div className="bg-primary h-full" style={{ width: `${Math.min(projection.usage_percent, 100)}%` }} />
									</div>
								</div>
								<div>
									<p className="text-muted-foreground text-xs">{t("budgetProjection.riskLevel")}</p>
									<Badge
										variant={
											projection.risk_level === "high" ? "destructive" : projection.risk_level === "medium" ? "default" : "secondary"
										}
										className="mt-1"
									>
										{projection.risk_level === "high" && <AlertTriangle className="mr-1 h-3 w-3" />}
										{projection.risk_level}
									</Badge>
									<p className="text-muted-foreground mt-1 text-xs">
										{projection.has_projection && projection.predicted_exhaustion
											? t("budgetProjection.exhaustion", {
													when: new Date(projection.predicted_exhaustion).toLocaleDateString(),
												})
											: (projection.reason ?? t("budgetProjection.noProjection"))}
									</p>
								</div>
							</div>
						) : (
							<p className="text-muted-foreground text-sm">{t("budgetProjection.unavailable")}</p>
						)}
					</CardContent>
				</Card>
			)}

			<section className="flex flex-col gap-3">
				<header className="flex items-center justify-between">
					<h2 className="text-lg font-semibold">{t("detail_events")}</h2>
					<Select
						value={statusFilter}
						onValueChange={(v) => {
							setStatusFilter(v as typeof statusFilter);
							setOffset(0);
						}}
					>
						<SelectTrigger className="w-40" data-testid="alerting-events-status">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="all">{t("statusAll")}</SelectItem>
							<SelectItem value="firing">{t("status_firing")}</SelectItem>
							<SelectItem value="resolved">{t("status_resolved")}</SelectItem>
						</SelectContent>
					</Select>
				</header>

				<div className="border-border bg-card rounded-sm border">
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>{t("col_triggered")}</TableHead>
								<TableHead>{t("col_status")}</TableHead>
								<TableHead>{t("col_severity")}</TableHead>
								<TableHead>{t("col_value")}</TableHead>
								<TableHead>{t("col_resolved")}</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{eventsLoading ? (
								<TableRow>
									<TableCell colSpan={5} className="text-muted-foreground py-8 text-center text-sm">
										<Loader2 className="mr-2 inline h-4 w-4 animate-spin" /> {t("loading")}
									</TableCell>
								</TableRow>
							) : eventsList.length === 0 ? (
								<TableRow>
									<TableCell colSpan={5} className="text-muted-foreground py-8 text-center text-sm">
										{t("noEvents")}
									</TableCell>
								</TableRow>
							) : (
								eventsList.map((ev) => (
									<TableRow key={ev.id} data-testid={`alerting-event-${ev.id}`}>
										<TableCell className="text-xs">{new Date(ev.triggered_at).toLocaleString()}</TableCell>
										<TableCell>
											<Badge variant={statusVariant(ev.status)}>{t(`status_${ev.status}`)}</Badge>
										</TableCell>
										<TableCell>
											<Badge variant={severityVariant(ev.severity)}>{t(`severity_${ev.severity}`)}</Badge>
										</TableCell>
										<TableCell className="font-mono text-xs">{ev.value}</TableCell>
										<TableCell className="text-muted-foreground text-xs">
											{ev.resolved_at ? new Date(ev.resolved_at).toLocaleString() : "—"}
										</TableCell>
									</TableRow>
								))
							)}
						</TableBody>
					</Table>
				</div>
				{eventsList.length > 0 && (
					<Pagination offset={offset} limit={PAGE_SIZE} totalCount={totalEvents} onOffsetChange={(next) => setOffset(next)} showItemsInfo />
				)}
			</section>
		</div>
	);
}
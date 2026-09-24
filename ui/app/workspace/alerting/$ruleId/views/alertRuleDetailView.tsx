import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Pagination } from "@/components/ui/pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getErrorMessage, useGetAlertRuleQuery, useListAlertEventsQuery, useTestAlertRuleMutation } from "@/lib/store";
import type { AlertEvent } from "@/lib/store/apis/alertingApi";
import { Link, useParams } from "@tanstack/react-router";
import { ArrowLeft, BellRing, Loader2, Send } from "lucide-react";
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

	const [testAlertRule, { isLoading: isTesting }] = useTestAlertRuleMutation();

	const onTest = async () => {
		try {
			await testAlertRule(ruleId).unwrap();
			toast.success(t("ruleTestSent", { name: rule?.name }));
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	if (ruleLoading) {
		return (
			<div className="flex min-h-[40vh] items-center justify-center" data-testid="alerting-detail-loading">
				<Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
			</div>
		);
	}

	if (ruleError || !rule) {
		return (
			<div className="p-8">
				<Card>
					<CardHeader>
						<CardTitle>{t("loadFailed")}</CardTitle>
						<CardDescription>{ruleError ? getErrorMessage(ruleError) : t("ruleNotFound")}</CardDescription>
					</CardHeader>
					<CardContent>
						<Button asChild variant="outline">
							<Link to="/workspace/alerting">{t("backToList")}</Link>
						</Button>
					</CardContent>
				</Card>
			</div>
		);
	}

	const eventsList = events?.events ?? [];
	const totalEvents = events?.total ?? 0;

	return (
		<div className="flex h-full flex-col gap-4 p-6" data-testid={`alerting-detail-${ruleId}`}>
			<header className="flex flex-wrap items-start justify-between gap-3">
				<div className="flex items-center gap-3">
					<Button asChild variant="ghost" size="icon">
						<Link to="/workspace/alerting" data-testid="alerting-detail-back">
							<ArrowLeft className="h-4 w-4" />
						</Link>
					</Button>
					<div>
						<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold">
							<BellRing className="h-6 w-6" /> {rule.name}
						</h1>
						<p className="text-muted-foreground mt-1 font-mono text-xs">{rule.id}</p>
					</div>
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
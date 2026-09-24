import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Pagination } from "@/components/ui/pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getErrorMessage, useGetGatewayDeltaQuery } from "@/lib/store";
import type { GatewayDeltaResponse, GatewayDeltaRow } from "@/lib/store/apis/reportsApi";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, ArrowUpRight, Download, Loader2, Scale } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

const PAGE_SIZE = 25;

type Dimension = "provider" | "model";

function formatCurrency(value: number, currency: string): string {
	try {
		return new Intl.NumberFormat("en-US", {
			style: "currency",
			currency: currency || "USD",
			maximumFractionDigits: 4,
		}).format(value);
	} catch {
		return `${value.toFixed(4)} ${currency}`;
	}
}

function deltaVariant(delta: number) {
	if (delta > 0) return "default" as const;
	if (delta < 0) return "destructive" as const;
	return "secondary" as const;
}

export default function GatewayDeltaView() {
	const { t } = useTranslation("reports");
	const [dimension, setDimension] = useState<Dimension>("provider");
	const [accuracy, setAccuracy] = useState("auto");
	const [offset, setOffset] = useState(0);

	const params = useMemo(() => ({ accuracy }), [accuracy]);

	const { data, isLoading, error, isFetching } = useGetGatewayDeltaQuery(params);

	const handleExport = () => {
		if (!data) return;
		const search = new URLSearchParams({ start: data.period.start, end: data.period.end, accuracy });
		window.open(`/api/reports/gateway-delta/export?${search.toString()}`, "_blank", "noopener,noreferrer");
	};

	const rows: GatewayDeltaRow[] = data ? (dimension === "provider" ? data.providers : data.models) : [];
	const totalRows = rows.length;

	return (
		<div className="flex h-full flex-col gap-4 p-6" data-testid="gateway-delta-page">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div className="flex items-center gap-3">
					<Button asChild variant="ghost" size="icon">
						<Link to="/workspace/reports" data-testid="gateway-delta-back">
							<ArrowLeft className="h-4 w-4" />
						</Link>
					</Button>
					<div>
						<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold">
							<Scale className="h-6 w-6" /> {t("gatewayDelta.title")}
						</h1>
						<p className="text-muted-foreground mt-1 text-sm">{t("gatewayDelta.subtitle")}</p>
					</div>
				</div>
				<div className="flex items-center gap-2">
					<Select value={accuracy} onValueChange={setAccuracy}>
						<SelectTrigger className="w-32" data-testid="gateway-delta-accuracy">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="auto">{t("gatewayDelta.accuracy_auto")}</SelectItem>
							<SelectItem value="exact">{t("gatewayDelta.accuracy_exact")}</SelectItem>
							<SelectItem value="snapshot">{t("gatewayDelta.accuracy_snapshot")}</SelectItem>
						</SelectContent>
					</Select>
					<Button variant="outline" size="sm" onClick={handleExport} disabled={!data} data-testid="gateway-delta-export">
						<Download className="mr-1 h-3 w-3" /> {t("export")}
					</Button>
				</div>
			</header>

			{error ? (
				<Card>
					<CardHeader>
						<CardTitle>{t("loadFailed")}</CardTitle>
						<CardDescription>{getErrorMessage(error)}</CardDescription>
					</CardHeader>
				</Card>
			) : !data || isLoading ? (
				<div className="flex min-h-[40vh] items-center justify-center" data-testid="gateway-delta-loading">
					<Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
				</div>
			) : (
				<>
					<TotalStrip data={data} />
					<CoverageStrip data={data} />

					<div className="flex items-center justify-between">
						<Select
							value={dimension}
							onValueChange={(v) => {
								setDimension(v as Dimension);
								setOffset(0);
							}}
						>
							<SelectTrigger className="w-40" data-testid="gateway-delta-dimension">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="provider">{t("gatewayDelta.dimension_provider")}</SelectItem>
								<SelectItem value="model">{t("gatewayDelta.dimension_model")}</SelectItem>
							</SelectContent>
						</Select>
						<span className="text-muted-foreground text-sm">
							{t("totalCount", { count: totalRows })}
							{isFetching && <Loader2 className="ml-2 inline h-3 w-3 animate-spin" />}
						</span>
					</div>

					<div className="border-border bg-card rounded-sm border">
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>{dimension === "provider" ? t("gatewayDelta.col_provider") : t("gatewayDelta.col_model")}</TableHead>
									<TableHead>{t("gatewayDelta.col_requests")}</TableHead>
									<TableHead>{t("gatewayDelta.col_tokens")}</TableHead>
									<TableHead>{t("gatewayDelta.col_actual")}</TableHead>
									<TableHead>{t("gatewayDelta.col_standard")}</TableHead>
									<TableHead>{t("gatewayDelta.col_delta")}</TableHead>
									<TableHead>{t("gatewayDelta.col_share")}</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{rows.length === 0 ? (
									<TableRow>
										<TableCell colSpan={7} className="text-muted-foreground py-8 text-center text-sm">
											{t("empty")}
										</TableCell>
									</TableRow>
								) : (
									rows.slice(offset, offset + PAGE_SIZE).map((row) => (
										<TableRow key={row.id} data-testid={`gateway-delta-row-${row.id}`}>
											<TableCell className="font-mono text-xs">
												{row.name ?? row.provider ?? row.model ?? row.id}
												{!row.has_standard_price && (
													<Badge variant="outline" className="ml-2 text-xs">
														{t("gatewayDelta.unpriced")}
													</Badge>
												)}
											</TableCell>
											<TableCell className="font-mono text-xs">{row.requests.toLocaleString()}</TableCell>
											<TableCell className="font-mono text-xs">{row.tokens.toLocaleString()}</TableCell>
											<TableCell className="font-mono text-xs">{formatCurrency(row.cost_actual, data.currency)}</TableCell>
											<TableCell className="font-mono text-xs">{formatCurrency(row.cost_standard, data.currency)}</TableCell>
											<TableCell>
												<Badge variant={deltaVariant(row.delta)}>
													{row.delta >= 0 ? "+" : ""}
													{formatCurrency(row.delta, data.currency)}
												</Badge>
											</TableCell>
											<TableCell className="font-mono text-xs">{row.share_pct != null ? `${row.share_pct.toFixed(2)}%` : "—"}</TableCell>
										</TableRow>
									))
								)}
							</TableBody>
						</Table>
					</div>

					{totalRows > PAGE_SIZE && (
						<Pagination offset={offset} limit={PAGE_SIZE} totalCount={totalRows} onOffsetChange={(next) => setOffset(next)} showItemsInfo />
					)}

					{data.note && (
						<p className="text-muted-foreground text-xs" data-testid="gateway-delta-note">
							{data.note}
						</p>
					)}
				</>
			)}
		</div>
	);
}

function TotalStrip({ data }: { data: GatewayDeltaResponse }) {
	const { t } = useTranslation("reports");
	const total = data.total;
	return (
		<div className="grid grid-cols-2 gap-3 md:grid-cols-4" data-testid="gateway-delta-total">
			<Card>
				<CardHeader>
					<CardDescription>{t("gatewayDelta.metric_requests")}</CardDescription>
					<CardTitle className="font-mono text-2xl">{(total.requests ?? 0).toLocaleString()}</CardTitle>
				</CardHeader>
			</Card>
			<Card>
				<CardHeader>
					<CardDescription>{t("gatewayDelta.metric_tokens")}</CardDescription>
					<CardTitle className="font-mono text-2xl">{(total.tokens ?? 0).toLocaleString()}</CardTitle>
				</CardHeader>
			</Card>
			<Card>
				<CardHeader>
					<CardDescription>{t("gatewayDelta.metric_actual")}</CardDescription>
					<CardTitle className="font-mono text-2xl">{formatCurrency(total.cost_actual ?? 0, data.currency)}</CardTitle>
				</CardHeader>
			</Card>
			<Card>
				<CardHeader>
					<CardDescription>{t("gatewayDelta.metric_delta")}</CardDescription>
					<CardTitle className="font-mono text-2xl">
						<div className="flex items-center gap-1">
							{formatCurrency(total.delta ?? 0, data.currency)}
							<ArrowUpRight className="text-muted-foreground h-4 w-4" />
						</div>
					</CardTitle>
				</CardHeader>
				<CardContent className="text-muted-foreground text-xs">
					{t("gatewayDelta.periodLabel", {
						start: new Date(data.period.start).toLocaleDateString(),
						end: new Date(data.period.end).toLocaleDateString(),
					})}
				</CardContent>
			</Card>
		</div>
	);
}

function CoverageStrip({ data }: { data: GatewayDeltaResponse }) {
	const { t } = useTranslation("reports");
	const cov = data.coverage;
	const pct = cov.models_total > 0 ? ((cov.models_priced / cov.models_total) * 100).toFixed(1) : "0.0";
	return (
		<Card data-testid="gateway-delta-coverage">
			<CardHeader>
				<CardTitle className="text-base">{t("gatewayDelta.coverageTitle")}</CardTitle>
				<CardDescription>{t("gatewayDelta.coverageDesc")}</CardDescription>
			</CardHeader>
			<CardContent className="text-sm">
				<span className="font-mono">{pct}%</span>{" "}
				<span className="text-muted-foreground">
					({cov.models_priced}/{cov.models_total} {t("gatewayDelta.modelsPriced")})
				</span>
			</CardContent>
		</Card>
	);
}
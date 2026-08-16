import { Button } from "@/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuPortal,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "@/components/ui/dropdownMenu";
import { buildCSV, downloadCSV } from "@/lib/utils/csv";
import { Download, FileSpreadsheet, FileText, Loader2 } from "lucide-react";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { type DashboardData, type DashboardTab, type ExportTab, getCSVSections } from "../utils/exportUtils";

interface ExportPopoverProps {
	getData: () => DashboardData;
	/** The tab currently open, offered as the "this tab only" export scope. */
	activeTab: DashboardTab;
	/** Enters export mode, loads the scoped tabs' uncapped data and waits for it to render. */
	onPreloadData: (scope: ExportTab) => Promise<void>;
	onPdfExport: (scope: ExportTab) => Promise<{ element: HTMLElement; label: string }[]>;
	/** Leaves export mode; both flows must call this, since both enter it via onPreloadData. */
	onExportDone: () => void;
}

export function ExportPopover({ getData, activeTab, onPreloadData, onPdfExport, onExportDone }: ExportPopoverProps) {
	const { t } = useTranslation("dashboard");
	const [exporting, setExporting] = useState(false);

	const fileName = useCallback((scope: ExportTab) => (scope === "all" ? "dashboard-export" : `dashboard-${scope}`), []);

	const handleCsvExport = useCallback(
		async (scope: ExportTab) => {
			setExporting(true);
			try {
				await onPreloadData(scope);
				const sections = getCSVSections(getData(), scope);
				const parts: string[] = [];
				for (const section of sections) {
					if (section.csv.rows.length === 0) continue;
					parts.push(`# ${section.name}`);
					parts.push(buildCSV(section.csv.headers, section.csv.rows));
					parts.push("");
				}
				if (parts.length > 0) {
					downloadCSV(parts.join("\n"), fileName(scope));
				}
			} finally {
				onExportDone();
				setExporting(false);
			}
		},
		[getData, onPreloadData, onExportDone, fileName],
	);

	const handlePdfExport = useCallback(
		async (scope: ExportTab) => {
			setExporting(true);

			// Yield a frame so the spinner renders before heavy work starts
			await new Promise((r) => requestAnimationFrame(r));

			try {
				const { generatePdf } = await import("@/lib/utils/pdf");

				const sections = await onPdfExport(scope);

				await generatePdf(sections, fileName(scope), {
					branding: {
						logoSrc: "/pg-gateway-logo.webp",
						text: t("export.poweredBy"),
					},
				});
			} finally {
				onExportDone();
				setExporting(false);
			}
		},
		[onPdfExport, onExportDone, fileName],
	);

	const tabKey =
		activeTab === "overview"
			? "overview"
			: activeTab === "provider-usage"
				? "providerUsage"
				: activeTab === "rankings"
					? "modelRankings"
					: activeTab === "virtual-key-rankings"
						? "virtualKeyRankings"
						: "appRankings";
	const activeTabLabel = t("tabs." + tabKey);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline" size="default" disabled={exporting} data-testid="dashboard-export-trigger">
					{exporting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}
					{exporting ? t("export.exporting") : t("export.title")}
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuSub>
					<DropdownMenuSubTrigger data-testid="export-csv-item" className="flex gap-2">
						<FileSpreadsheet className="h-4 w-4" />
						{t("export.csv")}
					</DropdownMenuSubTrigger>
					<DropdownMenuPortal>
						<DropdownMenuSubContent>
							<DropdownMenuItem onClick={() => handleCsvExport(activeTab)} data-testid="export-csv-current-tab">
								{t("export.thisTab", { tab: activeTabLabel })}
							</DropdownMenuItem>
							<DropdownMenuItem onClick={() => handleCsvExport("all")} data-testid="export-csv-all-tabs">
								{t("export.allTabs")}
							</DropdownMenuItem>
						</DropdownMenuSubContent>
					</DropdownMenuPortal>
				</DropdownMenuSub>
				<DropdownMenuSub>
					<DropdownMenuSubTrigger data-testid="export-pdf-item" className="flex gap-2">
						<FileText className="h-4 w-4" />
						{t("export.pdf")}
					</DropdownMenuSubTrigger>
					<DropdownMenuPortal>
						<DropdownMenuSubContent>
							<DropdownMenuItem onClick={() => handlePdfExport(activeTab)} data-testid="export-pdf-current-tab">
								{t("export.thisTab", { tab: activeTabLabel })}
							</DropdownMenuItem>
							<DropdownMenuItem onClick={() => handlePdfExport("all")} data-testid="export-pdf-all-tabs">
								{t("export.allTabs")}
							</DropdownMenuItem>
						</DropdownMenuSubContent>
					</DropdownMenuPortal>
				</DropdownMenuSub>
			</DropdownMenuContent>
		</DropdownMenu>
	);
}
/**
 * @file Standalone log detail page — opened in a new browser window from the
 * LLM Logs and Request Timeline pages. Renders the same LogDetailView body that
 * the in-tab drawer used to, but without the Sheet wrapper, sidebar, or any
 * other workspace chrome.
 */

import { LogDetailView } from "@/app/workspace/logs/sheets/logDetailView";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useGetLogByIdQuery } from "@/lib/store/apis/logsApi";
import { useGetPromptQuery } from "@/lib/store/apis/promptsApi";
import type { LogEntry } from "@/lib/types/logs";
import { getErrorMessage, useDeleteLogsMutation } from "@/lib/store";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { AlertCircle, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Route } from "./layout";

function safeClose() {
	try {
		window.close();
	} catch {
		// window.close() can throw in some embedded contexts; ignore.
	}
}

export default function LogDetailPage() {
	const { id } = Route.useParams();
	const { t } = useTranslation("logs");
	const hasDeleteAccess = useRbac(RbacResource.Logs, RbacOperation.Delete);
	const hasRevealAccess = useRbac(RbacResource.Logs, RbacOperation.Reveal);
	const [deleteLogs] = useDeleteLogsMutation();
	const [error, setError] = useState<string | null>(null);

	const [pollingInterval, setPollingInterval] = useState(0);
	const {
		data: fullLog,
		isLoading,
		isError,
	} = useGetLogByIdQuery(id ?? "", {
		skip: !id,
		pollingInterval,
	});

	const shouldPoll = isError || fullLog?.status === "processing";
	useEffect(() => {
		setPollingInterval(shouldPoll ? 2000 : 0);
	}, [shouldPoll]);

	// Show a loader only on the initial fetch, not during background polling refetches.
	const isFullDataReady = isError || (fullLog?.id === id && !isLoading);
	const displayLog: LogEntry | null = isFullDataReady && fullLog ? fullLog : null;

	const { data: selectedPromptData } = useGetPromptQuery(displayLog?.selected_prompt_id ?? "", {
		skip: !displayLog?.selected_prompt_id,
	});
	const resolvedSelectedPromptName = selectedPromptData?.prompt?.name ?? displayLog?.selected_prompt_name ?? "";

	const handleDelete = async (log: LogEntry) => {
		try {
			await deleteLogs({ ids: [log.id] }).unwrap();
			safeClose();
		} catch (err) {
			setError(getErrorMessage(err));
		}
	};

	return (
		<div className="bg-background flex h-screen w-full flex-col overflow-hidden">
			<div className="border-border flex shrink-0 items-center justify-between gap-3 border-b px-6 py-3">
				<div className="flex min-w-0 items-center gap-2">
					<h1 className="text-foreground truncate text-sm font-medium">{t("detailPage.title")}</h1>
					{isError ? (
						<span className="text-muted-foreground text-xs">{t("detailPage.loadError")}</span>
					) : displayLog ? (
						<span className="text-muted-foreground truncate font-mono text-xs">{displayLog.id}</span>
					) : null}
				</div>
				<Button variant="outline" size="sm" onClick={safeClose} data-testid="logdetail-page-close-button">
					{t("detailPage.closeWindow")}
				</Button>
			</div>
			<div className="custom-scrollbar min-h-0 flex-1 overflow-auto">
				<div className="mx-auto w-full max-w-5xl space-y-4 p-6">
					{error ? (
						<Alert variant="destructive" className="shrink-0">
							<AlertCircle className="h-4 w-4" />
							<AlertDescription>{error}</AlertDescription>
						</Alert>
					) : null}
					{!isFullDataReady ? (
						<div className="flex h-[60vh] items-center justify-center">
							<Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
						</div>
					) : displayLog ? (
						<LogDetailView
							log={displayLog}
							resolvedSelectedPromptName={resolvedSelectedPromptName}
							handleDelete={hasDeleteAccess ? handleDelete : undefined}
							canReveal={hasRevealAccess}
							onClose={safeClose}
						/>
					) : (
						<div className="text-muted-foreground flex h-[60vh] items-center justify-center text-sm">{t("detailPage.notFound")}</div>
					)}
				</div>
			</div>
		</div>
	);
}
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useLazyGetRtkRawOutputQuery } from "@/lib/store";
import { isValidRawOutputID } from "@/lib/types/rtk";
import { useEffect, useState } from "react";
import { HelpCircle, Search } from "lucide-react";

// /workspace/plugins/rtk/raw-output — recovery viewer for persisted raw outputs.
//
// Reads ?id=<24-hex> from the URL or accepts input via a search box. When
// the plugin has raw_output_retention != "never" and a request actually
// compressed a payload, the response's raw_output_ptr.id can be opened
// here to inspect the post-redaction text without leaving the admin UI.

function RouteComponent() {
	const { t } = useTranslation();
	const navigate = useNavigate({ from: "/workspace/plugins/rtk/raw-output" });
	const search = (location.search as { id?: string })?.id ?? "";
	const [draft, setDraft] = useState(search);
	const [trigger, { data, isFetching, isError, error }] = useLazyGetRtkRawOutputQuery();

	useEffect(() => {
		if (search && isValidRawOutputID(search)) {
			trigger(search);
		}
	}, [search, trigger]);

	const handleSearch = () => {
		const trimmed = draft.trim();
		if (!trimmed) return;
		navigate({ search: { id: trimmed }, replace: true });
	};

	return (
		<div className="flex flex-col gap-6" data-testid="rtk-raw-output-page">
			<Card>
				<CardHeader>
					<div className="flex items-center gap-2">
						<HelpCircle className="text-muted-foreground h-4 w-4" />
						<CardTitle className="text-sm font-medium">{t("plugins:rtk.rawOutput.helpTitle")}</CardTitle>
					</div>
				</CardHeader>
				<CardContent>
					<ol className="text-muted-foreground ml-4 list-decimal space-y-1 text-sm">
						<li>{t("plugins:rtk.rawOutput.helpStep1")}</li>
						<li>{t("plugins:rtk.rawOutput.helpStep2")}</li>
						<li>{t("plugins:rtk.rawOutput.helpStep3")}</li>
						<li>{t("plugins:rtk.rawOutput.helpStep4")}</li>
					</ol>
				</CardContent>
			</Card>
			<Card>
				<CardHeader>
					<CardTitle>{t("plugins:rtk.rawOutput.title")}</CardTitle>
					<CardDescription>{t("plugins:rtk.rawOutput.subtitle")}</CardDescription>
				</CardHeader>
				<CardContent className="flex flex-col gap-3">
					<div className="flex flex-wrap items-center gap-2">
						<Input
							data-testid="rtk-raw-output-id"
							className="max-w-xs font-mono"
							value={draft}
							onChange={(e) => setDraft(e.target.value)}
							placeholder="0123456789abcdef01234567"
						/>
						<Button data-testid="rtk-raw-output-fetch" onClick={handleSearch} disabled={!draft.trim() || isFetching}>
							<Search className="h-4 w-4" />
							{t("plugins:rtk.rawOutput.fetch")}
						</Button>
					</div>
					{!isValidRawOutputID(search) && search !== "" && (
						<Alert variant="destructive">
							<AlertDescription>{t("plugins:rtk.rawOutput.invalidId")}</AlertDescription>
						</Alert>
					)}
					{isError && (
						<Alert variant="destructive">
							<AlertDescription>
								{(error as { data?: { error?: { message?: string } } })?.data?.error?.message ?? t("plugins:rtk.rawOutput.notFound")}
							</AlertDescription>
						</Alert>
					)}
					{data !== undefined && (
						<section>
							<h3 className="mb-2 text-sm font-medium">{t("plugins:rtk.rawOutput.content")}</h3>
							<pre data-testid="rtk-raw-output-body" className="bg-muted max-h-[60vh] overflow-auto rounded-md p-3 text-xs">
								{data || t("plugins:rtk.rawOutput.empty")}
							</pre>
						</section>
					)}
				</CardContent>
			</Card>
		</div>
	);
}

export const Route = createFileRoute("/workspace/plugins/rtk/raw-output")({
	component: RouteComponent,
});
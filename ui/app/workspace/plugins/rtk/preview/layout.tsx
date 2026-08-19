import { createFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { usePreviewCompressionMutation } from "@/lib/store";
import type { CompressionMode, PreviewResponse } from "@/lib/types/rtk";
import { Loader2 } from "lucide-react";

// /workspace/plugins/rtk/preview — preview playground.
//
// Picks a mode (rtk | stacked | off), runs /api/compression/preview, and
// renders the original vs compressed text. The page is the most direct
// way for an operator to confirm that a configuration change does what
// they expect before flipping it on for live traffic.

const DEFAULT_OUTPUT = `On branch main
Your branch is up to date with 'origin/main'.

Changes not staged for commit:
  modified:   plugins/rtk/admin.go
  modified:   plugins/rtk/rtk.go

no changes added to commit (use "git add" and/or "git commit -a")
`;

function RouteComponent() {
	const { t } = useTranslation();
	const [mode, setMode] = useState<CompressionMode>("rtk");
	const [intensity, setIntensity] = useState("");
	const [command, setCommand] = useState("git status");
	const [output, setOutput] = useState(DEFAULT_OUTPUT);
	const [result, setResult] = useState<PreviewResponse | null>(null);
	const [error, setError] = useState<string | null>(null);
	const [run, { isLoading }] = usePreviewCompressionMutation();

	const submit = async () => {
		setError(null);
		try {
			const r = await run({
				mode,
				intensity: intensity || undefined,
				payload: { command, output, apply_rules: true },
			}).unwrap();
			setResult(r);
		} catch (err) {
			setError((err as { message?: string })?.message ?? "unknown error");
		}
	};

	return (
		<div className="flex flex-col gap-6" data-testid="rtk-preview-page">
			<Card>
				<CardHeader>
					<CardTitle>{t("plugins:rtk.preview.title")}</CardTitle>
					<CardDescription>{t("plugins:rtk.preview.subtitle")}</CardDescription>
				</CardHeader>
				<CardContent className="flex flex-col gap-4">
					<div className="grid grid-cols-1 gap-3 md:grid-cols-3">
						<div className="flex flex-col gap-2">
							<Label htmlFor="rtk-preview-mode">{t("plugins:rtk.preview.modeLabel")}</Label>
							<Select value={mode} onValueChange={(v) => setMode(v as CompressionMode)}>
								<SelectTrigger id="rtk-preview-mode" data-testid="rtk-preview-mode">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="rtk">{t("plugins:rtk.preview.modeRtk")}</SelectItem>
									<SelectItem value="stacked">{t("plugins:rtk.preview.modeStacked")}</SelectItem>
									<SelectItem value="off">{t("plugins:rtk.preview.modeOff")}</SelectItem>
								</SelectContent>
							</Select>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="rtk-preview-intensity">{t("plugins:rtk.preview.intensityLabel")}</Label>
							<Select value={intensity} onValueChange={setIntensity}>
								<SelectTrigger id="rtk-preview-intensity" data-testid="rtk-preview-intensity">
									<SelectValue placeholder={t("plugins:rtk.preview.intensityCurrent")} />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="minimal">minimal</SelectItem>
									<SelectItem value="standard">standard</SelectItem>
									<SelectItem value="aggressive">aggressive</SelectItem>
								</SelectContent>
							</Select>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="rtk-preview-command">{t("plugins:rtk.test.commandLabel")}</Label>
							<Input
								id="rtk-preview-command"
								data-testid="rtk-preview-command"
								value={command}
								onChange={(e) => setCommand(e.target.value)}
							/>
						</div>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="rtk-preview-output">{t("plugins:rtk.test.outputLabel")}</Label>
						<Textarea
							id="rtk-preview-output"
							data-testid="rtk-preview-output"
							rows={12}
							value={output}
							onChange={(e) => setOutput(e.target.value)}
						/>
					</div>
					<div className="flex justify-end">
						<Button data-testid="rtk-preview-submit" onClick={submit} disabled={isLoading || !output.trim()}>
							{isLoading && <Loader2 className="h-4 w-4 animate-spin" />}
							{t("plugins:rtk.preview.submit")}
						</Button>
					</div>
					{error && <div className="text-destructive text-sm">{error}</div>}
				</CardContent>
			</Card>

			{result && (
				<Card data-testid="rtk-preview-result">
					<CardHeader>
						<CardTitle>{t("plugins:rtk.preview.resultTitle", { mode: result.mode })}</CardTitle>
						<CardDescription>
							{t("plugins:rtk.test.resultStats", {
								original: result.result.original_tokens,
								compressed: result.result.compressed_tokens,
								ratio: (result.result.compression_ratio * 100).toFixed(1),
							})}
						</CardDescription>
					</CardHeader>
					<CardContent className="flex flex-col gap-4">
						<div className="flex flex-wrap items-center gap-2">
							{result.engines_planned?.map((id) => (
								<Badge key={id} variant="secondary">
									{id}
								</Badge>
							))}
							{result.result.filter_matched && (
								<Badge variant="outline">{t("plugins:rtk.test.filterMatched", { name: result.result.filter_matched })}</Badge>
							)}
						</div>
						{result.engine_stats && result.engine_stats.length > 0 && (
							<section>
								<h3 className="mb-2 text-sm font-medium">{t("plugins:rtk.preview.engineStats")}</h3>
								<ul className="space-y-1 text-xs">
									{result.engine_stats.map((entry) => (
										<li key={entry.id} className="font-mono">
											{entry.id}: {(entry.compressed_by * 100).toFixed(1)}% ({entry.input_bytes} → {entry.output_bytes} bytes)
										</li>
									))}
								</ul>
							</section>
						)}
						<div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
							<section>
								<h3 className="mb-2 text-sm font-medium">{t("plugins:rtk.test.original")}</h3>
								<pre className="bg-muted max-h-96 overflow-auto rounded-md p-3 text-xs">{result.result.original_text}</pre>
							</section>
							<section>
								<h3 className="mb-2 text-sm font-medium">{t("plugins:rtk.test.compressed")}</h3>
								<pre data-testid="rtk-preview-compressed" className="bg-muted max-h-96 overflow-auto rounded-md p-3 text-xs">
									{result.result.compressed_text}
								</pre>
							</section>
						</div>
					</CardContent>
				</Card>
			)}
		</div>
	);
}

export const Route = createFileRoute("/workspace/plugins/rtk/preview")({
	component: RouteComponent,
});
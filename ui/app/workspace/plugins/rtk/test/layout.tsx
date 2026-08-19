import { createFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { useRunRtkTestMutation } from "@/lib/store";
import type { TestResult } from "@/lib/types/rtk";
import { Loader2 } from "lucide-react";

// /workspace/plugins/rtk/test — compression test runner.
//
// Sends a payload to POST /api/context/rtk/test and renders the compressed
// text alongside per-stage stats (techniques, filter matched, token counts).
// Mirrors OmniRoute's /api/context/rtk/test admin page.

const DEFAULT_OUTPUT = `On branch main
Your branch is up to date with 'origin/main'.

Changes not staged for commit:
  modified:   plugins/rtk/admin.go
  modified:   plugins/rtk/rtk.go

no changes added to commit (use "git add" and/or "git commit -a")
`;

function RouteComponent() {
	const { t } = useTranslation();
	const [command, setCommand] = useState("git status");
	const [output, setOutput] = useState(DEFAULT_OUTPUT);
	const [applyRules, setApplyRules] = useState(true);
	const [lastResult, setLastResult] = useState<TestResult | null>(null);
	const [error, setError] = useState<string | null>(null);
	const [runTest, { isLoading }] = useRunRtkTestMutation();

	const submit = async () => {
		setError(null);
		try {
			const result = await runTest({
				command,
				output,
				apply_rules: applyRules,
			}).unwrap();
			setLastResult(result);
		} catch (err) {
			setError((err as { message?: string })?.message ?? "unknown error");
		}
	};

	return (
		<div className="flex flex-col gap-6" data-testid="rtk-test-page">
			<Card>
				<CardHeader>
					<CardTitle>{t("plugins:rtk.test.title")}</CardTitle>
					<CardDescription>{t("plugins:rtk.test.subtitle")}</CardDescription>
				</CardHeader>
				<CardContent className="flex flex-col gap-4">
					<div className="flex flex-col gap-2">
						<Label htmlFor="rtk-test-command">{t("plugins:rtk.test.commandLabel")}</Label>
						<Input
							id="rtk-test-command"
							data-testid="rtk-test-command"
							value={command}
							onChange={(e) => setCommand(e.target.value)}
							placeholder="git status"
						/>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="rtk-test-output">{t("plugins:rtk.test.outputLabel")}</Label>
						<Textarea
							id="rtk-test-output"
							data-testid="rtk-test-output"
							rows={12}
							value={output}
							onChange={(e) => setOutput(e.target.value)}
						/>
					</div>
					<div className="flex items-center justify-between gap-3">
						<div className="flex items-center gap-2">
							<Switch id="rtk-test-apply-rules" data-testid="rtk-test-apply-rules" checked={applyRules} onCheckedChange={setApplyRules} />
							<Label htmlFor="rtk-test-apply-rules">{t("plugins:rtk.test.applyRulesLabel")}</Label>
						</div>
						<Button data-testid="rtk-test-submit" onClick={submit} disabled={isLoading || !output.trim()}>
							{isLoading && <Loader2 className="h-4 w-4 animate-spin" />}
							{t("plugins:rtk.test.submit")}
						</Button>
					</div>
					{error && <div className="text-destructive text-sm">{error}</div>}
				</CardContent>
			</Card>

			{lastResult && (
				<Card data-testid="rtk-test-result">
					<CardHeader>
						<CardTitle>{t("plugins:rtk.test.resultTitle")}</CardTitle>
						<CardDescription>
							{t("plugins:rtk.test.resultStats", {
								original: lastResult.original_tokens,
								compressed: lastResult.compressed_tokens,
								ratio: (lastResult.compression_ratio * 100).toFixed(1),
							})}
						</CardDescription>
					</CardHeader>
					<CardContent className="flex flex-col gap-4">
						<div className="flex flex-wrap items-center gap-2">
							{lastResult.filter_matched ? (
								<Badge variant="secondary" data-testid="rtk-test-filter-matched">
									{t("plugins:rtk.test.filterMatched", { name: lastResult.filter_matched })}
								</Badge>
							) : (
								<Badge variant="outline">{t("plugins:rtk.test.filterMatchedNone")}</Badge>
							)}
							{lastResult.techniques.map((tech) => (
								<Badge key={tech} variant="outline">
									{tech}
								</Badge>
							))}
						</div>
						{lastResult.raw_output_ptr && (
							<div className="text-muted-foreground text-xs">
								{t("plugins:rtk.test.rawOutputHint", { id: lastResult.raw_output_ptr.id })}
							</div>
						)}
						<div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
							<section>
								<h3 className="mb-2 text-sm font-medium">{t("plugins:rtk.test.original")}</h3>
								<pre data-testid="rtk-test-original" className="bg-muted max-h-96 overflow-auto rounded-md p-3 text-xs">
									{lastResult.original_text}
								</pre>
							</section>
							<section>
								<h3 className="mb-2 text-sm font-medium">{t("plugins:rtk.test.compressed")}</h3>
								<pre data-testid="rtk-test-compressed" className="bg-muted max-h-96 overflow-auto rounded-md p-3 text-xs">
									{lastResult.compressed_text}
								</pre>
							</section>
						</div>
					</CardContent>
				</Card>
			)}
		</div>
	);
}

export const Route = createFileRoute("/workspace/plugins/rtk/test")({
	component: RouteComponent,
});
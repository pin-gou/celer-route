import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { getErrorMessage, useMemberMeQuery, useMemberSetupGuideQuery } from "@/lib/store";
import type { MemberSetupGuideResponse } from "@/lib/store/apis/sessionApi";
import { BookOpen, ClipboardCopy, ExternalLink, RefreshCw, Terminal, Wifi } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

function CodeBlock({ value, testID }: { value: string; testID?: string }) {
	const { copy } = useCopyToClipboard({ successMessage: "Copied", toastOnSuccess: false });
	return (
		<div className="group relative">
			<pre className="bg-muted text-foreground overflow-x-auto rounded-md p-3 font-mono text-xs leading-relaxed" data-testid={testID}>
				{value}
			</pre>
			<Button
				type="button"
				variant="ghost"
				size="icon"
				className="absolute top-1 right-1 h-7 w-7 opacity-0 transition-opacity group-hover:opacity-100"
				onClick={() => copy(value)}
				data-testid={testID ? `${testID}-copy` : undefined}
				aria-label="Copy"
			>
				<ClipboardCopy className="h-3.5 w-3.5" />
			</Button>
		</div>
	);
}

// Member-self setup guide. Surface mirrors /api/member/setup-guide
// exactly (handlers/memberportal.go §setupGuide). The endpoint never
// returns provider credentials; only the OpenAI-compatible base URL
// + the caller's active VKs (id + name). For each VK we render a
// pre-filled curl example so the member can drop their value in and
// paste it.
export default function SetupGuideView() {
	const { t } = useTranslation("governance-ui");
	const { copy } = useCopyToClipboard({ toastOnSuccess: false });
	const { data, isLoading, isError, error, refetch, isFetching } = useMemberSetupGuideQuery();
	const me = useMemberMeQuery();

	const onCopyBaseURL = (baseURL: string) => {
		copy(baseURL);
		toast.success(t("users.memberSetupGuide.baseUrlCopied"));
	};

	return (
		<div className="mx-auto max-w-4xl space-y-6 p-8" data-testid="member-setup-guide">
			<header className="flex flex-wrap items-end justify-between gap-3">
				<div>
					<h1 className="text-foreground flex items-center gap-2 text-2xl font-semibold" data-testid="member-setup-guide-title">
						<BookOpen className="h-6 w-6" />
						{t("users.memberSetupGuide.title")}
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">{t("users.memberSetupGuide.subtitle")}</p>
				</div>
				<Button variant="outline" onClick={() => refetch()} isLoading={isFetching} data-testid="member-setup-guide-refresh">
					<RefreshCw className="mr-2 h-4 w-4" />
					{t("users.memberPortal.retry")}
				</Button>
			</header>

			{isLoading ? (
				<div className="flex min-h-[20vh] items-center justify-center" data-testid="member-setup-guide-loading">
					<RefreshCw className="text-muted-foreground h-5 w-5 animate-spin" />
				</div>
			) : isError || !data ? (
				<Card>
					<CardHeader>
						<CardTitle>{t("users.memberPortal.loadFailed")}</CardTitle>
						<CardDescription>{getErrorMessage(error)}</CardDescription>
					</CardHeader>
				</Card>
			) : (
				<>
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2 text-base">
								<Wifi className="h-4 w-4" />
								{t("users.memberSetupGuide.step1Title")}
							</CardTitle>
							<CardDescription>{t("users.memberSetupGuide.step1Desc")}</CardDescription>
						</CardHeader>
						<CardContent className="space-y-2">
							<div className="flex items-center gap-2">
								<code
									className="bg-muted text-foreground flex-1 rounded-md px-3 py-2 font-mono text-sm"
									data-testid="member-setup-guide-base-url"
								>
									{data.base_url}
								</code>
								<Button
									variant="outline"
									size="sm"
									onClick={() => onCopyBaseURL(data.base_url)}
									data-testid="member-setup-guide-base-url-copy"
								>
									<ClipboardCopy className="mr-2 h-4 w-4" />
									{t("users.memberSetupGuide.copyBaseUrl")}
								</Button>
							</div>
							<p className="text-muted-foreground text-xs">
								<Badge variant="secondary" className="mr-1">
									{data.compatible_with}
								</Badge>
								{t("users.memberSetupGuide.compatibleHint")}
							</p>
						</CardContent>
					</Card>

					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2 text-base">
								<Terminal className="h-4 w-4" />
								{t("users.memberSetupGuide.step2Title")}
							</CardTitle>
							<CardDescription>{t("users.memberSetupGuide.step2Desc")}</CardDescription>
						</CardHeader>
						<CardContent>
							{data.available_keys.length === 0 ? (
								<p className="text-muted-foreground text-sm" data-testid="member-setup-guide-no-keys">
									{t("users.memberSetupGuide.noKeys")}
								</p>
							) : (
								<ul className="space-y-2" data-testid="member-setup-guide-keys">
									{data.available_keys.map((vk) => (
										<li
											key={vk.id}
											className="bg-muted/50 flex items-center justify-between gap-3 rounded-md px-3 py-2"
											data-testid={`member-setup-guide-key-${vk.id}`}
										>
											<div className="min-w-0 flex-1">
												<p className="truncate font-medium">{vk.name}</p>
												<p className="text-muted-foreground truncate font-mono text-xs">{vk.id}</p>
											</div>
											<Badge variant="outline">{t("users.memberSetupGuide.vkBadge")}</Badge>
										</li>
									))}
								</ul>
							)}
						</CardContent>
					</Card>

					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2 text-base">
								<Terminal className="h-4 w-4" />
								{t("users.memberSetupGuide.step3Title")}
							</CardTitle>
							<CardDescription>{t("users.memberSetupGuide.step3Desc")}</CardDescription>
						</CardHeader>
						<CardContent>
							<CodeBlock value={data.example_request} testID="member-setup-guide-example" />
						</CardContent>
					</Card>

					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2 text-base">
								<BookOpen className="h-4 w-4" />
								{t("users.memberSetupGuide.docsTitle")}
							</CardTitle>
							<CardDescription>{t("users.memberSetupGuide.docsDesc")}</CardDescription>
						</CardHeader>
						<CardContent>
							<Button asChild variant="outline" data-testid="member-setup-guide-docs-link">
								<a href={data.docs} target="_blank" rel="noopener noreferrer">
									<ExternalLink className="mr-2 h-4 w-4" />
									{data.docs}
								</a>
							</Button>
						</CardContent>
					</Card>

					{me.data?.is_admin ? <p className="text-muted-foreground text-xs">{t("users.memberSetupGuide.adminHint")}</p> : null}
				</>
			)}
		</div>
	);
}

// Export the type so tests can narrow against the real wire shape
// without re-typing it.
export type { MemberSetupGuideResponse };
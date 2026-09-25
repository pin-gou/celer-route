import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
	getErrorMessage,
	useDeleteTeamModelPolicyMutation,
	useListTeamModelPoliciesQuery,
	useUpsertTeamModelPolicyMutation,
} from "@/lib/store";
import type { TeamModelPolicy, UpsertTeamModelPolicyRequest } from "@/lib/store/apis/teamModelPoliciesApi";
import { Plus, Save, ShieldCheck, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

const ALL_PROVIDERS = [
	"openai",
	"anthropic",
	"azure",
	"bedrock",
	"gemini",
	"vertex",
	"groq",
	"deepseek",
	"fireworks",
	"mistral",
	"ollama",
	"openrouter",
	"perplexity",
	"xai",
	"cohere",
	"sarvam",
	"elevenlabs",
	"runware",
	"runway",
	"vllm",
];

interface TeamModelPoliciesSectionProps {
	teamID: string;
}

// TeamModelPoliciesSection — the per-team model ACL editor (Phase 6 /
// D6). One row per provider; the table is the editing surface for both
// the existing rows (allowed_models + blacklisted_models edit inline) and
// the "add rule" flow (pick a provider not already in the table).
//
// D6 兜底 (plan.md §7 / model-acl.md §D6):
//   the resolver composes effective_allowed = VK.AllowedModels ∩
//   TeamPolicies.AllowedModels. The backend treats an empty
//   allowed_models list as deny-by-default (same as VK provider config),
//   so a rule with only blacklisted_models populated will silently
//   *forbid every model of that provider* — the opposite of what the
//   admin almost certainly meant. The submit handler detects this shape
//   and either:
//     1) prepends ["*"] to allowed_models with a one-line toast, OR
//     2) lets the admin confirm via the persistent hint banner.
//
// We picked option (1) because plan.md §7 picked it (no admin flow
// disruption); the banner is the "you can change your mind" affordance.
export default function TeamModelPoliciesSection({ teamID }: TeamModelPoliciesSectionProps) {
	const { t } = useTranslation("governance-ui");

	const { data: policies = [], isLoading, error, refetch } = useListTeamModelPoliciesQuery(teamID);
	const [upsertPolicy] = useUpsertTeamModelPolicyMutation();
	const [deletePolicy] = useDeleteTeamModelPolicyMutation();

	const [editing, setEditing] = useState<Record<string, { allowed: string; blacklisted: string }>>({});
	const [newProvider, setNewProvider] = useState<string>("");

	const orderedProviders = useMemo(() => {
		const configured = new Set(policies.map((p) => p.provider));
		const candidates = ALL_PROVIDERS.filter((p) => !configured.has(p));
		return { configured: policies.map((p) => p.provider), candidates };
	}, [policies]);

	const getRowState = (p: TeamModelPolicy) => {
		const inEdit = editing[p.id];
		return {
			allowed: inEdit?.allowed ?? (p.allowed_models ?? []).join(","),
			blacklisted: inEdit?.blacklisted ?? (p.blacklisted_models ?? []).join(","),
		};
	};

	const onSave = async (p: TeamModelPolicy) => {
		const state = getRowState(p);
		const body = parseCsv(state.allowed, state.blacklisted);
		await submit(teamID, p.provider, body);
		setEditing((e) => {
			const next = { ...e };
			delete next[p.id];
			return next;
		});
	};

	const onDelete = async (p: TeamModelPolicy) => {
		try {
			await deletePolicy({ teamID, provider: p.provider }).unwrap();
			toast.success(t("teams.teamPolicies.deleteSuccess", { provider: p.provider }));
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	const onCreate = async () => {
		if (!newProvider) return;
		const body: UpsertTeamModelPolicyRequest = { allowed_models: ["*"], blacklisted_models: [] };
		const ok = await submit(teamID, newProvider, body, { silent: false });
		if (ok) setNewProvider("");
	};

	const submit = async (team: string, provider: string, body: UpsertTeamModelPolicyRequest, opts: { silent?: boolean } = {}) => {
		const safeBody = d6Guard(body, opts.silent !== false ? toast : null);
		try {
			await upsertPolicy({ teamID: team, provider, body: safeBody }).unwrap();
			if (!opts.silent) {
				toast.success(t("teams.teamPolicies.saveSuccess", { provider }));
			}
			refetch();
			return true;
		} catch (e) {
			toast.error(getErrorMessage(e));
			return false;
		}
	};

	return (
		<section data-testid="team-model-policies">
			<header className="mb-2 flex items-center gap-2">
				<ShieldCheck className="h-4 w-4" />
				<h2 className="text-lg font-semibold">{t("teams.teamPolicies.title")}</h2>
				<span className="text-muted-foreground text-sm">({policies.length})</span>
			</header>
			<Alert className="mb-3" data-testid="team-model-policies-hint">
				<AlertDescription className="text-xs">{t("teams.teamPolicies.d6Hint")}</AlertDescription>
			</Alert>
			<Card>
				<CardHeader className="pb-2">
					<CardTitle className="text-sm">{t("teams.teamPolicies.subtitle")}</CardTitle>
					<CardDescription>{t("teams.teamPolicies.subtitleDesc")}</CardDescription>
				</CardHeader>
				<CardContent>
					{error ? (
						<p className="text-destructive text-sm">{getErrorMessage(error)}</p>
					) : isLoading ? (
						<p className="text-muted-foreground text-sm">{t("loading")}</p>
					) : policies.length === 0 ? (
						<p className="text-muted-foreground text-sm" data-testid="team-model-policies-empty">
							{t("teams.teamPolicies.empty")}
						</p>
					) : (
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead className="w-32">{t("teams.teamPolicies.colProvider")}</TableHead>
									<TableHead>{t("teams.teamPolicies.colAllowed")}</TableHead>
									<TableHead>{t("teams.teamPolicies.colBlacklisted")}</TableHead>
									<TableHead className="w-32 text-right">{t("col_actions")}</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{policies.map((p) => {
									const state = getRowState(p);
									const dirty = editing[p.id] !== undefined;
									return (
										<TableRow key={p.id} data-testid={`team-policy-row-${p.provider}`}>
											<TableCell className="font-mono text-xs">{p.provider}</TableCell>
											<TableCell>
												<textarea
													className="border-input bg-background min-h-[60px] w-full rounded-sm border px-2 py-1 font-mono text-xs"
													value={state.allowed}
													placeholder={t("teams.teamPolicies.allowedPlaceholder")}
													onChange={(e) =>
														setEditing((s) => ({
															...s,
															[p.id]: { ...state, allowed: e.target.value },
														}))
													}
													data-testid={`team-policy-allowed-${p.provider}`}
												/>
											</TableCell>
											<TableCell>
												<textarea
													className="border-input bg-background min-h-[60px] w-full rounded-sm border px-2 py-1 font-mono text-xs"
													value={state.blacklisted}
													placeholder={t("teams.teamPolicies.blacklistedPlaceholder")}
													onChange={(e) =>
														setEditing((s) => ({
															...s,
															[p.id]: { ...state, blacklisted: e.target.value },
														}))
													}
													data-testid={`team-policy-blacklisted-${p.provider}`}
												/>
											</TableCell>
											<TableCell className="text-right">
												<div className="flex items-center justify-end gap-1">
													<Button
														variant="ghost"
														size="icon"
														disabled={!dirty}
														onClick={() => onSave(p)}
														data-testid={`team-policy-save-${p.provider}`}
													>
														<Save className="h-4 w-4" />
													</Button>
													<Button variant="ghost" size="icon" onClick={() => onDelete(p)} data-testid={`team-policy-delete-${p.provider}`}>
														<Trash2 className="h-4 w-4" />
													</Button>
												</div>
											</TableCell>
										</TableRow>
									);
								})}
							</TableBody>
						</Table>
					)}

					{orderedProviders.candidates.length > 0 && (
						<div className="mt-4 flex items-end gap-2" data-testid="team-policy-add">
							<div className="flex-1">
								<Select value={newProvider} onValueChange={setNewProvider}>
									<SelectTrigger data-testid="team-policy-add-provider">
										<SelectValue placeholder={t("teams.teamPolicies.addPlaceholder")} />
									</SelectTrigger>
									<SelectContent>
										{orderedProviders.candidates.map((p) => (
											<SelectItem key={p} value={p}>
												<Badge variant="outline">{p}</Badge>
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
							<Button onClick={onCreate} disabled={!newProvider} data-testid="team-policy-add-submit">
								<Plus className="mr-1 h-4 w-4" /> {t("teams.teamPolicies.addRule")}
							</Button>
						</div>
					)}
				</CardContent>
			</Card>
		</section>
	);
}

// parseCsv — convert the comma-separated textarea contents into the
// normalized JSON-array shape the backend stores. Empty strings and
// whitespace-only entries are dropped so admins can leave the box blank
// rather than typing `[]`. Duplicates are removed (case-insensitive) so
// the stored shape stays canonical.
//
// Exported for vitest at ui/src/__tests__/components/governance/teamModelPoliciesSection.test.tsx
export function parseCsv(allowed: string, blacklisted: string): UpsertTeamModelPolicyRequest {
	const split = (s: string) =>
		s
			.split(/[\n,]/)
			.map((x) => x.trim())
			.filter(Boolean);
	const uniq = (xs: string[]) => {
		const seen = new Set<string>();
		const out: string[] = [];
		for (const x of xs) {
			const k = x.toLowerCase();
			if (!seen.has(k)) {
				seen.add(k);
				out.push(x);
			}
		}
		return out;
	};
	return {
		allowed_models: uniq(split(allowed)),
		blacklisted_models: uniq(split(blacklisted)),
	};
}

// d6Guard — implements the Batch C §7 UI-兜底 contract. If the admin
// submits a rule with non-empty blacklisted_models but empty
// allowed_models, the resolver (effective_allowed = VK.AllowedModels ∩
// TeamPolicies.AllowedModels) would deny every model of that provider.
// We auto-fill allowed_models: ["*"] so the intent ("blacklist a few
// specific models") survives. The toast tells the admin so it's not
// silent.
//
// Exported for vitest at ui/src/__tests__/components/governance/teamModelPoliciesSection.test.tsx
export function d6Guard(body: UpsertTeamModelPolicyRequest, toaster: typeof toast | null): UpsertTeamModelPolicyRequest {
	if (body.blacklisted_models.length > 0 && body.allowed_models.length === 0) {
		if (toaster) {
			toaster.warning("teams.teamPolicies.d6AutoFillWarning");
		}
		return { allowed_models: ["*"], blacklisted_models: body.blacklisted_models };
	}
	return body;
}
import Provider from "@/components/provider";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useGetAllKeysQuery } from "@/lib/store/apis/providersApi";
import { useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { Link } from "@tanstack/react-router";
import { KeyRound, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";

type DotState = "ok" | "missing" | "degraded" | "error";

const dotClass: Record<DotState, string> = {
	ok: "bg-emerald-500",
	missing: "bg-zinc-400",
	degraded: "bg-amber-500",
	error: "bg-red-500",
};

interface ProviderHealthSummary {
	provider: string;
	keysCount: number;
	modelsCount: number;
	dotState: DotState;
}

export default function ProviderTopologyCard() {
	const { t } = useTranslation("common");
	const { data: providers } = useGetProvidersQuery();
	const { data: allKeys } = useGetAllKeysQuery();

	const summaries: ProviderHealthSummary[] = (providers ?? []).map((p) => {
		const keysForProvider = (allKeys ?? []).filter((k) => k.provider === p.name);
		const models = new Set<string>();
		for (const k of keysForProvider) {
			for (const m of k.models ?? []) models.add(m);
		}

		let dotState: DotState;
		if (p.is_key_less) {
			// Keyless providers (opencode, custom keyless) have no keys — infer
			// health from the operational status instead of key presence.
			dotState = p.provider_status === "error" ? "missing" : p.status === "list_models_failed" ? "degraded" : "ok";
		} else {
			const enabled = (p.keys_enabled ?? true) && keysForProvider.length > 0;
			dotState = !enabled ? "missing" : p.last_error_at ? "error" : "ok";
		}

		return {
			provider: p.name,
			keysCount: keysForProvider.length,
			modelsCount: p.models_count ?? models.size,
			dotState,
		};
	});

	const isEmpty = summaries.length === 0;

	return (
		<Card className="bg-card gap-0 border py-0 shadow-sm" data-testid="home-provider-topology">
			<CardHeader className="flex flex-row items-center justify-between gap-2 border-b px-6 py-3">
				<CardTitle className="text-sm font-semibold">{t("home.providers.title")}</CardTitle>
				<Button size="sm" variant="outline" asChild>
					<Link to="/workspace/providers" data-testid="home-provider-topology-add">
						<Plus className="mr-1 h-4 w-4" />
						{t("home.providers.addProvider")}
					</Link>
				</Button>
			</CardHeader>
			<CardContent className="px-6 py-4">
				{isEmpty ? (
					<div className="text-muted-foreground flex flex-col items-center gap-2 py-6 text-center text-sm">
						<Provider provider={"openai"} size={28} className="mt-0 opacity-40" />
						<p>{t("home.providers.empty")}</p>
						<Button size="sm" asChild>
							<Link to="/workspace/providers">
								<Plus className="mr-1 h-4 w-4" />
								{t("home.providers.addProvider")}
							</Link>
						</Button>
					</div>
				) : (
					<ul className="space-y-2">
						{summaries.map((s) => {
							return (
								<li key={s.provider}>
									<Link
										to="/workspace/providers/$id"
										params={{ id: s.provider }}
										className="bg-muted/20 hover:bg-muted/40 flex items-center gap-3 rounded-md border px-3 py-2 transition-colors"
										data-testid={`home-provider-topology-row-${s.provider}`}
									>
										<Provider provider={s.provider} size={22} className="mt-0 shrink-0" />
										<span className="flex-1 truncate text-sm font-medium capitalize">{s.provider}</span>
										<span className={["inline-block h-2 w-2 shrink-0 rounded-full", dotClass[s.dotState]].join(" ")} aria-hidden />
										<span
											className="text-muted-foreground flex items-center gap-1 text-xs"
											title={t("home.providers.keysCount", { count: String(s.keysCount) })}
										>
											<KeyRound className="h-3.5 w-3.5" />
											{s.keysCount}
										</span>
										<span className="text-muted-foreground text-xs">
											{t("home.providers.modelsCount", { count: String(s.modelsCount) })}
										</span>
									</Link>
								</li>
							);
						})}
					</ul>
				)}
			</CardContent>
		</Card>
	);
}
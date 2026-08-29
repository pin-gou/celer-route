import Provider from "@/components/provider";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ProviderIconType, RenderProviderIcon } from "@/lib/constants/icons";
import { getProviderLabel } from "@/lib/constants/logs";
import { useGetAllKeysQuery } from "@/lib/store/apis/providersApi";
import { useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { Link } from "@tanstack/react-router";
import { KeyRound, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { computeDotState, dotClass, DotState } from "./providerHealth";

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

		const dotState: DotState = computeDotState(p, keysForProvider.length);

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
					<ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
						{summaries.map((s) => {
							return (
								<li key={s.provider} className="min-w-0">
									<Link
										to="/workspace/providers/$id"
										params={{ id: s.provider }}
										className="bg-muted/20 hover:bg-muted/40 flex h-full flex-col gap-2 rounded-md border px-3 py-2 transition-colors"
										data-testid={`home-provider-topology-row-${s.provider}`}
									>
										<div className="flex items-center gap-3">
											<RenderProviderIcon provider={s.provider as ProviderIconType} size={22} className="mt-0 shrink-0" />
											<span className="min-w-0 flex-1 truncate text-sm font-medium">{getProviderLabel(s.provider)}</span>
											<span
												className={["inline-block h-2 w-2 shrink-0 rounded-full", dotClass[s.dotState]].join(" ")}
												aria-hidden
												title={t(`home.providers.dotState.${s.dotState}`)}
											/>
										</div>
										<div className="text-muted-foreground flex items-center gap-3 pl-[30px] text-xs">
											<span className="flex items-center gap-1" title={t("home.providers.keysCount", { count: String(s.keysCount) })}>
												<KeyRound className="h-3.5 w-3.5" />
												{t("home.providers.keysCount", { count: String(s.keysCount) })}
											</span>
											<span>{t("home.providers.modelsCount", { count: String(s.modelsCount) })}</span>
										</div>
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
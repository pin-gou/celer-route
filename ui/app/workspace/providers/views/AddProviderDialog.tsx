import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { ProviderIconType, RenderProviderIcon } from "@/lib/constants/icons";
import { getProviderLabel, type ProviderName } from "@/lib/constants/logs";
import { cn } from "@/lib/utils";
import { CheckIcon, PlusIcon, SearchIcon, Settings2Icon, SparklesIcon, XIcon } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { FAMILY_ORDER, getFamilyName } from "./providerFamilies";
import { CAPABILITY_ORDER, type Capability, ProviderCapabilities, RECOMMENDED_PROVIDERS } from "./providerCapabilities";

export interface AddProviderDialogProps {
	/** Provider names already configured in the workspace (existing sidebar entries). */
	existingProviderNames: Set<string>;
	/** All known provider options to surface (typically `ProviderNames` from constants). */
	knownProviders: { name: ProviderName }[];
	/** Called when the user picks a known provider that is not yet configured. */
	onSelectKnownProvider: (name: ProviderName) => void;
	/** Called when the user picks "Custom provider..." */
	onAddCustomProvider: () => void;
	/** Trigger variant. `toolbar` for the page toolbar, `empty` for the empty state. */
	variant?: "toolbar" | "empty";
	disabled?: boolean;
}

function getCapabilities(name: ProviderName): Capability[] {
	return ProviderCapabilities[name] ?? [];
}

function matchesSearch(name: ProviderName, query: string): boolean {
	if (!query) return true;
	const q = query.toLowerCase();
	return name.toLowerCase().includes(q) || getProviderLabel(name).toLowerCase().includes(q);
}

function matchesCapabilities(name: ProviderName, selected: Set<Capability>): boolean {
	if (selected.size === 0) return true;
	const caps = getCapabilities(name);
	for (const c of selected) {
		if (!caps.includes(c)) return false;
	}
	return true;
}

export function AddProviderDialog({
	existingProviderNames,
	knownProviders,
	onSelectKnownProvider,
	onAddCustomProvider,
	variant = "toolbar",
	disabled = false,
}: AddProviderDialogProps) {
	const { t } = useTranslation("providers");
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	const [activeCaps, setActiveCaps] = useState<Set<Capability>>(new Set());

	const allKnownNames = useMemo(() => knownProviders.map((p) => p.name), [knownProviders]);

	const filteredNames = useMemo(() => {
		return allKnownNames.filter((n) => matchesSearch(n, search) && matchesCapabilities(n, activeCaps));
	}, [allKnownNames, search, activeCaps]);

	const recommended = useMemo(
		() => RECOMMENDED_PROVIDERS.filter((n) => allKnownNames.includes(n)).filter((n) => filteredNames.includes(n)),
		[allKnownNames, filteredNames],
	);

	const grouped = useMemo(() => {
		const map = new Map<string, ProviderName[]>();
		for (const name of filteredNames) {
			const family = getFamilyName({ name });
			if (!map.has(family)) map.set(family, []);
			map.get(family)!.push(name);
		}
		return FAMILY_ORDER.filter((f) => map.has(f)).map((family) => ({ family, providers: map.get(family)! }));
	}, [filteredNames]);

	const toggleCap = (cap: Capability) => {
		setActiveCaps((prev) => {
			const next = new Set(prev);
			if (next.has(cap)) next.delete(cap);
			else next.add(cap);
			return next;
		});
	};

	const reset = () => {
		setSearch("");
		setActiveCaps(new Set());
	};

	const handleOpenChange = (next: boolean) => {
		setOpen(next);
		if (!next) reset();
	};

	const handleKnownClick = (name: ProviderName) => {
		if (existingProviderNames.has(name)) return;
		onSelectKnownProvider(name);
		setOpen(false);
		reset();
	};

	const handleCustomClick = () => {
		onAddCustomProvider();
		setOpen(false);
		reset();
	};

	const hasActiveFilter = search.length > 0 || activeCaps.size > 0;
	const noResults = filteredNames.length === 0;

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogTrigger asChild>
				<Button
					variant="outline"
					size={variant === "empty" ? "default" : "sm"}
					data-testid="add-provider-btn"
					className={variant === "empty" ? "" : "gap-1 text-xs"}
					aria-label={t("providers2.addProvider")}
					disabled={disabled}
				>
					<PlusIcon className="h-4 w-4" />
					{variant === "empty" ? <span>{t("providers2.addProvider")}</span> : <div className="text-xs">{t("providers2.addProvider")}</div>}
				</Button>
			</DialogTrigger>
			<DialogContent
				className="custom-scrollbar flex max-h-[75vh] w-[75vw] max-w-[800px] flex-col gap-0 overflow-hidden p-0"
				data-testid="add-provider-dialog"
				showCloseButton={false}
			>
				<DialogHeader className="flex flex-row items-center justify-between gap-2 border-b px-6 py-4 pb-3 text-left">
					<div>
						<DialogTitle>{t("providers2.addProviderDialog.title")}</DialogTitle>
						<DialogDescription>{t("providers2.addProviderDialog.description")}</DialogDescription>
					</div>
					<Button
						variant="ghost"
						size="icon"
						className="h-8 w-8"
						onClick={() => handleOpenChange(false)}
						aria-label={t("providers2.addProviderDialog.close")}
					>
						<XIcon className="h-4 w-4" />
					</Button>
				</DialogHeader>

				{/* Search + capability filters */}
				<div className="flex flex-col gap-3 border-b px-6 py-4">
					<div className="relative">
						<SearchIcon className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
						<Input
							data-testid="add-provider-search"
							value={search}
							onChange={(e) => setSearch(e.target.value)}
							placeholder={t("providers2.addProviderDialog.searchPlaceholder")}
							className="pl-9"
							autoComplete="off"
						/>
					</div>
					<div className="flex flex-wrap gap-1.5">
						<span className="text-muted-foreground self-center text-xs">{t("providers2.addProviderDialog.capabilityLabel")}</span>
						{CAPABILITY_ORDER.map((cap) => {
							const isActive = activeCaps.has(cap);
							return (
								<button
									key={cap}
									type="button"
									data-testid={`add-provider-cap-${cap}`}
									onClick={() => toggleCap(cap)}
									aria-pressed={isActive}
									className={cn(
										"rounded-full border px-2.5 py-0.5 text-xs transition-colors",
										isActive
											? "border-primary bg-primary text-primary-foreground"
											: "bg-background hover:bg-accent text-muted-foreground hover:text-foreground",
									)}
								>
									{t(`providers2.capabilities.${cap}`)}
								</button>
							);
						})}
						{hasActiveFilter && (
							<button
								type="button"
								onClick={reset}
								className="text-muted-foreground hover:text-foreground ml-1 rounded-full px-2 py-0.5 text-xs underline-offset-2 hover:underline"
							>
								{t("providers2.addProviderDialog.reset")}
							</button>
						)}
					</div>
				</div>

				{/* Scrollable body */}
				<div className="custom-scrollbar flex-1 overflow-y-auto px-6 py-4">
					{noResults ? (
						<div className="text-muted-foreground flex h-32 items-center justify-center text-sm">
							{t("providers2.addProviderDialog.noResults")}
						</div>
					) : (
						<div className="flex flex-col gap-6">
							{/* Recommended row — only when no filter is active */}
							{!hasActiveFilter && recommended.length > 0 && (
								<section data-testid="add-provider-recommended">
									<header className="mb-3 flex items-center gap-2">
										<SparklesIcon className="text-primary h-4 w-4" />
										<h3 className="text-sm font-semibold">{t("providers2.addProviderDialog.recommended")}</h3>
									</header>
									<div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
										{recommended.map((name) => (
											<ProviderTile
												key={name}
												name={name}
												alreadyAdded={existingProviderNames.has(name)}
												onClick={() => handleKnownClick(name)}
											/>
										))}
									</div>
								</section>
							)}

							{/* Family sections */}
							{grouped.map(({ family, providers }) => (
								<section key={family} data-testid={`add-provider-family-${family.toLowerCase().replace(/\s+/g, "-")}`}>
									<header className="mb-3 flex items-center justify-between">
										<h3 className="text-sm font-semibold">
											{t(
												{
													Custom: "providers2.family.custom",
													Other: "providers2.family.other",
													"OpenAI Family": "providers2.family.openai",
													"Anthropic Family": "providers2.family.anthropic",
													"Google Family": "providers2.family.google",
													"Meta-Llama Family": "providers2.family.metaLlama",
													"AWS Family": "providers2.family.aws",
												}[family] ?? family,
											)}
										</h3>
										<span className="text-muted-foreground text-xs">({providers.length})</span>
									</header>
									<div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
										{providers.map((name) => (
											<ProviderTile
												key={name}
												name={name}
												alreadyAdded={existingProviderNames.has(name)}
												onClick={() => handleKnownClick(name)}
											/>
										))}
									</div>
								</section>
							))}
						</div>
					)}
				</div>

				{/* Custom provider footer */}
				<div className="border-t px-6 py-3">
					<button
						type="button"
						data-testid="add-provider-option-custom"
						onClick={handleCustomClick}
						className="hover:bg-accent flex w-full items-center gap-3 rounded-md px-2 py-2 text-left transition-colors"
					>
						<div className="bg-muted text-muted-foreground flex h-9 w-9 items-center justify-center rounded-md">
							<Settings2Icon className="h-4 w-4" />
						</div>
						<div className="flex flex-col">
							<span className="text-sm font-medium">{t("providers2.customProviderOption")}</span>
							<span className="text-muted-foreground text-xs">{t("providers2.addProviderDialog.customDescription")}</span>
						</div>
					</button>
				</div>
			</DialogContent>
		</Dialog>
	);
}

interface ProviderTileProps {
	name: ProviderName;
	alreadyAdded: boolean;
	onClick: () => void;
}

function ProviderTile({ name, alreadyAdded, onClick }: ProviderTileProps) {
	const { t } = useTranslation("providers");
	const caps = getCapabilities(name);
	const label = getProviderLabel(name);
	return (
		<button
			type="button"
			data-testid={`add-provider-option-${name}`}
			disabled={alreadyAdded}
			onClick={onClick}
			className={cn(
				"group relative flex flex-col gap-2 rounded-md border p-3 text-left transition-colors",
				alreadyAdded ? "bg-muted/40 text-muted-foreground cursor-not-allowed" : "hover:border-primary hover:bg-accent/40 cursor-pointer",
			)}
		>
			{alreadyAdded && (
				<Badge variant="success" className="absolute top-2 right-2" data-testid={`add-provider-option-${name}-added`}>
					<CheckIcon className="h-3 w-3" />
					{t("providers2.addProviderDialog.added")}
				</Badge>
			)}
			<div className="flex items-center gap-2">
				<RenderProviderIcon provider={name as ProviderIconType} size="sm" className="h-5 w-5" />
				<span className="truncate text-sm font-medium" title={label}>
					{label}
				</span>
			</div>
			<div className="flex flex-wrap gap-1">
				{caps.slice(0, 4).map((cap) => (
					<span key={cap} className="bg-background text-muted-foreground rounded border px-1.5 py-0.5 text-[10px] leading-none">
						{t(`providers2.capabilities.${cap}`)}
					</span>
				))}
				{caps.length > 4 && <span className="text-muted-foreground text-[10px] leading-none">+{caps.length - 4}</span>}
			</div>
		</button>
	);
}
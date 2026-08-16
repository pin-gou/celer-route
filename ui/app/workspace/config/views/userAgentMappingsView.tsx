import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
	AlertDialogTrigger,
} from "@/components/ui/alertDialog";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdownMenu";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
	getErrorMessage,
	type UserAgentMapping,
	type UserAgentMappingMatchType,
	type UserAgentMappingPayload,
	useCreateUserAgentMappingMutation,
	useDeleteUserAgentMappingMutation,
	useGetUserAgentMappingsQuery,
	useUpdateUserAgentMappingMutation,
} from "@/lib/store";
import { MoreVertical, Pencil, Plus, Trash2, Upload, X } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

const emptyDraft: UserAgentMappingPayload = {
	pattern: "",
	match_type: "contains",
	app: "",
	logo: undefined,
	logo_mime: null,
	is_active: true,
};

// Cap logo uploads before base64 conversion to avoid freezing the UI and sending oversized payloads.
const MAX_LOGO_BYTES = 256 * 1024;

interface UserAgentMappingsViewProps {
	disabled?: boolean;
}

export default function UserAgentMappingsView({ disabled }: UserAgentMappingsViewProps) {
	const { t } = useTranslation("config");
	const { data, isLoading } = useGetUserAgentMappingsQuery();
	const [createMapping, { isLoading: isCreating }] = useCreateUserAgentMappingMutation();
	const [updateMapping, { isLoading: isUpdating }] = useUpdateUserAgentMappingMutation();
	const [deleteMapping, { isLoading: isDeleting }] = useDeleteUserAgentMappingMutation();
	const [draft, setDraft] = useState<UserAgentMappingPayload>(emptyDraft);
	const [editingMappingId, setEditingMappingId] = useState<string | null>(null);
	const [isSheetOpen, setIsSheetOpen] = useState(false);

	const matchTypeOptions = useMemo<Array<{ value: UserAgentMappingMatchType; label: string }>>(
		() => [
			{ value: "contains", label: t("clientSettings.userAgentMappings.matchTypes.contains") },
			{ value: "starts_with", label: t("clientSettings.userAgentMappings.matchTypes.starts_with") },
			{ value: "exact", label: t("clientSettings.userAgentMappings.matchTypes.exact") },
			{ value: "regex", label: t("clientSettings.userAgentMappings.matchTypes.regex") },
		],
		[t],
	);

	const mappings = useMemo(() => data?.mappings ?? [], [data]);
	const controlsDisabled = disabled || isCreating || isUpdating || isDeleting;
	const isEditing = Boolean(editingMappingId);

	const openAddSheet = () => {
		setEditingMappingId(null);
		setDraft(emptyDraft);
		setIsSheetOpen(true);
	};

	const openEditSheet = (mapping: UserAgentMapping) => {
		setEditingMappingId(mapping.id);
		setDraft(mappingToPayload(mapping));
		setIsSheetOpen(true);
	};

	const handleSheetOpenChange = (open: boolean) => {
		setIsSheetOpen(open);
		if (!open) {
			setEditingMappingId(null);
			setDraft(emptyDraft);
		}
	};

	const handleSubmit = async () => {
		const validated = validateDraft(draft, t);
		if (!validated) return;
		try {
			if (editingMappingId) {
				await updateMapping({ id: editingMappingId, data: validated }).unwrap();
				toast.success(t("clientSettings.userAgentMappings.toast.updated"));
			} else {
				await createMapping(validated).unwrap();
				toast.success(t("clientSettings.userAgentMappings.toast.added"));
			}
			handleSheetOpenChange(false);
		} catch (error) {
			toast.error(
				t(editingMappingId ? "clientSettings.userAgentMappings.toast.updateFailed" : "clientSettings.userAgentMappings.toast.addFailed", {
					error: getErrorMessage(error),
				}),
			);
		}
	};

	const handleDelete = async (id: string) => {
		try {
			await deleteMapping(id).unwrap();
			toast.success(t("clientSettings.userAgentMappings.toast.deleted"));
		} catch (error) {
			toast.error(t("clientSettings.userAgentMappings.toast.deleteFailed", { error: getErrorMessage(error) }));
		}
	};

	return (
		<div className="space-y-4">
			<div className="flex items-start justify-between gap-4">
				<div>
					<h3 className="text-lg font-semibold tracking-tight">{t("clientSettings.userAgentMappings.title")}</h3>
					<p className="text-muted-foreground text-sm">{t("clientSettings.userAgentMappings.description")}</p>
				</div>
				<div className="pt-2">
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={openAddSheet}
						disabled={controlsDisabled}
						data-testid="user-agent-mapping-add-btn"
					>
						<Plus className="h-4 w-4" />
						{t("clientSettings.userAgentMappings.add")}
					</Button>
				</div>
			</div>

			<Sheet open={isSheetOpen} onOpenChange={handleSheetOpenChange}>
				<SheetContent className="p-0">
					<SheetHeader className="flex flex-col items-start px-6 pt-6">
						<SheetTitle>
							{isEditing ? t("clientSettings.userAgentMappings.editTitle") : t("clientSettings.userAgentMappings.addTitle")}
						</SheetTitle>
						<SheetDescription>{t("clientSettings.userAgentMappings.sheetDescription")}</SheetDescription>
					</SheetHeader>
					<div className="flex-1 space-y-4 px-6">
						<MappingForm draft={draft} onChange={setDraft} disabled={controlsDisabled} />
					</div>
					<SheetFooter className="flex-row justify-end border-t px-6 py-4">
						<Button
							type="button"
							variant="outline"
							onClick={() => handleSheetOpenChange(false)}
							data-testid="user-agent-mapping-cancel-btn"
						>
							{t("clientSettings.userAgentMappings.cancel")}
						</Button>
						<Button type="button" onClick={handleSubmit} disabled={controlsDisabled} data-testid="user-agent-mapping-submit-btn">
							{isEditing ? t("clientSettings.userAgentMappings.saveChanges") : t("clientSettings.userAgentMappings.addMapping")}
						</Button>
					</SheetFooter>
				</SheetContent>
			</Sheet>

			<Table containerClassName="rounded-sm border">
				<TableHeader>
					<TableRow>
						<TableHead>{t("clientSettings.userAgentMappings.pattern")}</TableHead>
						<TableHead>{t("clientSettings.userAgentMappings.match")}</TableHead>
						<TableHead>{t("clientSettings.userAgentMappings.app")}</TableHead>
						<TableHead>{t("clientSettings.userAgentMappings.logo")}</TableHead>
						<TableHead>{t("clientSettings.userAgentMappings.active")}</TableHead>
						<TableHead className="w-[92px] text-right">{t("clientSettings.userAgentMappings.actions")}</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading ? (
						<TableRow>
							<TableCell colSpan={6} className="text-muted-foreground py-6 text-center">
								{t("clientSettings.userAgentMappings.loading")}
							</TableCell>
						</TableRow>
					) : mappings.length === 0 ? (
						<TableRow>
							<TableCell colSpan={6} className="text-muted-foreground py-6 text-center">
								{t("clientSettings.userAgentMappings.empty")}
							</TableCell>
						</TableRow>
					) : (
						mappings.map((mapping) => {
							const logoSrc = mapping.logo && mapping.logo_mime ? `data:${mapping.logo_mime};base64,${mapping.logo}` : "";
							return (
								<TableRow key={mapping.id}>
									<TableCell className="max-w-[260px]">
										<span className="block truncate font-mono text-sm" title={mapping.pattern}>
											{mapping.pattern}
										</span>
									</TableCell>
									<TableCell>
										<span className="text-sm">{getMatchTypeLabel(mapping.match_type, matchTypeOptions)}</span>
									</TableCell>
									<TableCell className="max-w-[220px]">
										<span className="block truncate text-sm" title={mapping.app}>
											{mapping.app}
										</span>
									</TableCell>
									<TableCell>
										{logoSrc ? (
											<img src={logoSrc} alt={mapping.app} className="size-7 rounded-sm border object-contain" />
										) : (
											<span className="text-muted-foreground text-sm">-</span>
										)}
									</TableCell>
									<TableCell>
										<span className={mapping.is_active ? "text-sm text-emerald-700" : "text-muted-foreground text-sm"}>
											{mapping.is_active ? t("clientSettings.userAgentMappings.active") : t("clientSettings.userAgentMappings.inactive")}
										</span>
									</TableCell>
									<TableCell className="text-right">
										<AlertDialog>
											<DropdownMenu>
												<DropdownMenuTrigger asChild>
													<Button
														type="button"
														variant="ghost"
														size="icon"
														disabled={controlsDisabled}
														aria-label={t("clientSettings.userAgentMappings.mappingActionsAriaLabel")}
														data-testid={`user-agent-mapping-actions-${mapping.id}`}
													>
														<MoreVertical className="h-4 w-4" />
													</Button>
												</DropdownMenuTrigger>
												<DropdownMenuContent align="end">
													<DropdownMenuItem onSelect={() => openEditSheet(mapping)} data-testid={`user-agent-mapping-edit-${mapping.id}`}>
														<Pencil className="h-4 w-4" />
														{t("clientSettings.userAgentMappings.edit")}
													</DropdownMenuItem>
													<AlertDialogTrigger asChild>
														<DropdownMenuItem variant="destructive" data-testid={`user-agent-mapping-delete-${mapping.id}`}>
															<Trash2 className="h-4 w-4" />
															{t("clientSettings.userAgentMappings.delete")}
														</DropdownMenuItem>
													</AlertDialogTrigger>
												</DropdownMenuContent>
											</DropdownMenu>
											<AlertDialogContent>
												<AlertDialogHeader>
													<AlertDialogTitle>{t("clientSettings.userAgentMappings.deleteConfirmTitle")}</AlertDialogTitle>
													<AlertDialogDescription>{t("clientSettings.userAgentMappings.deleteConfirmDesc")}</AlertDialogDescription>
												</AlertDialogHeader>
												<AlertDialogFooter>
													<AlertDialogCancel data-testid={`user-agent-mapping-delete-cancel-${mapping.id}`}>
														{t("clientSettings.userAgentMappings.cancel")}
													</AlertDialogCancel>
													<AlertDialogAction
														data-testid={`user-agent-mapping-delete-confirm-${mapping.id}`}
														onClick={() => handleDelete(mapping.id)}
													>
														{t("clientSettings.userAgentMappings.delete")}
													</AlertDialogAction>
												</AlertDialogFooter>
											</AlertDialogContent>
										</AlertDialog>
									</TableCell>
								</TableRow>
							);
						})
					)}
				</TableBody>
			</Table>
		</div>
	);
}

function MappingForm({
	draft,
	onChange,
	disabled,
}: {
	draft: UserAgentMappingPayload;
	onChange: (next: UserAgentMappingPayload) => void;
	disabled?: boolean;
}) {
	const { t } = useTranslation("config");
	return (
		<div className="space-y-4">
			<div className="space-y-2">
				<label htmlFor="user-agent-mapping-pattern-input" className="text-sm font-medium">
					{t("clientSettings.userAgentMappings.pattern")}
				</label>
				<Input
					id="user-agent-mapping-pattern-input"
					placeholder={t("clientSettings.userAgentMappings.patternPlaceholder")}
					value={draft.pattern}
					onChange={(event) => onChange({ ...draft, pattern: event.target.value })}
					disabled={disabled}
					data-testid="user-agent-mapping-pattern-input"
				/>
			</div>
			<div className="space-y-2">
				<label htmlFor="user-agent-mapping-match-type-select" className="text-sm font-medium">
					{t("clientSettings.userAgentMappings.matchType")}
				</label>
				<MatchTypeSelect
					id="user-agent-mapping-match-type-select"
					value={draft.match_type}
					onChange={(matchType) => onChange({ ...draft, match_type: matchType })}
					disabled={disabled}
				/>
			</div>
			<div className="space-y-2">
				<label htmlFor="user-agent-mapping-app-input" className="text-sm font-medium">
					{t("clientSettings.userAgentMappings.app")}
				</label>
				<Input
					id="user-agent-mapping-app-input"
					placeholder={t("clientSettings.userAgentMappings.appPlaceholder")}
					value={draft.app}
					onChange={(event) => onChange({ ...draft, app: event.target.value })}
					disabled={disabled}
					data-testid="user-agent-mapping-app-input"
				/>
			</div>
			<div className="space-y-2">
				<label htmlFor="user-agent-mapping-logo-upload" className="text-sm font-medium">
					{t("clientSettings.userAgentMappings.logo")}
				</label>
				<LogoInput draft={draft} onChange={onChange} disabled={disabled} />
			</div>
			<div className="flex items-center justify-between rounded-sm border p-3">
				<div>
					<p className="text-sm font-medium">{t("clientSettings.userAgentMappings.active")}</p>
					<p className="text-muted-foreground text-xs">{t("clientSettings.userAgentMappings.activeDesc")}</p>
				</div>
				<Switch
					checked={draft.is_active}
					onCheckedChange={(checked) => onChange({ ...draft, is_active: checked })}
					disabled={disabled}
					data-testid="user-agent-mapping-active-switch"
				/>
			</div>
		</div>
	);
}

function MatchTypeSelect({
	value,
	onChange,
	disabled,
	id,
}: {
	value: UserAgentMappingMatchType;
	onChange: (value: UserAgentMappingMatchType) => void;
	disabled?: boolean;
	id?: string;
}) {
	const { t } = useTranslation("config");
	const options: Array<{ value: UserAgentMappingMatchType; label: string }> = [
		{ value: "contains", label: t("clientSettings.userAgentMappings.matchTypes.contains") },
		{ value: "starts_with", label: t("clientSettings.userAgentMappings.matchTypes.starts_with") },
		{ value: "exact", label: t("clientSettings.userAgentMappings.matchTypes.exact") },
		{ value: "regex", label: t("clientSettings.userAgentMappings.matchTypes.regex") },
	];
	return (
		<Select value={value} onValueChange={(next) => onChange(next as UserAgentMappingMatchType)} disabled={disabled}>
			<SelectTrigger id={id} className="w-full" data-testid="user-agent-mapping-match-type-select">
				<SelectValue />
			</SelectTrigger>
			<SelectContent>
				{options.map((option) => (
					<SelectItem key={option.value} value={option.value}>
						{option.label}
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	);
}

function LogoInput({
	draft,
	onChange,
	disabled,
}: {
	draft: UserAgentMappingPayload;
	onChange: (next: UserAgentMappingPayload) => void;
	disabled?: boolean;
}) {
	const { t } = useTranslation("config");
	const dataUrl = draft.logo && draft.logo_mime ? `data:${draft.logo_mime};base64,${draft.logo}` : "";
	return (
		<div className="flex items-center gap-2">
			{dataUrl && <img src={dataUrl} alt="" className="size-7 rounded-sm border object-contain" />}
			<Button type="button" variant="outline" size="icon" disabled={disabled} asChild>
				<label aria-label={t("clientSettings.userAgentMappings.uploadLogo")}>
					<Upload className="h-4 w-4" />
					<input
						id="user-agent-mapping-logo-upload"
						type="file"
						accept="image/*"
						className="hidden"
						data-testid="user-agent-mapping-logo-upload"
						onChange={async (event) => {
							const file = event.target.files?.[0];
							if (!file) return;
							if (file.size > MAX_LOGO_BYTES) {
								toast.error(t("clientSettings.userAgentMappings.logoTooLarge"));
								event.target.value = "";
								return;
							}
							try {
								const logo = await fileToBase64(file);
								onChange({ ...draft, logo, logo_mime: file.type || "application/octet-stream" });
							} catch {
								toast.error(t("clientSettings.userAgentMappings.logoReadFailed"));
							}
							event.target.value = "";
						}}
					/>
				</label>
			</Button>
			<Button
				type="button"
				variant="ghost"
				size="icon"
				disabled={disabled || !draft.logo}
				onClick={() => onChange({ ...draft, logo: undefined, logo_mime: null })}
				aria-label={t("clientSettings.userAgentMappings.removeLogo")}
				data-testid="user-agent-mapping-logo-remove"
			>
				<X className="h-4 w-4" />
			</Button>
		</div>
	);
}

function mappingToPayload(mapping: UserAgentMapping): UserAgentMappingPayload {
	return {
		pattern: mapping.pattern,
		match_type: mapping.match_type,
		app: mapping.app,
		logo: mapping.logo,
		logo_mime: mapping.logo_mime ?? null,
		is_active: mapping.is_active,
	};
}

function getMatchTypeLabel(matchType: UserAgentMappingMatchType, options: Array<{ value: UserAgentMappingMatchType; label: string }>): string {
	return options.find((o) => o.value === matchType)?.label ?? matchType;
}

function validateDraft(draft: UserAgentMappingPayload | undefined, t: (key: string) => string): UserAgentMappingPayload | null {
	if (!draft || !draft.pattern.trim() || !draft.app.trim()) {
		toast.error(t("clientSettings.userAgentMappings.patternAndAppRequired"));
		return null;
	}
	if (draft.match_type === "regex") {
		try {
			new RegExp(draft.pattern);
		} catch {
			toast.error(t("clientSettings.userAgentMappings.regexInvalid"));
			return null;
		}
	}
	return {
		...draft,
		pattern: draft.pattern.trim(),
		app: draft.app.trim(),
	};
}

function fileToBase64(file: File): Promise<string> {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => {
			const value = String(reader.result ?? "");
			resolve(value.includes(",") ? value.split(",")[1] : value);
		};
		reader.onerror = () => reject(reader.error);
		reader.readAsDataURL(file);
	});
}
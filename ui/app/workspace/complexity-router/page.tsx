import FullPageLoader from "@/components/fullPageLoader";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alertDialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scrollArea";
import { TagInput } from "@/components/ui/tagInput";
import { getErrorMessage } from "@/lib/store";
import {
	useGetComplexityAnalyzerConfigQuery,
	useResetComplexityAnalyzerConfigMutation,
	useUpdateComplexityAnalyzerConfigMutation,
} from "@/lib/store/apis/governanceApi";
import {
	AnalyzerConfig,
	DEFAULT_TIER_BOUNDARIES,
	KEYWORD_LIST_DEFINITIONS,
	KeywordListKey,
	TierBoundaries,
} from "@/lib/types/complexityRouter";
import { cn } from "@/lib/utils";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { useTranslation } from "react-i18next";
import { zodResolver } from "@hookform/resolvers/zod";
import { LoaderCircle, RotateCcw, Save } from "lucide-react";
import { type ChangeEvent, type ClipboardEvent, type DragEvent, type KeyboardEvent, useEffect, useMemo, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

type TierBoundaryKey = keyof TierBoundaries;

const KEYWORD_COLLAPSED_LIMIT = 8;

// Four progressive shades of --primary: faintest → full
const P1 = "color-mix(in oklch, var(--primary) 30%, transparent)";
const P2 = "color-mix(in oklch, var(--primary) 55%, transparent)";
const P3 = "color-mix(in oklch, var(--primary) 75%, transparent)";
const P4 = "var(--primary)";

const TIER_PALETTE = {
	simple: { color: P1, name: "SIMPLE" },
	medium: { color: P2, name: "MEDIUM" },
	complex: { color: P3, name: "COMPLEX" },
	reasoning: { color: P4, name: "REASONING" },
} as const;

interface BoundaryFieldConfig {
	key: TierBoundaryKey;
	label: string;
	description: string;
	fromTier: string;
	toTier: string;
	fromColor: string;
	toColor: string;
}

function buildBoundaryFields(t: (key: string, opts?: Record<string, unknown>) => string): BoundaryFieldConfig[] {
	return [
		{
			key: "simple_medium",
			label: t("complexityRouter.simpleToMedium"),
			description: t("complexityRouter.validation.simpleAbove"),
			fromTier: "SIMPLE",
			toTier: "MEDIUM",
			fromColor: P1,
			toColor: P2,
		},
		{
			key: "medium_complex",
			label: t("complexityRouter.mediumToComplex"),
			description: t("complexityRouter.validation.mediumAbove"),
			fromTier: "MEDIUM",
			toTier: "COMPLEX",
			fromColor: P2,
			toColor: P3,
		},
		{
			key: "complex_reasoning",
			label: t("complexityRouter.complexToReasoning"),
			description: t("complexityRouter.validation.complexAbove"),
			fromTier: "COMPLEX",
			toTier: "REASONING",
			fromColor: P3,
			toColor: P4,
		},
	];
}

function buildBoundaryField(t: (key: string, opts?: Record<string, unknown>) => string) {
	return z
		.number({ error: t("complexityRouter.validation.enterNumberBetweenZeroAndOne") })
		.gt(0, t("complexityRouter.validation.mustBeGreaterThanZero"))
		.lt(1, t("complexityRouter.validation.mustBeLessThanOne"));
}

function buildAnalyzerConfigSchema(t: (key: string, opts?: Record<string, unknown>) => string) {
	const boundaryField = buildBoundaryField(t);
	return z.object({
		tier_boundaries: z
			.object({
				simple_medium: boundaryField,
				medium_complex: boundaryField,
				complex_reasoning: boundaryField,
			})
			.superRefine((data, ctx) => {
				if (Number.isFinite(data.medium_complex) && Number.isFinite(data.simple_medium) && data.medium_complex <= data.simple_medium) {
					ctx.addIssue({
						code: "custom",
						message: t("complexityRouter.validation.mustBeGreaterThanSimpleMedium"),
						path: ["medium_complex"],
					});
				}
				if (
					Number.isFinite(data.complex_reasoning) &&
					Number.isFinite(data.medium_complex) &&
					data.complex_reasoning <= data.medium_complex
				) {
					ctx.addIssue({
						code: "custom",
						message: t("complexityRouter.validation.mustBeGreaterThanMediumComplex"),
						path: ["complex_reasoning"],
					});
				}
			}),
		keywords: z.object({
			simple_keywords: z.array(z.string()).min(1, t("complexityRouter.validation.simpleKeywordsCannotBeEmpty")),
			code_keywords: z.array(z.string()).min(1, t("complexityRouter.validation.codeKeywordsCannotBeEmpty")),
			technical_keywords: z.array(z.string()).min(1, t("complexityRouter.validation.technicalKeywordsCannotBeEmpty")),
			reasoning_keywords: z.array(z.string()).min(1, t("complexityRouter.validation.reasoningKeywordsCannotBeEmpty")),
		}),
	});
}

const DEFAULT_FORM_VALUES: AnalyzerConfig = {
	tier_boundaries: { ...DEFAULT_TIER_BOUNDARIES },
	keywords: {
		code_keywords: [],
		reasoning_keywords: [],
		technical_keywords: [],
		simple_keywords: [],
	},
};

function boundaryValueAsNumber(value: unknown): number {
	let numericValue = Number.NaN;
	if (typeof value === "number") {
		numericValue = value;
	} else if (typeof value === "string" && value.trim() !== "") {
		numericValue = Number(value);
	}
	return Number.isFinite(numericValue) ? Math.max(0, numericValue) : Number.NaN;
}

function finiteBoundaryValue(value: number | undefined, fallback: number) {
	return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function clampUnit(value: number) {
	return Math.min(1, Math.max(0, value));
}

function snakeToCamel(value: string): string {
	return value.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
}

function testIdPart(value: string) {
	return value.replace(/_/g, "-");
}

function preventNegativeBoundaryKey(event: KeyboardEvent<HTMLInputElement>) {
	if (event.key === "-") {
		event.preventDefault();
	}
}

function preventNegativeBoundaryPaste(event: ClipboardEvent<HTMLInputElement>) {
	if (/^\s*-/.test(event.clipboardData.getData("text"))) {
		event.preventDefault();
	}
}

function preventNegativeBoundaryDrop(event: DragEvent<HTMLInputElement>) {
	if (/^\s*-/.test(event.dataTransfer.getData("text"))) {
		event.preventDefault();
	}
}

function normalizeBoundaryInput(event: ChangeEvent<HTMLInputElement>) {
	const { value } = event.currentTarget;
	if (!/^\s*-/.test(value)) return;

	const numericValue = Number(value);
	event.currentTarget.value = Number.isFinite(numericValue) ? "0" : "";
}

function TierSpectrumBar({ boundaries }: { boundaries: TierBoundaries }) {
	const sm = clampUnit(finiteBoundaryValue(boundaries?.simple_medium, DEFAULT_TIER_BOUNDARIES.simple_medium));
	const mc = clampUnit(finiteBoundaryValue(boundaries?.medium_complex, DEFAULT_TIER_BOUNDARIES.medium_complex));
	const cr = clampUnit(finiteBoundaryValue(boundaries?.complex_reasoning, DEFAULT_TIER_BOUNDARIES.complex_reasoning));

	const segments = [
		{ tier: "SIMPLE", width: Math.max(0, sm * 100), color: TIER_PALETTE.simple.color },
		{ tier: "MEDIUM", width: Math.max(0, (mc - sm) * 100), color: TIER_PALETTE.medium.color },
		{ tier: "COMPLEX", width: Math.max(0, (cr - mc) * 100), color: TIER_PALETTE.complex.color },
		{ tier: "REASONING", width: Math.max(0, (1 - cr) * 100), color: TIER_PALETTE.reasoning.color },
	];

	const markers = [
		{ key: "simple-medium", pos: sm, value: sm.toFixed(2) },
		{ key: "medium-complex", pos: mc, value: mc.toFixed(2) },
		{ key: "complex-reasoning", pos: cr, value: cr.toFixed(2) },
	];

	return (
		<div className="space-y-1.5">
			<div className="relative flex h-9 w-full gap-[1.5px] overflow-hidden rounded-sm">
				{segments.map(({ tier, width, color }) => (
					<div
						key={tier}
						style={{ width: `${width}%`, backgroundColor: color }}
						className="relative flex items-center justify-center overflow-hidden transition-[width] duration-300 ease-in-out"
					>
						{width > 7 && (
							<span className="pointer-events-none absolute font-mono text-[8px] font-bold tracking-[0.12em] text-white select-none">
								{tier}
							</span>
						)}
					</div>
				))}
				{/* Boundary dividers */}
				{markers.map(({ key, pos }) => (
					<div
						key={key}
						className="bg-background/70 absolute inset-y-0 w-px transition-[left] duration-300 ease-in-out"
						style={{ left: `${pos * 100}%` }}
					/>
				))}
			</div>
			{/* Axis labels */}
			<div className="relative h-3.5 w-full">
				<span className="text-muted-foreground/50 absolute left-0 font-mono text-[9px]">0</span>
				{markers.map(({ key, pos, value }) => (
					<span
						key={key}
						className="text-muted-foreground absolute -translate-x-1/2 font-mono text-[9px] transition-[left] duration-300 ease-in-out"
						style={{ left: `${pos * 100}%` }}
					>
						{value}
					</span>
				))}
				<span className="text-muted-foreground/50 absolute right-0 font-mono text-[9px]">1</span>
			</div>
		</div>
	);
}

export default function ComplexityRouterPage() {
	const { t } = useTranslation("routing");
	const canUpdate = useRbac(RbacResource.RoutingRules, RbacOperation.Update);
	const { data, isLoading, isFetching, error, refetch } = useGetComplexityAnalyzerConfigQuery();
	const [updateConfig, { isLoading: isSaving }] = useUpdateComplexityAnalyzerConfigMutation();
	const [resetConfig, { isLoading: isResetting }] = useResetComplexityAnalyzerConfigMutation();

	const [submitError, setSubmitError] = useState<string | null>(null);
	const [restoreDialogOpen, setRestoreDialogOpen] = useState(false);

	const boundaryFields = useMemo(() => buildBoundaryFields(t), [t]);
	const analyzerSchema = useMemo(() => buildAnalyzerConfigSchema(t), [t]);

	const {
		register,
		handleSubmit,
		reset,
		control,
		watch,
		formState: { errors, isDirty, isSubmitted },
	} = useForm<AnalyzerConfig>({
		resolver: zodResolver(analyzerSchema),
		defaultValues: DEFAULT_FORM_VALUES,
		mode: "onSubmit",
		reValidateMode: "onChange",
	});

	const liveBoundaries = watch("tier_boundaries");

	useEffect(() => {
		if (!data || isDirty) return;
		reset(data);
		setSubmitError(null);
	}, [data, isDirty, reset]);

	const handleDiscard = () => {
		if (data) reset(data);
		setSubmitError(null);
	};

	const handleRestoreDefaults = () => {
		if (!canUpdate) return;
		setSubmitError(null);
		resetConfig()
			.unwrap()
			.then((defaults) => {
				reset(defaults);
				toast.success(t("complexityRouter.resetToDefaults"), { position: "top-right" });
			})
			.catch((err) => {
				setSubmitError(getErrorMessage(err));
			});
	};

	const onValid = (values: AnalyzerConfig) => {
		if (!canUpdate) return;
		setSubmitError(null);
		updateConfig(values)
			.unwrap()
			.then((res) => {
				reset(res);
				toast.success(t("complexityRouter.configSaved"), { position: "top-right" });
			})
			.catch((err) => {
				setSubmitError(getErrorMessage(err));
			});
	};

	if (isLoading && !data) {
		return <FullPageLoader />;
	}

	if (error && !data) {
		return (
			<div className="mx-auto w-full max-w-7xl space-y-4 px-14 pt-8">
				<p className="text-destructive font-mono text-sm">{getErrorMessage(error)}</p>
				<Button data-testid="complexity-router-fetch-retry-button" type="button" variant="outline" size="sm" onClick={() => refetch()}>
					{t("common.action.retry")}
				</Button>
			</div>
		);
	}

	if (!data) {
		return (
			<div className="mx-auto w-full max-w-7xl space-y-4 px-14 pt-8">
				<p className="text-muted-foreground font-mono text-sm">{t("complexityRouter.configUnavailable")}</p>
				<Button data-testid="complexity-router-fetch-retry-button" type="button" variant="outline" size="sm" onClick={() => refetch()}>
					{t("common.action.retry")}
				</Button>
			</div>
		);
	}

	const boundaryErrors = errors.tier_boundaries;
	const keywordErrors = errors.keywords;
	const hasErrors = Boolean(boundaryErrors || keywordErrors);

	return (
		<ScrollArea className="no-padding-parent h-[calc(100vh_-_16px)] w-full px-14 pt-4">
			<form className="mx-auto w-full max-w-7xl space-y-8" onSubmit={handleSubmit(onValid)} noValidate>
				{/* ── Page header ── */}
				<div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
					<div className="space-y-1.5">
						<h1 className="text-2xl font-semibold tracking-tight">{t("complexityRouter.title")}</h1>
						<p className="text-muted-foreground max-w-2xl text-sm leading-relaxed">
							{t("complexityRouter.headerDescription")}{" "}
							<code className="bg-muted rounded-sm px-1 py-0.5 font-mono text-xs">{t("complexityRouter.headerDescriptionCode")}</code>{" "}
							{t("complexityRouter.headerDescriptionSuffix")}
						</p>
					</div>
				</div>

				{/* ── Complexity Spectrum ── */}
				<div className="bg-card space-y-4 rounded-sm border p-5">
					<div className="flex items-center justify-between">
						<p className="text-muted-foreground font-mono text-xs font-semibold tracking-widest uppercase">
							{t("complexityRouter.complexitySpectrum")}
						</p>
						<div className="flex items-center gap-4">
							{Object.values(TIER_PALETTE).map(({ color, name }) => (
								<div key={name} className="flex items-center gap-1.5">
									<div className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: color }} />
									<span className="text-muted-foreground font-mono text-[9px] font-bold tracking-widest">{name}</span>
								</div>
							))}
						</div>
					</div>
					<TierSpectrumBar boundaries={liveBoundaries} />
				</div>

				{/* ── Tier Boundaries ── */}
				<div className="space-y-3">
					<h2 className="text-sm font-semibold">{t("complexityRouter.tierBoundaries")}</h2>

					<div className="grid gap-3 md:grid-cols-3">
						{boundaryFields.map(({ key, label, description, fromTier, toTier, fromColor, toColor }) => {
							const fieldError = boundaryErrors?.[key];
							const inputId = `boundary-${key}`;
							const errorId = `${inputId}-error`;
							const { onChange, ...boundaryInputProps } = register(`tier_boundaries.${key}`, {
								required: t("complexityRouter.validation.enterNumberBetweenZeroAndOne"),
								setValueAs: boundaryValueAsNumber,
								validate: (value) => {
									if (!Number.isFinite(value)) return t("complexityRouter.validation.enterNumberBetweenZeroAndOne");
									if (value <= 0) return t("complexityRouter.validation.mustBeGreaterThanZero");
									if (value >= 1) return t("complexityRouter.validation.mustBeLessThanOne");
									const { simple_medium, medium_complex } = liveBoundaries;
									if (key === "medium_complex" && Number.isFinite(simple_medium) && value <= simple_medium) {
										return t("complexityRouter.validation.mustBeGreaterThanSimpleMedium");
									}
									if (key === "complex_reasoning" && Number.isFinite(medium_complex) && value <= medium_complex) {
										return t("complexityRouter.validation.mustBeGreaterThanMediumComplex");
									}
									return true;
								},
								deps:
									key === "simple_medium"
										? ["tier_boundaries.medium_complex"]
										: key === "medium_complex"
											? ["tier_boundaries.complex_reasoning"]
											: undefined,
							});

							return (
								<div key={key} className="bg-card relative space-y-3 overflow-hidden rounded-sm border p-4">
									{/* Tier transition label */}
									<div className="flex items-center gap-1.5 pt-0.5">
										<span className="font-mono text-[10px] font-bold tracking-widest" style={{ color: fromColor }}>
											{fromTier}
										</span>
										<span className="text-muted-foreground/40 text-[10px]">→</span>
										<span className="font-mono text-[10px] font-bold tracking-widest" style={{ color: toColor }}>
											{toTier}
										</span>
									</div>

									<label htmlFor={inputId} className="sr-only">
										{label}
									</label>
									<Input
										data-testid={`complexity-router-boundary-${testIdPart(key)}-input`}
										id={inputId}
										type="number"
										inputMode="decimal"
										min={0}
										max={1}
										step={0.01}
										onKeyDown={preventNegativeBoundaryKey}
										onPaste={preventNegativeBoundaryPaste}
										onDrop={preventNegativeBoundaryDrop}
										onChange={(event) => {
											normalizeBoundaryInput(event);
											onChange(event);
										}}
										aria-invalid={fieldError ? true : undefined}
										aria-describedby={fieldError ? errorId : undefined}
										className={cn(
											"h-11 text-center text-lg font-mono font-medium",
											fieldError && "border-destructive focus-visible:ring-destructive",
										)}
										{...boundaryInputProps}
									/>

									{fieldError ? (
										<p id={errorId} className="text-destructive text-xs">
											{fieldError.message}
										</p>
									) : (
										<p className="text-muted-foreground text-xs leading-relaxed">{description}</p>
									)}
								</div>
							);
						})}
					</div>
				</div>

				{/* ── Keyword Lists ── */}
				<div className="space-y-3">
					<div className="flex items-baseline gap-2.5">
						<h2 className="text-sm font-semibold">{t("complexityRouter.keywordLists")}</h2>
						<span className="text-muted-foreground text-xs">{t("complexityRouter.keywordListsDescription")}</span>
					</div>

					<div className="grid gap-3 md:grid-cols-2">
						{KEYWORD_LIST_DEFINITIONS.map(({ key }) => {
							const fieldError = keywordErrors?.[key as KeywordListKey];
							const errorId = `keywords-${key}-error`;
							const camelKey = snakeToCamel(key);
							const labelKey = `complexityRouter.validation.${camelKey}Label`;
							const emptyKey = `complexityRouter.validation.${camelKey}CannotBeEmptyShort`;
							const descriptionKey = `complexityRouter.validation.${camelKey}Description`;
							return (
								<div key={key} className="bg-card relative overflow-hidden rounded-sm border">
									<Controller
										control={control}
										name={`keywords.${key}` as const}
										rules={{
											validate: (value) => (value.length > 0 ? true : t(emptyKey)),
										}}
										render={({ field }) => (
											<div className="space-y-2 p-4 pl-5">
												<div className="flex items-center justify-between">
													<span className="text-xs font-medium">{t(labelKey)}</span>
													<span className="text-muted-foreground font-mono text-[11px] tabular-nums">
														{field.value.length} {field.value.length === 1 ? t("complexityRouter.entry") : t("complexityRouter.entries")}
													</span>
												</div>
												<p className="text-muted-foreground text-xs leading-relaxed">{t(descriptionKey)}</p>
												<TagInput
													data-testid={`complexity-router-keywords-${testIdPart(key)}-input`}
													value={field.value}
													onValueChange={field.onChange}
													collapsedTagLimit={KEYWORD_COLLAPSED_LIMIT}
													expandButtonTestId={`complexity-router-keywords-${testIdPart(key)}-expand-button`}
													placeholder={t("complexityRouter.typeKeywordPlaceholder")}
													aria-invalid={fieldError ? true : undefined}
													aria-describedby={fieldError ? errorId : undefined}
													className={cn(fieldError && "border-destructive")}
												/>
												{fieldError && (
													<p id={errorId} className="text-destructive text-xs">
														{fieldError.message}
													</p>
												)}
											</div>
										)}
									/>
								</div>
							);
						})}
					</div>
				</div>

				{/* ── Submit error ── */}
				{submitError && (
					<div
						role="alert"
						className="border-destructive/40 bg-destructive/10 text-destructive rounded-sm border px-3 py-2 font-mono text-sm"
					>
						{submitError}
					</div>
				)}

				{/* ── Action footer ── */}
				<div className="bg-card sticky bottom-0 z-10 flex flex-wrap items-center justify-end gap-2.5 border-t py-4">
					<Button
						data-testid="complexity-router-restore-defaults-button"
						type="button"
						variant="ghost"
						size="sm"
						onClick={() => setRestoreDialogOpen(true)}
						disabled={!canUpdate || isSaving || isResetting}
					>
						{isResetting ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <RotateCcw className="h-3.5 w-3.5" />}
						{t("complexityRouter.restoreDefaults")}
					</Button>
					<Button
						data-testid="complexity-router-discard-changes-button"
						type="button"
						variant="outline"
						size="sm"
						onClick={handleDiscard}
						disabled={!isDirty || isSaving || isResetting || isFetching}
					>
						{t("complexityRouter.discardChanges")}
					</Button>
					<Button
						data-testid="complexity-router-save-changes-button"
						type="submit"
						size="sm"
						disabled={!canUpdate || !isDirty || isSaving || isResetting || (isSubmitted && hasErrors)}
					>
						{isSaving ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
						{isSaving ? t("complexityRouter.saving") : t("complexityRouter.saveChanges")}
					</Button>
				</div>
			</form>

			<AlertDialog open={restoreDialogOpen} onOpenChange={setRestoreDialogOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t("complexityRouter.restoreDefaultsTitle")}</AlertDialogTitle>
						<AlertDialogDescription>{t("complexityRouter.restoreDefaultsDescription")}</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel
							data-testid="complexity-router-restore-cancel-button"
							onClick={() => setRestoreDialogOpen(false)}
							disabled={isResetting}
						>
							{t("common.action.cancel")}
						</AlertDialogCancel>
						<AlertDialogAction
							data-testid="complexity-router-restore-confirm-button"
							onClick={() => {
								setRestoreDialogOpen(false);
								handleRestoreDefaults();
							}}
							disabled={!canUpdate || isResetting}
						>
							{t("complexityRouter.restoreDefaults")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</ScrollArea>
	);
}
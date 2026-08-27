import type { DefaultParamDefinition } from "@/lib/types/config";

/**
 * Default-parameters model gating helpers, mirroring the backend registry
 * (core/schemas/defaultparams.go). A default param only applies to models that
 * match its `model_patterns` (case-insensitive exact or substring); empty
 * patterns mean all models of the provider.
 */

export function modelMatchesDefaultParamPatterns(model: string, patterns: string[] | undefined): boolean {
	if (!patterns || patterns.length === 0) return true;
	if (!model) return false;
	const lower = model.toLowerCase();
	return patterns.some((p) => {
		if (!p) return false;
		const lp = p.toLowerCase();
		return lower === lp || lower.includes(lp);
	});
}

/** True if the provider exposes any default param usable by `model`. */
export function providerModelHasDefaultParams(definitions: DefaultParamDefinition[] | undefined, model: string): boolean {
	return (definitions ?? []).some((d) => modelMatchesDefaultParamPatterns(model, d.model_patterns));
}

/** Filter the provider's definitions down to those the given model supports. */
export function definitionsForModel(definitions: DefaultParamDefinition[], model: string): DefaultParamDefinition[] {
	return definitions.filter((d) => modelMatchesDefaultParamPatterns(model, d.model_patterns));
}
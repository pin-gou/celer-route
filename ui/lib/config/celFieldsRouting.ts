/**
 * CEL Fields Configuration for Routing Rules
 * Defines available fields for building routing rule expressions
 */

import { getProviderLabel } from "@/lib/constants/logs";
import { COMPLEXITY_TIER_VALUES } from "@/lib/types/complexityRouter";

export type FieldGroup = "request" | "metadata" | "usage";

export const FIELD_GROUP_ORDER: FieldGroup[] = ["request", "metadata", "usage"];

export interface CELFieldDefinition {
	name: string;
	label: string;
	group?: FieldGroup;
	placeholder?: string;
	inputType?: "text" | "select" | "keyValue" | "number";
	valueEditorType?:
		| "text"
		| "select"
		| "keyValue"
		| "number"
		| "textarea"
		| "budgetNumber"
		| ((operator: string) => "text" | "select" | "keyValue" | "number" | "textarea" | "budgetNumber");
	operators?: string[];
	defaultOperator?: string;
	defaultValue?: any;
	values?: Array<{ name: string; label: string; disabled?: boolean }>;
	metricOptions?: Array<{ name: string; label: string }>; // For budgetNumber type
	/** @deprecated Description moved to locale (`sheet.fieldDescription.<name>`); kept as fallback only. */
	description?: string;
}

export const baseRoutingFields: CELFieldDefinition[] = [
	{
		name: "model",
		group: "request",
		label: "Model",
		placeholder: "e.g., gpt-4, claude-3-sonnet",
		inputType: "text",
		valueEditorType: (operator: string) =>
			operator === "=" || operator === "!=" ? "select" : operator === "in" || operator === "notIn" ? "select" : "text",
		operators: ["=", "!=", "in", "notIn", "contains", "beginsWith", "endsWith", "matches"],
		defaultOperator: "=",
	},
	{
		name: "provider",
		group: "request",
		label: "Provider",
		placeholder: "Select provider",
		inputType: "select",
		valueEditorType: (operator: string) =>
			operator === "matches" ? "text" : operator === "in" || operator === "notIn" ? "select" : "select",
		operators: ["=", "!=", "in", "notIn", "matches", "null", "notNull"],
		defaultOperator: "=",
	},
	{
		name: "request_type",
		group: "request",
		label: "Request Type",
		placeholder: "Select request type",
		inputType: "select",
		valueEditorType: (operator: string) =>
			operator === "matches" ? "text" : operator === "in" || operator === "notIn" ? "select" : "select",
		operators: ["=", "!=", "in", "notIn", "matches"],
		defaultOperator: "=",
		values: [
			{ name: "text_completion", label: "Text Completion" },
			{ name: "text_completion_stream", label: "Text Completion (Streaming)" },
			{ name: "chat_completion", label: "Chat Completion" },
			{ name: "chat_completion_stream", label: "Chat Completion (Streaming)" },
			{ name: "responses", label: "Responses" },
			{ name: "responses_stream", label: "Responses (Streaming)" },
			{ name: "embedding", label: "Embeddings" },
			{ name: "image_generation", label: "Image Generation" },
			{ name: "image_generation_stream", label: "Image Generation (Streaming)" },
			{ name: "image_edit", label: "Image Edit" },
			{ name: "image_edit_stream", label: "Image Edit (Streaming)" },
			{ name: "image_variation", label: "Image Variation" },
			{ name: "speech", label: "Speech" },
			{ name: "speech_stream", label: "Speech (Streaming)" },
			{ name: "transcription", label: "Transcription" },
			{ name: "transcription_stream", label: "Transcription (Streaming)" },
			{ name: "count_tokens", label: "Count Tokens" },
			{ name: "rerank", label: "Rerank" },
			{ name: "video_generation", label: "Video Generation" },
		],
	},
	{
		name: "complexity_tier",
		group: "request",
		label: "Complexity Tier",
		placeholder: "Select complexity tier",
		inputType: "select",
		valueEditorType: "select",
		operators: ["=", "!=", "in", "notIn"],
		defaultOperator: "=",
		values: COMPLEXITY_TIER_VALUES.map((tier) => ({ name: tier, label: tier.charAt(0) + tier.slice(1).toLowerCase() })),
	},
	{
		name: "headers",
		group: "metadata",
		label: "Request Header",
		placeholder: "e.g., authorization, x-custom-header (use lowercase)",
		inputType: "keyValue",
		valueEditorType: "keyValue",
		operators: ["=", "!=", "contains", "beginsWith", "endsWith", "matches", "null", "notNull"],
		defaultOperator: "=",
	},
	{
		name: "params",
		group: "metadata",
		label: "URL Query Parameter",
		placeholder: "e.g., api_key, user_id",
		inputType: "keyValue",
		valueEditorType: "keyValue",
		operators: ["=", "!=", "contains", "beginsWith", "endsWith", "matches", "null", "notNull"],
		defaultOperator: "=",
	},
	{
		name: "tokens_used",
		group: "usage",
		label: "Token Usage (%)",
		placeholder: "e.g., 80",
		inputType: "text",
		valueEditorType: "number",
		operators: ["=", "!=", ">", "<", ">=", "<="],
		defaultOperator: ">=",
	},
	{
		name: "request",
		group: "usage",
		label: "Request Usage (%)",
		placeholder: "e.g., 80",
		inputType: "text",
		valueEditorType: "number",
		operators: ["=", "!=", ">", "<", ">=", "<="],
		defaultOperator: ">=",
	},
	{
		name: "budget_used",
		group: "usage",
		label: "Budget Usage (%)",
		placeholder: "e.g., 50",
		inputType: "text",
		valueEditorType: "number",
		operators: ["=", "!=", ">", "<", ">=", "<="],
		defaultOperator: ">=",
	},
];

/**
 * Get routing fields with dynamic providers and models
 * Provider field values are populated dynamically from available providers
 * Metric options for rate limits and budget are populated from available providers and models
 */
export function getRoutingFields(providers: string[] = [], models: string[] = []): CELFieldDefinition[] {
	// Create provider field values
	const providerValues =
		providers.length > 0
			? providers.map((provider) => ({
					name: provider,
					label: getProviderLabel(provider),
				}))
			: [{ name: "_no_providers", label: "No providers configured", disabled: true }];

	// Create model field values
	const modelValues =
		models.length > 0
			? models.map((model) => ({
					name: model,
					label: model,
				}))
			: [];

	// Create metric options for scope input: providers + models
	const scopeOptions = [
		{ name: "", label: "(provider-level)" }, // Empty scope for provider-level
		...providers.map((provider) => ({
			name: provider,
			label: `${provider} (provider)`,
		})),
		...models.map((model) => ({
			name: model,
			label: `${model} (model)`,
		})),
	];

	// Update provider field with dynamic values and rate limit/budget fields with scope options
	const fieldsWithDynamicValues = baseRoutingFields.map((field) => {
		if (field.name === "provider") {
			return {
				...field,
				values: providerValues,
			};
		}
		if (field.name === "model") {
			return {
				...field,
				values: modelValues,
			};
		}
		if (field.name === "tokens_used" || field.name === "request" || field.name === "budget_used") {
			return {
				...field,
				metricOptions: scopeOptions,
			};
		}
		return field;
	});

	return fieldsWithDynamicValues;
}

export const PROVIDER_DISPLAY_NAMES: Record<string, string> = {
	openai: "OpenAI",
	anthropic: "Anthropic",
	azure: "Azure OpenAI",
	gemini: "Google Gemini",
	vertex: "Vertex AI",
	cohere: "Cohere",
};
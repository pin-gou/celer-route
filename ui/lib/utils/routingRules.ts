/**
 * Routing Rules Utility Functions
 * Helper functions for CEL validation, formatting, and rule management
 */

import { RuleGroupType, RuleType } from "react-querybuilder";
import { RoutingRule } from "@/lib/types/routingRules";

/**
 * Validates if a CEL expression has basic correct syntax
 * @param expression - The CEL expression to validate
 * @returns true if expression appears syntactically valid
 */
export function isValidCELExpression(expression: string): boolean {
	if (!expression) {
		return true;
	}

	const trimmed = expression.trim();
	if (trimmed.length === 0 || trimmed === "true" || trimmed === "false") {
		return true;
	}

	// Check for basic syntax issues
	if (trimmed.includes(";;")) {
		return false;
	}

	// Check for matching brackets/parentheses
	const openBrackets = (trimmed.match(/[[{]/g) || []).length;
	const closeBrackets = (trimmed.match(/[\]}]/g) || []).length;
	const openParens = (trimmed.match(/\(/g) || []).length;
	const closeParens = (trimmed.match(/\)/g) || []).length;

	if (openBrackets !== closeBrackets || openParens !== closeParens) {
		return false;
	}

	return true;
}

/**
 * Formats a fallback string (provider/model) for display
 * @param fallback - The fallback string (e.g., "openai/gpt-4o")
 * @returns Formatted fallback string
 */
export function formatFallback(fallback: string): string {
	if (!fallback) return "";
	const parts = fallback.split("/");
	return parts.length === 2 ? `${parts[0].toUpperCase()} - ${parts[1]}` : fallback;
}

/**
 * Parses a fallback string into provider and model
 * @param fallback - The fallback string (e.g., "openai/gpt-4o")
 * @returns Object with provider and model, or null if invalid
 */
export function parseFallback(fallback: string): { provider: string; model: string } | null {
	if (!fallback) return null;
	const parts = fallback.split("/");
	if (parts.length !== 2) return null;
	return { provider: parts[0], model: parts[1] };
}

/**
 * Converts fallback array to string format for display/editing
 * @param fallbacks - Array of fallback strings
 * @returns Comma-separated string
 */
export function fallbacksToString(fallbacks?: string[]): string {
	if (!fallbacks || fallbacks.length === 0) return "";
	return fallbacks.join(", ");
}

/**
 * Converts comma-separated string to fallback array
 * @param str - Comma-separated fallback string
 * @returns Array of fallback strings
 */
export function stringToFallbacks(str: string): string[] {
	if (!str || str.trim().length === 0) return [];
	return str
		.split(",")
		.map((s) => s.trim())
		.filter((s) => s.length > 0);
}

/**
 * Truncates CEL expression for table display
 * @param expression - The CEL expression
 * @param maxLength - Maximum length (default 60)
 * @returns Truncated expression with ellipsis if needed
 */
export function truncateCELExpression(expression: string, maxLength: number = 60): string {
	if (!expression) return "";
	if (expression.length <= maxLength) return expression;
	return expression.substring(0, maxLength) + "...";
}

/**
 * Validates a provider/model combination
 * @param provider - The provider name
 * @param model - The model name (optional)
 * @returns Error message if invalid, empty string if valid
 */
export function validateProviderModel(provider: string, _model?: string): string {
	if (!provider || provider.trim().length === 0) {
		return "Provider is required";
	}
	return "";
}

/**
 * Generates a CSS class for priority badge color
 * @returns CSS class name for styling
 */
export function getPriorityBadgeClass(): string {
	return "bg-primary text-primary-foreground";
}

/**
 * Gets a user-friendly CEL operator from the expression
 * @param expression - The CEL expression
 * @returns Array of detected operators
 */
export function detectCELOperators(expression: string): string[] {
	const operators: string[] = [];
	if (!expression) return operators;

	// Common CEL operators
	const operatorPatterns = [
		{ regex: /==/, label: "Equals" },
		{ regex: /!=/, label: "Not equals" },
		{ regex: />=/, label: "Greater than or equal" },
		{ regex: /<=/, label: "Less than or equal" },
		{ regex: />/, label: "Greater than" },
		{ regex: /</, label: "Less than" },
		{ regex: /&&/, label: "AND" },
		{ regex: /\|\|/, label: "OR" },
		{ regex: /!(?!=)/, label: "NOT" },
		{ regex: /in\s/, label: "IN" },
		{ regex: /.matches\(/, label: "Regex" },
		{ regex: /.startsWith\(/, label: "StartsWith" },
		{ regex: /.contains\(/, label: "Contains" },
		{ regex: /.endsWith\(/, label: "EndsWith" },
	];

	operatorPatterns.forEach(({ regex, label }) => {
		if (regex.test(expression) && !operators.includes(label)) {
			operators.push(label);
		}
	});

	return operators;
}

/**
 * Internal type for a single routing rule condition (from query builder rules)
 */
interface ParsedRoutingCondition {
	type: "model" | "provider" | "header" | "param" | "request_type" | "skip";
	field: string;
	operator: string;
	value: string;
	key?: string;
}

/**
 * Recursively flattens query-builder rules into simple conditions.
 */
function flattenQueryConditions(query: RuleGroupType | undefined): ParsedRoutingCondition[] {
	if (!query || !Array.isArray(query.rules)) return [];

	const conditions: ParsedRoutingCondition[] = [];
	for (const rule of query.rules) {
		if ("combinator" in rule) {
			conditions.push(...flattenQueryConditions(rule as RuleGroupType));
			continue;
		}
		const r = rule as RuleType;
		const field = String(r.field ?? "");
		const operator = String(r.operator ?? "");
		const value = String(r.value ?? "");
		const keyMatch = field.match(/\["([^"]+)"\]/);

		if (field === "model" || field === "provider" || field === "request_type") {
			conditions.push({ type: field, field, operator, value });
		} else if (field === "headers" || field.startsWith("headers[")) {
			conditions.push({ type: "header", field, operator, value, key: keyMatch?.[1] });
		} else if (field === "params" || field.startsWith("params[")) {
			conditions.push({ type: "param", field, operator, value, key: keyMatch?.[1] });
		} else {
			conditions.push({ type: "skip", field, operator, value });
		}
	}
	return conditions;
}

/**
 * Maps CEL request types to their gateway HTTP endpoints.
 */
const REQUEST_TYPE_ENDPOINTS: Record<string, string> = {
	chat_completion: "/v1/chat/completions",
	chat_completion_stream: "/v1/chat/completions",
	text_completion: "/v1/completions",
	text_completion_stream: "/v1/completions",
	responses: "/v1/responses",
	responses_stream: "/v1/responses",
	embedding: "/v1/embeddings",
	image_generation: "/v1/images/generations",
	image_edit: "/v1/images/edits",
	image_variation: "/v1/images/variations",
	speech: "/v1/audio/speech",
	transcription: "/v1/audio/transcriptions",
	count_tokens: "/v1/chat/completions",
	rerank: "/v1/rerank",
	video_generation: "/v1/video/generations",
};

const DEFAULT_RESPONSE_MODEL = "gpt-4o-mini";

/**
 * Extracts a testable scalar value from a query-builder rule value.
 * Arrays (used by `in`/`notIn` operators) pick the first element.
 */
function scalarValue(value: any): string {
	if (Array.isArray(value)) return value.length > 0 ? String(value[0]) : "";
	return String(value ?? "");
}

/**
 * Generates a curl command that a user can copy and run to test whether an
 * incoming request would match this routing rule. Conditions that cannot be
 * expressed in a synthetic request (budget/rate-limit usage, complexity tier)
 * — or that would require a deliberately mismatching value (`!=`) — are
 * skipped and listed in a trailing comment.
 */
export function generateRoutingTestCommand(rule: RoutingRule, baseUrl?: string): string {
	const conditions = flattenQueryConditions(rule?.query);
	if (conditions.length === 0) return "";

	const url = baseUrl || (typeof window !== "undefined" ? window.location.origin : "");
	let endpoint = "/v1/chat/completions";
	let providerValue = "";
	let modelValue = "";
	const headerLines: string[] = [];
	const paramPairs: string[] = [];
	const skipped: string[] = [];

	for (const c of conditions) {
		if (c.type === "skip" || c.operator === "!=" || c.operator === "notIn" || c.operator === "null") {
			skipped.push(`${c.field} ${c.operator} ${c.value || "(empty)"}`);
			continue;
		}

		const value = scalarValue(c.value);

		switch (c.type) {
			case "model":
				modelValue = value;
				break;
			case "provider":
				providerValue = value;
				break;
			case "header":
				if (c.key) {
					if (c.operator === "notNull") {
						headerLines.push(`  -H "${c.key}: <value>" \\`);
					} else {
						headerLines.push(`  -H "${c.key}: ${value}" \\`);
					}
				}
				break;
			case "param":
				if (c.key) {
					paramPairs.push(`${encodeURIComponent(c.key)}=${encodeURIComponent(c.operator === "notNull" ? "<value>" : value)}`);
				}
				break;
			case "request_type":
				endpoint = REQUEST_TYPE_ENDPOINTS[value] || endpoint;
				break;
		}
	}

	// Provider is passed inline in the model string ("provider/model").
	const resolvedModel = providerValue
		? modelValue
			? `${providerValue}/${modelValue}`
			: `${providerValue}/${DEFAULT_RESPONSE_MODEL}`
		: modelValue;

	const queryString = paramPairs.length > 0 ? `?${paramPairs.join("&")}` : "";
	const fullUrl = `${url}${endpoint}${queryString}`;

	const body: Record<string, unknown> = {
		messages: [{ role: "user", content: "Hello" }],
	};
	if (resolvedModel) body.model = resolvedModel;

	const lines: string[] = [
		`curl -X POST ${fullUrl} \\`,
		`  -H "Content-Type: application/json" \\`,
		`  -H "Authorization: Bearer \${PG_API_KEY}" \\`,
	];
	if (rule.scope === "virtual_key" && rule.scope_id) {
		lines.push(`  -H "x-bf-vk: ${rule.scope_id}" \\`);
	}
	lines.push(...headerLines);
	lines.push(`  -d '${JSON.stringify(body, null, 2)}'`);

	if (skipped.length > 0) {
		lines.push("", `# Skipped untestable conditions: ${skipped.join(", ")}`);
	}

	return lines.join("\n");
}
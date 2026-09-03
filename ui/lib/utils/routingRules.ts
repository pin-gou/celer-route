/**
 * Routing Rules Utility Functions
 * Helper functions for CEL validation, formatting, and rule management
 */

import { RuleGroupType, RuleType } from "react-querybuilder";
import { RoutingRule } from "@/lib/types/routingRules";

/**
 * Returns true when a routing rule's model field uses an operator that does
 * NOT expose a literal name to /v1/models. Backed by the server-side
 * framework/routing.ExtractModelLiterals — only model == "x" and model in [...]
 * produce virtual-model entries on the wire, so any other operator should
 * warn the user at edit time.
 *
 * Used by the routing-rule sheet to surface a hint when the user picks a
 * non-literal model predicate. The function is informational — the editor
 * never blocks save, because dynamic rules still work at request time, they
 * just don't show up in the model catalog.
 */
export function detectNonLiteralModelOperator(input: { celExpression?: string; query?: RuleGroupType | null }): boolean {
	const cel = input.celExpression ?? "";
	if (cel) {
		if (hasNonLiteralModelPredicateInCEL(cel)) return true;
	}
	const query = input.query;
	if (query) {
		if (hasNonLiteralModelPredicateInQuery(query)) return true;
	}
	return false;
}

function hasNonLiteralModelPredicateInCEL(expr: string): boolean {
	// Find any `model OP ...` predicate and check whether OP exposes a
	// literal we can backfill into /v1/models.
	//
	// Only `model == "x"` and `model in ["x","y"]` produce literals; every
	// other operator (including != which is a reverse predicate) cannot be
	// enumerated.
	const m = expr.match(/\bmodel\s*(\.startsWith\(|\.endsWith\(|\.contains\(|\.matches\(|==|!=|!in|in\b)/);
	if (!m) return false;
	const op = m[1];
	if (op === "==" || op === "in") return false;
	return true;
}

function hasNonLiteralModelPredicateInQuery(rule: RuleGroupType): boolean {
	for (const child of rule.rules || []) {
		if ("rules" in child) {
			if (hasNonLiteralModelPredicateInQuery(child as RuleGroupType)) return true;
			continue;
		}
		const r = child as RuleType;
		if (r.field !== "model") continue;
		const op = r.operator;
		if (op !== "=" && op !== "==" && op !== "in") return true;
	}
	return false;
}

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
 * Resolves the HTTP endpoint path for a routing rule based on its `request_type`
 * conditions. Defaults to `/v1/chat/completions` when none is specified.
 */
export function resolveRoutingEndpoint(rule: RoutingRule): string {
	const conditions = flattenQueryConditions(rule?.query);
	for (const c of conditions) {
		if (c.type === "request_type" && c.operator !== "!=" && c.operator !== "notIn") {
			const value = scalarValue(c.value);
			if (value && REQUEST_TYPE_ENDPOINTS[value]) return REQUEST_TYPE_ENDPOINTS[value];
		}
	}
	return "/v1/chat/completions";
}

/**
 * Shared test-command derivation. Walks the rule's query once and returns the
 * pieces four language generators consume: endpoint path, model string,
 * header lines, query-string pairs, and skipped conditions.
 */
function buildRoutingRequestLines(rule: RoutingRule): {
	endpoint: string;
	resolvedModel: string;
	headerLines: string[];
	paramPairs: string[];
	skipped: string[];
} {
	const conditions = flattenQueryConditions(rule?.query);
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

	const resolvedModel = providerValue
		? modelValue
			? `${providerValue}/${modelValue}`
			: `${providerValue}/${DEFAULT_RESPONSE_MODEL}`
		: modelValue;

	return { endpoint, resolvedModel, headerLines, paramPairs, skipped };
}

function defaultBaseUrl(baseUrl?: string): string {
	return baseUrl || (typeof window !== "undefined" ? window.location.origin : "");
}

function fullUrl(baseUrl: string, endpoint: string, paramPairs: string[]): string {
	const queryString = paramPairs.length > 0 ? `?${paramPairs.join("&")}` : "";
	return `${baseUrl}${endpoint}${queryString}`;
}

function skippedComment(skipped: string[], prefix: string): string {
	return skipped.length > 0 ? `\n\n${prefix} Skipped untestable conditions: ${skipped.join(", ")}` : "";
}

/**
 * Options accepted by the four language-specific generators.
 * `vkValue` is the literal API key string; `vkName` is shown only as a comment.
 * `enforceAuth` indicates whether the gateway enforces API-key auth on
 * inference — when false, auth headers/keys are omitted from the snippet.
 */
interface CommandOptions {
	baseUrl?: string;
	vkValue?: string | null;
	vkName?: string | null;
	enforceAuth?: boolean;
}

/**
 * Generates a curl command that a user can copy and run to test whether an
 * incoming request would match this routing rule. Conditions that cannot be
 * expressed in a synthetic request (budget/rate-limit usage, complexity tier)
 * — or that would require a deliberately mismatching value (`!=`) — are
 * skipped and listed in a trailing comment.
 */
export function generateRoutingTestCommand(rule: RoutingRule, baseUrl?: string): string {
	return generateRoutingTestCommandCurl(rule, { baseUrl });
}

export function generateRoutingTestCommandCurl(rule: RoutingRule, opts: CommandOptions = {}): string {
	const conditions = flattenQueryConditions(rule?.query);
	if (conditions.length === 0) return "";

	const { endpoint, resolvedModel, headerLines, paramPairs, skipped } = buildRoutingRequestLines(rule);
	const url = defaultBaseUrl(opts.baseUrl);
	const targetUrl = fullUrl(url, endpoint, paramPairs);

	const body: Record<string, unknown> = {
		messages: [{ role: "user", content: "Hello" }],
	};
	if (resolvedModel) body.model = resolvedModel;

	const authValue = opts.vkValue ?? "${CELER_ROUTE_API_KEY}";
	const needsAuth = !!opts.vkValue || opts.enforceAuth === true;

	const lines: string[] = [`curl -X POST ${targetUrl} \\`, `  -H "Content-Type: application/json" \\`];
	if (needsAuth) {
		lines.push(`  -H "Authorization: Bearer ${authValue}" \\`);
	}
	if (rule.scope === "virtual_key" && rule.scope_id) {
		lines.push(`  -H "x-bf-vk: ${rule.scope_id}" \\`);
	}
	lines.push(...headerLines);
	lines.push(`  -d '${JSON.stringify(body, null, 2)}'`);

	return lines.join("\n") + skippedComment(skipped, "#");
}

export function generateRoutingTestCommandPython(rule: RoutingRule, opts: CommandOptions = {}): string {
	const conditions = flattenQueryConditions(rule?.query);
	if (conditions.length === 0) return "";

	const { endpoint, resolvedModel, paramPairs, skipped } = buildRoutingRequestLines(rule);
	const baseUrl = defaultBaseUrl(opts.baseUrl).replace(/\/$/, "");
	const targetUrl = fullUrl(baseUrl, endpoint, paramPairs);

	const apiKeyExpr = opts.vkValue ? `"${opts.vkValue}"` : `os.environ["CELER_ROUTE_API_KEY"]`;
	const needsAuth = !!opts.vkValue || opts.enforceAuth === true;
	const vkHeaderLine = rule.scope === "virtual_key" && rule.scope_id ? `    "x-bf-vk": "${rule.scope_id}",\n` : "";

	const body: Record<string, unknown> = {
		messages: [{ role: "user", content: "Hello" }],
	};
	if (resolvedModel) body.model = resolvedModel;
	const bodyStr = JSON.stringify(body, null, 4).replace(/\n/g, "\n");

	const lines: string[] = [
		...(needsAuth ? [`import os`] : []),
		`import httpx`,
		``,
		`headers = {`,
		vkHeaderLine,
		`    "Content-Type": "application/json",`,
	];
	if (needsAuth) {
		lines.push(`    "Authorization": f"Bearer ${apiKeyExpr}",`);
	}
	lines.push(
		`}`,
		``,
		`payload = ${bodyStr}`,
		``,
		`resp = httpx.post(`,
		`    "${targetUrl}",`,
		`    headers=headers,`,
		`    json=payload,`,
		`    timeout=30.0,`,
		`)`,
		`resp.raise_for_status()`,
		`print(resp.json())`,
	);

	return lines.join("\n") + skippedComment(skipped, "#");
}

export function generateRoutingTestCommandNode(rule: RoutingRule, opts: CommandOptions = {}): string {
	const conditions = flattenQueryConditions(rule?.query);
	if (conditions.length === 0) return "";

	const { endpoint, resolvedModel, paramPairs, skipped } = buildRoutingRequestLines(rule);
	const baseUrl = defaultBaseUrl(opts.baseUrl).replace(/\/$/, "");
	const baseForSdk = baseUrl.replace(/\/v1$/, "");

	const apiKeyValue = opts.vkValue ? `"${opts.vkValue}"` : "process.env.CELER_ROUTE_API_KEY";
	const needsAuth = !!opts.vkValue || opts.enforceAuth === true;
	const vkHeaderLine = rule.scope === "virtual_key" && rule.scope_id ? `    "x-bf-vk": "${rule.scope_id}",\n` : "";

	const body: Record<string, unknown> = {
		messages: [{ role: "user", content: "Hello" }],
	};
	if (resolvedModel) body.model = resolvedModel;
	const bodyStr = JSON.stringify(body, null, 2);

	const lines: string[] = [`import OpenAI from "openai";`, ``];
	if (needsAuth) {
		lines.push(`const client = new OpenAI({`, `  apiKey: ${apiKeyValue},`);
	} else {
		lines.push(`const client = new OpenAI({`);
	}
	lines.push(
		`  baseURL: "${baseForSdk}",`,
		`  defaultHeaders: {`,
		vkHeaderLine.replace(/\n$/, ""),
		`  },`,
		`});`,
		``,
		`const payload = ${bodyStr};`,
		``,
		`const resp = await fetch(payload, { method: "POST" });`,
		`const data = await resp.json();`,
		`console.log(data);`,
	);

	const cleaned = lines.filter((l, idx, arr) => !(l === "" && arr[idx - 1] === ""));
	return cleaned.join("\n") + skippedComment(skipped, "//");
}

export function generateRoutingTestCommandGo(rule: RoutingRule, opts: CommandOptions = {}): string {
	const conditions = flattenQueryConditions(rule?.query);
	if (conditions.length === 0) return "";

	const { endpoint, resolvedModel, paramPairs, skipped } = buildRoutingRequestLines(rule);
	const baseUrl = defaultBaseUrl(opts.baseUrl).replace(/\/$/, "");
	const baseForSdk = baseUrl.replace(/\/v1$/, "");

	const apiKeyExpr = opts.vkValue ? `"${opts.vkValue}"` : `os.Getenv("CELER_ROUTE_API_KEY")`;
	const needsAuth = !!opts.vkValue || opts.enforceAuth === true;
	const vkHeaderLine = rule.scope === "virtual_key" && rule.scope_id ? `\t\toption.WithHeader("x-bf-vk", "${rule.scope_id}"),` : "";

	const imports: string[] = [`\t"context"`, `\t"fmt"`, `\t"github.com/openai/openai-go"`, `\t"github.com/openai/openai-go/option"`];
	if (needsAuth && !opts.vkValue) {
		imports.push(`\t"os"`);
	}
	imports.sort();

	const clientArgs: string[] = [];
	if (vkHeaderLine) clientArgs.push(vkHeaderLine);
	clientArgs.push(`\t\toption.WithBaseURL("${baseForSdk}"),`);
	if (needsAuth) {
		clientArgs.push(`\t\toption.WithAPIKey(${apiKeyExpr}),`);
	}

	const lines: string[] = [
		`package main`,
		``,
		`import (`,
		...imports,
		`)`,
		``,
		`func main() {`,
		`\tclient := openai.NewClient(`,
		...clientArgs,
		`\t)`,
		`\tresp, err := client.Chat.Completions.New(`,
		`\t\tcontext.Background(),`,
		`\t\topenai.ChatCompletionNewParams{`,
		`\t\t\tModel: openai.F("${resolvedModel || "gpt-4o-mini"}"),`,
		`\t\t\tMessages: openai.F([]openai.ChatCompletionMessageParamUnion{`,
		`\t\t\t\topenai.UserMessage("Hello"),`,
		`\t\t\t}),`,
		`\t\t},`,
		`\t)`,
		`\tif err != nil {`,
		`\t\tpanic(err)`,
		`\t}`,
		`\tfmt.Println(resp.Choices[0].Message.Content)`,
		`}`,
	];

	return lines.join("\n") + skippedComment(skipped, "//");
}
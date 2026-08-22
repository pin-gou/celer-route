import { RuleGroupType, RuleType } from "react-querybuilder";

export type ExampleRuleKind = "model" | "provider" | "requestType";

export function buildExampleRule(kind: ExampleRuleKind): RuleType {
	switch (kind) {
		case "model":
			return {
				field: "model",
				operator: "contains",
				value: "gpt-",
			};
		case "provider":
			return {
				field: "provider",
				operator: "=",
				value: "openai",
			};
		case "requestType":
			return {
				field: "request_type",
				operator: "in",
				value: JSON.stringify(["chat_completion", "chat_completion_stream"]),
			};
	}
}

export function appendRuleToGroup(query: RuleGroupType, rule: RuleType): RuleGroupType {
	return {
		...query,
		rules: [...(query.rules || []), rule],
	};
}
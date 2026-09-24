import { createFileRoute } from "@tanstack/react-router";
import AlertRuleDetailPage from "./page";

export const Route = createFileRoute("/workspace/alerting/$ruleId")({
	component: AlertRuleDetailPage,
});
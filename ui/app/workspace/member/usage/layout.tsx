import { createFileRoute } from "@tanstack/react-router";
import MyUsagePage from "./page";

// Member-self usage page. The /workspace/member parent layout's
// loader handles the auth gate (see /workspace/member/layout.tsx).
export const Route = createFileRoute("/workspace/member/usage")({
	component: MyUsagePage,
});
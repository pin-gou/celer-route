import { createFileRoute } from "@tanstack/react-router";
import SetupGuidePage from "./page";

// Member-self setup guide. The /workspace/member parent layout's
// loader handles the auth gate (see /workspace/member/layout.tsx).
export const Route = createFileRoute("/workspace/member/setup-guide")({
	component: SetupGuidePage,
});
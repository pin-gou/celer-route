import { createFileRoute } from "@tanstack/react-router";
import MyKeysPage from "./page";

// Member-self VK list. Same auth gate as the landing portal — see
// /workspace/member/layout.tsx for the canonical explanation; the
// root layout's loader fires before this route renders.
export const Route = createFileRoute("/workspace/member/keys")({
	component: MyKeysPage,
});
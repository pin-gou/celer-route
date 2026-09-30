import { createFileRoute } from "@tanstack/react-router";
import UserDetailPage from "./page";

export const Route = createFileRoute("/workspace/governance/users/$userId")({
	component: UserDetailPage,
});
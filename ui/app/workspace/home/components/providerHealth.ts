import { ModelProvider } from "@/lib/types/config";

export type DotState = "ok" | "missing" | "degraded" | "error";

export const dotClass: Record<DotState, string> = {
	ok: "bg-emerald-500",
	missing: "bg-zinc-400",
	degraded: "bg-amber-500",
	error: "bg-red-500",
};

// Shared health-dot derivation used by both the "你的提供商" topology card and
// the configured rows of the free-tier recommendation card, so the same
// provider shows the same status color on both surfaces.
export function computeDotState(p: ModelProvider, keysCount?: number): DotState {
	if (p.is_key_less) {
		// Keyless providers (opencode, custom keyless) have no keys — infer
		// health from the operational status instead of key presence.
		return p.provider_status === "error" ? "missing" : p.status === "list_models_failed" ? "degraded" : "ok";
	}
	const count = keysCount ?? p.keys_count ?? 0;
	const enabled = (p.keys_enabled ?? true) && count > 0;
	return !enabled ? "missing" : p.last_error_at ? "error" : "ok";
}
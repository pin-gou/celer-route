/**
 * Extracts the HTTP status from an RTK Query error without assuming its shape.
 * Used to distinguish "plugin not loaded" (400) from a real failure: the
 * capacity/learning endpoints 400 when the plugin isn't active, which the UI
 * hides instead of surfacing an un-actionable error.
 */
export function getQueryErrorStatus(error: unknown): number | undefined {
	if (typeof error !== "object" || error === null || !("status" in error)) return undefined;
	const status = (error as { status?: unknown }).status;
	return typeof status === "number" ? status : undefined;
}
// Types for the home-page free-tier catalog feature.
//
// Mirrors the backend response shapes documented in design.md:
//   - GET /api/catalog/bundles?lang=<lang>

/**
 * A single provider inside a bundle. `apply_url` is an external sign-up page
 * (opened in a new tab); `is_keyless` marks providers that need no API key
 * (the configure dialog skips the key step for them).
 */
export interface BundleProviderEntry {
	provider: string;
	models: string[];
	apply_url?: string;
	apply_steps?: string[];
	is_keyless?: boolean;
	notes?: string;
}

/**
 * One operational "bundle" — a curated set of free providers/models.
 */
export interface BundleEntry {
	id: string;
	title: string;
	description: string;
	providers: BundleProviderEntry[];
}

/**
 * Response of GET /api/catalog/bundles. The backend always returns 200 with a
 * possibly empty `bundles` array even when the remote catalog fetch failed.
 */
export interface BundlesResponse {
	bundles: BundleEntry[];
	updated_at: string | null;
	version: string | null;
}
import type { ClientPlatform } from "@/lib/types/platform";

/** Detect the user's OS from the browser. Falls back to linux (SSR-safe). */
export function detectPlatform(): ClientPlatform {
	if (typeof window === "undefined") return "linux";
	const nav = navigator as Navigator & { userAgentData?: { platform?: string } };
	const ua = nav.userAgentData?.platform ?? navigator.platform ?? "";
	const p = ua.toLowerCase();
	if (p.includes("win")) return "windows";
	if (p.includes("mac")) return "macos";
	return "linux";
}

/** Home-directory prefix used in displayed config paths. */
export function userHomePrefix(platform: ClientPlatform): string {
	return platform === "windows" ? "%USERPROFILE%" : "~";
}

/** Join path parts under the platform's home prefix with the right separators. */
export function displayPath(platform: ClientPlatform, ...parts: string[]): string {
	const sep = platform === "windows" ? "\\" : "/";
	return `${userHomePrefix(platform)}${sep}${parts.join(sep)}`;
}
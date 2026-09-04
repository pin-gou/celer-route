/**
 * Operating systems a config template can target. The agent-setup page
 * detects the user's OS and lets them override it; generated config paths,
 * env-var recipes and in-app steps adapt to the choice.
 */
export type ClientPlatform = "macos" | "windows" | "linux";
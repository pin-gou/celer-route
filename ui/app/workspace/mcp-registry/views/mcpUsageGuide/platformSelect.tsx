import { PlatformSelect as SharedPlatformSelect } from "@/components/ui/platformSelect";
import type { HarnessPlatform } from "./types";

interface PlatformSelectProps {
	platform: HarnessPlatform;
	onPlatformChange: (platform: HarnessPlatform) => void;
}

/** Re-exports the shared OS picker with the MCP usage guide's test ids. */
export function PlatformSelect({ platform, onPlatformChange }: PlatformSelectProps) {
	return <SharedPlatformSelect platform={platform} onPlatformChange={onPlatformChange} testIdPrefix="mcp-usage-guide" />;
}
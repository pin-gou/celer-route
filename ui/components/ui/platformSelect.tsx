import type { ClientPlatform } from "@/lib/types/platform";
import { cn } from "@/lib/utils";
import { Check } from "lucide-react";

interface PlatformSelectProps {
	platform: ClientPlatform;
	onPlatformChange: (platform: ClientPlatform) => void;
	/** Prefix for `data-testid`s, so every surface can keep its own test ids. */
	testIdPrefix?: string;
}

const PLATFORMS: Array<{ value: ClientPlatform; label: string; icon: string }> = [
	{ value: "macos", label: "macOS", icon: "/images/platforms/mac.svg" },
	{ value: "windows", label: "Windows", icon: "/images/platforms/windows.svg" },
	{ value: "linux", label: "Linux", icon: "/images/platforms/linux.svg" },
];

export function PlatformSelect({ platform, onPlatformChange, testIdPrefix = "platform-select" }: PlatformSelectProps) {
	return (
		<div className="grid gap-2 sm:grid-cols-3" data-testid={`${testIdPrefix}-platform`}>
			{PLATFORMS.map((option) => (
				<button
					key={option.value}
					type="button"
					onClick={() => onPlatformChange(option.value)}
					className={cn(
						"flex h-9 cursor-pointer items-center gap-2 rounded-sm border px-3 py-2 text-left text-sm transition-[background-color,border-color,transform] duration-150 ease-out hover:bg-accent active:scale-[0.99]",
						platform === option.value && "border-primary bg-primary/5",
					)}
					aria-pressed={platform === option.value}
					data-testid={`${testIdPrefix}-platform-${option.value}`}
				>
					<img src={option.icon} alt="" aria-hidden="true" className="size-4 shrink-0" />
					<span className="font-medium">{option.label}</span>
					{platform === option.value && <Check className="ml-auto size-4 text-green-600" />}
				</button>
			))}
		</div>
	);
}
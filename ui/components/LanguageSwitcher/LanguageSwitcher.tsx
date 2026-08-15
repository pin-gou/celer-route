import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdownMenu";
import { useLocale } from "@/lib/i18n/useLocale";
import { LOCALE_LABELS } from "@/lib/i18n/config";
import { Languages } from "lucide-react";

export function LanguageSwitcher() {
	const { locale, setLocale, availableLocales } = useLocale();

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					className="hover:text-primary text-muted-foreground flex cursor-pointer items-center gap-1.5 p-0.5 text-xs"
					data-testid="language-switcher-trigger"
					aria-label="Switch Language"
				>
					<Languages className="h-4 w-4" />
					<span className="hidden group-data-[collapsible=icon]:hidden">{locale}</span>
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end" className="min-w-[120px]">
				{availableLocales.map((l) => (
					<DropdownMenuItem
						key={l}
						data-testid={`language-switcher-option-${l}`}
						onSelect={() => setLocale(l)}
						className={l === locale ? "bg-accent font-medium" : ""}
					>
						{LOCALE_LABELS[l]}
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	);
}
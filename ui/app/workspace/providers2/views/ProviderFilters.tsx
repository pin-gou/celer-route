import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { Search } from "lucide-react";

export interface FilterState {
	search: string;
	health: "all" | "active" | "error";
}

export interface ProviderFiltersProps {
	filters: FilterState;
	onChange: (filters: FilterState) => void;
}

const healthChips = [
	{ key: "all" as const, label: "All", testId: "providers2-filter-chip-all" },
	{ key: "active" as const, label: "Active", testId: "providers2-filter-chip-active" },
	{ key: "error" as const, label: "Error", testId: "providers2-filter-chip-error" },
];

export function ProviderFilters({ filters, onChange }: ProviderFiltersProps) {
	return (
		<div className="flex items-center gap-4">
			{/* Search input */}
			<div className="relative flex-1">
				<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
				<Input
					data-testid="providers2-filter-search"
					aria-label="Search providers"
					placeholder="Search providers..."
					value={filters.search}
					onChange={(e) => onChange({ ...filters, search: e.target.value })}
					className="h-9 pl-9 text-sm"
				/>
			</div>

			{/* Health status chips */}
			<div className="flex items-center gap-1">
				{healthChips.map((chip) => (
					<button
						key={chip.key}
						data-testid={chip.testId}
						data-active={filters.health === chip.key ? "true" : "false"}
						aria-label={`Filter by ${chip.label} status`}
						onClick={() => onChange({ ...filters, health: chip.key })}
						className={cn(
							"rounded-md px-3 py-1.5 text-xs font-medium transition-colors",
							filters.health === chip.key ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:bg-muted/80",
						)}
					>
						{chip.label}
					</button>
				))}
			</div>
		</div>
	);
}
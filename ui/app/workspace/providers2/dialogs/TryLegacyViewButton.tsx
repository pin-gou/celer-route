import { Button } from "@/components/ui/button";
import { ArrowUpRight } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";

interface TryLegacyViewButtonProps {
	currentProvider?: string;
}

export default function TryLegacyViewButton({ currentProvider }: TryLegacyViewButtonProps) {
	const navigate = useNavigate();

	const handleClick = () => {
		const search: Record<string, string> = {};
		if (currentProvider) {
			search.provider = currentProvider;
		}
		navigate({ to: "/workspace/providers", search });
	};

	return (
		<Button variant="outline" size="sm" data-testid="providers2-try-legacy-view" onClick={handleClick} className="gap-1 text-xs">
			Try legacy view
			<ArrowUpRight className="h-3 w-3" />
		</Button>
	);
}
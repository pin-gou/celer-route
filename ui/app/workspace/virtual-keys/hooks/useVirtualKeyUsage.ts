import { Budget, RateLimit, VirtualKey } from "@/lib/types/governance";
import { getEffectiveBudgetLimit } from "@/lib/utils/governance";

/**
 * Resolves display budgets and rate limits for a virtual key.
 * In OSS builds, there are no access profiles or user assignments,
 * so the VK's own budgets and rate limits are used directly.
 */
export function useVirtualKeyUsage(vk: VirtualKey | null | undefined): {
	assignedUsers: { id: string; name?: string; email?: string }[];
	isManagedByProfile: boolean;
	managingProfile: undefined;
	hasApRateLimit: boolean;
	displayBudgets: Budget[] | undefined;
	displayRateLimit: RateLimit | undefined;
	isExhausted: boolean;
} {
	const isManagedByProfile = vk?.is_access_profile_managed ?? false;

	const displayBudgets: Budget[] | undefined = vk?.budgets;
	const displayRateLimit: RateLimit | undefined = vk?.rate_limit;

	const isExhausted =
		(displayBudgets?.some((b) => b.current_usage >= getEffectiveBudgetLimit(b)) ?? false) ||
		(displayRateLimit?.token_current_usage != null &&
			displayRateLimit?.token_max_limit != null &&
			displayRateLimit.token_current_usage >= displayRateLimit.token_max_limit) ||
		(displayRateLimit?.request_current_usage != null &&
			displayRateLimit?.request_max_limit != null &&
			displayRateLimit.request_current_usage >= displayRateLimit.request_max_limit);

	return { assignedUsers: [], isManagedByProfile, managingProfile: undefined, hasApRateLimit: false, displayBudgets, displayRateLimit, isExhausted };
}
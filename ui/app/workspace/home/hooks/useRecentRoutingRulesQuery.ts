import { useGetRecentRoutingRulesQuery } from "@/lib/store/apis/catalogApi";

/**
 * Wraps the catalog recent-routing-rules query with a default limit of 100.
 *
 * The heat footer under each bundle card consumes this hook (see
 * freeTierRecommendationCard.tsx) and slices the top rules locally.
 */
export const useRecentRoutingRulesQuery = (args: { limit?: number }) => useGetRecentRoutingRulesQuery({ limit: args.limit ?? 100 });
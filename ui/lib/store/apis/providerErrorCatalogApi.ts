import { baseApi } from "./baseApi";

export interface ProviderErrorCatalogResponse {
	provider: string;
	types: string[];
	codes: string[];
}

export const providerErrorCatalogApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getProviderErrorCatalog: builder.query<ProviderErrorCatalogResponse, string>({
			query: (provider) => ({
				url: "/cooldown/error-catalog",
				params: { provider },
			}),
		}),
	}),
});

export const { useGetProviderErrorCatalogQuery } = providerErrorCatalogApi;
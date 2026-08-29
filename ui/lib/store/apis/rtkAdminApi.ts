import {
	FilterCatalog,
	PreviewRequest,
	PreviewResponse,
	PutRtkConfigRequest,
	RtkConfigResponse,
	TestPayload,
	TestResult,
} from "@/lib/types/rtk";
import { baseApi } from "./baseApi";

// rtkApi holds the five RTK-specific endpoints. All of them live under
// /api/context/rtk/* except /api/compression/preview which is mounted at
// the top level. The split mirrors the OmniRoute surface so operators
// familiar with either tool see the same paths.
export const rtkAdminApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		// GET /api/context/rtk/config
		getRtkConfig: builder.query<RtkConfigResponse, void>({
			query: () => ({ url: "/context/rtk/config" }),
			providesTags: ["RtkConfig"],
		}),

		// PUT /api/context/rtk/config
		updateRtkConfig: builder.mutation<RtkConfigResponse, PutRtkConfigRequest>({
			query: (body) => ({ url: "/context/rtk/config", method: "PUT", body }),
			invalidatesTags: ["RtkConfig"],
		}),

		// GET /api/context/rtk/filters
		getRtkFilters: builder.query<FilterCatalog, void>({
			query: () => ({ url: "/context/rtk/filters" }),
			providesTags: ["RtkFilters"],
		}),

		// POST /api/context/rtk/test
		runRtkTest: builder.mutation<TestResult, TestPayload>({
			query: (body) => ({ url: "/context/rtk/test", method: "POST", body }),
		}),

		// POST /api/compression/preview
		previewCompression: builder.mutation<PreviewResponse, PreviewRequest>({
			query: (body) => ({ url: "/compression/preview", method: "POST", body }),
		}),

		// GET /api/context/rtk/raw-output/{id} — query against the raw text
		// directly. transformResponse drops the response wrapper so callers
		// can render the body as-is. The `?raw=1` query parameter requests
		// the verbatim file body (no sentinel prefix) so the ops viewer at
		// /workspace/plugins/rtk/raw-output renders clean text instead of
		// NUL-prefixed noise.
		getRtkRawOutput: builder.query<string, string>({
			query: (id) => ({
				url: `/context/rtk/raw-output/${id}`,
				params: { raw: "1" },
				responseHandler: "text",
			}),
		}),
	}),
});

export const {
	useGetRtkConfigQuery,
	useUpdateRtkConfigMutation,
	useGetRtkFiltersQuery,
	useRunRtkTestMutation,
	usePreviewCompressionMutation,
	useGetRtkRawOutputQuery,
	useLazyGetRtkRawOutputQuery,
} = rtkAdminApi;
import { useEffect, useState } from "react";

import { resolveEndpointUrl } from "@/lib/utils/testCommandSnippets";

// V1Model mirrors the shape returned by GET ${baseUrl}/v1/models. That
// surface is the OpenAI "list models" wire format — i.e. exactly what
// inference clients see when they ask for available models. The agent
// config generator writes `id` straight into the agent's `models` block,
// so the UI must show the same ids the agent will request.
export interface V1Model {
	id: string;
	name?: string;
	context_length?: number;
	max_input_tokens?: number;
	max_output_tokens?: number;
}

export interface V1ModelsState {
	models: V1Model[];
	isLoading: boolean;
	error: string | null;
	refetch: () => void;
}

interface V1ModelDTO {
	id?: string;
	name?: string;
	context_length?: number;
	max_input_tokens?: number;
	max_output_tokens?: number;
}

/**
 * useV1Models fetches the live catalog from ${baseUrl}/v1/models. The
 * page passes the user-selected virtual key in `Authorization: Bearer …`
 * so the result matches what the agent itself will see on
 * `enforce_auth_on_inference`-on gateways. Pass an empty apiKey to skip
 * the Authorization header (open-mode gateways).
 *
 * Refetches when baseUrl or apiKey change.
 */
export function useV1Models(baseUrl: string, apiKey: string | null): V1ModelsState {
	const [models, setModels] = useState<V1Model[]>([]);
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState<string | null>(null);
	const [nonce, setNonce] = useState(0);

	useEffect(() => {
		let cancelled = false;
		setIsLoading(true);
		setError(null);

		const url = stripV1(resolveWithBase(baseUrl)) + "/v1/models";
		const headers: Record<string, string> = { Accept: "application/json" };
		if (apiKey) headers.Authorization = `Bearer ${apiKey}`;

		fetch(url, { headers, credentials: "omit" })
			.then(async (resp) => {
				if (!resp.ok) {
					const text = await resp.text().catch(() => "");
					throw new Error(`GET ${url} returned ${resp.status}${text ? `: ${truncate(text)}` : ""}`);
				}
				const body = (await resp.json()) as { data?: V1ModelDTO[] };
				const list = Array.isArray(body.data) ? body.data : [];
				if (cancelled) return;
				setModels(
					list
						.filter((m): m is V1ModelDTO & { id: string } => typeof m.id === "string" && m.id.length > 0)
						.map((m) => ({
							id: m.id,
							name: m.name,
							context_length: m.context_length,
							max_input_tokens: m.max_input_tokens,
							max_output_tokens: m.max_output_tokens,
						})),
				);
				setIsLoading(false);
			})
			.catch((err) => {
				if (cancelled) return;
				setModels([]);
				setError(err instanceof Error ? err.message : String(err));
				setIsLoading(false);
			});

		return () => {
			cancelled = true;
		};
	}, [baseUrl, apiKey, nonce]);

	return {
		models,
		isLoading,
		error,
		refetch: () => setNonce((n) => n + 1),
	};
}

function resolveWithBase(baseUrl: string): string {
	if (baseUrl) return baseUrl;
	return resolveEndpointUrl();
}

function stripV1(url: string): string {
	return url.replace(/\/+$/, "").replace(/\/v1$/, "");
}

function truncate(s: string, max = 200): string {
	return s.length > max ? `${s.slice(0, max)}…` : s;
}
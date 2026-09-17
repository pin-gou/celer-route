#!/usr/bin/env python3
"""Regenerate the pricing / model-parameters datasheets from LiteLLM + local data.

celer-route ships four copies of the same catalog; this script keeps them in
sync. Existing (hand-curated) entries always win — LiteLLM only fills gaps, it
never overwrites.

Sources (in priority order):
  1. The current bundled datasheets (framework/modelcatalog/datasheet/fallback/)
     — hand-curated celer-route entries (opencode-zen, wafer, …) and prior
     syncs. These always win on conflict.
  2. LiteLLM model_prices_and_context_window.json — fetched live, or a local
     snapshot via --litellm-local. Fills in models / providers missing from
     the bundled copy (e.g. azure_ai, nvidia, qwencloud, baidu, zhipu, …).
  3. Optional hand-authored custom entries (scripts/sync-datasheet/custom.json)
     for celer-route providers that have no upstream source at all
     (runware, alibaba_tokenplan, …). Pass --custom PATH.

Artifacts written (identical content, two encodings for the params file):
  framework/modelcatalog/datasheet/fallback/datasheet.json
  framework/modelcatalog/datasheet/fallback/model-parameters.json.gz
  website/static/datasheet/datasheet.json
  website/static/datasheet/model-parameters.json

Usage:
  python3 scripts/sync-datasheet/sync-datasheet.py            # live fetch, fill missing providers
  python3 scripts/sync-datasheet/sync-datasheet.py --only-providers azure_ai,nvidia
  python3 scripts/sync-datasheet/sync-datasheet.py --only-providers all
  python3 scripts/sync-datasheet/sync-datasheet.py --litellm-local /tmp/prices.json --dry-run
  python3 scripts/sync-datasheet/sync-datasheet.py --litellm-local /tmp/prices.json --custom custom.json
"""

import argparse
import gzip
import json
import os
import sys
import urllib.request
from collections import Counter

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))

FALLBACK_PRICING = os.path.join(REPO_ROOT, "framework/modelcatalog/datasheet/fallback/datasheet.json")
FALLBACK_PARAMS = os.path.join(REPO_ROOT, "framework/modelcatalog/datasheet/fallback/model-parameters.json.gz")
STATIC_PRICING = os.path.join(REPO_ROOT, "website/static/datasheet/datasheet.json")
STATIC_PARAMS = os.path.join(REPO_ROOT, "website/static/datasheet/model-parameters.json")

LITELLM_URL = (
    "https://raw.githubusercontent.com/BerriAI/litellm/main/"
    "model_prices_and_context_window.json"
)
LITELLM_FALLBACK_SOURCE = "https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json"

# celer-route providers that the bundled datasheet currently has no rows for.
# Default harvest set; --only-providers overrides it ("all" = every mapped provider).
DEFAULT_TARGETS = [
    "360ai", "alibaba", "alibaba_tokenplan", "antling", "azure_ai", "baichuan",
    "baidu", "byteplus", "coze", "coze_cn", "iflytek", "internlm", "minimax_cn",
    "modelscope", "nvidia", "parasail", "qiniu", "qwencloud", "runware",
    "sensenova", "sgl", "siliconflow", "stepfun", "vertex", "vllm",
    "xiaomi_mimo", "yi", "zhipu",
]

# celer-route canonical provider -> LiteLLM model_prices provider name(s).
# Empty list = no upstream source; entries for these must live in --custom.
# "vertex" additionally matches any litellm_provider starting with "vertex_ai"
# (vertex_ai-embedding-models, vertex_ai-anthropic_models, …).
PROVIDER_MAP = {
    "360ai": ["360ai"],
    "alibaba": ["alibaba", "dashscope", "qwen_ai_platform"],
    "alibaba_tokenplan": [],
    "anthropic": ["anthropic"],
    "antling": ["antling"],
    "azure": ["azure"],
    "azure_ai": ["azure_ai"],
    "baichuan": ["baichuan"],
    "baidu": ["baidu"],
    "bedrock": ["bedrock", "bedrock_converse"],
    "bedrock_mantle": ["bedrock_mantle"],
    "byteplus": ["byteplus"],
    "cerebras": ["cerebras"],
    "cohere": ["cohere", "cohere_chat"],
    "coze": ["coze"],
    "coze_cn": ["coze_cn"],
    "deepinfra": ["deepinfra"],
    "deepseek": ["deepseek"],
    "elevenlabs": ["elevenlabs"],
    "fireworks": ["fireworks_ai", "fireworks_ai-embedding-models"],
    "gemini": ["gemini"],
    "gmicloud": ["gmi"],
    "groq": ["groq"],
    "huggingface": ["huggingface"],
    "hyperbolic": ["hyperbolic"],
    "iflytek": ["iflytek"],
    "internlm": ["internlm"],
    "minimax": ["minimax"],
    "minimax_cn": ["minimax_cn"],
    "mistral": ["mistral"],
    "modelscope": ["modelscope"],
    "moonshot": ["moonshot"],
    "nebius": ["nebius"],
    "nvidia": ["nvidia_nim"],
    "ollama": ["ollama"],
    "opencode": [],
    "openrouter": ["openrouter"],
    "parasail": ["parasail"],
    "perplexity": ["perplexity"],
    "qiniu": ["qiniu"],
    "qwencloud": ["qwencloud"],
    "replicate": ["replicate"],
    "runware": [],
    "runway": ["runwayml"],
    "sambanova": ["sambanova"],
    "sarvam": ["sarvam"],
    "sensenova": ["sensenova"],
    "sgl": ["sglang"],
    "siliconflow": ["siliconflow"],
    "stepfun": ["stepfun"],
    "tencent": ["tencent"],
    "together": ["together_ai"],
    "vertex": ["vertex_ai"],
    "vllm": ["vllm", "vllm_v1"],
    "volcengine": ["volcengine"],
    "wafer": [],
    "watsonx": ["watsonx"],
    "xai": ["xai"],
    "xiaomi_mimo": ["xiaomi"],
    "yi": ["yi", "01ai"],
    "zai": ["zai"],
    "zhipu": ["zhipu"],
}

# Datasheet Entry / Options JSON field whitelist (mirrors
# framework/modelcatalog/datasheet/types.go). Anything else LiteLLM carries
# (supports_*, deprecation_date, …) is dropped from the pricing catalog.
PRICING_FIELDS = [
    "base_model", "provider", "mode", "context_length", "max_input_tokens",
    "max_output_tokens", "is_deprecated",
    # text
    "input_cost_per_token", "output_cost_per_token",
    "input_cost_per_token_batches", "output_cost_per_token_batches",
    "input_cost_per_token_priority", "output_cost_per_token_priority",
    "input_cost_per_token_flex", "output_cost_per_token_flex",
    "input_cost_per_token_fast", "output_cost_per_token_fast",
    "input_cost_per_character",
    # 128k tier
    "input_cost_per_token_above_128k_tokens", "input_cost_per_image_above_128k_tokens",
    "input_cost_per_video_per_second_above_128k_tokens",
    "input_cost_per_audio_per_second_above_128k_tokens",
    "output_cost_per_token_above_128k_tokens",
    # 200k tier
    "input_cost_per_token_above_200k_tokens", "input_cost_per_token_above_200k_tokens_priority",
    "output_cost_per_token_above_200k_tokens", "output_cost_per_token_above_200k_tokens_priority",
    # 272k tier
    "input_cost_per_token_above_272k_tokens", "input_cost_per_token_above_272k_tokens_priority",
    "input_cost_per_token_flex_above_272k_tokens",
    "output_cost_per_token_above_272k_tokens", "output_cost_per_token_above_272k_tokens_priority",
    "output_cost_per_token_flex_above_272k_tokens",
    # cache
    "cache_creation_input_token_cost", "cache_read_input_token_cost",
    "cache_creation_input_token_cost_above_200k_tokens",
    "cache_read_input_token_cost_above_200k_tokens",
    "cache_read_input_token_cost_above_200k_tokens_priority",
    "cache_creation_input_token_cost_above_1hr",
    "cache_creation_input_token_cost_above_1hr_above_200k_tokens",
    "cache_creation_input_audio_token_cost",
    "cache_read_input_token_cost_priority", "cache_read_input_token_cost_flex",
    "cache_read_input_image_token_cost",
    "cache_read_input_token_cost_above_272k_tokens",
    "cache_read_input_token_cost_above_272k_tokens_priority",
    "cache_read_input_token_cost_flex_above_272k_tokens",
    "cache_creation_input_token_cost_above_272k_tokens",
    "cache_creation_input_token_cost_flex",
    "cache_creation_input_token_cost_flex_above_272k_tokens",
    "cache_creation_input_token_cost_priority",
    "cache_creation_input_token_cost_fast", "cache_creation_input_token_cost_above_1hr_fast",
    "cache_read_input_token_cost_fast",
    # image
    "input_cost_per_image", "input_cost_per_pixel", "output_cost_per_image",
    "output_cost_per_pixel", "output_cost_per_image_premium_image",
    "output_cost_per_image_above_512_and_512_pixels",
    "output_cost_per_image_above_512_and_512_pixels_and_premium_image",
    "output_cost_per_image_above_1024_and_1024_pixels",
    "output_cost_per_image_above_1024_and_1024_pixels_and_premium_image",
    "output_cost_per_image_above_2048_and_2048_pixels",
    "output_cost_per_image_above_4096_and_4096_pixels",
    "output_cost_per_image_low_quality", "output_cost_per_image_medium_quality",
    "output_cost_per_image_high_quality", "output_cost_per_image_auto_quality",
    "input_cost_per_image_token", "output_cost_per_image_token",
    # audio/video
    "input_cost_per_audio_token", "input_cost_per_audio_per_second",
    "input_cost_per_second", "input_cost_per_video_per_second",
    "output_cost_per_audio_token", "output_cost_per_video_per_second",
    "output_cost_per_second",
    # other
    "search_context_cost_per_query", "code_interpreter_cost_per_session",
    "inference_geo_us_multiplier", "cost_per_request",
    # OCR
    "ocr_cost_per_page", "annotation_cost_per_page",
    # provenance (kept as an extra field; Go ignores unknown fields)
    "source",
]

# supports_* capability flags copied from LiteLLM into generated
# model-parameters entries (modelParametersParseResult-compatible).
SUPPORTS_KEYS = [
    "supports_assistant_prefill", "supports_function_calling",
    "supports_parallel_function_calling", "supports_tool_choice",
    "supports_reasoning", "supports_response_schema",
    "supports_reasoning_with_tool_calls", "supports_service_tier",
    "supports_prompt_caching", "supports_web_search",
]

MODE_ENDPOINTS = {
    "chat": ["/v1/chat/completions"],
    "completion": ["/v1/completions"],
    "embedding": ["/v1/embeddings"],
    "audio_speech": ["/v1/audio/speech"],
    "audio_transcription": ["/v1/audio/transcriptions"],
    "image_generation": ["/v1/images/generations"],
    "image_edit": ["/v1/images/edits"],
    "video_generation": ["/v1/video/generations"],
    "rerank": ["/v1/rerank"],
    "moderation": ["/v1/moderations"],
}


def log(msg):
    print(msg, file=sys.stderr)


def load_json(path):
    with open(path, "rb") as f:
        return json.load(f)


def load_params(path):
    """Load model-parameters, decompressing when the path ends in .gz."""
    if path.endswith(".gz"):
        with gzip.open(path, "rb") as f:
            return json.load(f)
    return load_json(path)


def save_json(data, path, *, compact=True):
    text = json.dumps(data, ensure_ascii=False, sort_keys=True,
                      separators=(",", ":") if compact else None)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)
    log("wrote %s (%d bytes)" % (os.path.relpath(path, REPO_ROOT), len(text)))


def save_params_gz(data, path):
    payload = json.dumps(data, ensure_ascii=False, sort_keys=True,
                         separators=(",", ":")).encode("utf-8")
    # mtime=0 keeps the gzip byte-identical across runs for reproducible builds.
    gz = gzip.compress(payload, mtime=0)
    with open(path, "wb") as f:
        f.write(gz)
    log("wrote %s (%d -> %d bytes)" % (os.path.relpath(path, REPO_ROOT), len(payload), len(gz)))


def fetch(url):
    req = urllib.request.Request(url, headers={"User-Agent": "celer-route-sync-datasheet"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.load(resp)


def extract_model_name(model_key):
    """Strip a leading 'provider/' segment, matching Go's extractModelName."""
    if "/" in model_key:
        return model_key.split("/", 1)[1]
    return model_key


def litellm_provider(entry):
    return entry.get("litellm_provider") or entry.get("provider")


def provider_matches(entry_provider, celer_provider, litellm_names):
    if celer_provider == "vertex" and entry_provider is not None \
            and entry_provider.startswith("vertex_ai"):
        return True
    return entry_provider in litellm_names


def project_entry(le, celer_provider):
    """Map a LiteLLM entry to a datasheet Entry (whitelist projection)."""
    out = {}
    for f in PRICING_FIELDS:
        if f == "provider":
            continue
        if f in le:
            out[f] = le[f]
    out["provider"] = celer_provider
    if le.get("deprecation_date") or le.get("is_deprecated"):
        out["is_deprecated"] = True
    out.setdefault("source", LITELLM_FALLBACK_SOURCE)
    return out


def generate_params(le, celer_provider, pricing_key):
    """Build a minimal model-parameters entry for a newly added model.

    Existing entries are never regenerated — this only runs for models the
    pricing datasheet gained from LiteLLM. Capability flags are carried over;
    the detailed model_parameters[] list (labels/helpText) is left for
    hand-curation.
    """
    entry = {
        "mode": le.get("mode"),
        "base_model": le.get("base_model") or extract_model_name(pricing_key),
        "provider": celer_provider,
    }
    for k in ("max_input_tokens", "max_output_tokens", "max_tokens"):
        if le.get(k) is not None:
            entry[k] = le[k]
    mode = le.get("mode")
    if mode in MODE_ENDPOINTS:
        entry["supported_endpoints"] = list(MODE_ENDPOINTS[mode])
    for k in SUPPORTS_KEYS:
        if le.get(k) is not None:
            entry[k] = le[k]
    if "supports_tool_choice" not in entry and le.get("supports_function_calling"):
        entry["supports_tool_choice"] = True
    entry["source"] = le.get("source", LITELLM_FALLBACK_SOURCE)
    return entry


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--litellm-local", metavar="PATH",
                    help="Use a local LiteLLM snapshot instead of fetching %s" % LITELLM_URL)
    ap.add_argument("--custom", metavar="PATH",
                    help="Hand-authored datasheet entries (map of model key -> entry); "
                         "these always win over existing data.")
    ap.add_argument("--only-providers", metavar="LIST",
                    help="Comma-separated celer-route providers to fill, or 'all'. "
                         "Default: the providers missing from the bundled datasheet.")
    ap.add_argument("--dry-run", action="store_true",
                    help="Report the merge without writing anything.")
    ap.add_argument("--verbose", action="store_true")
    args = ap.parse_args()

    targets = DEFAULT_TARGETS
    if args.only_providers:
        if args.only_providers.strip().lower() == "all":
            targets = list(PROVIDER_MAP)
        else:
            targets = [p.strip() for p in args.only_providers.split(",") if p.strip()]

    # --- load the merge base (existing data always wins) ---
    pricing = load_json(FALLBACK_PRICING)
    params = load_params(FALLBACK_PARAMS)
    log("base: %d pricing entries, %d model-parameters entries" % (len(pricing), len(params)))

    # (stored model, stored provider, mode) set for dedup. Stored provider
    # mirrors Go's normalizeProvider.
    existing_keys = set()
    for key, e in pricing.items():
        model = extract_model_name(key)
        prov = e.get("provider", "")
        existing_keys.add((model, prov, e.get("mode")))
    existing_params = set(params)

    # --- load upstream LiteLLM ---
    if args.litellm_local:
        litellm = load_json(args.litellm_local)
        log("litellm: %d entries from %s" % (len(litellm), args.litellm_local))
    else:
        try:
            litellm = fetch(LITELLM_URL)
        except Exception as exc:  # noqa: BLE001 — report and exit cleanly
            log("error: failed to fetch %s: %s" % (LITELLM_URL, exc))
            log("hint: pass a local snapshot with --litellm-local PATH "
                "(e.g. a previously downloaded model_prices_and_context_window.json)")
            sys.exit(1)
        log("litellm: %d entries fetched from %s" % (len(litellm), LITELLM_URL))

    # --- load custom entries (win over everything) ---
    if args.custom:
        custom = load_json(args.custom)
        log("custom: %d entries from %s" % (len(custom), args.custom))
    else:
        custom = {}

    added_pricing = {}     # model key -> (celer provider, litellm entry)
    added_params = {}      # model-parameters key -> entry
    stats = Counter()

    for celer_provider in targets:
        if celer_provider not in PROVIDER_MAP:
            log("!! target provider %r has no entry in PROVIDER_MAP" % celer_provider)
            stats["no_mapping"] += 1
            continue
        litellm_names = PROVIDER_MAP[celer_provider]
        if not litellm_names:
            log("!! %-16s has no upstream source; needs hand-authored --custom entries" % celer_provider)
            stats["custom_only"] += 1
            continue
        n = 0
        for lkey, le in litellm.items():
            lp = litellm_provider(le)
            if not provider_matches(lp, celer_provider, litellm_names):
                continue
            if lkey in pricing:
                stats["dup_existing"] += 1
                continue
            stored_model = extract_model_name(lkey)
            mode = le.get("mode")
            if (stored_model, celer_provider, mode) in existing_keys:
                stats["dup_existing"] += 1
                continue
            pricing[lkey] = project_entry(le, celer_provider)
            existing_keys.add((stored_model, celer_provider, mode))
            added_pricing[lkey] = (celer_provider, le)
            n += 1
        log("  %-16s litellm=%s -> %d added" % (
            celer_provider, ",".join(litellm_names), n))
        stats["added"] += n

    # --- model-parameters: keep existing, add minimal entries for new models ---
    for lkey, (celer_provider, le) in added_pricing.items():
        key = extract_model_name(lkey)
        if key in existing_params:
            stats["params_existing"] += 1
            continue
        params[key] = generate_params(le, celer_provider, lkey)
        existing_params.add(key)
        added_params[key] = params[key]
        stats["params_added"] += 1
        if args.verbose:
            log("    params +%s (%s)" % (key, celer_provider))

    # --- merge custom entries ---
    for key, e in custom.items():
        pricing[key] = e
        stats["custom_added"] += 1
        pkey = extract_model_name(key)
        if pkey not in params:
            params[pkey] = {
                "mode": e.get("mode"),
                "base_model": e.get("base_model") or pkey,
                "provider": e.get("provider"),
            }
            for k in ("max_input_tokens", "max_output_tokens", "max_tokens"):
                if e.get(k) is not None:
                    params[pkey][k] = e[k]
            for k in SUPPORTS_KEYS:
                if e.get(k) is not None:
                    params[pkey][k] = e[k]
            params[pkey]["source"] = e.get("source", "custom")
            stats["custom_params_added"] += 1

    # --- report ---
    per_provider = Counter()
    for key, e in pricing.items():
        per_provider[e.get("provider")] += 1
    log("")
    log("merge summary: %d pricing (+%d from litellm, +%d custom), %d model-parameters (+%d)"
        % (len(pricing), stats["added"], stats["custom_added"],
           len(params), stats["params_added"] + stats["custom_params_added"]))
    log("new providers coverage (pricing rows):")
    for t in sorted(targets):
        log("  %-18s %d" % (t, per_provider.get(t, 0)))

    if args.dry_run:
        log("dry run — nothing written")
        return

    if not added_pricing and not custom:
        log("nothing to add; files left untouched")
        return

    save_json(pricing, FALLBACK_PRICING)
    save_params_gz(params, FALLBACK_PARAMS)
    save_json(pricing, STATIC_PRICING)
    save_json(params, STATIC_PARAMS)
    log("")
    log("next: run 'make test-core' or at least 'go test ./modelcatalog/datasheet/...' "
        "to verify the artifacts parse.")


if __name__ == "__main__":
    main()

# providercooldown

A Bifrost LLM plugin that suppresses (provider, key) pairs whose quota has
been exhausted for a configurable cooldown window. Without it, every new
request still hits the exhausted provider, fails, and only then trips the
fallback chain — wasting a 429 round trip on every request until the
provider's quota resets. With it, the failing key is removed from the
weighted-random selection pool for the cooldown duration and Bifrost
routes straight to the next available key or fallback.

## Behaviour

1. `PostLLMHook` inspects each terminal `BifrostError`. When the error
   looks like a quota / billing exhaustion (HTTP 402, or HTTP 429 with a
   matching message — see `IsQuotaExhausted` in `cooldown.go`), the
   `(provider, key_id)` of the last failed attempt is recorded in an
   in-memory map with an expiry timestamp.
2. The bifrost core `KeyPoolFilter` hook consumes that map and removes
   cooled keys from the weighted-random candidate pool. Subsequent
   requests skip the cooled key entirely — LB picks another key in the
   same provider's pool, or the request falls through to the configured
   `Fallbacks` chain.
3. The `RunGC` ticker prunes expired entries every minute. `IsCoolingDown`
   also prunes lazily on read.

Transient 429s ("too many requests, retry later") are intentionally NOT
treated as quota exhaustion and do not trigger a cooldown — they
self-heal on retry and over-cooling them would cause unnecessary
fallback churn.

The cooldown state is in-memory per process. Container / process restarts
wipe it. For multi-instance deployments, replace `CooldownState` with a
store backed by `BifrostConfig.KVStore` (interface-compatible).

## Configuration (config.json)

```json
{
  "plugins": [
    {
      "enabled": true,
      "name": "provider-cooldown",
      "config": {
        "default_ttl_seconds": 300,
        "ttl_overrides": { "openai": 30, "anthropic": 1200 },
        "escalation": { "enabled": false, "max_multiplier": 4 }
      }
    }
  ]
}
```

| Field | Type | Default | Notes |
|---|---|---|---|
| `enabled` | bool | — | set false to disable the plugin entirely |
| `name` | string | — | must be `"provider-cooldown"` |
| `config.default_ttl_seconds` | int | 300 | applied to every provider without an override; `<= 0` falls back to 300 |
| `config.ttl_overrides` | object | `{}` | map of provider name → seconds; non-positive entries ignored |
| `config.escalation.enabled` | bool | `false` | adaptive TTL escalation (see below); off = unchanged behaviour |
| `config.escalation.max_multiplier` | int | `4` | per-key TTL growth ceiling, clamped to `1..16` |
| `config.capacity` | object | absent | optional retrospective capacity gate (see below); absent or `enabled=false` = unchanged behaviour |

### Adaptive TTL escalation (optional, default off)

When `escalation.enabled` is true, cooldown TTLs grow on repeat failures:

- the same `(provider, key, model)` failing again within 300s doubles the TTL
  (1x → 2x → 4x … up to `max_multiplier`); a successful call clears the count;
- when 3+ distinct keys fail for the same provider/model within 60s, new marks
  start at 2x until that window goes quiet;
- escalated TTLs are capped at 1 hour and are never shorter than the base TTL.

It is expressed only as a longer TTL — no sleeps, no blocking, and no change to
the upstream request bytes.

### Retrospective capacity gate (optional, default off)

The passive cooldown reacts to failures that already happened. `capacity` adds
an optional **budget gate**: once our own traffic to a provider has spent a
configured amount in a rolling window, the *next* requests are short-circuited
before they go out.

This is deliberately retrospective — it never estimates tokens and never reads
the request body:

1. `PostLLMHook` adds each terminal request's **real** usage to a per-second
   bucket ring: the token count from the response usage, or
   `ExtraFields.BilledUsage` when a failed attempt was still billed. The request
   count advances on every terminal outcome (success or failure); tokens are 0
   when the provider reported none.
2. `PreProviderHook` sums the live buckets. Once the window's usage reaches
   `token_limit * headroom` (or `request_limit * headroom`), the attempt is
   short-circuited with a 429 `rate_limit_exceeded` whose message names the
   provider/model, the window and the seconds to wait.
3. The request that crosses the line still goes out — the same trade-off the
   passive cooldown makes — in exchange for zero estimation error and a
   threshold that is easy to reason about.

```json
{
  "config": {
    "default_ttl_seconds": 300,
    "ttl_overrides": {},
    "quota_patterns": ["insufficient_quota"],
    "capacity": {
      "enabled": true,
      "token_limit": 200000,
      "request_limit": 500,
      "window_seconds": 60,
      "headroom": 0.9,
      "scope": "provider",
      "action": "fallback"
    }
  }
}
```

| Field | Default | Notes |
|---|---|---|
| `capacity.enabled` | `false` | `false` = feature fully inert (no state, no lock) |
| `capacity.token_limit` | `0` | tokens per window; `0` = unlimited |
| `capacity.request_limit` | `0` | requests per window; `0` = unlimited |
| `capacity.window_seconds` | `60` | clamped to `10..3600` |
| `capacity.headroom` | `0.9` | gate engages at `limit * headroom`; clamped to `0.1..1.0` |
| `capacity.scope` | `"provider"` | `"provider"` shares one budget per provider; `"provider_model"` per provider/model |
| `capacity.action` | `"fallback"` | `"fallback"` lets the fallback chain take over; `"reject"` returns the 429 immediately |

Notes:

- The gate is inert unless **at least one** of `token_limit` / `request_limit`
  is positive.
- Rejected requests are not accounted, so a gate cannot extend its own window.
- Attempts that never reached a provider (this plugin's own short-circuit, the
  all-keys-in-cooldown short-circuit, cache hits) are not accounted.
- Streaming responses are accounted once, on the terminal chunk. A stream that
  dies without ever emitting a terminal marker is not accounted (it consumed
  an unknown amount of usage; under-counting is preferred over guessing).
- State is in-memory and per-process, and is dropped when the process restarts.
  Buckets are second-granular and expire lazily; the key map is swept at most
  once per window.

Provider names use Bifrost's `ModelProvider` enum (e.g. `"openai"`,
`"anthropic"`, `"bedrock"`).

## Build the deployment image

The `Dockerfile` in this directory builds a single arm64 alpine image
that contains a bifrost-http binary with the providercooldown plugin
statically linked in. The plugin is a regular Go package (not a
`-buildmode=plugin` .so), so it shares the bifrost-http process and
avoids the Go plugin ABI risk surface entirely.

Build from the repo root:

```bash
make -C plugins/providercooldown image \
  IMAGE=myco/bifrost \
  TAG=v1.0.0-cooldown
```

Or directly:

```bash
docker build -f plugins/providercooldown/Dockerfile \
  --build-arg VERSION=v1.0.0-cooldown \
  -t myco/bifrost:v1.0.0-cooldown \
  .
```

Multi-arch (arm64 + amd64) via buildx — pushes directly to the registry,
no local load:

```bash
make -C plugins/providercooldown buildx-image \
  IMAGE=myco/bifrost TAG=v1.0.0-cooldown \
  PLATFORMS=linux/arm64,linux/amd64
```

## Run

```bash
mkdir -p ./data
cp path/to/your/config.json ./data/config.json  # must include the plugins[] block above

docker run -d --name bifrost \
  --platform linux/arm64 \
  -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  -e OPENAI_API_KEY=sk-... \
  myco/bifrost:v1.0.0-cooldown

curl http://localhost:8080/health
```

`make -C plugins/providercooldown run` does the same with a default
`./data` directory and forwards `OPENAI_API_KEY` / `ANTHROPIC_API_KEY`
from your shell.

## Verify the plugin is wired

```bash
# the plugin should be in the active plugins list
curl http://localhost:8080/api/plugins | jq '.[] | select(.name == "provider-cooldown")'
```

To exercise the cooldown behaviour end-to-end, point a provider at a key
whose quota is exhausted and observe that subsequent requests route
around the key instead of burning another 429. The cooldown state is
in-memory and ephemeral; container restart clears it.

## How it's wired into bifrost-http

Three small changes in the bifrost repository:

1. `transports/celer-route-http/server/server.go` — adds an optional
   `KeyPoolFilter schemas.KeyPoolFilter` field on `BifrostHTTPServer`
   and threads it into the `bifrost.Init` call (and the subsequent
   `s.Client.ReloadConfig`).
2. `transports/celer-route-http/server/plugins.go` — registers
   `providercooldown` as a built-in plugin during `loadBuiltinPlugins`,
   calling `providercooldown.NewPlugin()` + `Init(config)` and wiring
   `plugin.State.AsFilter()` into `s.KeyPoolFilter`.
3. `transports/celer-route-http/lib/config.go` — adds `providercooldown.PluginName`
   to `builtinPluginNames` so the plugin admin API recognises it as
   built-in.

Plus `transports/go.mod` gets a `require` and `replace` for the local
plugin directory, which is what lets the Dockerfile build the plugin
into the main binary without a published release.

## Tests

```bash
cd plugins/providercooldown
go test -race -count=1 -timeout 60s ./...
```

Covers mark/expiry, TTL overrides, key-pool filter behaviour,
quota-exhaustion detection (`IsQuotaExhausted` covers 402 / 429 / 401 /
500 / message-based detection), plugin `PostLLMHook` wiring, and `Init`
config parsing.

## Limitations

- The plugin cannot read the upstream provider's `Retry-After` /
  `retry-after-ms` response header — those aren't surfaced through the
  `LLMPlugin` interface. The TTL is therefore a fixed configured value
  rather than provider-driven. Use `ttl_overrides` to tune per provider
  if the default 10 minutes doesn't match a known reset cadence.
- The cooldown state lives in-process. For multi-instance deployments
  replace `CooldownState` with a `KVStore`-backed implementation that
  satisfies the same `Mark` / `IsCoolingDown` semantics.

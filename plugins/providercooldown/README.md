# providercooldown

A Bifrost LLM plugin that suppresses (provider, key) pairs whose quota has
been exhausted for a configurable cooldown window. Without it, every new
request still hits the exhausted provider, fails, and only then trips the
fallback chain — wasting a 429 round trip on every request until the
provider's quota resets. With it, the failing key is removed from the
weighted-random selection pool for the cooldown duration and Bifrost
routes straight to the next available key or fallback.

## Behaviour

1. `PostLLMHook` inspects each terminal `BifrostError` and classifies it
   against the provider's `CooldownPolicy`: quota matches are checked first,
   then rate-limit matches. A provider with no policy of its own uses the
   built-in default for that provider (`schemas.DefaultCooldownPolicy`). When
   a rule fires, the `(provider, key_id)` — or `(provider, key_id, model)`
   when the rule's scope is `model` — of the last failed attempt is recorded
   in an in-memory map with an expiry timestamp.
2. The bifrost core `KeyPoolFilter` hook consumes that map and removes
   cooled keys from the weighted-random candidate pool. Subsequent
   requests skip the cooled key entirely — LB picks another key in the
   same provider's pool, or the request falls through to the configured
   `Fallbacks` chain.
3. The `RunGC` ticker prunes expired entries every minute. `IsCoolingDown`
   also prunes lazily on read.

A 429 that matches the rate-limit rule is cooled for that rule's TTL; an
error that only *looks* transient but matches no rule is left alone, so it
self-heals on retry without causing fallback churn.

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
        "quota_patterns": ["insufficient_quota"],
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
| `config.quota_patterns` | array | `[]` | extra lower-case substrings treated as quota exhaustion; required at the config level (falls back to the built-in list when empty) |
| `config.escalation.*` | object | absent | **plugin-level default** adaptive TTL escalation (see below); a provider's own `cooldown_policy.escalation` overrides it. Its `chronic` sub-object is the optional long-backoff for chronically bad keys |
| `config.capacity.*` | object | absent | **deprecated legacy fleet default** retrospective capacity gate (see below); a provider's own `cooldown_policy.capacity` overrides it. Kept for backward compatibility — the UI no longer edits it and only shows a read-only warning plus the fleet water level |
| `config.learning.*` | object | absent | **plugin-level default** read-only limit learner (see below); a provider's own `cooldown_policy.learning` overrides it |
| `config.health.*` | object | absent | **plugin-level default** health-aware key weighting (see below); a provider's own `cooldown_policy.health` overrides it |

### Plugin-level defaults vs per-provider overrides

The capability sections in the plugin config are **defaults**, not the only
place they can live. A provider carries its own section in
`schemas.CooldownPolicy` (set from the provider detail page in the UI). The
plugin-level `capacity` section is deprecated and no longer editable, so only
`escalation`, `learning` and `health` are still presented as fleet defaults:

- **section absent** → the provider inherits the plugin-level section; if that
  is absent too, the capability is off;
- **`{ "enabled": false }`** → explicitly off for this provider, even when the
  plugin default is on;
- **section present with values** → replaces the plugin-level section
  **wholesale** — there is no field-level merge.

Resolution lives in `effective.go` (`capabilityDefaults.resolveCapacity` /
`resolveEscalation` / `resolveLearning` / `resolveHealth`); core stamps the
provider policy on the request context before every attempt
(`BifrostContextKeyCooldownPolicy`). The config types and their bounds/defaults
are shared with the write API and live in `core/schemas/cooldownpolicy.go`.

Read-only endpoints exposed by the plugin (wired in
`transports/celer-route-http/handlers/cooldown.go`):

| Endpoint | Returns |
|---|---|
| `GET /api/plugins/provider-cooldown/state` | active cooldown entries |
| `GET /api/plugins/provider-cooldown/stats` | mark / suppressed counters, per provider and per provider+model |
| `GET /api/plugins/provider-cooldown/learned-limits` | learned per-window limits; each entry carries its own `window_seconds` (the top-level one is the plugin default, kept for compatibility) |
| `GET /api/plugins/provider-cooldown/capacity-usage` | live capacity-gate accounting per provider (or provider+model) with `source` = `default` \| `provider` |

### Adaptive TTL escalation (optional, default off)

When `escalation.enabled` is true, cooldown TTLs grow on repeat failures:

- the same `(provider, key, model)` failing again within 300s doubles the TTL
  (1x → 2x → 4x … up to `max_multiplier`); a successful call clears the count
  (see `escalation.chronic` below for the hysteresis that replaces this rule for
  chronically bad keys);
- when 3+ distinct keys fail for the same provider/model within 60s, new marks
  start at 2x until that window goes quiet;
- escalated TTLs are capped at 1 hour and are never shorter than the base TTL.

It is expressed only as a longer TTL — no sleeps, no blocking, and no change to
the upstream request bytes.

#### `escalation.chronic` — long backoff for chronically bad keys (default off)

Escalation alone is capped at `base_ttl × max_multiplier`: with the common
`ttl_seconds=30` + `max_multiplier=4` that tops out at 120s, so a key whose
quota is consumed outside this gateway is probed every two minutes — and each
probe is itself an upstream request that consumes the account's RPM. The
"success clears every counter" rule makes it worse: a key with a ~15% success
rate keeps resetting on its lucky probe, so it is never left alone.

`escalation.chronic` reuses the health EWMA (the same observations that feed
health-aware weighting) to detect that case and switch it to a long, jittered
backoff:

| Field | Default | Notes |
|---|---|---|
| `enabled` | `false` | Requires `escalation.enabled=true`; the combination with escalation off is rejected |
| `success_rate_threshold` | `0.3` | EWMA success rate below which a row with enough samples is chronic; strictly inside `(0, 1)` |
| `min_samples` | `10` | Observations required before the predicate may fire (1–1000) |
| `chronic_max_ttl_seconds` | `1800` | Long backoff, before jitter (30–86400). Never shortens a longer base/escalated TTL |
| `jitter_ratio` | `0` | `0.2` = ±20% random jitter so keys that failed together do not retry in lockstep. `0` disables jitter; `0.2` is the recommended production value |
| `recovery_successes` | `3` | Consecutive successes required to lift the chronic mark (1–1000) |

Behaviour:

- **Detection** — chronic is evaluated per `(provider, key, model)`, and once
  latched it survives a single success. Key-scope marks (no model) aggregate all
  model rows of that key, and any chronically bad model makes the key chronic at
  key scope (a bad model cannot hide behind a healthy sibling).
- **Long TTL** — while chronic, each failure records
  `chronic_max_ttl_seconds × (1 ± jitter_ratio)`, floored at the normal
  (escalated) TTL. The escalation/storm counters keep advancing as usual, so the
  state stays self-consistent when the key recovers.
- **Half-open probe, no scheduler** — when the long TTL lapses the key is
  selectable again (with health weighting it is de-prioritised but never
  unselectable), so the next natural rotation is the probe. A failed probe earns
  another long backoff; a successful one just serves traffic.
- **Hysteresis recovery** — one success does NOT clear the chronic mark or the
  key's escalation counters. Only `recovery_successes` consecutive successes do,
  after which the next failure starts from the original 1x TTL. A recovered
  upstream is therefore picked up within at most ~2× `chronic_max_ttl_seconds`.
- **Rollback** — `escalation.chronic.enabled=false` (or omitting the block)
  restores the pre-chronic behaviour byte-for-byte, including "success clears
  the counters".

Because sections are replaced wholesale, a provider-level `escalation` section
that omits `chronic` disables chronic for that provider even when the plugin
default enables it.

### RPM/TPM dimension split (optional, default off)

A rate-limit 429 can come from two different budgets, and they recover on very
different timescales:

- **RPM/RPS** (request rate) — the account's request budget is full. When it is
  consumed by traffic outside this gateway (the production SenseNova case) it
  does not clear until that external consumer stops, so the default token-rate
  TTL merely probes the key again seconds later, and each probe consumes
  another request from the same exhausted budget.
- **TPM** (token rate) — our own token throughput hit the window limit. The
  usual short TTL matches how fast the window drains.

The plugin derives the dimension from the error body (`dimension.go`,
`limiterDimensionOf`), case-insensitively, from the error type, code and
rendered message:

| Dimension | Recognised signals (production samples included) |
|---|---|
| `rpm` | `EndpointRPMExceeded`, `ModelAccountRpmRateLimitExceeded`, `rpm exhausted`, `rps exhausted`, `RequestBurstTooFast`, `Throttling.BurstRate`, and a generic `rate limit exceeded` that mentions no token/TPM wording. `rps` folds into this bucket: requests-per-second is still a request-rate limit |
| `tpm` | `EndpointTPMExceeded`, `ModelAccountTpmRateLimitExceeded`, `tpm exhausted`, `token rate limit`, `tokens per minute` |
| `unknown` | anything else. Falls back to `ttl_seconds` |

Token patterns are evaluated before request patterns, so a body carrying both
a generic "rate limit exceeded" phrase and a token budget signal resolves to
`tpm`; `EndpointTPMExceeded` can therefore never be misfiled as `rpm`.

Set the per-dimension TTLs on the **rate_limit** rule (they are rejected on a
quota rule — a quota exhaustion is not a rate window):

| Field | Default | Notes |
|---|---|---|
| `rpm_ttl_seconds` | `0` | TTL for `rpm` marks. `0` falls back to `ttl_seconds`. Ergonomics: 60–120s for an externally consumed RPM budget |
| `tpm_ttl_seconds` | `0` | TTL for `tpm` marks. `0` falls back to `ttl_seconds`. Leaving both at `0` restores the pre-B3 behaviour byte-for-byte |

Recommended SenseNova configuration (the provider page's cooldown editor can
set the same two fields):

```json
{
  "providers": {
    "sensenova": {
      "cooldown_policy": {
        "rate_limit": {
          "match": [{ "status_code": 429 }],
          "ttl_seconds": 30,
          "rpm_ttl_seconds": 90
        }
      }
    }
  }
}
```

#### Interaction with `retry_after_mode`

With either dimension TTL set, the dimension-specific value becomes the base
and the retry hint is combined with it as follows:

| `retry_after_mode` | no usable provider hint | usable provider hint |
|---|---|---|
| `off` (default) | base | `max(base, hint)` — the hint is a **lower bound** |
| `cap` | base | `min(base, hint)` |
| `exact` | base | `hint` |

`base` = `rpm_ttl_seconds` for an rpm mark, `tpm_ttl_seconds` for a tpm mark,
otherwise `ttl_seconds`. **This is a deliberate change to `off`**: previously
`off` ignored provider hints entirely, but a dimension TTL is a floor derived
from how the budget recovers, and a provider asking us to wait longer must
still be honoured. When both dimension fields are `0`, `off` keeps its exact
legacy meaning (fixed TTL, no header read). Hints remain bounded by
`maxProviderRetryAfterHint` (24h) in every branch.

The production SenseNova 429 carries only `X-Request-Id` and **no**
`retry-after`, so the dimension TTL — not the hint — is what makes this work;
the hint can only extend it further.

#### Learning decontamination

The same dimension is recorded on every learning sample. The learner still
records **all** dimensions (for diagnostics and the report's
`dimension_sample_counts`), but it estimates the token limit from **non-RPM
samples only**:

- an RPM rejection at `usedTokens≈0` says nothing about the token budget;
  including it dragged the median and the lower bound to ~0, which is the
  misleading `learned-limits` report the deployment produced;
- `sample_count` / `key_count` / `lower_bound` / `upper_bound` /
  `confidence` therefore describe the participating (tpm + unknown) samples;
- `limiter_dimension` is the dominant dimension among those participants; when
  every retained sample is an RPM rejection it reports `"rpm"`, and the entry
  stays `insufficient` with no estimate — the operator can tell the token
  estimate is unavailable instead of trusting a ~0;
- `dimension_sample_counts` (e.g. `{"rpm":12,"tpm":8,"unknown":2}`) is
  aggregated on each report from the bounded sample buffer; summing it
  reconstructs the total retained sample count.

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

Set as the **deprecated** plugin-level fleet default (a provider's own
`cooldown_policy.capacity` replaces it wholesale). This key is legacy
compatibility only: the plugin page no longer offers an editor for it, just a
warning with a "clear global default" action, and `GET
/api/plugins/provider-cooldown/capacity-usage` is surfaced there as a read-only
fleet water level:

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

### Read-only limit learning (optional, default off)

`learning` adds a **read-only observer** that learns a provider's effective
per-window token limit from its own rate-limit rejections. It never changes
any configuration — the only output is a report.

Why: our own request/token rate cannot be fitted to a provider's limit (there
is no cliff to fit — the very first request in a minute can already be
rejected). But every 429 *does* carry a bracket: at the moment the provider
refused, the window had already consumed `used` real tokens, and the refused
request itself would have added `request` more. The effective limit therefore
lies in `[used, used + request]`.

How it works:

1. Every provider rejection classified as **rate limit** by the provider's
   `CooldownPolicy` produces one sample. Quota / entitlement failures
   (`insufficient_quota`, …) are deliberately ignored — they say nothing
   about a window limit. Each sample also records its limiter dimension
   (rpm / tpm / unknown, see the RPM/TPM section above); RPM samples are kept
   for diagnostics but excluded from the token-limit estimate.
2. `used` comes from the same **real-usage** accounting the capacity gate
   uses (the token usage the provider reported, or `billed_usage` on a
   failed-but-billed attempt) — never an estimate. The learner keeps its own
   read-only window, so it works whether or not the capacity gate is enabled.
3. Each `(provider, model)` pair keeps the last `max_samples` samples.

Estimation formula, for the `n` samples of one `(provider, model)` pair:

```
lower_i    = used_i                  # tokens already consumed when the 429 arrived
upper_i    = used_i + request_i      # tokens the refused request would have added
LowerBound = min_i lower_i
UpperBound = max_i upper_i
Estimated  = median_i lower_i
```

The median is used instead of the mean because the samples are a noisy
process (concurrent requests, a provider window that does not line up with
ours) and the tails should not drag the estimate. The bounds are the
*envelope* of the per-sample brackets, not their intersection: with
`request_i == 0` (the common case — a 429 is usually not billed) the
intersection is empty and cannot carry an estimate.

Below `min_samples` observations no estimate is published at all: the pair is
still listed with its sample count and the confidence band `insufficient`, so
the operator can see the system is observing but has not learned anything
trustworthy yet.

Confidence bands are derived from the sample count relative to
`min_samples`: `< min_samples` → `insufficient`, `< 3 × min_samples` → `low`,
`< 6 × min_samples` → `medium`, otherwise `high`.

Each provider can learn over its own window; the plugin-level section below is
the default for providers that do not override it:

```json
{
  "config": {
    "default_ttl_seconds": 300,
    "ttl_overrides": {},
    "quota_patterns": ["insufficient_quota"],
    "learning": {
      "enabled": true,
      "window_seconds": 60,
      "max_samples": 50,
      "min_samples": 5
    }
  }
}
```

| Field | Default | Notes |
|---|---|---|
| `learning.enabled` | `false` | `false` = feature fully inert (no state, no lock, no samples) |
| `learning.window_seconds` | `60` | window used to measure the consumption that preceded a rejection; clamped to `10..3600` |
| `learning.max_samples` | `50` | samples kept per provider/model; clamped to `5..500` |
| `learning.min_samples` | `5` | samples required before an estimate is published; clamped to `1..100` |

Read the result through the read-only endpoint:

```
GET /api/plugins/provider-cooldown/learned-limits
```

```json
{
  "plugin": "provider-cooldown",
  "count": 1,
  "window_seconds": 60,
  "limits": [
    {
      "provider": "openai",
      "model": "gpt-4o",
      "sample_count": 12,
      "key_count": 2,
      "estimated_token_limit": 82000,
      "window_seconds": 60,
      "lower_bound": 71000,
      "upper_bound": 91000,
      "confidence": "medium",
      "limiter_dimension": "tpm",
      "dimension_sample_counts": { "rpm": 4, "tpm": 10, "unknown": 2 },
      "first_seen": "2026-11-20T10:15:02Z",
      "last_seen": "2026-11-20T11:02:41Z"
    }
  ]
}
```

Notes:

- **The learner only reads.** It never writes a cooldown policy, a capacity
  limit, or anything else — the estimate is a *suggestion* for the operator.
- `sample_count` counts the samples that participate in the estimate
  (non-RPM); `dimension_sample_counts` covers every retained sample, so
  `sample_count + dimension_sample_counts["rpm"]` is the retained total.
- **An RPM rejection is not token evidence.** See the RPM/TPM section: only a
  tpm/unknown sample is used for the estimate, which is why a pair that only
  ever hits RPM reports `limiter_dimension: "rpm"` and `confidence:
  "insufficient"` instead of a ~0 limit.
- Each entry carries its own `window_seconds` because providers may learn over
  different windows; the top-level `window_seconds` in the response is the
  plugin-level default window, kept for backward compatibility.
- The learned number is the aggregate limit our own traffic hit for one
  `(provider, model)` pair. With several keys in use, the provider's per-key
  limits add up, so the learned value is the sum of the limits we exercised;
  `key_count` says how many keys contributed.
- The window must be the same order of magnitude as the provider's own limit
  window; a 60s window matches a TPM limit, not a daily quota.
- State is in-memory and per-process, and is dropped when the process restarts
  (or when the plugin is re-initialized).

### Health-aware key weighting (optional, default off)

Passive cooldown removes a failing key for the length of its TTL, but between
cooldowns weighted-random still gives that key its full configured weight, so
every rotation spends a request on a credential that is known to be failing.
`health` closes that gap: it folds each `(provider, key, model)` triple's
observed success/failure stream into an exponentially weighted moving average
(EWMA) success rate and scales that key's `Weight` by it **before** weighted
selection.

```json
{
  "config": {
    "default_ttl_seconds": 300,
    "ttl_overrides": {},
    "quota_patterns": ["insufficient_quota"],
    "health": {
      "enabled": true,
      "half_life_seconds": 120,
      "min_samples": 5,
      "min_weight_factor": 0.05
    }
  }
}
```

| Field | Default | Notes |
|---|---|---|
| `health.enabled` | `false` | `false` = feature fully inert (no state, no lock, weights unchanged) |
| `health.half_life_seconds` | `120` | EWMA half-life; an observation one half-life old counts half. Clamped to `10..3600` |
| `health.min_samples` | `5` | observations needed before the weight may be scaled; below it the weight is untouched. Clamped to `1..1000` |
| `health.min_weight_factor` | `0.05` | floor of the scaling factor; clamped to `0.01..1.0` and never zero |

Notes:

- **Selection only.** A success in `PostLLMHook` raises the key's EWMA; a
  policy-matched rate-limit / quota rejection observed by the per-key failure
  marker lowers it. Nothing here marks a cooldown, refuses a request or changes
  the upstream bytes.
- **Never zero.** `min_weight_factor` keeps a de-prioritised key in the rotation
  at a small, non-zero share. That share is its recovery probe: without it the
  key could never report success again and the pool would slowly starve. A
  scaled weight is also floored at the selector's minimum selectable unit
  (`0.01`) so the guarantee survives `WeightedRandom`'s integer quantization.
- **Fast recovery.** The EWMA is time-aware: a key that sat idle for several
  half-lives snaps back to healthy on its first success afterwards instead of
  being punished for stale history.
- **No-op on the disabled path.** With the feature off (the default) every key
  keeps its configured weight byte-for-byte and no state is allocated.
- The plugin page does not edit `health` yet. Saving an unrelated plugin or
  provider setting preserves a stored `health` section untouched (both forms
  spread the stored object before overwriting the fields they own).

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

- Provider retry hints (`Retry-After`, `x-ratelimit-reset-*`) are only used
  when the matched rule sets `retry_after_mode` to `cap` or `exact`; the
  default `off` keeps the fixed `ttl_seconds`. The one exception is a
  rate_limit rule with `rpm_ttl_seconds` / `tpm_ttl_seconds` configured: `off`
  then treats a usable hint as a lower bound (`max(base, hint)`). The hint is
  read from `BifrostError.ExtraFields.ProviderResponseHeaders`, so it is
  available through the plugin interface without any transport-specific code.
- `escalation`, `learning` and `health` are plugin-level defaults; `capacity` is
  a **deprecated legacy fleet default** (still honoured for providers without
  their own section, but no longer editable from the plugin page). A provider
  that sets its own section in `cooldown_policy` replaces the whole section, so
  changing a plugin default does not retroactively change a provider that
  already overrides it.
- The cooldown state lives in-process. For multi-instance deployments
  replace `CooldownState` with a `KVStore`-backed implementation that
  satisfies the same `Mark` / `IsCoolingDown` semantics.

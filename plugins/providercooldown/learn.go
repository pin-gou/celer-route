package providercooldown

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Read-only limit learning.
//
// The learner watches provider rate-limit rejections (HTTP 429 classified as
// `rate_limit` by the per-provider CooldownPolicy) and derives an *estimate*
// of the provider's effective per-window token limit. It never changes any
// configuration: the only output is a read-only report the operator can read
// via GET /api/plugins/provider-cooldown/learned-limits.
//
// Why a rejection is informative
//
// A 429 means the provider refused the request because the window's consumption
// had reached its limit. At that moment we know two things:
//
//	used_i      the REAL tokens already consumed in the window (our own
//	            accounting of what the provider actually billed us for)
//	request_i   the tokens the rejected request itself would have added
//	            (0 when the provider did not report them — a 429 is normally
//	            not billed at all)
//
// Under the assumption that the provider's window matches ours and that it
// rejects on the *would-be* total, the effective limit L is bracketed:
//
//	L ∈ [ used_i , used_i + request_i ]
//
// With request_i == 0 the bracket degenerates to the point [used_i, used_i]:
// we know the limit is *around* used_i, but not exactly.
//
// Estimation formula (n samples for one (provider, model) pair)
//
//	lower_i    = used_i
//	upper_i    = used_i + request_i
//	LowerBound = min_i lower_i
//	UpperBound = max_i upper_i
//	Estimated  = median_i lower_i
//
// The median (rather than the mean) is used because the samples are a
// *survival* sample of a noisy process: concurrent requests, a provider window
// that does not line up with ours, and provider-side accounting that differs
// from ours all show up as spread. The median ignores the tails, and it is
// always inside [min lower_i, max upper_i] = [LowerBound, UpperBound].
//
// The bounds are the *envelope* of the per-sample brackets (not their
// intersection): with request_i == 0 for most samples the intersection is
// usually empty, and an empty interval cannot carry an estimate.
//
// Below `min_samples` samples no estimate is produced at all — the entry is
// still reported (with its sample count and the "insufficient" confidence) so
// the operator can see that the system is observing but has not yet learned
// anything trustworthy.
//
// Dimension split (B3)
//
// A rate-limit 429 does not always mean the token budget was hit: the request
// rate can be exhausted while the token window is nearly empty. The plugin
// derives each rejection's limiter dimension (rpm / tpm / unknown, see
// dimension.go). RPM samples are still recorded — they are useful for
// diagnostics and are exposed as DimensionSampleCounts — but they are EXCLUDED
// from the token-limit estimate. Without that split, a key rejected on RPM at
// usedTokens≈0 dragged the median and the lower bound to ~0, which is exactly
// the misleading report the production SenseNova deployment produced.
// SampleCount / the token estimate / the token confidence all describe the
// non-RPM samples only.
//
// Request budget (P2)
//
// The RPM samples are not wasted: they are the only evidence about the
// REQUEST budget, so they feed a second estimate over the same bracket logic,
// in requests instead of tokens:
//
//	lower_i = used_requests_i
//	upper_i = used_requests_i + 1     (the rejected request itself)
//
// EstimatedRequestLimit / RequestLowerBound / RequestUpperBound /
// RequestConfidence describe that second estimate. Every retained sample feeds
// exactly one of the two estimates, so SampleCount + RequestSampleCount equals
// the number of retained samples.
//
// Two scopes (P1)
//
// Every rejection is also recorded against the credential that hit it, so the
// report carries both levels: "provider_model" (the aggregate budget of every
// key serving a model) and "key_model" (one credential's own budget for one
// model). The capacity gate's scope decides which one is actionable — adopting
// an aggregate estimate into a key-scoped gate would multiply the budget by the
// number of keys — so LearnedLimit.Scope travels with every entry and consumers
// must only pair same-level numbers.
//
// A note on what "the limit" means here: a "provider_model" number is the
// aggregate limit our own traffic hit for one (provider, model) pair. When
// several keys of the same provider are in use, the provider's per-key limits
// add up, so that learned value is the sum of the per-key limits we actually
// exercised, not a single key's limit. The "key_model" entries are the
// per-credential view that the summary cannot provide.

const (
	// defaultLearningWindowSeconds is the rolling window the learner uses to
	// measure "tokens already consumed". It matches the order of magnitude of
	// a provider's TPM window (60s) and the capacity gate's default window.
	defaultLearningWindowSeconds = schemas.DefaultLearningWindowSeconds
	minLearningWindowSeconds     = schemas.MinLearningWindowSeconds
	maxLearningWindowSeconds     = schemas.MaxLearningWindowSeconds

	// defaultLearningMaxSamples is how many 429 samples are kept per
	// (provider, model) pair. Older samples are dropped first.
	defaultLearningMaxSamples = schemas.DefaultLearningMaxSamples
	minLearningMaxSamples     = schemas.MinLearningMaxSamples
	maxLearningMaxSamples     = schemas.MaxLearningMaxSamples

	// defaultLearningMinSamples is the number of samples required before an
	// estimate is published. Below it the entry reports "insufficient".
	defaultLearningMinSamples = schemas.DefaultLearningMinSamples
	minLearningMinSamples     = schemas.MinLearningMinSamples
	maxLearningMinSamples     = schemas.MaxLearningMinSamples
)

// Confidence bands published alongside every estimate. The bands are derived
// from the sample count relative to min_samples, so they follow the operator's
// own tolerance for "how many observations do I need".
const (
	// LearningConfidenceInsufficient marks an entry that has samples but not
	// enough of them to publish an estimate.
	LearningConfidenceInsufficient = "insufficient"
	// LearningConfidenceLow means the estimate exists but rests on few samples.
	LearningConfidenceLow = "low"
	// LearningConfidenceMedium means the estimate rests on a decent sample.
	LearningConfidenceMedium = "medium"
	// LearningConfidenceHigh means the estimate rests on many samples.
	LearningConfidenceHigh = "high"
)

// Learning scopes. Every rejection is recorded at BOTH levels, because the
// two answer different questions and must never be mixed:
//
//   - LearningScopeProviderModel ("provider_model") — the aggregate budget of
//     all credentials serving one model, i.e. what the account as a whole may
//     spend. This is what a "provider_model" capacity gate enforces.
//   - LearningScopeKeyModel ("key_model") — one credential's own budget for
//     one model. This is what a "key"-scoped capacity gate enforces.
//
// Adopting an aggregate estimate into a key-scoped gate would multiply the
// budget by the number of keys, which is why the report carries its scope and
// the UI only offers estimates from the level the gate is configured for.
const (
	LearningScopeProviderModel = "provider_model"
	LearningScopeKeyModel      = "key_model"
)

// LearnedLimit is the read-only estimate for one (scope, provider, model[, key])
// tuple.
//
// The estimate fields are omitted (zero) when the tuple has not collected
// enough samples yet — the "insufficient" confidence says so explicitly.
//
// Two budgets can be estimated from the same tuple, because the two kinds of
// rejection carry different evidence:
//
//   - token: TPM / unknown rejections, measured in tokens consumed at the
//     moment the provider refused.
//   - request: RPM rejections, measured in requests already counted in the
//     window. RPM samples are still excluded from the token estimate (they say
//     nothing about a token budget), but they are the ONLY evidence about a
//     request budget.
//
// No key material is ever exposed here: only the provider name, the model
// name, the credential's display name and aggregate counters.
type LearnedLimit struct {
	// Provider is the upstream provider name (e.g. "openai").
	Provider string `json:"provider"`
	// Model is the model the rejected requests targeted.
	Model string `json:"model"`
	// Scope is "provider_model" (aggregate across every credential) or
	// "key_model" (one credential). Older servers omit it; treat a missing
	// value as "provider_model".
	Scope string `json:"scope"`
	// KeyName identifies the credential for a "key_model" entry; it is empty
	// for an aggregate entry. KeyID is internal (dedupe/identity only) and is
	// never serialized: the report exposes the user-facing label, not the
	// credential identifier, matching the endpoint's "no key material" contract.
	KeyID   string `json:"-"`
	KeyName string `json:"key_name,omitempty"`
	// SampleCount is how many rate-limit rejections were observed for the
	// tuple inside the retained sample buffer AND participate in the token-limit
	// estimate. RPM rejections are excluded (they say nothing about a token
	// budget), so SampleCount + DimensionSampleCounts["rpm"] equals the total
	// number of retained samples. Reconstruction: sum the values of
	// DimensionSampleCounts.
	SampleCount int `json:"sample_count"`
	// KeyCount is how many distinct provider keys the retained samples were
	// attributed to. When it is greater than 1 the estimate is the AGGREGATE
	// limit our traffic hit (the provider's per-key limits add up), not a
	// single key's limit. A "key_model" entry is always 1.
	KeyCount int `json:"key_count,omitempty"`
	// EstimatedTokenLimit is the suggested per-window token limit, in tokens
	// per learning window. 0 when no estimate is available.
	EstimatedTokenLimit int64 `json:"estimated_token_limit,omitempty"`
	// WindowSeconds is the window length this estimate is expressed in.
	// Different providers can learn over different windows (each provider's
	// learning section may set its own window_seconds), so the unit of every
	// estimate travels with the estimate itself.
	WindowSeconds int `json:"window_seconds"`
	// LowerBound is the smallest observed rejection point (tokens already
	// consumed in the window when the provider refused).
	LowerBound int64 `json:"lower_bound,omitempty"`
	// UpperBound is the largest "would-be" total observed (tokens consumed
	// plus the tokens the rejected request itself would have added).
	UpperBound int64 `json:"upper_bound,omitempty"`
	// Confidence is one of "insufficient", "low", "medium", "high". It always
	// describes the TOKEN estimate, which keeps the field's historical meaning;
	// RequestConfidence describes the request estimate.
	Confidence string `json:"confidence"`
	// RequestSampleCount is how many retained samples were RPM rejections,
	// i.e. the evidence behind the request-budget estimate. It is disjoint from
	// SampleCount: every sample feeds exactly one of the two estimates.
	RequestSampleCount int `json:"request_sample_count,omitempty"`
	// EstimatedRequestLimit is the suggested per-window REQUEST limit. 0 when
	// no request-budget estimate is available.
	EstimatedRequestLimit int64 `json:"estimated_request_limit,omitempty"`
	// RequestLowerBound / RequestUpperBound bracket the request estimate the
	// same way LowerBound / UpperBound bracket the token estimate, in requests.
	RequestLowerBound int64 `json:"request_lower_bound,omitempty"`
	RequestUpperBound int64 `json:"request_upper_bound,omitempty"`
	// RequestConfidence is "insufficient", "low", "medium" or "high" for the
	// request estimate; "insufficient" whenever EstimatedRequestLimit is 0.
	RequestConfidence string `json:"request_confidence,omitempty"`
	// LimiterDimension is the dominant limiter dimension among the samples
	// that participate in the estimate (tpm or unknown). When no sample
	// participates (every retained sample is an RPM rejection) it falls back to
	// the dominant dimension among ALL retained samples, which lets an operator
	// see "rpm" and understand why the token estimate is unavailable instead of
	// reading a misleading ~0.
	LimiterDimension string `json:"limiter_dimension,omitempty"`
	// DimensionSampleCounts is how many retained samples carried each limiter
	// dimension (e.g. {"rpm":12,"tpm":8,"unknown":2}). Summing the values
	// reconstructs the total retained sample count, including the RPM samples
	// excluded from SampleCount. Aggregated on each report from the bounded
	// sample buffer, so it adds no long-lived state.
	DimensionSampleCounts map[string]int `json:"dimension_sample_counts,omitempty"`
	// FirstSeen / LastSeen bound the observation period.
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// learnSample is one observed rate-limit rejection, measured in the bucket the
// sample belongs to (aggregate or per-credential).
type learnSample struct {
	keyID   string
	keyName string
	// usedTokens / usedRequests are the real usage already accounted in the
	// bucket's rolling window at the moment the provider refused.
	usedTokens   int64
	usedRequests int64
	// requestTokens is what the rejected request itself would have added (0
	// when the provider did not report it — a 429 is normally not billed).
	requestTokens int64
	// dimension records which budget the rejection hit. RPM samples feed the
	// request-budget estimate only; every other sample feeds the token one.
	dimension limiterDimension
	at        time.Time
}

// learnPair holds the samples of one (scope, provider, model[, key]) tuple.
type learnPair struct {
	provider  schemas.ModelProvider
	model     string
	scope     string
	keyID     string
	keyName   string
	samples   []learnSample
	firstSeen time.Time
	lastSeen  time.Time
}

// learnObservation is one rejection to record into one bucket. The caller
// builds two of them from a single rejection (aggregate + per-credential), each
// carrying the usage measured in its own observer ring.
type learnObservation struct {
	provider      schemas.ModelProvider
	model         string
	scope         string
	keyID         string
	keyName       string
	usedTokens    int64
	usedRequests  int64
	requestTokens int64
	dimension     limiterDimension
	at            time.Time
}

// limitLearner accumulates rate-limit rejections and derives the read-only
// estimate. It owns its own mutex — the cooldown map's lock is held on the
// mark/filter path, and the learner is touched on every rejection.
//
// Concurrency: every exported entry point takes l.mu. The observer window has
// its own lock (inherited from capacityGate), so the two never nest.
type limitLearner struct {
	mu sync.Mutex

	// Immutable after construction.
	windowSeconds int64
	maxSamples    int
	minSamples    int

	// now is the clock, shared with the owning CooldownState so tests inject
	// one fake clock for the whole plugin. It is a method value, so a clock
	// installed after the learner was built is still honoured.
	now func() time.Time

	// observer is a READ-ONLY usage ledger: the same per-second bucket ring
	// the capacity gate uses, built with no token/request thresholds so
	// check() can never gate anything. It exists so the learner can measure
	// "real tokens consumed in the last window" without the capacity gate
	// having to be enabled — see the capacityGate doc comment for the
	// observer mode. It is provider+model granular: the aggregate budget.
	observer *capacityGate

	// keyObserver is the same ledger at (provider, key, model) granularity, so
	// a rejection can also be measured against the credential that hit it. One
	// extra ring per key/model; a 60s window costs ~1.4KB each.
	keyObserver *capacityGate

	// pairs maps a scope-qualified tuple key to its retained samples.
	pairs map[string]*learnPair
}

// newLearner builds a learner from an already-normalized config. The clock is
// shared with the state so a fake clock injected in tests drives both the
// sample timestamps and the observer window.
func newLearner(cfg *LearningConfig, now func() time.Time) *limitLearner {
	n := cfg.Normalized()
	l := &limitLearner{
		windowSeconds: int64(n.WindowSeconds),
		maxSamples:    n.MaxSamples,
		minSamples:    n.MinSamples,
		now:           now,
		observer: &capacityGate{
			windowSeconds: int64(n.WindowSeconds),
			scope:         CapacityScopeProviderModel,
			now:           now,
			keys:          make(map[string]*capacityWindow),
		},
		keyObserver: &capacityGate{
			windowSeconds: int64(n.WindowSeconds),
			scope:         CapacityScopeKey,
			now:           now,
			keys:          make(map[string]*capacityWindow),
		},
		pairs: make(map[string]*learnPair),
	}
	return l
}

// window returns the learner's effective window length in seconds.
func (l *limitLearner) window() int64 {
	if l == nil {
		return 0
	}
	return l.windowSeconds
}

// observe records one rate-limit rejection sample into one bucket.
//
// The caller supplies the usage measured in that bucket's own observer ring:
// usedTokens / usedRequests are the real consumption already accounted in the
// window BEFORE the rejection, and requestTokens is what the rejected request
// itself would have added (0 when the provider did not report it). dimension
// says which budget the rejection hit: RPM samples feed the request-budget
// estimate, every other sample feeds the token one.
func (l *limitLearner) observe(o learnObservation) {
	if l == nil || o.provider == "" || o.model == "" {
		return
	}
	if o.usedTokens < 0 {
		o.usedTokens = 0
	}
	if o.usedRequests < 0 {
		o.usedRequests = 0
	}
	if o.requestTokens < 0 {
		o.requestTokens = 0
	}
	if o.dimension == "" {
		o.dimension = dimensionUnknown
	}
	if o.scope == "" {
		o.scope = LearningScopeProviderModel
	}
	if o.scope == LearningScopeKeyModel && o.keyID == "" {
		// A key-scoped bucket without a credential would silently duplicate the
		// aggregate one; skip instead.
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// The aggregate bucket's identity is (provider, model) even though its
	// samples still carry the credential that produced them (KeyCount needs
	// it); only a key-scoped bucket is identified by the credential.
	pairKeyID := ""
	if o.scope == LearningScopeKeyModel {
		pairKeyID = o.keyID
	}
	key := learnPairKey(o.provider, o.scope, pairKeyID, o.model)
	pair := l.pairs[key]
	if pair == nil {
		// KeyName identifies a single credential, so an aggregate bucket never
		// carries one even though its samples do (KeyCount needs them).
		pairKeyName := ""
		if o.scope == LearningScopeKeyModel {
			pairKeyName = o.keyName
		}
		pair = &learnPair{provider: o.provider, model: o.model, scope: o.scope, keyID: pairKeyID, keyName: pairKeyName}
		l.pairs[key] = pair
	} else if pair.keyName == "" && o.keyName != "" && o.scope == LearningScopeKeyModel {
		// A provider key can be renamed; keep the display name fresh.
		pair.keyName = o.keyName
	}
	pair.samples = append(pair.samples, learnSample{
		keyID:         o.keyID,
		keyName:       o.keyName,
		usedTokens:    o.usedTokens,
		usedRequests:  o.usedRequests,
		requestTokens: o.requestTokens,
		dimension:     o.dimension,
		at:            o.at,
	})
	// Bounded buffer: the oldest sample is dropped once the cap is hit.
	if len(pair.samples) > l.maxSamples {
		drop := len(pair.samples) - l.maxSamples
		pair.samples = append(pair.samples[:0], pair.samples[drop:]...)
	}
	if pair.firstSeen.IsZero() || o.at.Before(pair.firstSeen) {
		pair.firstSeen = o.at
	}
	if o.at.After(pair.lastSeen) {
		pair.lastSeen = o.at
	}
}

// learnPairKey builds the internal bucket identity. A NUL separator is used
// because model names can contain "|"; the only consumer is this map.
func learnPairKey(provider schemas.ModelProvider, scope string, keyID string, model string) string {
	if scope == LearningScopeKeyModel {
		return string(provider) + "\x00" + scope + "\x00" + keyID + "\x00" + model
	}
	return string(provider) + "\x00" + scope + "\x00" + "\x00" + model
}

// limits returns the current estimate for every observed (provider, model)
// pair, sorted by provider then model for a stable output.
func (l *limitLearner) limits() []LearnedLimit {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]LearnedLimit, 0, len(l.pairs))
	for _, pair := range l.pairs {
		// Partition once: every sample is counted per dimension for the
		// diagnostic map, then split by the budget its dimension is evidence
		// for. Both partitions are transient (report path only).
		counts := make(map[string]int, 3)
		tokenSamples := make([]learnSample, 0, len(pair.samples))
		requestSamples := make([]learnSample, 0, len(pair.samples))
		for _, s := range pair.samples {
			counts[string(s.dimension)]++
			if isTokenBudgetDimension(s.dimension) {
				tokenSamples = append(tokenSamples, s)
			} else {
				requestSamples = append(requestSamples, s)
			}
		}
		limit := LearnedLimit{
			Provider:              string(pair.provider),
			Model:                 pair.model,
			Scope:                 pair.scope,
			KeyID:                 pair.keyID,
			KeyName:               pair.keyName,
			SampleCount:           len(tokenSamples),
			KeyCount:              distinctKeyCount(pair.samples),
			RequestSampleCount:    len(requestSamples),
			WindowSeconds:         int(l.windowSeconds),
			Confidence:            LearningConfidenceInsufficient,
			RequestConfidence:     LearningConfidenceInsufficient,
			LimiterDimension:      string(dominantDimension(pair.samples, tokenSamples)),
			DimensionSampleCounts: counts,
			FirstSeen:             pair.firstSeen,
			LastSeen:              pair.lastSeen,
		}
		// A zero upper bound means the observer never measured any real usage
		// for this tuple (every sampled rejection saw an idle window — e.g.
		// usage accounting is not wired, or the window is far shorter than the
		// provider's). There is nothing to estimate, so the entry stays
		// "insufficient" instead of publishing a meaningless 0.
		if token := estimateTokenBudget(tokenSamples); token.samples >= l.minSamples && token.upper > 0 {
			limit.EstimatedTokenLimit = token.estimate
			limit.LowerBound = token.lower
			limit.UpperBound = token.upper
			limit.Confidence = learningConfidence(token.samples, l.minSamples)
		}
		if req := estimateRequestBudget(requestSamples); req.samples >= l.minSamples && req.upper > 1 {
			// `upper > 1` means at least one sample saw real request accounting
			// (used_requests >= 1). A window that never counted a single request
			// is the request-side equivalent of "usage accounting is not wired"
			// — the same reason the token estimate requires upper > 0 — so it
			// stays "insufficient" instead of publishing a limit of 1.
			limit.EstimatedRequestLimit = req.estimate
			limit.RequestLowerBound = req.lower
			limit.RequestUpperBound = req.upper
			limit.RequestConfidence = learningConfidence(req.samples, l.minSamples)
		}
		out = append(out, limit)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		if out[i].KeyID != out[j].KeyID {
			return out[i].KeyID < out[j].KeyID
		}
		return out[i].Model < out[j].Model
	})
	return out
}

// learnedBudget is one budget estimate plus the sample count it rests on.
type learnedBudget struct {
	samples      int
	estimate     int64
	lower, upper int64
}

// distinctKeyCount reports how many distinct provider keys the samples were
// attributed to. The estimate aggregates the traffic of all of them, so the
// operator can tell a single-key limit from a multi-key aggregate.
func distinctKeyCount(samples []learnSample) int {
	seen := make(map[string]struct{}, len(samples))
	for _, s := range samples {
		if s.keyID == "" {
			continue
		}
		seen[s.keyID] = struct{}{}
	}
	return len(seen)
}

// dominantDimension returns the most common limiter dimension among the
// samples that participate in the estimate. When none participate (every
// retained sample is an RPM rejection) it falls back to all retained samples,
// so the report says "rpm" instead of omitting the field — that is the
// operator's signal that this pair only ever hit the request-rate budget and
// has no usable token estimate. Ties resolve rpm > tpm > unknown for a stable
// output.
func dominantDimension(all []learnSample, participants []learnSample) limiterDimension {
	if len(all) == 0 {
		return dimensionUnknown
	}
	src := participants
	if len(src) == 0 {
		src = all
	}
	var rpm, tpm, unknown int
	for _, s := range src {
		switch s.dimension {
		case dimensionRPM:
			rpm++
		case dimensionTPM:
			tpm++
		default:
			unknown++
		}
	}
	best, bestN := dimensionRPM, rpm
	if tpm > bestN {
		best, bestN = dimensionTPM, tpm
	}
	if unknown > bestN {
		best = dimensionUnknown
	}
	return best
}

// estimateTokenBudget derives the token-budget point estimate and its bracket
// from the samples that carry token evidence (TPM / unknown). See the file
// header for the formula.
//
//	lower_i   = used_i
//	upper_i   = used_i + request_i
//	Lower     = min_i lower_i
//	Upper     = max_i upper_i
//	Estimate  = median_i used_i
//
// The returned estimate is guaranteed to satisfy Lower <= estimate <= Upper.
func estimateTokenBudget(samples []learnSample) learnedBudget {
	if len(samples) == 0 {
		return learnedBudget{}
	}
	used := make([]int64, 0, len(samples))
	lower := int64(math.MaxInt64)
	upper := int64(0)
	for _, s := range samples {
		used = append(used, s.usedTokens)
		if s.usedTokens < lower {
			lower = s.usedTokens
		}
		if total := s.usedTokens + s.requestTokens; total > upper {
			upper = total
		}
	}
	return newLearnedBudget(used, lower, upper)
}

// estimateRequestBudget derives the REQUEST-budget point estimate from the
// samples that carry request evidence (RPM rejections). The same bracket logic
// applies, in requests instead of tokens:
//
//	lower_i   = used_i          (requests already counted when the provider refused)
//	upper_i   = used_i + 1      (the rejected request itself would have been one more)
//
// With no usable provider-side request count this is the only honest bracket:
// the provider refuses once the window reaches its limit, so the limit is at
// `used` or one above it.
func estimateRequestBudget(samples []learnSample) learnedBudget {
	if len(samples) == 0 {
		return learnedBudget{}
	}
	used := make([]int64, 0, len(samples))
	lower := int64(math.MaxInt64)
	upper := int64(0)
	for _, s := range samples {
		used = append(used, s.usedRequests)
		if s.usedRequests < lower {
			lower = s.usedRequests
		}
		if total := s.usedRequests + 1; total > upper {
			upper = total
		}
	}
	return newLearnedBudget(used, lower, upper)
}

// newLearnedBudget computes the median and clamps it into the bracket. The
// median (rather than the mean) is used because the samples are a survival
// sample of a noisy process: concurrent requests, a provider window that does
// not line up with ours, and provider-side accounting that differs from ours
// all show up as spread, and the median ignores the tails.
func newLearnedBudget(used []int64, lower, upper int64) learnedBudget {
	sort.Slice(used, func(i, j int) bool { return used[i] < used[j] })
	mid := len(used) / 2
	var estimate int64
	if len(used)%2 == 1 {
		estimate = used[mid]
	} else {
		// Even count: average the two central values. Integer division is
		// fine — the estimate is a suggestion, not an accounting figure.
		estimate = (used[mid-1] + used[mid]) / 2
	}
	if estimate < lower {
		estimate = lower
	}
	if estimate > upper {
		estimate = upper
	}
	return learnedBudget{samples: len(used), estimate: estimate, lower: lower, upper: upper}
}

// learningConfidence maps a sample count to a confidence band. The bands are
// relative to min_samples so they follow the operator's own tolerance:
//
//	n <  min_samples      → insufficient (no estimate at all)
//	n <  3 * min_samples  → low
//	n <  6 * min_samples  → medium
//	otherwise             → high
func learningConfidence(sampleCount, minSamples int) string {
	if minSamples <= 0 {
		minSamples = 1
	}
	switch {
	case sampleCount < minSamples:
		return LearningConfidenceInsufficient
	case sampleCount < 3*minSamples:
		return LearningConfidenceLow
	case sampleCount < 6*minSamples:
		return LearningConfidenceMedium
	default:
		return LearningConfidenceHigh
	}
}

// learnedSampledKey marks a request context whose rejection has already been
// sampled by the per-key failure marker.
//
// The marker fires per failed attempt (inside the retry loop) while
// PostLLMHook fires once with the terminal outcome — for a terminal 429 both
// observe the same rejection. The flag keeps that from producing two
// identical samples. It lives on the request context, so it is naturally
// scoped to one request.
type learnedSampledKey struct{}

func markLearnedSampled(ctx *schemas.BifrostContext) {
	if ctx == nil {
		return
	}
	ctx.SetValue(learnedSampledKey{}, true)
}

func learnedAlreadySampled(ctx *schemas.BifrostContext) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(learnedSampledKey{}).(bool)
	return v
}

// --- CooldownState / plugin wiring ---

// learnerHolder owns one provider's learner. The atomic pointer keeps the
// steady-state read lock-free; the mutex serializes a rebuild when the
// provider's effective learning config changes at runtime.
type learnerHolder struct {
	mu      sync.Mutex
	learner atomic.Pointer[limitLearner]
}

// enableLearning registers the plugin-level learning section as the DEFAULT
// for every provider that does not configure its own. Like enableCapacity and
// enableEscalation this is normally called while the state is being built
// (Config.AsState); the atomic store also makes a mid-flight reconfiguration
// race-free.
func (c *CooldownState) enableLearning(cfg *LearningConfig) {
	if c == nil || cfg == nil || !cfg.Active() {
		return
	}
	spec, ok := learningSpecOf(cfg)
	if !ok {
		return
	}
	n := cfg.Normalized()
	c.defaultLearningSpec = spec
	c.defaultLearningActive = true
	c.defaults.learning = n
	c.learning.Store(newLearner(n, c.nowTime))
}

// learner returns the plugin-DEFAULT learner installed by enableLearning, or
// nil when learning is off by default. Per-provider learners (which also cover
// providers that enabled learning through their own CooldownPolicy) are
// obtained through learnerFor.
func (c *CooldownState) learner() *limitLearner {
	if c == nil {
		return nil
	}
	return c.learning.Load()
}

// learnerFor returns the effective learner for one provider under the given
// per-provider policy, or nil when learning is off for it (nil config, or
// enabled=false). Disabled providers allocate nothing and collect no samples.
//
// A provider with no learning section shares the plugin-default learner: the
// pair key already includes the provider, and the window is identical, so a
// separate observer would only duplicate memory. A provider whose section
// differs gets its own learner (and therefore its own observer window), since
// window lengths genuinely differ and cannot share one ring.
func (c *CooldownState) learnerFor(provider schemas.ModelProvider, policy *schemas.CooldownPolicy) *limitLearner {
	if c == nil {
		return nil
	}
	spec, ok := learningSpecOf(c.defaults.resolveLearning(policy))
	if !ok {
		return nil
	}
	if c.defaultLearningActive && spec == c.defaultLearningSpec {
		if l := c.learning.Load(); l != nil {
			return l
		}
	}

	holderAny, _ := c.learningRegistry.Load(provider)
	holder, _ := holderAny.(*learnerHolder)
	if holder == nil {
		fresh := &learnerHolder{}
		holderAny, _ = c.learningRegistry.LoadOrStore(provider, fresh)
		holder, _ = holderAny.(*learnerHolder)
	}
	if l := holder.learner.Load(); l != nil && l.matchesLearningSpec(spec) {
		return l
	}

	holder.mu.Lock()
	defer holder.mu.Unlock()
	if l := holder.learner.Load(); l != nil && l.matchesLearningSpec(spec) {
		return l
	}
	l := newLearner(&LearningConfig{
		Enabled:       true,
		WindowSeconds: int(spec.windowSeconds),
		MaxSamples:    spec.maxSamples,
		MinSamples:    spec.minSamples,
	}, c.nowTime)
	holder.learner.Store(l)
	return l
}

// matchesLearningSpec reports whether the learner was built from the given
// normalized spec. Field-by-field scalar comparison: no reflect, no
// allocation.
func (l *limitLearner) matchesLearningSpec(spec learningSpec) bool {
	if l == nil {
		return false
	}
	return l.windowSeconds == spec.windowSeconds &&
		l.maxSamples == spec.maxSamples &&
		l.minSamples == spec.minSamples
}

// observeRateLimit records one learning sample for a rate-limit rejection
// observed by the per-key failure marker.
//
// The marker is the primary sampling point: it fires per failed attempt, at
// the moment the provider refused, so it also catches the rejections a later
// retry succeeded around (which PostLLMHook never sees).
func (c *CooldownState) observeRateLimit(ctx *schemas.BifrostContext, policy *schemas.CooldownPolicy, provider schemas.ModelProvider, model string, keyID string, keyName string, bifrostErr *schemas.BifrostError) {
	if bifrostErr == nil || model == "" {
		return
	}
	l := c.learnerFor(provider, policy)
	if l == nil {
		return
	}
	l.recordRejection(provider, model, keyID, keyName,
		usageTotal(bifrostErr.ExtraFields.BilledUsage),
		resolveDimension(rateLimitRuleOf(policy), bifrostErr), l.now())
	markLearnedSampled(ctx)
}

// observeLearnedRateLimit records one learning sample for a terminal
// rate-limit rejection observed by PostLLMHook.
//
// This is the fallback path: it covers the rejections the per-key marker
// cannot see — keyless providers (no key pool, so core never invokes the
// marker) and deployments that use the Go SDK without wiring the marker. When
// the marker already sampled the same rejection, this is a no-op.
//
// The sample is taken BEFORE the terminal attempt's own usage is accounted,
// so usedTokens reflects the consumption that preceded the rejection.
func (c *CooldownState) observeLearnedRateLimit(ctx *schemas.BifrostContext, policy *schemas.CooldownPolicy, bifrostErr *schemas.BifrostError) {
	if c == nil || bifrostErr == nil {
		return
	}
	if learnedAlreadySampled(ctx) {
		return
	}
	provider, keyID, keyName, model := lastAttemptProviderAndKey(ctx, bifrostErr)
	// Same rule as classify(): an unattributable key on a keyed provider
	// means we cannot trust the attribution, so we do not sample.
	if keyID == "" && !schemas.IsKeylessProvider(provider) {
		return
	}
	// Streaming runs PostLLMHook once per chunk; only the terminal chunk
	// carries the terminal error worth sampling. (The per-key marker is the
	// primary sampling point and already saw the rejection.)
	if isStreamingRequest(bifrostErr.ExtraFields.RequestType) && !isFinalChunk(ctx) {
		return
	}
	_, _, _, kind, _, ok := classifyForMarker(ctx, provider, keyID, keyName, bifrostErr)
	if !ok || kind != CooldownKindRateLimit {
		return
	}
	l := c.learnerFor(provider, policy)
	if l == nil || model == "" {
		return
	}
	l.recordRejection(provider, model, keyID, keyName,
		usageTotal(bifrostErr.ExtraFields.BilledUsage),
		resolveDimension(rateLimitRuleOf(policy), bifrostErr), l.now())
	markLearnedSampled(ctx)
}

// recordRejection records one rate-limit rejection at both learning levels.
//
// Both samples describe the SAME rejection, measured against two different
// ledgers: the aggregate (provider, model) ring answers "what is the account's
// budget for this model", and the per-credential ring answers "what is this
// key's budget". The capacity gate's scope decides which one is actionable, so
// both are always recorded — the report carries the scope and consumers must
// not mix them.
//
// Both usage reads happen BEFORE the rejected attempt's own usage is accounted
// (recordCapacity runs after the post-hook), so `used` is the consumption that
// preceded the rejection.
func (l *limitLearner) recordRejection(provider schemas.ModelProvider, model string, keyID string, keyName string, requestTokens int64, dimension limiterDimension, at time.Time) {
	if l == nil {
		return
	}
	aggTokens, aggRequests := l.observer.usage(provider, "", model)
	l.observe(learnObservation{
		provider:      provider,
		model:         model,
		scope:         LearningScopeProviderModel,
		keyID:         keyID,
		keyName:       keyName,
		usedTokens:    aggTokens,
		usedRequests:  aggRequests,
		requestTokens: requestTokens,
		dimension:     dimension,
		at:            at,
	})
	if keyID == "" {
		// Keyless provider: there is no credential to attribute the rejection
		// to, so only the aggregate bucket exists.
		return
	}
	keyTokens, keyRequests := l.keyObserver.usage(provider, keyID, model)
	l.observe(learnObservation{
		provider:      provider,
		model:         model,
		scope:         LearningScopeKeyModel,
		keyID:         keyID,
		keyName:       keyName,
		usedTokens:    keyTokens,
		usedRequests:  keyRequests,
		requestTokens: requestTokens,
		dimension:     dimension,
		at:            at,
	})
}

// allLearnedLimits aggregates the estimates of the plugin-default learner and
// every per-provider learner into one slice sorted by provider then model. A
// (provider, model) pair that exists in more than one learner (e.g. the
// provider's section was added while the plugin was running) is reported once,
// with the per-provider learner's window.
func (c *CooldownState) allLearnedLimits() []LearnedLimit {
	if c == nil {
		return nil
	}
	out := c.learning.Load().limits()
	c.learningRegistry.Range(func(_, v any) bool {
		if holder, ok := v.(*learnerHolder); ok {
			out = append(out, holder.learner.Load().limits()...)
		}
		return true
	})
	return dedupeLearnedLimits(out)
}

// LearnedLimits returns the read-only limit estimates for every observed
// (provider, model) pair across every provider, with the plugin defaults and
// per-provider overrides both represented. Returns nil when the plugin has no
// state or no learner is enabled anywhere.
//
// The returned slice is a snapshot: mutating it does not affect the learners.
func (p *CooldownPlugin) LearnedLimits() []LearnedLimit {
	if p == nil || p.State == nil {
		return nil
	}
	return p.State.allLearnedLimits()
}

// LearningWindowSeconds returns the plugin-default learner's rolling window
// (in seconds), or 0 when learning is disabled at the plugin level. The unit
// of every reported limit is "tokens per window"; providers that configure
// their own learning section can use a different window, which is reported per
// entry as LearnedLimit.WindowSeconds.
func (p *CooldownPlugin) LearningWindowSeconds() int {
	if p == nil || p.State == nil {
		return 0
	}
	return int(p.State.learner().window())
}

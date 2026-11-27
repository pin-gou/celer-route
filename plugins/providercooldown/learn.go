package providercooldown

import (
	"math"
	"sort"
	"sync"
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
// A note on what "the limit" means here: the learned number is the aggregate
// limit our own traffic hit for one (provider, model) pair. When several keys
// of the same provider are in use, the provider's per-key limits add up, so
// the learned value is the sum of the per-key limits we actually exercised,
// not a single key's limit.

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

// LearnedLimit is the read-only estimate for one (provider, model) pair.
//
// The estimate fields are omitted (zero) when the pair has not collected
// enough samples yet — the "insufficient" confidence says so explicitly.
//
// No key material is ever exposed here: only the provider name, the model
// name and aggregate counters.
type LearnedLimit struct {
	// Provider is the upstream provider name (e.g. "openai").
	Provider string `json:"provider"`
	// Model is the model the rejected requests targeted.
	Model string `json:"model"`
	// SampleCount is how many rate-limit rejections were observed for the
	// pair inside the retained sample buffer.
	SampleCount int `json:"sample_count"`
	// KeyCount is how many distinct provider keys the retained samples were
	// attributed to. When it is greater than 1 the estimate is the AGGREGATE
	// limit our traffic hit (the provider's per-key limits add up), not a
	// single key's limit.
	KeyCount int `json:"key_count,omitempty"`
	// EstimatedTokenLimit is the suggested per-window token limit, in tokens
	// per learning window. 0 when no estimate is available.
	EstimatedTokenLimit int64 `json:"estimated_token_limit,omitempty"`
	// LowerBound is the smallest observed rejection point (tokens already
	// consumed in the window when the provider refused).
	LowerBound int64 `json:"lower_bound,omitempty"`
	// UpperBound is the largest "would-be" total observed (tokens consumed
	// plus the tokens the rejected request itself would have added).
	UpperBound int64 `json:"upper_bound,omitempty"`
	// Confidence is one of "insufficient", "low", "medium", "high".
	Confidence string `json:"confidence"`
	// FirstSeen / LastSeen bound the observation period.
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// learnSample is one observed rate-limit rejection.
type learnSample struct {
	keyID         string
	usedTokens    int64
	requestTokens int64
	at            time.Time
}

// learnPair holds the samples of one (provider, model) pair.
type learnPair struct {
	provider  schemas.ModelProvider
	model     string
	samples   []learnSample
	firstSeen time.Time
	lastSeen  time.Time
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
	// observer mode.
	observer *capacityGate

	// pairs maps "<provider>|<model>" to the retained samples.
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

// observe records one rate-limit rejection sample.
//
// usedTokens is the real token consumption measured over the window BEFORE the
// rejection; requestTokens is the token count of the rejected request itself
// (0 when the provider did not report it).
func (l *limitLearner) observe(provider schemas.ModelProvider, model string, keyID string, usedTokens int64, requestTokens int64, at time.Time) {
	if l == nil || provider == "" || model == "" {
		return
	}
	if usedTokens < 0 {
		usedTokens = 0
	}
	if requestTokens < 0 {
		requestTokens = 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	key := learnPairKey(provider, model)
	pair := l.pairs[key]
	if pair == nil {
		pair = &learnPair{provider: provider, model: model}
		l.pairs[key] = pair
	}
	pair.samples = append(pair.samples, learnSample{
		keyID:         keyID,
		usedTokens:    usedTokens,
		requestTokens: requestTokens,
		at:            at,
	})
	// Bounded buffer: the oldest sample is dropped once the cap is hit.
	if len(pair.samples) > l.maxSamples {
		drop := len(pair.samples) - l.maxSamples
		pair.samples = append(pair.samples[:0], pair.samples[drop:]...)
	}
	if pair.firstSeen.IsZero() || at.Before(pair.firstSeen) {
		pair.firstSeen = at
	}
	if at.After(pair.lastSeen) {
		pair.lastSeen = at
	}
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
		limit := LearnedLimit{
			Provider:    string(pair.provider),
			Model:       pair.model,
			SampleCount: len(pair.samples),
			KeyCount:    distinctKeyCount(pair.samples),
			Confidence:  LearningConfidenceInsufficient,
			FirstSeen:   pair.firstSeen,
			LastSeen:    pair.lastSeen,
		}
		if len(pair.samples) >= l.minSamples {
			estimate, lower, upper := estimateLearnedLimit(pair.samples)
			// A zero upper bound means the observer never measured any real
			// usage for this pair (every sampled rejection saw an idle
			// window — e.g. usage accounting is not wired, or the window is
			// far shorter than the provider's). There is nothing to estimate,
			// so the entry stays "insufficient" instead of publishing a
			// meaningless 0.
			if upper > 0 {
				limit.EstimatedTokenLimit = estimate
				limit.LowerBound = lower
				limit.UpperBound = upper
				limit.Confidence = learningConfidence(len(pair.samples), l.minSamples)
			}
		}
		out = append(out, limit)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Model < out[j].Model
	})
	return out
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

// estimateLearnedLimit derives the point estimate and the confidence interval
// from one pair's samples. See the file header for the formula.
//
//	LowerBound = min_i used_i
//	UpperBound = max_i (used_i + request_i)
//	Estimated  = median_i used_i
//
// The returned estimate is guaranteed to satisfy
// LowerBound <= estimate <= UpperBound.
func estimateLearnedLimit(samples []learnSample) (estimate int64, lower int64, upper int64) {
	if len(samples) == 0 {
		return 0, 0, 0
	}
	used := make([]int64, 0, len(samples))
	lower = math.MaxInt64
	upper = 0
	for _, s := range samples {
		used = append(used, s.usedTokens)
		if s.usedTokens < lower {
			lower = s.usedTokens
		}
		if total := s.usedTokens + s.requestTokens; total > upper {
			upper = total
		}
	}
	sort.Slice(used, func(i, j int) bool { return used[i] < used[j] })
	mid := len(used) / 2
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
	return estimate, lower, upper
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

// learnPairKey is the map key of one (provider, model) pair.
func learnPairKey(provider schemas.ModelProvider, model string) string {
	return string(provider) + "|" + model
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

// enableLearning installs the learner on the state. Like enableCapacity and
// enableEscalation this is normally called while the state is being built
// (Config.AsState), but the atomic store makes a mid-flight reconfiguration
// race-free too.
func (c *CooldownState) enableLearning(cfg *LearningConfig) {
	if c == nil || cfg == nil || !cfg.Active() {
		return
	}
	c.learning.Store(newLearner(cfg, c.nowTime))
}

// learner returns the learner installed on the state, or nil when the feature
// is off.
func (c *CooldownState) learner() *limitLearner {
	if c == nil {
		return nil
	}
	return c.learning.Load()
}

// observeRateLimit records one learning sample for a rate-limit rejection
// observed by the per-key failure marker.
//
// The marker is the primary sampling point: it fires per failed attempt, at
// the moment the provider refused, so it also catches the rejections a later
// retry succeeded around (which PostLLMHook never sees).
func (c *CooldownState) observeRateLimit(ctx *schemas.BifrostContext, provider schemas.ModelProvider, model string, keyID string, bifrostErr *schemas.BifrostError) {
	l := c.learner()
	if l == nil {
		return
	}
	if model == "" {
		return
	}
	usedTokens, _ := l.observer.usage(provider, model)
	requestTokens := usageTotal(bifrostErr.ExtraFields.BilledUsage)
	l.observe(provider, model, keyID, usedTokens, requestTokens, l.now())
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
func (c *CooldownState) observeLearnedRateLimit(ctx *schemas.BifrostContext, bifrostErr *schemas.BifrostError) {
	if c == nil || bifrostErr == nil {
		return
	}
	l := c.learner()
	if l == nil {
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
	if model == "" {
		return
	}
	usedTokens, _ := l.observer.usage(provider, model)
	requestTokens := usageTotal(bifrostErr.ExtraFields.BilledUsage)
	l.observe(provider, model, keyID, usedTokens, requestTokens, l.now())
	markLearnedSampled(ctx)
}

// LearnedLimits returns the read-only limit estimates for every observed
// (provider, model) pair. Returns nil when the plugin has no state or the
// learner is disabled.
//
// The returned slice is a snapshot: mutating it does not affect the learner.
func (p *CooldownPlugin) LearnedLimits() []LearnedLimit {
	if p == nil || p.State == nil {
		return nil
	}
	return p.State.learner().limits()
}

// LearningWindowSeconds returns the effective rolling window (in seconds)
// the learner measures consumption over, or 0 when the learner is disabled.
// The unit of every reported limit is "tokens per window", so the window
// length is part of the meaning of the number.
func (p *CooldownPlugin) LearningWindowSeconds() int {
	if p == nil || p.State == nil {
		return 0
	}
	return int(p.State.learner().window())
}

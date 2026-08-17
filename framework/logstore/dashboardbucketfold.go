package logstore

import (
	"sort"
	"time"
)

// foldIntoBucket maps a fine-grained aggregator bucket timestamp to the
// start of the coarser bucket the caller asked for. bucketSeconds is the
// aggregator's bucket size; requestedSize is the larger size to fold into.
func foldIntoBucket(t time.Time, bucketSeconds, requestedSize int64) time.Time {
	if requestedSize == bucketSeconds || requestedSize <= 0 {
		return t
	}
	unix := t.UTC().Unix()
	aligned := (unix / requestedSize) * requestedSize
	return time.Unix(aligned, 0).UTC()
}

// sortedBucketStarts returns the union of requested buckets over [start, end)
// in chronological order, filling gaps with zero-value rows so the chart
// layer renders a contiguous x-axis.
func sortedBucketStarts(start, end *time.Time, requestedSize int64) []time.Time {
	if start == nil || end == nil || requestedSize <= 0 {
		return nil
	}
	var out []time.Time
	s := alignBucket(*start, requestedSize)
	e := end.UTC()
	for t := s; t.Before(e); t = t.Add(time.Duration(requestedSize) * time.Second) {
		out = append(out, t)
	}
	return out
}

// foldHistogramBuckets merges the per-(bucket_start, status) rows returned by
// the bucket SELECT into per-requested-bucket HistogramBucket entries with
// success/error/cancelled counts.
func foldHistogramBuckets(
	rows []histogramBucketRow,
	bucketSeconds, requestedSize int64,
	start, end *time.Time,
) []HistogramBucket {
	// Aggregate into coarse buckets in a map; using map[int64] for fast lookups.
	type acc struct {
		Success, Error, Cancelled int64
	}
	bucketMap := make(map[int64]*acc)

	for _, r := range rows {
		coarse := foldIntoBucket(r.BucketStart, bucketSeconds, requestedSize).Unix()
		a, ok := bucketMap[coarse]
		if !ok {
			a = &acc{}
			bucketMap[coarse] = a
		}
		switch r.Status {
		case "success":
			a.Success += r.RequestCount
		case "error":
			a.Error += r.RequestCount
		case "cancelled":
			a.Cancelled += r.RequestCount
		default:
			// Other terminal statuses (none today) accumulate as cancelled-ish.
			a.Cancelled += r.RequestCount
		}
	}

	buckets := make([]HistogramBucket, 0, len(bucketMap))
	for ts, a := range bucketMap {
		buckets = append(buckets, HistogramBucket{
			Timestamp: time.Unix(ts, 0).UTC(),
			Count:     a.Success + a.Error + a.Cancelled,
			Success:   a.Success,
			Error:     a.Error,
			Cancelled: a.Cancelled,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})
	return buckets
}

// foldTokenBuckets merges per-bucket prompt/completion/total tokens.
func foldTokenBuckets(
	rows []tokenHistogramRow,
	bucketSeconds, requestedSize int64,
	start, end *time.Time,
) []TokenHistogramBucket {
	type acc struct {
		Prompt, Completion, Total int64
	}
	bucketMap := make(map[int64]*acc)
	for _, r := range rows {
		coarse := foldIntoBucket(r.BucketStart, bucketSeconds, requestedSize).Unix()
		a, ok := bucketMap[coarse]
		if !ok {
			a = &acc{}
			bucketMap[coarse] = a
		}
		a.Prompt += r.PromptTokens
		a.Completion += r.CompletionTokens
		a.Total += r.TotalTokens
	}

	buckets := make([]TokenHistogramBucket, 0, len(bucketMap))
	for ts, a := range bucketMap {
		buckets = append(buckets, TokenHistogramBucket{
			Timestamp:        time.Unix(ts, 0).UTC(),
			PromptTokens:     a.Prompt,
			CompletionTokens: a.Completion,
			TotalTokens:      a.Total,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})
	return buckets
}

// foldCostBuckets merges per-(bucket_start, model) cost rows into
// per-bucket CostHistogramBucket with a model breakdown.
func foldCostBuckets(
	rows []costHistogramRow,
	bucketSeconds, requestedSize int64,
	start, end *time.Time,
) ([]CostHistogramBucket, []string) {
	type bucketAcc struct {
		Total    float64
		ByModel  map[string]float64
	}
	bucketMap := make(map[int64]*bucketAcc)
	modelSet := make(map[string]struct{})

	for _, r := range rows {
		coarse := foldIntoBucket(r.BucketStart, bucketSeconds, requestedSize).Unix()
		b, ok := bucketMap[coarse]
		if !ok {
			b = &bucketAcc{ByModel: make(map[string]float64)}
			bucketMap[coarse] = b
		}
		b.Total += r.Cost
		b.ByModel[r.Model] += r.Cost
		modelSet[r.Model] = struct{}{}
	}

	buckets := make([]CostHistogramBucket, 0, len(bucketMap))
	for ts, b := range bucketMap {
		buckets = append(buckets, CostHistogramBucket{
			Timestamp: time.Unix(ts, 0).UTC(),
			TotalCost: b.Total,
			ByModel:   b.ByModel,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})

	models := make([]string, 0, len(modelSet))
	for m := range modelSet {
		models = append(models, m)
	}
	sort.Strings(models)
	return buckets, models
}

// foldLatencyBuckets merges per-bucket latency sums. Avg latency is the sum
// over total latency / latency count.
func foldLatencyBuckets(
	rows []latencyHistogramRow,
	bucketSeconds, requestedSize int64,
	start, end *time.Time,
) []LatencyHistogramBucket {
	type acc struct {
		TotalLatency float64
		LatencyCount int64
		Requests     int64
	}
	bucketMap := make(map[int64]*acc)
	for _, r := range rows {
		coarse := foldIntoBucket(r.BucketStart, bucketSeconds, requestedSize).Unix()
		a, ok := bucketMap[coarse]
		if !ok {
			a = &acc{}
			bucketMap[coarse] = a
		}
		a.TotalLatency += r.TotalLatencyMS
		a.LatencyCount += r.LatencyCount
		a.Requests += r.RequestCount
	}
	buckets := make([]LatencyHistogramBucket, 0, len(bucketMap))
	for ts, a := range bucketMap {
		var avg float64
		if a.LatencyCount > 0 {
			avg = a.TotalLatency / float64(a.LatencyCount)
		}
		buckets = append(buckets, LatencyHistogramBucket{
			Timestamp:     time.Unix(ts, 0).UTC(),
			AvgLatency:    avg,
			TotalRequests: a.Requests,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})
	return buckets
}

// foldThroughputBuckets merges per-bucket completion tokens and total latency
// for the tokens/sec rate.
func foldThroughputBuckets(
	rows []throughputHistogramRow,
	bucketSeconds, requestedSize int64,
	start, end *time.Time,
) []ThroughputHistogramBucket {
	type acc struct {
		TotalLatency    float64
		LatencyCount    int64
		Requests        int64
		CompletionToks  int64
	}
	bucketMap := make(map[int64]*acc)
	for _, r := range rows {
		coarse := foldIntoBucket(r.BucketStart, bucketSeconds, requestedSize).Unix()
		a, ok := bucketMap[coarse]
		if !ok {
			a = &acc{}
			bucketMap[coarse] = a
		}
		a.TotalLatency += r.TotalLatencyMS
		a.LatencyCount += r.LatencyCount
		a.Requests += r.RequestCount
		a.CompletionToks += r.CompletionTokens
	}
	buckets := make([]ThroughputHistogramBucket, 0, len(bucketMap))
	for ts, a := range bucketMap {
		var tps float64
		if a.TotalLatency > 0 {
			tps = float64(a.CompletionToks) / (a.TotalLatency / 1000.0)
		}
		buckets = append(buckets, ThroughputHistogramBucket{
			Timestamp:             time.Unix(ts, 0).UTC(),
			TokensPerSecond:       tps,
			TotalCompletionTokens: a.CompletionToks,
			TotalRequests:         a.Requests,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})
	return buckets
}

// foldModelBuckets merges per-(bucket, model, status) rows into ModelHistogramBucket.
func foldModelBuckets(
	rows []modelHistogramRow,
	bucketSeconds, requestedSize int64,
	start, end *time.Time,
) ([]ModelHistogramBucket, []string) {
	type bucketAcc struct {
		ByModel map[string]*ModelUsageStats
	}
	bucketMap := make(map[int64]*bucketAcc)
	modelSet := make(map[string]struct{})

	for _, r := range rows {
		coarse := foldIntoBucket(r.BucketStart, bucketSeconds, requestedSize).Unix()
		b, ok := bucketMap[coarse]
		if !ok {
			b = &bucketAcc{ByModel: make(map[string]*ModelUsageStats)}
			bucketMap[coarse] = b
		}
		stats, ok := b.ByModel[r.Model]
		if !ok {
			stats = &ModelUsageStats{}
			b.ByModel[r.Model] = stats
		}
		stats.Total += r.RequestCount
		switch r.Status {
		case "success":
			stats.Success += r.RequestCount
		case "error":
			stats.Error += r.RequestCount
		case "cancelled":
			stats.Cancelled += r.RequestCount
		}
		modelSet[r.Model] = struct{}{}
	}

	buckets := make([]ModelHistogramBucket, 0, len(bucketMap))
	for ts, b := range bucketMap {
		buckets = append(buckets, ModelHistogramBucket{
			Timestamp: time.Unix(ts, 0).UTC(),
			ByModel:   mergeModelStats(b.ByModel),
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})
	models := make([]string, 0, len(modelSet))
	for m := range modelSet {
		models = append(models, m)
	}
	sort.Strings(models)
	return buckets, models
}

// mergeModelStats turns a map of pointers into a map of values for the result
// type. Empty stats are filled with zero values for every model so the chart
// layer never sees a nil key.
func mergeModelStats(in map[string]*ModelUsageStats) map[string]ModelUsageStats {
	out := make(map[string]ModelUsageStats, len(in))
	for k, v := range in {
		if v == nil {
			out[k] = ModelUsageStats{}
			continue
		}
		out[k] = *v
	}
	return out
}

// foldProviderCostBuckets merges per-(bucket, provider) cost rows.
func foldProviderCostBuckets(
	rows []providerCostRow,
	bucketSeconds, requestedSize int64,
	start, end *time.Time,
) ([]ProviderCostHistogramBucket, []string) {
	type bucketAcc struct {
		Total      float64
		ByProvider map[string]float64
	}
	bucketMap := make(map[int64]*bucketAcc)
	providerSet := make(map[string]struct{})

	for _, r := range rows {
		coarse := foldIntoBucket(r.BucketStart, bucketSeconds, requestedSize).Unix()
		b, ok := bucketMap[coarse]
		if !ok {
			b = &bucketAcc{ByProvider: make(map[string]float64)}
			bucketMap[coarse] = b
		}
		b.Total += r.Cost
		b.ByProvider[r.Provider] += r.Cost
		providerSet[r.Provider] = struct{}{}
	}

	buckets := make([]ProviderCostHistogramBucket, 0, len(bucketMap))
	for ts, b := range bucketMap {
		buckets = append(buckets, ProviderCostHistogramBucket{
			Timestamp:  time.Unix(ts, 0).UTC(),
			TotalCost:  b.Total,
			ByProvider: b.ByProvider,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})
	providers := make([]string, 0, len(providerSet))
	for p := range providerSet {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	return buckets, providers
}

// foldProviderTokenBuckets merges per-(bucket, provider) token rows.
func foldProviderTokenBuckets(
	rows []providerTokenRow,
	bucketSeconds, requestedSize int64,
	start, end *time.Time,
) ([]ProviderTokenHistogramBucket, []string) {
	type bucketAcc struct {
		ByProvider map[string]*ProviderTokenStats
	}
	bucketMap := make(map[int64]*bucketAcc)
	providerSet := make(map[string]struct{})

	for _, r := range rows {
		coarse := foldIntoBucket(r.BucketStart, bucketSeconds, requestedSize).Unix()
		b, ok := bucketMap[coarse]
		if !ok {
			b = &bucketAcc{ByProvider: make(map[string]*ProviderTokenStats)}
			bucketMap[coarse] = b
		}
		stats, ok := b.ByProvider[r.Provider]
		if !ok {
			stats = &ProviderTokenStats{}
			b.ByProvider[r.Provider] = stats
		}
		stats.PromptTokens += r.PromptTokens
		stats.CompletionTokens += r.CompletionTokens
		stats.TotalTokens += r.TotalTokens
		providerSet[r.Provider] = struct{}{}
	}

	buckets := make([]ProviderTokenHistogramBucket, 0, len(bucketMap))
	for ts, b := range bucketMap {
		buckets = append(buckets, ProviderTokenHistogramBucket{
			Timestamp:  time.Unix(ts, 0).UTC(),
			ByProvider: mergeProviderTokenStats(b.ByProvider),
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})
	providers := make([]string, 0, len(providerSet))
	for p := range providerSet {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	return buckets, providers
}

func mergeProviderTokenStats(in map[string]*ProviderTokenStats) map[string]ProviderTokenStats {
	out := make(map[string]ProviderTokenStats, len(in))
	for k, v := range in {
		if v == nil {
			out[k] = ProviderTokenStats{}
			continue
		}
		out[k] = *v
	}
	return out
}

// foldProviderLatencyBuckets merges per-(bucket, provider) latency rows.
func foldProviderLatencyBuckets(
	rows []providerLatencyRow,
	bucketSeconds, requestedSize int64,
	start, end *time.Time,
) ([]ProviderLatencyHistogramBucket, []string) {
	type bucketAcc struct {
		ByProvider map[string]*providerLatencyAcc
	}
	bucketMap := make(map[int64]*bucketAcc)
	providerSet := make(map[string]struct{})

	for _, r := range rows {
		coarse := foldIntoBucket(r.BucketStart, bucketSeconds, requestedSize).Unix()
		b, ok := bucketMap[coarse]
		if !ok {
			b = &bucketAcc{ByProvider: make(map[string]*providerLatencyAcc)}
			bucketMap[coarse] = b
		}
		a, ok := b.ByProvider[r.Provider]
		if !ok {
			a = &providerLatencyAcc{}
			b.ByProvider[r.Provider] = a
		}
		a.totalLatency += r.TotalLatencyMS
		a.latencyCount += r.LatencyCount
		a.requests += r.RequestCount
		providerSet[r.Provider] = struct{}{}
	}

	buckets := make([]ProviderLatencyHistogramBucket, 0, len(bucketMap))
	for ts, b := range bucketMap {
		stats := make(map[string]ProviderLatencyStats, len(b.ByProvider))
		for p, a := range b.ByProvider {
			var avg float64
			if a.latencyCount > 0 {
				avg = a.totalLatency / float64(a.latencyCount)
			}
			stats[p] = ProviderLatencyStats{
				AvgLatency:    avg,
				TotalRequests: a.requests,
			}
		}
		buckets = append(buckets, ProviderLatencyHistogramBucket{
			Timestamp:  time.Unix(ts, 0).UTC(),
			ByProvider: stats,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})
	providers := make([]string, 0, len(providerSet))
	for p := range providerSet {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	return buckets, providers
}

type providerLatencyAcc struct {
	totalLatency float64
	latencyCount int64
	requests     int64
}

// foldProviderThroughputBuckets merges per-(bucket, provider) throughput rows.
func foldProviderThroughputBuckets(
	rows []providerThroughputRow,
	bucketSeconds, requestedSize int64,
	start, end *time.Time,
) ([]ProviderThroughputHistogramBucket, []string) {
	type bucketAcc struct {
		ByProvider map[string]*providerThroughputAcc
	}
	bucketMap := make(map[int64]*bucketAcc)
	providerSet := make(map[string]struct{})

	for _, r := range rows {
		coarse := foldIntoBucket(r.BucketStart, bucketSeconds, requestedSize).Unix()
		b, ok := bucketMap[coarse]
		if !ok {
			b = &bucketAcc{ByProvider: make(map[string]*providerThroughputAcc)}
			bucketMap[coarse] = b
		}
		a, ok := b.ByProvider[r.Provider]
		if !ok {
			a = &providerThroughputAcc{}
			b.ByProvider[r.Provider] = a
		}
		a.totalLatency += r.TotalLatencyMS
		a.requests += r.RequestCount
		a.completionToks += r.CompletionTokens
		providerSet[r.Provider] = struct{}{}
	}

	buckets := make([]ProviderThroughputHistogramBucket, 0, len(bucketMap))
	for ts, b := range bucketMap {
		stats := make(map[string]ProviderThroughputStats, len(b.ByProvider))
		for p, a := range b.ByProvider {
			var tps float64
			if a.totalLatency > 0 {
				tps = float64(a.completionToks) / (a.totalLatency / 1000.0)
			}
			stats[p] = ProviderThroughputStats{
				TokensPerSecond:       tps,
				TotalCompletionTokens: a.completionToks,
				TotalRequests:         a.requests,
			}
		}
		buckets = append(buckets, ProviderThroughputHistogramBucket{
			Timestamp:  time.Unix(ts, 0).UTC(),
			ByProvider: stats,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})
	providers := make([]string, 0, len(providerSet))
	for p := range providerSet {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	return buckets, providers
}

type providerThroughputAcc struct {
	totalLatency   float64
	requests       int64
	completionToks int64
}
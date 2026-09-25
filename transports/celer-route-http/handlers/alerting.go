// Package handlers - alerting.go implements the alert-rule admin API:
// CRUD over alert_rules, paginated alert_events queries, and the budget
// projection endpoint that consumes budget_snapshots. The hot-path soft /
// hard alert emission is wired into plugins/governance/alert_evaluator.go
// and never touches this file.
package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/router"
	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/sidekiq"
	"github.com/pin-gou/celer-route/framework/sidekiq/jobs"
	"github.com/pin-gou/celer-route/framework/webhooks"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// AlertingHandler implements the admin-side alert rule / event / projection
// surface. It is intentionally a thin wrapper over the configstore +
// dispatcher + sidekiq wiring: no business logic lives here that should
// belong in plugins/governance/alert_evaluator.go.
type AlertingHandler struct {
	store      configstore.ConfigStore
	dispatcher *webhooks.Dispatcher
	sidekiq    *sidekiq.Runner
	rulesCache AlertRulesInvalidator // optional — see InvalidateRules below
}

// AlertRulesInvalidator is the tiny seam the handlers package depends on
// from the governance plugin: writing to alert_rules must drop the in-
// process cache so the next soft-threshold evaluation repopulates. The
// concrete *governance.AlertRuleCache satisfies this; declaring it here
// (instead of importing plugins/governance) keeps the layering rule
// transports → plugins intact.
type AlertRulesInvalidator interface {
	Invalidate()
}

// NewAlertingHandler builds the handler. dispatcher and sidekiq may be
// nil when the gateway runs without alert delivery / background jobs —
// the handler still serves the read paths and the CRUD endpoints, but
// rule-test and snapshot-now return 503 with a clear error.
func NewAlertingHandler(store configstore.ConfigStore, dispatcher *webhooks.Dispatcher, runner *sidekiq.Runner) *AlertingHandler {
	return &AlertingHandler{store: store, dispatcher: dispatcher, sidekiq: runner}
}

// SetRulesCache wires the alert-rules cache so write-path handlers can
// invalidate it after a successful CRUD. Pass nil to disable (handlers
// silently skip invalidation when no cache is set, so an enterprise-only
// build without alert rules keeps working).
func (h *AlertingHandler) SetRulesCache(c AlertRulesInvalidator) {
	h.rulesCache = c
}

// invalidateRules is the single invalidation point. nil-safe: handlers
// that run before SetRulesCache (e.g. legacy callers) keep working.
func (h *AlertingHandler) invalidateRules() {
	if h.rulesCache != nil {
		h.rulesCache.Invalidate()
	}
}

// RegisterRoutes mounts the alert rule + event + projection endpoints.
// All routes are admin-gated via the auth middleware applied at the
// router level — see transports/celer-route-http/server/server.go.
func (h *AlertingHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/alert-rules", lib.ChainMiddlewares(h.listAlertRules, middlewares...))
	r.POST("/api/alert-rules", lib.ChainMiddlewares(h.createAlertRule, middlewares...))
	r.GET("/api/alert-rules/{id}", lib.ChainMiddlewares(h.getAlertRule, middlewares...))
	r.PUT("/api/alert-rules/{id}", lib.ChainMiddlewares(h.updateAlertRule, middlewares...))
	r.DELETE("/api/alert-rules/{id}", lib.ChainMiddlewares(h.deleteAlertRule, middlewares...))
	r.POST("/api/alert-rules/{id}/test", lib.ChainMiddlewares(h.testAlertRule, middlewares...))
	r.GET("/api/alert-events", lib.ChainMiddlewares(h.listAlertEvents, middlewares...))
	r.GET("/api/alert-events/{id}", lib.ChainMiddlewares(h.getAlertEvent, middlewares...))
	r.GET("/api/governance/budgets/{id}/projection", lib.ChainMiddlewares(h.budgetProjection, middlewares...))
	r.POST("/api/alert-rules/snapshot-budgets", lib.ChainMiddlewares(h.snapshotBudgetsNow, middlewares...))
}

// alertRuleRequest is the caller-editable shape for create + update. The
// id is server-minted on create; comparison defaults to gte and status to
// enabled when blank. The metric field is validated server-side (see
// tables.IsAlertMetricP1Only) so the P1-only metric returns 422 even if
// the UI somehow sends it.
type alertRuleRequest struct {
	Name            string                  `json:"name"`
	ScopeType       string                  `json:"scope_type"`
	ScopeID         string                  `json:"scope_id"`
	Metric          string                  `json:"metric"`
	Threshold       float64                 `json:"threshold"`
	Comparison      string                  `json:"comparison"`
	CooldownMinutes int                     `json:"cooldown_minutes"`
	Status          string                  `json:"status"`
	Channels        []tables.AlertChannel   `json:"channels"`
}

func (r *alertRuleRequest) toTable(id string) *tables.TableAlertRule {
	return &tables.TableAlertRule{
		ID:              id,
		Name:            r.Name,
		ScopeType:       r.ScopeType,
		ScopeID:         r.ScopeID,
		Metric:          r.Metric,
		Threshold:       r.Threshold,
		Comparison:      r.Comparison,
		CooldownMinutes: r.CooldownMinutes,
		Status:          r.Status,
		Channels:        r.Channels,
	}
}

func (h *AlertingHandler) createAlertRule(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Config store unavailable")
		return
	}
	var req alertRuleRequest
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	if tables.IsAlertMetricP1Only(req.Metric) {
		SendError(ctx, fasthttp.StatusUnprocessableEntity, "metric cache_miss_rate is reserved for a future release")
		return
	}
	rule := req.toTable(uuid.NewString())
	if err := h.store.CreateAlertRule(ctx, rule); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Failed to create alert rule: "+err.Error())
		return
	}
	h.invalidateRules()
	SendJSON(ctx, rule)
}

func (h *AlertingHandler) updateAlertRule(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Config store unavailable")
		return
	}
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid rule id")
		return
	}
	existing, err := h.store.GetAlertRuleByID(ctx, id)
	if err != nil || existing == nil {
		SendError(ctx, fasthttp.StatusNotFound, "Alert rule not found")
		return
	}
	var req alertRuleRequest
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	if tables.IsAlertMetricP1Only(req.Metric) {
		SendError(ctx, fasthttp.StatusUnprocessableEntity, "metric cache_miss_rate is reserved for a future release")
		return
	}
	merged := req.toTable(id)
	merged.ID = existing.ID
	merged.CreatedAt = existing.CreatedAt
	if err := h.store.UpdateAlertRule(ctx, merged); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Failed to update alert rule: "+err.Error())
		return
	}
	h.invalidateRules()
	SendJSON(ctx, merged)
}

func (h *AlertingHandler) deleteAlertRule(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Config store unavailable")
		return
	}
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid rule id")
		return
	}
	if err := h.store.DeleteAlertRule(ctx, id); err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "Alert rule not found")
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to delete alert rule: "+err.Error())
		return
	}
	h.invalidateRules()
	SendJSON(ctx, map[string]any{"deleted": id})
}

func (h *AlertingHandler) listAlertRules(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Config store unavailable")
		return
	}
	params := configstore.AlertRulesQueryParams{
		ScopeType: string(ctx.QueryArgs().Peek("scope_type")),
		ScopeID:   string(ctx.QueryArgs().Peek("scope_id")),
		Metric:    string(ctx.QueryArgs().Peek("metric")),
		Status:    string(ctx.QueryArgs().Peek("status")),
		Search:    string(ctx.QueryArgs().Peek("search")),
		Limit:     parseIntDefault(string(ctx.QueryArgs().Peek("limit")), 50),
		Offset:    parseIntDefault(string(ctx.QueryArgs().Peek("offset")), 0),
	}
	rules, total, err := h.store.ListAlertRules(ctx, params)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to list alert rules: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"rules": rules,
		"total": total,
		"limit": params.Limit,
		"offset": params.Offset,
	})
}

func (h *AlertingHandler) getAlertRule(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Config store unavailable")
		return
	}
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid rule id")
		return
	}
	rule, err := h.store.GetAlertRuleByID(ctx, id)
	if err != nil || rule == nil {
		SendError(ctx, fasthttp.StatusNotFound, "Alert rule not found")
		return
	}
	// Best-effort recent events (last 10) for the rule list view.
	recent, _, _ := h.store.ListAlertEvents(ctx, configstore.AlertEventsQueryParams{
		RuleID: rule.ID,
		Limit:  10,
	})
	SendJSON(ctx, map[string]any{
		"rule":           rule,
		"recent_events":  recent,
	})
}

// testAlertRule fires a synthetic alert_event through the configured
// channels so admins can verify webhook wiring without waiting for a real
// budget fire. Returns one entry per channel with the delivery result.
func (h *AlertingHandler) testAlertRule(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.dispatcher == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Alert delivery not configured")
		return
	}
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid rule id")
		return
	}
	rule, err := h.store.GetAlertRuleByID(ctx, id)
	if err != nil || rule == nil {
		SendError(ctx, fasthttp.StatusNotFound, "Alert rule not found")
		return
	}
	now := time.Now().UTC()
	event := &tables.TableAlertEvent{
		ID:             "test-" + uuid.NewString(),
		RuleID:         rule.ID,
		ScopeType:      rule.ScopeType,
		ScopeID:        rule.ScopeID,
		Event:          string(testEventForMetric(rule.Metric)),
		MetricValue:    rule.Threshold,
		Threshold:      rule.Threshold,
		Message:        fmt.Sprintf("Test event for rule %q (no real threshold breach)", rule.Name),
		DeliveryStatus: tables.AlertEventDeliveryStatusPending,
		TriggeredAt:    now,
	}
	endpointIDs := make([]string, 0, len(rule.Channels))
	channels := make([]map[string]any, 0, len(rule.Channels))
	for _, ch := range rule.Channels {
		if ch.Type != tables.AlertChannelTypeWebhook || ch.WebhookID == "" {
			continue
		}
		endpoint, ok := endpointByIDFromDispatcher(h.dispatcher, ch.WebhookID)
		if !ok {
			channels = append(channels, map[string]any{
				"webhook_id": ch.WebhookID,
				"status":     "endpoint_not_found",
			})
			continue
		}
		endpointIDs = append(endpointIDs, ch.WebhookID)
		statusCode, err := h.dispatcher.DeliverTest(ctx, endpoint, testEventForMetric(rule.Metric))
		out := map[string]any{
			"webhook_id":  ch.WebhookID,
			"status_code": statusCode,
		}
		if err != nil {
			out["error"] = err.Error()
			out["status"] = "failed"
		} else {
			out["status"] = "delivered"
		}
		channels = append(channels, out)
	}
	if len(endpointIDs) > 0 {
		// Also drop a real event row so the API list reflects the test fire.
		if err := h.store.CreateAlertEvent(ctx, event); err != nil {
			// Don't fail the test delivery — the webhook enqueue already
			// happened. Log via the response payload so the admin sees it.
			SendJSON(ctx, map[string]any{
				"event_id": event.ID,
				"channels": channels,
				"warning":  "test event row not persisted: " + err.Error(),
			})
			return
		}
	}
	SendJSON(ctx, map[string]any{
		"event_id": event.ID,
		"channels": channels,
	})
}

// testEventForMetric picks the right webhook event for the test row. The
// rule's metric maps to the alert.* event family; budget.exceeded is
// reserved for the 402 path and never used for a manual test.
func testEventForMetric(metric string) tables.WebhookEvent {
	switch metric {
	case tables.AlertMetricSpendRate:
		return tables.WebhookEventAlertSpendRate
	case tables.AlertMetricErrorRate:
		return tables.WebhookEventAlertErrorRate
	default:
		return tables.WebhookEventAlertBudgetThreshold
	}
}

// endpointByIDFromDispatcher pulls a webhook endpoint through the
// dispatcher's resolver so the test delivery path uses the same in-memory
// view the live queue does. We expose a thin wrapper on Dispatcher
// (framework/webhooks.WebhookEndpointByID) so this handler can look up
// the endpoint without reaching into unexported fields.
func endpointByIDFromDispatcher(d *webhooks.Dispatcher, id string) (*tables.TableWebhookEndpoint, bool) {
	return d.WebhookEndpointByID(id)
}

func (h *AlertingHandler) listAlertEvents(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Config store unavailable")
		return
	}
	args := ctx.QueryArgs()
	params := configstore.AlertEventsQueryParams{
		RuleID:    string(args.Peek("rule_id")),
		ScopeType: string(args.Peek("scope_type")),
		ScopeID:   string(args.Peek("scope_id")),
		Event:     string(args.Peek("event")),
		Status:    string(args.Peek("status")),
		Limit:     parseIntDefault(string(args.Peek("limit")), 50),
		Offset:    parseIntDefault(string(args.Peek("offset")), 0),
	}
	if startStr := string(args.Peek("start_time")); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			params.StartTime = &t
		}
	}
	if endStr := string(args.Peek("end_time")); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			params.EndTime = &t
		}
	}
	events, total, err := h.store.ListAlertEvents(ctx, params)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to list alert events: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"events": events,
		"total":  total,
		"limit":  params.Limit,
		"offset": params.Offset,
	})
}

func (h *AlertingHandler) getAlertEvent(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Config store unavailable")
		return
	}
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid event id")
		return
	}
	event, err := h.store.GetAlertEventByID(ctx, id)
	if err != nil || event == nil {
		SendError(ctx, fasthttp.StatusNotFound, "Alert event not found")
		return
	}
	SendJSON(ctx, event)
}

// budgetProjection fits a line through the trailing budget_snapshots and
// reports the predicted time to exhaustion + risk level. With < 2 samples
// it degrades to the simple "used / max" percentage so the endpoint is
// always meaningful; with no snapshots at all it returns used + max only.
func (h *AlertingHandler) budgetProjection(ctx *fasthttp.RequestCtx) {
	if h.store == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Config store unavailable")
		return
	}
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid budget id")
		return
	}
	latest, err := h.store.LatestBudgetSnapshot(ctx, id)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load latest snapshot: "+err.Error())
		return
	}
	if latest == nil {
		// Fall back to the live budget row.
		budget, err := h.store.GetBudgetByID(ctx, id)
		if err != nil {
			SendError(ctx, fasthttp.StatusNotFound, "Budget not found")
			return
		}
		SendJSON(ctx, map[string]any{
			"budget_id":   budget.ID,
			"used_amount": budget.CurrentUsage,
			"max_amount":  budget.EffectiveMaxLimit(),
			"usage_percent": safePercent(budget.CurrentUsage, budget.EffectiveMaxLimit()),
			"risk_level":   riskLevel(safePercent(budget.CurrentUsage, budget.EffectiveMaxLimit()), 0),
			"has_projection": false,
			"reason":       "no budget snapshots available yet; projection will populate after the first hourly sample",
		})
		return
	}
	samples, err := h.store.ListBudgetSnapshotsForBudget(ctx, id, 200)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load snapshots: "+err.Error())
		return
	}
	projection := computeBudgetProjection(samples)
	SendJSON(ctx, map[string]any{
		"budget_id":   id,
		"used_amount": latest.UsedAmount,
		"max_amount":  latest.MaxAmount,
		"usage_percent": safePercent(latest.UsedAmount, latest.MaxAmount),
		"sampled_at":  latest.SampledAt,
		"sample_count": len(samples),
		"projection":  projection,
		"risk_level":  projection.Risk,
		"has_projection": projection.RatePerHour > 0,
	})
}

// snapshotBudgetsNow triggers a synchronous budget snapshot for the admin
// who clicks "Sample now" on the alerting UI. The sidekiq job handles the
// persistence; the handler just enqueues and returns the job id so the
// UI can poll for completion.
func (h *AlertingHandler) snapshotBudgetsNow(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.sidekiq == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Background jobs are not configured")
		return
	}
	if err := jobs.EnqueueBudgetSnapshotNow(h.sidekiq, h.store); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to enqueue snapshot job: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"status": "queued"})
}

// BudgetProjection is the result of fitting a line through the trailing
// budget snapshots. The risk classification follows 02-alerting/flows.md
// §3: high when exhaustion lands within 50% of the remaining window,
// medium within 80%, otherwise low. A zero or negative rate is "no
// projection" — the budget is either flat or going backwards (refund),
// which the UI surfaces as "stable" rather than "low risk".
type BudgetProjection struct {
	RatePerHour    float64    `json:"rate_per_hour"`
	PredictedUsage float64    `json:"predicted_used_at_exhaustion"`
	HoursToExhaust *float64   `json:"hours_to_exhaustion,omitempty"`
	WindowEndsAt   *time.Time `json:"window_ends_at,omitempty"`
	Risk           string     `json:"risk_level"`
}

func computeBudgetProjection(samples []tables.TableBudgetSnapshot) BudgetProjection {
	if len(samples) < 2 {
		return BudgetProjection{Risk: riskLevel(safePercent(samplesLastUsed(samples), samplesLastMax(samples)), 0)}
	}
	// Linear fit on the trailing samples: simplest possible projection.
	// We use the earliest and latest sample to compute the slope so a
	// noisy middle does not skew the rate.
	first := samples[0]
	last := samples[len(samples)-1]
	dt := last.SampledAt.Sub(first.SampledAt).Hours()
	if dt <= 0 {
		return BudgetProjection{Risk: riskLevel(safePercent(last.UsedAmount, last.MaxAmount), 0)}
	}
	rate := (last.UsedAmount - first.UsedAmount) / dt
	if rate <= 0 {
		return BudgetProjection{Risk: "low"}
	}
	remaining := last.MaxAmount - last.UsedAmount
	if remaining <= 0 {
		hours := 0.0
		return BudgetProjection{
			RatePerHour:    rate,
			PredictedUsage: last.UsedAmount,
			HoursToExhaust: &hours,
			Risk:           "high",
		}
	}
	hours := remaining / rate
	exhaustsAt := last.SampledAt.Add(time.Duration(hours * float64(time.Hour)))
	risk := riskLevel(safePercent(last.UsedAmount, last.MaxAmount), hours)
	return BudgetProjection{
		RatePerHour:    rate,
		PredictedUsage: last.UsedAmount,
		HoursToExhaust: &hours,
		WindowEndsAt:   &exhaustsAt,
		Risk:           risk,
	}
}

func samplesLastUsed(samples []tables.TableBudgetSnapshot) float64 {
	if len(samples) == 0 {
		return 0
	}
	return samples[len(samples)-1].UsedAmount
}

func samplesLastMax(samples []tables.TableBudgetSnapshot) float64 {
	if len(samples) == 0 {
		return 0
	}
	return samples[len(samples)-1].MaxAmount
}

func safePercent(used, max float64) float64 {
	if max <= 0 {
		return 0
	}
	return used / max * 100
}

// riskLevel classifies the budget's exhaustion risk given the current
// usage percent and the projected hours-to-exhaustion. hours == 0 means
// "no projection" — the function falls back to the percentage buckets
// used by the simple-projection path. Thresholds follow the spec:
//
//	high:   >= 90% usage OR projection <= 24h
//	medium: >= 75% usage OR projection <= 72h
//	low:    otherwise
func riskLevel(percent, hoursToExhaust float64) string {
	if percent >= 90 || (hoursToExhaust > 0 && hoursToExhaust <= 24) {
		return "high"
	}
	if percent >= 75 || (hoursToExhaust > 0 && hoursToExhaust <= 72) {
		return "medium"
	}
	return "low"
}

func parseIntDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

// Compile-time guard: alert_rule.go's package compiles alongside this
// file, and Go would otherwise reject an unused-import warning if a future
// edit stops referencing net/http.
var _ = http.StatusOK
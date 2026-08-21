package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/framework/configstore/tables"
	"github.com/pin-gou/pg-gateway/framework/logstore"
	"github.com/pin-gou/pg-gateway/framework/queryscope"
	"github.com/pin-gou/pg-gateway/framework/sidekiq"
	loggingplugin "github.com/pin-gou/pg-gateway/plugins/logging"
	"github.com/pin-gou/pg-gateway/transports/pg-gateway-http/lib"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

// TestShouldUseFilterDataCacheAllowsUnscopedEmptyQuery verifies unscoped
// requests can still share the no-query filterdata cache.
func TestShouldUseFilterDataCacheAllowsUnscopedEmptyQuery(t *testing.T) {
	if !shouldUseFilterDataCache(context.Background(), "") {
		t.Fatal("expected unscoped empty-query request to use filterdata cache")
	}
	if !shouldUseFilterDataCache(context.Background(), "   ") {
		t.Fatal("expected whitespace-only query to use filterdata cache")
	}
}

// TestShouldUseFilterDataCacheRejectsSearchQuery verifies search requests are
// request-specific and must not share the empty-query cache.
func TestShouldUseFilterDataCacheRejectsSearchQuery(t *testing.T) {
	if shouldUseFilterDataCache(context.Background(), "vk") {
		t.Fatal("expected non-empty query to bypass filterdata cache")
	}
}

// TestShouldUseFilterDataCacheRejectsScopedContext verifies DAC-scoped
// requests never consume or populate the shared all-data cache.
func TestShouldUseFilterDataCacheRejectsScopedContext(t *testing.T) {
	ctx := queryscope.WithQueryScope(context.Background(), func(db *gorm.DB) *gorm.DB {
		return db.Where("1 = 0")
	})
	if shouldUseFilterDataCache(ctx, "") {
		t.Fatal("expected scoped request to bypass filterdata cache")
	}
}

// TestGetMCPLogByIDRedactionMapping verifies raw mappings stay hidden and only resolver-approved mappings are returned.
func TestGetMCPLogByIDRedactionMapping(t *testing.T) {
	SetLogger(&mockLogger{})
	revealed := &schemas.RedactionMapsByPhase{
		Input: map[string]string{"EMAIL-1": "revealed@example.com"},
	}
	tests := []struct {
		name        string
		resolver    *staticMCPLogRedactionResolver
		wantMapping bool
		wantCalls   int
	}{
		{name: "no resolver"},
		{name: "authorized mapping", resolver: &staticMCPLogRedactionResolver{mapping: revealed}, wantMapping: true, wantCalls: 1},
		{name: "resolver error", resolver: &staticMCPLogRedactionResolver{err: errors.New("decode failed")}, wantCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &dashboardLogManager{mcpLog: &logstore.MCPToolLog{
				ID:               "mcp-1",
				RedactionMapping: `plain:{"input":{"EMAIL-1":"private@example.com"}}`,
			}}
			handler := &LoggingHandler{logManager: manager}
			if tt.resolver != nil {
				handler.SetMCPLogRedactionMappingResolver(tt.resolver)
			}
			ctx := &fasthttp.RequestCtx{}
			ctx.SetUserValue("id", "mcp-1")

			handler.getMCPLogByID(ctx)

			if ctx.Response.StatusCode() != fasthttp.StatusOK {
				t.Fatalf("status = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusOK)
			}
			var response struct {
				RedactionMapping *schemas.RedactionMapsByPhase `json:"redaction_mapping"`
			}
			if err := json.Unmarshal(ctx.Response.Body(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if bytes.Contains(ctx.Response.Body(), []byte("private@example.com")) {
				t.Fatalf("raw persisted mapping leaked in response: %s", ctx.Response.Body())
			}
			if tt.wantMapping != (response.RedactionMapping != nil) {
				t.Fatalf("redaction mapping present = %t, want %t", response.RedactionMapping != nil, tt.wantMapping)
			}
			if tt.wantMapping && response.RedactionMapping.Input["EMAIL-1"] != "revealed@example.com" {
				t.Fatalf("revealed mapping = %#v", response.RedactionMapping)
			}
			if tt.resolver != nil && tt.resolver.calls != tt.wantCalls {
				t.Fatalf("resolver calls = %d, want %d", tt.resolver.calls, tt.wantCalls)
			}
		})
	}
}

// TestShouldCacheFilterDimensions_NarrowsToRawScans verifies the cache is spent
// only where it saves real work. Matview-backed dimensions are indexed lookups
// and a cache entry serves exactly one caller, so they are not worth caching;
// metadata_keys still hits the raw logs table and is.
func TestShouldCacheFilterDimensions_NarrowsToRawScans(t *testing.T) {
	pg := &LoggingHandler{config: &lib.Config{
		LogsStoreConfig: &logstore.Config{Type: logstore.LogStoreTypePostgres},
	}}

	cases := []struct {
		name string
		dims []string
		want bool
	}{
		{"single matview dimension", []string{filterDimUsers}, false},
		{"several matview dimensions", []string{filterDimUsers, filterDimTeams, filterDimModels}, false},
		{"metadata keys alone", []string{filterDimMetadataKeys}, true},
		{"metadata keys mixed in", []string{filterDimUsers, filterDimMetadataKeys}, true},
		{"default all dimensions", allFilterDimensions, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pg.shouldCacheFilterDimensions(tc.dims); got != tc.want {
				t.Fatalf("shouldCacheFilterDimensions(%v) = %v, want %v", tc.dims, got, tc.want)
			}
		})
	}
}

// TestShouldCacheFilterDimensions_NonPostgresCachesEverything verifies stores
// without matviews keep the original behaviour: there every dimension is a raw
// 30-day DISTINCT, so none of them should lose the cache.
func TestShouldCacheFilterDimensions_NonPostgresCachesEverything(t *testing.T) {
	for _, h := range []*LoggingHandler{
		{config: &lib.Config{LogsStoreConfig: &logstore.Config{Type: logstore.LogStoreTypeSQLite}}},
		{config: &lib.Config{}}, // no logs-store config: fail safe, keep caching
		{},                      // no config at all
	} {
		if !h.shouldCacheFilterDimensions([]string{filterDimUsers}) {
			t.Fatal("stores without matviews must keep caching every dimension")
		}
	}
}

// TestFilterDataCacheIdentity_PartitionsPerCaller is the regression for
// cross-user leakage through the filterdata cache. Filter dropdowns are
// row-visibility-scoped, but the scope is resolved below the handler, so the
// cache cannot detect it — two callers must therefore never share a key.
func TestFilterDataCacheIdentity_PartitionsPerCaller(t *testing.T) {
	withUser := func(roleID uint) *fasthttp.RequestCtx {
		ctx := &fasthttp.RequestCtx{}
		ctx.SetUserValue(schemas.BifrostContextKeyUserRoleID, roleID)
		return ctx
	}

	alice := filterDataCacheIdentity(withUser(2))
	bob := filterDataCacheIdentity(withUser(2))
	if alice != bob {
		t.Fatalf("same role must share a cache partition, got %q vs %q", alice, bob)
	}

	// A role change flips visibility, so it must miss the cache immediately
	// rather than serve the old scope for the remainder of the TTL.
	if promoted := filterDataCacheIdentity(withUser(1)); promoted == alice {
		t.Fatalf("role change must repartition the cache, both got %q", alice)
	}

	// Unauthenticated / local-admin requests keep the single shared partition.
	if anon := filterDataCacheIdentity(withUser(0)); anon != "anon" {
		t.Fatalf("identity-less request should use the shared partition, got %q", anon)
	}
}

func TestGetDashboard(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		failStats  bool
		wantStatus int
		assert     func(t *testing.T, mgr *dashboardLogManager, body []byte)
	}{
		{
			name:       "success includes all sections",
			query:      "providers=openai&models=gpt-4&tool_names=calculator&server_labels=primary",
			wantStatus: fasthttp.StatusOK,
			assert: func(t *testing.T, mgr *dashboardLogManager, body []byte) {
				t.Helper()
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatalf("decode dashboard response: %v", err)
				}
				for _, key := range []string{"meta", "overview", "provider_usage", "model_rankings", "dimension_rankings", "mcp"} {
					if _, ok := payload[key]; !ok {
						t.Fatalf("expected top-level key %q in response", key)
					}
				}
				var response logstore.DashboardResult
				if err := json.Unmarshal(body, &response); err != nil {
					t.Fatalf("decode dashboard result: %v", err)
				}
				if response.Overview.Models == nil || response.ModelRankings.Histogram == nil {
					t.Fatal("expected shared model histogram to populate both sections")
				}
				if len(response.DimensionRankings) != len(dashboardRankingDimensions) {
					t.Fatalf("expected %d dimension rankings, got %d", len(dashboardRankingDimensions), len(response.DimensionRankings))
				}
				if got := mgr.lastLLMFilters.Providers; len(got) != 1 || got[0] != "openai" {
					t.Fatalf("expected LLM providers filter, got %#v", got)
				}
				if got := mgr.lastMCPFilters.ToolNames; len(got) != 1 || got[0] != "calculator" {
					t.Fatalf("expected MCP tool_names filter, got %#v", got)
				}
			},
		},
		{
			name:       "sub-query error returns no partial dashboard",
			failStats:  true,
			wantStatus: fasthttp.StatusInternalServerError,
			assert: func(t *testing.T, mgr *dashboardLogManager, body []byte) {
				t.Helper()
				if json.Valid(body) {
					var payload map[string]json.RawMessage
					if err := json.Unmarshal(body, &payload); err == nil {
						if _, ok := payload["overview"]; ok {
							t.Fatalf("expected error payload, got partial dashboard: %s", string(body))
						}
					}
				}
			},
		},
		{
			name:       "MCP filters are isolated from LLM filters",
			query:      "providers=openai&models=gpt-4&tool_names=calculator,clock&server_labels=primary&virtual_key_ids=vk-llm",
			wantStatus: fasthttp.StatusOK,
			assert: func(t *testing.T, mgr *dashboardLogManager, body []byte) {
				t.Helper()
				if len(mgr.lastLLMFilters.Providers) != 1 || mgr.lastLLMFilters.Providers[0] != "openai" {
					t.Fatalf("expected LLM providers filter, got %#v", mgr.lastLLMFilters.Providers)
				}
				if len(mgr.lastMCPFilters.ToolNames) != 2 {
					t.Fatalf("expected MCP tool filters, got %#v", mgr.lastMCPFilters.ToolNames)
				}
				if mgr.lastLLMFilters.ContentSearch == "calculator" {
					t.Fatal("MCP tool_names leaked into LLM filters")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetLogger(&mockLogger{})

			mgr := &dashboardLogManager{failStats: tt.failStats}
			h := &LoggingHandler{logManager: mgr}
			var req fasthttp.Request
			uri := "/api/logs/dashboard"
			if tt.query != "" {
				uri += "?" + tt.query
			}
			req.SetRequestURI(uri)

			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}, nil)

			h.getDashboard(ctx)

			if got := ctx.Response.StatusCode(); got != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, got, string(ctx.Response.Body()))
			}
			if tt.assert != nil {
				tt.assert(t, mgr, ctx.Response.Body())
			}
		})
	}
}

func TestRecalculateLogCostsResolvesPeriodFilter(t *testing.T) {
	SetLogger(&mockLogger{})

	mgr := &dashboardLogManager{}
	store := newFakeSidekiqStore()
	runner := sidekiq.New(store, &mockLogger{}, 1, "")
	h := &LoggingHandler{logManager: mgr}
	h.SetSidekiqBackend(runner, store)

	var req fasthttp.Request
	req.Header.SetMethod(fasthttp.MethodPost)
	req.SetRequestURI("/api/logs/recalculate-cost")
	req.Header.SetContentType("application/json")
	req.SetBodyString(`{"filters":{"period":"1h"}}`)

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}, nil)

	h.recalculateLogCosts(ctx)

	// The job is enqueued for background processing, so the endpoint returns 202.
	if got := ctx.Response.StatusCode(); got != fasthttp.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", got, string(ctx.Response.Body()))
	}
	// The period must be resolved into an explicit window before the job is built.
	filters := mgr.lastRecalculateFilters
	if filters.StartTime == nil || filters.EndTime == nil {
		t.Fatalf("expected period to resolve start/end, got start=%v end=%v", filters.StartTime, filters.EndTime)
	}
	if !filters.EndTime.After(*filters.StartTime) {
		t.Fatalf("expected end_time after start_time, got start=%s end=%s", filters.StartTime, filters.EndTime)
	}
	if store.createdCount() != 1 {
		t.Fatalf("expected exactly one job to be enqueued, got %d", store.createdCount())
	}
}

func TestRecalculateLogCostsRejectsDuplicateJob(t *testing.T) {
	SetLogger(&mockLogger{})

	mgr := &dashboardLogManager{}
	store := newFakeSidekiqStore()
	// Seed an in-flight job so the endpoint should refuse to start a second one.
	store.inFlight = &tables.TableSidekiqJob{
		ID:       "logs_recalculate_cost_existing",
		Kind:     loggingplugin.CostRecalcJobKind,
		Status:   tables.SidekiqStatusRunning,
		Metadata: "{}",
	}
	runner := sidekiq.New(store, &mockLogger{}, 1, "")
	h := &LoggingHandler{logManager: mgr}
	h.SetSidekiqBackend(runner, store)

	var req fasthttp.Request
	req.Header.SetMethod(fasthttp.MethodPost)
	req.SetRequestURI("/api/logs/recalculate-cost")
	req.Header.SetContentType("application/json")
	req.SetBodyString(`{"filters":{"period":"1h"}}`)

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}, nil)

	h.recalculateLogCosts(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusConflict {
		t.Fatalf("expected status 409, got %d: %s", got, string(ctx.Response.Body()))
	}
	if store.createdCount() != 0 {
		t.Fatalf("expected no new job to be enqueued, got %d", store.createdCount())
	}
}

func TestCancelRecalculateCost(t *testing.T) {
	SetLogger(&mockLogger{})

	// newHandler wires a handler over a store seeded with one recalculation job.
	newHandler := func(job *tables.TableSidekiqJob) (*LoggingHandler, *fakeSidekiqStore) {
		store := newFakeSidekiqStore()
		if job != nil {
			store.jobs[job.ID] = job
			if !tables.IsSidekiqTerminalStatus(job.Status) {
				store.inFlight = job
			}
		}
		h := &LoggingHandler{logManager: &dashboardLogManager{}}
		h.SetSidekiqBackend(sidekiq.New(store, &mockLogger{}, 1, ""), store)
		return h, store
	}

	call := func(h *LoggingHandler, uri string) *fasthttp.RequestCtx {
		var req fasthttp.Request
		req.Header.SetMethod(fasthttp.MethodPost)
		req.SetRequestURI(uri)
		ctx := &fasthttp.RequestCtx{}
		ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}, nil)
		h.cancelRecalculateCost(ctx)
		return ctx
	}

	runningJob := func() *tables.TableSidekiqJob {
		return &tables.TableSidekiqJob{
			ID:       "job-1",
			Kind:     loggingplugin.CostRecalcJobKind,
			Status:   tables.SidekiqStatusRunning,
			Metadata: `{"total":10,"processed":4,"updated":3,"skipped":1}`,
		}
	}

	t.Run("cancels the in-flight job when no id is given", func(t *testing.T) {
		h, store := newHandler(runningJob())
		ctx := call(h, "/api/logs/recalculate-cost/cancel")

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
			t.Fatalf("expected 200, got %d: %s", got, ctx.Response.Body())
		}
		if got := store.jobs["job-1"].Status; got != tables.SidekiqStatusCancelled {
			t.Fatalf("job status = %q, want cancelled", got)
		}
		var body recalcJobStatus
		if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body.Status != tables.SidekiqStatusCancelled {
			t.Fatalf("response status = %q, want cancelled", body.Status)
		}
		// The counters committed before the stop must survive for the UI to report.
		if body.Updated != 3 || body.Skipped != 1 || body.Processed != 4 {
			t.Fatalf("partial progress lost: %+v", body)
		}
	})

	t.Run("cancels the job named by id", func(t *testing.T) {
		h, store := newHandler(runningJob())
		ctx := call(h, "/api/logs/recalculate-cost/cancel?id=job-1")

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
			t.Fatalf("expected 200, got %d: %s", got, ctx.Response.Body())
		}
		if got := store.jobs["job-1"].Status; got != tables.SidekiqStatusCancelled {
			t.Fatalf("job status = %q, want cancelled", got)
		}
	})

	t.Run("an already-terminal job is returned unchanged", func(t *testing.T) {
		done := runningJob()
		done.Status = tables.SidekiqStatusCompleted
		h, store := newHandler(done)
		ctx := call(h, "/api/logs/recalculate-cost/cancel?id=job-1")

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
			t.Fatalf("expected 200, got %d: %s", got, ctx.Response.Body())
		}
		if got := store.jobs["job-1"].Status; got != tables.SidekiqStatusCompleted {
			t.Fatalf("a completed job must not be rewritten, got %q", got)
		}
	})

	t.Run("refuses to cancel a job of another kind", func(t *testing.T) {
		other := runningJob()
		other.Kind = "some_other_job"
		h, store := newHandler(other)
		ctx := call(h, "/api/logs/recalculate-cost/cancel?id=job-1")

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", got, ctx.Response.Body())
		}
		if got := store.jobs["job-1"].Status; got != tables.SidekiqStatusRunning {
			t.Fatalf("unrelated job must be untouched, got %q", got)
		}
	})

	t.Run("404 when there is nothing to cancel", func(t *testing.T) {
		h, _ := newHandler(nil)
		ctx := call(h, "/api/logs/recalculate-cost/cancel")

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", got, ctx.Response.Body())
		}
	})

	t.Run("503 when the background runner is not wired", func(t *testing.T) {
		h := &LoggingHandler{logManager: &dashboardLogManager{}}
		ctx := call(h, "/api/logs/recalculate-cost/cancel")

		if got := ctx.Response.StatusCode(); got != fasthttp.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", got, ctx.Response.Body())
		}
	})
}

// fakeSidekiqStore implements both sidekiq.Store (for the runner) and
// handlers.SidekiqJobStore (for the endpoints), backed by an in-memory map.
type fakeSidekiqStore struct {
	mu       sync.Mutex
	jobs     map[string]*tables.TableSidekiqJob
	created  int
	inFlight *tables.TableSidekiqJob
}

func newFakeSidekiqStore() *fakeSidekiqStore {
	return &fakeSidekiqStore{jobs: make(map[string]*tables.TableSidekiqJob)}
}

func (s *fakeSidekiqStore) createdCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.created
}

func (s *fakeSidekiqStore) CreateSidekiqJob(ctx context.Context, job *tables.TableSidekiqJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.created++
	copy := *job
	s.jobs[job.ID] = &copy
	return nil
}

func (s *fakeSidekiqStore) GetSidekiqJob(ctx context.Context, id string) (*tables.TableSidekiqJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		copy := *job
		return &copy, nil
	}
	return nil, nil
}

func (s *fakeSidekiqStore) GetInFlightSidekiqJobByKind(ctx context.Context, kind string) (*tables.TableSidekiqJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inFlight != nil && s.inFlight.Kind == kind {
		copy := *s.inFlight
		return &copy, nil
	}
	return nil, nil
}

func (s *fakeSidekiqStore) ClaimSidekiqJob(ctx context.Context, id, runnerID string, staleBefore time.Time) (bool, error) {
	return true, nil
}
func (s *fakeSidekiqStore) ClaimPartitionedSidekiqJob(ctx context.Context, id, runnerID string, staleBefore time.Time, partitioningKey string, createdAt time.Time) (bool, error) {
	return true, nil
}
func (s *fakeSidekiqStore) HeartbeatSidekiqJob(ctx context.Context, id, runnerID string) (bool, error) {
	return true, nil
}
func (s *fakeSidekiqStore) UpdateSidekiqJobProgress(ctx context.Context, id, runnerID, metadata string) error {
	return nil
}
func (s *fakeSidekiqStore) CompleteSidekiqJob(ctx context.Context, id, runnerID, metadata string) error {
	return nil
}
func (s *fakeSidekiqStore) FailSidekiqJob(ctx context.Context, id, runnerID, metadata, lastErr string) error {
	return nil
}
func (s *fakeSidekiqStore) ListClaimableSidekiqJobs(ctx context.Context, staleBefore time.Time) ([]tables.TableSidekiqJob, error) {
	return nil, nil
}
func (s *fakeSidekiqStore) CancelSidekiqJob(ctx context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok || tables.IsSidekiqTerminalStatus(job.Status) {
		return false, nil
	}
	job.Status = tables.SidekiqStatusCancelled
	if s.inFlight != nil && s.inFlight.ID == id {
		s.inFlight = nil
	}
	return true, nil
}
func (s *fakeSidekiqStore) FinalizeCancelledSidekiqJob(ctx context.Context, id, runnerID, metadata string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok && job.Status == tables.SidekiqStatusCancelled && metadata != "" {
		job.Metadata = metadata
	}
	return nil
}

type dashboardLogManager struct {
	failStats              bool
	mcpLog                 *logstore.MCPToolLog
	lastLLMFilters         logstore.SearchFilters
	lastMCPFilters         logstore.MCPToolLogSearchFilters
	lastRecalculateFilters logstore.SearchFilters
	lastRecalculateContext chan context.Context

	// Timeline test fields
	log            *logstore.Log
	timelineEvents []logstore.TimelineEvent
	failGetLog     bool
	activeLogs     []*logstore.Log
	// activeLogStreamCh receives Log status updates for SSE streaming
	activeLogStreamCh chan *logstore.Log
	// sseSubscribed is set to true when an SSE subscriber connects
	sseSubscribed bool
	// sseDisconnected is set to true when the SSE subscriber disconnects
	sseDisconnected bool
}

func (m *dashboardLogManager) GetLog(ctx context.Context, id string) (*logstore.Log, error) {
	if m.failGetLog {
		return nil, errors.New("internal error")
	}
	if m.log != nil && m.log.ID == id {
		cp := *m.log
		return &cp, nil
	}
	return nil, logstore.ErrNotFound
}
func (m *dashboardLogManager) Search(ctx context.Context, filters *logstore.SearchFilters, pagination *logstore.PaginationOptions) (*logstore.SearchResult, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetSessionLogs(ctx context.Context, sessionID string, pagination *logstore.PaginationOptions) (*logstore.SessionDetailResult, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetSessionSummary(ctx context.Context, sessionID string) (*logstore.SessionSummaryResult, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetStats(ctx context.Context, filters *logstore.SearchFilters) (*logstore.SearchStats, error) {
	m.lastLLMFilters = *filters
	if m.failStats {
		return nil, errors.New("stats failed")
	}
	return &logstore.SearchStats{}, nil
}
func (m *dashboardLogManager) GetHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64) (*logstore.HistogramResult, error) {
	return &logstore.HistogramResult{}, nil
}
func (m *dashboardLogManager) GetTokenHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64) (*logstore.TokenHistogramResult, error) {
	return &logstore.TokenHistogramResult{}, nil
}
func (m *dashboardLogManager) GetCostHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64) (*logstore.CostHistogramResult, error) {
	return &logstore.CostHistogramResult{}, nil
}
func (m *dashboardLogManager) GetModelHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64) (*logstore.ModelHistogramResult, error) {
	return &logstore.ModelHistogramResult{}, nil
}
func (m *dashboardLogManager) GetLatencyHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64) (*logstore.LatencyHistogramResult, error) {
	return &logstore.LatencyHistogramResult{}, nil
}
func (m *dashboardLogManager) GetProviderCostHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64) (*logstore.ProviderCostHistogramResult, error) {
	return &logstore.ProviderCostHistogramResult{}, nil
}
func (m *dashboardLogManager) GetProviderTokenHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64) (*logstore.ProviderTokenHistogramResult, error) {
	return &logstore.ProviderTokenHistogramResult{}, nil
}
func (m *dashboardLogManager) GetProviderLatencyHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64) (*logstore.ProviderLatencyHistogramResult, error) {
	return &logstore.ProviderLatencyHistogramResult{}, nil
}
func (m *dashboardLogManager) GetThroughputHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64) (*logstore.ThroughputHistogramResult, error) {
	return &logstore.ThroughputHistogramResult{}, nil
}
func (m *dashboardLogManager) GetProviderThroughputHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64) (*logstore.ProviderThroughputHistogramResult, error) {
	return &logstore.ProviderThroughputHistogramResult{}, nil
}
func (m *dashboardLogManager) GetModelRankings(ctx context.Context, filters *logstore.SearchFilters) (*logstore.ModelRankingResult, error) {
	return &logstore.ModelRankingResult{}, nil
}
func (m *dashboardLogManager) GetDimensionRankings(ctx context.Context, filters *logstore.SearchFilters, dimension logstore.RankingDimension) (*logstore.DimensionRankingResult, error) {
	return &logstore.DimensionRankingResult{Dimension: dimension}, nil
}
func (m *dashboardLogManager) GetDroppedRequests(ctx context.Context) int64 { return 0 }
func (m *dashboardLogManager) GetAvailableModels(ctx context.Context, limit int, query string) ([]string, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableAliases(ctx context.Context, limit int, query string) ([]string, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableSelectedKeys(ctx context.Context, limit int, query string) ([]loggingplugin.KeyPair, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableVirtualKeys(ctx context.Context, limit int, query string) ([]loggingplugin.KeyPair, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableRoutingRules(ctx context.Context, limit int, query string) ([]loggingplugin.KeyPair, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableRoutingEngines(ctx context.Context, limit int, query string) ([]string, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableStopReasons(ctx context.Context, limit int, query string) ([]string, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableTeams(ctx context.Context, limit int, query string) ([]loggingplugin.KeyPair, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableCustomers(ctx context.Context, limit int, query string) ([]loggingplugin.KeyPair, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableUsers(ctx context.Context, limit int, query string) ([]loggingplugin.KeyPair, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableBusinessUnits(ctx context.Context, limit int, query string) ([]loggingplugin.KeyPair, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableMetadataKeys(ctx context.Context, limit int, query string) (map[string][]string, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetDimensionCostHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64, dimension logstore.HistogramDimension) (*logstore.DimensionCostHistogramResult, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetDimensionTokenHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64, dimension logstore.HistogramDimension) (*logstore.DimensionTokenHistogramResult, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetDimensionLatencyHistogram(ctx context.Context, filters *logstore.SearchFilters, bucketSizeSeconds int64, dimension logstore.HistogramDimension) (*logstore.DimensionLatencyHistogramResult, error) {
	return nil, nil
}
func (m *dashboardLogManager) DeleteLog(ctx context.Context, id string) error     { return nil }
func (m *dashboardLogManager) DeleteLogs(ctx context.Context, ids []string) error { return nil }
func (m *dashboardLogManager) RecalculateCosts(ctx context.Context, filters *logstore.SearchFilters, limit int) (*loggingplugin.RecalculateCostResult, error) {
	m.lastRecalculateFilters = *filters
	return &loggingplugin.RecalculateCostResult{}, nil
}
func (m *dashboardLogManager) RecalculateCostsWithProgress(ctx context.Context, filters *logstore.SearchFilters, limit int, progress func(loggingplugin.RecalculateCostProgress)) (*loggingplugin.RecalculateCostResult, error) {
	m.lastRecalculateFilters = *filters
	if m.lastRecalculateContext != nil {
		m.lastRecalculateContext <- ctx
	}
	return nil, nil
}
func (m *dashboardLogManager) BuildCostRecalcJobMeta(ctx context.Context, filters logstore.SearchFilters, missingCostOnly bool) (string, error) {
	m.lastRecalculateFilters = filters
	return "{}", nil
}
func (m *dashboardLogManager) RunCostRecalcJob(ctx context.Context, metaJSON string, checkpoint func(string) error) (string, error) {
	return metaJSON, nil
}
func (m *dashboardLogManager) GetMCPToolLog(ctx context.Context, id string) (*logstore.MCPToolLog, error) {
	if m.mcpLog == nil {
		return nil, nil
	}
	entry := *m.mcpLog
	return &entry, nil
}
func (m *dashboardLogManager) SearchMCPToolLogs(ctx context.Context, filters *logstore.MCPToolLogSearchFilters, pagination *logstore.PaginationOptions) (*logstore.MCPToolLogSearchResult, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetMCPToolLogStats(ctx context.Context, filters *logstore.MCPToolLogSearchFilters) (*logstore.MCPToolLogStats, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableToolNames(ctx context.Context, limit int, query string) ([]string, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableServerLabels(ctx context.Context, limit int, query string) ([]string, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetAvailableMCPVirtualKeys(ctx context.Context, limit int, query string) ([]loggingplugin.KeyPair, error) {
	return nil, nil
}
func (m *dashboardLogManager) GetMCPHistogram(ctx context.Context, filters logstore.MCPToolLogSearchFilters, bucketSizeSeconds int64) (*logstore.MCPHistogramResult, error) {
	m.lastMCPFilters = filters
	return &logstore.MCPHistogramResult{}, nil
}
func (m *dashboardLogManager) GetMCPCostHistogram(ctx context.Context, filters logstore.MCPToolLogSearchFilters, bucketSizeSeconds int64) (*logstore.MCPCostHistogramResult, error) {
	return &logstore.MCPCostHistogramResult{}, nil
}
func (m *dashboardLogManager) GetMCPTopTools(ctx context.Context, filters logstore.MCPToolLogSearchFilters, limit int) (*logstore.MCPTopToolsResult, error) {
	return &logstore.MCPTopToolsResult{}, nil
}

func (m *dashboardLogManager) DeleteMCPToolLogs(ctx context.Context, ids []string) error { return nil }

// staticMCPLogRedactionResolver records calls and returns a configured reveal result.
type staticMCPLogRedactionResolver struct {
	mapping *schemas.RedactionMapsByPhase
	err     error
	calls   int
}

// ResolveMCPLogRedactionMapping returns the configured test result.
func (r *staticMCPLogRedactionResolver) ResolveMCPLogRedactionMapping(_ *fasthttp.RequestCtx, _ *logstore.MCPToolLog) (*schemas.RedactionMapsByPhase, error) {
	r.calls++
	return r.mapping, r.err
}

func (m *dashboardLogManager) CreateUserAgentMapping(ctx context.Context, mapping *logstore.UserAgentMapping) (*logstore.UserAgentMapping, error) {
	return nil, nil
}

func (m *dashboardLogManager) DeleteUserAgentMapping(ctx context.Context, id string) error {
	return nil
}

func (m *dashboardLogManager) UpdateUserAgentMapping(ctx context.Context, id string, mapping *logstore.UserAgentMapping) (*logstore.UserAgentMapping, error) {
	return nil, nil
}

func (m *dashboardLogManager) ListUserAgentMappings(ctx context.Context) ([]logstore.UserAgentMapping, error) {
	return nil, nil
}

func (m *dashboardLogManager) ListTimelineEventsByLogID(ctx context.Context, logID string) ([]logstore.TimelineEvent, error) {
	if m.timelineEvents == nil {
		return nil, nil
	}
	return m.timelineEvents, nil
}

func (m *dashboardLogManager) GetActiveLogs(ctx context.Context) ([]*logstore.Log, error) {
	return m.activeLogs, nil
}

func (m *dashboardLogManager) SubscribeActiveLogStream(ctx context.Context) (<-chan *logstore.Log, error) {
	m.sseSubscribed = true
	return m.activeLogStreamCh, nil
}

func (m *dashboardLogManager) UnsubscribeActiveLogStream(ctx context.Context, ch <-chan *logstore.Log) error {
	m.sseDisconnected = true
	return nil
}

func (m *dashboardLogManager) GetAvailableUserAgents(ctx context.Context, _ int, _ string) ([]string, error) {
	return nil, nil
}

func (m *dashboardLogManager) GetAvailableApps(ctx context.Context, _ int, _ string) ([]string, error) {
	return nil, nil
}

func (m *dashboardLogManager) GetAvailableMCPApps(ctx context.Context, _ int, _ string) ([]string, error) {
	return nil, nil
}

func (m *dashboardLogManager) GetAvailableMCPUserAgents(ctx context.Context, _ int, _ string) ([]string, error) {
	return nil, nil
}

// ptrFloat64 returns a pointer to the given float64 value.
func ptrFloat64(v float64) *float64 {
	return &v
}

// ---------------------------------------------------------------------------
// Task 6.1: GetLogTimeline handler tests
// ---------------------------------------------------------------------------

// TestGetLogTimeline_LogFound verifies that GetLogTimeline returns 200 with
// the correct JSON structure when the log exists and has timeline events.
func TestGetLogTimeline_LogFound(t *testing.T) {
	SetLogger(&mockLogger{})

	now := time.Now()
	mgr := &dashboardLogManager{
		log: &logstore.Log{
			ID:                "log-test-1",
			Status:            "success",
			Provider:          "openai",
			Model:             "gpt-4",
			Latency:           ptrFloat64(1234.56),
			Timestamp:         now,
			RoutingEngineLogs: `[{"engine":"governance","level":"info","message":"provider=openai attempt=0","timestamp":1700000000000}]`,
			PluginLogs:        `{"logging":[{"plugin_name":"logging","level":"info","message":"pre-llm hook executed","timestamp":1700000000000}]}`,
			AttemptTrail:      `[{"attempt":0,"key_id":"key-1","key_name":"Key 1"}]`,
		},
		timelineEvents: []logstore.TimelineEvent{
			{
				ID:           "event-1",
				LogID:        "log-test-1",
				Phase:        "pre_llm",
				Source:       "plugin_logging",
				PluginName:   "logging",
				Level:        "info",
				Message:      "pre-llm hook executed",
				TimeOffsetMS: 0.0,
				DurationMS:   8.2,
				Timestamp:    now,
			},
			{
				ID:           "event-2",
				LogID:        "log-test-1",
				Phase:        "post_llm",
				Source:       "plugin_logging",
				PluginName:   "logging",
				Level:        "info",
				Message:      "post-llm hook executed",
				TimeOffsetMS: 1128.0,
				DurationMS:   6.5,
				Timestamp:    now.Add(1128 * time.Millisecond),
			},
		},
	}
	h := &LoggingHandler{logManager: mgr}

	var req fasthttp.Request
	req.SetRequestURI("/api/logs/log-test-1/timeline")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}, nil)
	ctx.SetUserValue("id", "log-test-1")

	// RED PHASE: getLogTimeline does not exist yet → compile error expected.
	h.getLogTimeline(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", got, string(ctx.Response.Body()))
	}

	var response struct {
		LogID           string  `json:"log_id"`
		TotalDurationMs float64 `json:"total_duration_ms"`
		Events          []struct {
			TimeOffsetMS float64 `json:"time_ms_offset"`
			DurationMS   float64 `json:"duration_ms"`
			Phase        string  `json:"phase"`
			Source       string  `json:"source"`
			Message      string  `json:"message"`
			Level        string  `json:"level"`
			PluginName   string  `json:"plugin_name"`
		} `json:"events"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.LogID != "log-test-1" {
		t.Fatalf("log_id = %q, want %q", response.LogID, "log-test-1")
	}
	if len(response.Events) == 0 {
		t.Fatal("expected non-empty events array")
	}

	// At least one event must be a timeline_events source (pre_llm or post_llm)
	hasTimelineEvent := false
	for _, e := range response.Events {
		if e.Source == "plugin_logging" {
			hasTimelineEvent = true
			break
		}
	}
	if !hasTimelineEvent {
		t.Fatal("expected at least one event with source=plugin_logging")
	}
}

// TestGetLogTimeline_LogNotFound verifies that GetLogTimeline returns 404
// with error code "log_not_found" when the log does not exist.
func TestGetLogTimeline_LogNotFound(t *testing.T) {
	SetLogger(&mockLogger{})

	mgr := &dashboardLogManager{}
	h := &LoggingHandler{logManager: mgr}

	var req fasthttp.Request
	req.SetRequestURI("/api/logs/nonexistent/timeline")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}, nil)
	ctx.SetUserValue("id", "nonexistent")

	// RED PHASE: getLogTimeline does not exist yet → compile error expected.
	h.getLogTimeline(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", got, string(ctx.Response.Body()))
	}

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != "log_not_found" {
		t.Fatalf("error code = %q, want %q", errResp.Error.Code, "log_not_found")
	}
}

// TestGetLogTimeline_EmptyEvents verifies that GetLogTimeline returns `"events":[]`
// (not null) when a log has no timeline events, routing engine logs, plugin logs,
// or attempt trail. A nil Go slice marshals to JSON null, which crashes the frontend
// TimelineDetail component (`events.length` on null).
func TestGetLogTimeline_EmptyEvents(t *testing.T) {
	SetLogger(&mockLogger{})

	now := time.Now()
	mgr := &dashboardLogManager{
		log: &logstore.Log{
			ID:                "log-empty-1",
			Status:            "error",
			Provider:          "minimax",
			Model:             "test-model",
			Latency:           ptrFloat64(5000.0),
			Timestamp:         now,
			RoutingEngineLogs: "",
			PluginLogs:        "",
			AttemptTrail:      "",
		},
		// timelineEvents is nil — no events in the timeline_events table
	}
	h := &LoggingHandler{logManager: mgr}

	var req fasthttp.Request
	req.SetRequestURI("/api/logs/log-empty-1/timeline")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}, nil)
	ctx.SetUserValue("id", "log-empty-1")

	h.getLogTimeline(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", got, string(ctx.Response.Body()))
	}

	// Unmarshal events as raw JSON to distinguish [] from null
	var raw struct {
		Events json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &raw); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(raw.Events) == 0 {
		t.Fatal("events field is null/empty in JSON — expected non-null empty array '[]'")
	}
	if string(raw.Events) != "[]" {
		t.Fatalf("events JSON = %s, want '[]' (empty array, not null)", string(raw.Events))
	}
}

// TestGetLogTimeline_InternalError verifies that GetLogTimeline returns 500
// when the log manager returns an unexpected error.
func TestGetLogTimeline_InternalError(t *testing.T) {
	SetLogger(&mockLogger{})

	mgr := &dashboardLogManager{failGetLog: true}
	h := &LoggingHandler{logManager: mgr}

	var req fasthttp.Request
	req.SetRequestURI("/api/logs/log-test-1/timeline")

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}, nil)
	ctx.SetUserValue("id", "log-test-1")

	// RED PHASE: getLogTimeline does not exist yet → compile error expected.
	h.getLogTimeline(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", got, string(ctx.Response.Body()))
	}
}

// ---------------------------------------------------------------------------
// Task 6.2: GetActiveLogStream SSE handler tests
// ---------------------------------------------------------------------------

// TestGetActiveLogStream_Handshake verifies that the SSE handler sends an
// active_logs handshake event with the full list of processing logs when a
// client connects and then pushes incremental log_updated events.
func TestGetActiveLogStream_Handshake(t *testing.T) {
	SetLogger(&mockLogger{})

	now := time.Now()
	activeLogStreamCh := make(chan *logstore.Log, 10)
	mgr := &dashboardLogManager{
		activeLogs: []*logstore.Log{
			{
				ID:        "log-active-1",
				Status:    "processing",
				Provider:  "openai",
				Model:     "gpt-4",
				Timestamp: now,
				Latency:   nil,
			},
		},
		activeLogStreamCh: activeLogStreamCh,
	}
	h := &LoggingHandler{logManager: mgr}

	// Use net.Pipe for deterministic in-process SSE streaming testing
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	// Run the handler on one end of the pipe
	go func() {
		_ = fasthttp.ServeConn(serverConn, func(ctx *fasthttp.RequestCtx) {
			h.getActiveLogStream(ctx)
		})
	}()

	// Send HTTP request
	_, err := clientConn.Write([]byte("GET /api/logs/active/stream HTTP/1.1\r\nHost: test\r\n\r\n"))
	if err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	// Read response — the body is chunked transfer-encoded.
	readChunk := func(br *bufio.Reader) ([]byte, error) {
		sizeLine, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		sizeLine = strings.TrimSpace(sizeLine)
		var chunkSize int
		if _, err := fmt.Sscanf(sizeLine, "%x", &chunkSize); err != nil {
			return nil, fmt.Errorf("parse chunk size %q: %w", sizeLine, err)
		}
		if chunkSize == 0 {
			return nil, nil
		}
		chunkData := make([]byte, chunkSize+2)
		if _, err := io.ReadFull(br, chunkData); err != nil {
			return nil, err
		}
		return chunkData[:chunkSize], nil
	}

	br := bufio.NewReader(clientConn)

	// Read and skip HTTP response headers
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read response header: %v", err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	// Read first chunk: should be the active_logs handshake
	chunk1, err := readChunk(br)
	if err != nil {
		t.Fatalf("failed to read chunk 1 (handshake): %v", err)
	}
	if !strings.Contains(string(chunk1), "event: active_logs") {
		t.Fatalf("expected 'event: active_logs' in handshake chunk, got: %q", chunk1)
	}
	if !strings.Contains(string(chunk1), "log-active-1") {
		t.Fatalf("expected active log ID 'log-active-1' in handshake chunk, got: %q", chunk1)
	}

	// Push a log_updated event through the channel
	updatedLog := &logstore.Log{
		ID:        "log-active-1",
		Status:    "success",
		Provider:  "openai",
		Model:     "gpt-4",
		Timestamp: now,
		Latency:   ptrFloat64(1234.0),
	}
	activeLogStreamCh <- updatedLog

	// Read second chunk: should be the log_updated event
	chunk2, err := readChunk(br)
	if err != nil {
		t.Fatalf("failed to read chunk 2 (log_updated): %v", err)
	}
	if !strings.Contains(string(chunk2), "event: log_updated") {
		t.Fatalf("expected 'event: log_updated' in chunk, got: %q", chunk2)
	}
	if !strings.Contains(string(chunk2), "log-active-1") {
		t.Fatalf("expected log ID in log_updated chunk, got: %q", chunk2)
	}
	if !strings.Contains(string(chunk2), "success") {
		t.Fatalf("expected 'success' in log_updated chunk, got: %q", chunk2)
	}
	if !strings.Contains(string(chunk2), "openai") {
		t.Fatalf("expected 'openai' provider in log_updated chunk, got: %q", chunk2)
	}
	if !strings.Contains(string(chunk2), "gpt-4") {
		t.Fatalf("expected 'gpt-4' model in log_updated chunk, got: %q", chunk2)
	}
	if !strings.Contains(string(chunk2), "1234") {
		t.Fatalf("expected latency 1234 in log_updated chunk, got: %q", chunk2)
	}
	if !strings.Contains(string(chunk2), now.Format(time.RFC3339Nano)) {
		t.Fatalf("expected timestamp in log_updated chunk, got: %q", chunk2)
	}

	// Close the client connection to terminate the streaming
	clientConn.Close()
}

// TestGetActiveLogStream_DisconnectCleanup verifies that when the SSE client
// disconnects, the handler cleans up the subscription and closes the stream.
func TestGetActiveLogStream_DisconnectCleanup(t *testing.T) {
	SetLogger(&mockLogger{})

	mgr := &dashboardLogManager{
		activeLogs:        []*logstore.Log{},
		activeLogStreamCh: make(chan *logstore.Log, 10),
	}
	h := &LoggingHandler{logManager: mgr}

	// Use net.Pipe for deterministic in-process testing
	serverConn, clientConn := net.Pipe()

	// Run the handler on one end of the pipe
	done := make(chan struct{})
	go func() {
		_ = fasthttp.ServeConn(serverConn, func(ctx *fasthttp.RequestCtx) {
			h.getActiveLogStream(ctx)
		})
		close(done)
	}()

	// Send HTTP request
	_, err := clientConn.Write([]byte("GET /api/logs/active/stream HTTP/1.1\r\nHost: test\r\n\r\n"))
	if err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	// Close the client connection immediately to trigger disconnect
	clientConn.Close()

	// Wait for the handler to finish
	select {
	case <-done:
		// Handler exited cleanly
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not clean up after client disconnect")
	}
}

// ---------------------------------------------------------------------------
// Task 6.2: RTK compression metadata propagation into logs-db
// ---------------------------------------------------------------------------

// newLoggingPluginStore creates an in-memory SQLite log store for testing the
// logging plugin's metadata pipeline from the transport (handlers) package.
func newLoggingPluginStore(t *testing.T) logstore.LogStore {
	t.Helper()

	dir := t.TempDir()
	store, err := logstore.NewLogStore(context.Background(), &logstore.Config{
		Enabled: true,
		Type:    logstore.LogStoreTypeSQLite,
		Config: &logstore.SQLiteConfig{
			Path: filepath.Join(dir, "logging.db"),
		},
	}, &mockLogger{})
	if err != nil {
		t.Fatalf("NewLogStore() error = %v", err)
	}
	return store
}

// TestLoggingHandler_OriginalPromptTokensInMetadata verifies that the logging
// handler's underlying log path reads BifrostContextKeyOriginalPromptTokens and
// BifrostContextKeyCompressedPromptTokens (set by the RTK compression plugin's
// PostLLMHook) and persists them into the logs-db metadata JSON as
// `original_prompt_tokens` / `compressed_prompt_tokens`.
//
// RED PHASE: the logging plugin's metadata merge does not yet read these keys,
// so the metadata map will lack them and the assertions below fail. The dev
// phase (tasks 7.3 in tasks.md) wires the keys into the log entry metadata.
func TestLoggingHandler_OriginalPromptTokensInMetadata(t *testing.T) {
	SetLogger(&mockLogger{})

	store := newLoggingPluginStore(t)
	t.Cleanup(func() { _ = store.Close(context.Background()) })

	loggingHeaders := []string{}
	plugin, err := loggingplugin.Init(context.Background(), &loggingplugin.Config{LoggingHeaders: &loggingHeaders}, &mockLogger{}, store, nil, nil)
	if err != nil {
		t.Fatalf("loggingplugin.Init() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := plugin.Cleanup(); cleanupErr != nil {
			t.Errorf("plugin.Cleanup() error = %v", cleanupErr)
		}
	})

	requestID := "req-rtk-compression"
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyRequestID, requestID)
	// Request headers and a dimension guarantee the logging plugin materializes
	// a metadata map (captureLoggingHeaders + mergeRealtimeMetadata), so the
	// assertions below fail exactly on the missing compression keys, not on a
	// nil metadata map.
	ctx.SetValue(schemas.BifrostContextKeyRequestHeaders, map[string]string{
		"x-bf-lh-tenant": "rtk-test",
	})
	ctx.SetValue(schemas.BifrostContextKeyDimensions, map[string]string{
		"region": "us-east",
	})
	// The RTK plugin's PostLLMHook sets these on the request context:
	// original token count before compression, compressed count after.
	ctx.SetValue(schemas.BifrostContextKeyOriginalPromptTokens, 2000)
	ctx.SetValue(schemas.BifrostContextKeyCompressedPromptTokens, 800)

	// Execute PreLLMHook so the plugin has pending input data for the request.
	_, _, err = plugin.PreLLMHook(ctx, &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-4o",
			Params:   &schemas.ChatParameters{},
		},
	})
	if err != nil {
		t.Fatalf("PreLLMHook() error = %v", err)
	}

	// Execute PostLLMHook with a success response. The handler's log path must
	// read the two ctx keys and merge them into the entry metadata.
	result := &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			ExtraFields: schemas.BifrostResponseExtraFields{
				RequestType:            schemas.ChatCompletionRequest,
				Provider:               schemas.OpenAI,
				OriginalModelRequested: "gpt-4o",
				ResolvedModelUsed:      "gpt-4o",
				Latency:                42,
			},
		},
	}
	_, _, err = plugin.PostLLMHook(ctx, result, nil)
	if err != nil {
		t.Fatalf("PostLLMHook() error = %v", err)
	}

	// Cleanup drains the write queue so the log row is persisted.
	if err := plugin.Cleanup(); err != nil {
		t.Fatalf("plugin.Cleanup() error = %v", err)
	}

	logEntry, err := store.FindByID(context.Background(), requestID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if logEntry.MetadataParsed == nil {
		t.Fatal("expected metadata to be persisted for the RTK-compressed request")
	}

	// original_prompt_tokens must hold the pre-compression token count.
	if got := logEntry.MetadataParsed["original_prompt_tokens"]; got == nil {
		t.Fatal("expected metadata[\"original_prompt_tokens\"] — the logging handler must read BifrostContextKeyOriginalPromptTokens into logs-db metadata")
	} else if fmt.Sprintf("%v", got) != "2000" {
		t.Fatalf("metadata original_prompt_tokens = %#v, want 2000", got)
	}

	// compressed_prompt_tokens must hold the post-compression token count.
	if got := logEntry.MetadataParsed["compressed_prompt_tokens"]; got == nil {
		t.Fatal("expected metadata[\"compressed_prompt_tokens\"] — the logging handler must read BifrostContextKeyCompressedPromptTokens into logs-db metadata")
	} else if fmt.Sprintf("%v", got) != "800" {
		t.Fatalf("metadata compressed_prompt_tokens = %#v, want 800", got)
	}
}

// TestLoggingHandler_NoCompressionKeysLeavesMetadataUnchanged verifies the
// negative contract: requests that were never compressed (no ctx keys set) must
// not gain original/compressed_prompt_tokens entries in the metadata. This
// guards against the handler inventing compression stats for plain requests.
// In the red phase metadata may be nil (no headers set → no metadata map), so
// the test tolerates nil metadata — the key assertion is only that compression
// keys are absent if metadata exists.
func TestLoggingHandler_NoCompressionKeysLeavesMetadataUnchanged(t *testing.T) {
	SetLogger(&mockLogger{})

	store := newLoggingPluginStore(t)
	t.Cleanup(func() { _ = store.Close(context.Background()) })

	loggingHeaders := []string{}
	plugin, err := loggingplugin.Init(context.Background(), &loggingplugin.Config{LoggingHeaders: &loggingHeaders}, &mockLogger{}, store, nil, nil)
	if err != nil {
		t.Fatalf("loggingplugin.Init() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := plugin.Cleanup(); cleanupErr != nil {
			t.Errorf("plugin.Cleanup() error = %v", cleanupErr)
		}
	})

	requestID := "req-no-compression"
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyRequestID, requestID)
	// Request headers and a dimension guarantee the metadata map gets
	// materialized, so the negative assertions below test the right thing:
	// the compression keys are absent.
	ctx.SetValue(schemas.BifrostContextKeyRequestHeaders, map[string]string{
		"x-bf-lh-tenant": "rtk-test",
	})
	ctx.SetValue(schemas.BifrostContextKeyDimensions, map[string]string{
		"region": "us-east",
	})
	// Deliberately do NOT set BifrostContextKeyOriginalPromptTokens /
	// BifrostContextKeyCompressedPromptTokens — this is a plain, uncompressed request.

	_, _, err = plugin.PreLLMHook(ctx, &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-4o",
			Params:   &schemas.ChatParameters{},
		},
	})
	if err != nil {
		t.Fatalf("PreLLMHook() error = %v", err)
	}

	result := &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			ExtraFields: schemas.BifrostResponseExtraFields{
				RequestType:            schemas.ChatCompletionRequest,
				Provider:               schemas.OpenAI,
				OriginalModelRequested: "gpt-4o",
				ResolvedModelUsed:      "gpt-4o",
				Latency:                19,
			},
		},
	}
	_, _, err = plugin.PostLLMHook(ctx, result, nil)
	if err != nil {
		t.Fatalf("PostLLMHook() error = %v", err)
	}

	if err := plugin.Cleanup(); err != nil {
		t.Fatalf("plugin.Cleanup() error = %v", err)
	}

	logEntry, err := store.FindByID(context.Background(), requestID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	// The dimensions set above guarantee a metadata map materializes — assert it
	// exists and, crucially, that it carries no compression keys for an
	// uncompressed request.
	if logEntry.MetadataParsed == nil {
		t.Fatal("expected metadata to be persisted for the plain request (dimensions set)")
	}
	if _, ok := logEntry.MetadataParsed["original_prompt_tokens"]; ok {
		t.Fatal("metadata must not contain original_prompt_tokens for an uncompressed request")
	}
	if _, ok := logEntry.MetadataParsed["compressed_prompt_tokens"]; ok {
		t.Fatal("metadata must not contain compressed_prompt_tokens for an uncompressed request")
	}
}

// TestLoggingHandler_OriginalPromptTokensInStreamingFinalChunk verifies that the
// streaming final-chunk path also persists the RTK compression metadata. A
// streaming request's PostLLMHook is invoked per chunk; non-final chunks are
// processed by the accumulator and skipped, while the final chunk (marked by
// BifrostContextKeyStreamEndIndicator) reaches the metadata merge that reads
// BifrostContextKeyOriginalPromptTokens / BifrostContextKeyCompressedPromptTokens.
func TestLoggingHandler_OriginalPromptTokensInStreamingFinalChunk(t *testing.T) {
	SetLogger(&mockLogger{})

	store := newLoggingPluginStore(t)
	t.Cleanup(func() { _ = store.Close(context.Background()) })

	loggingHeaders := []string{}
	plugin, err := loggingplugin.Init(context.Background(), &loggingplugin.Config{LoggingHeaders: &loggingHeaders}, &mockLogger{}, store, nil, nil)
	if err != nil {
		t.Fatalf("loggingplugin.Init() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := plugin.Cleanup(); cleanupErr != nil {
			t.Errorf("plugin.Cleanup() error = %v", cleanupErr)
		}
	})

	requestID := "req-rtk-streaming-final"
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyRequestID, requestID)
	// Materialize the metadata map so the assertions below fail exactly on the
	// missing compression keys, not on a nil metadata map.
	ctx.SetValue(schemas.BifrostContextKeyRequestHeaders, map[string]string{
		"x-bf-lh-tenant": "rtk-streaming-test",
	})
	ctx.SetValue(schemas.BifrostContextKeyDimensions, map[string]string{
		"region": "us-west",
	})
	// The RTK plugin's PostLLMHook sets these on the request context.
	ctx.SetValue(schemas.BifrostContextKeyOriginalPromptTokens, 5000)
	ctx.SetValue(schemas.BifrostContextKeyCompressedPromptTokens, 1500)
	// Mark this as the streaming final chunk so the PostLLMHook writes the
	// full log entry instead of skipping on the accumulator path.
	ctx.SetValue(schemas.BifrostContextKeyStreamEndIndicator, true)

	_, _, err = plugin.PreLLMHook(ctx, &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionStreamRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-4o",
			Params:   &schemas.ChatParameters{},
		},
	})
	if err != nil {
		t.Fatalf("PreLLMHook() error = %v", err)
	}

	result := &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			ExtraFields: schemas.BifrostResponseExtraFields{
				RequestType:            schemas.ChatCompletionStreamRequest,
				Provider:               schemas.OpenAI,
				OriginalModelRequested: "gpt-4o",
				ResolvedModelUsed:      "gpt-4o",
				Latency:                88,
			},
		},
	}
	_, _, err = plugin.PostLLMHook(ctx, result, nil)
	if err != nil {
		t.Fatalf("PostLLMHook() error = %v", err)
	}

	if err := plugin.Cleanup(); err != nil {
		t.Fatalf("plugin.Cleanup() error = %v", err)
	}

	logEntry, err := store.FindByID(context.Background(), requestID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if logEntry.MetadataParsed == nil {
		t.Fatal("expected metadata to be persisted for the streaming final chunk")
	}

	if got := logEntry.MetadataParsed["original_prompt_tokens"]; got == nil {
		t.Fatal("expected metadata[\"original_prompt_tokens\"] in streaming final chunk — the logging handler must read BifrostContextKeyOriginalPromptTokens into logs-db metadata")
	} else if fmt.Sprintf("%v", got) != "5000" {
		t.Fatalf("metadata original_prompt_tokens = %#v, want 5000", got)
	}

	if got := logEntry.MetadataParsed["compressed_prompt_tokens"]; got == nil {
		t.Fatal("expected metadata[\"compressed_prompt_tokens\"] in streaming final chunk — the logging handler must read BifrostContextKeyCompressedPromptTokens into logs-db metadata")
	} else if fmt.Sprintf("%v", got) != "1500" {
		t.Fatalf("metadata compressed_prompt_tokens = %#v, want 1500", got)
	}
}

// TestLoggingHandler_RTKObservabilityInMetadata verifies that the logging
// handler's metadata merge reads the RTK observability keys
// (BifrostContextKeyRTKTechniques, BifrostContextKeyRTKFilterMatched,
// BifrostContextKeyRTKCompressionRatio, BifrostContextKeyRTKSnapshotMode,
// BifrostContextKeyRTKRawOutputID, and the pre-compression snapshot
// payload) and persists them into the logs-db metadata JSON. These are
// the keys that drive the "RTK Compression" tab and the metadata badges
// in the log detail view.
func TestLoggingHandler_RTKObservabilityInMetadata(t *testing.T) {
	SetLogger(&mockLogger{})

	store := newLoggingPluginStore(t)
	t.Cleanup(func() { _ = store.Close(context.Background()) })

	loggingHeaders := []string{}
	plugin, err := loggingplugin.Init(context.Background(), &loggingplugin.Config{LoggingHeaders: &loggingHeaders}, &mockLogger{}, store, nil, nil)
	if err != nil {
		t.Fatalf("loggingplugin.Init() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := plugin.Cleanup(); cleanupErr != nil {
			t.Errorf("plugin.Cleanup() error = %v", cleanupErr)
		}
	})

	requestID := "req-rtk-observability"
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyRequestID, requestID)
	ctx.SetValue(schemas.BifrostContextKeyRequestHeaders, map[string]string{
		"x-bf-lh-tenant": "rtk-obs-test",
	})

	// RTK PostLLMHook sets all of these on the context.
	ctx.SetValue(schemas.BifrostContextKeyRTKTechniques, []string{"dedup", "linefilter"})
	ctx.SetValue(schemas.BifrostContextKeyRTKFilterMatched, "git-status")
	ctx.SetValue(schemas.BifrostContextKeyRTKCompressionRatio, 0.42)
	ctx.SetValue(schemas.BifrostContextKeyRTKRawOutputID, "abcdef0123456789abcdef01")
	ctx.SetValue(schemas.BifrostContextKeyRTKSnapshotMode, "split")
	ctx.SetValue(schemas.BifrostContextKeyRTKOriginalSnapshot, json.RawMessage(`{"mode":"split","items":[{"index":0,"role":"tool","content":"original"}]}`))

	_, _, err = plugin.PreLLMHook(ctx, &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-4o",
			Params:   &schemas.ChatParameters{},
		},
	})
	if err != nil {
		t.Fatalf("PreLLMHook() error = %v", err)
	}

	result := &schemas.BifrostResponse{
		ChatResponse: &schemas.BifrostChatResponse{
			ExtraFields: schemas.BifrostResponseExtraFields{
				RequestType:            schemas.ChatCompletionRequest,
				Provider:               schemas.OpenAI,
				OriginalModelRequested: "gpt-4o",
				ResolvedModelUsed:      "gpt-4o",
				Latency:                42,
			},
		},
	}
	_, _, err = plugin.PostLLMHook(ctx, result, nil)
	if err != nil {
		t.Fatalf("PostLLMHook() error = %v", err)
	}
	if err := plugin.Cleanup(); err != nil {
		t.Fatalf("plugin.Cleanup() error = %v", err)
	}

	logEntry, err := store.FindByID(context.Background(), requestID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if logEntry.MetadataParsed == nil {
		t.Fatal("expected metadata to be persisted for the RTK-observed request")
	}

	if got := logEntry.MetadataParsed["rtk_techniques"]; got == nil {
		t.Error("metadata rtk_techniques missing")
	} else {
		arr, ok := got.([]interface{})
		if !ok || len(arr) != 2 || arr[0] != "dedup" || arr[1] != "linefilter" {
			t.Errorf("metadata rtk_techniques = %#v, want [dedup linefilter]", got)
		}
	}
	if got, want := logEntry.MetadataParsed["rtk_filter_matched"], "git-status"; got != want {
		t.Errorf("metadata rtk_filter_matched = %v, want %s", got, want)
	}
	// Ratio is stored as float64; tolerate a small epsilon.
	if got, ok := logEntry.MetadataParsed["rtk_compression_ratio"].(float64); !ok || got < 0.41 || got > 0.43 {
		t.Errorf("metadata rtk_compression_ratio = %v, want ~0.42", logEntry.MetadataParsed["rtk_compression_ratio"])
	}
	if got, want := logEntry.MetadataParsed["rtk_raw_output_id"], "abcdef0123456789abcdef01"; got != want {
		t.Errorf("metadata rtk_raw_output_id = %v, want %s", got, want)
	}
	if got, want := logEntry.MetadataParsed["rtk_snapshot_mode"], "split"; got != want {
		t.Errorf("metadata rtk_snapshot_mode = %v, want %s", got, want)
	}
	if got := logEntry.MetadataParsed["rtk_original_snapshot"]; got == nil {
		t.Error("metadata rtk_original_snapshot missing")
	}
	if got, present := logEntry.MetadataParsed["rtk_compressed_snapshot"]; present {
		t.Errorf("metadata rtk_compressed_snapshot should not be persisted, got %v", got)
	}
}

package handlers

// Red-phase 契约（dev phase 需按此实现生产代码 handlers/logs.go，测试才会编译并通过）：
//
//   - type RecentRoutingRule struct {
//         ID         string    `json:"id"`
//         Name       string    `json:"name"`
//         LastUsedAt time.Time `json:"last_used_at"`
//         UseCount   int64     `json:"use_count"`
//     }
//   - type RecentRoutingRulesStore interface {
//         RecentRoutingRules(ctx context.Context, limit int) ([]RecentRoutingRule, error)
//     }
//   - func (h *LoggingHandler) SetRecentRoutingRulesStore(store RecentRoutingRulesStore)
//   - func (h *LoggingHandler) recentRoutingRules(ctx *fasthttp.RequestCtx)   // GET /api/logs/recent-routing-rules
//
//   - GET /api/logs/recent-routing-rules 语义（来自 design.md）：
//       * 请求参数 limit（可选 int，范围 [1,1000]，缺省 100）。
//       * 存储契约：单 SQL 查询 — SELECT routing_rule_id, routing_rule_name, MAX(timestamp),
//         COUNT(*) ... WHERE routing_rule_id IS NOT NULL GROUP BY routing_rule_id
//         ORDER BY last_used_at DESC LIMIT N（logs.db 行中 routing_rule_id 为 NULL/空串不参与聚合）。
//       * limit 越界 → 400 + 错误码 INVALID_LIMIT；存储查询失败 → 500 + 错误码 LOGS_QUERY_FAILED。
//
//   - 本文件中的 fakeRecentRoutingRulesStore 用纯内存实现上述 SQL 存储契约（去重 / 计数 /
//     MAX(timestamp) / 倒序 / LIMIT），让单测能在不依赖 logs.db 的情况下验证聚合语义。
//
//   - 各行断言前均有对应输入，保证输入-断言自洽（见 TDD 红 phase 自检清单）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// fakeRecentRoutingRuleRow simulates one raw row of the logs table.
type fakeRecentRoutingRuleRow struct {
	ruleID   string
	ruleName string
	ts       time.Time
}

// fakeRecentRoutingRulesStore implements RecentRoutingRulesStore with the same
// contract as the design's single SQL query (group / dedup / count / order / limit).
type fakeRecentRoutingRulesStore struct {
	rows      []fakeRecentRoutingRuleRow
	err       error
	calls     int
	lastLimit int
}

func (s *fakeRecentRoutingRulesStore) RecentRoutingRules(ctx context.Context, limit int) ([]RecentRoutingRule, error) {
	s.calls++
	s.lastLimit = limit
	if s.err != nil {
		return nil, s.err
	}

	type agg struct {
		id    string
		name  string
		ts    time.Time
		count int64
	}
	byID := map[string]*agg{}
	for _, row := range s.rows {
		// WHERE routing_rule_id IS NOT NULL AND routing_rule_id != ''
		if row.ruleID == "" {
			continue
		}
		a, ok := byID[row.ruleID]
		if !ok {
			a = &agg{id: row.ruleID, name: row.ruleName, ts: row.ts}
			byID[row.ruleID] = a
		}
		a.count++
		if row.ts.After(a.ts) {
			a.ts = row.ts
			a.name = row.ruleName
		}
	}

	out := make([]RecentRoutingRule, 0, len(byID))
	for _, a := range byID {
		out = append(out, RecentRoutingRule{ID: a.id, Name: a.name, LastUsedAt: a.ts, UseCount: a.count})
	}
	// ORDER BY last_used_at DESC LIMIT N
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsedAt.After(out[j].LastUsedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// runRecentRoutingRulesRequest builds a GET /api/logs/recent-routing-rules
// request and invokes the handler directly on a LoggingHandler wired with the
// given store.
func runRecentRoutingRulesRequest(t *testing.T, store RecentRoutingRulesStore, query string) *fasthttp.RequestCtx {
	t.Helper()
	h := &LoggingHandler{}
	h.SetRecentRoutingRulesStore(store)

	uri := "/api/logs/recent-routing-rules"
	if query != "" {
		uri += "?" + query
	}
	var req fasthttp.Request
	req.SetRequestURI(uri)
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}, nil)

	h.recentRoutingRules(ctx)
	return ctx
}

// decodeRecentRoutingRulesResponse decodes the 200 payload { "rules": [...] }.
func decodeRecentRoutingRulesResponse(t *testing.T, body []byte) []RecentRoutingRule {
	t.Helper()
	var resp struct {
		Rules []RecentRoutingRule `json:"rules"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode recent-routing-rules response: %v (body=%s)", err, body)
	}
	return resp.Rules
}

// decodeErrorResponse extracts the error.code / error.message from a non-200 body.
func decodeErrorResponse(t *testing.T, body []byte) (code string, message string) {
	t.Helper()
	var resp struct {
		Error *struct {
			Code    *string `json:"code"`
			Message string  `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode error response: %v (body=%s)", err, body)
	}
	if resp.Error == nil {
		t.Fatalf("expected error object in body: %s", body)
	}
	if resp.Error.Code != nil {
		code = *resp.Error.Code
	}
	return code, resp.Error.Message
}

// makeAggregationRows builds exactly 100 raw log rows spread across 5 distinct
// routing_rule_ids: rr-a(50) rr-b(25) rr-c(15) rr-d(5) rr-e(5) = 100 rows, so
// the aggregated result must contain exactly 5 deduplicated rules.
func makeAggregationRows() []fakeRecentRoutingRuleRow {
	base := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	rows := make([]fakeRecentRoutingRuleRow, 0, 100)
	add := func(id, name string, n int, at time.Time) {
		for i := 0; i < n; i++ {
			rows = append(rows, fakeRecentRoutingRuleRow{ruleID: id, ruleName: name, ts: at.Add(time.Duration(i) * time.Millisecond)})
		}
	}
	add("rr-a", "pg-master", 50, base.Add(50*time.Minute))      // 50 行, 最新
	add("rr-b", "hermes-default", 25, base.Add(40*time.Minute)) // 25 行
	add("rr-c", "data-shard", 15, base.Add(30*time.Minute))     // 15 行
	add("rr-d", "edge-token", 5, base.Add(20*time.Minute))      // 5 行
	add("rr-e", "batch-import", 5, base.Add(10*time.Minute))    // 5 行, 最旧
	return rows
}

// TestRecentRoutingRulesAggregation covers the normal path: 100 logs across 5
// distinct routing_rule_ids must deduplicate into exactly 5 rules ordered by
// last_used_at desc, with per-rule use_count matching the row distribution.
func TestRecentRoutingRulesAggregation(t *testing.T) {
	SetLogger(&mockLogger{})
	store := &fakeRecentRoutingRulesStore{rows: makeAggregationRows()}

	ctx := runRecentRoutingRulesRequest(t, store, "limit=100")

	status := ctx.Response.StatusCode()
	if status != fasthttp.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, ctx.Response.Body())
	}

	rules := decodeRecentRoutingRulesResponse(t, ctx.Response.Body())

	// 输入：5 个不同 routing_rule_id（50/25/15/5/5 = 100 行）→ 断言输出：去重后恰好 5 条。
	if len(rules) != 5 {
		t.Fatalf("rules len = %d, want 5 (dedup of 100 rows across 5 ids)", len(rules))
	}

	// 输入：rr-a 拥有最新的 MAX(timestamp) → 断言输出：按 last_used_at 倒序后第一条是 rr-a。
	if rules[0].ID != "rr-a" {
		t.Fatalf("rules[0].id = %s, want rr-a (latest last_used_at sorts first)", rules[0].ID)
	}
	if rules[len(rules)-1].ID != "rr-e" {
		t.Fatalf("rules[last].id = %s, want rr-e (oldest last_used_at sorts last)", rules[len(rules)-1].ID)
	}
	for i := 1; i < len(rules); i++ {
		if rules[i-1].LastUsedAt.Before(rules[i].LastUsedAt) {
			t.Fatalf("rules not ordered by last_used_at desc: %s(%s) < %s(%s)",
				rules[i-1].ID, rules[i-1].LastUsedAt, rules[i].ID, rules[i].LastUsedAt)
		}
	}

	// 输入：每个 id 的行数 {rr-a:50, rr-b:25, rr-c:15, rr-d:5, rr-e:5} → 断言输出：use_count 精确匹配。
	counts := map[string]int64{}
	var total int64
	for _, r := range rules {
		counts[r.ID] = r.UseCount
		total += r.UseCount
	}
	want := map[string]int64{"rr-a": 50, "rr-b": 25, "rr-c": 15, "rr-d": 5, "rr-e": 5}
	for id, c := range want {
		if counts[id] != c {
			t.Fatalf("use_count[%s] = %d, want %d", id, counts[id], c)
		}
	}
	if total != 100 {
		t.Fatalf("sum of use_count = %d, want 100 (all rows aggregated once)", total)
	}
	if store.calls != 1 {
		t.Fatalf("store calls = %d, want 1", store.calls)
	}
	if store.lastLimit != 100 {
		t.Fatalf("store limit = %d, want 100 (passed through from query)", store.lastLimit)
	}
}

// TestRecentRoutingRulesAllNullRules covers the empty path: every one of the
// 100 log rows has a NULL/empty routing_rule_id, so the aggregation returns no
// rules and the endpoint responds 200 with an empty rules array.
func TestRecentRoutingRulesAllNullRules(t *testing.T) {
	SetLogger(&mockLogger{})
	base := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	rows := make([]fakeRecentRoutingRuleRow, 100)
	for i := range rows {
		rows[i] = fakeRecentRoutingRuleRow{ruleID: "", ruleName: "", ts: base.Add(time.Duration(i) * time.Minute)}
	}
	store := &fakeRecentRoutingRulesStore{rows: rows}

	ctx := runRecentRoutingRulesRequest(t, store, "")

	status := ctx.Response.StatusCode()
	if status != fasthttp.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, ctx.Response.Body())
	}

	rules := decodeRecentRoutingRulesResponse(t, ctx.Response.Body())

	// 输入：100 行全为 routing_rule_id 空 → 断言输出：空 rules 数组。
	if len(rules) != 0 {
		t.Fatalf("rules len = %d, want 0 (all routing_rule_id null)", len(rules))
	}
}

// TestRecentRoutingRulesInvalidLimit covers the validation path: limit outside
// [1, 1000] (or unparseable) yields 400 + INVALID_LIMIT and the store is never
// queried; boundary values 1 and 1000 are accepted.
func TestRecentRoutingRulesInvalidLimit(t *testing.T) {
	SetLogger(&mockLogger{})

	for _, tt := range []struct {
		name       string
		query      string
		wantStatus int
		wantCode   string
	}{
		{name: "limit zero", query: "limit=0", wantStatus: fasthttp.StatusBadRequest, wantCode: "INVALID_LIMIT"},
		{name: "limit negative", query: "limit=-1", wantStatus: fasthttp.StatusBadRequest, wantCode: "INVALID_LIMIT"},
		{name: "limit exceeds max", query: "limit=99999", wantStatus: fasthttp.StatusBadRequest, wantCode: "INVALID_LIMIT"},
		{name: "limit not a number", query: "limit=abc", wantStatus: fasthttp.StatusBadRequest}, // 只断言 400，不做错误码约束
		{name: "limit min boundary", query: "limit=1", wantStatus: fasthttp.StatusOK},
		{name: "limit max boundary", query: "limit=1000", wantStatus: fasthttp.StatusOK},
		{name: "limit absent", query: "", wantStatus: fasthttp.StatusOK}, // 缺省 100
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeRecentRoutingRulesStore{rows: makeAggregationRows()}
			ctx := runRecentRoutingRulesRequest(t, store, tt.query)

			if status := ctx.Response.StatusCode(); status != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", status, tt.wantStatus, ctx.Response.Body())
			}

			if tt.wantStatus == fasthttp.StatusBadRequest {
				if store.calls != 0 {
					t.Fatalf("store must not be queried on invalid limit, got %d calls", store.calls)
				}
				code, message := decodeErrorResponse(t, ctx.Response.Body())
				if tt.wantCode != "" && code != tt.wantCode {
					t.Fatalf("error code = %q, want %q (message=%q)", code, tt.wantCode, message)
				}
				if !containsStr(message, "limit") {
					t.Fatalf("error message %q should mention limit", message)
				}
			}
		})
	}
}

// TestRecentRoutingRulesStoreError covers the 500 path (design contract
// LOGS_QUERY_FAILED): a store query failure surfaces as 500 with the error code
// set, so the UI can distinguish it from the 200 empty-list degradation.
func TestRecentRoutingRulesStoreError(t *testing.T) {
	SetLogger(&mockLogger{})
	store := &fakeRecentRoutingRulesStore{err: errors.New("logs.db query failed")}

	ctx := runRecentRoutingRulesRequest(t, store, "limit=10")

	status := ctx.Response.StatusCode()
	if status != fasthttp.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", status, ctx.Response.Body())
	}
	code, _ := decodeErrorResponse(t, ctx.Response.Body())
	if code != "LOGS_QUERY_FAILED" {
		t.Fatalf("error code = %q, want LOGS_QUERY_FAILED", code)
	}
}

// containsStr reports whether s contains the substring sub.
func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// recentRoutingRulesSQLiteLog mirrors the logs-table columns the
// recentRoutingRulesGormStore query touches, with production column types:
// timestamp as datetime not null, routing_rule_id / routing_rule_name as
// nullable varchar.
type recentRoutingRulesSQLiteLog struct {
	ID              string    `gorm:"primaryKey;type:varchar(255)"`
	Timestamp       time.Time `gorm:"not null"`
	RoutingRuleID   *string   `gorm:"type:varchar(255)"`
	RoutingRuleName *string   `gorm:"type:varchar(255)"`
}

// TableName pins the struct onto the real "logs" table name the store queries.
func (recentRoutingRulesSQLiteLog) TableName() string { return "logs" }

// TestRecentRoutingRulesGormStoreSQLite is the regression test for the
// V-transports-2 bug. On sqlite the gorm driver stores Log.Timestamp as ISO
// text, so MAX(timestamp) scans back as a string and a plain time.Time scan
// field fails with "unsupported Scan, storing driver.Value type string into
// type *time.Time" — surfacing as 500 LOGS_QUERY_FAILED. It drives the real
// recentRoutingRulesGormStore end-to-end against an in-memory sqlite DB (the
// fake store used elsewhere returns time.Time directly and cannot catch this).
func TestRecentRoutingRulesGormStoreSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&recentRoutingRulesSQLiteLog{}); err != nil {
		t.Fatalf("migrate logs table: %v", err)
	}

	// Seed 100 rows across 5 routing rules (same distribution as
	// makeAggregationRows): rr-a 50 rows (latest MAX), rr-b 25, rr-c 15,
	// rr-d 5, rr-e 5 (oldest MAX).
	base := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	type seed struct {
		id    string
		name  string
		n     int
		start time.Time
	}
	seeds := []seed{
		{id: "rr-a", name: "pg-master", n: 50, start: base.Add(50 * time.Minute)},
		{id: "rr-b", name: "hermes-default", n: 25, start: base.Add(40 * time.Minute)},
		{id: "rr-c", name: "data-shard", n: 15, start: base.Add(30 * time.Minute)},
		{id: "rr-d", name: "edge-token", n: 5, start: base.Add(20 * time.Minute)},
		{id: "rr-e", name: "batch-import", n: 5, start: base.Add(10 * time.Minute)},
	}
	serial := 0
	for _, sd := range seeds {
		for i := 0; i < sd.n; i++ {
			ruleID, ruleName := sd.id, sd.name
			row := recentRoutingRulesSQLiteLog{
				ID:              fmt.Sprintf("log-%03d", serial),
				Timestamp:       sd.start.Add(time.Duration(i) * time.Millisecond),
				RoutingRuleID:   &ruleID,
				RoutingRuleName: &ruleName,
			}
			if err := db.Create(&row).Error; err != nil {
				t.Fatalf("insert log row: %v", err)
			}
			serial++
		}
	}

	store := NewRecentRoutingRulesGormStore(db)

	// 输入：100 行 / 5 个 routing_rule_id → 输出：恰好 5 条聚合 rule，
	// 无 Scan 错误（修复前此处返回 500 LOGS_QUERY_FAILED）。
	rules, err := store.RecentRoutingRules(context.Background(), 100)
	if err != nil {
		t.Fatalf("RecentRoutingRules err (was 500 LOGS_QUERY_FAILED before fix): %v", err)
	}
	if len(rules) != 5 {
		t.Fatalf("rules len = %d, want 5 (dedup of 100 rows across 5 ids)", len(rules))
	}
	if rules[0].ID != "rr-a" {
		t.Fatalf("rules[0].id = %s, want rr-a (latest last_used_at sorts first)", rules[0].ID)
	}
	if rules[len(rules)-1].ID != "rr-e" {
		t.Fatalf("rules[last].id = %s, want rr-e (oldest last_used_at sorts last)", rules[len(rules)-1].ID)
	}

	// 输入：每 rule 最新行的时间戳 → 断言输出：MAX(timestamp) 精确等于该值
	// （证明 sqlite 字符串已按 ISO 布局成功解析）。
	wantLast := map[string]time.Time{
		"rr-a": base.Add(50*time.Minute + 49*time.Millisecond),
		"rr-b": base.Add(40*time.Minute + 24*time.Millisecond),
		"rr-c": base.Add(30*time.Minute + 14*time.Millisecond),
		"rr-d": base.Add(20*time.Minute + 4*time.Millisecond),
		"rr-e": base.Add(10*time.Minute + 4*time.Millisecond),
	}
	counts := map[string]int64{}
	for _, r := range rules {
		counts[r.ID] = r.UseCount
		want, ok := wantLast[r.ID]
		if !ok {
			t.Fatalf("unexpected rule id %s", r.ID)
		}
		if !r.LastUsedAt.Equal(want) {
			t.Fatalf("last_used_at[%s] = %s, want %s (MAX(timestamp) parsed from sqlite text)",
				r.ID, r.LastUsedAt.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
		}
	}
	// 输入：每 id 行数 {50,25,15,5,5} → 断言输出：use_count 精确匹配。
	for id, want := range map[string]int64{"rr-a": 50, "rr-b": 25, "rr-c": 15, "rr-d": 5, "rr-e": 5} {
		if counts[id] != want {
			t.Fatalf("use_count[%s] = %d, want %d", id, counts[id], want)
		}
	}

	// ORDER BY + LIMIT 透传：limit=2 返回最新的两条 rr-a / rr-b。
	limited, err := store.RecentRoutingRules(context.Background(), 2)
	if err != nil {
		t.Fatalf("RecentRoutingRules(limit=2): %v", err)
	}
	if len(limited) != 2 || limited[0].ID != "rr-a" || limited[1].ID != "rr-b" {
		t.Fatalf("limit=2 rules = %+v, want [rr-a rr-b]", limited)
	}

	// 空结果路径：全部 routing_rule_id 为空 → 返回空数组而非错误。
	if err := db.Exec(`DELETE FROM logs`).Error; err != nil {
		t.Fatalf("truncate logs: %v", err)
	}
	for i := 0; i < 3; i++ {
		row := recentRoutingRulesSQLiteLog{ID: fmt.Sprintf("null-%d", i), Timestamp: base.Add(time.Duration(i) * time.Minute)}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("insert null-rule log row: %v", err)
		}
	}
	empty, err := store.RecentRoutingRules(context.Background(), 100)
	if err != nil {
		t.Fatalf("RecentRoutingRules (all null rules): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("all-null rules len = %d, want 0", len(empty))
	}
}

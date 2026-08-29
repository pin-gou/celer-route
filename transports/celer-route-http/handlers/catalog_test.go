package handlers

// Red-phase 契约（dev phase 需按此实现生产代码 handlers/catalog.go，测试才会编译并通过）：
//
//   - type CatalogHandler struct
//   - func NewCatalogHandler(config *lib.Config) *CatalogHandler
//   - func (h *CatalogHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware)
//   - func (h *CatalogHandler) getCatalogBundles(ctx *fasthttp.RequestCtx)   // GET /api/catalog/bundles
//
//   - lib.Config 新增字段 RemoteCatalog *lib.RemoteCatalogConfig（json "remote_catalog"），
//     其中 URLTemplate string（json "url_template"）用于拼接上游 URL：将模板中的 "{lang}"
//     占位符替换为请求的 lang（如 "https://cdn.example.com/bundles/{lang}.json"）。
//
//   - GET /api/catalog/bundles 语义（来自 design.md）：
//       * 请求参数 lang（可选，缺省回退 en），请求头 If-None-Match（可选）。
//       * 内存快照缓存 miss 时惰性拉取上游（超时 5s），校验 JSON 后按快照 hash 派生 ETag 并缓存；
//         任意上游 HTTP 错误 / 解析失败一律降级为 200 + 空 bundles，绝不返回 5xx。
//       * 客户端 If-None-Match 与当前 ETag 相等时返回 304（无 body，仅 ETag 头）。
//
//   - 各行断言前均有对应输入，保证输入-断言自洽（见 TDD 红 phase 自检清单）。

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// catalogBundlesResponse mirrors the wire shape of GET /api/catalog/bundles.
type catalogBundlesResponse struct {
	Bundles []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Providers   []struct {
			Provider     string   `json:"provider"`
			Models       []string `json:"models"`
			ApplyURL     string   `json:"apply_url"`
			ApplySteps   []string `json:"apply_steps"`
			IsKeyless    bool     `json:"is_keyless"`
			Notes        string   `json:"notes"`
			BaseProvider string   `json:"base_provider"`
			BaseURL      string   `json:"base_url"`
			Supported    bool     `json:"supported"`
		} `json:"providers"`
	} `json:"bundles"`
	UpdatedAt *string `json:"updated_at"`
	Version   *string `json:"version"`
}

// runCatalogBundlesRequest builds a fasthttp request for GET /api/catalog/bundles
// and invokes the handler directly (matching the existing handler-test pattern).
func runCatalogBundlesRequest(t *testing.T, h *CatalogHandler, query string, ifNoneMatch string) *fasthttp.RequestCtx {
	t.Helper()
	uri := "/api/catalog/bundles"
	if query != "" {
		uri += "?" + query
	}
	var req fasthttp.Request
	req.SetRequestURI(uri)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}, nil)
	h.getCatalogBundles(ctx)
	return ctx
}

// newCatalogHandlerForTest wires a CatalogHandler over an httptest upstream
// server. The returned handler's remote_catalog URLTemplate points at the mock.
func newCatalogHandlerForTest(t *testing.T, upstream http.Handler) (*CatalogHandler, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(upstream)
	t.Cleanup(srv.Close)
	cfg := &lib.Config{
		RemoteCatalog: &lib.RemoteCatalogConfig{
			URLTemplate: srv.URL + "/bundles/{lang}.json",
		},
	}
	return NewCatalogHandler(cfg), srv
}

// TestCatalogBundlesSuccess covers the happy path: the upstream mock returns a
// valid bundles JSON and the endpoint proxies it with 200 + an ETag header.
func TestCatalogBundlesSuccess(t *testing.T) {
	SetLogger(&mockLogger{})
	mux := http.NewServeMux()
	mux.HandleFunc("/bundles/zh-CN.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"version": "2026-08-28",
			"updated_at": "2026-08-28T08:00:00Z",
			"bundles": [
				{
					"id": "coding",
					"title": "编程开发",
					"description": "代码补全与调试首选",
					"providers": [
						{
							"provider": "openai",
							"models": ["gpt-4o-mini", "gpt-4.1"],
							"apply_url": "https://platform.openai.com/signup",
							"apply_steps": ["注册账号", "申请 API Key"],
							"is_keyless": false,
							"notes": "新用户首月 $5 免费额度"
						}
					]
				},
				{
					"id": "opencode",
					"title": "免 Key 直达",
					"description": "Opencode 内置",
					"providers": [
						{
							"provider": "opencode",
							"models": ["default"],
							"apply_url": "",
							"apply_steps": [],
							"is_keyless": true,
							"notes": "免 Key，直接添加"
						}
					]
				}
			]
		}`))
	})
	h, _ := newCatalogHandlerForTest(t, mux)

	ctx := runCatalogBundlesRequest(t, h, "lang=zh-CN", "")

	// 契约：成功路径永远 200，绝不 5xx。
	status := ctx.Response.StatusCode()
	if status != fasthttp.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, ctx.Response.Body())
	}
	if status >= fasthttp.StatusInternalServerError {
		t.Fatalf("endpoint must never return 5xx, got %d", status)
	}

	var resp catalogBundlesResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("decode bundles response: %v (body=%s)", err, ctx.Response.Body())
	}

	// 输入（mock 返回 2 个 bundle）→ 断言输出（len==2 且 id 一致）。
	if len(resp.Bundles) != 2 {
		t.Fatalf("bundles len = %d, want 2", len(resp.Bundles))
	}
	if resp.Bundles[0].ID != "coding" || resp.Bundles[1].ID != "opencode" {
		t.Fatalf("bundle ids = [%s %s], want [coding opencode]", resp.Bundles[0].ID, resp.Bundles[1].ID)
	}
	if resp.Bundles[0].Providers[0].Provider != "openai" || resp.Bundles[0].Providers[0].IsKeyless {
		t.Fatalf("bundle[0] provider = %+v, want openai with is_keyless=false", resp.Bundles[0].Providers[0])
	}
	if !resp.Bundles[1].Providers[0].IsKeyless {
		t.Fatalf("bundle[1] provider = %+v, want is_keyless=true for opencode", resp.Bundles[1].Providers[0])
	}
	if resp.Version == nil || *resp.Version != "2026-08-28" {
		t.Fatalf("version = %v, want 2026-08-28", resp.Version)
	}
	if resp.UpdatedAt == nil || *resp.UpdatedAt != "2026-08-28T08:00:00Z" {
		t.Fatalf("updated_at = %v, want 2026-08-28T08:00:00Z", resp.UpdatedAt)
	}

	// 契约：200 响应必须携带 ETag 头，供客户端后续 If-None-Match 协商。
	if etag := string(ctx.Response.Header.Peek("ETag")); etag == "" {
		t.Fatal("expected non-empty ETag header on 200 response")
	}
}

// TestCatalogBundlesEmpty covers the empty-bundles path: the upstream mock
// returns a valid payload with an empty bundle array; the endpoint still
// responds 200 (never 5xx) with zero bundles.
func TestCatalogBundlesEmpty(t *testing.T) {
	SetLogger(&mockLogger{})
	mux := http.NewServeMux()
	mux.HandleFunc("/bundles/zh-CN.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"bundles": [], "updated_at": null, "version": null}`))
	})
	h, _ := newCatalogHandlerForTest(t, mux)

	ctx := runCatalogBundlesRequest(t, h, "lang=zh-CN", "")

	status := ctx.Response.StatusCode()
	if status != fasthttp.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, ctx.Response.Body())
	}
	if status >= fasthttp.StatusInternalServerError {
		t.Fatalf("endpoint must never return 5xx, got %d", status)
	}

	var resp catalogBundlesResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("decode bundles response: %v (body=%s)", err, ctx.Response.Body())
	}

	// 输入（mock 返回空数组）→ 断言输出（len==0；updated_at/version 为 null）。
	if len(resp.Bundles) != 0 {
		t.Fatalf("bundles len = %d, want 0", len(resp.Bundles))
	}
	if resp.UpdatedAt != nil {
		t.Fatalf("updated_at = %v, want null on empty catalog", *resp.UpdatedAt)
	}
	if resp.Version != nil {
		t.Fatalf("version = %v, want null on empty catalog", *resp.Version)
	}
}

// TestCatalogBundlesUpstreamFailure covers the degraded path: the upstream mock
// answers 500 (or a malformed body) and the endpoint must still return 200 with
// empty bundles — never 5xx.
func TestCatalogBundlesUpstreamFailure(t *testing.T) {
	SetLogger(&mockLogger{})

	for _, tt := range []struct {
		name string
		resp func(w http.ResponseWriter)
	}{
		{
			name: "upstream returns 500",
			resp: func(w http.ResponseWriter) {
				http.Error(w, "boom", http.StatusInternalServerError)
			},
		},
		{
			name: "upstream returns 200 with invalid JSON",
			resp: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`this is not json`))
			},
		},
		{
			name: "upstream returns 503",
			resp: func(w http.ResponseWriter) {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/bundles/zh-CN.json", func(w http.ResponseWriter, r *http.Request) {
				tt.resp(w)
			})
			h, _ := newCatalogHandlerForTest(t, mux)

			ctx := runCatalogBundlesRequest(t, h, "lang=zh-CN", "")

			// 契约：任意上游/解析失败一律 200 + 空 bundles，绝不返 5xx。
			if status := ctx.Response.StatusCode(); status != fasthttp.StatusOK {
				t.Fatalf("status = %d, want 200 (degraded): %s", status, ctx.Response.Body())
			}

			var resp catalogBundlesResponse
			if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
				t.Fatalf("decode bundles response: %v (body=%s)", err, ctx.Response.Body())
			}
			if len(resp.Bundles) != 0 {
				t.Fatalf("bundles len = %d, want 0 on upstream failure", len(resp.Bundles))
			}
		})
	}
}

// TestCatalogBundlesETagNegotiation covers HTTP cache revalidation:
//   - first request (no If-None-Match) → 200 + non-empty ETag header;
//   - second request echoing that exact If-None-Match → 304 with an empty body
//     and the ETag header still present.
func TestCatalogBundlesETagNegotiation(t *testing.T) {
	SetLogger(&mockLogger{})
	mux := http.NewServeMux()
	mux.HandleFunc("/bundles/zh-CN.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"2026-08-28","updated_at":"2026-08-28T08:00:00Z","bundles":[
			{"id":"coding","title":"编程开发","description":"代码补全与调试首选","providers":[
				{"provider":"openai","models":["gpt-4o-mini"],"apply_url":"https://platform.openai.com/signup","apply_steps":["注册账号"],"is_keyless":false,"notes":"新用户首月 $5 免费额度"}]}
		]}`))
	})
	h, _ := newCatalogHandlerForTest(t, mux)

	// 首次请求：无 If-None-Match → 200 + ETag。
	first := runCatalogBundlesRequest(t, h, "lang=zh-CN", "")
	if status := first.Response.StatusCode(); status != fasthttp.StatusOK {
		t.Fatalf("first request status = %d, want 200: %s", status, first.Response.Body())
	}
	etag := string(first.Response.Header.Peek("ETag"))
	if etag == "" {
		t.Fatal("first request must return a non-empty ETag header")
	}

	// 第二次请求：携带相同的 If-None-Match → 304 + 空 body + ETag 头仍在。
	second := runCatalogBundlesRequest(t, h, "lang=zh-CN", etag)
	if status := second.Response.StatusCode(); status != fasthttp.StatusNotModified {
		t.Fatalf("second request status = %d, want 304: %s", status, second.Response.Body())
	}
	if body := second.Response.Body(); len(body) != 0 {
		t.Fatalf("304 response must have an empty body, got %d bytes: %s", len(body), body)
	}
	if got := string(second.Response.Header.Peek("ETag")); got == "" {
		t.Fatalf("304 response must still carry the ETag header, want %q", etag)
	} else if got != etag {
		t.Fatalf("304 ETag = %q, want %q (must match the client's If-None-Match)", got, etag)
	}
}

// TestCatalogBundlesSupportedAnnotation covers the server-side "supported"
// annotation that protects against catalog/binary version drift: the remote
// catalog is always "latest" while the running binary may lack some built-in
// providers. Three shapes:
//   - built-in provider            → supported=true, fallback fields cleared
//   - unknown + valid fallback     → supported=true (custom-provider route),
//     base_provider/base_url preserved
//   - unknown + missing/invalid    → supported=false, fallback fields cleared
//     so clients cannot submit a broken custom-provider payload
func TestCatalogBundlesSupportedAnnotation(t *testing.T) {
	SetLogger(&mockLogger{})
	mux := http.NewServeMux()
	mux.HandleFunc("/bundles/en.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"2026-08-29","updated_at":"2026-08-29T08:00:00Z","bundles":[
			{"id":"coding","title":"Coding","description":"d","providers":[
				{"provider":"openai","models":["gpt-4o-mini"],"apply_url":"","apply_steps":[],"is_keyless":false,"notes":"","base_provider":"openai","base_url":"https://ignored.example.com/v1"},
				{"provider":"together","models":["m1"],"apply_url":"","apply_steps":[],"is_keyless":false,"notes":"","base_provider":"openai","base_url":"https://api.together.xyz/v1"},
				{"provider":"acme","models":["m1"],"apply_url":"","apply_steps":[],"is_keyless":false,"notes":""},
				{"provider":"badbase","models":["m1"],"apply_url":"","apply_steps":[],"is_keyless":false,"notes":"","base_provider":"fireworks","base_url":"https://api.bad.example.com/v1"},
				{"provider":"badurl","models":["m1"],"apply_url":"","apply_steps":[],"is_keyless":false,"notes":"","base_provider":"openai","base_url":"ftp://api.bad.example.com"}]}
		]}`))
	})
	h, _ := newCatalogHandlerForTest(t, mux)

	ctx := runCatalogBundlesRequest(t, h, "lang=en", "")
	if status := ctx.Response.StatusCode(); status != fasthttp.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, ctx.Response.Body())
	}

	var resp catalogBundlesResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("decode bundles response: %v (body=%s)", err, ctx.Response.Body())
	}
	provs := resp.Bundles[0].Providers
	if len(provs) != 5 {
		t.Fatalf("providers len = %d, want 5", len(provs))
	}

	// Built-in: supported, and any upstream fallback fields are cleared —
	// the built-in path always wins.
	if !provs[0].Supported || provs[0].BaseProvider != "" || provs[0].BaseURL != "" {
		t.Fatalf("built-in openai = %+v, want supported=true with cleared fallback fields", provs[0])
	}
	// Unknown + valid fallback: supported via custom-provider route, fields kept.
	if !provs[1].Supported || provs[1].BaseProvider != "openai" || provs[1].BaseURL != "https://api.together.xyz/v1" {
		t.Fatalf("together = %+v, want supported=true with fallback preserved", provs[1])
	}
	// Unknown, no fallback: unsupported, fields stay empty.
	if provs[2].Supported {
		t.Fatalf("acme = %+v, want supported=false (no fallback)", provs[2])
	}
	// Unknown + unsupported base protocol (fireworks is not a supported base
	// provider in core): unsupported, fallback cleared.
	if provs[3].Supported || provs[3].BaseProvider != "" || provs[3].BaseURL != "" {
		t.Fatalf("badbase = %+v, want supported=false with cleared fallback fields", provs[3])
	}
	// Unknown + invalid base URL scheme: unsupported, fallback cleared.
	if provs[4].Supported || provs[4].BaseURL != "" {
		t.Fatalf("badurl = %+v, want supported=false with cleared fallback fields", provs[4])
	}
}

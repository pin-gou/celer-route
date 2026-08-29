package handlers

// Catalog handler: proxies a remote free-tier bundle catalog and serves it at
// GET /api/catalog/bundles with ETag negotiation.
//
// The remote catalog URL is operator-configured via
// config.remote_catalog.url_template (e.g. https://cdn.example.com/bundles/{lang}.json).
// When unconfigured it defaults to
// lib.DefaultRemoteCatalogURLTemplate (the project's GitHub Pages catalog).
// The {lang} placeholder is substituted with the request's lang query param
// (defaulting to en). Because that substitution value is client-controlled,
// every upstream fetch is SSRF-guarded:
//
//   - the final URL host must match the operator-configured template host
//     (after substitution), so the lang parameter can never redirect the
//     fetch to another origin;
//   - fetches against non-loopback hosts go through a transport whose dial
//     uses network.SSRFSafeDialContext — every dial re-resolves the host and
//     refuses non-public addresses (DNS-rebinding safe);
//   - only an operator-declared loopback template host (local/dev/mock
//     catalogs, e.g. http://127.0.0.1:8000/bundles/{lang}.json) is dialed via
//     a private dialer that still blocks link-local/unspecified addresses
//     (cloud metadata).
//
// Any upstream HTTP error, invalid JSON, or size/limit violation degrades the
// endpoint to 200 + an empty bundle list — it never returns 5xx. A background
// refresher (StartCatalogRefresher) keeps the most common languages warm and
// stops itself after 3 consecutive failed refresh cycles.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/router"
	"github.com/pin-gou/celer-route/core/network"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/transports/celer-route-http/lib"
	"github.com/valyala/fasthttp"
)

// catalogFetchTimeout bounds a single upstream fetch (design: 5s).
const catalogFetchTimeout = 5 * time.Second

// catalogRefreshFailureLimit is the number of consecutive failed refresh
// cycles after which the background refresher stops itself.
const catalogRefreshFailureLimit = 3

// catalogDefaultLanguages are prefetched by the background refresher. zh-CN
// and en are the two catalog languages the UI requests (i18n).
var catalogDefaultLanguages = []string{"zh-CN", "en"}

// bundleSnapshot is one validated catalog snapshot for a language. Version and
// UpdatedAt are pointers so an upstream null (empty catalog) round-trips as
// null on the wire.
type bundleSnapshot struct {
	Version   *string        `json:"version"`
	UpdatedAt *string        `json:"updated_at"`
	Bundles   []*bundleEntry `json:"bundles"`
}

// bundleEntry is a single free-tier recommendation bundle.
type bundleEntry struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Providers   []*bundleProviderEntry `json:"providers"`
}

// bundleProviderEntry is one provider entry inside a bundle.
type bundleProviderEntry struct {
	Provider   string   `json:"provider"`
	Models     []string `json:"models"`
	ApplyURL   string   `json:"apply_url"`
	ApplySteps []string `json:"apply_steps"`
	IsKeyless  bool     `json:"is_keyless"`
	Notes      string   `json:"notes"`
}

// bundleCatalogResponse is the wire shape of GET /api/catalog/bundles.
// UpdatedAt/Version are pointers so an empty catalog serializes nulls.
type bundleCatalogResponse struct {
	Bundles   []*bundleEntry `json:"bundles"`
	UpdatedAt *string        `json:"updated_at"`
	Version   *string        `json:"version"`
}

// bundleCatalog is the in-memory snapshot store. It is safe for concurrent
// reads (handler path) and writes (refresher / lazy fetch).
type bundleCatalog struct {
	mu        sync.RWMutex
	snapshots map[string]*bundleSnapshot // keyed by lang
	etags     map[string]string          // keyed by lang
	fetchedAt map[string]time.Time       // keyed by lang
	lastErr   string                     // most recent fetch failure, for logs
}

func newBundleCatalog() *bundleCatalog {
	return &bundleCatalog{
		snapshots: make(map[string]*bundleSnapshot),
		etags:     make(map[string]string),
		fetchedAt: make(map[string]time.Time),
	}
}

// snapshot returns the cached snapshot and ETag for lang (nil when absent).
func (bc *bundleCatalog) snapshot(lang string) (*bundleSnapshot, string) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.snapshots[lang], bc.etags[lang]
}

// store caches a validated snapshot and its derived ETag for lang.
func (bc *bundleCatalog) store(lang string, snap *bundleSnapshot, etag string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.snapshots[lang] = snap
	bc.etags[lang] = etag
	bc.fetchedAt[lang] = time.Now()
}

// lastError returns the most recently recorded fetch failure (best-effort,
// for the refresher stoppage log).
func (bc *bundleCatalog) lastError() string {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.lastErr
}

// recordError stores the latest fetch failure reason.
func (bc *bundleCatalog) recordError(err error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if err != nil {
		bc.lastErr = err.Error()
	}
}

// CatalogHandler proxies the remote free-tier bundle catalog.
type CatalogHandler struct {
	config  *lib.Config
	catalog *bundleCatalog

	// fetchMu serializes upstream fetches so a lazy request and the
	// background refresher never stampede the catalog origin.
	fetchMu sync.Mutex

	// strictClient dials through network.SSRFSafeDialContext (blocks all
	// non-public IPs, re-validated per dial). Used for every non-loopback
	// catalog host.
	strictClient *http.Client
	// localClient permits loopback/RFC1918 but still blocks
	// link-local/unspecified addresses. Used only for operator-declared
	// loopback catalog hosts (local/dev/mock).
	localClient *http.Client

	// templateHost is the operator-configured host extracted from
	// url_template before substitution. Every fetch re-validates that the
	// substituted URL resolves to this same host.
	templateHost string
}

// NewCatalogHandler creates a new CatalogHandler.
func NewCatalogHandler(config *lib.Config) *CatalogHandler {
	dial := func(d func(ctx context.Context, netw, addr string) (net.Conn, error)) *http.Client {
		return &http.Client{
			// Redirects are refused: following a redirect from an
			// attacker-influenced path is a classic SSRF vector and the
			// catalog origin should serve the bundle directly.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: catalogFetchTimeout,
			Transport: &http.Transport{
				DialContext: d,
			},
		}
	}
	h := &CatalogHandler{
		config:       config,
		catalog:      newBundleCatalog(),
		strictClient: dial(network.SSRFSafeDialContext(catalogFetchTimeout)),
		localClient:  dial(catalogPrivateDialContext()),
	}
	if config != nil && config.RemoteCatalog != nil {
		h.templateHost = catalogTemplateHost(config.RemoteCatalog.URLTemplate)
	}
	return h
}

// RegisterRoutes registers the catalog routes.
func (h *CatalogHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	r.GET("/api/catalog/bundles", lib.ChainMiddlewares(h.getCatalogBundles, middlewares...))
}

// StartCatalogRefresher starts the background refresh goroutine. It fetches
// the default languages immediately, then every refresh_interval_seconds.
// After catalogRefreshFailureLimit consecutive failed refresh cycles (no
// language fetched successfully) it stops and logs the reason at INFO level.
// Safe to call multiple times: only one goroutine is started per handler.
func (h *CatalogHandler) StartCatalogRefresher(ctx context.Context) {
	if h.config == nil || h.config.RemoteCatalog == nil || strings.TrimSpace(h.config.RemoteCatalog.URLTemplate) == "" {
		return // catalog module disabled (e.g. handler built without a config)
	}
	interval := time.Duration(h.config.RemoteCatalog.RefreshIntervalSec) * time.Second
	go h.startCatalogRefresher(ctx, interval)
}

// startCatalogRefresher is the refresher loop body (goroutine, see
// StartCatalogRefresher).
func (h *CatalogHandler) startCatalogRefresher(ctx context.Context, interval time.Duration) {
	consecutiveFailures := 0
	for {
		// Trigger an immediate refresh at startup, then on the ticker.
		if refreshed := h.refreshAll(ctx); !refreshed {
			consecutiveFailures++
			if consecutiveFailures >= catalogRefreshFailureLimit {
				logger.Info("remote catalog refresher stopped: %d consecutive refresh cycles failed (last error: %v)", consecutiveFailures, h.lastRefreshError(ctx))
				return
			}
		} else {
			consecutiveFailures = 0
		}

		if interval <= 0 {
			interval = time.Duration(3600) * time.Second
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

// lastRefreshError returns the most recent per-language refresh error for the
// stoppage log. It is best-effort; empty string when none is recorded.
func (h *CatalogHandler) lastRefreshError(_ context.Context) string {
	return h.catalog.lastError()
}

// refreshAll attempts to fetch every default language, caching each success.
// It reports whether at least one language was refreshed.
func (h *CatalogHandler) refreshAll(ctx context.Context) bool {
	ok := false
	for _, lang := range catalogDefaultLanguages {
		if err := h.fetchAndStore(ctx, lang); err != nil {
			logger.Warn("remote catalog refresh failed for lang %s: %v", lang, err)
		} else {
			ok = true
		}
	}
	return ok
}

// getCatalogBundles handles GET /api/catalog/bundles.
//
// Contract (design.md):
//   - lang query param (optional, defaults to en); invalid values fall back
//     to en.
//   - If-None-Match header (optional): when it equals the current ETag, the
//     endpoint answers 304 with an empty body and the ETag header.
//   - On cache miss the upstream is fetched lazily (5s timeout). Any upstream
//     HTTP error / JSON parse failure / size violation degrades to
//     200 + empty bundles — never 5xx.
func (h *CatalogHandler) getCatalogBundles(ctx *fasthttp.RequestCtx) {
	lang := catalogNormalizeLang(string(ctx.QueryArgs().Peek("lang")))

	snap, etag := h.catalog.snapshot(lang)
	if snap == nil {
		h.fetchMu.Lock()
		err := h.fetchAndStore(ctx, lang)
		h.fetchMu.Unlock()
		if err != nil {
			logger.Warn("remote catalog lazy fetch failed for lang %s: %v", lang, err)
		}
		snap, etag = h.catalog.snapshot(lang)
	}

	ctx.Response.Header.Set("ETag", etag)

	if inm := strings.TrimSpace(string(ctx.Request.Header.Peek("If-None-Match"))); inm != "" && inm == etag {
		ctx.SetStatusCode(fasthttp.StatusNotModified)
		ctx.Response.ResetBody()
		return
	}

	resp := bundleCatalogResponse{Bundles: []*bundleEntry{}}
	if snap != nil {
		resp.Bundles = snap.Bundles
		if resp.Bundles == nil {
			resp.Bundles = []*bundleEntry{}
		}
		resp.Version = snap.Version
		resp.UpdatedAt = snap.UpdatedAt
	}
	SendJSON(ctx, resp)
}

// catalogNormalizeLang validates and normalizes a language code. The value is
// embedded into the url_template, so it is strictly constrained to prevent
// URL/host injection; anything invalid falls back to the default "en".
func catalogNormalizeLang(lang string) string {
	if lang == "" {
		return "en"
	}
	if !catalogLangPattern.MatchString(lang) {
		return "en"
	}
	return lang
}

// catalogLangPattern allows only plain RFC 5646-ish language tags
// (letters, digits, hyphens, underscores) of bounded length — never
// characters that could alter the URL structure (@, :, /, ?, #, ...).
var catalogLangPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// fetchAndStore fetches the catalog for lang, validates it, and stores the
// snapshot + derived ETag. Any failure returns a non-nil error and leaves the
// existing snapshot untouched.
func (h *CatalogHandler) fetchAndStore(ctx context.Context, lang string) (err error) {
	defer func() {
		h.catalog.recordError(err)
	}()
	if h.config == nil || h.config.RemoteCatalog == nil {
		return fmt.Errorf("remote catalog not configured")
	}
	rc := h.config.RemoteCatalog
	if strings.TrimSpace(rc.URLTemplate) == "" {
		return fmt.Errorf("remote catalog not configured")
	}

	rawURL := strings.Replace(rc.URLTemplate, "{lang}", lang, 1)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid catalog URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("blocked non-HTTP(S) catalog scheme: %s", parsed.Scheme)
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("catalog URL has no host: %s", rawURL)
	}
	// Host pinning: the substituted language must never change the origin.
	// url_template hosts containing the {lang} placeholder are rejected.
	if h.templateHost != "" && !strings.EqualFold(parsed.Hostname(), h.templateHost) {
		return fmt.Errorf("catalog URL host %q does not match template host %q", parsed.Hostname(), h.templateHost)
	}

	// Only operator-declared loopback catalog hosts (local/dev/mock) may use
	// the private dialer; everything else goes through the SSRF-safe client.
	client := h.strictClient
	if catalogIsLoopbackHost(parsed.Hostname()) {
		client = h.localClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return fmt.Errorf("create catalog request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("catalog fetch: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("catalog origin returned status %d", resp.StatusCode)
	}

	maxSize := int64(rc.MaxBundleSizeBytes)
	if maxSize <= 0 {
		maxSize = 1048576
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return fmt.Errorf("read catalog body: %w", err)
	}
	if int64(len(body)) > maxSize {
		return fmt.Errorf("catalog payload exceeds %d bytes", maxSize)
	}

	var snap bundleSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return fmt.Errorf("invalid catalog JSON: %w", err)
	}

	// Apply limits: keep at most max_provider_models models per provider
	// entry, so a rogue upstream cannot balloon memory.
	if maxModels := rc.MaxProviderModels; maxModels > 0 {
		for _, b := range snap.Bundles {
			for _, p := range b.Providers {
				if len(p.Models) > maxModels {
					p.Models = p.Models[:maxModels]
				}
			}
		}
	}
	if maxBundles := rc.MaxBundles; maxBundles > 0 && len(snap.Bundles) > maxBundles {
		snap.Bundles = snap.Bundles[:maxBundles]
	}

	etag := catalogDeriveETag(body)
	h.catalog.store(lang, &snap, etag)
	return nil
}

// catalogDeriveETag derives a quoted HTTP ETag from the raw snapshot bytes.
func catalogDeriveETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// catalogTemplateHost extracts the operator-configured host from the URL
// template (before {lang} substitution). Empty when the template is
// unparsable or the placeholder sits in the host position.
func catalogTemplateHost(template string) string {
	parsed, err := url.Parse(strings.TrimSpace(template))
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if strings.Contains(host, "{lang}") {
		return "" // placeholder in host position is not allowed
	}
	return host
}

// catalogIsLoopbackHost reports whether host is "localhost" or a loopback IP.
func catalogIsLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// catalogPrivateDialContext returns a dialer for operator-declared loopback
// catalog hosts. Loopback/RFC1918/public receivers are the point of the
// exemption, but unspecified and link-local (cloud metadata) addresses stay
// blocked at dial time — the same gate the provider clients apply — so a DNS
// record that flips to 169.254.169.254 after configuration still cannot be
// reached.
func catalogPrivateDialContext() func(ctx context.Context, netw, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: catalogFetchTimeout}
	return func(ctx context.Context, netw, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no addresses found for %s", host)
		}
		for _, ip := range ips {
			if ip.IsUnspecified() {
				return nil, fmt.Errorf("connection to unspecified IP %s is not allowed", ip)
			}
			if network.IsLinkLocal(ip) {
				return nil, fmt.Errorf("connection to link-local IP %s is not allowed", ip)
			}
		}
		return dialer.DialContext(ctx, netw, net.JoinHostPort(ips[0].String(), port))
	}
}

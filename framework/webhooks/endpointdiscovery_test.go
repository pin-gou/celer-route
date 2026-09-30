package webhooks

import (
	"context"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
)

// EndpointIDsForEvent is the discovery half of the enqueue contract: both
// Enqueue* helpers deliver to nothing when the id list is empty, so a wrong
// answer here silently means "notify nobody" (the M-2 failure mode) or
// "notify the wrong listener". These tests pin the filter and the ordering
// directly, rather than only through the governance plugin that consumes it.

const discoveryRetention = 30 * 24 * time.Hour

// discoveryDispatcher builds a stopped dispatcher wired only with a resolver —
// EndpointIDsForEvent touches nothing else, so the queue and log stores stay
// nil and no worker goroutine is started.
func discoveryDispatcher(r *fakeResolver) *Dispatcher {
	return NewDispatcher(context.Background(), "runner", discoveryRetention, nil, nil, r, nil)
}

func endpointFor(id string, disabled bool, events ...tables.WebhookEvent) *tables.TableWebhookEndpoint {
	ep := testEndpoint(id, "https://example.invalid/"+id)
	ep.Disabled = disabled
	ep.Events = events
	return ep
}

func TestEndpointIDsForEventFiltersBySubscription(t *testing.T) {
	r := newFakeResolver(
		endpointFor("wh-budget", false, tables.WebhookEventBudgetExceeded),
		endpointFor("wh-other", false, tables.WebhookEventAsyncJobCompleted),
		endpointFor("wh-both", false, tables.WebhookEventBudgetExceeded, tables.WebhookEventAsyncJobCompleted),
	)
	d := discoveryDispatcher(r)

	got := d.EndpointIDsForEvent(tables.WebhookEventBudgetExceeded)
	assert.Equal(t, []string{"wh-both", "wh-budget"}, got,
		"only endpoints subscribing to the event may be returned")
}

func TestEndpointIDsForEventSkipsDisabled(t *testing.T) {
	r := newFakeResolver(
		endpointFor("wh-on", false, tables.WebhookEventBudgetExceeded),
		endpointFor("wh-off", true, tables.WebhookEventBudgetExceeded),
	)
	d := discoveryDispatcher(r)

	got := d.EndpointIDsForEvent(tables.WebhookEventBudgetExceeded)
	assert.Equal(t, []string{"wh-on"}, got, "a disabled endpoint is silenced even though it subscribes")
}

// TestEndpointIDsForEventIsSorted pins determinism. The resolver is backed by a
// map, so unsorted output would make fanout order — and any test or log that
// depends on it — vary between runs.
func TestEndpointIDsForEventIsSorted(t *testing.T) {
	r := newFakeResolver(
		endpointFor("wh-zulu", false, tables.WebhookEventBudgetExceeded),
		endpointFor("wh-alpha", false, tables.WebhookEventBudgetExceeded),
		endpointFor("wh-mike", false, tables.WebhookEventBudgetExceeded),
	)
	d := discoveryDispatcher(r)

	for i := 0; i < 20; i++ {
		got := d.EndpointIDsForEvent(tables.WebhookEventBudgetExceeded)
		assert.Equal(t, []string{"wh-alpha", "wh-mike", "wh-zulu"}, got,
			"iteration %d: ids must come back in a stable order", i)
	}
}

func TestEndpointIDsForEventNoSubscribersReturnsEmpty(t *testing.T) {
	r := newFakeResolver(
		endpointFor("wh-other", false, tables.WebhookEventAsyncJobCompleted),
	)
	d := discoveryDispatcher(r)

	assert.Empty(t, d.EndpointIDsForEvent(tables.WebhookEventBudgetExceeded))
}

func TestEndpointIDsForEventNoEndpointsReturnsEmpty(t *testing.T) {
	d := discoveryDispatcher(newFakeResolver())

	assert.Empty(t, d.EndpointIDsForEvent(tables.WebhookEventBudgetExceeded))
}

// TestEndpointIDsForEventNilSafe covers the unconfigured deployment: no
// dispatcher, or a dispatcher with no resolver wired.
func TestEndpointIDsForEventNilSafe(t *testing.T) {
	var nilDispatcher *Dispatcher
	assert.NotPanics(t, func() {
		assert.Empty(t, nilDispatcher.EndpointIDsForEvent(tables.WebhookEventBudgetExceeded))
	})

	d := NewDispatcher(context.Background(), "runner", discoveryRetention, nil, nil, nil, nil)
	assert.NotPanics(t, func() {
		assert.Empty(t, d.EndpointIDsForEvent(tables.WebhookEventBudgetExceeded))
	})
}

// TestEndpointIDsForEventIgnoresMalformedEndpoints guards against a resolver
// that hands back nil entries or rows with no id, which would otherwise be
// passed to Enqueue* and produce jobs keyed by an empty endpoint id.
//
// The malformed rows are injected straight into the resolver's map because
// newFakeResolver dereferences each argument to key it by ID — it cannot carry
// a nil, which is precisely the case under test.
func TestEndpointIDsForEventIgnoresMalformedEndpoints(t *testing.T) {
	r := newFakeResolver(endpointFor("wh-good", false, tables.WebhookEventBudgetExceeded))
	r.endpoints[""] = endpointFor("", false, tables.WebhookEventBudgetExceeded)
	r.endpoints["nil-slot"] = nil

	d := discoveryDispatcher(r)

	assert.NotPanics(t, func() {
		assert.Equal(t, []string{"wh-good"},
			d.EndpointIDsForEvent(tables.WebhookEventBudgetExceeded),
			"nil and id-less endpoints must be dropped, not fanned out to")
	})
}

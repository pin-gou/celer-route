package governance

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/logstore"
	"github.com/pin-gou/celer-route/framework/webhooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fakes below implement the three narrow interfaces webhooks.NewDispatcher
// asks for, so the evaluator can be driven against a REAL dispatcher rather
// than a stub. That matters for M-2: the bug was that the evaluator never
// produced an endpoint id list, and a stubbed dispatcher would have reported
// success regardless. Only a real dispatcher writing into a fake queue proves
// the notification actually reaches the wire.

// fakeWebhookQueue is the dispatcher's ConfigStore. Only CreateWebhookJob is
// load-bearing for these tests; the rest exist to satisfy the interface and
// return "nothing to do" so an accidentally-started worker stays idle.
type fakeWebhookQueue struct {
	mu   sync.Mutex
	jobs []*tables.TableWebhookJob
	// failInsert makes CreateWebhookJob fail, so a test can pin that a
	// delivery-side failure does not panic the hot path.
	failInsert bool
}

func (f *fakeWebhookQueue) CreateWebhookJob(_ context.Context, job *tables.TableWebhookJob) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failInsert {
		return assertError("injected webhook job insert failure")
	}
	cp := *job
	f.jobs = append(f.jobs, &cp)
	return nil
}

func (f *fakeWebhookQueue) ListDueWebhookJobs(context.Context, int) ([]tables.TableWebhookJob, error) {
	return nil, nil
}

func (f *fakeWebhookQueue) ClaimWebhookJob(context.Context, string, string, time.Time) (bool, error) {
	return false, nil
}

func (f *fakeWebhookQueue) RescheduleWebhookJob(context.Context, string, string, time.Time, time.Time) error {
	return nil
}

func (f *fakeWebhookQueue) DeleteWebhookJob(context.Context, string, string, time.Time) error {
	return nil
}

func (f *fakeWebhookQueue) RecordWebhookEndpointSuccess(context.Context, string) error { return nil }

func (f *fakeWebhookQueue) RecordWebhookEndpointFailure(context.Context, string) (int, error) {
	return 0, nil
}

func (f *fakeWebhookQueue) jobCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobs)
}

func (f *fakeWebhookQueue) jobsFor(event tables.WebhookEvent) []*tables.TableWebhookJob {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*tables.TableWebhookJob
	for _, j := range f.jobs {
		if j.Event == event {
			out = append(out, j)
		}
	}
	return out
}

type assertError string

func (e assertError) Error() string { return string(e) }

// fakeWebhookLogStore is the dispatcher's LogStore. Enqueue never reads it, so
// both methods are inert.
type fakeWebhookLogStore struct{}

func (fakeWebhookLogStore) FindAsyncJobByID(context.Context, string) (*logstore.AsyncJob, error) {
	return nil, nil
}

func (fakeWebhookLogStore) CreateWebhookDelivery(context.Context, *logstore.WebhookDelivery) error {
	return nil
}

// fakeEndpointResolver serves endpoints from memory, mirroring lib.Config's
// role in production.
type fakeEndpointResolver struct {
	mu        sync.Mutex
	endpoints map[string]*tables.TableWebhookEndpoint
}

func newFakeEndpointResolver(eps ...*tables.TableWebhookEndpoint) *fakeEndpointResolver {
	r := &fakeEndpointResolver{endpoints: map[string]*tables.TableWebhookEndpoint{}}
	for _, ep := range eps {
		cp := *ep
		r.endpoints[ep.ID] = &cp
	}
	return r
}

func (r *fakeEndpointResolver) WebhookEndpointByID(id string) (*tables.TableWebhookEndpoint, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ep, ok := r.endpoints[id]
	return ep, ok
}

// WebhookEndpoints implements the enumeration half of webhooks.EndpointResolver
// that Dispatcher.EndpointIDsForEvent relies on.
func (r *fakeEndpointResolver) WebhookEndpoints() []*tables.TableWebhookEndpoint {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*tables.TableWebhookEndpoint, 0, len(r.endpoints))
	for _, ep := range r.endpoints {
		out = append(out, ep)
	}
	return out
}

// subscribedEndpoint builds an enabled endpoint listening for the given events.
func subscribedEndpoint(id string, events ...tables.WebhookEvent) *tables.TableWebhookEndpoint {
	return &tables.TableWebhookEndpoint{
		ID:       id,
		Name:     "ep-" + id,
		URL:      "https://example.invalid/hooks/" + id,
		Events:   events,
		Disabled: false,
	}
}

// newStoppedDispatcher builds a real dispatcher that is never Start()ed, so no
// worker goroutine runs and the queue is only ever appended to. That keeps the
// assertion "a job was enqueued" free of delivery races.
func newStoppedDispatcher(t *testing.T, queue *fakeWebhookQueue, resolver webhooks.EndpointResolver) *webhooks.Dispatcher {
	t.Helper()
	d := webhooks.NewDispatcher(context.Background(), "test-runner", time.Hour, queue, fakeWebhookLogStore{}, resolver, noopLogger{})
	require.NotNil(t, d)
	return d
}

func exceededBudget(id string, usage, max float64) *tables.TableBudget {
	teamID := "t1"
	return &tables.TableBudget{
		ID:           id,
		TeamID:       &teamID,
		MaxLimit:     max,
		CurrentUsage: usage,
	}
}

// TestEnqueueBudgetExceededDeliversWebhook is the M-2 regression pin.
//
// The budget.exceeded notification is the only signal an operator gets when a
// hard block starts rejecting traffic with 402. Pre-fix, subscribedEndpoints
// was a stub returning (nil, nil) unconditionally, so the len(endpointIDs)==0
// guard short-circuited and the dispatcher was never called: the webhook was
// silently never delivered, no matter how many endpoints subscribed to the
// event.
//
// The assertion that goes red before the fix: exactly one webhook job for
// budget.exceeded must land in the queue.
func TestEnqueueBudgetExceededDeliversWebhook(t *testing.T) {
	store := newFakeAlertStore()
	queue := &fakeWebhookQueue{}
	resolver := newFakeEndpointResolver(
		subscribedEndpoint("wh-budget", tables.WebhookEventBudgetExceeded),
	)
	eval := NewAlertEvaluator(store, noopLogger{}, newStoppedDispatcher(t, queue, resolver))

	eval.EnqueueBudgetExceeded(context.Background(), exceededBudget("budget-1", 100, 100))

	require.Equal(t, 1, queue.jobCount(),
		"M-2 BUG: budget.exceeded was never enqueued; subscribedEndpoints returned an empty list so the 402 hard block notified nobody")

	jobs := queue.jobsFor(tables.WebhookEventBudgetExceeded)
	require.Len(t, jobs, 1)
	assert.Equal(t, "wh-budget", jobs[0].EndpointID)
	assert.Equal(t, tables.WebhookEventBudgetExceeded, jobs[0].Event)
	assert.NotEmpty(t, jobs[0].PayloadJSON, "the payload must carry the scope and amounts")
}

// TestEnqueueBudgetExceededFansOutToEverySubscriber pins the multi-endpoint
// case: two endpoints subscribed to the event both get a job, so a single
// budget breach notifies every listener rather than an arbitrary one.
func TestEnqueueBudgetExceededFansOutToEverySubscriber(t *testing.T) {
	store := newFakeAlertStore()
	queue := &fakeWebhookQueue{}
	resolver := newFakeEndpointResolver(
		subscribedEndpoint("wh-a", tables.WebhookEventBudgetExceeded),
		subscribedEndpoint("wh-b", tables.WebhookEventBudgetExceeded),
	)
	eval := NewAlertEvaluator(store, noopLogger{}, newStoppedDispatcher(t, queue, resolver))

	eval.EnqueueBudgetExceeded(context.Background(), exceededBudget("budget-1", 150, 100))

	jobs := queue.jobsFor(tables.WebhookEventBudgetExceeded)
	require.Len(t, jobs, 2, "every subscribed endpoint must receive the notification")
	got := map[string]bool{jobs[0].EndpointID: true, jobs[1].EndpointID: true}
	assert.True(t, got["wh-a"] && got["wh-b"], "both endpoints must be notified, got %v", got)
}

// TestEnqueueBudgetExceededSkipsUnsubscribedEndpoint confirms the event filter
// is honoured: an endpoint that exists but listens only to other events must
// NOT receive budget.exceeded. Without this, the fix would over-deliver.
func TestEnqueueBudgetExceededSkipsUnsubscribedEndpoint(t *testing.T) {
	store := newFakeAlertStore()
	queue := &fakeWebhookQueue{}
	resolver := newFakeEndpointResolver(
		subscribedEndpoint("wh-budget", tables.WebhookEventBudgetExceeded),
		subscribedEndpoint("wh-other", tables.WebhookEventAsyncJobCompleted),
	)
	eval := NewAlertEvaluator(store, noopLogger{}, newStoppedDispatcher(t, queue, resolver))

	eval.EnqueueBudgetExceeded(context.Background(), exceededBudget("budget-1", 100, 100))

	jobs := queue.jobsFor(tables.WebhookEventBudgetExceeded)
	require.Len(t, jobs, 1, "only the endpoint subscribed to budget.exceeded may be notified")
	assert.Equal(t, "wh-budget", jobs[0].EndpointID)
}

// TestEnqueueBudgetExceededSkipsDisabledEndpoint pins that a disabled endpoint
// is not notified even though it still subscribes to the event — operators
// disable an endpoint to silence it, and a hard block must respect that.
func TestEnqueueBudgetExceededSkipsDisabledEndpoint(t *testing.T) {
	store := newFakeAlertStore()
	queue := &fakeWebhookQueue{}
	disabled := subscribedEndpoint("wh-off", tables.WebhookEventBudgetExceeded)
	disabled.Disabled = true
	resolver := newFakeEndpointResolver(
		disabled,
		subscribedEndpoint("wh-on", tables.WebhookEventBudgetExceeded),
	)
	eval := NewAlertEvaluator(store, noopLogger{}, newStoppedDispatcher(t, queue, resolver))

	eval.EnqueueBudgetExceeded(context.Background(), exceededBudget("budget-1", 100, 100))

	jobs := queue.jobsFor(tables.WebhookEventBudgetExceeded)
	require.Len(t, jobs, 1, "a disabled endpoint must be silenced")
	assert.Equal(t, "wh-on", jobs[0].EndpointID)
}

// TestEnqueueBudgetExceededNoSubscribersIsSilent pins the legitimate empty
// case: with no endpoint subscribed, nothing is enqueued and nothing panics.
// This is the branch the pre-fix stub always took, so it must stay green.
func TestEnqueueBudgetExceededNoSubscribersIsSilent(t *testing.T) {
	store := newFakeAlertStore()
	queue := &fakeWebhookQueue{}
	resolver := newFakeEndpointResolver(
		subscribedEndpoint("wh-other", tables.WebhookEventAsyncJobCompleted),
	)
	eval := NewAlertEvaluator(store, noopLogger{}, newStoppedDispatcher(t, queue, resolver))

	eval.EnqueueBudgetExceeded(context.Background(), exceededBudget("budget-1", 100, 100))

	assert.Equal(t, 0, queue.jobCount(), "no subscriber means no job")
}

// TestEnqueueBudgetExceededNilDispatcherIsSafe covers the wiring where webhook
// delivery is not configured at all: the evaluator must return quietly rather
// than dereference a nil dispatcher.
func TestEnqueueBudgetExceededNilDispatcherIsSafe(t *testing.T) {
	store := newFakeAlertStore()
	eval := NewAlertEvaluator(store, noopLogger{}, nil)

	require.NotPanics(t, func() {
		eval.EnqueueBudgetExceeded(context.Background(), exceededBudget("budget-1", 100, 100))
	})
}

// TestEnqueueBudgetExceededSurvivesInsertFailure pins the "失败绝不阻断"
// contract: a queue write failure is logged and swallowed, never propagated to
// the request path that triggered the 402.
func TestEnqueueBudgetExceededSurvivesInsertFailure(t *testing.T) {
	store := newFakeAlertStore()
	queue := &fakeWebhookQueue{failInsert: true}
	resolver := newFakeEndpointResolver(
		subscribedEndpoint("wh-budget", tables.WebhookEventBudgetExceeded),
	)
	eval := NewAlertEvaluator(store, noopLogger{}, newStoppedDispatcher(t, queue, resolver))

	require.NotPanics(t, func() {
		eval.EnqueueBudgetExceeded(context.Background(), exceededBudget("budget-1", 100, 100))
	})
	assert.Equal(t, 0, queue.jobCount())
}

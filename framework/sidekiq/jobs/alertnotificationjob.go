package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/sidekiq"
	"github.com/pin-gou/celer-route/framework/webhooks"
)

// AlertNotificationStore is the subset of configstore the alert
// notification job needs. Splitting it lets us unit-test the job without
// standing up the full schema.
type AlertNotificationStore interface {
	GetAlertRuleByID(ctx context.Context, id string) (*tables.TableAlertRule, error)
	GetAlertEventByID(ctx context.Context, id string) (*tables.TableAlertEvent, error)
}

// AlertNotificationJob is the asynchronous path for budget.exceeded
// notifications (02-alerting/flows.md §2): when a 402 fires we record the
// alert_event row and let sidekiq dispatch the webhook enqueue so the 402
// response is never gated on alert I/O. Soft-threshold emissions use the
// inline AlertEvaluator path; this job exists primarily so a budget.exceeded
// emission survives a process restart that would otherwise drop the
// dispatcher-enqueued row before the worker drained it.
//
// In practice the inline AlertEvaluator.EnqueueBudgetExceeded already
// enqueues via the dispatcher (which is durable). This job is the
// retry-with-backoff path for the rare case where the dispatcher is
// unreachable at the moment of the 402; the job re-reads the rule + event
// and re-enqueues, picking up the dispatcher's normal retry/backoff.
type AlertNotificationJob struct {
	store      AlertNotificationStore
	dispatcher *webhooks.Dispatcher
}

// NewAlertNotificationJob wires the job. dispatcher may be nil; the
// handler then returns an error so the sidekiq runner marks the job
// failed rather than silently dropping the notification.
func NewAlertNotificationJob(store AlertNotificationStore, dispatcher *webhooks.Dispatcher) *AlertNotificationJob {
	return &AlertNotificationJob{store: store, dispatcher: dispatcher}
}

// Kind is the sidekiq kind string.
func (j *AlertNotificationJob) Kind() string { return "alert_notification" }

// AlertNotificationJobMetadata is the JSON payload carried in the
// sidekiq job's Metadata field. Keeping it typed makes it easy to
// inspect the queue from ops tooling without parsing free-form blobs.
type AlertNotificationJobMetadata struct {
	RuleID   string    `json:"rule_id"`
	EventID  string    `json:"event_id"`
	QueuedAt time.Time `json:"queued_at"`
}

// Handle satisfies sidekiq.HandlerFunc. It re-reads the rule and event
// (the rows may have been written milliseconds ago and are now stable),
// builds the dispatcher's alert envelope, and enqueues per-channel
// deliveries through the dispatcher's normal queue.
func (j *AlertNotificationJob) Handle(ctx context.Context, job tables.TableSidekiqJob, _ sidekiq.ProgressFunc) (string, error) {
	if j.store == nil || j.dispatcher == nil {
		return "", fmt.Errorf("alert notification job: dependencies are not wired")
	}
	var meta AlertNotificationJobMetadata
	if job.Metadata != "" && job.Metadata != "{}" {
		if err := json.Unmarshal([]byte(job.Metadata), &meta); err != nil {
			return "", fmt.Errorf("alert notification job: parse metadata: %w", err)
		}
	}
	if meta.RuleID == "" || meta.EventID == "" {
		return "", fmt.Errorf("alert notification job: rule_id and event_id are required")
	}
	rule, err := j.store.GetAlertRuleByID(ctx, meta.RuleID)
	if err != nil || rule == nil {
		return "", fmt.Errorf("alert notification job: rule %s not found", meta.RuleID)
	}
	event, err := j.store.GetAlertEventByID(ctx, meta.EventID)
	if err != nil || event == nil {
		return "", fmt.Errorf("alert notification job: event %s not found", meta.EventID)
	}
	endpointIDs := make([]string, 0, len(rule.Channels))
	for _, ch := range rule.Channels {
		if ch.Type == tables.AlertChannelTypeWebhook && ch.WebhookID != "" {
			endpointIDs = append(endpointIDs, ch.WebhookID)
		}
	}
	queued := j.dispatcher.EnqueueAlertEvent(ctx, rule, event, endpointIDs)
	out, _ := json.Marshal(map[string]any{
		"queued":      queued,
		"rule_id":     rule.ID,
		"event_id":    event.ID,
		"completed":   time.Now().UTC(),
	})
	return string(out), nil
}

// EnqueueAlertNotification persists a durable alert-notification job so the
// 402 path can return immediately. The job re-reads the rule + event and
// emits the webhook delivery on the next sidekiq slot.
func EnqueueAlertNotification(r *sidekiq.Runner, store AlertNotificationStore, dispatcher *webhooks.Dispatcher, ruleID, eventID string) error {
	if r == nil {
		return fmt.Errorf("sidekiq runner is required")
	}
	meta, _ := json.Marshal(AlertNotificationJobMetadata{
		RuleID:   ruleID,
		EventID:  eventID,
		QueuedAt: time.Now().UTC(),
	})
	job := NewAlertNotificationJob(store, dispatcher)
	r.Register(job.Kind(), job.Handle)
	id := "alert_notification-" + uuid.NewString()
	return r.Enqueue(context.Background(), id, job.Kind(), string(meta), "")
}
package notification

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func NewBudgetStoppedEvent(projectID uuid.UUID, spentUSD, budgetUSD float64, now time.Time, dashboardURL string) Event {
	period := now.Format("2006-01")
	budget := fmt.Sprintf("%.2f", budgetUSD)
	payload := mustPayload(map[string]any{
		"project_id":    projectID.String(),
		"spent_usd":     spentUSD,
		"budget_usd":    budgetUSD,
		"period":        period,
		"dashboard_url": dashboardURL,
		"message":       fmt.Sprintf("CiteLoop budget stopped for %s: $%.2f spent of $%.2f budget.", period, spentUSD, budgetUSD),
	})
	return Event{
		ProjectID: projectID,
		Type:      "budget.stopped",
		ID:        BudgetStoppedEventID(projectID.String(), period, budget),
		Payload:   payload,
	}
}

func NewPublishFailedEvent(projectID, articleID uuid.UUID, title, slug, phase string, attempt int32, errText string, now time.Time, dashboardURL string, transition bool) Event {
	payload := mustPayload(map[string]any{
		"project_id":    projectID.String(),
		"article_id":    articleID.String(),
		"title":         title,
		"slug":          slug,
		"phase":         phase,
		"attempt":       attempt,
		"error":         errText,
		"dashboard_url": dashboardURL,
		"message":       fmt.Sprintf("CiteLoop publish failed for %s: %s", articleID, errText),
	})
	eventID := fmt.Sprintf("publish.failed:%s:%s:transition:%d", articleID, phase, attempt)
	if !transition {
		fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(errText)))
		eventID = fmt.Sprintf("publish.failed:%s:%s:daily:%s:%s", articleID, phase, now.Format("2006-01-02"), fingerprint)
	}
	return Event{
		ProjectID: projectID,
		Type:      "publish.failed",
		ID:        eventID,
		Payload:   payload,
	}
}

func NewGenerationFailedEvent(projectID uuid.UUID, agent, scope, title, errText string, now time.Time, dashboardURL string) Event {
	day := now.Format("2006-01-02")
	payload := mustPayload(map[string]any{
		"project_id":    projectID.String(),
		"agent":         agent,
		"scope":         scope,
		"title":         title,
		"error":         errText,
		"dashboard_url": dashboardURL,
		"message":       fmt.Sprintf("CiteLoop %s generation failed for %s: %s", agent, title, errText),
	})
	return Event{
		ProjectID: projectID,
		Type:      "generation.failed",
		ID:        fmt.Sprintf("generation.failed:%s:%s:%s:%s", projectID, agent, scope, day),
		Payload:   payload,
	}
}

func NewReviewOverdueEvent(projectID, articleID uuid.UUID, title string, ageHours int, now time.Time, dashboardURL string) Event {
	payload := mustPayload(map[string]any{
		"project_id":    projectID.String(),
		"article_id":    articleID.String(),
		"title":         title,
		"age_hours":     ageHours,
		"dashboard_url": dashboardURL,
		"message":       fmt.Sprintf("CiteLoop review overdue for %s: pending for %d hours.", title, ageHours),
	})
	return Event{
		ProjectID: projectID,
		Type:      "review.overdue",
		ID:        fmt.Sprintf("review.overdue:%s:%s", articleID, now.Format("2006-01-02")),
		Payload:   payload,
	}
}

func NewWebhookDeliveryDeadEvent(projectID, deliveryID, channelID uuid.UUID, eventType, lastError, dashboardURL string) Event {
	payload := mustPayload(map[string]any{
		"project_id":    projectID.String(),
		"delivery_id":   deliveryID.String(),
		"channel_id":    channelID.String(),
		"event_type":    eventType,
		"last_error":    lastError,
		"dashboard_url": dashboardURL,
		"message":       fmt.Sprintf("CiteLoop notification delivery dead for %s: %s", eventType, lastError),
	})
	return Event{
		ProjectID: projectID,
		Type:      "webhook.delivery.dead",
		ID:        fmt.Sprintf("webhook.delivery.dead:%s", deliveryID),
		Payload:   payload,
	}
}

func mustPayload(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

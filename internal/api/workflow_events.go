package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) enqueueWorkflowEvent(ctx context.Context, projectID uuid.UUID, eventType, entityType string, entityID uuid.UUID, dedupeKey string, payload any) error {
	if s.Q == nil {
		return nil
	}
	raw := json.RawMessage(`{}`)
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		raw = b
	}
	var entityTypePtr *string
	var entityUUID pgtype.UUID
	if entityType != "" {
		entityTypePtr = &entityType
	}
	if entityID != uuid.Nil {
		entityUUID = pgtype.UUID{Bytes: entityID, Valid: true}
	}
	_, err := s.Q.EnqueueWorkflowEvent(ctx, db.EnqueueWorkflowEventParams{
		ProjectID:  projectID,
		EventType:  eventType,
		DedupeKey:  dedupeKey,
		Payload:    raw,
		EntityType: entityTypePtr,
		EntityID:   entityUUID,
		RunAfter:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	return err
}

func workflowDedupeKey(eventType string, projectID uuid.UUID, parts ...any) string {
	key := eventType + ":" + projectID.String()
	for _, part := range parts {
		key += ":" + fmt.Sprint(part)
	}
	return key
}

package agents

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/aicalls"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const runtimeProvider = "runtime_route"

type aiCallRetryContextKey struct{}

// WithAICallRetry marks a workflow re-entry as a retry of the prior exact
// request. Fresh periodic/manual executions remain independent roots.
func WithAICallRetry(ctx context.Context) context.Context {
	return context.WithValue(ctx, aiCallRetryContextKey{}, true)
}

func completeTracked(ctx context.Context, store aicalls.Store, provider llm.Provider, projectID uuid.UUID, stage, objectType string, objectID uuid.UUID, promptVersion string, parentCallID, causedByCallID uuid.UUID, req llm.CompletionReq) (llm.CompletionResp, uuid.UUID, error) {
	// Small unit tests exercise the prompt/parse helpers without a database.
	// Production agent entry points always supply both Q and project identity.
	if store == nil || projectID == uuid.Nil || objectID == uuid.Nil {
		resp, err := provider.Complete(ctx, req)
		return resp, uuid.Nil, err
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = runtimeProvider
	}
	fingerprint := aicalls.Fingerprint(req)
	if parentCallID == uuid.Nil {
		latest, latestErr := aicalls.New(store).Latest(ctx, aicalls.Spec{
			ProjectID: projectID, Stage: stage, LinkedObjectType: objectType,
			LinkedObjectID: objectID, RequestFingerprint: fingerprint,
		})
		if latestErr == nil {
			explicitRetry, _ := ctx.Value(aiCallRetryContextKey{}).(bool)
			if explicitRetry || latest.Status == "failed" || latest.Status == "partial" || latest.Status == "skipped" {
				parentCallID = latest.ID
			}
		} else if !errors.Is(latestErr, pgx.ErrNoRows) {
			return llm.CompletionResp{}, uuid.Nil, latestErr
		}
	}
	completion, err := aicalls.New(store).Complete(ctx, aicalls.Spec{
		ProjectID: projectID, Stage: stage, LinkedObjectType: objectType, LinkedObjectID: objectID,
		Provider: runtimeProvider, Model: model, PromptVersion: promptVersion,
		RequestFingerprint: fingerprint, ParentCallID: parentCallID, CausedByCallID: causedByCallID,
	}, provider, req)
	return completion.Response, completion.Call.ID, err
}

func failTrackedOutput(ctx context.Context, store aicalls.Store, projectID, callID uuid.UUID, errorCode string) error {
	if store == nil || projectID == uuid.Nil || callID == uuid.Nil {
		return nil
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err := aicalls.New(store).FailOutput(finishCtx, callID, projectID, errorCode)
	return err
}

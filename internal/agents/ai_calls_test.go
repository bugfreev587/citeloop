package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestExplicitSeparateAgentRetryContinuesRetryChain(t *testing.T) {
	provider := &sequenceLLM{resps: []string{"first", "second"}}
	ledger := &agentAICallStore{rows: map[uuid.UUID]db.AiCallRecord{}}
	projectID, topicID := uuid.New(), uuid.New()
	req := llm.CompletionReq{Prompt: "same exact request"}

	_, firstID, err := completeTracked(context.Background(), ledger, provider, projectID, "content_generation", "topic", topicID, "writer-v2", uuid.Nil, uuid.Nil, req)
	if err != nil {
		t.Fatal(err)
	}
	_, secondID, err := completeTracked(WithAICallRetry(context.Background()), ledger, provider, projectID, "content_generation", "topic", topicID, "writer-v2", uuid.Nil, uuid.Nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if firstID == secondID || len(ledger.starts) != 2 || !ledger.starts[1].ParentCallID.Valid || uuid.UUID(ledger.starts[1].ParentCallID.Bytes) != firstID {
		t.Fatalf("first=%s second=%s starts=%+v", firstID, secondID, ledger.starts)
	}
}

func TestFreshExactAgentInvocationRemainsIndependentRoot(t *testing.T) {
	provider := &sequenceLLM{resps: []string{"first", "second"}}
	ledger := &agentAICallStore{rows: map[uuid.UUID]db.AiCallRecord{}}
	projectID, objectID := uuid.New(), uuid.New()
	req := llm.CompletionReq{Prompt: "unchanged periodic observation"}
	for range 2 {
		if _, _, err := completeTracked(context.Background(), ledger, provider, projectID, "evidence", "project", objectID, "profile-v2", uuid.Nil, uuid.Nil, req); err != nil {
			t.Fatal(err)
		}
	}
	if len(ledger.starts) != 2 || ledger.starts[1].ParentCallID.Valid {
		t.Fatalf("fresh exact observations must be roots: %+v", ledger.starts)
	}
}

func TestGenerationRunPreservesMetadataButDoesNotDuplicateCanonicalSpend(t *testing.T) {
	spy := &insightDBSpy{}
	recordRun(context.Background(), db.New(spy), uuid.New(), agentWriter, map[string]string{"in": "x"}, map[string]string{"out": "y"}, llm.CompletionResp{
		Model: "resolved-model", Tokens: 17, CostUSD: 4.25,
	}, nil)
	if len(spy.runs) != 1 {
		t.Fatalf("runs=%+v", spy.runs)
	}
	run := spy.runs[0]
	if run.model == nil || *run.model != "resolved-model" || run.tokens == nil || *run.tokens != 17 {
		t.Fatalf("legacy metadata regressed: %+v", run)
	}
	cost, err := run.cost.Float64Value()
	if err != nil || !cost.Valid || cost.Float64 != 0 {
		t.Fatalf("generation run must carry explicit zero cost, got %+v err=%v", cost, err)
	}
}

func TestQARetryCreatesLinkedProviderCallsAndPreservesInvalidAttempt(t *testing.T) {
	valid := `{"claims":[],"qa_blocking":false,"geo_score":0.9,"seo_score":0.9,"issues":[]}`
	provider := &sequenceLLM{resps: []string{"not json", valid}}
	ledger := &agentAICallStore{rows: map[uuid.UUID]db.AiCallRecord{}}
	qa := NewQA(Deps{LLM: provider, AICalls: ledger}, nil)
	projectID, topicID := uuid.New(), uuid.New()

	_, _, lastCallID, err := qa.completeQAWithRetryForObject(context.Background(), projectID, "topic", topicID, uuid.Nil, uuid.Nil, llm.CompletionReq{Prompt: "audit", JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger.starts) != 2 || len(ledger.finishes) != 2 || len(ledger.reclassified) != 1 {
		t.Fatalf("starts=%d finishes=%d reclassified=%d", len(ledger.starts), len(ledger.finishes), len(ledger.reclassified))
	}
	if !ledger.starts[1].ParentCallID.Valid || uuid.UUID(ledger.starts[1].ParentCallID.Bytes) != ledger.starts[0].ID {
		t.Fatalf("retry parent=%+v first=%s", ledger.starts[1].ParentCallID, ledger.starts[0].ID)
	}
	if ledger.starts[0].RequestFingerprint != ledger.starts[1].RequestFingerprint {
		t.Fatal("QA retry must retain request fingerprint")
	}
	if lastCallID != ledger.starts[1].ID || ledger.rows[ledger.starts[0].ID].Status != "failed" || ledger.rows[lastCallID].Status != "ok" {
		t.Fatalf("rows=%+v last=%s", ledger.rows, lastCallID)
	}
}

type agentAICallStore struct {
	starts       []db.AiCallRecord
	finishes     []db.FinishAICallRecordParams
	reclassified []db.ReclassifyAICallRecordOutputFailureParams
	rows         map[uuid.UUID]db.AiCallRecord
}

func (s *agentAICallStore) CreateAICallRecord(_ context.Context, arg db.CreateAICallRecordParams) (db.AiCallRecord, error) {
	row := db.AiCallRecord{ID: uuid.New(), ProjectID: arg.ProjectID, Stage: arg.Stage, LinkedObjectType: arg.LinkedObjectType, LinkedObjectID: arg.LinkedObjectID, Status: "running", RequestFingerprint: arg.RequestFingerprint, ParentCallID: arg.ParentCallID}
	s.starts = append(s.starts, row)
	s.rows[row.ID] = row
	return row, nil
}

func (s *agentAICallStore) CreateSkippedAICallRecord(_ context.Context, arg db.CreateSkippedAICallRecordParams) (db.AiCallRecord, error) {
	row := db.AiCallRecord{ID: uuid.New(), ProjectID: arg.ProjectID, Status: "skipped", ErrorCode: arg.ErrorCode}
	s.rows[row.ID] = row
	return row, nil
}

func (s *agentAICallStore) FinishAICallRecord(_ context.Context, arg db.FinishAICallRecordParams) (db.AiCallRecord, error) {
	row, ok := s.rows[arg.ID]
	if !ok {
		return db.AiCallRecord{}, errors.New("missing call")
	}
	s.finishes = append(s.finishes, arg)
	row.Status, row.ErrorCode = arg.Status, arg.ErrorCode
	row.PromptTokens, row.CompletionTokens, row.TotalTokens, row.CostUsd = arg.PromptTokens, arg.CompletionTokens, arg.TotalTokens, arg.CostUsd
	if arg.ResolvedProvider != nil {
		row.Provider = *arg.ResolvedProvider
	}
	if arg.ResolvedModel != nil {
		row.Model = *arg.ResolvedModel
	}
	s.rows[row.ID] = row
	return row, nil
}

func (s *agentAICallStore) FinishCanonicalAICallFenced(_ context.Context, arg db.FinishCanonicalAICallFencedParams) (db.AiCallRecord, error) {
	row, ok := s.rows[arg.ID]
	if !ok {
		return db.AiCallRecord{}, errors.New("missing call")
	}
	s.finishes = append(s.finishes, db.FinishAICallRecordParams{
		Status: arg.Status, ResolvedProvider: arg.ResolvedProvider, ResolvedModel: arg.ResolvedModel,
		ErrorCode: arg.ErrorCode, PromptTokens: arg.PromptTokens, CompletionTokens: arg.CompletionTokens,
		TotalTokens: arg.TotalTokens, CostUsd: arg.CostUsd, ID: arg.ID, ProjectID: arg.ProjectID,
	})
	row.Status, row.ErrorCode = arg.Status, arg.ErrorCode
	row.PromptTokens, row.CompletionTokens, row.TotalTokens, row.CostUsd = arg.PromptTokens, arg.CompletionTokens, arg.TotalTokens, arg.CostUsd
	if arg.ResolvedProvider != nil {
		row.Provider = *arg.ResolvedProvider
	}
	if arg.ResolvedModel != nil {
		row.Model = *arg.ResolvedModel
	}
	s.rows[row.ID] = row
	return row, nil
}

func (s *agentAICallStore) ReclassifyAICallRecordOutputFailure(_ context.Context, arg db.ReclassifyAICallRecordOutputFailureParams) (db.AiCallRecord, error) {
	row, ok := s.rows[arg.ID]
	if !ok {
		return db.AiCallRecord{}, errors.New("missing call")
	}
	s.reclassified = append(s.reclassified, arg)
	row.Status, row.ErrorCode = "failed", arg.ErrorCode
	s.rows[row.ID] = row
	return row, nil
}

func (s *agentAICallStore) GetAICallRecord(_ context.Context, arg db.GetAICallRecordParams) (db.AiCallRecord, error) {
	row, ok := s.rows[arg.ID]
	if !ok {
		return db.AiCallRecord{}, errors.New("missing call")
	}
	return row, nil
}

func (s *agentAICallStore) MarkAICallProviderStarted(_ context.Context, arg db.MarkAICallProviderStartedParams) (db.AiCallRecord, error) {
	row, ok := s.rows[arg.ID]
	if !ok {
		return db.AiCallRecord{}, errors.New("missing call")
	}
	row.Status, row.ProviderCalled = "running", true
	if arg.ResolvedModel != nil {
		row.Model = *arg.ResolvedModel
	}
	s.rows[row.ID] = row
	return row, nil
}

func (s *agentAICallStore) GetLatestAICallForRequest(_ context.Context, arg db.GetLatestAICallForRequestParams) (db.AiCallRecord, error) {
	for i := len(s.starts) - 1; i >= 0; i-- {
		row := s.starts[i]
		if row.ProjectID == arg.ProjectID && row.Stage == arg.Stage && row.LinkedObjectType == arg.LinkedObjectType && row.LinkedObjectID == arg.LinkedObjectID && row.RequestFingerprint == arg.RequestFingerprint {
			return row, nil
		}
	}
	return db.AiCallRecord{}, pgx.ErrNoRows
}

func (s *agentAICallStore) AggregateAICallsForObject(context.Context, db.AggregateAICallsForObjectParams) (db.AggregateAICallsForObjectRow, error) {
	return db.AggregateAICallsForObjectRow{}, nil
}

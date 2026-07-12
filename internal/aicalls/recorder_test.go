package aicalls

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
)

func TestCompletePersistsOneTerminalProviderCall(t *testing.T) {
	store := &storeStub{}
	recorder := New(store)
	spec := testSpec()
	completion, err := recorder.Complete(context.Background(), spec, providerStub{resp: llm.CompletionResp{
		Provider: "actual", Model: "resolved", PromptTokens: 7, CompletionTokens: 5, Tokens: 12, CostUSD: .02,
	}}, llm.CompletionReq{Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if completion.Call.Status != "ok" || len(store.starts) != 1 || len(store.finishes) != 1 {
		t.Fatalf("completion=%+v starts=%d finishes=%d", completion.Call, len(store.starts), len(store.finishes))
	}
	finish := store.finishes[0]
	if finish.TotalTokens != 12 || finish.ResolvedProvider == nil || *finish.ResolvedProvider != "actual" {
		t.Fatalf("finish=%+v", finish)
	}
}

func TestRetryCreatesSeparateChildRecordWithSameFingerprint(t *testing.T) {
	store := &storeStub{}
	recorder := New(store)
	first := testSpec()
	firstCall, err := recorder.Start(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	retry := first
	retry.ParentCallID = firstCall.ID
	retryCall, err := recorder.Start(context.Background(), retry)
	if err != nil {
		t.Fatal(err)
	}
	if retryCall.ID == firstCall.ID || len(store.starts) != 2 || !store.starts[1].ParentCallID.Valid {
		t.Fatalf("first=%+v retry=%+v starts=%+v", firstCall, retryCall, store.starts)
	}
	if store.starts[0].RequestFingerprint != store.starts[1].RequestFingerprint {
		t.Fatal("retry request fingerprint changed")
	}
}

func TestUnavailableProviderCreatesSkippedRecordWithoutStartingProviderCall(t *testing.T) {
	store := &storeStub{}
	recorder := New(store)
	completion, err := recorder.Complete(context.Background(), testSpec(), nil, llm.CompletionReq{Prompt: "hello"})
	if err == nil || completion.Call.Status != "skipped" {
		t.Fatalf("completion=%+v err=%v", completion, err)
	}
	if len(store.starts) != 0 || len(store.skips) != 1 || len(store.finishes) != 0 {
		t.Fatalf("starts=%d skips=%d finishes=%d", len(store.starts), len(store.skips), len(store.finishes))
	}
}

func TestFinishRejectsSkippedBecauseNoProviderCallMayBeFaked(t *testing.T) {
	_, err := New(&storeStub{}).Finish(context.Background(), uuid.New(), uuid.New(), Finish{Status: "skipped"})
	if err == nil {
		t.Fatal("expected skipped finish rejection")
	}
}

func TestOpenAIModelFallbackCreatesTwoPhysicalAttemptRows(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"model not supported for ChatGPT account on OpenAI"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"gpt-5.1","choices":[{"message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":11,"completion_tokens":4,"total_tokens":15}}`))
	}))
	defer server.Close()
	store := &storeStub{}
	req := llm.CompletionReq{Prompt: "hello", Model: "claude-sonnet-4-6", JSON: true}
	completion, err := New(store).Complete(context.Background(), testSpec(), llm.NewOpenAIChat("key", server.URL, req.Model), req)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(store.starts) != 2 || len(store.finishes) != 2 {
		t.Fatalf("requests=%d starts=%d finishes=%d", requests, len(store.starts), len(store.finishes))
	}
	if !store.starts[1].ParentCallID.Valid || uuid.UUID(store.starts[1].ParentCallID.Bytes) != store.rowsByStart(0).ID {
		t.Fatalf("fallback parent=%+v first=%s", store.starts[1].ParentCallID, store.rowsByStart(0).ID)
	}
	if store.finishes[0].Status != "failed" || store.finishes[1].Status != "ok" || completion.Call.Model != "gpt-5.1" {
		t.Fatalf("finishes=%+v completion=%+v", store.finishes, completion.Call)
	}
}

func TestObservableProviderWithoutAttemptIsSkippedAndFails(t *testing.T) {
	store := &storeStub{}
	completion, err := New(store).Complete(context.Background(), testSpec(), silentObservableProvider{}, llm.CompletionReq{Prompt: "hello"})
	if err == nil || completion.Call.Status != "skipped" || len(store.starts) != 0 || len(store.skips) != 1 {
		t.Fatalf("completion=%+v err=%v starts=%d skips=%d", completion, err, len(store.starts), len(store.skips))
	}
}

func TestOpenAIAccountingFailureStopsBeforeModelFallback(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"model not supported for ChatGPT account on OpenAI"}}`))
	}))
	defer server.Close()
	store := &storeStub{fenceFailures: 3}
	req := llm.CompletionReq{Prompt: "hello", Model: "claude-sonnet-4-6"}
	_, err := New(store).Complete(context.Background(), testSpec(), llm.NewOpenAIChat("key", server.URL, req.Model), req)
	if err == nil || requests != 1 {
		t.Fatalf("err=%v requests=%d, accounting failure must stop before fallback spend", err, requests)
	}
}

func TestFinishAcceptsReclaimedVerdictOnlyWhenLateUsageWasStored(t *testing.T) {
	store := &storeStub{preserveTerminalStatus: true}
	recorder := New(store)
	call, err := recorder.Start(context.Background(), testSpec())
	if err != nil {
		t.Fatal(err)
	}
	row := store.rows[call.ID]
	cleanup := "stale_running_call"
	row.Status, row.ErrorCode = "failed", &cleanup
	store.rows[call.ID] = row
	finished, err := recorder.Finish(context.Background(), call.ID, call.ProjectID, Finish{
		Status: "ok", ResolvedProvider: "actual", ResolvedModel: "resolved",
		PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8, CostUSD: 0.02,
	})
	if err != nil || finished.Status != "failed" || finished.TotalTokens != 8 {
		t.Fatalf("finished=%+v err=%v", finished, err)
	}
	store.discardAccounting = true
	_, err = recorder.Finish(context.Background(), call.ID, call.ProjectID, Finish{
		Status: "ok", ResolvedProvider: "actual", ResolvedModel: "resolved",
		PromptTokens: 6, CompletionTokens: 4, TotalTokens: 10, CostUSD: 0.03,
	})
	if err == nil {
		t.Fatal("discarded late accounting must be surfaced as an error")
	}
}

type silentObservableProvider struct{}

func (silentObservableProvider) ObservesProviderAttempts() {}
func (silentObservableProvider) Complete(context.Context, llm.CompletionReq) (llm.CompletionResp, error) {
	return llm.CompletionResp{Text: "ignored"}, nil
}

type providerStub struct {
	resp llm.CompletionResp
	err  error
}

func (p providerStub) Complete(context.Context, llm.CompletionReq) (llm.CompletionResp, error) {
	return p.resp, p.err
}

type storeStub struct {
	starts                 []db.CreateAICallRecordParams
	skips                  []db.CreateSkippedAICallRecordParams
	finishes               []db.FinishAICallRecordParams
	rows                   map[uuid.UUID]db.AiCallRecord
	startIDs               []uuid.UUID
	fenceFailures          int
	preserveTerminalStatus bool
	discardAccounting      bool
}

func (s *storeStub) CreateAICallRecord(_ context.Context, arg db.CreateAICallRecordParams) (db.AiCallRecord, error) {
	s.starts = append(s.starts, arg)
	row := db.AiCallRecord{ID: uuid.New(), ProjectID: arg.ProjectID, Status: arg.Status, RequestFingerprint: arg.RequestFingerprint, ParentCallID: arg.ParentCallID, Provider: arg.Provider, Model: arg.Model, ProviderCalled: true}
	if s.rows == nil {
		s.rows = map[uuid.UUID]db.AiCallRecord{}
	}
	s.rows[row.ID] = row
	s.startIDs = append(s.startIDs, row.ID)
	return row, nil
}

func (s *storeStub) CreateSkippedAICallRecord(_ context.Context, arg db.CreateSkippedAICallRecordParams) (db.AiCallRecord, error) {
	s.skips = append(s.skips, arg)
	return db.AiCallRecord{ID: uuid.New(), ProjectID: arg.ProjectID, Status: "skipped", ErrorCode: arg.ErrorCode}, nil
}

func (s *storeStub) FinishAICallRecord(_ context.Context, arg db.FinishAICallRecordParams) (db.AiCallRecord, error) {
	s.finishes = append(s.finishes, arg)
	row, ok := s.rows[arg.ID]
	if !ok {
		return db.AiCallRecord{}, errors.New("missing call")
	}
	row.Status = arg.Status
	row.TotalTokens = arg.TotalTokens
	s.rows[arg.ID] = row
	return row, nil
}

func (s *storeStub) FinishCanonicalAICallFenced(_ context.Context, arg db.FinishCanonicalAICallFencedParams) (db.AiCallRecord, error) {
	if s.fenceFailures > 0 {
		s.fenceFailures--
		return db.AiCallRecord{}, errors.New("transient finish failure")
	}
	row, ok := s.rows[arg.ID]
	if !ok {
		return db.AiCallRecord{}, errors.New("missing call")
	}
	s.finishes = append(s.finishes, db.FinishAICallRecordParams{
		Status: arg.Status, ResolvedProvider: arg.ResolvedProvider, ResolvedModel: arg.ResolvedModel,
		ErrorCode: arg.ErrorCode, PromptTokens: arg.PromptTokens, CompletionTokens: arg.CompletionTokens,
		TotalTokens: arg.TotalTokens, CostUsd: arg.CostUsd, ID: arg.ID, ProjectID: arg.ProjectID,
	})
	if !s.preserveTerminalStatus || row.Status == "running" {
		row.Status, row.ErrorCode = arg.Status, arg.ErrorCode
	}
	if !s.discardAccounting {
		row.PromptTokens, row.CompletionTokens, row.TotalTokens, row.CostUsd = arg.PromptTokens, arg.CompletionTokens, arg.TotalTokens, arg.CostUsd
	}
	if arg.ResolvedProvider != nil {
		row.Provider = *arg.ResolvedProvider
	}
	if arg.ResolvedModel != nil {
		row.Model = *arg.ResolvedModel
	}
	s.rows[row.ID] = row
	return row, nil
}

func (s *storeStub) rowsByStart(index int) db.AiCallRecord { return s.rows[s.startIDs[index]] }

func (s *storeStub) ReclassifyAICallRecordOutputFailure(_ context.Context, arg db.ReclassifyAICallRecordOutputFailureParams) (db.AiCallRecord, error) {
	row, ok := s.rows[arg.ID]
	if !ok {
		return db.AiCallRecord{}, errors.New("missing call")
	}
	row.Status = "failed"
	row.ErrorCode = arg.ErrorCode
	s.rows[arg.ID] = row
	return row, nil
}

func (s *storeStub) GetAICallRecord(_ context.Context, arg db.GetAICallRecordParams) (db.AiCallRecord, error) {
	row, ok := s.rows[arg.ID]
	if !ok {
		return db.AiCallRecord{}, errors.New("missing call")
	}
	return row, nil
}

func (s *storeStub) MarkAICallProviderStarted(_ context.Context, arg db.MarkAICallProviderStartedParams) (db.AiCallRecord, error) {
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

func (s *storeStub) GetLatestAICallForRequest(context.Context, db.GetLatestAICallForRequestParams) (db.AiCallRecord, error) {
	return db.AiCallRecord{}, errors.New("missing call")
}

func (s *storeStub) AggregateAICallsForObject(context.Context, db.AggregateAICallsForObjectParams) (db.AggregateAICallsForObjectRow, error) {
	return db.AggregateAICallsForObjectRow{}, nil
}

func testSpec() Spec {
	return Spec{
		ProjectID: uuid.New(), Stage: "content_generation", LinkedObjectType: "topic", LinkedObjectID: uuid.New(),
		Provider: "planned", Model: "planned", PromptVersion: "writer-v1", RequestFingerprint: "sha256:fixture",
	}
}

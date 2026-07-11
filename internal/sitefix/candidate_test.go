package sitefix

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
)

func TestCandidateMaterializerCreatesOneCanonicalDoctorCandidate(t *testing.T) {
	projectID, findingID, candidateID := uuid.New(), uuid.New(), uuid.New()
	store := &candidateStoreStub{candidateID: candidateID}
	materializer := newCandidateMaterializer(store)
	finding := canonicalFinding(projectID, findingID)

	got, err := materializer.Materialize(context.Background(), finding)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if got.ID != candidateID || got.RunID == uuid.Nil {
		t.Fatalf("materialized identity = %+v", got)
	}
	if store.ensureRunCalls != 1 || store.saveCalls != 1 || store.completeRunCalls != 1 {
		t.Fatalf("run/candidate/complete writes = %d/%d/%d", store.ensureRunCalls, store.saveCalls, store.completeRunCalls)
	}
	if store.mode != "canonical" {
		t.Fatalf("run mode = %q", store.mode)
	}
	if store.candidate.Status != discovery.StatusIdentityReady || store.candidate.SuggestedOwner != discovery.OwnerDoctor {
		t.Fatalf("candidate = %+v", store.candidate)
	}
	if store.candidate.CandidateSchemaVersion != discovery.CandidateSchemaVersionV1 ||
		store.candidate.SignatureVersion != discovery.SignatureVersionV1 {
		t.Fatalf("candidate versions = %q/%q", store.candidate.CandidateSchemaVersion, store.candidate.SignatureVersion)
	}
	if len(store.identity.ConflictBucketKeys) == 0 || len(store.buckets) != len(store.identity.ConflictBucketKeys) {
		t.Fatalf("identity buckets = %v, materialized = %v", store.identity.ConflictBucketKeys, store.buckets)
	}

	second, err := materializer.Materialize(context.Background(), finding)
	if err != nil {
		t.Fatalf("repeat Materialize: %v", err)
	}
	if second.RunID != got.RunID || second.ID != got.ID {
		t.Fatalf("repeat materialization changed identity: first=%+v second=%+v", got, second)
	}
}

func TestCandidateMaterializerFailsClosedForIncompleteOrHealthyFinding(t *testing.T) {
	tests := []struct {
		name    string
		finding db.SeoDoctorFinding
		wantErr error
	}{
		{name: "healthy", finding: func() db.SeoDoctorFinding {
			f := canonicalFinding(uuid.New(), uuid.New())
			f.FindingKind = "healthy"
			return f
		}(), wantErr: ErrHealthyFinding},
		{name: "incomplete", finding: func() db.SeoDoctorFinding {
			f := canonicalFinding(uuid.New(), uuid.New())
			f.IssueType = "unknown_future_detector"
			return f
		}(), wantErr: ErrIncompleteCandidate},
		{name: "empty evidence object", finding: func() db.SeoDoctorFinding {
			f := canonicalFinding(uuid.New(), uuid.New())
			f.Evidence = json.RawMessage(`{}`)
			return f
		}(), wantErr: ErrIncompleteCandidate},
		{name: "missing evidence id", finding: func() db.SeoDoctorFinding {
			f := canonicalFinding(uuid.New(), uuid.New())
			f.RunID = uuid.Nil
			return f
		}(), wantErr: ErrIncompleteCandidate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &candidateStoreStub{candidateID: uuid.New()}
			_, err := newCandidateMaterializer(store).Materialize(context.Background(), tt.finding)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if store.ensureRunCalls != 0 || store.saveCalls != 0 || len(store.buckets) != 0 {
				t.Fatal("invalid finding caused canonical candidate writes")
			}
		})
	}
}

func TestCandidateMaterializerSnapshotIdentityIsStableAndComplete(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	finding := canonicalFinding(projectID, findingID)
	firstStore := &candidateStoreStub{candidateID: uuid.New()}
	first, err := newCandidateMaterializer(firstStore).Materialize(context.Background(), finding)
	if err != nil {
		t.Fatal(err)
	}
	retryStore := &candidateStoreStub{candidateID: first.ID}
	retry, err := newCandidateMaterializer(retryStore).Materialize(context.Background(), finding)
	if err != nil {
		t.Fatal(err)
	}
	if retry.RunID != first.RunID || retry.ID != first.ID {
		t.Fatalf("same snapshot changed identity: first=%+v retry=%+v", first, retry)
	}

	mutations := []func(*db.SeoDoctorFinding){
		func(f *db.SeoDoctorFinding) { f.FixIntent += " revised" },
		func(f *db.SeoDoctorFinding) { f.DeveloperInstructions += " revised" },
		func(f *db.SeoDoctorFinding) { f.LikelyFilesOrSurfaces = json.RawMessage(`["other.tsx"]`) },
		func(f *db.SeoDoctorFinding) { f.AcceptanceTests = json.RawMessage(`[{"type":"canonical_present"}]`) },
		func(f *db.SeoDoctorFinding) { f.Evidence = json.RawMessage(`{"canonical":"wrong"}`) },
	}
	for i, mutate := range mutations {
		changed := finding
		mutate(&changed)
		store := &candidateStoreStub{candidateID: uuid.New()}
		got, err := newCandidateMaterializer(store).Materialize(context.Background(), changed)
		if err != nil {
			t.Fatalf("mutation %d: %v", i, err)
		}
		if got.RunID == first.RunID || got.ID == first.ID {
			t.Fatalf("mutation %d reused old snapshot identity", i)
		}
	}
}

func TestCandidateMaterializerCompletionFailureLeavesCandidateInvisible(t *testing.T) {
	finding := canonicalFinding(uuid.New(), uuid.New())
	store := &candidateStoreStub{candidateID: uuid.New(), completeErr: errors.New("completion failed")}
	_, err := newCandidateMaterializer(store).Materialize(context.Background(), finding)
	if err == nil || !strings.Contains(err.Error(), "completion failed") {
		t.Fatalf("error = %v", err)
	}
	if store.saveCalls != 1 || store.completeRunCalls != 1 {
		t.Fatalf("save/complete calls = %d/%d", store.saveCalls, store.completeRunCalls)
	}
	// The database contract separately requires arbitration reads to join a
	// completed canonical run, so this persisted candidate remains invisible.
}

func TestCandidateMaterializerDoesNotPublishCandidateBeforeBucketsExist(t *testing.T) {
	finding := canonicalFinding(uuid.New(), uuid.New())
	store := &candidateStoreStub{candidateID: uuid.New(), bucketErr: errors.New("bucket unavailable")}

	_, err := newCandidateMaterializer(store).Materialize(context.Background(), finding)
	if err == nil || !strings.Contains(err.Error(), "bucket unavailable") {
		t.Fatalf("error = %v", err)
	}
	if store.saveCalls != 0 || store.completeRunCalls != 0 {
		t.Fatalf("candidate became visible before buckets: saves=%d completes=%d", store.saveCalls, store.completeRunCalls)
	}
	if got := strings.Join(store.operations, ","); got != "ensure_run,ensure_bucket" {
		t.Fatalf("operation order = %q", got)
	}
}

type candidateStoreStub struct {
	candidateID      uuid.UUID
	ensureRunCalls   int
	saveCalls        int
	mode             string
	runID            uuid.UUID
	candidate        discovery.Candidate
	identity         discovery.Identity
	buckets          []string
	completeRunCalls int
	bucketErr        error
	operations       []string
	completeErr      error
}

func (s *candidateStoreStub) EnsureRun(_ context.Context, runID, _ uuid.UUID, mode string) error {
	s.ensureRunCalls++
	s.operations = append(s.operations, "ensure_run")
	s.mode = mode
	if s.runID == uuid.Nil {
		s.runID = runID
	} else if s.runID != runID {
		return errors.New("run id changed")
	}
	return nil
}

func (s *candidateStoreStub) SaveCandidate(_ context.Context, runID uuid.UUID, candidate discovery.Candidate, identity discovery.Identity) (uuid.UUID, error) {
	s.saveCalls++
	s.operations = append(s.operations, "save_candidate")
	if runID != s.runID {
		return uuid.Nil, errors.New("wrong run")
	}
	s.candidate, s.identity = candidate, identity
	return s.candidateID, nil
}

func (s *candidateStoreStub) EnsureBucket(_ context.Context, _ uuid.UUID, bucket string) error {
	s.buckets = append(s.buckets, bucket)
	s.operations = append(s.operations, "ensure_bucket")
	return s.bucketErr
}

func (s *candidateStoreStub) CompleteRun(_ context.Context, runID, _ uuid.UUID) error {
	if runID != s.runID {
		return errors.New("wrong run")
	}
	s.completeRunCalls++
	s.operations = append(s.operations, "complete_run")
	return s.completeErr
}

func canonicalFinding(projectID, findingID uuid.UUID) db.SeoDoctorFinding {
	return db.SeoDoctorFinding{
		ID: findingID, ProjectID: projectID, RunID: uuid.New(), FindingKey: "canonical-missing",
		IssueType: "canonical_missing", Status: "active", FindingKind: "broken",
		NormalizedUrls:        json.RawMessage(`["https://example.com/pricing"]`),
		AffectedUrls:          json.RawMessage(`["https://example.com/pricing"]`),
		Evidence:              json.RawMessage(`{"canonical":null,"url":"https://example.com/pricing"}`),
		FixIntent:             "Add a self-referencing canonical",
		DeveloperInstructions: "Set rel=canonical in the document head.",
		LikelyFilesOrSurfaces: json.RawMessage(`["app/pricing/page.tsx"]`),
		AcceptanceTests:       json.RawMessage(`[{"type":"canonical_equals","expected":"https://example.com/pricing"}]`),
	}
}

package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

func TestShadowServiceProjectsAndReportsWithoutEnforcement(t *testing.T) {
	projectID := uuid.New()
	target := "https://example.com/pricing"
	repo := newMemoryRepository()
	repo.doctor = []db.SeoDoctorFinding{{
		ID:             uuid.New(),
		ProjectID:      projectID,
		IssueType:      "structured_data_missing",
		NormalizedUrls: json.RawMessage(`["https://example.com/pricing"]`),
	}, {
		ID:             uuid.New(),
		ProjectID:      projectID,
		IssueType:      "title_duplicate",
		NormalizedUrls: json.RawMessage(`["https://example.com/pricing"]`),
	}}
	repo.opportunities = []db.SeoOpportunity{
		{ID: uuid.New(), ProjectID: projectID, Type: "schema_gap", NormalizedPageUrl: target},
		{ID: uuid.New(), ProjectID: projectID, Type: "low_ctr", NormalizedPageUrl: target},
		{ID: uuid.New(), ProjectID: projectID, Type: "unknown_future_detector"},
	}

	service := NewService(repo)
	report, err := service.RunProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("RunProject: %v", err)
	}
	if report.Mode != "shadow" || report.Status != "completed" {
		t.Fatalf("run mode/status = %s/%s", report.Mode, report.Status)
	}
	if report.DoctorCandidates != 2 || report.OpportunityCandidates != 3 {
		t.Fatalf("source counts = %d/%d", report.DoctorCandidates, report.OpportunityCandidates)
	}
	if report.IdentityReady != 4 || report.NeedsSpecification != 1 {
		t.Fatalf("identity counts = %d/%d", report.IdentityReady, report.NeedsSpecification)
	}
	if report.ExactDuplicateGroups != 1 {
		t.Fatalf("exact duplicate groups = %d, want 1", report.ExactDuplicateGroups)
	}
	if report.PossibleConflictGroups != 1 {
		t.Fatalf("possible conflict groups = %d, want 1", report.PossibleConflictGroups)
	}
	if len(repo.candidates) != 5 || len(repo.signatures) != 4 {
		t.Fatalf("stored candidates/signatures = %d/%d", len(repo.candidates), len(repo.signatures))
	}
	for _, signature := range repo.signatures {
		if signature.Mode != "shadow" || signature.Active {
			t.Fatalf("signature enforcement leaked: %+v", signature)
		}
	}
}

func TestShadowServiceRerunPreservesPerRunProvenance(t *testing.T) {
	projectID := uuid.New()
	repo := newMemoryRepository()
	repo.opportunities = []db.SeoOpportunity{{
		ID:                uuid.New(),
		ProjectID:         projectID,
		Type:              "low_ctr",
		NormalizedPageUrl: "https://example.com/pricing",
	}}
	service := NewService(repo)
	if _, err := service.RunProject(context.Background(), projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RunProject(context.Background(), projectID); err != nil {
		t.Fatal(err)
	}
	if len(repo.candidates) != 2 || len(repo.signatures) != 2 {
		t.Fatalf("rerun did not preserve both snapshots: %d/%d", len(repo.candidates), len(repo.signatures))
	}
	runIDs := map[uuid.UUID]bool{}
	for _, candidate := range repo.candidates {
		runIDs[candidate.RunID] = true
	}
	if len(runIDs) != 2 {
		t.Fatalf("candidate provenance run IDs = %v, want 2 distinct runs", runIDs)
	}
}

type memoryRepository struct {
	doctor        []db.SeoDoctorFinding
	opportunities []db.SeoOpportunity
	runs          map[uuid.UUID]Report
	candidates    map[string]storedCandidate
	signatures    map[string]ShadowSignature
}

type storedCandidate struct {
	ID        uuid.UUID
	RunID     uuid.UUID
	Candidate Candidate
	Identity  *Identity
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		runs:       map[uuid.UUID]Report{},
		candidates: map[string]storedCandidate{},
		signatures: map[string]ShadowSignature{},
	}
}

func (r *memoryRepository) CreateRun(_ context.Context, projectID uuid.UUID) (Report, error) {
	run := Report{RunID: uuid.New(), ProjectID: projectID, Mode: "shadow", Status: "running", CreatedAt: time.Now().UTC()}
	r.runs[run.RunID] = run
	return run, nil
}

func (r *memoryRepository) ListDoctorFindings(_ context.Context, _ uuid.UUID) ([]db.SeoDoctorFinding, error) {
	return r.doctor, nil
}

func (r *memoryRepository) ListOpportunities(_ context.Context, _ uuid.UUID) ([]db.SeoOpportunity, error) {
	return r.opportunities, nil
}

func (r *memoryRepository) SaveCandidate(_ context.Context, runID uuid.UUID, candidate Candidate, identity *Identity) (uuid.UUID, error) {
	key := fmt.Sprintf("%s|%s|%s|%s|%s", runID, candidate.ProjectID, candidate.SourceKind, candidate.SourceObjectID, candidate.CandidateSchemaVersion)
	id := uuid.New()
	if existing, ok := r.candidates[key]; ok {
		id = existing.ID
	}
	r.candidates[key] = storedCandidate{ID: id, RunID: runID, Candidate: candidate, Identity: identity}
	if identity == nil {
		delete(r.signatures, key)
	}
	return id, nil
}

func (r *memoryRepository) SaveShadowSignature(_ context.Context, signature ShadowSignature) error {
	key := fmt.Sprintf("%s|%s|%s|%s|%s", signature.RunID, signature.ProjectID, signature.SourceKind, signature.SourceObjectID, CandidateSchemaVersionV1)
	r.signatures[key] = signature
	return nil
}

func (r *memoryRepository) CompleteRun(_ context.Context, report Report) (Report, error) {
	report.Status = "completed"
	report.FinishedAt = time.Now().UTC()
	r.runs[report.RunID] = report
	return report, nil
}

func (r *memoryRepository) FailRun(_ context.Context, report Report, runErr error) error {
	report.Status = "failed"
	report.Error = runErr.Error()
	r.runs[report.RunID] = report
	return nil
}

func (r *memoryRepository) LatestRun(_ context.Context, projectID uuid.UUID) (Report, error) {
	var latest Report
	for _, run := range r.runs {
		if run.ProjectID == projectID && run.CreatedAt.After(latest.CreatedAt) {
			latest = run
		}
	}
	if latest.RunID == uuid.Nil {
		return Report{}, fmt.Errorf("not found")
	}
	return latest, nil
}

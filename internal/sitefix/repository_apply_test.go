package sitefix

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

func TestCanonicalApplyUsesSeparateSourceSelectionAndPatchGenerationCalls(t *testing.T) {
	fix := groundedOptimizationFix()
	fix.Status = "approved"
	store := &applyStoreStub{fix: fix}
	loader := &repositoryLoaderStub{
		target:     RepositoryTarget{Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit-1"},
		candidates: []RepositorySourceCandidate{{Path: "app/page.tsx", SHA: "blob-1", Size: 31}},
		snapshot:   RepositorySnapshot{Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit-1", Sources: []RepositorySource{{Path: "app/page.tsx", SHA: "blob-1", Content: "const title = 'Old title'\n"}}},
	}
	selector := &repositorySelectorStub{store: store, paths: []string{"app/page.tsx"}}
	generator := &repositoryGeneratorStub{store: store, plan: ApplicationPlan{
		TargetURL: "https://example.com/product", NormalizedTargetURL: "https://example.com/product", OpportunityKey: "doctor:" + fix.ID.String(), Status: "ready_for_pr",
		SourceFilePaths: json.RawMessage(`["app/page.tsx"]`), PatchSnapshot: json.RawMessage(`{"repo":"acme/site","base_branch":"main","base_commit_sha":"commit-1","files":[{"path":"app/page.tsx","base_sha":"blob-1"}]}`),
		DiffSnapshot: json.RawMessage(`{"files":[{"path":"app/page.tsx","changes":[{"before":"Old title","after":"New title"}]}]}`), ResolutionCriteria: json.RawMessage(`{"acceptance_tests":[]}`),
	}}
	verifier := &patchVerifierStub{store: store, verification: PatchVerification{Approved: true, PrimaryIntentPreserved: true, PreservedPropositions: []string{"Existing supported fact."}}}

	_, err := (ApplyService{Store: store, SourceLoader: loader, SourceSelector: selector, Generator: generator, Verifier: verifier}).Apply(context.Background(), fix.ProjectID, fix.ID)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(store.events, ",")
	for _, ordered := range []string{"start_selector", "selector", "finish_selector:ok", "start_ai", "provider", "finish_ai:ok", "start_verifier", "verifier", "finish_verifier:ok", "finalize"} {
		if !strings.Contains(joined, ordered) {
			t.Fatalf("missing %q in events %v", ordered, store.events)
		}
	}
	if loader.loadedTarget != loader.target || len(loader.loadedPaths) != 1 || loader.loadedPaths[0] != "app/page.tsx" {
		t.Fatalf("loader target=%+v paths=%#v", loader.loadedTarget, loader.loadedPaths)
	}
	if store.selectionCallID == uuid.Nil || store.generationCausedBy != store.selectionCallID {
		t.Fatalf("generation causal link selection=%s causedBy=%s", store.selectionCallID, store.generationCausedBy)
	}
}

func TestLLMApplicationGeneratorRequiresRepositoryPatchAndComputesActualArtifacts(t *testing.T) {
	provider := &groundingProviderStub{response: `{
		"files":[{"path":"app/page.tsx","base_sha":"blob-1","replacements":[{"old_text":"Old title","new_text":"New title"}]}]
	}`}
	fix := groundedOptimizationFix()
	contextSnapshot := GenerationContext{ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`), ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot}
	repository := RepositorySnapshot{Repo: "acme/site", Branch: "release/site", BaseCommitSHA: "commit-1", Sources: []RepositorySource{{Path: "app/page.tsx", SHA: "blob-1", Content: "export const title = 'Old title'\n"}}}

	plan, result, err := (LLMApplicationGenerator{Provider: provider, Model: "test-model"}).Generate(context.Background(), fix, contextSnapshot, repository, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || plan.Status != "ready_for_pr" {
		t.Fatalf("plan=%+v result=%+v", plan, result)
	}
	for _, grounded := range []string{"app/page.tsx", "blob-1", "Old title"} {
		if !strings.Contains(provider.request.Prompt, grounded) {
			t.Fatalf("generator prompt omitted %q: %s", grounded, provider.request.Prompt)
		}
	}
	for _, actual := range []struct {
		artifact json.RawMessage
		want     string
	}{
		{plan.PatchSnapshot, `"base_commit_sha":"commit-1"`},
		{plan.PatchSnapshot, `"result_content_sha256"`},
		{plan.DiffSnapshot, `"before":"Old title"`},
		{plan.DiffSnapshot, `"after":"New title"`},
	} {
		if !strings.Contains(string(actual.artifact), actual.want) {
			t.Fatalf("artifact %s missing %s", actual.artifact, actual.want)
		}
	}
}

type repositoryLoaderStub struct {
	target       RepositoryTarget
	candidates   []RepositorySourceCandidate
	snapshot     RepositorySnapshot
	loadedTarget RepositoryTarget
	loadedPaths  []string
}

func (l *repositoryLoaderStub) Candidates(context.Context, db.SiteFix) (RepositoryTarget, []RepositorySourceCandidate, error) {
	return l.target, l.candidates, nil
}

func (l *repositoryLoaderStub) LoadSelected(_ context.Context, target RepositoryTarget, paths []string) (RepositorySnapshot, error) {
	l.loadedTarget = target
	l.loadedPaths = append([]string(nil), paths...)
	return l.snapshot, nil
}

type repositorySelectorStub struct {
	store *applyStoreStub
	paths []string
}

func (*repositorySelectorStub) Describe(db.SiteFix, []RepositorySourceCandidate) GenerationCall {
	return GenerationCall{Provider: "test", Model: "selector", PromptVersion: "selector-v1", RequestFingerprint: "selector-fingerprint"}
}

func (s *repositorySelectorStub) Select(ctx context.Context, _ db.SiteFix, _ []RepositorySourceCandidate, attempt siteFixAICallAttempt) ([]string, GenerationResult, error) {
	_, _ = attempt.StartAttempt(ctx, "selector")
	s.store.events = append(s.store.events, "selector")
	return append([]string(nil), s.paths...), GenerationResult{Provider: "test", Model: "selector", Status: "ok"}, nil
}

type repositoryGeneratorStub struct {
	store *applyStoreStub
	plan  ApplicationPlan
}

func (*repositoryGeneratorStub) Describe(db.SiteFix, GenerationContext, RepositorySnapshot) GenerationCall {
	return GenerationCall{Provider: "test", Model: "generator", PromptVersion: "generator-v1", RequestFingerprint: "generator-fingerprint"}
}

func (g *repositoryGeneratorStub) Generate(ctx context.Context, fix db.SiteFix, generationContext GenerationContext, _ RepositorySnapshot, attempt siteFixAICallAttempt) (ApplicationPlan, GenerationResult, error) {
	_, _ = attempt.StartAttempt(ctx, "generator")
	g.store.events = append(g.store.events, "provider")
	plan := g.plan
	var err error
	if len(plan.GroundingSnapshot) == 0 {
		plan.GroundingSnapshot, err = approvedGroundingSnapshot(fix, generationContext)
	}
	return plan, GenerationResult{Provider: "test", Model: "generator", Status: "ok"}, err
}

package sitefix

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
)

func TestRankRepositorySourceCandidatesFiltersUnsafePathsAndRanksFindingFamilies(t *testing.T) {
	candidates := []RepositorySourceCandidate{
		{Path: "app/sitemap.ts", SHA: "sitemap", Size: 120},
		{Path: "app/layout.tsx", SHA: "layout", Size: 120},
		{Path: "lib/schema.ts", SHA: "schema", Size: 120},
		{Path: "components/navigation.tsx", SHA: "nav", Size: 120},
		{Path: "public/robots.txt", SHA: "robots", Size: 120},
		{Path: "app/page.tsx", SHA: "page", Size: 120},
		{Path: "node_modules/pkg/index.js", SHA: "dep", Size: 120},
		{Path: "vendor/pkg/file.go", SHA: "vendor", Size: 120},
		{Path: "dist/generated.js", SHA: "generated", Size: 120},
		{Path: ".github/workflows/deploy.yml", SHA: "workflow", Size: 120},
		{Path: ".env", SHA: "secret", Size: 120},
		{Path: "package-lock.json", SHA: "lock", Size: 120},
		{Path: "public/logo.png", SHA: "binary", Size: 120},
	}
	tests := []struct {
		family string
		want   string
	}{
		{"sitemap missing URL", "app/sitemap.ts"},
		{"canonical tag incorrect", "app/layout.tsx"},
		{"schema structured data", "lib/schema.ts"},
		{"internal-link navigation", "components/navigation.tsx"},
		{"robots directive", "public/robots.txt"},
		{"metadata title description", "app/page.tsx"},
	}
	for _, tc := range tests {
		t.Run(tc.family, func(t *testing.T) {
			fix := db.SiteFix{FindingKind: tc.family, TargetUrls: json.RawMessage(`["https://example.com/products/widget"]`), ProposedFix: json.RawMessage(`{"intent":"` + tc.family + `"}`)}
			ranked, err := RankRepositorySourceCandidates(fix, candidates, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(ranked) == 0 || ranked[0].Path != tc.want {
				t.Fatalf("ranked[0] = %#v, want %q; all=%#v", ranked, tc.want, ranked)
			}
			for _, got := range ranked {
				for _, unsafe := range []string{"node_modules", "vendor/", "dist/", ".github/workflows", ".env", "package-lock", ".png"} {
					if strings.Contains(got.Path, unsafe) {
						t.Fatalf("unsafe candidate survived: %q", got.Path)
					}
				}
			}
		})
	}
	if _, err := RankRepositorySourceCandidates(db.SiteFix{}, candidates, true); err == nil {
		t.Fatal("truncated repository tree was accepted")
	}
}

// Regression: a sitemap fix in a repository where hundreds of paths match the
// generic target-URL tokens and metadata hints must still surface the sitemap
// source within the bounded candidate list, or the generator can never patch it.
func TestRankRepositorySourceCandidatesKeepsIntentMatchInNoisyRepository(t *testing.T) {
	candidates := []RepositorySourceCandidate{{Path: "dashboard/src/app/sitemap.ts", SHA: "sitemap", Size: 900}}
	for i := 0; i < MaxRepositorySourceCandidates+100; i++ {
		// Every noisy path matches URL tokens ("api", "publishing") and the
		// metadata family hint ("page"), like a large marketing/docs site.
		candidates = append(candidates, RepositorySourceCandidate{
			Path: fmt.Sprintf("dashboard/src/app/docs/api/publishing/section-%03d/page.tsx", i), SHA: fmt.Sprintf("noise-%03d", i), Size: 900,
		})
	}
	fix := db.SiteFix{
		FindingKind: "broken",
		TargetUrls:  json.RawMessage(`["https://example.com/blog/evidence-led-social-publishing-api-planning-brief"]`),
		ProposedFix: json.RawMessage(`{"fix_intent":"Include the canonical URL in the sitemap.","mutations":[{"field":"sitemap_entry","operation":"add"}]}`),
		EvidenceSnapshot: json.RawMessage(`{"finding":{"title":"Sitemap missing canonical URL","description":"The page is absent from the sitemap."}}`),
	}
	ranked, err := RankRepositorySourceCandidates(fix, candidates, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked) != MaxRepositorySourceCandidates {
		t.Fatalf("ranked = %d candidates, want the %d cap", len(ranked), MaxRepositorySourceCandidates)
	}
	for _, candidate := range ranked {
		if candidate.Path == "dashboard/src/app/sitemap.ts" {
			return
		}
	}
	t.Fatalf("sitemap source was pushed out of the bounded candidate list; top=%q", ranked[0].Path)
}

func TestLLMRepositorySourceSelectorIntersectsSafeSetAndSendsMetadataOnly(t *testing.T) {
	provider := &repositorySelectorProvider{response: `{"paths":["app/page.tsx","../../.env","unknown.ts","app/page.tsx","app/layout.tsx"]}`}
	selector := LLMRepositorySourceSelector{Provider: provider, Model: "selector-test"}
	fix := db.SiteFix{FindingKind: "canonical", TargetUrls: json.RawMessage(`["https://example.com/product"]`), ProposedFix: json.RawMessage(`{"intent":"set canonical"}`)}
	candidates := []RepositorySourceCandidate{
		{Path: "app/page.tsx", SHA: "secret-sha-1", Size: 120},
		{Path: "app/layout.tsx", SHA: "secret-sha-2", Size: 180},
	}
	attempt := &repositorySelectorAttempt{}
	paths, result, err := selector.Select(context.Background(), fix, candidates, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || len(paths) != 2 || paths[0] != "app/page.tsx" || paths[1] != "app/layout.tsx" {
		t.Fatalf("paths=%#v result=%+v", paths, result)
	}
	if !attempt.started {
		t.Fatal("selector did not report a physical provider attempt")
	}
	if strings.Contains(provider.request.Prompt, "secret-sha") || strings.Contains(provider.request.Prompt, "source content") {
		t.Fatalf("selector prompt leaked source data: %s", provider.request.Prompt)
	}
	if !strings.Contains(provider.request.Prompt, `"path":"app/page.tsx"`) || !strings.Contains(provider.request.Prompt, `"size":120`) {
		t.Fatalf("selector prompt omitted safe metadata: %s", provider.request.Prompt)
	}
}

func TestLLMRepositorySourceSelectorRejectsEmptyOrOverBudgetSelection(t *testing.T) {
	fix := db.SiteFix{FindingKind: "robots", TargetUrls: json.RawMessage(`["https://example.com/robots.txt"]`)}
	for _, tc := range []struct {
		name       string
		response   string
		candidates []RepositorySourceCandidate
	}{
		{name: "unknown only", response: `{"paths":["unknown"]}`, candidates: []RepositorySourceCandidate{{Path: "public/robots.txt", SHA: "s", Size: 20}}},
		{name: "over total budget", response: `{"paths":["a.ts","b.ts","c.ts","d.ts","e.ts"]}`, candidates: []RepositorySourceCandidate{
			{Path: "a.ts", SHA: "a", Size: MaxRepositorySourceFileBytes}, {Path: "b.ts", SHA: "b", Size: MaxRepositorySourceFileBytes},
			{Path: "c.ts", SHA: "c", Size: MaxRepositorySourceFileBytes}, {Path: "d.ts", SHA: "d", Size: MaxRepositorySourceFileBytes},
			{Path: "e.ts", SHA: "e", Size: 1},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selector := LLMRepositorySourceSelector{Provider: &repositorySelectorProvider{response: tc.response}, Model: "test"}
			if _, _, err := selector.Select(context.Background(), fix, tc.candidates, &repositorySelectorAttempt{}); err == nil {
				t.Fatal("unsafe selection was accepted")
			}
		})
	}
}

func TestRepositorySourceSelectorBoundsLargeTreeMetadata(t *testing.T) {
	fix := db.SiteFix{FindingKind: "canonical metadata", TargetUrls: json.RawMessage(`["https://example.com/products/widget"]`), ProposedFix: json.RawMessage(`{"field":"canonical"}`)}
	candidates := make([]RepositorySourceCandidate, 0, 1001)
	for i := 0; i < 1000; i++ {
		candidates = append(candidates, RepositorySourceCandidate{Path: fmt.Sprintf("src/ordinary/file-%04d.ts", i), SHA: fmt.Sprintf("blob-%04d", i), Size: 100})
	}
	candidates = append(candidates, RepositorySourceCandidate{Path: "app/products/widget/layout.tsx", SHA: "highest-ranked", Size: 120})
	ranked, err := RankRepositorySourceCandidates(fix, candidates, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked) > MaxRepositorySourceCandidates || ranked[0].Path != "app/products/widget/layout.tsx" {
		t.Fatalf("ranked count=%d first=%#v", len(ranked), ranked[0])
	}
	request := (LLMRepositorySourceSelector{Model: "test"}).completionRequest(fix, ranked)
	jsonStart := strings.LastIndex(request.Prompt, "\n")
	if jsonStart < 0 {
		t.Fatalf("selector prompt missing JSON payload: %s", request.Prompt)
	}
	var payload struct {
		Candidates json.RawMessage `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(request.Prompt[jsonStart+1:]), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Candidates) > MaxRepositoryCandidateMetadataBytes {
		t.Fatalf("candidate metadata bytes=%d limit=%d", len(payload.Candidates), MaxRepositoryCandidateMetadataBytes)
	}
	if !strings.Contains(string(payload.Candidates), `"path":"app/products/widget/layout.tsx"`) {
		t.Fatalf("highest-ranked candidate was truncated: %s", payload.Candidates)
	}
}

func TestRepositorySourceCandidateRejectsOperationalAndSensitivePaths(t *testing.T) {
	denied := []string{
		".circleci/config.yml", ".buildkite/pipeline.yml", ".gitlab/ci.yml",
		"ops/workflows/deploy.yml", "ops/pipelines/release.yaml", "azure-pipelines.yml",
		"bitbucket-pipelines.yml", ".travis.yml", "appveyor.yml", "buildspec.yml", "cloudbuild.yaml",
		"package.json", "requirements.txt", "requirements-dev.txt", "pyproject.toml", "Pipfile", "Gemfile",
		"go.mod", "Cargo.toml", "composer.json", "pom.xml", "build.gradle", "settings.gradle.kts",
		"config/service-account.json", "config/firebase.json", "config/auth-token.ts", "secrets/private-key.pem", "certs/server.crt",
		"config/serviceAccount.json", "config/privateKey.ts", "config/firebaseClient.json", "config/clientToken.ts", "certs/clientCert.ts",
		"app/page%2etsx", "src/routes.generated.ts", "src/client.gen.ts", "public/bundle.min.js",
	}
	for _, candidatePath := range denied {
		t.Run(candidatePath, func(t *testing.T) {
			if safeRepositoryCandidate(RepositorySourceCandidate{Path: candidatePath, SHA: "blob", Size: 100}) {
				t.Fatalf("sensitive/operational path was accepted: %s", candidatePath)
			}
		})
	}
	for _, candidatePath := range []string{"app/products/widget/page.tsx", "components/CertificationCard.tsx", "content/security-authors.mdx"} {
		if !safeRepositoryCandidate(RepositorySourceCandidate{Path: candidatePath, SHA: "blob", Size: 100}) {
			t.Fatalf("ordinary page source was rejected: %s", candidatePath)
		}
	}
}

type repositorySelectorProvider struct {
	request  llm.CompletionReq
	response string
}

func (p *repositorySelectorProvider) Complete(_ context.Context, req llm.CompletionReq) (llm.CompletionResp, error) {
	p.request = req
	if req.AttemptObserver != nil {
		_, _ = req.AttemptObserver.StartAttempt(context.Background(), "selector")
	}
	return llm.CompletionResp{Text: p.response, Provider: "test", Model: "selector-test"}, nil
}

type repositorySelectorAttempt struct{ started bool }

func (a *repositorySelectorAttempt) StartAttempt(context.Context, string) (string, error) {
	a.started = true
	return "attempt", nil
}
func (*repositorySelectorAttempt) FinishAttempt(context.Context, string, llm.CompletionResp, error) error {
	return nil
}
func (a *repositorySelectorAttempt) Started() bool { return a.started }

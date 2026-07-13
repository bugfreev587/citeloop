package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/sitefix"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestSiteFixRepositoryLoaderUsesConfiguredPublisherBranchAndBlobSHAs(t *testing.T) {
	projectID := uuid.New()
	client := &siteFixRepositoryClientStub{
		baseCommit: "base-commit",
		entries: []publisher.GitHubTreeEntry{
			{Path: "app/page.tsx", Mode: "100644", Type: "blob", SHA: "blob-page", Size: 33},
			{Path: "node_modules/pkg/index.js", Mode: "100644", Type: "blob", SHA: "blob-dep", Size: 30},
			{Path: ".github/workflows/deploy.yml", Mode: "100644", Type: "blob", SHA: "blob-workflow", Size: 30},
		},
		blobs: map[string][]byte{"blob-page": []byte("export const title = 'Old title'\n")},
	}
	loader := newSiteFixRepositorySourceLoader(
		func(_ context.Context, fix db.SiteFix) (resolvedSiteFixRepository, error) {
			if fix.ProjectID != projectID {
				t.Fatalf("project = %s", fix.ProjectID)
			}
			return resolvedSiteFixRepository{ConnectionID: uuid.New(), Repo: "acme/site", Branch: "release/configured", Token: "token", AuthorityFingerprint: "authority-1"}, nil
		},
		func(token, repo, branch string) siteFixRepositoryClient {
			if token != "token" || repo != "acme/site" || branch != "release/configured" {
				t.Fatalf("client target token=%q repo=%q branch=%q", token, repo, branch)
			}
			return client
		},
	)
	fix := db.SiteFix{
		ID: uuid.New(), ProjectID: projectID, FindingKind: "metadata title",
		// This target path deliberately resembles another branch. It must never
		// override the configured publisher branch.
		TargetUrls:  json.RawMessage(`["https://example.com/preview/target-branch"]`),
		ProposedFix: json.RawMessage(`{"intent":"replace title metadata"}`),
	}
	target, candidates, err := loader.Candidates(context.Background(), fix)
	if err != nil {
		t.Fatal(err)
	}
	if target.Repo != "acme/site" || target.Branch != "release/configured" || target.BaseCommitSHA != "base-commit" {
		t.Fatalf("target = %+v", target)
	}
	if client.resolvedBranch != "release/configured" || client.listedRef != "base-commit" {
		t.Fatalf("resolved branch=%q listed ref=%q", client.resolvedBranch, client.listedRef)
	}
	if len(candidates) != 1 || candidates[0].Path != "app/page.tsx" || candidates[0].SHA != "blob-page" {
		t.Fatalf("safe candidates = %#v", candidates)
	}
	snapshot, err := loader.LoadSelected(context.Background(), target, []string{"app/page.tsx"})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sources) != 1 || snapshot.Sources[0].SHA != "blob-page" || !strings.Contains(snapshot.Sources[0].Content, "Old title") {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if len(client.readSHAs) != 1 || client.readSHAs[0] != "blob-page" {
		t.Fatalf("blob reads = %#v", client.readSHAs)
	}
	if _, err := loader.LoadSelected(context.Background(), target, []string{"unknown.ts"}); err == nil {
		t.Fatal("unknown model-selected path was accepted")
	}
}

func TestResolvedSiteFixRepositoryUsesOnlyExactEnabledGitHubConfig(t *testing.T) {
	branch := "release/site"
	connectionID := uuid.New()
	connection := db.PublisherConnection{
		ID: connectionID, ProjectID: uuid.New(), Kind: publisher.ConnectionKindGitHubNextJS,
		Status: "connected", IsDefault: true, Enabled: true,
		Config:    json.RawMessage(`{"repo":"acme/site","branch":"` + branch + `","base_url":"https://example.com"}`),
		UpdatedAt: pgtype.Timestamptz{Valid: true},
	}
	resolved, err := resolvedSiteFixRepositoryFromConnection(connection, " private-token ")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ConnectionID != connectionID || resolved.Repo != "acme/site" || resolved.Branch != branch || resolved.Token != "private-token" || resolved.AuthorityFingerprint == "" {
		t.Fatalf("resolved = %+v", resolved)
	}
	connection.Enabled = false
	if _, err := resolvedSiteFixRepositoryFromConnection(connection, "token"); err == nil {
		t.Fatal("disabled connection was accepted")
	}
	connection.Enabled = true
	connection.Config = json.RawMessage(`{"repo":"acme/site","base_url":"https://example.com"}`)
	resolved, err = resolvedSiteFixRepositoryFromConnection(connection, "token")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Branch != "citeloop-content" {
		t.Fatalf("normalized default branch = %q", resolved.Branch)
	}
}

func TestExplicitSiteFixApplyUsesRepositoryAIWithoutScheduledDoctorAuthorityFallback(t *testing.T) {
	raw, err := os.ReadFile("handlers_site_fixes.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if strings.Contains(source, "service.Generator = sitefix.DeterministicApplicationGenerator") || strings.Contains(source, "service.Verifier = sitefix.DeterministicPatchGroundingVerifier") {
		t.Fatal("explicit Apply still falls back to a manual deterministic application when scheduled Doctor AI is disabled")
	}
	for _, required := range []string{"SourceLoader:", "SourceSelector:", "LLMRepositorySourceSelector"} {
		if !strings.Contains(source, required) {
			t.Fatalf("production Apply wiring missing %q", required)
		}
	}
}

func TestReadinessBoundRepositoryLoaderRetainsExactCheckedTarget(t *testing.T) {
	projectID := uuid.New()
	target := githubPRReadinessTarget{
		ConnectionID: uuid.New(), Repo: "acme/checked", Branch: "release/checked",
		credentialKind: publisher.GitHubPRCredentialGitHubApp, token: "checked-token",
		ExpectedUpdatedAt: pgtype.Timestamptz{Time: time.Unix(123, 0).UTC(), Valid: true},
	}
	loader, err := (&Server{}).siteFixRepositorySourceLoaderForReadiness(projectID, target)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := loader.resolve(context.Background(), db.SiteFix{ProjectID: projectID})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ConnectionID != target.ConnectionID || resolved.Repo != target.Repo || resolved.Branch != target.Branch || resolved.Token != "checked-token" || resolved.AuthorityFingerprint == "" {
		t.Fatalf("resolved = %+v", resolved)
	}
	if _, err := loader.resolve(context.Background(), db.SiteFix{ProjectID: uuid.New()}); err == nil {
		t.Fatal("readiness-bound loader crossed project scope")
	}
}

func TestPublisherSiteFixRepositoryClientRejectsOversizedBlobAtHTTPBoundary(t *testing.T) {
	oversized := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", sitefix.MaxRepositorySourceFileBytes+1)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sha":"blob-1","encoding":"base64","content":"` + oversized + `"}`))
	}))
	defer server.Close()
	client := &publisherSiteFixRepositoryClient{token: "token", repo: "acme/site", httpClient: server.Client(), apiBase: server.URL}
	if _, err := client.ReadBlobBounded(context.Background(), "blob-1", sitefix.MaxRepositorySourceFileBytes); err == nil {
		t.Fatal("oversized blob response was accepted")
	}
}

func TestPublisherSiteFixRepositoryClientReturnsSafeTypedGitHubStatusErrors(t *testing.T) {
	const secretBody = "ghp_do_not_log_this raw upstream body"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(secretBody))
	}))
	defer server.Close()
	client := &publisherSiteFixRepositoryClient{token: "token", repo: "acme/site", httpClient: server.Client(), apiBase: server.URL}
	for _, operation := range []string{"resolve", "blob"} {
		t.Run(operation, func(t *testing.T) {
			var err error
			switch operation {
			case "resolve":
				_, err = client.ResolveRefCommitSHA(context.Background(), "main")
			case "blob":
				_, err = client.ReadBlobBounded(context.Background(), "blob-1", 16)
			}
			var statusError interface{ StatusCode() int }
			if !errors.As(err, &statusError) || statusError.StatusCode() != http.StatusForbidden {
				t.Fatalf("error = %T %v", err, err)
			}
			if strings.Contains(err.Error(), secretBody) {
				t.Fatalf("typed error leaked upstream body: %v", err)
			}
		})
	}
}

func TestReadinessFencedRepositoryClientChecksBeforeEveryRead(t *testing.T) {
	delegate := &siteFixRepositoryClientStub{baseCommit: "base", entries: []publisher.GitHubTreeEntry{{SHA: "blob"}}, blobs: map[string][]byte{"blob": []byte("ok")}}
	checks := 0
	client := &readinessFencedSiteFixRepositoryClient{
		delegate: delegate,
		ensureCurrent: func(context.Context) error {
			checks++
			return nil
		},
	}
	if _, err := client.ResolveRefCommitSHA(context.Background(), "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListTree(context.Background(), "base"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadBlobBounded(context.Background(), "blob", 16); err != nil {
		t.Fatal(err)
	}
	if checks != 3 {
		t.Fatalf("readiness checks = %d", checks)
	}

	changed := errors.New("target changed")
	blocked := &readinessFencedSiteFixRepositoryClient{
		delegate:      delegate,
		ensureCurrent: func(context.Context) error { return changed },
	}
	delegate.resolvedBranch = ""
	if _, err := blocked.ResolveRefCommitSHA(context.Background(), "release"); !errors.Is(err, changed) {
		t.Fatalf("blocked error = %v", err)
	}
	if delegate.resolvedBranch != "" {
		t.Fatal("delegate ran after the target fence failed")
	}
}

func TestSiteFixRepositoryLoaderPreservesTypedGitHubFailuresForReadinessDowngrade(t *testing.T) {
	projectID := uuid.New()
	fix := db.SiteFix{ID: uuid.New(), ProjectID: projectID, FindingKind: "canonical", TargetUrls: json.RawMessage(`["https://example.com/"]`)}
	for _, phase := range []string{"resolve", "tree", "blob"} {
		t.Run(phase, func(t *testing.T) {
			upstream := readinessHTTPStatusError{status: http.StatusForbidden}
			client := &siteFixRepositoryClientStub{
				baseCommit: "base-commit",
				entries:    []publisher.GitHubTreeEntry{{Path: "app/page.tsx", Mode: "100644", Type: "blob", SHA: "blob-page", Size: 3}},
				blobs:      map[string][]byte{"blob-page": []byte("old")},
			}
			switch phase {
			case "resolve":
				client.resolveErr = upstream
			case "tree":
				client.listErr = upstream
			case "blob":
				client.readErr = upstream
			}
			loader := newSiteFixRepositorySourceLoader(
				func(context.Context, db.SiteFix) (resolvedSiteFixRepository, error) {
					return resolvedSiteFixRepository{ConnectionID: uuid.New(), Repo: "acme/site", Branch: "main", Token: "token"}, nil
				},
				func(string, string, string) siteFixRepositoryClient { return client },
			)
			target, candidates, err := loader.Candidates(context.Background(), fix)
			if phase == "blob" && err == nil {
				_, err = loader.LoadSelected(context.Background(), target, []string{candidates[0].Path})
			}
			var statusError interface{ StatusCode() int }
			if !errors.As(err, &statusError) || statusError.StatusCode() != http.StatusForbidden {
				t.Fatalf("error = %T %v, want preserved typed status", err, err)
			}
			if strings.Contains(err.Error(), upstream.Error()) {
				t.Fatalf("safe wrapper exposed raw upstream text: %v", err)
			}
		})
	}
}

type siteFixRepositoryClientStub struct {
	baseCommit     string
	entries        []publisher.GitHubTreeEntry
	blobs          map[string][]byte
	resolvedBranch string
	listedRef      string
	readSHAs       []string
	resolveErr     error
	listErr        error
	readErr        error
}

func (c *siteFixRepositoryClientStub) ResolveRefCommitSHA(_ context.Context, branch string) (string, error) {
	c.resolvedBranch = branch
	if c.resolveErr != nil {
		return "", c.resolveErr
	}
	return c.baseCommit, nil
}

func (c *siteFixRepositoryClientStub) ListTree(_ context.Context, ref string) ([]publisher.GitHubTreeEntry, error) {
	c.listedRef = ref
	if c.listErr != nil {
		return nil, c.listErr
	}
	return append([]publisher.GitHubTreeEntry(nil), c.entries...), nil
}

func (c *siteFixRepositoryClientStub) ReadBlobBounded(_ context.Context, sha string, _ int) ([]byte, error) {
	c.readSHAs = append(c.readSHAs, sha)
	if c.readErr != nil {
		return nil, c.readErr
	}
	return append([]byte(nil), c.blobs[sha]...), nil
}

var _ sitefix.RepositorySourceLoader = (*siteFixRepositorySourceLoader)(nil)

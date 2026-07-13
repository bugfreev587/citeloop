package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/sitefix"
	"github.com/google/uuid"
)

type resolvedSiteFixRepository struct {
	ConnectionID         uuid.UUID
	Repo                 string
	Branch               string
	Token                string
	AuthorityFingerprint string
}

type siteFixRepositoryClient interface {
	ResolveRefCommitSHA(context.Context, string) (string, error)
	ListTree(context.Context, string) ([]publisher.GitHubTreeEntry, error)
	ReadBlobBounded(context.Context, string, int) ([]byte, error)
}

type siteFixRepositoryResolver func(context.Context, db.SiteFix) (resolvedSiteFixRepository, error)
type siteFixRepositoryClientFactory func(token, repo, branch string) siteFixRepositoryClient

type siteFixRepositorySession struct {
	client     siteFixRepositoryClient
	candidates map[string]sitefix.RepositorySourceCandidate
}

type siteFixRepositorySourceLoader struct {
	resolve   siteFixRepositoryResolver
	newClient siteFixRepositoryClientFactory
	mu        sync.Mutex
	sessions  map[string]siteFixRepositorySession
}

func newSiteFixRepositorySourceLoader(resolve siteFixRepositoryResolver, newClient siteFixRepositoryClientFactory) *siteFixRepositorySourceLoader {
	return &siteFixRepositorySourceLoader{resolve: resolve, newClient: newClient, sessions: make(map[string]siteFixRepositorySession)}
}

func (l *siteFixRepositorySourceLoader) Candidates(ctx context.Context, fix db.SiteFix) (sitefix.RepositoryTarget, []sitefix.RepositorySourceCandidate, error) {
	if l == nil || l.resolve == nil || l.newClient == nil {
		return sitefix.RepositoryTarget{}, nil, errors.New("Site Fix repository source dependencies unavailable")
	}
	resolved, err := l.resolve(ctx, fix)
	if err != nil {
		return sitefix.RepositoryTarget{}, nil, err
	}
	client := l.newClient(resolved.Token, resolved.Repo, resolved.Branch)
	if client == nil {
		return sitefix.RepositoryTarget{}, nil, errors.New("Site Fix repository client unavailable")
	}
	baseCommitSHA, err := client.ResolveRefCommitSHA(ctx, resolved.Branch)
	if err != nil {
		return sitefix.RepositoryTarget{}, nil, errors.New("configured repository base branch could not be resolved")
	}
	baseCommitSHA = strings.TrimSpace(baseCommitSHA)
	if baseCommitSHA == "" {
		return sitefix.RepositoryTarget{}, nil, errors.New("configured repository base branch returned an empty commit")
	}
	entries, err := client.ListTree(ctx, baseCommitSHA)
	if err != nil {
		return sitefix.RepositoryTarget{}, nil, errors.New("configured repository tree could not be loaded")
	}
	raw := make([]sitefix.RepositorySourceCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.Type != "blob" {
			continue
		}
		raw = append(raw, sitefix.RepositorySourceCandidate{Path: entry.Path, SHA: entry.SHA, Size: entry.Size})
	}
	ranked, err := sitefix.RankRepositorySourceCandidates(fix, raw, false)
	if err != nil {
		return sitefix.RepositoryTarget{}, nil, err
	}
	target := sitefix.RepositoryTarget{Repo: resolved.Repo, Branch: resolved.Branch, BaseCommitSHA: baseCommitSHA}
	byPath := make(map[string]sitefix.RepositorySourceCandidate, len(ranked))
	for _, candidate := range ranked {
		byPath[candidate.Path] = candidate
	}
	l.mu.Lock()
	l.sessions[repositoryTargetSessionKey(target)] = siteFixRepositorySession{client: client, candidates: byPath}
	l.mu.Unlock()
	return target, ranked, nil
}

func (l *siteFixRepositorySourceLoader) LoadSelected(ctx context.Context, target sitefix.RepositoryTarget, selectedPaths []string) (sitefix.RepositorySnapshot, error) {
	if len(selectedPaths) == 0 || len(selectedPaths) > sitefix.MaxRepositorySourceFiles {
		return sitefix.RepositorySnapshot{}, fmt.Errorf("repository source selection must contain between one and %d files", sitefix.MaxRepositorySourceFiles)
	}
	l.mu.Lock()
	session, ok := l.sessions[repositoryTargetSessionKey(target)]
	l.mu.Unlock()
	if !ok || session.client == nil {
		return sitefix.RepositorySnapshot{}, errors.New("repository source target was not resolved by this preparation attempt")
	}
	snapshot := sitefix.RepositorySnapshot{Repo: target.Repo, Branch: target.Branch, BaseCommitSHA: target.BaseCommitSHA}
	seen := make(map[string]struct{}, len(selectedPaths))
	total := 0
	for _, selected := range selectedPaths {
		candidate, exists := session.candidates[selected]
		if !exists {
			return sitefix.RepositorySnapshot{}, fmt.Errorf("selected repository path %q is not a safe candidate", selected)
		}
		if _, duplicate := seen[selected]; duplicate {
			return sitefix.RepositorySnapshot{}, fmt.Errorf("selected repository path %q is duplicated", selected)
		}
		seen[selected] = struct{}{}
		content, err := session.client.ReadBlobBounded(ctx, candidate.SHA, sitefix.MaxRepositorySourceFileBytes)
		if err != nil {
			return sitefix.RepositorySnapshot{}, fmt.Errorf("selected repository blob for %q could not be read", selected)
		}
		if int64(len(content)) != candidate.Size {
			return sitefix.RepositorySnapshot{}, fmt.Errorf("selected repository blob size changed for %q", selected)
		}
		total += len(content)
		if len(content) > sitefix.MaxRepositorySourceFileBytes || total > sitefix.MaxRepositorySourceTotalBytes {
			return sitefix.RepositorySnapshot{}, errors.New("selected repository source exceeds the bounded input budget")
		}
		snapshot.Sources = append(snapshot.Sources, sitefix.RepositorySource{Path: candidate.Path, SHA: candidate.SHA, Content: string(content)})
	}
	if err := sitefix.ValidateRepositorySnapshot(snapshot); err != nil {
		return sitefix.RepositorySnapshot{}, err
	}
	return snapshot, nil
}

func repositoryTargetSessionKey(target sitefix.RepositoryTarget) string {
	return target.Repo + "\x00" + target.Branch + "\x00" + target.BaseCommitSHA
}

func resolvedSiteFixRepositoryFromConnection(connection db.PublisherConnection, token string) (resolvedSiteFixRepository, error) {
	if connection.Kind != publisher.ConnectionKindGitHubNextJS || connection.Status != "connected" || !connection.IsDefault || !connection.Enabled || connection.RevokedAt.Valid {
		return resolvedSiteFixRepository{}, errors.New("an exact enabled GitHub publisher connection is required")
	}
	cfg, err := publisher.ParseGitHubNextJSConfig(connection.Config)
	if err != nil {
		return resolvedSiteFixRepository{}, errors.New("GitHub publisher configuration is invalid")
	}
	repo, branch := strings.TrimSpace(cfg.Repo), strings.TrimSpace(cfg.Branch)
	token = strings.TrimSpace(token)
	if repo == "" || branch == "" || token == "" {
		return resolvedSiteFixRepository{}, errors.New("GitHub publisher repository, branch, and credential are required")
	}
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) != parts[0] || strings.TrimSpace(parts[1]) != parts[1] || parts[0] == "" || parts[1] == "" {
		return resolvedSiteFixRepository{}, errors.New("GitHub publisher repository must be owner/name")
	}
	fingerprintPayload := strings.Join([]string{
		connection.ID.String(), connection.ProjectID.String(), repo, branch,
		connection.UpdatedAt.Time.UTC().Format(time.RFC3339Nano), string(connection.Config),
	}, "\x00")
	fingerprintHash := sha256.Sum256([]byte(fingerprintPayload))
	return resolvedSiteFixRepository{
		ConnectionID: connection.ID, Repo: repo, Branch: branch, Token: token,
		AuthorityFingerprint: hex.EncodeToString(fingerprintHash[:]),
	}, nil
}

func (s *Server) resolveSiteFixRepository(ctx context.Context, fix db.SiteFix) (resolvedSiteFixRepository, error) {
	if s == nil || s.Q == nil {
		return resolvedSiteFixRepository{}, errors.New("publisher connection store unavailable")
	}
	connection, err := s.Q.GetEnabledPublisherConnectionForProject(ctx, db.GetEnabledPublisherConnectionForProjectParams{ProjectID: fix.ProjectID, Kind: publisher.ConnectionKindGitHubNextJS})
	if err != nil {
		return resolvedSiteFixRepository{}, err
	}
	token, err := s.publisherConnectionToken(ctx, fix.ProjectID, connection)
	if err != nil {
		return resolvedSiteFixRepository{}, err
	}
	return resolvedSiteFixRepositoryFromConnection(connection, token)
}

func (s *Server) siteFixRepositorySourceLoader() *siteFixRepositorySourceLoader {
	return newSiteFixRepositorySourceLoader(s.resolveSiteFixRepository, s.siteFixRepositoryClientFactory())
}

// siteFixRepositorySourceLoaderForReadiness binds source reads to the exact
// connection/repository/branch/token snapshot proved by the final live
// readiness check. Task 6 uses this constructor for mutation-authorized flows;
// the query-backed constructor above remains for historical Apply recovery.
func (s *Server) siteFixRepositorySourceLoaderForReadiness(projectID uuid.UUID, target githubPRReadinessTarget) (*siteFixRepositorySourceLoader, error) {
	if projectID == uuid.Nil || target.ConnectionID == uuid.Nil || !target.ExpectedUpdatedAt.Valid ||
		strings.TrimSpace(target.Repo) == "" || strings.TrimSpace(target.Branch) == "" || strings.TrimSpace(target.token) == "" {
		return nil, errors.New("checked GitHub readiness target is incomplete")
	}
	if target.credentialKind != publisher.GitHubPRCredentialGitHubApp && target.credentialKind != publisher.GitHubPRCredentialAdvancedToken {
		return nil, errors.New("checked GitHub readiness credential kind is invalid")
	}
	permissions, _ := json.Marshal(target.permissions)
	fingerprintPayload := strings.Join([]string{
		projectID.String(), target.ConnectionID.String(), target.Repo, target.Branch,
		target.ExpectedUpdatedAt.Time.UTC().Format(time.RFC3339Nano), string(target.credentialKind), string(permissions),
	}, "\x00")
	sum := sha256.Sum256([]byte(fingerprintPayload))
	resolved := resolvedSiteFixRepository{
		ConnectionID: target.ConnectionID, Repo: strings.TrimSpace(target.Repo), Branch: strings.TrimSpace(target.Branch),
		Token: strings.TrimSpace(target.token), AuthorityFingerprint: hex.EncodeToString(sum[:]),
	}
	return newSiteFixRepositorySourceLoader(func(_ context.Context, fix db.SiteFix) (resolvedSiteFixRepository, error) {
		if fix.ProjectID != projectID {
			return resolvedSiteFixRepository{}, errors.New("checked GitHub readiness target belongs to another project")
		}
		return resolved, nil
	}, s.siteFixRepositoryClientFactory()), nil
}

func (s *Server) siteFixRepositoryClientFactory() siteFixRepositoryClientFactory {
	return func(token, repo, branch string) siteFixRepositoryClient {
		return &publisherSiteFixRepositoryClient{
			GitHubPRClient: publisher.NewGitHubPRClient(token, repo, branch, s.Log),
			token:          token, repo: repo, httpClient: &http.Client{Timeout: 30 * time.Second}, apiBase: "https://api.github.com",
		}
	}
}

type publisherSiteFixRepositoryClient struct {
	*publisher.GitHubPRClient
	token      string
	repo       string
	httpClient *http.Client
	apiBase    string
}

func (c *publisherSiteFixRepositoryClient) ResolveRefCommitSHA(ctx context.Context, branch string) (string, error) {
	parts := strings.Split(c.repo, "/")
	if len(parts) != 2 {
		return "", errors.New("invalid repository")
	}
	endpoint := strings.TrimRight(firstNonEmptyAPI(c.apiBase, "https://api.github.com"), "/") + "/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]) + "/git/ref/heads/" + url.PathEscape(strings.TrimSpace(branch))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32*1024))
		return "", fmt.Errorf("GitHub base ref lookup returned status %d", resp.StatusCode)
	}
	var payload struct {
		Ref    string `json:"ref"`
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 32*1024))
	if err := decoder.Decode(&payload); err != nil {
		return "", errors.New("GitHub base ref response was invalid")
	}
	branch = strings.TrimSpace(branch)
	if payload.Ref != "refs/heads/"+branch || strings.TrimSpace(payload.Object.SHA) == "" {
		return "", errors.New("GitHub base ref response did not match the configured branch")
	}
	return strings.TrimSpace(payload.Object.SHA), nil
}

func (c *publisherSiteFixRepositoryClient) ReadBlobBounded(ctx context.Context, blobSHA string, maxBytes int) ([]byte, error) {
	parts := strings.Split(c.repo, "/")
	if len(parts) != 2 || strings.TrimSpace(blobSHA) == "" || maxBytes <= 0 {
		return nil, errors.New("invalid bounded repository blob request")
	}
	endpoint := strings.TrimRight(firstNonEmptyAPI(c.apiBase, "https://api.github.com"), "/") + "/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]) + "/git/blobs/" + url.PathEscape(strings.TrimSpace(blobSHA))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32*1024))
		return nil, fmt.Errorf("GitHub blob lookup returned status %d", resp.StatusCode)
	}
	maxResponseBytes := base64.StdEncoding.EncodedLen(maxBytes) + 4096
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxResponseBytes+1)))
	if err != nil {
		return nil, errors.New("GitHub blob response could not be read")
	}
	if len(raw) > maxResponseBytes {
		return nil, errors.New("GitHub blob response exceeds the bounded source limit")
	}
	var payload struct {
		SHA      string `json:"sha"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if json.Unmarshal(raw, &payload) != nil || strings.TrimSpace(payload.Encoding) != "base64" || strings.TrimSpace(payload.SHA) != strings.TrimSpace(blobSHA) {
		return nil, errors.New("GitHub blob response was invalid")
	}
	encoded := strings.NewReplacer("\n", "", "\r", "", " ", "", "\t", "").Replace(payload.Content)
	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(content) > maxBytes {
		return nil, errors.New("GitHub blob content exceeds the bounded source limit")
	}
	return content, nil
}

func firstNonEmptyAPI(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

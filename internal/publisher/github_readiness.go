package publisher

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultGitHubAPIBase = "https://api.github.com"

type GitHubPRReadinessStatus string

const (
	GitHubPRReadinessNotConnected          GitHubPRReadinessStatus = "not_connected"
	GitHubPRReadinessNotChecked            GitHubPRReadinessStatus = "not_checked"
	GitHubPRReadinessReady                 GitHubPRReadinessStatus = "ready"
	GitHubPRReadinessPermissionMissing     GitHubPRReadinessStatus = "permission_missing"
	GitHubPRReadinessRepositoryUnavailable GitHubPRReadinessStatus = "repository_unavailable"
	GitHubPRReadinessError                 GitHubPRReadinessStatus = "error"
)

const (
	githubPRReadyDetail           = "GitHub can create pull requests for the selected repository and branch."
	githubPRAppPermissionDetail   = "The GitHub App needs contents: write and pull requests: write permissions."
	githubPRTokenPermissionDetail = "Repository write authority could not be proven for this token. Connect the GitHub App instead."
	githubPRRepositoryDetail      = "The selected GitHub repository or base branch is unavailable."
	githubPRErrorDetail           = "GitHub readiness could not be checked. Try again."
	githubPRNotConnectedDetail    = "Connect and enable GitHub with a project-scoped credential to create repair pull requests."
)

// GitHubPRReadiness is the redacted public readiness contract. Credential and
// upstream response data never belong in this value.
type GitHubPRReadiness struct {
	Status    GitHubPRReadinessStatus `json:"status"`
	CheckedAt *time.Time              `json:"checked_at"`
	Detail    string                  `json:"detail"`
	Repo      string                  `json:"repo"`
	Branch    string                  `json:"branch"`
}

type GitHubPRCredentialKind string

const (
	GitHubPRCredentialGitHubApp     GitHubPRCredentialKind = "github_app"
	GitHubPRCredentialAdvancedToken GitHubPRCredentialKind = "advanced_token"
)

type GitHubPRReadinessProbeInput struct {
	CredentialKind GitHubPRCredentialKind
	Token          string
	Permissions    map[string]string
	Repo           string
	Branch         string
}

// HasGitHubPRWritePermissions accepts only the exact grants required to write
// repository content and open pull requests. Accepted-permission response
// headers are request requirements, not proof of an installation's grants.
func HasGitHubPRWritePermissions(permissions map[string]string) bool {
	return permissions["contents"] == "write" && permissions["pull_requests"] == "write"
}

// ProbeGitHubPRReadiness checks the exact selected repository and configured
// base ref. It always returns controlled, redacted status/detail values.
func ProbeGitHubPRReadiness(
	ctx context.Context,
	client *http.Client,
	apiBase string,
	input GitHubPRReadinessProbeInput,
) GitHubPRReadiness {
	repo := strings.TrimSpace(input.Repo)
	branch := strings.TrimSpace(input.Branch)
	result := GitHubPRReadiness{Repo: repo, Branch: branch}
	if strings.TrimSpace(input.Token) == "" {
		result.Status = GitHubPRReadinessNotConnected
		result.Detail = githubPRNotConnectedDetail
		return result
	}
	owner, name, ok := githubRepositoryParts(repo)
	if !ok || branch == "" {
		result.Status = GitHubPRReadinessRepositoryUnavailable
		result.Detail = githubPRRepositoryDetail
		return result
	}
	if input.CredentialKind == GitHubPRCredentialGitHubApp && !HasGitHubPRWritePermissions(input.Permissions) {
		result.Status = GitHubPRReadinessPermissionMissing
		result.Detail = githubPRAppPermissionDetail
		return result
	}
	if input.CredentialKind != GitHubPRCredentialGitHubApp && input.CredentialKind != GitHubPRCredentialAdvancedToken {
		result.Status = GitHubPRReadinessError
		result.Detail = githubPRErrorDetail
		return result
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if base == "" {
		base = defaultGitHubAPIBase
	}
	repositoryURL := base + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
	var repository struct {
		FullName    string          `json:"full_name"`
		Private     *bool           `json:"private"`
		Permissions map[string]bool `json:"permissions"`
	}
	repositoryMetadata, ok := githubReadinessGET(ctx, client, repositoryURL, input.Token, &repository)
	if !ok {
		return githubReadinessFailure(result, repositoryMetadata.statusCode, input.CredentialKind)
	}
	if repository.FullName == "" || !strings.EqualFold(repository.FullName, repo) {
		result.Status = GitHubPRReadinessError
		result.Detail = githubPRErrorDetail
		return result
	}
	if input.CredentialKind == GitHubPRCredentialAdvancedToken {
		push, explicitlyReported := repository.Permissions["push"]
		if !explicitlyReported || !push {
			result.Status = GitHubPRReadinessPermissionMissing
			result.Detail = githubPRTokenPermissionDetail
			return result
		}
		_, hasRepoScope := repositoryMetadata.grantedOAuthScopes["repo"]
		_, hasPublicRepoScope := repositoryMetadata.grantedOAuthScopes["public_repo"]
		publicRepoScopeApplies := hasPublicRepoScope && repository.Private != nil && !*repository.Private
		if !hasRepoScope && !publicRepoScopeApplies {
			result.Status = GitHubPRReadinessPermissionMissing
			result.Detail = githubPRTokenPermissionDetail
			return result
		}
	}
	refURL := repositoryURL + "/git/ref/heads/" + url.PathEscape(branch)
	var ref struct {
		Ref    string `json:"ref"`
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	refMetadata, ok := githubReadinessGET(ctx, client, refURL, input.Token, &ref)
	if !ok {
		return githubReadinessFailure(result, refMetadata.statusCode, input.CredentialKind)
	}
	if ref.Ref != "refs/heads/"+branch || strings.TrimSpace(ref.Object.SHA) == "" {
		result.Status = GitHubPRReadinessError
		result.Detail = githubPRErrorDetail
		return result
	}
	result.Status = GitHubPRReadinessReady
	result.Detail = githubPRReadyDetail
	return result
}

func githubRepositoryParts(repo string) (string, string, bool) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	owner, name := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if owner == "" || name == "" || owner == "." || owner == ".." || name == "." || name == ".." {
		return "", "", false
	}
	return owner, name, true
}

type githubReadinessResponseMetadata struct {
	statusCode         int
	grantedOAuthScopes map[string]struct{}
}

func githubReadinessGET(ctx context.Context, client *http.Client, endpoint, token string, out any) (githubReadinessResponseMetadata, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return githubReadinessResponseMetadata{}, false
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := client.Do(req)
	if err != nil {
		return githubReadinessResponseMetadata{}, false
	}
	defer response.Body.Close()
	metadata := githubReadinessResponseMetadata{
		statusCode:         response.StatusCode,
		grantedOAuthScopes: grantedGitHubOAuthScopes(response.Header.Values("X-OAuth-Scopes")),
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 32*1024))
		return metadata, false
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 32*1024))
	if err := decoder.Decode(out); err != nil {
		return githubReadinessResponseMetadata{}, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return githubReadinessResponseMetadata{}, false
	}
	return metadata, true
}

func grantedGitHubOAuthScopes(headerValues []string) map[string]struct{} {
	scopes := make(map[string]struct{})
	for _, headerValue := range headerValues {
		for _, scope := range strings.Split(headerValue, ",") {
			if scope = strings.TrimSpace(scope); scope != "" {
				scopes[scope] = struct{}{}
			}
		}
	}
	return scopes
}

func githubReadinessFailure(result GitHubPRReadiness, status int, credentialKind GitHubPRCredentialKind) GitHubPRReadiness {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		result.Status = GitHubPRReadinessPermissionMissing
		if credentialKind == GitHubPRCredentialAdvancedToken {
			result.Detail = githubPRTokenPermissionDetail
		} else {
			result.Detail = githubPRAppPermissionDetail
		}
	case http.StatusNotFound:
		result.Status = GitHubPRReadinessRepositoryUnavailable
		result.Detail = githubPRRepositoryDetail
	default:
		result.Status = GitHubPRReadinessError
		result.Detail = githubPRErrorDetail
	}
	return result
}

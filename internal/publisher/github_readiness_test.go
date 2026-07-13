package publisher

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGitHubPRPermissionsRequireContentsAndPullRequestsWrite(t *testing.T) {
	for _, tc := range []struct {
		name        string
		permissions map[string]string
		ready       bool
	}{
		{name: "both exact write grants", permissions: map[string]string{"contents": "write", "pull_requests": "write"}, ready: true},
		{name: "contents read", permissions: map[string]string{"contents": "read", "pull_requests": "write"}},
		{name: "pull requests read", permissions: map[string]string{"contents": "write", "pull_requests": "read"}},
		{name: "contents absent", permissions: map[string]string{"pull_requests": "write"}},
		{name: "pull requests absent", permissions: map[string]string{"contents": "write"}},
		{name: "unexpected stronger-looking value", permissions: map[string]string{"contents": "admin", "pull_requests": "write"}},
		{name: "nil permissions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasGitHubPRWritePermissions(tc.permissions); got != tc.ready {
				t.Fatalf("HasGitHubPRWritePermissions(%#v) = %v, want %v", tc.permissions, got, tc.ready)
			}
		})
	}
}

func TestGitHubPRReadinessProbeUsesExactRepositoryAndConfiguredBaseRef(t *testing.T) {
	for _, tc := range []struct {
		name                string
		credentialKind      GitHubPRCredentialKind
		permissions         map[string]string
		repositoryBody      string
		grantedOAuthScopes  string
		acceptedPermissions string
		wantStatus          GitHubPRReadinessStatus
		wantRequestCount    int
	}{
		{
			name:             "GitHub App grants plus exact reads are ready",
			credentialKind:   GitHubPRCredentialGitHubApp,
			permissions:      map[string]string{"contents": "write", "pull_requests": "write"},
			repositoryBody:   `{"full_name":"acme/site","private":true,"permissions":{"push":false}}`,
			wantStatus:       GitHubPRReadinessReady,
			wantRequestCount: 2,
		},
		{
			name:             "advanced token push without granted scopes is ambiguous",
			credentialKind:   GitHubPRCredentialAdvancedToken,
			repositoryBody:   `{"full_name":"acme/site","private":false,"permissions":{"push":true}}`,
			wantStatus:       GitHubPRReadinessPermissionMissing,
			wantRequestCount: 1,
		},
		{
			name:               "classic repo scope plus push proves authority",
			credentialKind:     GitHubPRCredentialAdvancedToken,
			repositoryBody:     `{"full_name":"acme/site","private":true,"permissions":{"push":true}}`,
			grantedOAuthScopes: "read:user, repo, workflow",
			wantStatus:         GitHubPRReadinessReady,
			wantRequestCount:   2,
		},
		{
			name:               "classic public repo scope plus public push proves authority",
			credentialKind:     GitHubPRCredentialAdvancedToken,
			repositoryBody:     `{"full_name":"acme/site","private":false,"permissions":{"push":true}}`,
			grantedOAuthScopes: "public_repo",
			wantStatus:         GitHubPRReadinessReady,
			wantRequestCount:   2,
		},
		{
			name:               "classic public repo scope cannot authorize private repository",
			credentialKind:     GitHubPRCredentialAdvancedToken,
			repositoryBody:     `{"full_name":"acme/site","private":true,"permissions":{"push":true}}`,
			grantedOAuthScopes: "public_repo",
			wantStatus:         GitHubPRReadinessPermissionMissing,
			wantRequestCount:   1,
		},
		{
			name:                "accepted permissions header is not a granted scope",
			credentialKind:      GitHubPRCredentialAdvancedToken,
			repositoryBody:      `{"full_name":"acme/site","private":false,"permissions":{"push":true}}`,
			acceptedPermissions: "contents=write, pull_requests=write",
			wantStatus:          GitHubPRReadinessPermissionMissing,
			wantRequestCount:    1,
		},
		{
			name:             "advanced token ambiguous permissions are rejected",
			credentialKind:   GitHubPRCredentialAdvancedToken,
			repositoryBody:   `{"full_name":"acme/site","permissions":{"pull":true}}`,
			wantStatus:       GitHubPRReadinessPermissionMissing,
			wantRequestCount: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			client := &http.Client{Transport: publisherReadinessRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				paths = append(paths, req.URL.EscapedPath())
				if req.Header.Get("Authorization") != "Bearer private-token" {
					t.Fatalf("authorization header was not set for probe")
				}
				switch req.URL.EscapedPath() {
				case "/repos/acme/site":
					response := publisherReadinessResponse(req, http.StatusOK, tc.repositoryBody)
					response.Header.Set("X-OAuth-Scopes", tc.grantedOAuthScopes)
					response.Header.Set("X-Accepted-GitHub-Permissions", tc.acceptedPermissions)
					return response, nil
				case "/repos/acme/site/git/ref/heads/release":
					return publisherReadinessResponse(req, http.StatusOK, `{"ref":"refs/heads/release","object":{"sha":"base-sha"}}`), nil
				default:
					t.Fatalf("unexpected GitHub request path %q", req.URL.EscapedPath())
					return nil, nil
				}
			})}

			got := ProbeGitHubPRReadiness(context.Background(), client, "https://github.example", GitHubPRReadinessProbeInput{
				CredentialKind: tc.credentialKind,
				Token:          "private-token",
				Permissions:    tc.permissions,
				Repo:           "acme/site",
				Branch:         "release",
			})
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (detail %q)", got.Status, tc.wantStatus, got.Detail)
			}
			if got.Repo != "acme/site" || got.Branch != "release" {
				t.Fatalf("target = %q@%q", got.Repo, got.Branch)
			}
			if len(paths) != tc.wantRequestCount {
				t.Fatalf("request paths = %#v, want %d calls", paths, tc.wantRequestCount)
			}
			for _, path := range paths {
				if strings.Contains(path, "installation/repositories") {
					t.Fatalf("readiness must not use installation repository listing: %q", path)
				}
			}
		})
	}
}

func TestGitHubPRReadinessProbeClassifiesRepositoryAndBaseRefFailuresSafely(t *testing.T) {
	const unsafe = `ghp_leaked Authorization: Bearer private-token {"message":"raw upstream body"}`
	for _, tc := range []struct {
		name           string
		repositoryCode int
		repositoryBody string
		refCode        int
		refBody        string
		transportCall  int
		wantStatus     GitHubPRReadinessStatus
		wantDetail     string
	}{
		{name: "repository unauthorized", repositoryCode: http.StatusUnauthorized, repositoryBody: unsafe, wantStatus: GitHubPRReadinessPermissionMissing, wantDetail: githubPRTokenPermissionDetail},
		{name: "repository forbidden", repositoryCode: http.StatusForbidden, repositoryBody: unsafe, wantStatus: GitHubPRReadinessPermissionMissing, wantDetail: githubPRTokenPermissionDetail},
		{name: "repository missing", repositoryCode: http.StatusNotFound, repositoryBody: unsafe, wantStatus: GitHubPRReadinessRepositoryUnavailable},
		{name: "repository server error", repositoryCode: http.StatusBadGateway, repositoryBody: unsafe, wantStatus: GitHubPRReadinessError},
		{name: "repository malformed", repositoryCode: http.StatusOK, repositoryBody: `{`, wantStatus: GitHubPRReadinessError},
		{name: "repository transport error", transportCall: 1, wantStatus: GitHubPRReadinessError},
		{name: "base ref unauthorized", repositoryCode: http.StatusOK, refCode: http.StatusUnauthorized, refBody: unsafe, wantStatus: GitHubPRReadinessPermissionMissing, wantDetail: githubPRTokenPermissionDetail},
		{name: "base ref forbidden", repositoryCode: http.StatusOK, refCode: http.StatusForbidden, refBody: unsafe, wantStatus: GitHubPRReadinessPermissionMissing, wantDetail: githubPRTokenPermissionDetail},
		{name: "base ref missing", repositoryCode: http.StatusOK, refCode: http.StatusNotFound, refBody: unsafe, wantStatus: GitHubPRReadinessRepositoryUnavailable},
		{name: "base ref server error", repositoryCode: http.StatusOK, refCode: http.StatusInternalServerError, refBody: unsafe, wantStatus: GitHubPRReadinessError},
		{name: "base ref malformed", repositoryCode: http.StatusOK, refCode: http.StatusOK, refBody: `{}`, wantStatus: GitHubPRReadinessError},
		{name: "base ref transport error", repositoryCode: http.StatusOK, transportCall: 2, wantStatus: GitHubPRReadinessError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: publisherReadinessRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls == tc.transportCall {
					return nil, errors.New(unsafe)
				}
				if calls == 1 {
					code := tc.repositoryCode
					if code == 0 {
						code = http.StatusOK
					}
					body := tc.repositoryBody
					if body == "" {
						body = `{"full_name":"acme/site","permissions":{"push":true}}`
					}
					response := publisherReadinessResponse(req, code, body)
					if code == http.StatusOK {
						response.Header.Set("X-OAuth-Scopes", "repo")
					}
					return response, nil
				}
				code := tc.refCode
				if code == 0 {
					code = http.StatusOK
				}
				body := tc.refBody
				if body == "" {
					body = `{"ref":"refs/heads/main","object":{"sha":"base-sha"}}`
				}
				return publisherReadinessResponse(req, code, body), nil
			})}

			got := ProbeGitHubPRReadiness(context.Background(), client, "https://github.example", GitHubPRReadinessProbeInput{
				CredentialKind: GitHubPRCredentialAdvancedToken,
				Token:          "private-token",
				Repo:           "acme/site",
				Branch:         "main",
			})
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (detail %q)", got.Status, tc.wantStatus, got.Detail)
			}
			if tc.wantDetail != "" && got.Detail != tc.wantDetail {
				t.Fatalf("detail = %q, want %q", got.Detail, tc.wantDetail)
			}
			for _, secret := range []string{"ghp_leaked", "private-token", "Authorization", "raw upstream body"} {
				if strings.Contains(got.Detail, secret) {
					t.Fatalf("detail leaked %q: %q", secret, got.Detail)
				}
			}
		})
	}
}

func TestGitHubPRReadinessProbeClassifiesLocalPreconditionsWithoutNetwork(t *testing.T) {
	client := &http.Client{Transport: publisherReadinessRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("local precondition failure must not call GitHub")
		return nil, nil
	})}

	for _, tc := range []struct {
		name       string
		input      GitHubPRReadinessProbeInput
		wantStatus GitHubPRReadinessStatus
	}{
		{name: "missing credential", input: GitHubPRReadinessProbeInput{Repo: "acme/site", Branch: "main"}, wantStatus: GitHubPRReadinessNotConnected},
		{name: "missing repository", input: GitHubPRReadinessProbeInput{CredentialKind: GitHubPRCredentialAdvancedToken, Token: "private-token", Branch: "main"}, wantStatus: GitHubPRReadinessRepositoryUnavailable},
		{name: "missing branch", input: GitHubPRReadinessProbeInput{CredentialKind: GitHubPRCredentialAdvancedToken, Token: "private-token", Repo: "acme/site"}, wantStatus: GitHubPRReadinessRepositoryUnavailable},
		{name: "App missing grants", input: GitHubPRReadinessProbeInput{CredentialKind: GitHubPRCredentialGitHubApp, Token: "private-token", Repo: "acme/site", Branch: "main", Permissions: map[string]string{"contents": "read", "pull_requests": "write"}}, wantStatus: GitHubPRReadinessPermissionMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ProbeGitHubPRReadiness(context.Background(), client, "https://github.example", tc.input)
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", got.Status, tc.wantStatus)
			}
		})
	}
}

func publisherReadinessResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

type publisherReadinessRoundTripFunc func(*http.Request) (*http.Response, error)

func (f publisherReadinessRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

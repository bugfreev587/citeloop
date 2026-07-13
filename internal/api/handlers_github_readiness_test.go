package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/githubapp"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestGitHubPRReadinessGETReturnsStoredStateWithoutLiveCalls(t *testing.T) {
	projectID := uuid.New()
	checkedAt := time.Date(2026, 7, 12, 18, 30, 0, 0, time.UTC)
	detail := "GitHub can create pull requests for the selected repository and branch."
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessReady,
		`{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`,
		&checkedAt,
		&detail,
	)}}}
	checker := &fakeGitHubPRReadinessChecker{}
	app := &fakeGitHubAppClient{configured: true}
	server := &Server{
		githubReadinessStore:   store,
		githubReadinessChecker: checker,
		githubAppClient:        app,
	}

	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, httptest.NewRequest(
		http.MethodGet,
		"/api/projects/"+projectID.String()+"/integrations/github/pr-readiness",
		nil,
	))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got publisher.GitHubPRReadiness
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != publisher.GitHubPRReadinessReady || got.Repo != "acme/site" || got.Branch != "main" || got.Detail != detail {
		t.Fatalf("readiness = %#v", got)
	}
	if got.CheckedAt == nil || !got.CheckedAt.Equal(checkedAt) {
		t.Fatalf("checked_at = %v, want %v", got.CheckedAt, checkedAt)
	}
	if checker.calls != 0 {
		t.Fatalf("GET invoked live checker %d times", checker.calls)
	}
	if len(app.calls) != 0 {
		t.Fatalf("GET invoked GitHub App methods: %#v", app.calls)
	}
	if store.setCalls != 0 {
		t.Fatalf("GET persisted readiness %d times", store.setCalls)
	}
}

func TestGitHubPRReadinessGETSynthesizesNotConnectedWhenNoRowExists(t *testing.T) {
	projectID := uuid.New()
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{err: pgx.ErrNoRows}}}
	server := &Server{githubReadinessStore: store}

	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, httptest.NewRequest(
		http.MethodGet,
		"/api/projects/"+projectID.String()+"/integrations/github/pr-readiness",
		nil,
	))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got publisher.GitHubPRReadiness
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != publisher.GitHubPRReadinessNotConnected || got.CheckedAt != nil || got.Repo != "" || got.Branch != "" {
		t.Fatalf("readiness = %#v", got)
	}
}

func TestGitHubPRReadinessPOSTUsesAppGrantsChecksExactTargetAndPersistsRedactedState(t *testing.T) {
	projectID := uuid.New()
	checkedAt := time.Date(2026, 7, 12, 19, 45, 0, 987654321, time.FixedZone("offset", -7*60*60))
	connection := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessNotChecked,
		`{"repo":"acme/site","branch":"release","content_dir":"content/blog","base_url":"https://example.com/blog","installation_id":"12345"}`,
		nil,
		nil,
	)
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: connection}}}
	store.set = func(params db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
		updated := connection
		updated.PrReadinessStatus = params.PrReadinessStatus
		updated.PrReadinessCheckedAt = params.PrReadinessCheckedAt
		updated.PrReadinessDetail = params.PrReadinessDetail
		return updated, nil
	}
	app := &fakeGitHubAppClient{
		configured: true,
		access: githubapp.InstallationAccess{
			Token:       "installation-secret",
			Permissions: map[string]string{"contents": "write", "pull_requests": "write"},
		},
	}
	var requestPaths []string
	client := &http.Client{Transport: apiGitHubReadinessRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestPaths = append(requestPaths, req.URL.EscapedPath())
		if req.Header.Get("Authorization") != "Bearer installation-secret" {
			t.Fatalf("probe Authorization header = %q", req.Header.Get("Authorization"))
		}
		switch req.URL.EscapedPath() {
		case "/repos/acme/site":
			return apiGitHubReadinessResponse(req, http.StatusOK, `{"full_name":"acme/site"}`), nil
		case "/repos/acme/site/git/ref/heads/release":
			return apiGitHubReadinessResponse(req, http.StatusOK, `{"ref":"refs/heads/release","object":{"sha":"base-sha"}}`), nil
		default:
			t.Fatalf("unexpected probe path %q", req.URL.EscapedPath())
			return nil, nil
		}
	})}
	server := &Server{
		githubReadinessStore:      store,
		githubAppClient:           app,
		githubReadinessHTTPClient: client,
		githubReadinessAPIBase:    "https://github.example",
		githubReadinessNow:        func() time.Time { return checkedAt },
	}

	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/projects/"+projectID.String()+"/integrations/github/pr-readiness/check",
		nil,
	))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got, want := requestPaths, []string{"/repos/acme/site", "/repos/acme/site/git/ref/heads/release"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("probe paths = %#v, want %#v", got, want)
	}
	if app.gotInstallationID != "12345" || !readinessSliceContains(app.calls, "InstallationAccess") {
		t.Fatalf("GitHub App calls = %#v, installation = %q", app.calls, app.gotInstallationID)
	}
	if store.setCalls != 1 {
		t.Fatalf("readiness set calls = %d", store.setCalls)
	}
	params := store.setParams[0]
	if params.ConnectionID != connection.ID || params.ProjectID != projectID || params.ExpectedUpdatedAt != connection.UpdatedAt {
		t.Fatalf("CAS target = %#v", params)
	}
	if params.PrReadinessStatus != string(publisher.GitHubPRReadinessReady) {
		t.Fatalf("persisted status = %q", params.PrReadinessStatus)
	}
	if !params.PrReadinessCheckedAt.Valid || !params.PrReadinessCheckedAt.Time.Equal(checkedAt.UTC()) || params.PrReadinessCheckedAt.Time.Location() != time.UTC {
		t.Fatalf("persisted checked_at = %#v, want UTC %v", params.PrReadinessCheckedAt, checkedAt.UTC())
	}

	responseBody := response.Body.String()
	for _, unsafe := range []string{"installation-secret", "Authorization", "Bearer", "permissions"} {
		if strings.Contains(responseBody, unsafe) {
			t.Fatalf("response leaked %q: %s", unsafe, responseBody)
		}
	}
	var got publisher.GitHubPRReadiness
	if err := json.Unmarshal([]byte(responseBody), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != publisher.GitHubPRReadinessReady || got.Repo != "acme/site" || got.Branch != "release" {
		t.Fatalf("readiness = %#v", got)
	}
	if got.CheckedAt == nil || !got.CheckedAt.Equal(checkedAt.UTC()) {
		t.Fatalf("checked_at = %v, want %v", got.CheckedAt, checkedAt.UTC())
	}
}

func TestGitHubPRReadinessPOSTNormalizesOmittedBranchBeforeProbeAndPersistence(t *testing.T) {
	projectID := uuid.New()
	checkedAt := time.Date(2026, 7, 13, 9, 15, 0, 0, time.UTC)
	connection := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessNotChecked,
		`{"repo":" acme/site ","base_url":"https://staging.unipost.dev/blog","installation_id":" 12345 "}`,
		nil,
		nil,
	)
	normalized, err := publisher.ParseGitHubNextJSConfig(connection.Config)
	if err != nil {
		t.Fatalf("test fixture must be a valid GitHub publisher config: %v", err)
	}
	if normalized.Branch != "staging" {
		t.Fatalf("test fixture branch = %q, want parser-derived staging", normalized.Branch)
	}
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: connection}}}
	store.set = func(params db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
		updated := connection
		updated.PrReadinessStatus = params.PrReadinessStatus
		updated.PrReadinessCheckedAt = params.PrReadinessCheckedAt
		updated.PrReadinessDetail = params.PrReadinessDetail
		store.getResults = append(store.getResults, fakeGitHubPRReadinessStoreResult{connection: updated})
		return updated, nil
	}
	app := &fakeGitHubAppClient{
		configured: true,
		access: githubapp.InstallationAccess{
			Token:       "installation-secret",
			Permissions: map[string]string{"contents": "write", "pull_requests": "write"},
		},
	}
	var requestPaths []string
	client := &http.Client{Transport: apiGitHubReadinessRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestPaths = append(requestPaths, req.URL.EscapedPath())
		switch req.URL.EscapedPath() {
		case "/repos/acme/site":
			return apiGitHubReadinessResponse(req, http.StatusOK, `{"full_name":"acme/site"}`), nil
		case "/repos/acme/site/git/ref/heads/" + normalized.Branch:
			return apiGitHubReadinessResponse(req, http.StatusOK, `{"ref":"refs/heads/staging","object":{"sha":"base-sha"}}`), nil
		default:
			t.Fatalf("unexpected probe path %q", req.URL.EscapedPath())
			return nil, nil
		}
	})}
	server := &Server{
		githubReadinessStore:      store,
		githubAppClient:           app,
		githubReadinessHTTPClient: client,
		githubReadinessAPIBase:    "https://github.example",
		githubReadinessNow:        func() time.Time { return checkedAt },
	}
	capturingChecker := &capturingGitHubPRReadinessChecker{delegate: serverGitHubPRReadinessChecker{server: server}}
	server.githubReadinessChecker = capturingChecker

	postResponse := httptest.NewRecorder()
	server.Router().ServeHTTP(postResponse, httptest.NewRequest(
		http.MethodPost,
		"/api/projects/"+projectID.String()+"/integrations/github/pr-readiness/check",
		nil,
	))

	if postResponse.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body = %s", postResponse.Code, postResponse.Body.String())
	}
	if got, want := requestPaths, []string{"/repos/acme/site", "/repos/acme/site/git/ref/heads/" + normalized.Branch}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("probe paths = %#v, want %#v", got, want)
	}
	if capturingChecker.target.ConnectionID != connection.ID || capturingChecker.target.ExpectedUpdatedAt != connection.UpdatedAt || capturingChecker.target.Repo != normalized.Repo || capturingChecker.target.Branch != normalized.Branch {
		t.Fatalf("checked target = %#v, want %s@%s with original identity/version", capturingChecker.target, normalized.Repo, normalized.Branch)
	}
	if app.gotInstallationID != "12345" {
		t.Fatalf("installation id = %q, want trimmed raw installation", app.gotInstallationID)
	}
	if store.setCalls != 1 {
		t.Fatalf("readiness set calls = %d", store.setCalls)
	}
	params := store.setParams[0]
	if params.ConnectionID != connection.ID || params.ProjectID != projectID || params.ExpectedUpdatedAt != connection.UpdatedAt || params.PrReadinessStatus != string(publisher.GitHubPRReadinessReady) {
		t.Fatalf("CAS params = %#v", params)
	}
	var posted publisher.GitHubPRReadiness
	if err := json.NewDecoder(postResponse.Body).Decode(&posted); err != nil {
		t.Fatal(err)
	}
	if posted.Status != publisher.GitHubPRReadinessReady || posted.Repo != normalized.Repo || posted.Branch != normalized.Branch {
		t.Fatalf("POST readiness = %#v", posted)
	}

	appCallCount := len(app.calls)
	getResponse := httptest.NewRecorder()
	server.Router().ServeHTTP(getResponse, httptest.NewRequest(
		http.MethodGet,
		"/api/projects/"+projectID.String()+"/integrations/github/pr-readiness",
		nil,
	))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	var persisted publisher.GitHubPRReadiness
	if err := json.NewDecoder(getResponse.Body).Decode(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Status != publisher.GitHubPRReadinessReady || persisted.Repo != normalized.Repo || persisted.Branch != normalized.Branch {
		t.Fatalf("persisted readiness = %#v", persisted)
	}
	if len(app.calls) != appCallCount {
		t.Fatalf("stored GET invoked GitHub App: before=%d after=%d", appCallCount, len(app.calls))
	}
}

func TestGitHubPRReadinessPOSTReloadsNewerStateWhenCASLoses(t *testing.T) {
	projectID := uuid.New()
	staleUpdatedAt := pgtype.Timestamptz{Time: time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC), Valid: true}
	newer := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessNotConnected,
		`{"repo":"acme/new-target","branch":"stable","base_url":"https://new.example/blog"}`,
		nil,
		nil,
	)
	newer.Enabled = false
	store := &fakeGitHubPRReadinessStore{
		getResults: []fakeGitHubPRReadinessStoreResult{{connection: newer}},
		set: func(db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
			return db.PublisherConnection{}, pgx.ErrNoRows
		},
	}
	checker := &fakeGitHubPRReadinessChecker{
		readiness: publisher.GitHubPRReadiness{
			Status: publisher.GitHubPRReadinessReady,
			Detail: "stale success",
			Repo:   "acme/old-target",
			Branch: "main",
		},
		target: githubPRReadinessTarget{
			ConnectionID:      uuid.New(),
			ExpectedUpdatedAt: staleUpdatedAt,
			Repo:              "acme/old-target",
			Branch:            "main",
			token:             "ghp_stale_secret",
		},
	}
	server := &Server{
		githubReadinessStore:   store,
		githubReadinessChecker: checker,
		githubReadinessNow:     func() time.Time { return time.Date(2026, 7, 12, 20, 5, 0, 0, time.UTC) },
	}

	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/projects/"+projectID.String()+"/integrations/github/pr-readiness/check",
		nil,
	))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got publisher.GitHubPRReadiness
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != publisher.GitHubPRReadinessNotConnected || got.Repo != "acme/new-target" || got.Branch != "stable" || got.CheckedAt != nil {
		t.Fatalf("CAS-lost response = %#v", got)
	}
	if strings.Contains(response.Body.String(), "stale") || strings.Contains(response.Body.String(), "ghp_stale_secret") {
		t.Fatalf("CAS-lost response returned stale result or secret: %s", response.Body.String())
	}
	if checker.calls != 1 || store.setCalls != 1 || store.getCalls != 1 {
		t.Fatalf("calls: checker=%d set=%d reload=%d", checker.calls, store.setCalls, store.getCalls)
	}
}

func TestGitHubPRReadinessPOSTClassifiesAppAccessForbiddenAsPermissionMissing(t *testing.T) {
	projectID := uuid.New()
	connection := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessNotChecked,
		`{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog","installation_id":"12345"}`,
		nil,
		nil,
	)
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: connection}}}
	store.set = func(params db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
		updated := connection
		updated.PrReadinessStatus = params.PrReadinessStatus
		updated.PrReadinessCheckedAt = params.PrReadinessCheckedAt
		updated.PrReadinessDetail = params.PrReadinessDetail
		return updated, nil
	}
	app := &fakeGitHubAppClient{
		configured: true,
		err:        readinessHTTPStatusError{status: http.StatusForbidden},
	}
	server := &Server{
		githubReadinessStore: store,
		githubAppClient:      app,
		githubReadinessNow:   func() time.Time { return time.Date(2026, 7, 12, 20, 30, 0, 0, time.UTC) },
	}

	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/projects/"+projectID.String()+"/integrations/github/pr-readiness/check",
		nil,
	))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got publisher.GitHubPRReadiness
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != publisher.GitHubPRReadinessPermissionMissing {
		t.Fatalf("status = %q, want permission_missing (detail %q)", got.Status, got.Detail)
	}
	if store.setCalls != 1 || store.setParams[0].PrReadinessStatus != string(publisher.GitHubPRReadinessPermissionMissing) {
		t.Fatalf("persisted readiness = %#v", store.setParams)
	}
	for _, unsafe := range []string{"Authorization", "Bearer", "raw upstream body", "ghp_secret"} {
		if strings.Contains(response.Body.String(), unsafe) {
			t.Fatalf("response leaked %q: %s", unsafe, response.Body.String())
		}
	}
}

func TestGitHubPRReadinessPOSTDistinguishesNoConnectionFromDatabaseFailure(t *testing.T) {
	projectID := uuid.New()
	for _, tc := range []struct {
		name       string
		storeError error
		wantStatus int
		wantBody   string
	}{
		{name: "no connection", storeError: pgx.ErrNoRows, wantStatus: http.StatusOK, wantBody: `"status":"not_connected"`},
		{name: "database failure", storeError: errors.New("database includes ghp_secret raw body"), wantStatus: http.StatusInternalServerError, wantBody: "GitHub readiness could not be checked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{err: tc.storeError}}}
			server := &Server{githubReadinessStore: store}
			response := httptest.NewRecorder()
			server.Router().ServeHTTP(response, httptest.NewRequest(
				http.MethodPost,
				"/api/projects/"+projectID.String()+"/integrations/github/pr-readiness/check",
				nil,
			))
			if response.Code != tc.wantStatus || !strings.Contains(response.Body.String(), tc.wantBody) {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			for _, unsafe := range []string{"ghp_secret", "raw body", "database includes"} {
				if strings.Contains(response.Body.String(), unsafe) {
					t.Fatalf("response leaked %q: %s", unsafe, response.Body.String())
				}
			}
		})
	}
}

func TestGitHubPRReadinessPOSTDoesNotPersistCredentialDatabaseFailure(t *testing.T) {
	projectID := uuid.New()
	connection := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessNotChecked,
		`{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`,
		nil,
		nil,
	)
	credentialRef := publisher.PublisherCredentialRef(uuid.New())
	connection.CredentialRef = &credentialRef
	rawDatabaseError := errors.New("credential database failed with ghp_secret raw text")
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: connection}}}
	server := &Server{
		Q:                    db.New(readinessCredentialErrorDB{err: rawDatabaseError}),
		githubReadinessStore: store,
		githubReadinessHTTPClient: &http.Client{Transport: apiGitHubReadinessRoundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("credential database failure must stop before GitHub probe")
			return nil, nil
		})},
	}
	server.Env.NotificationSecretKey = "test-secretbox-key"

	request := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/integrations/github/pr-readiness/check", nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("projectID", projectID.String())
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	response := httptest.NewRecorder()
	server.checkGitHubPRReadiness(response, request)

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "GitHub readiness could not be checked") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if store.setCalls != 0 {
		t.Fatalf("credential database failure persisted readiness %d times", store.setCalls)
	}
	for _, unsafe := range []string{"credential database failed", "ghp_secret", "raw text"} {
		if strings.Contains(response.Body.String(), unsafe) {
			t.Fatalf("response leaked %q: %s", unsafe, response.Body.String())
		}
	}
}

func TestGitHubPRReadinessTargetKeepsCredentialPrivate(t *testing.T) {
	target := githubPRReadinessTarget{
		ConnectionID:      uuid.New(),
		ExpectedUpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Repo:              "acme/site",
		Branch:            "main",
		token:             "ghp_private_snapshot",
	}
	body, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "ghp_private_snapshot") || strings.Contains(strings.ToLower(string(body)), "token") {
		t.Fatalf("target snapshot serialized credential: %s", body)
	}
}

func TestGitHubPRMutationAuthorizationRejectsStoredNonReadyWithoutLiveCheck(t *testing.T) {
	projectID := uuid.New()
	connection := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessNotChecked,
		`{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`,
		nil,
		nil,
	)
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: connection}}}
	checker := &fakeGitHubPRReadinessChecker{}
	server := &Server{githubReadinessStore: store, githubReadinessChecker: checker}

	_, err := server.authorizeGitHubPRMutation(context.Background(), projectID)

	if !errors.Is(err, errGitHubPRNotReady) {
		t.Fatalf("error = %v, want persisted-readiness conflict", err)
	}
	if checker.calls != 0 || store.setCalls != 0 {
		t.Fatalf("non-ready stored gate called live checker/persist: checker=%d set=%d", checker.calls, store.setCalls)
	}
}

func TestGitHubPRMutationAuthorizationPersistsLiveDowngradeBeforeRejecting(t *testing.T) {
	projectID := uuid.New()
	connection := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessReady,
		`{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`,
		nil,
		nil,
	)
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: connection}}}
	store.set = func(params db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
		updated := connection
		updated.PrReadinessStatus = params.PrReadinessStatus
		updated.PrReadinessCheckedAt = params.PrReadinessCheckedAt
		updated.PrReadinessDetail = params.PrReadinessDetail
		return updated, nil
	}
	checker := &fakeGitHubPRReadinessChecker{
		readiness: publisher.GitHubPRReadiness{Status: publisher.GitHubPRReadinessPermissionMissing},
		target: githubPRReadinessTarget{
			ConnectionID: connection.ID, ExpectedUpdatedAt: connection.UpdatedAt,
			Repo: "acme/site", Branch: "main", token: "secret",
		},
	}
	server := &Server{
		githubReadinessStore: store, githubReadinessChecker: checker,
		githubReadinessNow: func() time.Time { return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC) },
	}

	_, err := server.authorizeGitHubPRMutation(context.Background(), projectID)

	if !errors.Is(err, errGitHubPRNotReady) {
		t.Fatalf("error = %v, want live-readiness conflict", err)
	}
	if checker.calls != 1 || store.setCalls != 1 {
		t.Fatalf("calls: checker=%d set=%d", checker.calls, store.setCalls)
	}
	if got := store.setParams[0].PrReadinessStatus; got != string(publisher.GitHubPRReadinessPermissionMissing) {
		t.Fatalf("persisted status = %q", got)
	}
}

func TestGitHubPRMutationAuthorizationRetainsExactConnectionVersion(t *testing.T) {
	projectID := uuid.New()
	connection := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessReady,
		`{"repo":"acme/site","branch":"release","base_url":"https://example.com/blog"}`,
		nil,
		nil,
	)
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: connection}}}
	store.set = func(params db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
		updated := connection
		updated.PrReadinessStatus = params.PrReadinessStatus
		updated.PrReadinessCheckedAt = params.PrReadinessCheckedAt
		updated.PrReadinessDetail = params.PrReadinessDetail
		return updated, nil
	}
	wantTarget := githubPRReadinessTarget{
		ConnectionID: connection.ID, ExpectedUpdatedAt: connection.UpdatedAt,
		Repo: "acme/site", Branch: "release",
		credentialKind: publisher.GitHubPRCredentialAdvancedToken,
		token:          "ghp_exact_checked_token",
	}
	checker := &fakeGitHubPRReadinessChecker{
		readiness: publisher.GitHubPRReadiness{Status: publisher.GitHubPRReadinessReady},
		target:    wantTarget,
	}
	server := &Server{githubReadinessStore: store, githubReadinessChecker: checker}

	target, err := server.authorizeGitHubPRMutation(context.Background(), projectID)

	if err != nil {
		t.Fatalf("authorize mutation: %v", err)
	}
	if target.ConnectionID != wantTarget.ConnectionID || target.Repo != wantTarget.Repo || target.Branch != wantTarget.Branch || target.token != wantTarget.token {
		t.Fatalf("target = %#v, want exact checked target %#v", target, wantTarget)
	}
	if target.ExpectedUpdatedAt != connection.UpdatedAt {
		t.Fatalf("target version = %#v, want unchanged connection version %#v", target.ExpectedUpdatedAt, connection.UpdatedAt)
	}
}

func TestGitHubPRReadyRefreshesDoNotInvalidateAnAuthorizedTarget(t *testing.T) {
	projectID := uuid.New()
	connection := githubPRReadinessConnection(projectID, publisher.GitHubPRReadinessReady, `{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`, nil, nil)
	store := &statefulGitHubPRReadinessStore{connection: connection}
	checker := &fakeGitHubPRReadinessChecker{
		readiness: publisher.GitHubPRReadiness{Status: publisher.GitHubPRReadinessReady},
		target: githubPRReadinessTarget{
			ConnectionID: connection.ID, ExpectedUpdatedAt: connection.UpdatedAt,
			Repo: "acme/site", Branch: "main", credentialKind: publisher.GitHubPRCredentialAdvancedToken, token: "checked-token",
		},
	}
	times := []time.Time{
		time.Date(2026, 7, 13, 18, 0, 0, 1, time.UTC),
		time.Date(2026, 7, 13, 18, 0, 0, 2, time.UTC),
		time.Date(2026, 7, 13, 18, 0, 0, 3, time.UTC),
	}
	nowCall := 0
	server := &Server{githubReadinessStore: store, githubReadinessChecker: checker, githubReadinessNow: func() time.Time {
		value := times[nowCall]
		nowCall++
		return value
	}}

	first, err := server.authorizeGitHubPRMutation(context.Background(), projectID)
	if err != nil {
		t.Fatalf("first authorization: %v", err)
	}
	second, err := server.authorizeGitHubPRMutation(context.Background(), projectID)
	if err != nil {
		t.Fatalf("second authorization: %v", err)
	}
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/integrations/github/pr-readiness/check", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("settings readiness status=%d body=%s", response.Code, response.Body.String())
	}
	if first.ExpectedUpdatedAt != connection.UpdatedAt || second.ExpectedUpdatedAt != connection.UpdatedAt {
		t.Fatalf("connection versions first=%#v second=%#v", first.ExpectedUpdatedAt, second.ExpectedUpdatedAt)
	}
	if err := server.ensureGitHubPRTargetCurrent(context.Background(), projectID, first); err != nil {
		t.Fatalf("later ready refresh invalidated first target: %v", err)
	}
}

func TestOlderLiveReadyCheckCannotOverwriteNewerMutationDowngrade(t *testing.T) {
	projectID := uuid.New()
	connection := githubPRReadinessConnection(projectID, publisher.GitHubPRReadinessReady, `{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`, nil, nil)
	store := &statefulGitHubPRReadinessStore{connection: connection}
	oldStartedAt := time.Date(2026, 7, 13, 18, 30, 0, 1, time.UTC)
	newerFailureAt := oldStartedAt.Add(time.Nanosecond)
	times := []time.Time{oldStartedAt, newerFailureAt}
	nowCall := 0
	server := &Server{githubReadinessStore: store, githubReadinessNow: func() time.Time {
		value := times[nowCall]
		nowCall++
		return value
	}}
	target := githubPRReadinessTarget{
		ConnectionID: connection.ID, ExpectedUpdatedAt: connection.UpdatedAt,
		Repo: "acme/site", Branch: "main", credentialKind: publisher.GitHubPRCredentialAdvancedToken, token: "checked-token",
	}
	server.githubReadinessChecker = &fakeGitHubPRReadinessChecker{
		readiness: publisher.GitHubPRReadiness{Status: publisher.GitHubPRReadinessReady}, target: target,
		onCheck: func() {
			if err := server.downgradeGitHubPRReadinessAfterMutationFailure(context.Background(), projectID, target, readinessHTTPStatusError{status: http.StatusForbidden}); err != nil {
				t.Fatalf("persist newer downgrade: %v", err)
			}
		},
	}

	_, err := server.authorizeGitHubPRMutation(context.Background(), projectID)
	if !errors.Is(err, errGitHubPRReadinessChanged) {
		t.Fatalf("older ready authorization err=%v", err)
	}
	if store.connection.PrReadinessStatus != string(publisher.GitHubPRReadinessPermissionMissing) || !store.connection.PrReadinessCheckedAt.Valid || !store.connection.PrReadinessCheckedAt.Time.Equal(newerFailureAt) {
		t.Fatalf("newer downgrade was overwritten: %#v", store.connection)
	}
}

func TestGitHubPRMutationAuthorizationRejectsCASLoss(t *testing.T) {
	projectID := uuid.New()
	connection := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessReady,
		`{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`,
		nil,
		nil,
	)
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: connection}}}
	store.set = func(db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
		return db.PublisherConnection{}, pgx.ErrNoRows
	}
	checker := &fakeGitHubPRReadinessChecker{
		readiness: publisher.GitHubPRReadiness{Status: publisher.GitHubPRReadinessReady},
		target: githubPRReadinessTarget{
			ConnectionID: connection.ID, ExpectedUpdatedAt: connection.UpdatedAt,
			Repo: "acme/site", Branch: "main", token: "stale-token",
		},
	}
	server := &Server{githubReadinessStore: store, githubReadinessChecker: checker}

	_, err := server.authorizeGitHubPRMutation(context.Background(), projectID)

	if !errors.Is(err, errGitHubPRReadinessChanged) {
		t.Fatalf("error = %v, want strict CAS-loss conflict", err)
	}
	if store.getCalls != 1 {
		t.Fatalf("mutation CAS loss reloaded a replacement target: get calls = %d", store.getCalls)
	}
}

func TestGitHubPRMutationFailureDowngradesExactReadinessTarget(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusCode int
		want       publisher.GitHubPRReadinessStatus
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, want: publisher.GitHubPRReadinessPermissionMissing},
		{name: "forbidden", statusCode: http.StatusForbidden, want: publisher.GitHubPRReadinessPermissionMissing},
		{name: "repository missing", statusCode: http.StatusNotFound, want: publisher.GitHubPRReadinessRepositoryUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectID := uuid.New()
			connection := githubPRReadinessConnection(projectID, publisher.GitHubPRReadinessReady, `{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`, nil, nil)
			store := &fakeGitHubPRReadinessStore{}
			store.set = func(params db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
				updated := connection
				updated.PrReadinessStatus = params.PrReadinessStatus
				return updated, nil
			}
			server := &Server{githubReadinessStore: store, githubReadinessNow: func() time.Time {
				return time.Date(2026, 7, 13, 15, 0, 0, 0, time.UTC)
			}}
			target := githubPRReadinessTarget{
				ConnectionID: connection.ID, ExpectedUpdatedAt: connection.UpdatedAt,
				Repo: "acme/site", Branch: "main", token: "private-token",
			}

			if err := server.downgradeGitHubPRReadinessAfterMutationFailure(context.Background(), projectID, target, readinessHTTPStatusError{status: tc.statusCode}); err != nil {
				t.Fatalf("downgrade readiness: %v", err)
			}
			if store.setCalls != 1 {
				t.Fatalf("set calls = %d", store.setCalls)
			}
			params := store.setParams[0]
			if params.PrReadinessStatus != string(tc.want) || params.ConnectionID != connection.ID || params.ExpectedUpdatedAt != connection.UpdatedAt {
				t.Fatalf("params = %#v", params)
			}
			if params.PrReadinessDetail == nil || strings.Contains(*params.PrReadinessDetail, "ghp_secret") || strings.Contains(*params.PrReadinessDetail, "raw upstream") {
				t.Fatalf("unsafe/missing detail = %#v", params.PrReadinessDetail)
			}
		})
	}
}

func TestGitHubPRMutationFailureIgnoresCASLossAndUnclassifiedStatus(t *testing.T) {
	projectID := uuid.New()
	target := githubPRReadinessTarget{ConnectionID: uuid.New(), ExpectedUpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
	store := &fakeGitHubPRReadinessStore{set: func(db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
		return db.PublisherConnection{}, pgx.ErrNoRows
	}}
	server := &Server{githubReadinessStore: store}
	if err := server.downgradeGitHubPRReadinessAfterMutationFailure(context.Background(), projectID, target, readinessHTTPStatusError{status: http.StatusForbidden}); err != nil {
		t.Fatalf("CAS loss must preserve newer connection state: %v", err)
	}
	if err := server.downgradeGitHubPRReadinessAfterMutationFailure(context.Background(), projectID, target, readinessHTTPStatusError{status: http.StatusInternalServerError}); err != nil {
		t.Fatalf("unclassified error: %v", err)
	}
	if store.setCalls != 1 {
		t.Fatalf("unclassified status persisted downgrade; calls=%d", store.setCalls)
	}
}

func TestEnsureGitHubPRTargetCurrentAcceptsOnlyExactReadyConnectionVersion(t *testing.T) {
	projectID := uuid.New()
	connection := githubPRReadinessConnection(projectID, publisher.GitHubPRReadinessReady, `{"repo":"acme/site","branch":"release","base_url":"https://example.com/blog"}`, nil, nil)
	target := githubPRReadinessTarget{
		ConnectionID: connection.ID, ExpectedUpdatedAt: connection.UpdatedAt,
		Repo: "acme/site", Branch: "release", token: "private-token",
	}
	for _, tc := range []struct {
		name    string
		mutate  func(*db.PublisherConnection)
		wantErr bool
	}{
		{name: "exact"},
		{name: "version changed", mutate: func(connection *db.PublisherConnection) {
			connection.UpdatedAt.Time = connection.UpdatedAt.Time.Add(time.Second)
		}, wantErr: true},
		{name: "repository changed", mutate: func(connection *db.PublisherConnection) {
			connection.Config = json.RawMessage(`{"repo":"other/site","branch":"release","base_url":"https://example.com/blog"}`)
		}, wantErr: true},
		{name: "branch changed", mutate: func(connection *db.PublisherConnection) {
			connection.Config = json.RawMessage(`{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`)
		}, wantErr: true},
		{name: "readiness invalidated", mutate: func(connection *db.PublisherConnection) {
			connection.PrReadinessStatus = string(publisher.GitHubPRReadinessNotChecked)
		}, wantErr: true},
		{name: "connection disabled", mutate: func(connection *db.PublisherConnection) {
			connection.Enabled = false
		}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := connection
			if tc.mutate != nil {
				tc.mutate(&current)
			}
			store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: current}}}
			checker := &fakeGitHubPRReadinessChecker{}
			server := &Server{githubReadinessStore: store, githubReadinessChecker: checker}

			err := server.ensureGitHubPRTargetCurrent(context.Background(), projectID, target)

			if tc.wantErr != errors.Is(err, errGitHubPRReadinessChanged) {
				t.Fatalf("error = %v, want conflict=%v", err, tc.wantErr)
			}
			if checker.calls != 0 {
				t.Fatalf("connection fence performed live check %d times", checker.calls)
			}
		})
	}
}

type fakeGitHubPRReadinessChecker struct {
	readiness publisher.GitHubPRReadiness
	target    githubPRReadinessTarget
	err       error
	calls     int
	onCheck   func()
}

type capturingGitHubPRReadinessChecker struct {
	delegate  githubPRReadinessChecker
	readiness publisher.GitHubPRReadiness
	target    githubPRReadinessTarget
	err       error
}

func (c *capturingGitHubPRReadinessChecker) Check(ctx context.Context, projectID uuid.UUID) (publisher.GitHubPRReadiness, githubPRReadinessTarget, error) {
	c.readiness, c.target, c.err = c.delegate.Check(ctx, projectID)
	return c.readiness, c.target, c.err
}

func (f *fakeGitHubPRReadinessChecker) Check(context.Context, uuid.UUID) (publisher.GitHubPRReadiness, githubPRReadinessTarget, error) {
	f.calls++
	if f.onCheck != nil {
		f.onCheck()
	}
	return f.readiness, f.target, f.err
}

type fakeGitHubPRReadinessStoreResult struct {
	connection db.PublisherConnection
	err        error
}

type fakeGitHubPRReadinessStore struct {
	getResults []fakeGitHubPRReadinessStoreResult
	getCalls   int
	set        func(db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error)
	setParams  []db.SetGitHubPRReadinessIfUnchangedParams
	setCalls   int
}

func (f *fakeGitHubPRReadinessStore) GetGitHubPRReadinessForProject(context.Context, uuid.UUID) (db.PublisherConnection, error) {
	index := f.getCalls
	f.getCalls++
	if index >= len(f.getResults) {
		return db.PublisherConnection{}, pgx.ErrNoRows
	}
	return f.getResults[index].connection, f.getResults[index].err
}

func (f *fakeGitHubPRReadinessStore) SetGitHubPRReadinessIfUnchanged(_ context.Context, params db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
	f.setCalls++
	f.setParams = append(f.setParams, params)
	if f.set == nil {
		return db.PublisherConnection{}, errors.New("unexpected readiness persistence")
	}
	return f.set(params)
}

// statefulGitHubPRReadinessStore mirrors the database readiness CAS closely
// enough for ordering and version-fence tests. Readiness observations advance
// only their checked_at value; they do not mutate the connection/config
// version represented by updated_at.
type statefulGitHubPRReadinessStore struct {
	connection db.PublisherConnection
}

func (s *statefulGitHubPRReadinessStore) GetGitHubPRReadinessForProject(_ context.Context, projectID uuid.UUID) (db.PublisherConnection, error) {
	if s.connection.ProjectID != projectID {
		return db.PublisherConnection{}, pgx.ErrNoRows
	}
	return s.connection, nil
}

func (s *statefulGitHubPRReadinessStore) SetGitHubPRReadinessIfUnchanged(_ context.Context, params db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
	if s.connection.ID != params.ConnectionID || s.connection.ProjectID != params.ProjectID ||
		s.connection.UpdatedAt != params.ExpectedUpdatedAt || !params.PrReadinessCheckedAt.Valid ||
		(s.connection.PrReadinessCheckedAt.Valid && !s.connection.PrReadinessCheckedAt.Time.Before(params.PrReadinessCheckedAt.Time)) {
		return db.PublisherConnection{}, pgx.ErrNoRows
	}
	s.connection.PrReadinessStatus = params.PrReadinessStatus
	s.connection.PrReadinessCheckedAt = params.PrReadinessCheckedAt
	s.connection.PrReadinessDetail = params.PrReadinessDetail
	return s.connection, nil
}

func githubPRReadinessConnection(
	projectID uuid.UUID,
	status publisher.GitHubPRReadinessStatus,
	config string,
	checkedAt *time.Time,
	detail *string,
) db.PublisherConnection {
	connection := db.PublisherConnection{
		ID:                uuid.New(),
		ProjectID:         projectID,
		Kind:              publisher.ConnectionKindGitHubNextJS,
		Status:            "connected",
		IsDefault:         true,
		Enabled:           true,
		Config:            json.RawMessage(config),
		UpdatedAt:         pgtype.Timestamptz{Time: time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC), Valid: true},
		PrReadinessStatus: string(status),
		PrReadinessDetail: detail,
	}
	if checkedAt != nil {
		connection.PrReadinessCheckedAt = pgtype.Timestamptz{Time: *checkedAt, Valid: true}
	}
	return connection
}

func apiGitHubReadinessResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

type apiGitHubReadinessRoundTripFunc func(*http.Request) (*http.Response, error)

func (f apiGitHubReadinessRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type readinessHTTPStatusError struct {
	status int
}

type readinessCredentialErrorDB struct {
	err error
}

func (db readinessCredentialErrorDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, db.err
}

func (db readinessCredentialErrorDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, db.err
}

func (db readinessCredentialErrorDB) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return readinessCredentialErrorRow{err: db.err}
}

type readinessCredentialErrorRow struct {
	err error
}

func (row readinessCredentialErrorRow) Scan(...interface{}) error {
	return row.err
}

func (e readinessHTTPStatusError) Error() string {
	return "ghp_secret Authorization: Bearer raw upstream body"
}

func (e readinessHTTPStatusError) StatusCode() int {
	return e.status
}

func readinessSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

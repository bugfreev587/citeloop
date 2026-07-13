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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

type fakeGitHubPRReadinessChecker struct {
	readiness publisher.GitHubPRReadiness
	target    githubPRReadinessTarget
	err       error
	calls     int
}

func (f *fakeGitHubPRReadinessChecker) Check(context.Context, uuid.UUID) (publisher.GitHubPRReadiness, githubPRReadinessTarget, error) {
	f.calls++
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

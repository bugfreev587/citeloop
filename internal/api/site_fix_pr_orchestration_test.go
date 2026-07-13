package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/sitefix"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestApproveDoctorSiteFixRejectsStoredNonReadyBeforeLiveCheckOrApproval(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	connection := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessNotChecked,
		`{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`,
		nil,
		nil,
	)
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: connection}}}
	checker := &fakeGitHubPRReadinessChecker{}
	service := &doctorSiteFixServiceStub{}
	runnerCalls := 0
	server := &Server{
		SiteFixes:              service,
		githubReadinessStore:   store,
		githubReadinessChecker: checker,
		canonicalSiteFixPRRunner: func(context.Context, uuid.UUID, uuid.UUID, githubPRReadinessTarget) (sitefix.ApplyResult, error) {
			runnerCalls++
			return sitefix.ApplyResult{}, nil
		},
	}

	response := serveApproveOrApplySiteFix(t, server, projectID, fixID, "approve")

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if checker.calls != 0 || service.approveCalls != 0 || runnerCalls != 0 {
		t.Fatalf("non-ready gate calls checker=%d approve=%d runner=%d", checker.calls, service.approveCalls, runnerCalls)
	}
}

func TestApproveDoctorSiteFixPersistsLiveDowngradeBeforeApproval(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	connection := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessReady,
		`{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`,
		nil,
		nil,
	)
	store := mutationReadinessStore(connection)
	checker := &fakeGitHubPRReadinessChecker{
		readiness: publisher.GitHubPRReadiness{Status: publisher.GitHubPRReadinessRepositoryUnavailable},
		target: githubPRReadinessTarget{
			ConnectionID: connection.ID, ExpectedUpdatedAt: connection.UpdatedAt,
			Repo: "acme/site", Branch: "main", token: "checked-token",
		},
	}
	service := &doctorSiteFixServiceStub{}
	server := &Server{SiteFixes: service, githubReadinessStore: store, githubReadinessChecker: checker}

	response := serveApproveOrApplySiteFix(t, server, projectID, fixID, "approve")

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if store.setCalls != 1 || store.setParams[0].PrReadinessStatus != string(publisher.GitHubPRReadinessRepositoryUnavailable) {
		t.Fatalf("persisted readiness = %#v", store.setParams)
	}
	if service.approveCalls != 0 {
		t.Fatalf("live downgrade approved fix %d times", service.approveCalls)
	}
}

func TestApproveDoctorSiteFixChecksThenApprovesAndCreatesPRInOneRequest(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	connection := githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessReady,
		`{"repo":"acme/site","branch":"release","base_url":"https://example.com/blog"}`,
		nil,
		nil,
	)
	store := mutationReadinessStore(connection)
	events := make([]string, 0, 3)
	checker := &fakeGitHubPRReadinessChecker{
		readiness: publisher.GitHubPRReadiness{Status: publisher.GitHubPRReadinessReady},
		target: githubPRReadinessTarget{
			ConnectionID: connection.ID, ExpectedUpdatedAt: connection.UpdatedAt,
			Repo: "acme/site", Branch: "release",
			credentialKind: publisher.GitHubPRCredentialAdvancedToken,
			token:          "ghp_exact_checked_token",
		},
		onCheck: func() { events = append(events, "live_check") },
	}
	approvedAt := time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC)
	approvedFix := db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", ApprovedAt: pgtype.Timestamptz{Time: approvedAt, Valid: true}}
	service := &doctorSiteFixServiceStub{
		approveFix: DoctorSiteFixResponse{SiteFix: approvedFix},
		onApprove:  func() { events = append(events, "approve") },
	}
	prURL := "https://github.com/acme/site/pull/42"
	result := sitefix.ApplyResult{
		SiteFix: approvedFix,
		Application: db.SiteChangeApplication{
			ID: uuid.New(), ProjectID: projectID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
			Status: "github_pr_open", GithubPrUrl: &prURL,
		},
	}
	var gotTarget githubPRReadinessTarget
	server := &Server{
		SiteFixes:              service,
		githubReadinessStore:   store,
		githubReadinessChecker: checker,
		githubReadinessNow:     func() time.Time { return approvedAt.Add(-time.Minute) },
		canonicalSiteFixPRRunner: func(_ context.Context, gotProjectID, gotFixID uuid.UUID, target githubPRReadinessTarget) (sitefix.ApplyResult, error) {
			events = append(events, "create_pr")
			if gotProjectID != projectID || gotFixID != fixID {
				t.Fatalf("runner scope = %s/%s", gotProjectID, gotFixID)
			}
			gotTarget = target
			return result, nil
		},
	}

	response := serveApproveOrApplySiteFix(t, server, projectID, fixID, "approve")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := strings.Join(events, ","); got != "live_check,approve,create_pr" {
		t.Fatalf("operation order = %s", got)
	}
	if gotTarget.ConnectionID != connection.ID || gotTarget.Repo != "acme/site" || gotTarget.Branch != "release" || gotTarget.token != "ghp_exact_checked_token" {
		t.Fatalf("runner target = %#v", gotTarget)
	}
	if gotTarget.ExpectedUpdatedAt != connection.UpdatedAt {
		t.Fatalf("readiness-only check changed connection version %#v", gotTarget.ExpectedUpdatedAt)
	}
	var responseResult sitefix.ApplyResult
	if err := json.NewDecoder(response.Body).Decode(&responseResult); err != nil {
		t.Fatal(err)
	}
	if responseResult.SiteFix.Status != "approved" || responseResult.Application.GithubPrUrl == nil || *responseResult.Application.GithubPrUrl != prURL {
		t.Fatalf("response = %#v", responseResult)
	}
}

func TestApproveDoctorSiteFixKeepsDurableApprovalWhenPRCreationFails(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	connection := readyMutationConnection(projectID)
	store := mutationReadinessStore(connection)
	checker := readyMutationChecker(connection)
	service := &doctorSiteFixServiceStub{approveFix: DoctorSiteFixResponse{SiteFix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved"}}}
	rawFailure := errors.New("github Authorization Bearer ghp_secret raw response")
	server := &Server{
		SiteFixes:              service,
		githubReadinessStore:   store,
		githubReadinessChecker: checker,
		canonicalSiteFixPRRunner: func(context.Context, uuid.UUID, uuid.UUID, githubPRReadinessTarget) (sitefix.ApplyResult, error) {
			return sitefix.ApplyResult{}, rawFailure
		},
	}

	response := serveApproveOrApplySiteFix(t, server, projectID, fixID, "approve")

	if response.Code != http.StatusInternalServerError || service.approveCalls != 1 {
		t.Fatalf("status=%d approve=%d body=%s", response.Code, service.approveCalls, response.Body.String())
	}
	for _, unsafe := range []string{"Authorization", "Bearer", "ghp_secret", "raw response"} {
		if strings.Contains(response.Body.String(), unsafe) {
			t.Fatalf("response leaked %q: %s", unsafe, response.Body.String())
		}
	}
}

func TestApplyDoctorSiteFixRecoveryUsesSameReadinessGateWithoutApprovingAgain(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	connection := readyMutationConnection(projectID)
	store := mutationReadinessStore(connection)
	checker := readyMutationChecker(connection)
	service := &doctorSiteFixServiceStub{}
	runnerCalls := 0
	server := &Server{
		SiteFixes:              service,
		githubReadinessStore:   store,
		githubReadinessChecker: checker,
		canonicalSiteFixPRRunner: func(context.Context, uuid.UUID, uuid.UUID, githubPRReadinessTarget) (sitefix.ApplyResult, error) {
			runnerCalls++
			return sitefix.ApplyResult{SiteFix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "applying"}}, nil
		},
	}

	response := serveApproveOrApplySiteFix(t, server, projectID, fixID, "apply")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if checker.calls != 1 || runnerCalls != 1 || service.approveCalls != 0 {
		t.Fatalf("recovery calls checker=%d runner=%d approve=%d", checker.calls, runnerCalls, service.approveCalls)
	}
}

func TestCanonicalSiteFixPRRetryEligibilityNeverReopensPRBackedApplication(t *testing.T) {
	retryable := "github_pr_failed"
	prURL := "https://github.com/acme/site/pull/42"
	closed := "closed"
	for _, tc := range []struct {
		name string
		app  db.SiteChangeApplication
		want bool
	}{
		{
			name: "retryable creation failure without PR",
			app:  db.SiteChangeApplication{Status: "needs_follow_up", FailureReason: &retryable},
			want: true,
		},
		{
			name: "closed PR retains review record",
			app:  db.SiteChangeApplication{Status: "needs_follow_up", FailureReason: &retryable, GithubPrUrl: &prURL, GithubPrState: &closed},
			want: false,
		},
		{
			name: "PR URL alone fences retry",
			app:  db.SiteChangeApplication{Status: "needs_follow_up", FailureReason: &retryable, GithubPrUrl: &prURL},
			want: false,
		},
		{
			name: "verification follow up is not PR creation",
			app:  db.SiteChangeApplication{Status: "needs_follow_up", FailureReason: stringPointer("deployment_not_observed")},
			want: false,
		},
		{
			name: "wrong state",
			app:  db.SiteChangeApplication{Status: "github_pr_open"},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := canReopenCanonicalSiteFixPRCreation(tc.app); got != tc.want {
				t.Fatalf("eligible = %v, want %v for %#v", got, tc.want, tc.app)
			}
		})
	}
}

func TestCanonicalSiteFixPRClaimCollisionNeverTreatsActiveOrIncompletePRAsSuccess(t *testing.T) {
	url := "https://github.com/acme/site/pull/42"
	for _, testCase := range []struct {
		name string
		app  db.SiteChangeApplication
		want bool
	}{
		{name: "active competing claim", app: db.SiteChangeApplication{Status: "creating_pr"}},
		{name: "open status without URL", app: db.SiteChangeApplication{Status: "github_pr_open"}},
		{name: "open PR", app: db.SiteChangeApplication{Status: "github_pr_open", GithubPrUrl: &url}, want: true},
		{name: "closed PR history", app: db.SiteChangeApplication{Status: "needs_follow_up", GithubPrUrl: &url, GithubPrState: stringPointer("closed")}, want: true},
		{name: "merged PR history", app: db.SiteChangeApplication{Status: "deployment_pending", GithubPrUrl: &url, GithubPrState: stringPointer("merged")}, want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := canonicalSiteFixPRClaimCollisionIsComplete(testCase.app); got != testCase.want {
				t.Fatalf("complete = %v, want %v for %#v", got, testCase.want, testCase.app)
			}
		})
	}
}

func TestCanonicalSiteFixPRResponseRejectsNonReopenableFollowUpWithoutPR(t *testing.T) {
	exhausted := "repository_reprepare_exhausted"
	result := sitefix.ApplyResult{
		SiteFix:     db.SiteFix{Status: "applying"},
		Application: db.SiteChangeApplication{Status: "needs_follow_up", FailureReason: &exhausted},
	}
	if err := validateCanonicalSiteFixPRResponse(result); !errors.Is(err, sitefix.ErrLifecycleConflict) {
		t.Fatalf("err = %v", err)
	}
	result.Application = db.SiteChangeApplication{Status: "creating_pr"}
	if err := validateCanonicalSiteFixPRResponse(result); !errors.Is(err, sitefix.ErrLifecycleConflict) || !strings.Contains(err.Error(), "in progress") {
		t.Fatalf("active claim err = %v", err)
	}

	url := "https://github.com/acme/site/pull/42"
	result.Application = db.SiteChangeApplication{Status: "needs_follow_up", FailureReason: &exhausted}
	result.Application.GithubPrUrl = &url
	result.Application.GithubPrState = stringPointer("closed")
	if err := validateCanonicalSiteFixPRResponse(result); err != nil {
		t.Fatalf("PR-backed closed history rejected: %v", err)
	}
}

func TestCreateCanonicalSiteFixPRReturnsConflictForNonReopenableFollowUpWithoutPR(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	exhausted := "repository_reprepare_exhausted"
	server := &Server{SiteFixLifecycle: &doctorSiteFixLifecycleServiceStub{applyResult: sitefix.ApplyResult{
		SiteFix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "applying"},
		Application: db.SiteChangeApplication{
			ID: uuid.New(), ProjectID: projectID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
			Status: "needs_follow_up", FailureReason: &exhausted,
		},
	}}}

	_, err := server.createCanonicalSiteFixPR(context.Background(), projectID, fixID, githubPRReadinessTarget{})

	if !errors.Is(err, sitefix.ErrLifecycleConflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestCanonicalSiteFixPRFreshReprepareRetriesOnceAndReturnsSecondPR(t *testing.T) {
	applyCalls, openCalls := 0, 0
	firstAppID, secondAppID := uuid.New(), uuid.New()
	fixID := uuid.New()
	branches := make([]string, 0, 2)
	result, err := runCanonicalSiteFixPRAttempts(
		context.Background(),
		func(context.Context) (sitefix.ApplyResult, error) {
			applyCalls++
			appID := firstAppID
			if applyCalls == 2 {
				appID = secondAppID
			}
			return sitefix.ApplyResult{Application: db.SiteChangeApplication{ID: appID, Status: "ready_for_pr"}}, nil
		},
		func(_ context.Context, prepared sitefix.ApplyResult) (sitefix.ApplyResult, error) {
			openCalls++
			branches = append(branches, siteFixRepositoryWorkingBranch(fixID, prepared.Application.ID))
			if openCalls == 1 {
				return prepared, fmt.Errorf("%w: repository source changed", errCanonicalSiteFixFreshReprepare)
			}
			url := "https://github.com/acme/site/pull/42"
			prepared.Application.Status = "github_pr_open"
			prepared.Application.GithubPrUrl = &url
			return prepared, nil
		},
	)

	if err != nil {
		t.Fatalf("run attempts: %v", err)
	}
	if applyCalls != 2 || openCalls != 2 {
		t.Fatalf("calls apply=%d open=%d", applyCalls, openCalls)
	}
	if result.Application.ID != secondAppID || result.Application.GithubPrUrl == nil {
		t.Fatalf("result = %#v", result)
	}
	if len(branches) != 2 || branches[0] == branches[1] {
		t.Fatalf("fresh preparation branches = %#v", branches)
	}
	if branches[1] != siteFixRepositoryWorkingBranch(fixID, secondAppID) {
		t.Fatalf("second branch = %q", branches[1])
	}
}

func TestCanonicalSiteFixPRFreshReprepareNeverRunsAThirdAttempt(t *testing.T) {
	applyCalls, openCalls := 0, 0
	_, err := runCanonicalSiteFixPRAttempts(
		context.Background(),
		func(context.Context) (sitefix.ApplyResult, error) {
			applyCalls++
			return sitefix.ApplyResult{Application: db.SiteChangeApplication{ID: uuid.New(), Status: "ready_for_pr"}}, nil
		},
		func(_ context.Context, result sitefix.ApplyResult) (sitefix.ApplyResult, error) {
			openCalls++
			return result, errCanonicalSiteFixFreshReprepare
		},
	)

	if !errors.Is(err, errCanonicalSiteFixFreshReprepare) {
		t.Fatalf("err = %v", err)
	}
	if applyCalls != 2 || openCalls != 2 {
		t.Fatalf("bounded calls apply=%d open=%d", applyCalls, openCalls)
	}
}

func TestCanonicalSiteFixPRLegacyExactReconciliationIsFailSafe(t *testing.T) {
	fixID, appID := uuid.New(), uuid.New()
	legacy := legacySiteFixRepositoryWorkingBranch(fixID)
	primary := siteFixRepositoryWorkingBranch(fixID, appID)
	legacyURL := "https://github.com/acme/site/pull/41"
	client := &canonicalSiteFixPRClientStub{
		foundBranches: map[string]bool{legacy: true},
		createResults: map[string]publisher.GitHubPRResult{legacy: {Number: 41, URL: legacyURL, State: "open"}},
	}
	input := publisher.GitHubFileUpdatesPRInput{WorkingBranch: primary}

	pr, workingBranch, err := createCanonicalSiteFixPRWithLegacyReconciliation(context.Background(), client, legacy, input)

	if err != nil || workingBranch != legacy || pr.URL != legacyURL {
		t.Fatalf("pr=%+v branch=%q err=%v", pr, workingBranch, err)
	}
	if got := strings.Join(client.createBranches, ","); got != legacy {
		t.Fatalf("created branches = %q", got)
	}
}

func TestCanonicalSiteFixPRDivergentLegacyDoesNotBlockApplicationBranch(t *testing.T) {
	fixID, appID := uuid.New(), uuid.New()
	legacy := legacySiteFixRepositoryWorkingBranch(fixID)
	primary := siteFixRepositoryWorkingBranch(fixID, appID)
	primaryURL := "https://github.com/acme/site/pull/42"
	client := &canonicalSiteFixPRClientStub{
		foundBranches: map[string]bool{legacy: true},
		createErrors:  map[string]error{legacy: fmt.Errorf("%w: old target tree", publisher.ErrDivergentPullRequest)},
		createResults: map[string]publisher.GitHubPRResult{primary: {Number: 42, URL: primaryURL, State: "open"}},
	}
	input := publisher.GitHubFileUpdatesPRInput{WorkingBranch: primary}

	pr, workingBranch, err := createCanonicalSiteFixPRWithLegacyReconciliation(context.Background(), client, legacy, input)

	if err != nil || workingBranch != primary || pr.URL != primaryURL {
		t.Fatalf("pr=%+v branch=%q err=%v", pr, workingBranch, err)
	}
	if got := strings.Join(client.createBranches, ","); got != legacy+","+primary {
		t.Fatalf("created branches = %q", got)
	}
}

func TestCanonicalSiteFixPRResponseReloadsOpenClosedAndMergedFixState(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	for _, testCase := range []struct {
		prState        string
		appStatus      string
		persistedState string
	}{
		{prState: "open", appStatus: "github_pr_open", persistedState: "applying"},
		{prState: "closed", appStatus: "needs_follow_up", persistedState: "applying"},
		{prState: "merged", appStatus: "deployment_pending", persistedState: "awaiting_deploy"},
	} {
		t.Run(testCase.prState, func(t *testing.T) {
			store := &canonicalSiteFixByIDStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: testCase.persistedState}}
			result := sitefix.ApplyResult{
				SiteFix:     db.SiteFix{ID: fixID, ProjectID: projectID, Status: "applying"},
				Application: db.SiteChangeApplication{Status: testCase.appStatus, GithubPrState: stringPointer(testCase.prState)},
			}

			got, err := reloadCanonicalSiteFixAfterPRObservation(context.Background(), store, result)

			if err != nil || got.SiteFix.Status != testCase.persistedState || got.Application.Status != testCase.appStatus {
				t.Fatalf("result=%#v err=%v", got, err)
			}
			if store.params.ID != fixID || store.params.ProjectID != projectID {
				t.Fatalf("reload scope = %#v", store.params)
			}
		})
	}
}

func TestApproveCanonicalSiteFixIdempotentlyReturnsAlreadyApprovedRevision(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	approvedAt := pgtype.Timestamptz{Time: time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC), Valid: true}
	approved := db.SiteFix{ID: fixID, ProjectID: projectID, Status: "applying", ApprovedAt: approvedAt}
	store := &approvalStoreStub{getResults: []approvalStoreResult{{fix: approved}}}

	got, err := approveCanonicalSiteFixIdempotently(context.Background(), store, projectID, fixID, approvedAt.Time.Add(time.Hour))

	if err != nil || got.Status != "applying" || got.ApprovedAt != approvedAt {
		t.Fatalf("fix=%#v err=%v", got, err)
	}
	if store.approveCalls != 0 {
		t.Fatalf("already-approved revision executed transition %d times", store.approveCalls)
	}
}

func TestApproveCanonicalSiteFixIdempotentlyRecoversConcurrentApproval(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	approvedAt := pgtype.Timestamptz{Time: time.Date(2026, 7, 13, 14, 5, 0, 0, time.UTC), Valid: true}
	store := &approvalStoreStub{
		getResults: []approvalStoreResult{
			{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "proposed"}},
			{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", ApprovedAt: approvedAt}},
		},
		approveErr: pgx.ErrNoRows,
	}

	got, err := approveCanonicalSiteFixIdempotently(context.Background(), store, projectID, fixID, approvedAt.Time)

	if err != nil || got.Status != "approved" || got.ApprovedAt != approvedAt {
		t.Fatalf("fix=%#v err=%v", got, err)
	}
	if store.approveCalls != 1 || store.getCalls != 2 {
		t.Fatalf("calls approve=%d get=%d", store.approveCalls, store.getCalls)
	}
}

type approvalStoreResult struct {
	fix db.SiteFix
	err error
}

type canonicalSiteFixByIDStoreStub struct {
	fix    db.SiteFix
	err    error
	params db.GetCanonicalSiteFixParams
}

type canonicalSiteFixPRClientStub struct {
	foundBranches  map[string]bool
	findErrors     map[string]error
	createResults  map[string]publisher.GitHubPRResult
	createErrors   map[string]error
	createBranches []string
}

func (s *canonicalSiteFixPRClientStub) FindPullRequestByHead(_ context.Context, branch string) (publisher.GitHubPRResult, bool, error) {
	return publisher.GitHubPRResult{}, s.foundBranches[branch], s.findErrors[branch]
}

func (s *canonicalSiteFixPRClientStub) CreateFileUpdatesPR(_ context.Context, input publisher.GitHubFileUpdatesPRInput) (publisher.GitHubPRResult, error) {
	s.createBranches = append(s.createBranches, input.WorkingBranch)
	return s.createResults[input.WorkingBranch], s.createErrors[input.WorkingBranch]
}

func (s *canonicalSiteFixByIDStoreStub) GetCanonicalSiteFix(_ context.Context, params db.GetCanonicalSiteFixParams) (db.SiteFix, error) {
	s.params = params
	return s.fix, s.err
}

type approvalStoreStub struct {
	getResults   []approvalStoreResult
	getCalls     int
	approveFix   db.ApproveCanonicalSiteFixRow
	approveErr   error
	approveCalls int
}

func (s *approvalStoreStub) GetCanonicalSiteFix(context.Context, db.GetCanonicalSiteFixParams) (db.SiteFix, error) {
	index := s.getCalls
	s.getCalls++
	if index >= len(s.getResults) {
		return db.SiteFix{}, pgx.ErrNoRows
	}
	return s.getResults[index].fix, s.getResults[index].err
}

func (s *approvalStoreStub) ApproveCanonicalSiteFix(context.Context, db.ApproveCanonicalSiteFixParams) (db.ApproveCanonicalSiteFixRow, error) {
	s.approveCalls++
	return s.approveFix, s.approveErr
}

func stringPointer(value string) *string { return &value }

func serveApproveOrApplySiteFix(t *testing.T, server *Server, projectID, fixID uuid.UUID, action string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/site-fixes/"+fixID.String()+"/"+action, nil)
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	return response
}

func readyMutationConnection(projectID uuid.UUID) db.PublisherConnection {
	return githubPRReadinessConnection(
		projectID,
		publisher.GitHubPRReadinessReady,
		`{"repo":"acme/site","branch":"main","base_url":"https://example.com/blog"}`,
		nil,
		nil,
	)
}

func readyMutationChecker(connection db.PublisherConnection) *fakeGitHubPRReadinessChecker {
	return &fakeGitHubPRReadinessChecker{
		readiness: publisher.GitHubPRReadiness{Status: publisher.GitHubPRReadinessReady},
		target: githubPRReadinessTarget{
			ConnectionID: connection.ID, ExpectedUpdatedAt: connection.UpdatedAt,
			Repo: "acme/site", Branch: "main",
			credentialKind: publisher.GitHubPRCredentialAdvancedToken,
			token:          "checked-token",
		},
	}
}

func mutationReadinessStore(connection db.PublisherConnection) *fakeGitHubPRReadinessStore {
	store := &fakeGitHubPRReadinessStore{getResults: []fakeGitHubPRReadinessStoreResult{{connection: connection}}}
	store.set = func(params db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error) {
		updated := connection
		updated.PrReadinessStatus = params.PrReadinessStatus
		updated.PrReadinessCheckedAt = params.PrReadinessCheckedAt
		updated.PrReadinessDetail = params.PrReadinessDetail
		return updated, nil
	}
	return store
}

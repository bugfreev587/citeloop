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

func TestApproveCanonicalSiteFixWithMeasurementCreatesRequiredGenerationExactlyOnce(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	approvedAt := time.Date(2026, 5, 29, 1, 0, 0, 0, time.UTC)
	proposed := requiredMeasurementSiteFixForApprovalTest(projectID, fixID, "proposed", approvedAt)
	approved := proposed
	approved.Status = "approved"
	approved.ApprovedAt = pgtype.Timestamptz{Time: approvedAt, Valid: true}
	measurement := db.SiteFixMeasurement{ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: 1}
	store := &measurementApprovalStoreStub{
		getResults:  []approvalStoreResult{{fix: proposed}, {fix: approved}},
		measurement: measurement,
	}

	got, err := approveCanonicalSiteFixWithMeasurementIdempotently(context.Background(), store, projectID, fixID, approvedAt)
	if err != nil || got.Status != "approved" {
		t.Fatalf("fix=%+v err=%v", got, err)
	}
	if store.approveCalls != 1 || store.createCalls != 1 {
		t.Fatalf("approve=%d create=%d", store.approveCalls, store.createCalls)
	}
	if store.createParams.CreationIdempotencyKey != "approval-required-v1:"+fixID.String() || store.createParams.Status != "ready" || store.createParams.BaselineStatus != "ready" {
		t.Fatalf("measurement creation = %+v", store.createParams)
	}

	replay := &measurementApprovalStoreStub{getResults: []approvalStoreResult{{fix: approved}}, measurement: measurement}
	got, err = approveCanonicalSiteFixWithMeasurementIdempotently(context.Background(), replay, projectID, fixID, approvedAt.Add(30*24*time.Hour))
	if err != nil || got.Status != "approved" || replay.approveCalls != 0 || replay.createCalls != 1 || replay.createParams.CreationIdempotencyKey != store.createParams.CreationIdempotencyKey {
		t.Fatalf("replay fix=%+v err=%v approve=%d create=%d params=%+v", got, err, replay.approveCalls, replay.createCalls, replay.createParams)
	}
}

func TestApproveCanonicalSiteFixWithMeasurementRecoversConcurrentApproval(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	approvedAt := time.Date(2026, 5, 29, 1, 0, 0, 0, time.UTC)
	proposed := requiredMeasurementSiteFixForApprovalTest(projectID, fixID, "proposed", approvedAt)
	approved := proposed
	approved.Status = "approved"
	approved.ApprovedAt = pgtype.Timestamptz{Time: approvedAt, Valid: true}
	store := &measurementApprovalStoreStub{
		getResults:  []approvalStoreResult{{fix: proposed}, {fix: approved}},
		approveErr:  pgx.ErrNoRows,
		measurement: db.SiteFixMeasurement{ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: 1},
	}
	got, err := approveCanonicalSiteFixWithMeasurementIdempotently(context.Background(), store, projectID, fixID, approvedAt)
	if err != nil || got.Status != "approved" || store.approveCalls != 1 || store.createCalls != 1 {
		t.Fatalf("fix=%+v err=%v approve=%d create=%d", got, err, store.approveCalls, store.createCalls)
	}
}

func TestApproveCanonicalSiteFixWithMeasurementSkipsVerificationOnlyAndPropagatesCreationFailure(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	approvedAt := time.Date(2026, 5, 29, 1, 0, 0, 0, time.UTC)
	verificationOnly := db.SiteFix{ID: fixID, ProjectID: projectID, Status: "proposed", MeasurementPolicy: "verification_only"}
	approved := verificationOnly
	approved.Status = "approved"
	approved.ApprovedAt = pgtype.Timestamptz{Time: approvedAt, Valid: true}
	store := &measurementApprovalStoreStub{getResults: []approvalStoreResult{{fix: verificationOnly}, {fix: approved}}}
	if _, err := approveCanonicalSiteFixWithMeasurementIdempotently(context.Background(), store, projectID, fixID, approvedAt); err != nil {
		t.Fatal(err)
	}
	if store.createCalls != 0 {
		t.Fatalf("verification-only approval created %d measurements", store.createCalls)
	}

	required := requiredMeasurementSiteFixForApprovalTest(projectID, fixID, "proposed", approvedAt)
	failed := &measurementApprovalStoreStub{
		getResults: []approvalStoreResult{{fix: required}},
		createErr:  errors.New("measurement insert failed"),
	}
	if _, err := approveCanonicalSiteFixWithMeasurementIdempotently(context.Background(), failed, projectID, fixID, approvedAt); err == nil {
		t.Fatal("measurement creation failure was swallowed")
	}
	if failed.approveCalls != 1 || failed.createCalls != 1 {
		t.Fatalf("transactional sequence approve=%d create=%d", failed.approveCalls, failed.createCalls)
	}
}

func TestApprovalTransactionRollsBackApprovedStatusWhenMeasurementCreationFails(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	approvedAt := time.Date(2026, 5, 29, 1, 0, 0, 0, time.UTC)
	store := &mutatingMeasurementApprovalStore{fix: requiredMeasurementSiteFixForApprovalTest(projectID, fixID, "proposed", approvedAt), createErr: errors.New("insert failed")}
	runner := &rollbackApprovalRunner{store: store}
	service := &postgresDoctorSiteFixService{q: db.New(nil), approvalTx: runner}
	if _, err := service.Approve(context.Background(), projectID, fixID, approvedAt); err == nil {
		t.Fatal("approval unexpectedly committed")
	}
	if runner.committed || store.fix.Status != "proposed" || store.fix.ApprovedAt.Valid {
		t.Fatalf("transaction committed=%v fix=%+v", runner.committed, store.fix)
	}
}

func TestOptionalSiteFixMeasurementOptInIsProspectiveAndIdempotent(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	optedAt := time.Date(2026, 6, 28, 1, 0, 0, 0, time.UTC)
	fix := requiredMeasurementSiteFixForApprovalTest(projectID, fixID, "verified", optedAt.Add(-30*24*time.Hour))
	fix.FixType, fix.ImpactMode, fix.MeasurementPolicy = "metadata_rewrite", "search_visibility", "measurement_optional"
	measurement := db.SiteFixMeasurement{ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: 1, ProspectiveObservation: true, BaselineStatus: "unavailable", Status: "ready", AttributionConfidence: "low"}
	handoff := db.SiteFixMeasurementHandoffOutbox{ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: 1}
	store := &measurementOptInStoreStub{fix: fix, measurement: measurement, handoff: handoff}

	first, err := optInCanonicalSiteFixMeasurementIdempotently(context.Background(), store, projectID, fixID, optedAt)
	if err != nil || first.Measurement.ID != measurement.ID || first.Handoff.Status != "pending" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	if !store.createParams.ProspectiveObservation || store.createParams.BaselineStatus != "unavailable" || store.createParams.Status != "ready" || store.createParams.AttributionConfidence != "low" {
		t.Fatalf("prospective params=%+v", store.createParams)
	}
	if store.createParams.CreationIdempotencyKey != "verified-opt-in-v1:"+fixID.String() || store.enqueueParams.IdempotencyKey != "activate:"+fixID.String()+":1" {
		t.Fatalf("idempotency create=%q handoff=%q", store.createParams.CreationIdempotencyKey, store.enqueueParams.IdempotencyKey)
	}
	second, err := optInCanonicalSiteFixMeasurementIdempotently(context.Background(), store, projectID, fixID, optedAt.Add(time.Hour))
	if err != nil || second.Measurement.ID != first.Measurement.ID || second.Handoff.Status != first.Handoff.Status {
		t.Fatalf("replay=%+v err=%v", second, err)
	}
	if store.getParams.ProjectID != projectID || store.getParams.ID != fixID || store.createCalls != 2 || store.enqueueCalls != 2 {
		t.Fatalf("scope=%+v create=%d enqueue=%d", store.getParams, store.createCalls, store.enqueueCalls)
	}
}

func TestOptionalSiteFixMeasurementOptInRejectsLifecyclePolicyAndReadiness(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	optedAt := time.Date(2026, 6, 28, 1, 0, 0, 0, time.UTC)
	valid := requiredMeasurementSiteFixForApprovalTest(projectID, fixID, "verified", optedAt.Add(-30*24*time.Hour))
	valid.FixType, valid.ImpactMode, valid.MeasurementPolicy = "metadata_rewrite", "search_visibility", "measurement_optional"
	for name, testCase := range map[string]struct {
		mutate        func(*db.SiteFix)
		wantInvariant bool
	}{
		"status":    {mutate: func(f *db.SiteFix) { f.Status = "verifying" }},
		"policy":    {mutate: func(f *db.SiteFix) { f.MeasurementPolicy = "verification_only" }},
		"readiness": {mutate: func(f *db.SiteFix) { f.PrimaryMetric = nil }, wantInvariant: true},
	} {
		t.Run(name, func(t *testing.T) {
			fix := valid
			testCase.mutate(&fix)
			_, err := optInCanonicalSiteFixMeasurementIdempotently(context.Background(), &measurementOptInStoreStub{fix: fix}, projectID, fixID, optedAt)
			if testCase.wantInvariant {
				if !errors.Is(err, sitefix.ErrSiteFixMeasurementPlanInvariant) {
					t.Fatalf("error=%v", err)
				}
			} else if !errors.Is(err, ErrDoctorSiteFixMeasurementOptInConflict) {
				t.Fatalf("error=%v", err)
			}
		})
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

type measurementApprovalStoreStub struct {
	getResults   []approvalStoreResult
	getCalls     int
	approveCalls int
	approveErr   error
	createCalls  int
	createParams db.CreateSiteFixMeasurementParams
	measurement  db.SiteFixMeasurement
	createErr    error
}

type mutatingMeasurementApprovalStore struct {
	fix       db.SiteFix
	createErr error
}

func (s *mutatingMeasurementApprovalStore) GetCanonicalSiteFix(context.Context, db.GetCanonicalSiteFixParams) (db.SiteFix, error) {
	return s.fix, nil
}

func (s *mutatingMeasurementApprovalStore) ApproveCanonicalSiteFix(_ context.Context, params db.ApproveCanonicalSiteFixParams) (db.ApproveCanonicalSiteFixRow, error) {
	s.fix.Status = "approved"
	s.fix.ApprovedAt = params.ApprovedAt
	return db.ApproveCanonicalSiteFixRow{}, nil
}

func (s *mutatingMeasurementApprovalStore) CreateSiteFixMeasurement(context.Context, db.CreateSiteFixMeasurementParams) (db.SiteFixMeasurement, error) {
	return db.SiteFixMeasurement{}, s.createErr
}

type rollbackApprovalRunner struct {
	store     *mutatingMeasurementApprovalStore
	committed bool
}

func (r *rollbackApprovalRunner) Run(ctx context.Context, fn func(canonicalSiteFixApprovalMeasurementStore) error) error {
	before := r.store.fix
	if err := fn(r.store); err != nil {
		r.store.fix = before
		return err
	}
	r.committed = true
	return nil
}

type measurementOptInStoreStub struct {
	fix           db.SiteFix
	getErr        error
	getParams     db.GetCanonicalSiteFixParams
	measurement   db.SiteFixMeasurement
	createErr     error
	createCalls   int
	createParams  db.CreateSiteFixMeasurementParams
	handoff       db.SiteFixMeasurementHandoffOutbox
	enqueueErr    error
	enqueueCalls  int
	enqueueParams db.EnqueueSiteFixMeasurementHandoffParams
}

func (s *measurementOptInStoreStub) GetCanonicalSiteFix(_ context.Context, params db.GetCanonicalSiteFixParams) (db.SiteFix, error) {
	s.getParams = params
	return s.fix, s.getErr
}

func (s *measurementOptInStoreStub) CreateSiteFixMeasurement(_ context.Context, params db.CreateSiteFixMeasurementParams) (db.SiteFixMeasurement, error) {
	s.createCalls++
	s.createParams = params
	return s.measurement, s.createErr
}

func (s *measurementOptInStoreStub) EnqueueSiteFixMeasurementHandoff(_ context.Context, params db.EnqueueSiteFixMeasurementHandoffParams) (db.SiteFixMeasurementHandoffOutbox, error) {
	s.enqueueCalls++
	s.enqueueParams = params
	return s.handoff, s.enqueueErr
}

func (s *measurementApprovalStoreStub) GetCanonicalSiteFix(context.Context, db.GetCanonicalSiteFixParams) (db.SiteFix, error) {
	index := s.getCalls
	s.getCalls++
	if index >= len(s.getResults) {
		return db.SiteFix{}, pgx.ErrNoRows
	}
	return s.getResults[index].fix, s.getResults[index].err
}

func (s *measurementApprovalStoreStub) ApproveCanonicalSiteFix(context.Context, db.ApproveCanonicalSiteFixParams) (db.ApproveCanonicalSiteFixRow, error) {
	s.approveCalls++
	return db.ApproveCanonicalSiteFixRow{}, s.approveErr
}

func (s *measurementApprovalStoreStub) CreateSiteFixMeasurement(_ context.Context, params db.CreateSiteFixMeasurementParams) (db.SiteFixMeasurement, error) {
	s.createCalls++
	s.createParams = params
	return s.measurement, s.createErr
}

func requiredMeasurementSiteFixForApprovalTest(projectID, fixID uuid.UUID, status string, cutoff time.Time) db.SiteFix {
	hypothesis := "A clearer title will improve qualified organic CTR without reducing impressions."
	metric, version := "ctr", "site-fix-growth-v1"
	plan := fmt.Sprintf(`{
		"growth_hypothesis":%q,"primary_metric":"ctr","secondary_metrics":["impressions","clicks","position"],
		"target_query":"social publishing api",
		"baseline_window":{"start":%q,"end":%q},
		"baseline_snapshot":{"ctr":0.04,"impressions":1200,"clicks":48,"position":7.2},
		"baseline_provenance":{"source":"gsc","captured_at":%q},
		"policy_snapshot":{"policy_version":"site-fix-growth-v1","early_signal_offset_days":7,"primary_checkpoint_offset_days":28,"follow_up_offsets_days":[42],"max_follow_up_attempts":1,"max_measuring_duration_days":56,"minimum_sample":{"minimum_after_periods":7,"minimum_after_sample":100},"metric_thresholds":{"direction":"increase","kind":"relative","value":0.05},"guardrails":[{"metric":"impressions","max_adverse_relative":0.15}],"required_data_sources":["gsc"],"terminalization_grace_period_days":2}
	}`, hypothesis, cutoff.Add(-28*24*time.Hour).Format(time.RFC3339), cutoff.Add(-time.Hour).Format(time.RFC3339), cutoff.Add(-30*time.Minute).Format(time.RFC3339))
	var planDoc struct {
		Policy json.RawMessage `json:"policy_snapshot"`
	}
	_ = json.Unmarshal([]byte(plan), &planDoc)
	return db.SiteFix{
		ID: fixID, ProjectID: projectID, Status: status, TargetUrls: json.RawMessage(`["https://example.com/pricing"]`),
		ProposedFix: json.RawMessage(`{"fix_type":"metadata_ctr_optimization","measurement_plan":` + plan + `}`),
		FixType:     "metadata_ctr_optimization", ImpactMode: "conversion_or_ctr", MeasurementPolicy: "measurement_required",
		ClassifierVersion: sitefix.SiteFixClassifierVersionV1, DecisionOrigin: "system_rule", DecisionConfidence: "high",
		GrowthHypothesis: &hypothesis, PrimaryMetric: &metric, SecondaryMetrics: json.RawMessage(`["impressions","clicks","position"]`),
		MeasurementPolicyVersion: &version, MeasurementPolicySnapshot: planDoc.Policy,
	}
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

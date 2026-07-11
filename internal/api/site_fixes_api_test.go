package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/sitefix"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestDoctorSiteFixCreationHasSingleWriter(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo_doctor.go")
	if err != nil {
		t.Fatal(err)
	}
	body := functionSource(t, string(raw), "func (s *Server) convertSEODoctorFinding", "func firstDoctorFindingURL")
	for _, forbidden := range []string{
		"UpsertSEOOpportunity",
		"persistContentActionFromOpportunity",
		"LinkSEODoctorFindingToAction",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("deprecated Doctor alias still uses legacy writer %s", forbidden)
		}
	}
}

func TestOnDemandAuthorizationIsClaimedBeforeLifecycleMutation(t *testing.T) {
	raw, err := os.ReadFile("handlers_site_fixes.go")
	if err != nil {
		t.Fatal(err)
	}
	body := functionSource(t, string(raw), "func (s *postgresDoctorSiteFixLifecycleService) RequestVerification", "func (s *postgresDoctorSiteFixLifecycleService) Terminate")
	claim := strings.Index(body, "ClaimDoctorAIOnDemandTrigger")
	manual := strings.Index(body, "MarkCanonicalSiteFixManualApplied")
	reopen := strings.Index(body, "ReopenCanonicalSiteFix")
	if claim < 0 || manual < 0 || reopen < 0 || claim > manual || claim > reopen || !strings.Contains(body, "BeginTx") || !strings.Contains(body, "tx.Commit") {
		t.Fatalf("on-demand claim must share a transaction and precede lifecycle mutations")
	}
	legal := strings.Index(body, "legalLifecycle")
	if legal < 0 || legal > claim {
		t.Fatal("invalid lifecycle must be rejected before claiming an authorization marker")
	}
	if !strings.Contains(body, `marker.Status == "superseded"`) {
		t.Fatal("a superseded request id must be rejected as already handled")
	}
}

func TestDoctorSiteFixPersistenceContractsEnforceCanonicalAuthorityAndIdempotency(t *testing.T) {
	raw, err := os.ReadFile("../db/queries/site_fixes.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.ToLower(string(raw))
	if strings.Contains(source, "-- name: getactivecanonicalsitefixbyexactsignature :one") {
		t.Error("pre-arbitration exact lookup bypasses persisted exact_merge audit and must not exist")
	}
	getMerge := functionSource(t, source, "-- name: getcanonicalsitefixevidencemerge :one", "-- name: createcanonicalsitefixevidencemerge :one")
	for _, required := range []string{"join site_fixes", "join work_signature_registry", "registry.mode = 'enforced'", "registry.active = true", "registry.owner = 'doctor'", "registry.reserved_work_type = 'site_fix'"} {
		if !strings.Contains(getMerge, required) {
			t.Errorf("evidence merge reuse can return stale/non-Doctor work: missing %q", required)
		}
	}
	createMerge := functionSource(t, source, "-- name: createcanonicalsitefixevidencemerge :one", "-- name: lockcanonicalsitefixforupdate :one")
	existingMerge := functionSource(t, createMerge, "with existing_link as materialized", "), authority as materialized")
	if strings.Contains(existingMerge, "arbitration_decision_id") {
		t.Error("business-idempotent evidence reuse incorrectly requires the retry's arbitration decision")
	}
	for _, required := range []string{"decision.overlap_work_ids @> jsonb_build_array(registry.id::text)", "registry.exact_signature_hash = decision.exact_signature_hash", "candidate.source_object_type = 'seo_doctor_finding'", "candidate.source_object_id = sqlc.arg(doctor_finding_id)::text", "candidate.evidence_fingerprint = sqlc.arg(evidence_fingerprint)"} {
		if !strings.Contains(createMerge, required) {
			t.Errorf("evidence merge association is not bound to decision overlap/source: missing %q", required)
		}
	}
	reusable := functionSource(t, source, "-- name: getcompleteddoctorsitefixpreparationdecision :one", "-- name: getcanonicalsitefixevidencemerge :one")
	for _, required := range []string{"decision.candidate_id = sqlc.arg(candidate_id)", "decision.candidate_version = candidate.candidate_version", "decision.evidence_fingerprint = sqlc.arg(evidence_fingerprint)", "preparation.runtime_authority_fingerprint = sqlc.arg(runtime_authority_fingerprint)", "decision.rules_version = sqlc.arg(rules_version)", "decision.prompt_version = sqlc.arg(prompt_version)", "jsonb_each_text(decision.expected_bucket_versions)", "bucket.bucket_version <> expected.bucket_version::bigint"} {
		if !strings.Contains(reusable, required) {
			t.Errorf("cross-line owner reuse lacks invalidation guard %q", required)
		}
	}
	if strings.Contains(reusable, "decision.model = sqlc.arg(model)") {
		t.Error("completed outcome incorrectly keys reuse to constructor/env model instead of runtime authority")
	}
	for _, required := range []string{"from product_writer_authority", "writer_authority = 'canonical'", "for update", "work_conflict_buckets", "bucket_version = bucket_version + 1", "decision.candidate_version = candidate.candidate_version", "decision.expected_bucket_versions", "finding.status = 'active'", "finding.updated_at = sqlc.arg(expected_finding_updated_at)", "preparation.lease_token = sqlc.arg(lease_token)", "preparation.lease_expires_at > clock_timestamp()"} {
		if !strings.Contains(createMerge, required) {
			t.Errorf("evidence merge is not a fenced Phase-B CAS: missing %q", required)
		}
	}
	for _, queryName := range []string{
		"-- name: validatedoctorsitefixpreparationlease :one",
		"-- name: completedoctorsitefixpreparationlease :one",
		"-- name: lockdoctorsitefixpreparationleaseforreserve :one",
	} {
		body := functionSource(t, source, queryName, ";")
		if !strings.Contains(body, "clock_timestamp()") || strings.Contains(body, "lease_expires_at > now()") {
			t.Errorf("%s does not use wall-clock lease fencing", queryName)
		}
	}
	approve := functionSource(t, source, "-- name: approvecanonicalsitefix :one", "-- name: claimcanonicalsitefixapplying :one")
	for _, required := range []string{
		"from product_writer_authority",
		"for update",
		"writer_authority = 'canonical'",
		"write_fenced = false",
	} {
		if !strings.Contains(approve, required) {
			t.Errorf("approve transition does not enforce Doctor writer authority: missing %q", required)
		}
	}
	preparationMigration, err := os.ReadFile("../migrations/0055_site_fix_preparation_leases.sql")
	if err != nil {
		t.Fatal(err)
	}
	preparationSchema := strings.ToLower(string(preparationMigration))
	for _, required := range []string{
		"create table if not exists doctor_site_fix_preparation_leases",
		"primary key (project_id, exact_signature_hash)",
		"lease_token",
		"lease_expires_at",
		"result_expires_at",
		"leader_candidate_id",
		"arbitration_decision_id",
		"runtime_authority_fingerprint",
		"status text not null",
	} {
		if !strings.Contains(preparationSchema, required) {
			t.Errorf("persistent preparation lease schema missing %q", required)
		}
	}
	for _, required := range []string{
		"-- name: claimdoctorsitefixpreparationlease :one",
		"-- name: getdoctorsitefixpreparationlease :one",
		"-- name: completedoctorsitefixpreparationlease :one",
		"-- name: faildoctorsitefixpreparationlease :one",
		"-- name: lockdoctorsitefixpreparationleaseforreserve :one",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("persistent preparation lease query missing %q", required)
		}
	}
	mergeMigration, err := os.ReadFile("../migrations/0054_site_fix_evidence_merges.sql")
	if err != nil {
		t.Fatal(err)
	}
	mergeSchema := strings.ToLower(string(mergeMigration))
	for _, required := range []string{
		"create table if not exists site_fix_evidence_merges",
		"candidate_id",
		"arbitration_decision_id",
		"site_fix_id",
		"doctor_finding_id",
		"evidence_fingerprint",
		"evidence_snapshot",
		"unique (project_id, candidate_id, site_fix_id, evidence_fingerprint)",
		"reject_doctor_append_only_mutation",
	} {
		if !strings.Contains(mergeSchema, required) {
			t.Errorf("append-only evidence merge schema missing %q", required)
		}
	}
	for _, required := range []string{
		"-- name: getcanonicalsitefixevidencemerge :one",
		"-- name: createcanonicalsitefixevidencemerge :one",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("evidence merge query missing %q", required)
		}
	}
}

func TestPreparationFencedCreatorRejectsUnrelatedClaimBeforeDatabase(t *testing.T) {
	claim := doctorSiteFixPreparationClaim{ProjectID: uuid.New(), CandidateID: uuid.New(), ExactSignatureHash: "exact", LeaseToken: uuid.New()}
	creator := preparationFencedSiteFixCreator{claim: claim}
	for _, work := range []discovery.ReservedWork{
		{ProjectID: uuid.New(), CandidateID: claim.CandidateID, Owner: discovery.OwnerDoctor},
		{ProjectID: claim.ProjectID, CandidateID: uuid.New(), Owner: discovery.OwnerDoctor},
	} {
		if _, err := creator.CreateInTransaction(context.Background(), nil, work); !errors.Is(err, errDoctorSiteFixPreparationLost) {
			t.Fatalf("unrelated work %+v error = %v", work, err)
		}
	}
}

func TestPreparationMutationNoRowsIsLeaseLostConflict(t *testing.T) {
	if !errors.Is(normalizeDoctorSiteFixPreparationMutationError(pgx.ErrNoRows), errDoctorSiteFixPreparationLost) {
		t.Fatal("pgx.ErrNoRows from Complete/Validate must become lease-lost, not resource not-found")
	}
}

func TestDoctorSiteFixCreationDoesNotRepeatPreparationWithAStaleCandidate(t *testing.T) {
	raw, err := os.ReadFile("handlers_site_fixes.go")
	if err != nil {
		t.Fatal(err)
	}
	body := functionSource(t, string(raw), "func (c doctorSiteFixCreationCoordinator) runPreparationLeader", "func (c doctorSiteFixCreationCoordinator) consumeFollowerPreparation")
	if strings.Contains(body, "for attempt") || strings.Contains(body, "CreateFromFinding(") {
		t.Fatal("snapshot-stale retry reuses the original materialized candidate; return conflict so a new request reloads the finding")
	}
}

func TestDoctorSiteFixRuntimeWiresConcreteCanonicalService(t *testing.T) {
	raw, err := os.ReadFile("../../cmd/api/main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "SiteFixes: api.NewDoctorSiteFixService(pool, q, llmP, env.TokenGateModel)") {
		t.Fatal("API runtime does not explicitly wire the canonical Doctor Site Fix service")
	}
}

func TestDoctorSiteFixRuntimeAuthorityDoesNotUseEnvOrResponseModelIdentity(t *testing.T) {
	provider := &runtimeAuthorityProviderStub{fingerprint: "admin-route-authority-v7", responseModel: "runtime-response-model"}
	service := NewDoctorSiteFixService(nil, nil, provider, "stale-env-model").(*postgresDoctorSiteFixService)
	manager := service.creation.preparations.(*postgresDoctorSiteFixPreparationManager)
	fingerprint, err := manager.runtimeAuthorityFingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint != provider.fingerprint || strings.Contains(fingerprint, "stale-env-model") || strings.Contains(fingerprint, provider.responseModel) {
		t.Fatalf("runtime authority fingerprint = %q", fingerprint)
	}
	if provider.fingerprintPurpose != llm.PurposeSiteFix {
		t.Fatalf("fingerprint purpose = %q", provider.fingerprintPurpose)
	}
	raw, err := os.ReadFile("handlers_site_fixes.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), ".WithPurpose(llm.PurposeSiteFix)") {
		t.Fatal("Doctor semantic comparator request purpose is not aligned with runtime authority purpose")
	}
}

type runtimeAuthorityProviderStub struct {
	fingerprint        string
	responseModel      string
	fingerprintPurpose llm.CompletionPurpose
}

func (p *runtimeAuthorityProviderStub) Complete(context.Context, llm.CompletionReq) (llm.CompletionResp, error) {
	return llm.CompletionResp{Model: p.responseModel}, nil
}

func (p *runtimeAuthorityProviderStub) RuntimeAuthorityFingerprint(_ context.Context, purpose llm.CompletionPurpose) (string, error) {
	p.fingerprintPurpose = purpose
	return p.fingerprint, nil
}

func TestDoctorSiteFixPreparationLeaseDoesNotHoldDatabaseResourcesDuringProviderCall(t *testing.T) {
	raw, err := os.ReadFile("handlers_site_fixes.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{"Claim", "Wait", "Complete", "Fail", "Validate"} {
		if !strings.Contains(source, required) {
			t.Errorf("persistent preparation lease implementation missing %q", required)
		}
	}
	coordinator := functionSource(t, source, "func (c doctorSiteFixCreationCoordinator) CreateFromFinding", "type postgresDoctorSiteFixService")
	for _, forbidden := range []string{"pg_advisory_lock", "pg_advisory_xact_lock", "pgxpool.Conn", ".Acquire("} {
		if strings.Contains(coordinator, forbidden) {
			t.Errorf("provider path must not hold a DB lock or dedicated pool connection, found %q", forbidden)
		}
	}
	if !strings.Contains(coordinator, "backend.Prepare") {
		t.Fatal("coordinator no longer invokes persisted arbitration preparation")
	}
}

func TestDoctorSiteFixCoordinatorSerializesConcurrentCreationAndAuditsMerge(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, findingID)
	coordinator := doctorSiteFixCreationCoordinator{preparations: newDurableDoctorSiteFixPreparationManager(), backend: backend}

	start := make(chan struct{})
	type result struct {
		fix     db.SiteFix
		created bool
		err     error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			fix, created, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID)
			results <- result{fix: fix, created: created, err: err}
		}()
	}
	close(start)
	createdCount := 0
	for i := 0; i < 2; i++ {
		got := <-results
		if got.err != nil || got.fix.ID != backend.fix.ID {
			t.Fatalf("concurrent create result = %+v", got)
		}
		if got.created {
			createdCount++
		}
	}
	backend.mu.Lock()
	if createdCount != 1 || backend.semanticPrepareCalls != 1 || backend.reserveCalls != 1 || backend.mergeCalls != 1 || backend.prepareCalls != 2 {
		t.Fatalf("created=%d semantic=%d reserve=%d merge=%d prepare=%d", createdCount, backend.semanticPrepareCalls, backend.reserveCalls, backend.mergeCalls, backend.prepareCalls)
	}
	backend.mu.Unlock()

	// Once the exact-merge link exists, later repeats return through that
	// audited association without another decision or provider call.
	fix, created, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID)
	if err != nil || created || fix.ID != backend.fix.ID {
		t.Fatalf("linked repeat = %+v created=%v err=%v", fix, created, err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.prepareCalls != 2 || backend.semanticPrepareCalls != 1 || backend.mergeCalls != 1 {
		t.Fatalf("linked repeat repeated work: prepare=%d semantic=%d merge=%d", backend.prepareCalls, backend.semanticPrepareCalls, backend.mergeCalls)
	}
}

func TestDoctorSiteFixCoordinatorSerializesDifferentFindingsWithSameExactSignature(t *testing.T) {
	projectID := uuid.New()
	firstFindingID, secondFindingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, firstFindingID)
	backend.addEquivalentFinding(secondFindingID)
	coordinator := doctorSiteFixCreationCoordinator{preparations: newDurableDoctorSiteFixPreparationManager(), backend: backend}

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, findingID := range []uuid.UUID{firstFindingID, secondFindingID} {
		findingID := findingID
		go func() {
			<-start
			_, _, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID)
			results <- err
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.semanticPrepareCalls != 1 || backend.reserveCalls != 1 || backend.mergeCalls != 1 {
		t.Fatalf("different-finding exact concurrency semantic=%d reserve=%d merge=%d", backend.semanticPrepareCalls, backend.reserveCalls, backend.mergeCalls)
	}
	seen := make(map[uuid.UUID]struct{})
	for _, candidateID := range backend.preparedCandidateIDs {
		seen[candidateID] = struct{}{}
	}
	if len(seen) != 2 {
		t.Fatalf("different findings did not preserve distinct candidate audit identities: %v", backend.preparedCandidateIDs)
	}
}

func TestDoctorSiteFixSameCandidateFollowerReusesLeaderMergeLink(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, findingID)
	backend.workExists = true
	coordinator := doctorSiteFixCreationCoordinator{preparations: newDurableDoctorSiteFixPreparationManager(), backend: backend}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, _, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID)
			results <- err
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.mergeCalls != 1 || backend.prepareCalls != 1 {
		t.Fatalf("same-candidate follower duplicated merge audit: merge=%d prepare=%d", backend.mergeCalls, backend.prepareCalls)
	}
}

func TestDoctorSiteFixRechecksEvidenceLinkBeforeEveryReclaimAttempt(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, findingID)
	backend.workExists = true
	canonical := backend.candidates[findingID]
	preparations := newDurableDoctorSiteFixPreparationManager()
	leader, err := preparations.Claim(context.Background(), projectID, canonical)
	if err != nil {
		t.Fatal(err)
	}
	if err := preparations.Complete(context.Background(), leader, discovery.PreparedDecision{
		ID: uuid.New(), ProjectID: projectID, CandidateID: canonical.ID,
		EvidenceFingerprint: canonical.Candidate.EvidenceFingerprint,
		Owner:               discovery.OwnerDoctor, Decision: discovery.DecisionMergeEvidence,
		Status: discovery.ArbitrationStatusPrepared,
	}); err != nil {
		t.Fatal(err)
	}
	preparations.reclaimOnce = func() {
		backend.mu.Lock()
		backend.mergeLinked[canonical.ID] = true
		backend.mu.Unlock()
	}
	coordinator := doctorSiteFixCreationCoordinator{preparations: preparations, backend: backend}
	if _, created, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID); err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.prepareCalls != 0 || backend.mergeCalls != 0 {
		t.Fatalf("reclaim duplicated merge: prepare=%d merge=%d", backend.prepareCalls, backend.mergeCalls)
	}
}

func TestDoctorSiteFixCrashAfterEvidenceMergeDoesNotDuplicateLinkOrPreparation(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, findingID)
	backend.workExists = true
	backend.mergeErrAfterRecord = errors.New("process crashed after merge commit")
	coordinator := doctorSiteFixCreationCoordinator{preparations: newDurableDoctorSiteFixPreparationManager(), backend: backend}
	if _, _, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID); err == nil {
		t.Fatal("first request should observe simulated post-commit crash")
	}
	backend.mergeErrAfterRecord = nil
	if _, created, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID); err != nil || created {
		t.Fatalf("retry created=%v err=%v", created, err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.prepareCalls != 1 || backend.mergeCalls != 1 {
		t.Fatalf("post-commit retry duplicated work: prepare=%d merge=%d", backend.prepareCalls, backend.mergeCalls)
	}
}

func TestDoctorSiteFixDifferentCandidateDoesNotInheritCrossLineOutcome(t *testing.T) {
	projectID := uuid.New()
	firstFindingID, secondFindingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, firstFindingID)
	backend.addEquivalentFinding(secondFindingID)
	backend.prepareOwner = discovery.OwnerOpportunities
	preparations := newDurableDoctorSiteFixPreparationManager()
	coordinator := doctorSiteFixCreationCoordinator{preparations: preparations, backend: backend}
	for _, findingID := range []uuid.UUID{firstFindingID, secondFindingID} {
		_, _, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID)
		if !errors.Is(err, ErrDoctorSiteFixCrossLineOwnership) {
			t.Fatalf("finding %s error = %v", findingID, err)
		}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.semanticPrepareCalls != 2 || len(backend.preparedCandidateIDs) != 2 || backend.preparedCandidateIDs[0] == backend.preparedCandidateIDs[1] {
		t.Fatalf("cross-line outcome leaked across candidate identity: semantic=%d candidates=%v", backend.semanticPrepareCalls, backend.preparedCandidateIDs)
	}
}

func TestDoctorSiteFixCoordinatorRejectsCrossLineOwnerBeforeReserveAndReusesDecision(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, findingID)
	backend.prepareOwner = discovery.OwnerOpportunities
	coordinator := doctorSiteFixCreationCoordinator{preparations: newDurableDoctorSiteFixPreparationManager(), backend: backend}

	for i := 0; i < 2; i++ {
		_, _, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID)
		if !errors.Is(err, ErrDoctorSiteFixCrossLineOwnership) {
			t.Fatalf("attempt %d error = %v", i+1, err)
		}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.reserveCalls != 0 || backend.prepareCalls != 1 || backend.semanticPrepareCalls != 1 {
		t.Fatalf("cross-line retry reserve=%d prepare=%d semantic=%d", backend.reserveCalls, backend.prepareCalls, backend.semanticPrepareCalls)
	}
}

func TestDoctorSiteFixCoordinatorDoesNotMisclassifyHeldOpportunitiesSuggestion(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, findingID)
	backend.prepareOwner = discovery.OwnerOpportunities
	backend.prepareStatus = discovery.ArbitrationStatusHeld
	backend.prepareDecision = discovery.DecisionHold
	backend.prepareDisposition = discovery.DispositionProviderFailure
	coordinator := doctorSiteFixCreationCoordinator{preparations: newDurableDoctorSiteFixPreparationManager(), backend: backend}

	for i := 0; i < 2; i++ {
		_, _, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID)
		var hold *DoctorSiteFixArbitrationHoldError
		if !errors.As(err, &hold) || errors.Is(err, ErrDoctorSiteFixCrossLineOwnership) {
			t.Fatalf("attempt %d held error = %v", i+1, err)
		}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.prepareCalls != 1 || backend.reserveCalls != 0 {
		t.Fatalf("held suggestion was misclassified or repeated provider work: prepare=%d reserve=%d", backend.prepareCalls, backend.reserveCalls)
	}
}

func TestDoctorSiteFixPreparationReusesResolvedNonCreateOutcome(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, findingID)
	backend.prepareStatus = discovery.ArbitrationStatusResolved
	backend.prepareDecision = discovery.DecisionSuppress
	backend.prepareDisposition = discovery.DispositionReviewMemory
	coordinator := doctorSiteFixCreationCoordinator{preparations: newDurableDoctorSiteFixPreparationManager(), backend: backend}
	for i := 0; i < 2; i++ {
		_, _, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID)
		var hold *DoctorSiteFixArbitrationHoldError
		if !errors.As(err, &hold) {
			t.Fatalf("attempt %d error = %v", i+1, err)
		}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.prepareCalls != 1 || backend.reserveCalls != 0 {
		t.Fatalf("resolved non-create outcome repeated work: prepare=%d reserve=%d", backend.prepareCalls, backend.reserveCalls)
	}
}

func TestDoctorSiteFixCompletesAndReusesOpportunitiesSuppressOutcome(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, findingID)
	backend.prepareOwner = discovery.OwnerOpportunities
	backend.prepareDecision = discovery.DecisionSuppress
	backend.prepareDisposition = discovery.DispositionReviewMemory
	coordinator := doctorSiteFixCreationCoordinator{preparations: newDurableDoctorSiteFixPreparationManager(), backend: backend}
	for i := 0; i < 2; i++ {
		_, _, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID)
		var hold *DoctorSiteFixArbitrationHoldError
		if !errors.As(err, &hold) || errors.Is(err, ErrDoctorSiteFixCrossLineOwnership) {
			t.Fatalf("attempt %d error=%v", i+1, err)
		}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.prepareCalls != 1 || backend.semanticPrepareCalls != 1 || backend.reserveCalls != 0 {
		t.Fatalf("Opportunities suppress repeated work: prepare=%d semantic=%d reserve=%d", backend.prepareCalls, backend.semanticPrepareCalls, backend.reserveCalls)
	}
}

func TestDoctorSiteFixPreparationFollowerRecoversAfterLeaderFailure(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, findingID)
	backend.prepareErrOnce = errors.New("leader provider crashed")
	preparations := newDurableDoctorSiteFixPreparationManager()
	coordinator := doctorSiteFixCreationCoordinator{preparations: preparations, backend: backend}

	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, _, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID)
			results <- err
		}()
	}
	close(start)
	var success, failed int
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			failed++
		} else {
			success++
		}
	}
	if success != 1 || failed != 1 {
		t.Fatalf("leader failure recovery success=%d failed=%d", success, failed)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.semanticPrepareCalls != 2 || backend.reserveCalls != 1 {
		t.Fatalf("leader failure recovery semantic=%d reserve=%d", backend.semanticPrepareCalls, backend.reserveCalls)
	}
}

func TestDoctorSiteFixPreparationExpiredLeaderIsReclaimedAndFenced(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, findingID)
	preparations := newDurableDoctorSiteFixPreparationManager()
	canonical := backend.candidates[findingID]
	first, err := preparations.Claim(context.Background(), projectID, canonical)
	if err != nil || !first.Leader {
		t.Fatalf("first claim = %+v err=%v", first, err)
	}
	preparations.mu.Lock()
	preparations.entries[preparationKey(projectID, canonical.Identity.ExactSignatureHash)].claim.ExpiresAt = time.Now().Add(-time.Second)
	preparations.mu.Unlock()
	second, err := preparations.Claim(context.Background(), projectID, canonical)
	if err != nil || !second.Leader || second.LeaseToken == first.LeaseToken {
		t.Fatalf("reclaimed claim = %+v err=%v", second, err)
	}
	if _, err := preparations.Validate(context.Background(), first); !errors.Is(err, errDoctorSiteFixPreparationLost) {
		t.Fatalf("expired leader validation = %v", err)
	}
}

func TestDoctorSiteFixProviderDeadlineAndReserveFenceStayInsideLease(t *testing.T) {
	projectID, findingID := uuid.New(), uuid.New()
	backend := newDoctorSiteFixCreationBackendStub(projectID, findingID)
	preparations := newDurableDoctorSiteFixPreparationManager()
	preparations.ttl = 20 * time.Second
	coordinator := doctorSiteFixCreationCoordinator{preparations: preparations, backend: backend}
	started := time.Now()
	if _, _, err := coordinator.CreateFromFinding(context.Background(), projectID, findingID); err != nil {
		t.Fatal(err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.prepareDeadlines) != 1 || backend.prepareDeadlines[0].IsZero() {
		t.Fatalf("provider deadline was not set: %v", backend.prepareDeadlines)
	}
	if backend.prepareDeadlines[0].After(started.Add(16 * time.Second)) {
		t.Fatalf("provider deadline %s is not safely before lease expiry", backend.prepareDeadlines[0])
	}
	if len(backend.reserveClaims) != 1 || backend.reserveClaims[0].LeaseTTLSeconds < 20 || !backend.reserveClaims[0].ExpiresAt.After(backend.prepareDeadlines[0]) {
		t.Fatalf("reserve did not receive renewed fencing claim: %+v", backend.reserveClaims)
	}
}

func TestCanonicalDoctorSiteFixHandlers(t *testing.T) {
	projectID := uuid.New()
	findingID := uuid.New()
	fixID := uuid.New()
	fix := db.SiteFix{
		ID: fixID, ProjectID: projectID, DoctorFindingID: findingID,
		CandidateID: uuid.New(), WorkSignatureID: uuid.New(),
		Status: "proposed", FindingKind: "broken",
		TargetUrls:       json.RawMessage(`["https://example.com/pricing"]`),
		EvidenceSnapshot: json.RawMessage(`{"source":"doctor"}`),
		ProposedFix:      json.RawMessage(`{"mutation":"canonical"}`),
		AcceptanceTests:  json.RawMessage(`[{"type":"canonical_equals"}]`),
	}

	t.Run("create returns canonical provenance and created status", func(t *testing.T) {
		service := &doctorSiteFixServiceStub{createFix: DoctorSiteFixResponse{SiteFix: fix}, created: true}
		response := serveSiteFixRequest(t, service, http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/findings/"+findingID.String()+"/site-fixes")
		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got db.SiteFix
		if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.ID != fixID || got.CandidateID != fix.CandidateID || got.WorkSignatureID != fix.WorkSignatureID {
			t.Fatalf("canonical provenance = %+v", got)
		}
		if service.createProject != projectID || service.createFinding != findingID {
			t.Fatalf("create scope = %s/%s", service.createProject, service.createFinding)
		}
	})

	t.Run("repeat create returns existing canonical fix", func(t *testing.T) {
		service := &doctorSiteFixServiceStub{createFix: DoctorSiteFixResponse{SiteFix: fix}, created: false}
		response := serveSiteFixRequest(t, service, http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/findings/"+findingID.String()+"/site-fixes")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("legacy convert is a deprecated alias to canonical creation", func(t *testing.T) {
		service := &doctorSiteFixServiceStub{createFix: DoctorSiteFixResponse{SiteFix: fix}, created: false}
		response := serveSiteFixRequest(t, service, http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/findings/"+findingID.String()+"/convert")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		if response.Header().Get("Deprecation") != "true" {
			t.Fatalf("Deprecation = %q", response.Header().Get("Deprecation"))
		}
		wantSuccessor := "/api/projects/" + projectID.String() + "/doctor/findings/" + findingID.String() + "/site-fixes"
		if link := response.Header().Get("Link"); !strings.Contains(link, wantSuccessor) || !strings.Contains(link, `rel="successor-version"`) {
			t.Fatalf("Link = %q", link)
		}
		if service.createCalls != 1 {
			t.Fatalf("canonical create calls = %d, want 1", service.createCalls)
		}
	})

	t.Run("held arbitration fails closed without exposing an internal queue", func(t *testing.T) {
		service := &doctorSiteFixServiceStub{createErr: &DoctorSiteFixArbitrationHoldError{
			Decision: discovery.PreparedDecision{
				Disposition: discovery.DispositionProviderFailure,
				Decision:    discovery.DecisionHold,
				Reason:      "semantic comparison unavailable; send to needs_arbitration_review queue owned by discovery_ops",
				Status:      discovery.ArbitrationStatusHeld,
			},
		}}
		response := serveSiteFixRequest(t, service, http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/findings/"+findingID.String()+"/site-fixes")
		if response.Code != http.StatusConflict {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		body := response.Body.String()
		if !strings.Contains(strings.ToLower(body), "semantic comparison") || !strings.Contains(body, "no Site Fix was created") {
			t.Fatalf("missing stable fail-closed explanation: %s", body)
		}
		for _, forbidden := range []string{"needs_arbitration_review", "discovery_ops", "migration_review", `"queue"`} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("response leaked internal queue concept %q: %s", forbidden, body)
			}
		}
	})

	t.Run("incomplete and healthy findings are unprocessable", func(t *testing.T) {
		for _, err := range []error{sitefix.ErrIncompleteCandidate, sitefix.ErrHealthyFinding} {
			service := &doctorSiteFixServiceStub{createErr: err}
			response := serveSiteFixRequest(t, service, http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/findings/"+findingID.String()+"/site-fixes")
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("err %v: status = %d, body = %s", err, response.Code, response.Body.String())
			}
		}
	})

	t.Run("concurrent active predecessor is a conflict", func(t *testing.T) {
		service := &doctorSiteFixServiceStub{createErr: sitefix.ErrActivePredecessor}
		response := serveSiteFixRequest(t, service, http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/findings/"+findingID.String()+"/site-fixes")
		if response.Code != http.StatusConflict {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("cross line owner is a stable conflict", func(t *testing.T) {
		service := &doctorSiteFixServiceStub{createErr: fmt.Errorf("internal owner opportunity: %w", ErrDoctorSiteFixCrossLineOwnership)}
		response := serveSiteFixRequest(t, service, http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/findings/"+findingID.String()+"/site-fixes")
		if response.Code != http.StatusConflict {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		body := response.Body.String()
		if strings.Contains(body, "internal owner") || strings.Contains(body, "ErrWrongOwner") || !strings.Contains(body, "Opportunities") {
			t.Fatalf("cross-line response is not stable/public: %s", body)
		}
	})

	t.Run("list get and approve stay project scoped", func(t *testing.T) {
		verificationID := uuid.New()
		aiCallID := uuid.New()
		applicationID := uuid.New()
		legacyOpportunityID := uuid.New()
		legacyActionID := uuid.New()
		migrationBatchID := uuid.New()
		fix.LegacyOpportunityID = pgtype.UUID{Bytes: legacyOpportunityID, Valid: true}
		fix.LegacyContentActionID = pgtype.UUID{Bytes: legacyActionID, Valid: true}
		fix.MigrationBatchID = pgtype.UUID{Bytes: migrationBatchID, Valid: true}
		detail := DoctorSiteFixResponse{
			SiteFix:       fix,
			Application:   &db.SiteChangeApplication{ID: applicationID, ProjectID: projectID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true}, Status: "manual_apply_required"},
			Verifications: []db.SiteFixVerification{{ID: verificationID, ProjectID: projectID, SiteFixID: fixID, AttemptNumber: 1, AiCallID: pgtype.UUID{Bytes: aiCallID, Valid: true}, Result: "failed"}},
		}
		approvedDetail := detail
		approvedDetail.Status = "approved"
		service := &doctorSiteFixServiceStub{listFixes: []DoctorSiteFixResponse{detail}, getFix: detail, approveFix: approvedDetail}
		for _, request := range []struct {
			method     string
			path       string
			wantStatus int
		}{
			{http.MethodGet, "/api/projects/" + projectID.String() + "/doctor/site-fixes?status=proposed", http.StatusOK},
			{http.MethodGet, "/api/projects/" + projectID.String() + "/doctor/site-fixes/" + fixID.String(), http.StatusOK},
			{http.MethodPost, "/api/projects/" + projectID.String() + "/doctor/site-fixes/" + fixID.String() + "/approve", http.StatusOK},
		} {
			response := serveSiteFixRequest(t, service, request.method, request.path)
			if response.Code != request.wantStatus {
				t.Fatalf("%s %s status = %d, body = %s", request.method, request.path, response.Code, response.Body.String())
			}
		}
		if service.listProject != projectID || service.getProject != projectID || service.approveProject != projectID || service.getFixID != fixID || service.approveFixID != fixID {
			t.Fatalf("service scope was not preserved: %+v", service)
		}
		if service.listStatus == nil || *service.listStatus != "proposed" {
			t.Fatalf("list status = %v", service.listStatus)
		}
		for _, request := range []struct {
			method string
			path   string
		}{
			{http.MethodGet, "/api/projects/" + projectID.String() + "/doctor/site-fixes"},
			{http.MethodGet, "/api/projects/" + projectID.String() + "/doctor/site-fixes/" + fixID.String()},
		} {
			response := serveSiteFixRequest(t, service, request.method, request.path)
			body := response.Body.String()
			for _, expected := range []string{applicationID.String(), verificationID.String(), aiCallID.String(), legacyOpportunityID.String(), legacyActionID.String(), migrationBatchID.String()} {
				if !strings.Contains(body, expected) {
					t.Fatalf("%s %s omitted canonical detail %s: %s", request.method, request.path, expected, body)
				}
			}
		}
	})

	t.Run("not found remains project scoped", func(t *testing.T) {
		service := &doctorSiteFixServiceStub{getErr: pgx.ErrNoRows}
		response := serveSiteFixRequest(t, service, http.MethodGet, "/api/projects/"+projectID.String()+"/doctor/site-fixes/"+fixID.String())
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("unexpected internal errors are redacted", func(t *testing.T) {
		service := &doctorSiteFixServiceStub{getErr: errors.New("postgres password=super-secret provider_token=private")}
		response := serveSiteFixRequest(t, service, http.MethodGet, "/api/projects/"+projectID.String()+"/doctor/site-fixes/"+fixID.String())
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		body := response.Body.String()
		if strings.Contains(body, "super-secret") || strings.Contains(body, "provider_token") || !strings.Contains(body, "temporarily unavailable") {
			t.Fatalf("internal error was not safely redacted: %s", body)
		}
	})

	t.Run("apply and verify use canonical lifecycle service", func(t *testing.T) {
		service := &doctorSiteFixServiceStub{}
		lifecycle := &doctorSiteFixLifecycleServiceStub{
			applyResult:  sitefix.ApplyResult{SiteFix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "applying"}},
			verifyResult: DoctorSiteFixVerificationRequest{SiteFix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "awaiting_deploy"}},
		}
		applyResponse := serveSiteFixLifecycleRequest(t, service, lifecycle, http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/site-fixes/"+fixID.String()+"/apply")
		if applyResponse.Code != http.StatusOK {
			t.Fatalf("apply status = %d, body = %s", applyResponse.Code, applyResponse.Body.String())
		}
		verifyResponse := serveSiteFixLifecycleRequest(t, service, lifecycle, http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/site-fixes/"+fixID.String()+"/verify")
		if verifyResponse.Code != http.StatusAccepted {
			t.Fatalf("verify status = %d, body = %s", verifyResponse.Code, verifyResponse.Body.String())
		}
		if lifecycle.applyProject != projectID || lifecycle.applyFix != fixID || lifecycle.verifyProject != projectID || lifecycle.verifyFix != fixID {
			t.Fatalf("lifecycle scope = %+v", lifecycle)
		}
	})

	t.Run("verify carries a durable on-demand AI request identity", func(t *testing.T) {
		requestID := uuid.New()
		lifecycle := &doctorSiteFixLifecycleServiceStub{verifyResult: DoctorSiteFixVerificationRequest{SiteFix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "awaiting_deploy"}}}
		server := &Server{SiteFixes: &doctorSiteFixServiceStub{}, SiteFixLifecycle: lifecycle}
		body := strings.NewReader(`{"ai_review_request_id":"` + requestID.String() + `"}`)
		request := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/site-fixes/"+fixID.String()+"/verify", body)
		response := httptest.NewRecorder()
		server.Router().ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("verify status = %d, body = %s", response.Code, response.Body.String())
		}
		if lifecycle.verifyInput.AIReviewRequestID == nil || *lifecycle.verifyInput.AIReviewRequestID != requestID {
			t.Fatalf("on-demand request identity was not preserved: %+v", lifecycle.verifyInput)
		}
	})

	t.Run("canonical lifecycle CAS conflicts return 409", func(t *testing.T) {
		lifecycle := &doctorSiteFixLifecycleServiceStub{applyErr: sitefix.ErrLifecycleConflict, verifyErr: sitefix.ErrLifecycleConflict}
		for _, action := range []string{"apply", "verify"} {
			response := serveSiteFixLifecycleRequest(t, &doctorSiteFixServiceStub{}, lifecycle, http.MethodPost, "/api/projects/"+projectID.String()+"/doctor/site-fixes/"+fixID.String()+"/"+action)
			if response.Code != http.StatusConflict {
				t.Fatalf("%s status=%d body=%s", action, response.Code, response.Body.String())
			}
		}
	})
}

func TestDoctorSiteFixManualEvidenceRequiresAuthenticatedStructuredAttestation(t *testing.T) {
	tests := []json.RawMessage{json.RawMessage(`{"type":"canonical_present"}`), json.RawMessage(`"manual visual check"`)}
	rawTests, _ := json.Marshal(tests)
	results := make([]DoctorSiteFixManualAcceptanceResult, len(tests))
	for i, test := range tests {
		sum := sha256.Sum256(test)
		results[i] = DoctorSiteFixManualAcceptanceResult{Index: i, TestFingerprint: fmt.Sprintf("sha256:%x", sum[:]), Status: "passed", Evidence: json.RawMessage(`{"observed":"reviewed production artifact"}`)}
	}
	fix := db.SiteFix{AcceptanceTests: rawTests}
	app := db.SiteChangeApplication{TargetUrl: "https://example.com/page"}
	valid := DoctorSiteFixManualEvidence{HumanConfirmed: true, TargetURL: app.TargetUrl, AcceptanceResults: results}
	if _, err := validateDoctorSiteFixManualEvidence(fix, app, valid); err != nil {
		t.Fatalf("valid manual evidence rejected: %v", err)
	}
	invalid := valid
	invalid.HumanConfirmed = false
	if _, err := validateDoctorSiteFixManualEvidence(fix, app, invalid); !errors.Is(err, ErrDoctorSiteFixManualEvidenceInvalid) {
		t.Fatalf("unconfirmed evidence error=%v", err)
	}
	invalid = valid
	invalid.AcceptanceResults[0].TestFingerprint = "sha256:wrong"
	if _, err := validateDoctorSiteFixManualEvidence(fix, app, invalid); !errors.Is(err, ErrDoctorSiteFixManualEvidenceInvalid) {
		t.Fatalf("wrong fingerprint error=%v", err)
	}
}

func TestCanonicalGitHubApplyOnlySupportsTypedMetadataMutationFamily(t *testing.T) {
	metadata := db.SiteFix{ProposedFix: json.RawMessage(`{"mutations":[{"field":"title","operation":"replace"},{"field":"meta_description","operation":"update"}]}`)}
	if got := canonicalSiteFixPRMutationFamily(metadata); got != "metadata_rewrite" {
		t.Fatalf("metadata family=%q", got)
	}
	for _, raw := range []string{
		`{"mutations":[{"field":"canonical","operation":"replace"}]}`,
		`{"mutations":[{"field":"body","operation":"add"}]}`,
		`{}`,
	} {
		if got := canonicalSiteFixPRMutationFamily(db.SiteFix{ProposedFix: json.RawMessage(raw)}); got != "" {
			t.Fatalf("unsupported %s classified %q", raw, got)
		}
	}
}

func TestDoctorAIProviderAuthorityMustBeExplicit(t *testing.T) {
	defaultCfg, _ := config.Parse(json.RawMessage(`{}`))
	disabledCfg, _ := config.Parse(json.RawMessage(`{"doctor_ai_enabled":false,"doctor_ai_run_policy":"manual_only"}`))
	if defaultCfg.AllowsDoctorAI(config.DoctorAITriggerApplyUser) || disabledCfg.AllowsDoctorAI(config.DoctorAITriggerApplyUser) {
		t.Fatal("Doctor provider authority must default off")
	}
	enabledCfg, _ := config.Parse(json.RawMessage(`{"doctor_ai_enabled":true,"doctor_ai_run_policy":"manual_only"}`))
	if !enabledCfg.AllowsDoctorAI(config.DoctorAITriggerApplyUser) {
		t.Fatal("explicit Doctor provider authority was ignored")
	}
}

func serveSiteFixRequest(t *testing.T, service DoctorSiteFixService, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	server := &Server{SiteFixes: service}
	request := httptest.NewRequest(method, path, nil)
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	return response
}

func serveSiteFixLifecycleRequest(t *testing.T, service DoctorSiteFixService, lifecycle DoctorSiteFixLifecycleService, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	server := &Server{SiteFixes: service, SiteFixLifecycle: lifecycle}
	request := httptest.NewRequest(method, path, nil)
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	return response
}

type doctorSiteFixServiceStub struct {
	createFix      DoctorSiteFixResponse
	created        bool
	createErr      error
	createCalls    int
	createProject  uuid.UUID
	createFinding  uuid.UUID
	listFixes      []DoctorSiteFixResponse
	listErr        error
	listProject    uuid.UUID
	listStatus     *string
	getFix         DoctorSiteFixResponse
	getErr         error
	getProject     uuid.UUID
	getFixID       uuid.UUID
	approveFix     DoctorSiteFixResponse
	approveErr     error
	approveProject uuid.UUID
	approveFixID   uuid.UUID
}

type doctorSiteFixLifecycleServiceStub struct {
	applyResult      sitefix.ApplyResult
	applyErr         error
	applyProject     uuid.UUID
	applyFix         uuid.UUID
	verifyResult     DoctorSiteFixVerificationRequest
	verifyErr        error
	verifyProject    uuid.UUID
	verifyFix        uuid.UUID
	verifyInput      DoctorSiteFixVerificationInput
	terminateResult  DoctorSiteFixVerificationRequest
	terminateErr     error
	terminateProject uuid.UUID
	terminateFix     uuid.UUID
}

func (s *doctorSiteFixLifecycleServiceStub) Apply(_ context.Context, projectID, fixID uuid.UUID) (sitefix.ApplyResult, error) {
	s.applyProject, s.applyFix = projectID, fixID
	return s.applyResult, s.applyErr
}

func (s *doctorSiteFixLifecycleServiceStub) RequestVerification(_ context.Context, projectID, fixID uuid.UUID, input DoctorSiteFixVerificationInput) (DoctorSiteFixVerificationRequest, error) {
	s.verifyProject, s.verifyFix = projectID, fixID
	s.verifyInput = input
	return s.verifyResult, s.verifyErr
}

func (s *doctorSiteFixLifecycleServiceStub) Terminate(_ context.Context, projectID, fixID uuid.UUID) (DoctorSiteFixVerificationRequest, error) {
	s.terminateProject, s.terminateFix = projectID, fixID
	return s.terminateResult, s.terminateErr
}

type durableDoctorSiteFixPreparationManager struct {
	mu          sync.Mutex
	ttl         time.Duration
	entries     map[string]*durableDoctorSiteFixPreparationEntry
	reclaimOnce func()
}

type durableDoctorSiteFixPreparationEntry struct {
	claim    doctorSiteFixPreparationClaim
	status   string
	decision discovery.PreparedDecision
}

func newDurableDoctorSiteFixPreparationManager() *durableDoctorSiteFixPreparationManager {
	return &durableDoctorSiteFixPreparationManager{ttl: time.Minute, entries: make(map[string]*durableDoctorSiteFixPreparationEntry)}
}

func preparationKey(projectID uuid.UUID, exact string) string {
	return projectID.String() + ":" + exact
}

func (m *durableDoctorSiteFixPreparationManager) Claim(_ context.Context, projectID uuid.UUID, canonical sitefix.CanonicalCandidate) (doctorSiteFixPreparationClaim, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := preparationKey(projectID, canonical.Identity.ExactSignatureHash)
	now := time.Now()
	entry := m.entries[key]
	if entry == nil || entry.status == "failed" || entry.claim.ExpiresAt.Before(now) {
		claim := doctorSiteFixPreparationClaim{ProjectID: projectID, ExactSignatureHash: canonical.Identity.ExactSignatureHash, CandidateID: canonical.ID, EvidenceFingerprint: canonical.Candidate.EvidenceFingerprint, RuntimeAuthorityFingerprint: "test-authority", LeaseToken: uuid.New(), ExpiresAt: now.Add(m.ttl), LeaseTTLSeconds: int32(m.ttl / time.Second), Leader: true}
		m.entries[key] = &durableDoctorSiteFixPreparationEntry{claim: claim, status: "preparing"}
		return claim, nil
	}
	claim := entry.claim
	claim.CandidateID = canonical.ID
	claim.EvidenceFingerprint = canonical.Candidate.EvidenceFingerprint
	claim.Leader = false
	return claim, nil
}

func (m *durableDoctorSiteFixPreparationManager) Wait(ctx context.Context, claim doctorSiteFixPreparationClaim) (discovery.PreparedDecision, error) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		m.mu.Lock()
		if m.reclaimOnce != nil {
			reclaim := m.reclaimOnce
			m.reclaimOnce = nil
			if entry := m.entries[preparationKey(claim.ProjectID, claim.ExactSignatureHash)]; entry != nil {
				entry.status = "failed"
			}
			m.mu.Unlock()
			reclaim()
			return discovery.PreparedDecision{}, errDoctorSiteFixPreparationReclaim
		}
		entry := m.entries[preparationKey(claim.ProjectID, claim.ExactSignatureHash)]
		if entry == nil || entry.status == "failed" || entry.claim.ExpiresAt.Before(time.Now()) {
			m.mu.Unlock()
			return discovery.PreparedDecision{}, errDoctorSiteFixPreparationReclaim
		}
		if entry.status == "completed" {
			if entry.decision.CandidateID != claim.CandidateID || entry.decision.EvidenceFingerprint != claim.EvidenceFingerprint {
				entry.status = "failed"
				m.mu.Unlock()
				return discovery.PreparedDecision{}, errDoctorSiteFixPreparationReclaim
			}
			decision := entry.decision
			m.mu.Unlock()
			return decision, nil
		}
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return discovery.PreparedDecision{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (m *durableDoctorSiteFixPreparationManager) Validate(_ context.Context, claim doctorSiteFixPreparationClaim) (doctorSiteFixPreparationClaim, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[preparationKey(claim.ProjectID, claim.ExactSignatureHash)]
	if entry == nil || entry.status != "preparing" || entry.claim.LeaseToken != claim.LeaseToken || entry.claim.ExpiresAt.Before(time.Now()) {
		return doctorSiteFixPreparationClaim{}, errDoctorSiteFixPreparationLost
	}
	entry.claim.ExpiresAt = time.Now().Add(m.ttl)
	claim.ExpiresAt = entry.claim.ExpiresAt
	return claim, nil
}

func (m *durableDoctorSiteFixPreparationManager) Complete(_ context.Context, claim doctorSiteFixPreparationClaim, prepared discovery.PreparedDecision) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[preparationKey(claim.ProjectID, claim.ExactSignatureHash)]
	if entry == nil || entry.status != "preparing" || entry.claim.LeaseToken != claim.LeaseToken || entry.claim.ExpiresAt.Before(time.Now()) {
		return errDoctorSiteFixPreparationLost
	}
	entry.status = "completed"
	entry.decision = prepared
	return nil
}

func (m *durableDoctorSiteFixPreparationManager) Fail(_ context.Context, claim doctorSiteFixPreparationClaim, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[preparationKey(claim.ProjectID, claim.ExactSignatureHash)]
	if entry != nil && entry.status == "preparing" && entry.claim.LeaseToken == claim.LeaseToken {
		entry.status = "failed"
	}
	return nil
}

func (m *durableDoctorSiteFixPreparationManager) Invalidate(_ context.Context, claim doctorSiteFixPreparationClaim) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[preparationKey(claim.ProjectID, claim.ExactSignatureHash)]
	if entry == nil || entry.status != "completed" || entry.claim.LeaseToken != claim.LeaseToken {
		return errDoctorSiteFixPreparationLost
	}
	entry.status = "failed"
	return nil
}

type doctorSiteFixCreationBackendStub struct {
	mu                   sync.Mutex
	projectID            uuid.UUID
	findings             map[uuid.UUID]db.SeoDoctorFinding
	candidates           map[uuid.UUID]sitefix.CanonicalCandidate
	fix                  db.SiteFix
	workSignatureID      uuid.UUID
	workExists           bool
	mergeLinked          map[uuid.UUID]bool
	prepareOwner         discovery.Owner
	prepareStatus        discovery.ArbitrationStatus
	prepareDecision      discovery.DecisionKind
	prepareDisposition   discovery.ArbitrationDisposition
	prepareCalls         int
	semanticPrepareCalls int
	reserveCalls         int
	mergeCalls           int
	mergeErrAfterRecord  error
	preparedCandidateIDs []uuid.UUID
	prepareErrOnce       error
	prepareDeadlines     []time.Time
	reserveClaims        []doctorSiteFixPreparationClaim
}

func newDoctorSiteFixCreationBackendStub(projectID, findingID uuid.UUID) *doctorSiteFixCreationBackendStub {
	candidateID, signatureID, fixID := uuid.New(), uuid.New(), uuid.New()
	return &doctorSiteFixCreationBackendStub{
		projectID: projectID,
		findings: map[uuid.UUID]db.SeoDoctorFinding{
			findingID: {ID: findingID, ProjectID: projectID, Status: "active", FindingKind: "broken", Evidence: json.RawMessage(`{"source":"gsc"}`)},
		},
		candidates: map[uuid.UUID]sitefix.CanonicalCandidate{findingID: {
			ID:        candidateID,
			Candidate: discovery.Candidate{ProjectID: projectID, EvidenceFingerprint: "evidence-v1"},
			Identity:  discovery.Identity{ExactSignatureHash: "exact-v1"},
		}},
		fix:                db.SiteFix{ID: fixID, ProjectID: projectID, DoctorFindingID: findingID, CandidateID: candidateID, WorkSignatureID: signatureID, Status: "proposed", FindingKind: "broken"},
		workSignatureID:    signatureID,
		prepareOwner:       discovery.OwnerDoctor,
		prepareStatus:      discovery.ArbitrationStatusPrepared,
		prepareDecision:    discovery.DecisionCreate,
		prepareDisposition: discovery.DispositionSemanticComparison,
		mergeLinked:        make(map[uuid.UUID]bool),
	}
}

func (b *doctorSiteFixCreationBackendStub) addEquivalentFinding(findingID uuid.UUID) {
	b.findings[findingID] = db.SeoDoctorFinding{ID: findingID, ProjectID: b.projectID, Status: "active", FindingKind: "broken", Evidence: json.RawMessage(`{"source":"ga4"}`)}
	b.candidates[findingID] = sitefix.CanonicalCandidate{
		ID:        uuid.New(),
		Candidate: discovery.Candidate{ProjectID: b.projectID, EvidenceFingerprint: "evidence-" + findingID.String()},
		Identity:  discovery.Identity{ExactSignatureHash: "exact-v1"},
	}
}

func (b *doctorSiteFixCreationBackendStub) LoadFinding(_ context.Context, _ uuid.UUID, findingID uuid.UUID) (db.SeoDoctorFinding, error) {
	return b.findings[findingID], nil
}

func (b *doctorSiteFixCreationBackendStub) Materialize(_ context.Context, finding db.SeoDoctorFinding) (sitefix.CanonicalCandidate, error) {
	return b.candidates[finding.ID], nil
}

func (b *doctorSiteFixCreationBackendStub) FindEvidenceMerge(_ context.Context, _ uuid.UUID, canonical sitefix.CanonicalCandidate) (db.SiteFix, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.fix, b.mergeLinked[canonical.ID], nil
}

func (b *doctorSiteFixCreationBackendStub) Prepare(ctx context.Context, _ uuid.UUID, canonical sitefix.CanonicalCandidate) (discovery.PreparedDecision, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prepareCalls++
	b.preparedCandidateIDs = append(b.preparedCandidateIDs, canonical.ID)
	deadline, _ := ctx.Deadline()
	b.prepareDeadlines = append(b.prepareDeadlines, deadline)
	if b.workExists {
		return discovery.PreparedDecision{ID: uuid.New(), ProjectID: b.projectID, CandidateID: canonical.ID, EvidenceFingerprint: canonical.Candidate.EvidenceFingerprint, Disposition: discovery.DispositionExactMerge, Decision: discovery.DecisionMergeEvidence, Owner: discovery.OwnerDoctor, OverlapWorkIDs: []uuid.UUID{b.workSignatureID}, Status: discovery.ArbitrationStatusPrepared}, nil
	}
	b.semanticPrepareCalls++
	if b.prepareErrOnce != nil {
		err := b.prepareErrOnce
		b.prepareErrOnce = nil
		return discovery.PreparedDecision{}, err
	}
	decision := discovery.PreparedDecision{ID: uuid.New(), ProjectID: b.projectID, CandidateID: canonical.ID, EvidenceFingerprint: canonical.Candidate.EvidenceFingerprint, Disposition: b.prepareDisposition, Decision: b.prepareDecision, Owner: b.prepareOwner, Status: b.prepareStatus}
	return decision, nil
}

func (b *doctorSiteFixCreationBackendStub) Reserve(_ context.Context, _ uuid.UUID, _ discovery.PreparedDecision, claim doctorSiteFixPreparationClaim) (discovery.ReservationResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reserveCalls++
	b.reserveClaims = append(b.reserveClaims, claim)
	b.workExists = true
	return discovery.ReservationResult{SignatureID: b.workSignatureID, Work: discovery.WorkReference{Type: "site_fix", ID: b.fix.ID}}, nil
}

func (b *doctorSiteFixCreationBackendStub) ResolveOverlap(context.Context, uuid.UUID, discovery.PreparedDecision) (db.SiteFix, error) {
	return b.fix, nil
}

func (b *doctorSiteFixCreationBackendStub) RecordEvidenceMerge(_ context.Context, _ db.SeoDoctorFinding, canonical sitefix.CanonicalCandidate, _ discovery.PreparedDecision, _ db.SiteFix, _ doctorSiteFixPreparationClaim) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mergeCalls++
	b.mergeLinked[canonical.ID] = true
	return b.mergeErrAfterRecord
}

func (b *doctorSiteFixCreationBackendStub) LoadSiteFix(context.Context, uuid.UUID, uuid.UUID) (db.SiteFix, error) {
	return b.fix, nil
}

func (s *doctorSiteFixServiceStub) CreateFromFinding(_ context.Context, projectID, findingID uuid.UUID) (DoctorSiteFixResponse, bool, error) {
	s.createCalls++
	s.createProject, s.createFinding = projectID, findingID
	return s.createFix, s.created, s.createErr
}

func (s *doctorSiteFixServiceStub) List(_ context.Context, projectID uuid.UUID, status *string) ([]DoctorSiteFixResponse, error) {
	s.listProject, s.listStatus = projectID, status
	return s.listFixes, s.listErr
}

func (s *doctorSiteFixServiceStub) Get(_ context.Context, projectID, fixID uuid.UUID) (DoctorSiteFixResponse, error) {
	s.getProject, s.getFixID = projectID, fixID
	return s.getFix, s.getErr
}

func (s *doctorSiteFixServiceStub) Approve(_ context.Context, projectID, fixID uuid.UUID, _ time.Time) (DoctorSiteFixResponse, error) {
	s.approveProject, s.approveFixID = projectID, fixID
	return s.approveFix, s.approveErr
}

func functionSource(t *testing.T, source, start, end string) string {
	t.Helper()
	startIndex := strings.Index(source, start)
	if startIndex < 0 {
		t.Fatalf("missing %q", start)
	}
	endIndex := strings.Index(source[startIndex:], end)
	if endIndex < 0 {
		t.Fatalf("missing end marker %q", end)
	}
	return source[startIndex : startIndex+endIndex]
}

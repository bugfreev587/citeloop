package sitefix

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCreatorCreatesCanonicalDoctorSiteFix(t *testing.T) {
	projectID, findingID, candidateID, signatureID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	finding := canonicalFinding(projectID, findingID)
	storage := &creatorDBStub{
		candidate: canonicalDiscoveryCandidateForFinding(finding, candidateID),
		finding:   finding,
	}
	q := db.New(storage)

	ref, err := (Creator{}).CreateInTransaction(context.Background(), q, discovery.ReservedWork{
		ProjectID: projectID, CandidateID: candidateID, DecisionID: uuid.New(),
		WorkSignatureID: signatureID, Owner: discovery.OwnerDoctor,
	})
	if err != nil {
		t.Fatalf("CreateInTransaction: %v", err)
	}
	if ref.Type != "site_fix" || ref.ID == uuid.Nil || storage.createCalls != 1 {
		t.Fatalf("reference/create calls = %+v/%d", ref, storage.createCalls)
	}
	created := storage.created
	if created.ProjectID != projectID || created.DoctorFindingID != findingID ||
		created.CandidateID != candidateID || created.WorkSignatureID != signatureID {
		t.Fatalf("canonical provenance = %+v", created)
	}
	if created.Status != "proposed" || created.FindingKind != "broken" {
		t.Fatalf("status/kind = %q/%q", created.Status, created.FindingKind)
	}
	if created.FixType != "canonical_repair" || created.ImpactMode != "technical_reliability" ||
		created.MeasurementPolicy != "verification_only" || created.ClassifierVersion != SiteFixClassifierVersionV1 ||
		created.DecisionOrigin != "system_rule" || created.DecisionConfidence != "high" {
		t.Fatalf("persisted measurement classification = %+v", created)
	}
	if storage.measurementCreateCalls != 0 {
		t.Fatal("Site Fix creation must not create a measurement before approval")
	}
	if created.LegacyOpportunityID.Valid || created.LegacyContentActionID.Valid {
		t.Fatal("new canonical Site Fix contains legacy opportunity/action ids")
	}
	if !json.Valid(created.EvidenceSnapshot) || !json.Valid(created.ProposedFix) || !json.Valid(created.AcceptanceTests) {
		t.Fatal("canonical payloads are not valid JSON")
	}
	var proposed map[string]any
	if err := json.Unmarshal(created.ProposedFix, &proposed); err != nil {
		t.Fatal(err)
	}
	if _, ok := proposed["mutations"]; !ok {
		t.Fatalf("proposed fix lacks candidate mutations: %s", created.ProposedFix)
	}
	if string(created.AcceptanceTests) != string(finding.AcceptanceTests) {
		t.Fatalf("acceptance tests = %s, want %s", created.AcceptanceTests, finding.AcceptanceTests)
	}
}

func TestCreatorAcceptsPersistedEmptyDoctorIdentitySets(t *testing.T) {
	projectID, findingID, candidateID := uuid.New(), uuid.New(), uuid.New()
	finding := canonicalFinding(projectID, findingID)
	candidate := canonicalDiscoveryCandidateForFinding(finding, candidateID)
	// PostgresRepository normalizes nil topic/audience identity sets to JSON
	// arrays before persistence. The locked snapshot must compare against that
	// canonical representation rather than treating [] as different from nil.
	candidate.TopicEntityIdentity = json.RawMessage(`[]`)
	candidate.AudienceIdentity = json.RawMessage(`[]`)
	storage := &creatorDBStub{candidate: candidate, finding: finding}

	_, err := (Creator{}).CreateInTransaction(context.Background(), db.New(storage), discovery.ReservedWork{
		ProjectID: projectID, CandidateID: candidateID, DecisionID: uuid.New(),
		WorkSignatureID: uuid.New(), Owner: discovery.OwnerDoctor,
	})
	if err != nil {
		t.Fatalf("CreateInTransaction: %v", err)
	}
	if storage.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", storage.createCalls)
	}
}

func TestCreatorRejectsInvalidStructuredMeasurementOverride(t *testing.T) {
	projectID, findingID, candidateID := uuid.New(), uuid.New(), uuid.New()
	finding := canonicalFinding(projectID, findingID)
	finding.Evidence = json.RawMessage(`{
		"canonical":null,
		"url":"https://example.com/pricing",
		"site_fix_policy_override":{"fix_type":"not-a-real-fix","measurement_policy":"measurement_required"}
	}`)
	storage := &creatorDBStub{
		candidate: canonicalDiscoveryCandidateForFinding(finding, candidateID),
		finding:   finding,
	}
	_, err := (Creator{}).CreateInTransaction(context.Background(), db.New(storage), discovery.ReservedWork{
		ProjectID: projectID, CandidateID: candidateID, DecisionID: uuid.New(),
		WorkSignatureID: uuid.New(), Owner: discovery.OwnerDoctor,
	})
	if !errors.Is(err, ErrInvalidMeasurementPolicy) {
		t.Fatalf("error = %v, want ErrInvalidMeasurementPolicy", err)
	}
	if storage.createCalls != 0 || storage.measurementCreateCalls != 0 {
		t.Fatal("invalid override persisted Site Fix or measurement")
	}
}

func TestCreatorPersistsReadyRequiredPlanWithoutCreatingMeasurement(t *testing.T) {
	projectID, findingID, candidateID := uuid.New(), uuid.New(), uuid.New()
	finding := canonicalFinding(projectID, findingID)
	finding.Evidence = json.RawMessage(`{
		"canonical":null,
		"url":"https://example.com/pricing",
		"site_fix_policy_override":{
			"fix_type":"metadata_ctr_optimization",
			"impact_mode":"conversion_or_ctr",
			"measurement_policy":"measurement_required",
			"measurement_plan":` + string(completeCTRMeasurementPlanJSONAt(time.Now().UTC().Add(-time.Second))) + `
		}
	}`)
	storage := &creatorDBStub{
		candidate: canonicalDiscoveryCandidateForFinding(finding, candidateID),
		finding:   finding,
	}
	_, err := (Creator{}).CreateInTransaction(context.Background(), db.New(storage), discovery.ReservedWork{
		ProjectID: projectID, CandidateID: candidateID, DecisionID: uuid.New(),
		WorkSignatureID: uuid.New(), Owner: discovery.OwnerDoctor,
	})
	if err != nil {
		t.Fatalf("CreateInTransaction: %v", err)
	}
	created := storage.created
	if created.FixType != "metadata_ctr_optimization" || created.ImpactMode != "conversion_or_ctr" ||
		created.MeasurementPolicy != "measurement_required" || created.DecisionOrigin != "user_override" ||
		created.GrowthHypothesis == nil || created.PrimaryMetric == nil || *created.PrimaryMetric != "ctr" ||
		created.MeasurementPolicyVersion == nil || *created.MeasurementPolicyVersion != "site-fix-growth-v1" ||
		len(created.MeasurementPolicySnapshot) == 0 || string(created.MeasurementPolicySnapshot) == `{}` ||
		len(created.MeasurementPlanSnapshot) == 0 || string(created.MeasurementPlanSnapshot) == `{}` {
		t.Fatalf("persisted required plan = %+v", created)
	}
	if _, err := RecoverApprovedSiteFixMeasurementPlan(storedMeasurementInputFromCreatedFix(created), created.CreatedAt.Time); err != nil {
		t.Fatalf("override creator output cannot be recovered at approval: %v", err)
	}
	if storage.createCalls != 1 || storage.measurementCreateCalls != 0 {
		t.Fatalf("Site Fix/measurement creates = %d/%d, want 1/0", storage.createCalls, storage.measurementCreateCalls)
	}
}

func TestCreatorPersistsRegularFindingMeasurementPlanForApprovalRecovery(t *testing.T) {
	projectID, findingID, candidateID := uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC().Add(-time.Second)
	finding := canonicalFinding(projectID, findingID)
	finding.IssueType = "metadata_ctr_optimization"
	finding.FindingKind = "optimization"
	finding.Evidence = json.RawMessage(`{"url":"https://example.com/pricing","measurement_plan":` + string(completeCTRMeasurementPlanJSONAt(now)) + `}`)
	storage := &creatorDBStub{candidate: canonicalDiscoveryCandidateForFinding(finding, candidateID), finding: finding}
	_, err := (Creator{}).CreateInTransaction(context.Background(), db.New(storage), discovery.ReservedWork{
		ProjectID: projectID, CandidateID: candidateID, DecisionID: uuid.New(), WorkSignatureID: uuid.New(), Owner: discovery.OwnerDoctor,
	})
	if err != nil {
		t.Fatalf("CreateInTransaction: %v", err)
	}
	created := storage.created
	if created.MeasurementPolicy != "measurement_required" || string(created.MeasurementPlanSnapshot) == `{}` {
		t.Fatalf("regular finding plan was not persisted: %+v", created)
	}
	if _, err := RecoverApprovedSiteFixMeasurementPlan(storedMeasurementInputFromCreatedFix(created), created.CreatedAt.Time); err != nil {
		t.Fatalf("regular finding creator output cannot be recovered at approval: %v", err)
	}
}

func storedMeasurementInputFromCreatedFix(fix db.SiteFix) StoredSiteFixMeasurementInput {
	return StoredSiteFixMeasurementInput{
		TargetURLs: fix.TargetUrls, ProposedFix: fix.ProposedFix, EvidenceSnapshot: fix.EvidenceSnapshot,
		MeasurementPlanSnapshot: fix.MeasurementPlanSnapshot,
		FixType:                 fix.FixType, ImpactMode: fix.ImpactMode, MeasurementPolicy: fix.MeasurementPolicy,
		ClassifierVersion: fix.ClassifierVersion, DecisionOrigin: fix.DecisionOrigin, DecisionConfidence: fix.DecisionConfidence,
		GrowthHypothesis: fix.GrowthHypothesis, PrimaryMetric: fix.PrimaryMetric, SecondaryMetrics: fix.SecondaryMetrics,
		MeasurementPolicyVersion: fix.MeasurementPolicyVersion, MeasurementPolicySnapshot: fix.MeasurementPolicySnapshot,
	}
}

func TestCreatorRejectsInvalidDoctorWork(t *testing.T) {
	baseProject, findingID, candidateID := uuid.New(), uuid.New(), uuid.New()
	baseFinding := canonicalFinding(baseProject, findingID)
	baseCandidate := canonicalDiscoveryCandidateForFinding(baseFinding, candidateID)

	tests := []struct {
		name    string
		owner   discovery.Owner
		mutate  func(*db.DiscoveryCandidate, *db.SeoDoctorFinding, *db.SiteFix)
		wantErr error
	}{
		{name: "non Doctor owner", owner: discovery.OwnerOpportunities, wantErr: ErrWrongOwner},
		{name: "incomplete candidate", owner: discovery.OwnerDoctor, mutate: func(c *db.DiscoveryCandidate, _ *db.SeoDoctorFinding, _ *db.SiteFix) {
			c.Status = string(discovery.StatusNeedsSpecification)
		}, wantErr: ErrIncompleteCandidate},
		{name: "growth artifact intent", owner: discovery.OwnerDoctor, mutate: func(c *db.DiscoveryCandidate, _ *db.SeoDoctorFinding, _ *db.SiteFix) {
			c.ArtifactIntent = string(discovery.ArtifactUpdateExistingContent)
		}, wantErr: ErrIncompleteCandidate},
		{name: "growth success metric", owner: discovery.OwnerDoctor, mutate: func(c *db.DiscoveryCandidate, _ *db.SeoDoctorFinding, _ *db.SiteFix) { c.PrimarySuccessMetric = "ctr" }, wantErr: ErrIncompleteCandidate},
		{name: "blank target", owner: discovery.OwnerDoctor, mutate: func(c *db.DiscoveryCandidate, _ *db.SeoDoctorFinding, _ *db.SiteFix) {
			c.NormalizedTargetSet = json.RawMessage(`[" "]`)
		}, wantErr: ErrIncompleteCandidate},
		{name: "blank mutation field", owner: discovery.OwnerDoctor, mutate: func(c *db.DiscoveryCandidate, _ *db.SeoDoctorFinding, _ *db.SiteFix) {
			c.ProposedMutations = json.RawMessage(`[{"operation":"add","field":""}]`)
		}, wantErr: ErrIncompleteCandidate},
		{name: "empty evidence ids", owner: discovery.OwnerDoctor, mutate: func(c *db.DiscoveryCandidate, _ *db.SeoDoctorFinding, _ *db.SiteFix) {
			c.EvidenceIds = json.RawMessage(`[]`)
		}, wantErr: ErrIncompleteCandidate},
		{name: "missing finding evidence after preparation", owner: discovery.OwnerDoctor, mutate: func(_ *db.DiscoveryCandidate, f *db.SeoDoctorFinding, _ *db.SiteFix) { f.Evidence = nil }, wantErr: discovery.ErrSnapshotStale},
		{name: "finding became healthy after preparation", owner: discovery.OwnerDoctor, mutate: func(_ *db.DiscoveryCandidate, f *db.SeoDoctorFinding, _ *db.SiteFix) { f.FindingKind = "healthy" }, wantErr: discovery.ErrSnapshotStale},
		{name: "project mismatch", owner: discovery.OwnerDoctor, mutate: func(c *db.DiscoveryCandidate, _ *db.SeoDoctorFinding, _ *db.SiteFix) { c.ProjectID = uuid.New() }, wantErr: ErrProjectMismatch},
		{name: "candidate finding mismatch", owner: discovery.OwnerDoctor, mutate: func(c *db.DiscoveryCandidate, _ *db.SeoDoctorFinding, _ *db.SiteFix) {
			c.SourceObjectID = uuid.NewString()
		}, wantErr: ErrCandidateFindingMismatch},
		{name: "active predecessor", owner: discovery.OwnerDoctor, mutate: func(_ *db.DiscoveryCandidate, _ *db.SeoDoctorFinding, predecessor *db.SiteFix) {
			predecessor.ID = uuid.New()
			predecessor.ProjectID = baseProject
			predecessor.DoctorFindingID = findingID
			predecessor.Status = "failed_retryable"
		}, wantErr: ErrActivePredecessor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate, finding := baseCandidate, baseFinding
			predecessor := db.SiteFix{}
			if tt.mutate != nil {
				tt.mutate(&candidate, &finding, &predecessor)
			}
			storage := &creatorDBStub{candidate: candidate, finding: finding}
			if predecessor.ID != uuid.Nil {
				storage.predecessor = &predecessor
			}
			_, err := (Creator{}).CreateInTransaction(context.Background(), db.New(storage), discovery.ReservedWork{
				ProjectID: baseProject, CandidateID: candidateID, DecisionID: uuid.New(),
				WorkSignatureID: uuid.New(), Owner: tt.owner,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if storage.createCalls != 0 {
				t.Fatal("invalid work inserted a Site Fix")
			}
		})
	}
}

func TestCreatorLinksInactivePredecessorAsRevision(t *testing.T) {
	projectID, findingID, predecessorCandidateID, revisionCandidateID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	predecessorID := uuid.New()
	oldSnapshot := canonicalFinding(projectID, findingID)
	finding := oldSnapshot
	finding.FixIntent += " with immutable revision snapshot"
	storage := &creatorDBStub{
		candidate: canonicalDiscoveryCandidateForFinding(finding, revisionCandidateID),
		finding:   finding,
		predecessor: &db.SiteFix{
			ID: predecessorID, ProjectID: projectID, DoctorFindingID: findingID,
			CandidateID: predecessorCandidateID, Status: "failed_terminal",
		},
	}
	_, err := (Creator{}).CreateInTransaction(context.Background(), db.New(storage), discovery.ReservedWork{
		ProjectID: projectID, CandidateID: revisionCandidateID, DecisionID: uuid.New(),
		WorkSignatureID: uuid.New(), Owner: discovery.OwnerDoctor,
	})
	if err != nil {
		t.Fatalf("CreateInTransaction: %v", err)
	}
	if !storage.created.SupersedesSiteFixID.Valid || storage.created.SupersedesSiteFixID.Bytes != predecessorID {
		t.Fatalf("supersedes = %+v, want %s", storage.created.SupersedesSiteFixID, predecessorID)
	}
	if storage.created.CandidateID != revisionCandidateID || storage.created.CandidateID == predecessorCandidateID {
		t.Fatalf("revision candidate = %s, predecessor candidate = %s", storage.created.CandidateID, predecessorCandidateID)
	}
}

func TestCreatorRejectsStaleFindingSnapshot(t *testing.T) {
	projectID, findingID, candidateID := uuid.New(), uuid.New(), uuid.New()
	original := canonicalFinding(projectID, findingID)
	candidate := canonicalDiscoveryCandidateForFinding(original, candidateID)

	mutations := []func(*db.SeoDoctorFinding){
		func(f *db.SeoDoctorFinding) { f.FixIntent += " revised" },
		func(f *db.SeoDoctorFinding) { f.DeveloperInstructions += " revised" },
		func(f *db.SeoDoctorFinding) { f.LikelyFilesOrSurfaces = json.RawMessage(`["other.tsx"]`) },
		func(f *db.SeoDoctorFinding) { f.AcceptanceTests = json.RawMessage(`[{"type":"canonical_present"}]`) },
		func(f *db.SeoDoctorFinding) { f.Evidence = json.RawMessage(`{"canonical":"changed"}`) },
		func(f *db.SeoDoctorFinding) { f.NormalizedUrls = json.RawMessage(`["https://example.com/other"]`) },
		func(f *db.SeoDoctorFinding) { f.IssueType = "canonical_mismatch" },
	}
	for i, mutate := range mutations {
		finding := original
		mutate(&finding)
		storage := &creatorDBStub{candidate: candidate, finding: finding}
		_, err := (Creator{}).CreateInTransaction(context.Background(), db.New(storage), discovery.ReservedWork{
			ProjectID: projectID, CandidateID: candidateID, DecisionID: uuid.New(), WorkSignatureID: uuid.New(), Owner: discovery.OwnerDoctor,
		})
		if !errors.Is(err, discovery.ErrSnapshotStale) {
			t.Fatalf("mutation %d error = %v, want ErrSnapshotStale", i, err)
		}
		if storage.createCalls != 0 {
			t.Fatalf("mutation %d inserted a Site Fix", i)
		}
	}
}

func TestCreatorRejectsShadowOrMismatchedCanonicalCandidate(t *testing.T) {
	projectID, findingID, candidateID := uuid.New(), uuid.New(), uuid.New()
	finding := canonicalFinding(projectID, findingID)
	base := canonicalDiscoveryCandidateForFinding(finding, candidateID)
	tests := []struct {
		name   string
		mutate func(*db.DiscoveryCandidate)
	}{
		{name: "shadow run provenance", mutate: func(c *db.DiscoveryCandidate) { c.ShadowRunID = uuid.New() }},
		{name: "targets", mutate: func(c *db.DiscoveryCandidate) {
			c.NormalizedTargetSet = json.RawMessage(`["https://example.com/other"]`)
		}},
		{name: "change family", mutate: func(c *db.DiscoveryCandidate) { c.ChangeFamily = "url.redirect" }},
		{name: "mutations", mutate: func(c *db.DiscoveryCandidate) {
			c.ProposedMutations = json.RawMessage(`[{"operation":"update","field":"canonical"}]`)
		}},
		{name: "evidence fingerprint", mutate: func(c *db.DiscoveryCandidate) { c.EvidenceFingerprint = "other" }},
		{name: "evidence ids", mutate: func(c *db.DiscoveryCandidate) { c.EvidenceIds = json.RawMessage(`["other"]`) }},
		{name: "exact signature", mutate: func(c *db.DiscoveryCandidate) { value := "other"; c.ExactSignatureHash = &value }},
		{name: "signature payload", mutate: func(c *db.DiscoveryCandidate) {
			c.SignaturePayload = json.RawMessage(`{"signature_version":"work-signature-v1","other":true}`)
		}},
		{name: "bucket keys", mutate: func(c *db.DiscoveryCandidate) { c.ConflictBucketKeys = json.RawMessage(`["other"]`) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := base
			tt.mutate(&candidate)
			storage := &creatorDBStub{candidate: candidate, finding: finding}
			_, err := (Creator{}).CreateInTransaction(context.Background(), db.New(storage), discovery.ReservedWork{
				ProjectID: projectID, CandidateID: candidateID, DecisionID: uuid.New(), WorkSignatureID: uuid.New(), Owner: discovery.OwnerDoctor,
			})
			if !errors.Is(err, discovery.ErrSnapshotStale) {
				t.Fatalf("error = %v, want ErrSnapshotStale", err)
			}
			if storage.createCalls != 0 {
				t.Fatal("mismatched candidate inserted a Site Fix")
			}
		})
	}
}

func canonicalDiscoveryCandidateForFinding(finding db.SeoDoctorFinding, candidateID uuid.UUID) db.DiscoveryCandidate {
	candidate := discovery.ProjectDoctorFinding(finding)
	identity, err := discovery.BuildIdentity(candidate)
	if err != nil {
		panic(err)
	}
	snapshot, err := doctorFindingSnapshotFingerprint(finding, candidate, identity)
	if err != nil {
		panic(err)
	}
	targets, _ := json.Marshal(candidate.NormalizedTargetSet)
	mutations, _ := json.Marshal(candidate.ProposedMutations)
	topics, _ := json.Marshal(candidate.TopicEntityIdentity)
	audience, _ := json.Marshal(candidate.AudienceIdentity)
	evidenceIDs, _ := json.Marshal(candidate.EvidenceIDs)
	buckets, _ := json.Marshal(identity.ConflictBucketKeys)
	exact := identity.ExactSignatureHash
	var confidence pgtype.Numeric
	if err := confidence.Scan("1.0000"); err != nil {
		panic(err)
	}
	return db.DiscoveryCandidate{
		ID: candidateID, ProjectID: finding.ProjectID,
		ShadowRunID: canonicalRunID(finding.ProjectID, finding.ID, snapshot), SourceKind: string(candidate.SourceKind),
		SourceObjectType: candidate.SourceObjectType, SourceObjectID: candidate.SourceObjectID, TargetKind: candidate.TargetKind,
		NormalizedTargetSet: targets, IssueOrHypothesisFamily: candidate.IssueOrHypothesisFamily, ChangeFamily: candidate.ChangeFamily,
		ProposedMutations: mutations, ArtifactIntent: string(candidate.ArtifactIntent), TopicEntityIdentity: topics,
		AudienceIdentity: audience, PrimarySuccessMetric: candidate.PrimarySuccessMetric,
		VerificationMode: string(candidate.VerificationMode), EvidenceIds: evidenceIDs,
		EvidenceFingerprint: candidate.EvidenceFingerprint, SuggestedOwner: string(candidate.SuggestedOwner),
		Confidence:             confidence,
		CandidateSchemaVersion: candidate.CandidateSchemaVersion, Status: string(candidate.Status),
		ExactSignatureHash: &exact, SignaturePayload: identity.SignaturePayload,
		ConflictBucketKeys: buckets, CandidateVersion: 1,
	}
}

type creatorDBStub struct {
	candidate              db.DiscoveryCandidate
	finding                db.SeoDoctorFinding
	predecessor            *db.SiteFix
	created                db.SiteFix
	createCalls            int
	measurementCreateCalls int
}

func (s *creatorDBStub) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (s *creatorDBStub) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}
func (s *creatorDBStub) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	switch {
	case strings.Contains(query, "from discovery_candidates"):
		return scanValues(candidateValues(s.candidate))
	case strings.Contains(query, "from seo_doctor_findings"):
		return scanValues(findingValues(s.finding))
	case strings.Contains(query, "from site_fixes") && strings.Contains(query, "status in"):
		if s.predecessor == nil || !activeTestSiteFixStatus(s.predecessor.Status) {
			return scanRow{err: pgx.ErrNoRows}
		}
		return scanValues(siteFixValues(*s.predecessor))
	case strings.Contains(query, "from site_fixes"):
		if s.predecessor == nil {
			return scanRow{err: pgx.ErrNoRows}
		}
		return scanValues(siteFixValues(*s.predecessor))
	case strings.Contains(query, "insert into site_fixes"):
		s.createCalls++
		s.created = siteFixFromArgs(args)
		return scanValues(siteFixValues(s.created))
	case strings.Contains(query, "insert into site_fix_measurements"):
		s.measurementCreateCalls++
		return scanRow{err: errors.New("measurement creation is not allowed during Site Fix creation")}
	default:
		return scanRow{err: errors.New("unexpected query: " + query)}
	}
}

func activeTestSiteFixStatus(status string) bool {
	switch status {
	case "proposed", "approved", "preparing", "ready_to_apply", "applying", "awaiting_deploy", "verifying", "failed_retryable", "reopened":
		return true
	default:
		return false
	}
}

type scanRow struct {
	values []any
	err    error
}

func scanValues(values []any) scanRow { return scanRow{values: values} }
func (r scanRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("scan arity mismatch")
	}
	for i := range dest {
		dv := reflect.ValueOf(dest[i])
		if dv.Kind() != reflect.Pointer {
			return errors.New("scan destination is not a pointer")
		}
		value := reflect.ValueOf(r.values[i])
		if !value.IsValid() {
			dv.Elem().SetZero()
			continue
		}
		dv.Elem().Set(value)
	}
	return nil
}

func candidateValues(v db.DiscoveryCandidate) []any {
	return []any{v.ID, v.ProjectID, v.ShadowRunID, v.SourceKind, v.SourceObjectType, v.SourceObjectID, v.TargetKind, v.NormalizedTargetSet, v.IssueOrHypothesisFamily, v.ChangeFamily, v.ProposedMutations, v.ArtifactIntent, v.IntendedSlugOrCanonical, v.TopicEntityIdentity, v.AudienceIdentity, v.PrimarySuccessMetric, v.VerificationMode, v.EvidenceIds, v.EvidenceFingerprint, v.SuggestedOwner, v.Confidence, v.CandidateSchemaVersion, v.Status, v.HoldReason, v.ExactSignatureHash, v.SignaturePayload, v.ConflictBucketKeys, v.CreatedAt, v.UpdatedAt, v.CandidateVersion}
}
func findingValues(v db.SeoDoctorFinding) []any {
	return []any{v.ID, v.ProjectID, v.RunID, v.FindingKey, v.Severity, v.Category, v.IssueType, v.Status, v.AffectedUrls, v.NormalizedUrls, v.Evidence, v.WhyItMatters, v.FixIntent, v.DeveloperInstructions, v.LikelyFilesOrSurfaces, v.AcceptanceTests, v.RiskLevel, v.ReviewRequired, v.AutofixEligible, v.LinkedOpportunityID, v.LinkedContentActionID, v.FirstSeenAt, v.LastSeenAt, v.ResolvedAt, v.CreatedAt, v.UpdatedAt, v.FindingKind}
}
func siteFixValues(v db.SiteFix) []any {
	return []any{v.ID, v.ProjectID, v.DoctorFindingID, v.CandidateID, v.WorkSignatureID, v.SupersedesSiteFixID, v.Status, v.FindingKind, v.TargetUrls, v.EvidenceSnapshot, v.ProposedFix, v.AcceptanceTests, v.VerificationSnapshot, v.FailureReason, v.RetryCount, v.MaxRetries, v.LegacyOpportunityID, v.LegacyContentActionID, v.MigrationBatchID, v.ApprovedAt, v.AppliedAt, v.DeployedAt, v.VerifiedAt, v.CreatedAt, v.UpdatedAt, v.DoctorLinkDismissedAt, v.DoctorLinkDismissedBy, v.FixType, v.ImpactMode, v.MeasurementPolicy, v.ClassifierVersion, v.DecisionOrigin, v.DecisionConfidence, v.GrowthHypothesis, v.PrimaryMetric, v.SecondaryMetrics, v.MeasurementPolicyVersion, v.MeasurementPolicySnapshot, v.MeasurementPlanSnapshot}
}
func siteFixFromArgs(args []interface{}) db.SiteFix {
	return db.SiteFix{
		ID: args[0].(uuid.UUID), ProjectID: args[1].(uuid.UUID), DoctorFindingID: args[2].(uuid.UUID),
		CandidateID: args[3].(uuid.UUID), WorkSignatureID: args[4].(uuid.UUID), SupersedesSiteFixID: args[5].(pgtype.UUID),
		Status: args[6].(string), FindingKind: args[7].(string), TargetUrls: args[8].(json.RawMessage),
		EvidenceSnapshot: args[9].(json.RawMessage), ProposedFix: args[10].(json.RawMessage), AcceptanceTests: args[11].(json.RawMessage),
		VerificationSnapshot: args[12].(json.RawMessage), FailureReason: args[13].(*string), RetryCount: args[14].(int32),
		MaxRetries: args[15].(int32), LegacyOpportunityID: args[16].(pgtype.UUID), LegacyContentActionID: args[17].(pgtype.UUID),
		MigrationBatchID: args[18].(pgtype.UUID), ApprovedAt: args[19].(pgtype.Timestamptz), AppliedAt: args[20].(pgtype.Timestamptz),
		DeployedAt: args[21].(pgtype.Timestamptz), VerifiedAt: args[22].(pgtype.Timestamptz), CreatedAt: args[23].(pgtype.Timestamptz), UpdatedAt: args[24].(pgtype.Timestamptz),
		FixType: args[25].(string), ImpactMode: args[26].(string), MeasurementPolicy: args[27].(string),
		ClassifierVersion: args[28].(string), DecisionOrigin: args[29].(string), DecisionConfidence: args[30].(string),
		GrowthHypothesis: args[31].(*string), PrimaryMetric: args[32].(*string), SecondaryMetrics: args[33].(json.RawMessage),
		MeasurementPolicyVersion: args[34].(*string), MeasurementPolicySnapshot: args[35].(json.RawMessage), MeasurementPlanSnapshot: args[36].(json.RawMessage),
	}
}

package sitefix

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

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
		candidate: canonicalDiscoveryCandidate(projectID, candidateID, findingID),
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

func TestCreatorRejectsInvalidDoctorWork(t *testing.T) {
	baseProject, findingID, candidateID := uuid.New(), uuid.New(), uuid.New()
	baseFinding := canonicalFinding(baseProject, findingID)
	baseCandidate := canonicalDiscoveryCandidate(baseProject, candidateID, findingID)

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
		{name: "missing finding evidence", owner: discovery.OwnerDoctor, mutate: func(_ *db.DiscoveryCandidate, f *db.SeoDoctorFinding, _ *db.SiteFix) { f.Evidence = nil }, wantErr: ErrIncompleteCandidate},
		{name: "healthy finding", owner: discovery.OwnerDoctor, mutate: func(_ *db.DiscoveryCandidate, f *db.SeoDoctorFinding, _ *db.SiteFix) { f.FindingKind = "healthy" }, wantErr: ErrHealthyFinding},
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
	projectID, findingID, candidateID := uuid.New(), uuid.New(), uuid.New()
	predecessorID := uuid.New()
	storage := &creatorDBStub{
		candidate:   canonicalDiscoveryCandidate(projectID, candidateID, findingID),
		finding:     canonicalFinding(projectID, findingID),
		predecessor: &db.SiteFix{ID: predecessorID, ProjectID: projectID, DoctorFindingID: findingID, Status: "failed_terminal"},
	}
	_, err := (Creator{}).CreateInTransaction(context.Background(), db.New(storage), discovery.ReservedWork{
		ProjectID: projectID, CandidateID: candidateID, DecisionID: uuid.New(),
		WorkSignatureID: uuid.New(), Owner: discovery.OwnerDoctor,
	})
	if err != nil {
		t.Fatalf("CreateInTransaction: %v", err)
	}
	if !storage.created.SupersedesSiteFixID.Valid || storage.created.SupersedesSiteFixID.Bytes != predecessorID {
		t.Fatalf("supersedes = %+v, want %s", storage.created.SupersedesSiteFixID, predecessorID)
	}
}

func canonicalDiscoveryCandidate(projectID, candidateID, findingID uuid.UUID) db.DiscoveryCandidate {
	exact := "exact-doctor-canonical"
	return db.DiscoveryCandidate{
		ID: candidateID, ProjectID: projectID, ShadowRunID: uuid.New(), SourceKind: string(discovery.SourceDoctor),
		SourceObjectType: "seo_doctor_finding", SourceObjectID: findingID.String(), TargetKind: "page",
		NormalizedTargetSet:     json.RawMessage(`["https://example.com/pricing"]`),
		IssueOrHypothesisFamily: "canonical_missing", ChangeFamily: "url.canonical",
		ProposedMutations: json.RawMessage(`[{"operation":"add","field":"canonical"}]`),
		ArtifactIntent:    string(discovery.ArtifactRepairExistingSurface), TopicEntityIdentity: json.RawMessage(`[]`),
		AudienceIdentity: json.RawMessage(`[]`), PrimarySuccessMetric: "acceptance_test_pass",
		VerificationMode: string(discovery.VerificationImmediate), EvidenceIds: json.RawMessage(`[]`),
		EvidenceFingerprint: "evidence-v1", SuggestedOwner: string(discovery.OwnerDoctor),
		CandidateSchemaVersion: discovery.CandidateSchemaVersionV1, Status: string(discovery.StatusIdentityReady),
		ExactSignatureHash: &exact, SignaturePayload: json.RawMessage(`{"signature_version":"work-signature-v1"}`),
		ConflictBucketKeys: json.RawMessage(`["target:pricing:url"]`), CandidateVersion: 1,
	}
}

type creatorDBStub struct {
	candidate   db.DiscoveryCandidate
	finding     db.SeoDoctorFinding
	predecessor *db.SiteFix
	created     db.SiteFix
	createCalls int
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
	return []any{v.ID, v.ProjectID, v.DoctorFindingID, v.CandidateID, v.WorkSignatureID, v.SupersedesSiteFixID, v.Status, v.FindingKind, v.TargetUrls, v.EvidenceSnapshot, v.ProposedFix, v.AcceptanceTests, v.VerificationSnapshot, v.FailureReason, v.RetryCount, v.MaxRetries, v.LegacyOpportunityID, v.LegacyContentActionID, v.MigrationBatchID, v.ApprovedAt, v.AppliedAt, v.DeployedAt, v.VerifiedAt, v.CreatedAt, v.UpdatedAt}
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
	}
}

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) prepareAdminDiscoveryArbitration(w http.ResponseWriter, r *http.Request) {
	projectID, candidateID, ok := adminDiscoveryObjectIDs(w, r)
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	store := discovery.NewPostgresArbitrationStore(s.Pool, s.Q).WithSemanticRuntime("tokengate", s.Env.TokenGateModel)
	var comparator discovery.SemanticComparator
	if s.LLM != nil {
		comparator = discovery.NewLLMSemanticComparator(s.LLM, "tokengate", s.Env.TokenGateModel)
	}
	prepared, err := discovery.NewArbitrationService(store, comparator).Prepare(r.Context(), projectID, candidateID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "discovery candidate not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := http.StatusOK
	switch prepared.Disposition {
	case discovery.DispositionIncompleteSpecification:
		status = http.StatusUnprocessableEntity
	case discovery.DispositionProviderFailure:
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, prepared)
}

func (s *Server) listAdminDiscoveryReview(w http.ResponseWriter, r *http.Request) {
	projectID, ok := adminDiscoveryProjectID(w, r)
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	state := optionalQuery(r, "state")
	assignee := optionalQuery(r, "assignee")
	minAgeSeconds := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("min_age_seconds")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeErr(w, http.StatusBadRequest, "min_age_seconds must be a non-negative integer")
			return
		}
		minAgeSeconds = parsed
	}
	items, err := s.Q.ListDiscoveryReviewItems(r.Context(), db.ListDiscoveryReviewItemsParams{
		ProjectID: projectID, State: state, Assignee: assignee, MinAgeSeconds: minAgeSeconds,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) getAdminDiscoveryReview(w http.ResponseWriter, r *http.Request) {
	projectID, candidateID, ok := adminDiscoveryObjectIDs(w, r)
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	item, err := s.Q.GetDiscoveryReviewItem(r.Context(), db.GetDiscoveryReviewItemParams{
		ProjectID: projectID, CandidateID: candidateID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "discovery review item not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	candidate, err := s.Q.GetDiscoveryCandidateForReview(r.Context(), db.GetDiscoveryCandidateForReviewParams{
		ProjectID: projectID, CandidateID: candidateID,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	response := map[string]any{"review": item, "candidate": candidate}
	if item.ArbitrationDecisionID.Valid {
		decision, decisionErr := s.Q.GetArbitrationDecision(r.Context(), db.GetArbitrationDecisionParams{
			ProjectID: projectID, ID: item.ArbitrationDecisionID.Bytes,
		})
		if decisionErr != nil && !errors.Is(decisionErr, pgx.ErrNoRows) {
			writeErr(w, http.StatusInternalServerError, decisionErr.Error())
			return
		}
		if decisionErr == nil {
			response["arbitration_decision"] = decision
		}
	}
	writeJSON(w, http.StatusOK, response)
}

type adminDiscoveryReviewResolutionRequest struct {
	Action                   discovery.ReviewResolutionAction `json:"action"`
	Owner                    discovery.Owner                  `json:"owner"`
	OverlapWorkIDs           []uuid.UUID                      `json:"overlap_work_ids"`
	ExpectedCandidateVersion int64                            `json:"expected_candidate_version"`
	ExpectedBucketVersions   map[string]int64                 `json:"expected_bucket_versions"`
	Reason                   string                           `json:"reason"`
	SnoozedUntil             time.Time                        `json:"snoozed_until"`
}

func (s *Server) resolveAdminDiscoveryReview(w http.ResponseWriter, r *http.Request) {
	projectID, candidateID, ok := adminDiscoveryObjectIDs(w, r)
	if !ok {
		return
	}
	if s.Pool == nil || s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	var body adminDiscoveryReviewResolutionRequest
	if err := decodeAdminDiscoveryJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	request := discovery.ReviewResolutionRequest{
		ProjectID: projectID, CandidateID: candidateID, Action: body.Action, Owner: body.Owner,
		OverlapWorkIDs: body.OverlapWorkIDs, ExpectedCandidateVersion: body.ExpectedCandidateVersion,
		ExpectedBucketVersions: body.ExpectedBucketVersions, ResolvedBy: s.adminAuditActor(r),
		Reason: body.Reason, SnoozedUntil: body.SnoozedUntil,
	}
	store := discovery.NewPostgresArbitrationStore(s.Pool, s.Q)
	result, err := discovery.NewReviewService(store).Resolve(r.Context(), request)
	if err != nil {
		switch {
		case errors.Is(err, discovery.ErrSnapshotStale):
			writeErr(w, http.StatusConflict, err.Error())
		case errors.Is(err, pgx.ErrNoRows):
			writeErr(w, http.StatusNotFound, "discovery review item not found")
		default:
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getAdminDiscoverySemanticEvaluation(w http.ResponseWriter, r *http.Request) {
	projectID, ok := adminDiscoveryProjectID(w, r)
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	row, err := s.Q.GetLatestDiscoverySemanticEvaluation(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "semantic evaluation not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, semanticEvaluationResponseFromDB(row))
}

type adminSemanticEvaluationRunRequest struct {
	DatasetVersion              string  `json:"dataset_version"`
	ConfidenceThreshold         float64 `json:"confidence_threshold"`
	DuplicateSafetyRecallTarget float64 `json:"duplicate_safety_recall_target"`
	FalseSuppressionRateTarget  float64 `json:"false_suppression_rate_target"`
	WeeklyOpsCapacity           int     `json:"weekly_ops_capacity"`
	EnableAutomaticSuppression  bool    `json:"enable_automatic_suppression"`
}

func (s *Server) runAdminDiscoverySemanticEvaluation(w http.ResponseWriter, r *http.Request) {
	projectID, ok := adminDiscoveryProjectID(w, r)
	if !ok {
		return
	}
	if s.Pool == nil || s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	var body adminSemanticEvaluationRunRequest
	if err := decodeAdminDiscoveryJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	store := discovery.NewPostgresArbitrationStore(s.Pool, s.Q)
	result, err := discovery.NewSemanticEvaluationService(store).Run(r.Context(), projectID, discovery.SemanticEvaluationPolicy{
		DatasetVersion: body.DatasetVersion, ConfidenceThreshold: body.ConfidenceThreshold,
		DuplicateSafetyRecallTarget: body.DuplicateSafetyRecallTarget,
		FalseSuppressionRateTarget:  body.FalseSuppressionRateTarget,
		WeeklyOpsCapacity:           body.WeeklyOpsCapacity,
	}, body.EnableAutomaticSuppression, s.adminAuditActor(r))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func adminDiscoveryObjectIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := adminDiscoveryProjectID(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	candidateID, err := uuid.Parse(chi.URLParam(r, "candidateID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad candidate id")
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, candidateID, true
}

func optionalQuery(r *http.Request, key string) *string {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil
	}
	return &value
}

func decodeAdminDiscoveryJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON object")
	}
	return nil
}

func (s *Server) adminAuditActor(r *http.Request) string {
	if s.Env.ClerkSecretKey == "" {
		return "local-admin"
	}
	if claims, ok := clerk.SessionClaimsFromContext(r.Context()); ok && claims != nil {
		subject := strings.TrimSpace(claims.Subject)
		if email := strings.TrimSpace(s.userEmail(r.Context(), subject)); email != "" {
			return email
		}
		if subject != "" {
			return subject
		}
	}
	return "platform-admin"
}

type adminSemanticEvaluationResponse struct {
	discovery.SemanticEvaluationResult
	AutomaticSuppressionEnabled bool      `json:"automatic_suppression_enabled"`
	EvaluatedBy                 string    `json:"evaluated_by"`
	CreatedAt                   time.Time `json:"created_at"`
}

func semanticEvaluationResponseFromDB(row db.DiscoverySemanticEvaluation) adminSemanticEvaluationResponse {
	blockers := []string{}
	_ = json.Unmarshal(row.Blockers, &blockers)
	return adminSemanticEvaluationResponse{
		SemanticEvaluationResult: discovery.SemanticEvaluationResult{
			ID: row.ID, DatasetVersion: row.DatasetVersion,
			ConfidenceThreshold:         numericValue(row.ConfidenceThreshold),
			DuplicateSafetyRecallTarget: numericValue(row.DuplicateSafetyRecallTarget),
			FalseSuppressionRateTarget:  numericValue(row.FalseSuppressionRateTarget),
			TotalCases:                  int(row.TotalCases), DuplicateSafetyCases: int(row.DuplicateSafetyCases),
			DistinctCases: int(row.DistinctCases), DuplicateSafetyRecall: numericValue(row.DuplicateSafetyRecall),
			FalseSuppressionRate: numericValue(row.FalseSuppressionRate), ComparatorCoverage: numericValue(row.ComparatorCoverage),
			AutomatedDispositionCoverage: numericValue(row.AutomatedDispositionCoverage), HoldRate: numericValue(row.HoldRate),
			ThresholdBacklog: int(row.ThresholdBacklog), WeeklyOpsCapacity: int(row.WeeklyOpsCapacity),
			LaunchReady: row.LaunchReady, Blockers: blockers,
		},
		AutomaticSuppressionEnabled: row.AutomaticSuppressionEnabled,
		EvaluatedBy:                 row.EvaluatedBy, CreatedAt: pgTimestamp(row.CreatedAt),
	}
}

func numericValue(value pgtype.Numeric) float64 {
	result, err := value.Float64Value()
	if err != nil || !result.Valid {
		return 0
	}
	return result.Float64
}

func pgTimestamp(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

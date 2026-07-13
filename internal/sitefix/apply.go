package sitefix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/aicalls"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrLifecycleConflict      = errors.New("canonical Site Fix lifecycle conflict")
	ErrPatchGroundingRejected = errors.New("independent patch grounding verification rejected the canonical Site Fix")
)

type GenerationCall struct {
	Provider           string
	Model              string
	PromptVersion      string
	RequestFingerprint string
}

type GenerationResult struct {
	Provider         string
	Model            string
	Status           string
	ErrorCode        string
	PromptTokens     int32
	CompletionTokens int32
	TotalTokens      int32
	CostUSD          float64
}

type siteFixAICallAttempt interface {
	llm.AttemptObserver
	Started() bool
}

// GenerationContext is the bounded, persisted product Context plus the exact
// observed evidence snapshot approved for this Site Fix. It is loaded before
// provider work and never while a lifecycle transaction is open.
type GenerationContext struct {
	ProductProfile   json.RawMessage `json:"product_profile"`
	ProfileVersion   int32           `json:"profile_version"`
	ObservedEvidence json.RawMessage `json:"observed_evidence"`
}

type ApplicationPlan struct {
	TargetURL               string
	NormalizedTargetURL     string
	OpportunityKey          string
	SourceFilePaths         json.RawMessage
	SourceMappingConfidence string
	SourceMappingReason     string
	PatchSnapshot           json.RawMessage
	DiffSnapshot            json.RawMessage
	ResolutionCriteria      json.RawMessage
	GroundingSnapshot       json.RawMessage
	Status                  string
}

type ApplyResult struct {
	SiteFix     db.SiteFix               `json:"site_fix"`
	Application db.SiteChangeApplication `json:"application"`
}

type FixGenerator interface {
	Describe(db.SiteFix, GenerationContext, RepositorySnapshot) GenerationCall
	Generate(context.Context, db.SiteFix, GenerationContext, RepositorySnapshot, siteFixAICallAttempt) (ApplicationPlan, GenerationResult, error)
}

// PatchVerification is an independent judgment over the actual generated
// patch/diff text. It deliberately does not reuse the generator's grounding
// self-report.
type PatchVerification struct {
	Approved               bool
	PrimaryIntentPreserved bool
	PreservedPropositions  []string
	AddedPropositions      []string
	RemovedPropositions    []string
	UnsupportedClaims      []string
	IntentDrift            bool
	Reason                 string
}

type PatchGroundingVerifier interface {
	Describe(db.SiteFix, GenerationContext, ApplicationPlan) GenerationCall
	Verify(context.Context, db.SiteFix, GenerationContext, ApplicationPlan, siteFixAICallAttempt) (PatchVerification, GenerationResult, error)
}

type ApplyStore interface {
	Load(context.Context, uuid.UUID, uuid.UUID) (db.SiteFix, error)
	LoadGenerationContext(context.Context, db.SiteFix) (GenerationContext, error)
	FindApplication(context.Context, db.SiteFix) (db.SiteChangeApplication, bool, error)
	MarkPreparing(context.Context, db.SiteFix) (db.SiteFix, error)
	StartSourceSelection(context.Context, db.SiteFix, GenerationCall) (uuid.UUID, siteFixAICallAttempt, error)
	FinishSourceSelection(context.Context, db.SiteFix, uuid.UUID, GenerationResult) error
	StartGeneration(context.Context, db.SiteFix, GenerationCall, uuid.UUID) (uuid.UUID, siteFixAICallAttempt, error)
	FinishGeneration(context.Context, db.SiteFix, uuid.UUID, GenerationResult) error
	StartGroundingVerification(context.Context, db.SiteFix, GenerationCall, uuid.UUID) (uuid.UUID, siteFixAICallAttempt, error)
	FinishGroundingVerification(context.Context, db.SiteFix, uuid.UUID, GenerationResult) error
	RecordPreparationFailure(context.Context, db.SiteFix, string) error
	Finalize(context.Context, db.SiteFix, ApplicationPlan) (db.SiteFix, db.SiteChangeApplication, error)
}

type ApplyService struct {
	Store          ApplyStore
	SourceLoader   RepositorySourceLoader
	SourceSelector RepositorySourceSelector
	Generator      FixGenerator
	Verifier       PatchGroundingVerifier
}

func (s ApplyService) Apply(ctx context.Context, projectID, fixID uuid.UUID) (result ApplyResult, resultErr error) {
	if s.Store == nil || s.SourceLoader == nil || s.SourceSelector == nil || s.Generator == nil || s.Verifier == nil {
		return ApplyResult{}, errors.New("canonical Site Fix apply dependencies unavailable")
	}
	fix, err := s.Store.Load(ctx, projectID, fixID)
	if err != nil {
		return ApplyResult{}, err
	}
	if fix.ProjectID != projectID || fix.ID != fixID {
		return ApplyResult{}, ErrProjectMismatch
	}
	if app, ok, err := s.Store.FindApplication(ctx, fix); err != nil {
		return ApplyResult{}, err
	} else if ok && app.Status != "failed" {
		if !app.SiteFixID.Valid || app.ContentActionID.Valid || uuid.UUID(app.SiteFixID.Bytes) != fix.ID {
			return ApplyResult{}, errors.New("canonical application returned an invalid source")
		}
		return ApplyResult{SiteFix: fix, Application: app}, nil
	}
	switch fix.Status {
	case "approved":
		fix, err = s.Store.MarkPreparing(ctx, fix)
		if err != nil {
			return ApplyResult{}, err
		}
	case "preparing":
		// A previous provider attempt may have failed. Retrying creates a new,
		// append-only ai_call_record below.
	default:
		return ApplyResult{}, fmt.Errorf("%w: cannot apply from %s", ErrLifecycleConflict, fix.Status)
	}
	preparationActive := true
	defer func() {
		if !preparationActive || resultErr == nil {
			return
		}
		failureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if persistErr := s.Store.RecordPreparationFailure(failureCtx, fix, safePreparationFailureCode(resultErr)); persistErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("record canonical Site Fix preparation failure: %w", persistErr))
		}
	}()
	generationContext, err := s.Store.LoadGenerationContext(ctx, fix)
	if err != nil {
		return ApplyResult{}, err
	}

	target, candidates, err := s.SourceLoader.Candidates(ctx, fix)
	if err != nil {
		return ApplyResult{}, err
	}
	selectionDescriptor := s.SourceSelector.Describe(fix, candidates)
	if strings.TrimSpace(selectionDescriptor.Provider) == "" || strings.TrimSpace(selectionDescriptor.Model) == "" ||
		strings.TrimSpace(selectionDescriptor.PromptVersion) == "" || strings.TrimSpace(selectionDescriptor.RequestFingerprint) == "" {
		return ApplyResult{}, errors.New("repository source selection descriptor is incomplete")
	}
	selectionCallID, selectionAttempt, err := s.Store.StartSourceSelection(ctx, fix, selectionDescriptor)
	if err != nil {
		return ApplyResult{}, err
	}
	selectedPaths, selection, selectErr := s.SourceSelector.Select(ctx, fix, candidates, selectionAttempt)
	if !selectionAttempt.Started() && selection.Status != "skipped" {
		selection.Status, selection.ErrorCode = "skipped", "provider_not_called"
		selectErr = errors.Join(selectErr, errors.New("repository source selector returned without reporting a physical attempt"))
	}
	if selectErr != nil && selection.Status == "" {
		selection.Status = "failed"
	}
	selectionFinishCtx, cancelSelectionFinish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelSelectionFinish()
	if finishErr := s.Store.FinishSourceSelection(selectionFinishCtx, fix, selectionCallID, selection); finishErr != nil {
		if selectErr != nil {
			return ApplyResult{}, errors.Join(selectErr, finishErr)
		}
		return ApplyResult{}, finishErr
	}
	if selectErr != nil {
		return ApplyResult{}, selectErr
	}
	if selection.Status != "ok" {
		return ApplyResult{}, fmt.Errorf("repository source selection ended in %q", selection.Status)
	}
	repositorySnapshot, err := s.SourceLoader.LoadSelected(ctx, target, selectedPaths)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := ValidateRepositorySnapshot(repositorySnapshot); err != nil {
		return ApplyResult{}, err
	}
	if repositorySnapshot.Repo != target.Repo || repositorySnapshot.Branch != target.Branch || repositorySnapshot.BaseCommitSHA != target.BaseCommitSHA {
		return ApplyResult{}, errors.New("repository source loader returned a different target snapshot")
	}

	descriptor := s.Generator.Describe(fix, generationContext, repositorySnapshot)
	if strings.TrimSpace(descriptor.Provider) == "" || strings.TrimSpace(descriptor.Model) == "" ||
		strings.TrimSpace(descriptor.PromptVersion) == "" || strings.TrimSpace(descriptor.RequestFingerprint) == "" {
		return ApplyResult{}, errors.New("fix generation descriptor is incomplete")
	}
	callID, generationAttempt, err := s.Store.StartGeneration(ctx, fix, descriptor, selectionCallID)
	if err != nil {
		return ApplyResult{}, err
	}
	plan, generation, generateErr := s.Generator.Generate(ctx, fix, generationContext, repositorySnapshot, generationAttempt)
	if !generationAttempt.Started() && generation.Status != "skipped" {
		generation.Status, generation.ErrorCode = "skipped", "provider_not_called"
		generateErr = errors.Join(generateErr, errors.New("Site Fix generation provider returned without reporting a physical attempt"))
	}
	if generateErr != nil && generation.Status == "" {
		generation.Status = "failed"
	}
	if generateErr == nil {
		if planErr := validateApplicationPlan(fix, generationContext, plan); planErr != nil {
			generation.Status = "failed"
			generation.ErrorCode = "invalid_output"
			generateErr = planErr
		} else if groundedCriteria, criteriaErr := persistGroundingCriteria(plan.ResolutionCriteria, plan.GroundingSnapshot); criteriaErr != nil {
			generation.Status = "failed"
			generation.ErrorCode = "invalid_output"
			generateErr = criteriaErr
		} else {
			plan.ResolutionCriteria = groundedCriteria
		}
	}
	finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelFinish()
	if finishErr := s.Store.FinishGeneration(finishCtx, fix, callID, generation); finishErr != nil {
		if generateErr != nil {
			return ApplyResult{}, errors.Join(generateErr, finishErr)
		}
		return ApplyResult{}, finishErr
	}
	if generateErr != nil {
		return ApplyResult{}, generateErr
	}
	if generation.Status != "ok" && generation.Status != "partial" && generation.Status != "skipped" {
		return ApplyResult{}, fmt.Errorf("fix generation ended in %q", generation.Status)
	}
	verificationDescriptor := s.Verifier.Describe(fix, generationContext, plan)
	if strings.TrimSpace(verificationDescriptor.Provider) == "" || strings.TrimSpace(verificationDescriptor.Model) == "" ||
		strings.TrimSpace(verificationDescriptor.PromptVersion) == "" || strings.TrimSpace(verificationDescriptor.RequestFingerprint) == "" {
		return ApplyResult{}, errors.New("independent patch grounding verifier descriptor is incomplete")
	}
	verificationCallID, verificationAttempt, err := s.Store.StartGroundingVerification(ctx, fix, verificationDescriptor, callID)
	if err != nil {
		return ApplyResult{}, err
	}
	verification, verificationGeneration, verificationErr := s.Verifier.Verify(ctx, fix, generationContext, plan, verificationAttempt)
	if !verificationAttempt.Started() && verificationGeneration.Status != "skipped" {
		verificationGeneration.Status, verificationGeneration.ErrorCode = "skipped", "provider_not_called"
		verificationErr = errors.Join(verificationErr, errors.New("Site Fix grounding provider returned without reporting a physical attempt"))
	}
	if verificationErr != nil && verificationGeneration.Status == "" {
		verificationGeneration.Status = "failed"
	}
	if verificationErr == nil {
		verificationErr = validatePatchVerification(fix, verification)
		if verificationErr != nil {
			verificationGeneration.Status = "failed"
			verificationGeneration.ErrorCode = "grounding_rejected"
		}
	}
	verificationFinishCtx, cancelVerificationFinish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelVerificationFinish()
	if finishErr := s.Store.FinishGroundingVerification(verificationFinishCtx, fix, verificationCallID, verificationGeneration); finishErr != nil {
		if verificationErr != nil {
			return ApplyResult{}, errors.Join(verificationErr, finishErr)
		}
		return ApplyResult{}, finishErr
	}
	if verificationErr != nil {
		return ApplyResult{}, verificationErr
	}
	if verificationGeneration.Status != "ok" && verificationGeneration.Status != "skipped" {
		return ApplyResult{}, fmt.Errorf("independent patch grounding verification ended in %q", verificationGeneration.Status)
	}
	fix, app, err := s.Store.Finalize(ctx, fix, plan)
	if err != nil {
		return ApplyResult{}, err
	}
	preparationActive = false
	if !app.SiteFixID.Valid || app.ContentActionID.Valid || uuid.UUID(app.SiteFixID.Bytes) != fix.ID {
		return ApplyResult{}, errors.New("canonical application returned an invalid source")
	}
	return ApplyResult{SiteFix: fix, Application: app}, nil
}

func safePreparationFailureCode(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "preparation_interrupted"
	}
	if errors.Is(err, ErrPatchGroundingRejected) {
		return "grounding_rejected"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "repository patch"), strings.Contains(message, "exact replacement"):
		return "invalid_repository_patch"
	case strings.Contains(message, "source selection"), strings.Contains(message, "source selector"):
		return "source_selection_failed"
	case strings.Contains(message, "provider"):
		return "provider_unavailable"
	case strings.Contains(message, "repository"), strings.Contains(message, "source loader"), strings.Contains(message, "blob"), strings.Contains(message, "tree"), strings.Contains(message, "branch"):
		return "repository_source_unavailable"
	default:
		return "preparation_failed"
	}
}

func validatePatchVerification(fix db.SiteFix, verification PatchVerification) error {
	_, expectedPropositions, err := approvedPropositionContract(fix.EvidenceSnapshot)
	if err != nil {
		return err
	}
	if !verification.Approved || !verification.PrimaryIntentPreserved || verification.IntentDrift ||
		len(verification.AddedPropositions) > 0 || len(verification.RemovedPropositions) > 0 || len(verification.UnsupportedClaims) > 0 ||
		!sameNormalizedStrings(verification.PreservedPropositions, expectedPropositions) {
		return ErrPatchGroundingRejected
	}
	return nil
}

func validateApplicationPlan(fix db.SiteFix, generationContext GenerationContext, plan ApplicationPlan) error {
	if strings.TrimSpace(plan.TargetURL) == "" || strings.TrimSpace(plan.NormalizedTargetURL) == "" || strings.TrimSpace(plan.OpportunityKey) == "" {
		return errors.New("generated canonical application is missing target identity")
	}
	if plan.Status != "ready_for_pr" {
		return fmt.Errorf("unsupported canonical application status %q", plan.Status)
	}
	for _, raw := range []json.RawMessage{plan.SourceFilePaths, plan.PatchSnapshot, plan.DiffSnapshot, plan.ResolutionCriteria} {
		if len(raw) == 0 || !json.Valid(raw) {
			return errors.New("generated canonical application contains invalid JSON")
		}
	}
	return validateGroundingSnapshot(fix, generationContext, plan)
}

func validateGroundingSnapshot(fix db.SiteFix, generationContext GenerationContext, plan ApplicationPlan) error {
	if !sameJSON(fix.EvidenceSnapshot, json.RawMessage(generationContext.ObservedEvidence)) {
		return errors.New("generated canonical application used stale observed evidence")
	}
	if len(plan.GroundingSnapshot) == 0 || !json.Valid(plan.GroundingSnapshot) {
		return errors.New("generated canonical application is missing its grounding validation")
	}
	var grounding struct {
		ContextProfileVersion    *int32          `json:"context_profile_version"`
		PrimaryIntentBefore      *string         `json:"primary_intent_before"`
		PrimaryIntentAfter       *string         `json:"primary_intent_after"`
		PreservedPropositions    *[]string       `json:"preserved_propositions"`
		AddedPropositions        *[]string       `json:"added_propositions"`
		RemovedPropositions      *[]string       `json:"removed_propositions"`
		UnsupportedClaims        *[]string       `json:"unsupported_claims"`
		SourceAssociationChanges json.RawMessage `json:"source_association_changes"`
	}
	if err := json.Unmarshal(plan.GroundingSnapshot, &grounding); err != nil {
		return errors.New("generated canonical application contains invalid grounding validation")
	}
	if grounding.ContextProfileVersion == nil || *grounding.ContextProfileVersion != generationContext.ProfileVersion {
		return errors.New("generated canonical application used a stale or missing Context version")
	}
	if grounding.PreservedPropositions == nil || grounding.AddedPropositions == nil || grounding.RemovedPropositions == nil || grounding.UnsupportedClaims == nil || grounding.SourceAssociationChanges == nil {
		return errors.New("generated canonical application omitted proposition-preservation evidence")
	}
	var associationChanges []any
	if json.Unmarshal(grounding.SourceAssociationChanges, &associationChanges) != nil {
		return errors.New("generated canonical application contains invalid source-association evidence")
	}
	if len(*grounding.AddedPropositions) > 0 || len(*grounding.RemovedPropositions) > 0 || len(*grounding.UnsupportedClaims) > 0 {
		return errors.New("Doctor fix generation cannot add, remove, or rely on unsupported propositions")
	}
	expectedIntent, expectedPropositions, err := approvedPropositionContract(fix.EvidenceSnapshot)
	if err != nil {
		return err
	}
	if !sameNormalizedStrings(*grounding.PreservedPropositions, expectedPropositions) {
		return errors.New("generated canonical application did not preserve the approved proposition set")
	}
	if expectedIntent != "" {
		if grounding.PrimaryIntentBefore == nil || grounding.PrimaryIntentAfter == nil ||
			normalizeGroundingText(*grounding.PrimaryIntentBefore) != expectedIntent ||
			normalizeGroundingText(*grounding.PrimaryIntentAfter) != expectedIntent {
			return errors.New("generated canonical application changed the approved primary intent")
		}
	}
	for _, raw := range []json.RawMessage{plan.PatchSnapshot, plan.DiffSnapshot, plan.ResolutionCriteria} {
		if containsUnsupportedPropositionMutation(raw) {
			return errors.New("generated canonical application contains an unsupported proposition mutation")
		}
	}
	return nil
}

func approvedPropositionContract(evidence json.RawMessage) (string, []string, error) {
	finding, err := approvedFindingEvidence(evidence)
	if err != nil {
		return "", nil, errors.New("canonical Site Fix evidence is missing the approved finding snapshot")
	}
	intent := normalizeGroundingText(firstNonEmpty(
		stringValue(finding["primary_intent_before"]),
		stringValue(finding["primary_intent_after"]),
		stringValue(finding["primary_intent"]),
	))
	return intent, normalizedStringList(finding["preserved_propositions"]), nil
}

func approvedFindingEvidence(evidence json.RawMessage) (map[string]any, error) {
	var root map[string]any
	if json.Unmarshal(evidence, &root) != nil || root == nil {
		return nil, errors.New("invalid finding evidence")
	}
	if nested, ok := root["finding"].(map[string]any); ok && nested != nil {
		return nested, nil
	}
	// Migration-derived canonical fixes predate the envelope but persist the
	// provenance-complete Doctor finding evidence directly.
	return root, nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func normalizedStringList(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if strings, ok := value.([]string); ok {
			out := make([]string, 0, len(strings))
			for _, item := range strings {
				if normalized := normalizeGroundingText(item); normalized != "" {
					out = append(out, normalized)
				}
			}
			return out
		}
		return []string{}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			if normalized := normalizeGroundingText(text); normalized != "" {
				out = append(out, normalized)
			}
		}
	}
	return out
}

func normalizeGroundingText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func sameNormalizedStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, item := range left {
		counts[normalizeGroundingText(item)]++
	}
	for _, item := range right {
		normalized := normalizeGroundingText(item)
		if counts[normalized] == 0 {
			return false
		}
		counts[normalized]--
	}
	return true
}

func containsUnsupportedPropositionMutation(raw json.RawMessage) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return true
	}
	var walk func(any) bool
	walk = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			for key, nested := range typed {
				normalizedKey := strings.ToLower(strings.TrimSpace(key))
				if normalizedKey == "added_propositions" || normalizedKey == "new_claims" || normalizedKey == "unsupported_claims" {
					switch candidate := nested.(type) {
					case []any:
						if len(candidate) > 0 {
							return true
						}
					case string:
						if strings.TrimSpace(candidate) != "" {
							return true
						}
					case nil:
					default:
						return true
					}
				}
				if walk(nested) {
					return true
				}
			}
		case []any:
			for _, nested := range typed {
				if walk(nested) {
					return true
				}
			}
		}
		return false
	}
	return walk(value)
}

func persistGroundingCriteria(criteria, grounding json.RawMessage) (json.RawMessage, error) {
	var object map[string]any
	if json.Unmarshal(criteria, &object) != nil || object == nil {
		return nil, errors.New("generated canonical application contains invalid resolution criteria")
	}
	var groundingObject map[string]any
	if json.Unmarshal(grounding, &groundingObject) != nil || groundingObject == nil {
		return nil, errors.New("generated canonical application contains invalid grounding criteria")
	}
	object["grounding_validation"] = groundingObject
	return json.Marshal(object)
}

type PostgresApplyStore struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

func (s PostgresApplyStore) Load(ctx context.Context, projectID, fixID uuid.UUID) (db.SiteFix, error) {
	return s.Q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
}

func (s PostgresApplyStore) LoadGenerationContext(ctx context.Context, fix db.SiteFix) (GenerationContext, error) {
	result := GenerationContext{ProductProfile: json.RawMessage(`{}`), ObservedEvidence: fix.EvidenceSnapshot}
	profile, err := s.Q.GetActiveProfile(ctx, fix.ProjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return GenerationContext{}, err
	}
	result.ProductProfile = rawJSONObject(profile.Profile)
	result.ProfileVersion = profile.Version
	return result, nil
}

func (s PostgresApplyStore) MarkPreparing(ctx context.Context, fix db.SiteFix) (db.SiteFix, error) {
	if _, err := s.Q.MarkCanonicalSiteFixPreparing(ctx, db.MarkCanonicalSiteFixPreparingParams{SiteFixID: fix.ID, ProjectID: fix.ProjectID}); err != nil {
		return db.SiteFix{}, lifecycleError(err)
	}
	return s.Load(ctx, fix.ProjectID, fix.ID)
}

func (s PostgresApplyStore) FindApplication(ctx context.Context, fix db.SiteFix) (db.SiteChangeApplication, bool, error) {
	app, err := s.Q.GetLatestCanonicalSiteFixApplication(ctx, db.GetLatestCanonicalSiteFixApplicationParams{ProjectID: fix.ProjectID, SiteFixID: validPGUUID(fix.ID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SiteChangeApplication{}, false, nil
	}
	return app, err == nil, err
}

func (s PostgresApplyStore) StartSourceSelection(ctx context.Context, fix db.SiteFix, call GenerationCall) (uuid.UUID, siteFixAICallAttempt, error) {
	return s.startFixGenerationCall(ctx, fix, call, uuid.Nil)
}

func (s PostgresApplyStore) FinishSourceSelection(ctx context.Context, fix db.SiteFix, callID uuid.UUID, result GenerationResult) error {
	return s.finishAICall(ctx, fix, callID, result)
}

func (s PostgresApplyStore) StartGeneration(ctx context.Context, fix db.SiteFix, call GenerationCall, causedByCallID uuid.UUID) (uuid.UUID, siteFixAICallAttempt, error) {
	return s.startFixGenerationCall(ctx, fix, call, causedByCallID)
}

func (s PostgresApplyStore) startFixGenerationCall(ctx context.Context, fix db.SiteFix, call GenerationCall, causedByCallID uuid.UUID) (uuid.UUID, siteFixAICallAttempt, error) {
	parentCallID := pgtype.UUID{}
	latest, latestErr := s.Q.GetLatestAICallForRequest(ctx, db.GetLatestAICallForRequestParams{
		ProjectID: fix.ProjectID, Stage: "fix_generation", LinkedObjectType: "site_fix", LinkedObjectID: fix.ID,
		RequestFingerprint: call.RequestFingerprint,
	})
	if latestErr == nil {
		parentCallID = validPGUUID(latest.ID)
	} else if !errors.Is(latestErr, pgx.ErrNoRows) {
		return uuid.Nil, nil, latestErr
	}
	row, err := s.Q.CreateAICallRecord(ctx, db.CreateAICallRecordParams{
		ProjectID: fix.ProjectID, Stage: "fix_generation", LinkedObjectType: "site_fix", LinkedObjectID: fix.ID,
		Provider: call.Provider, Model: call.Model, PromptVersion: call.PromptVersion,
		RequestFingerprint: call.RequestFingerprint, Status: "queued", ParentCallID: parentCallID,
		CausedByCallID: validPGUUIDOrEmpty(causedByCallID),
	})
	if err != nil {
		return uuid.Nil, nil, err
	}
	return row.ID, aicalls.NewExistingAttemptObserver(s.Q, fix.ProjectID, row.ID), nil
}

func (s PostgresApplyStore) FinishGeneration(ctx context.Context, fix db.SiteFix, callID uuid.UUID, result GenerationResult) error {
	return s.finishAICall(ctx, fix, callID, result)
}

func (s PostgresApplyStore) StartGroundingVerification(ctx context.Context, fix db.SiteFix, call GenerationCall, parentCallID uuid.UUID) (uuid.UUID, siteFixAICallAttempt, error) {
	row, err := s.Q.CreateAICallRecord(ctx, db.CreateAICallRecordParams{
		ProjectID: fix.ProjectID, Stage: "fix_grounding_verification", LinkedObjectType: "site_fix", LinkedObjectID: fix.ID,
		Provider: call.Provider, Model: call.Model, PromptVersion: call.PromptVersion,
		RequestFingerprint: call.RequestFingerprint, Status: "queued", CausedByCallID: validPGUUID(parentCallID),
	})
	if err != nil {
		return uuid.Nil, nil, err
	}
	return row.ID, aicalls.NewExistingAttemptObserver(s.Q, fix.ProjectID, row.ID), nil
}

func (s PostgresApplyStore) FinishGroundingVerification(ctx context.Context, fix db.SiteFix, callID uuid.UUID, result GenerationResult) error {
	return s.finishAICall(ctx, fix, callID, result)
}

func (s PostgresApplyStore) RecordPreparationFailure(ctx context.Context, fix db.SiteFix, code string) error {
	_, err := s.Q.RecordCanonicalSiteFixPreparationFailure(ctx, db.RecordCanonicalSiteFixPreparationFailureParams{
		FailureCode: &code,
		ProjectID:   fix.ProjectID,
		SiteFixID:   fix.ID,
	})
	return lifecycleError(err)
}

func (s PostgresApplyStore) finishAICall(ctx context.Context, fix db.SiteFix, callID uuid.UUID, result GenerationResult) error {
	cost := pgtype.Numeric{}
	if err := cost.Scan(fmt.Sprintf("%.8f", max(result.CostUSD, 0))); err != nil {
		return err
	}
	var errorCode *string
	if strings.TrimSpace(result.ErrorCode) != "" {
		errorCode = &result.ErrorCode
	}
	_, err := s.Q.FinishCanonicalAICallFenced(ctx, db.FinishCanonicalAICallFencedParams{
		Status: result.Status, ErrorCode: errorCode, ResolvedProvider: emptyStringPtr(result.Provider), ResolvedModel: emptyStringPtr(result.Model), PromptTokens: max(result.PromptTokens, 0),
		CompletionTokens: max(result.CompletionTokens, 0), TotalTokens: max(result.TotalTokens, 0),
		CostUsd: cost, ID: callID, ProjectID: fix.ProjectID,
	})
	return err
}

func emptyStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (s PostgresApplyStore) Finalize(ctx context.Context, fix db.SiteFix, plan ApplicationPlan) (db.SiteFix, db.SiteChangeApplication, error) {
	if s.Pool == nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, errors.New("canonical Site Fix database unavailable")
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	if _, err := q.MarkCanonicalSiteFixReadyToApply(ctx, db.MarkCanonicalSiteFixReadyToApplyParams{SiteFixID: fix.ID, ProjectID: fix.ProjectID}); err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, lifecycleError(err)
	}
	if _, err := q.ClaimCanonicalSiteFixApplying(ctx, db.ClaimCanonicalSiteFixApplyingParams{SiteFixID: fix.ID, ProjectID: fix.ProjectID}); err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, lifecycleError(err)
	}
	app, err := q.CreateCanonicalSiteFixApplication(ctx, db.CreateCanonicalSiteFixApplicationParams{
		ID: uuid.New(), ProjectID: fix.ProjectID, SiteFixID: validPGUUID(fix.ID),
		TargetUrl: plan.TargetURL, NormalizedTargetUrl: plan.NormalizedTargetURL,
		OpportunityKey: plan.OpportunityKey, SourceFilePaths: plan.SourceFilePaths,
		SourceMappingConfidence: plan.SourceMappingConfidence, SourceMappingReason: plan.SourceMappingReason,
		PatchSnapshot: plan.PatchSnapshot, DiffSnapshot: plan.DiffSnapshot,
		ResolutionCriteria: plan.ResolutionCriteria, Status: plan.Status,
	})
	if err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, lifecycleError(err)
	}
	updated, err := q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fix.ID, ProjectID: fix.ProjectID})
	if err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, err
	}
	return updated, app, nil
}

func lifecycleError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLifecycleConflict
	}
	return err
}

func validPGUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func validPGUUIDOrEmpty(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return validPGUUID(id)
}

// LLMApplicationGenerator grounds a narrow manual/PR-ready handoff in the
// canonical Site Fix snapshots. It performs no database work.
type LLMApplicationGenerator struct {
	Provider llm.Provider
	Model    string
}

func rawJSONObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage(`{}`)
	}
	return raw
}

func approvedGroundingSnapshot(fix db.SiteFix, generationContext GenerationContext) (json.RawMessage, error) {
	intent, propositions, err := approvedPropositionContract(fix.EvidenceSnapshot)
	if err != nil {
		return nil, err
	}
	finding, err := approvedFindingEvidence(fix.EvidenceSnapshot)
	if err != nil {
		return nil, err
	}
	changes := finding["source_association_changes"]
	if changes == nil {
		changes = []any{}
	}
	return json.Marshal(map[string]any{
		"context_profile_version":    generationContext.ProfileVersion,
		"primary_intent_before":      intent,
		"primary_intent_after":       intent,
		"preserved_propositions":     propositions,
		"added_propositions":         []string{},
		"removed_propositions":       []string{},
		"unsupported_claims":         []string{},
		"source_association_changes": changes,
	})
}

func (g LLMApplicationGenerator) Describe(fix db.SiteFix, generationContext GenerationContext, repository RepositorySnapshot) GenerationCall {
	req := g.completionRequest(fix, generationContext, repository)
	return GenerationCall{Provider: "tokengate", Model: firstNonEmpty(g.Model, llm.DefaultTokenGateModel), PromptVersion: "doctor-repository-patch-generation-v1", RequestFingerprint: aicalls.Fingerprint(req)}
}

func (g LLMApplicationGenerator) completionRequest(fix db.SiteFix, generationContext GenerationContext, repository RepositorySnapshot) llm.CompletionReq {
	target, _ := firstTargetURL(fix.TargetUrls)
	prompt, _ := json.Marshal(map[string]any{
		"target_url": target, "context": generationContext, "evidence": fix.EvidenceSnapshot,
		"proposed_fix": fix.ProposedFix, "acceptance_tests": fix.AcceptanceTests, "repository": repository,
	})
	return llm.CompletionReq{
		System:  "You generate a minimal exact-replacement patch for existing repository files. Use only the supplied SHA-pinned source contents, Product Context, approved evidence, proposed fix, and acceptance tests. Do not invent paths, create files, add unrelated edits, change product intent, or introduce unsupported claims. Return strict JSON only.",
		Prompt:  "Return exactly one RepositoryPatch JSON object: {\"files\":[{\"path\":string,\"base_sha\":string,\"replacements\":[{\"old_text\":string,\"new_text\":string}]}]}. Every path and base_sha must exactly match a supplied source; each old_text must occur exactly once. Return no diff, grounding self-report, prose, or markdown.\n" + string(prompt),
		Purpose: llm.PurposeSiteFix, Model: firstNonEmpty(g.Model, llm.DefaultTokenGateModel), JSON: true, MaxTokens: 4000, DisableProviderFallback: true,
	}
}

func (g LLMApplicationGenerator) Generate(ctx context.Context, fix db.SiteFix, generationContext GenerationContext, repository RepositorySnapshot, attempt siteFixAICallAttempt) (ApplicationPlan, GenerationResult, error) {
	if g.Provider == nil {
		return ApplicationPlan{}, GenerationResult{Provider: "none", Model: "none", Status: "skipped", ErrorCode: "provider_unavailable"}, errors.New("Doctor fix generation provider is unavailable")
	}
	target, err := firstTargetURL(fix.TargetUrls)
	if err != nil {
		return ApplicationPlan{}, GenerationResult{Status: "skipped", ErrorCode: "invalid_target"}, err
	}
	if !meaningfulJSON(generationContext.ProductProfile) || !meaningfulJSON(generationContext.ObservedEvidence) {
		return ApplicationPlan{}, GenerationResult{Status: "skipped", ErrorCode: "missing_grounding_context"}, errors.New("Doctor fix generation requires Product Context and observed page evidence")
	}
	grounding, err := approvedGroundingSnapshot(fix, generationContext)
	if err != nil {
		return ApplicationPlan{}, GenerationResult{Status: "skipped", ErrorCode: "invalid_snapshot"}, err
	}
	if err := ValidateRepositorySnapshot(repository); err != nil {
		return ApplicationPlan{}, GenerationResult{Status: "skipped", ErrorCode: "invalid_repository_snapshot"}, err
	}
	req := g.completionRequest(fix, generationContext, repository)
	req.AttemptObserver = attempt
	resp, err := llm.CompleteObserved(ctx, g.Provider, req)
	result := GenerationResult{Provider: firstNonEmpty(resp.Provider, "tokengate"), Model: firstNonEmpty(resp.Model, g.Model), Status: "ok", PromptTokens: int32(max(resp.PromptTokens, 0)), CompletionTokens: int32(max(resp.CompletionTokens, 0)), TotalTokens: int32(max(resp.Tokens, 0)), CostUSD: resp.CostUSD}
	if err != nil {
		result.Status, result.ErrorCode = "failed", "provider_error"
		return ApplicationPlan{}, result, err
	}
	var patch RepositoryPatch
	if err := decodeJSONObject(resp.Text, &patch); err != nil {
		result.Status, result.ErrorCode = "failed", "invalid_response"
		return ApplicationPlan{}, result, err
	}
	updates, actualDiff, err := ApplyRepositoryPatch(repository, patch)
	if err != nil {
		result.Status, result.ErrorCode = "failed", "invalid_repository_patch"
		return ApplicationPlan{}, result, err
	}
	preparedPatch, err := BuildRepositoryPreparedPatch(repository, patch, updates)
	if err != nil {
		result.Status, result.ErrorCode = "failed", "invalid_repository_patch"
		return ApplicationPlan{}, result, err
	}
	metadata := repositoryApplicationMetadata(fix)
	preparedPatch, err = mergeRepositoryArtifactMetadata(preparedPatch, metadata)
	if err != nil {
		result.Status, result.ErrorCode = "failed", "invalid_snapshot"
		return ApplicationPlan{}, result, err
	}
	actualDiff, err = mergeRepositoryArtifactMetadata(actualDiff, metadata)
	if err != nil {
		result.Status, result.ErrorCode = "failed", "invalid_snapshot"
		return ApplicationPlan{}, result, err
	}
	criteria, err := repositoryResolutionCriteria(fix, metadata)
	if err != nil {
		result.Status, result.ErrorCode = "failed", "invalid_snapshot"
		return ApplicationPlan{}, result, err
	}
	paths := make([]string, 0, len(updates))
	for _, update := range updates {
		paths = append(paths, update.Path)
	}
	sourcePaths, err := json.Marshal(paths)
	if err != nil {
		result.Status, result.ErrorCode = "failed", "invalid_snapshot"
		return ApplicationPlan{}, result, err
	}
	plan := ApplicationPlan{
		TargetURL: target, NormalizedTargetURL: target, OpportunityKey: "doctor:" + fix.ID.String(),
		SourceFilePaths: sourcePaths, SourceMappingConfidence: "high",
		SourceMappingReason: "Selected from the configured repository tree and loaded by immutable blob SHA.",
		PatchSnapshot:       preparedPatch, DiffSnapshot: actualDiff, ResolutionCriteria: criteria, GroundingSnapshot: grounding,
		Status: "ready_for_pr",
	}
	return plan, result, nil
}

func repositoryApplicationMetadata(fix db.SiteFix) map[string]any {
	metadata := map[string]any{}
	var proposed map[string]any
	if json.Unmarshal(fix.ProposedFix, &proposed) == nil {
		for _, key := range []string{"asset_type", "proposed_change", "proposed_metadata", "proposed_value", "proposed_title", "proposed_meta_description", "field"} {
			if value, ok := proposed[key]; ok {
				metadata[key] = value
			}
		}
	}
	if _, ok := metadata["asset_type"]; !ok {
		corpus := strings.ToLower(fix.FindingKind + " " + string(fix.ProposedFix))
		switch {
		case containsAny(corpus, []string{"title", "meta_description", "meta description", "canonical"}):
			metadata["asset_type"] = "metadata_rewrite"
		case strings.Contains(corpus, "sitemap"):
			metadata["asset_type"] = "sitemap_patch"
		case containsAny(corpus, []string{"schema", "jsonld", "json-ld", "structured data"}):
			metadata["asset_type"] = "schema_patch"
		case strings.Contains(corpus, "robots"):
			metadata["asset_type"] = "robots_patch"
		case containsAny(corpus, []string{"internal-link", "internal link"}):
			metadata["asset_type"] = "internal_link_patch"
		default:
			metadata["asset_type"] = firstNonEmpty(fix.FindingKind, "site_fix")
		}
	}
	return metadata
}

func mergeRepositoryArtifactMetadata(raw json.RawMessage, metadata map[string]any) (json.RawMessage, error) {
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, errors.New("repository artifact is not a JSON object")
	}
	for key, value := range metadata {
		object[key] = value
	}
	return json.Marshal(object)
}

func repositoryResolutionCriteria(fix db.SiteFix, metadata map[string]any) (json.RawMessage, error) {
	criteria := map[string]any{"acceptance_tests": json.RawMessage(fix.AcceptanceTests)}
	for key, value := range metadata {
		criteria[key] = value
	}
	return json.Marshal(criteria)
}

func firstTargetURL(raw json.RawMessage) (string, error) {
	var targets []string
	if json.Unmarshal(raw, &targets) != nil || len(targets) == 0 || strings.TrimSpace(targets[0]) == "" {
		return "", errors.New("canonical Site Fix target URL is missing")
	}
	return strings.TrimSpace(targets[0]), nil
}

func decodeJSONObject(text string, out any) error {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return errors.New("provider response must contain exactly one JSON object")
	}
	if err := json.Unmarshal([]byte(trimmed), out); err != nil {
		return errors.New("provider response must contain exactly one valid JSON object")
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

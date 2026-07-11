package seo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type DoctorTrigger string

const (
	DoctorTriggerOnboarding   DoctorTrigger = "onboarding"
	DoctorTriggerManual       DoctorTrigger = "manual"
	DoctorTriggerWeekly       DoctorTrigger = "weekly"
	DoctorTriggerPostPublish  DoctorTrigger = "post_publish"
	manualDoctorRunLimit                    = 3
	manualDoctorRunRateWindow               = time.Hour
)

type DoctorStage string

const (
	DoctorStageQueued        DoctorStage = "queued"
	DoctorStageDiscovering   DoctorStage = "discovering"
	DoctorStageCrawling      DoctorStage = "crawling"
	DoctorStageChecking      DoctorStage = "checking"
	DoctorStageClassifying   DoctorStage = "classifying"
	DoctorStageWritingReport DoctorStage = "writing_report"
	DoctorStageHandoff       DoctorStage = "handoff"
	DoctorStageCompleted     DoctorStage = "completed"
)

type DoctorRunRequest struct {
	ProjectID       uuid.UUID
	Trigger         DoctorTrigger
	SiteURL         string
	CreatedByUserID *string
}

type DoctorReport struct {
	Run      db.SeoDoctorRun       `json:"run"`
	Findings []db.SeoDoctorFinding `json:"findings"`
	Human    DoctorHumanReport     `json:"human_report"`
}

type DoctorHumanReport struct {
	HealthScore int            `json:"health_score"`
	Status      string         `json:"status"`
	Summary     string         `json:"summary"`
	IssueCounts map[string]int `json:"issue_counts"`
	CheckedURLs int            `json:"checked_urls"`
}

type doctorCheckCoverage struct {
	Check       string   `json:"check"`
	CheckedURLs []string `json:"checked_urls"`
	PassedURLs  []string `json:"passed_urls"`
	FailedURLs  []string `json:"failed_urls"`
	SkippedURLs []string `json:"skipped_urls"`
}

type doctorFindingCandidate struct {
	ProjectID             string
	FindingKey            string
	Severity              string
	Category              string
	IssueType             string
	Status                string
	AffectedURLs          []string
	NormalizedURLs        []string
	CanonicalTarget       string
	StructuralLocator     string
	Evidence              map[string]any
	WhyItMatters          string
	FixIntent             string
	DeveloperInstructions string
	LikelyFilesOrSurfaces []string
	AcceptanceTests       []string
	RiskLevel             string
	ReviewRequired        bool
	AutofixEligible       bool
	Confidence            int
	ConfidenceLabel       string
	ImportanceLabel       string
	ImportanceMultiplier  float64
	LinkedOpportunityID   pgtype.UUID
	LinkedContentActionID pgtype.UUID
	FindingKind           string
}

type soft404Evidence struct {
	CanonicalHost  bool
	GeneratedPath  bool
	ExpectedStatus int
	Probes         []soft404Probe
}

type soft404Probe struct {
	URL        string
	StatusCode int
	Similarity float64
}

var doctorStageStarts = map[DoctorStage]int{
	DoctorStageQueued:        0,
	DoctorStageDiscovering:   10,
	DoctorStageCrawling:      25,
	DoctorStageChecking:      50,
	DoctorStageClassifying:   75,
	DoctorStageWritingReport: 88,
	DoctorStageCompleted:     100,
}

var doctorStageOrder = []DoctorStage{
	DoctorStageQueued,
	DoctorStageDiscovering,
	DoctorStageCrawling,
	DoctorStageChecking,
	DoctorStageClassifying,
	DoctorStageWritingReport,
	DoctorStageCompleted,
}

func (s Service) StartDoctorRun(ctx context.Context, req DoctorRunRequest) (db.SeoDoctorRun, bool, error) {
	if s.Q == nil {
		return db.SeoDoctorRun{}, false, errors.New("database unavailable")
	}
	if req.ProjectID == uuid.Nil {
		return db.SeoDoctorRun{}, false, errors.New("project id required")
	}
	trigger := req.Trigger
	if trigger == "" {
		trigger = DoctorTriggerManual
	}
	if active, err := s.Q.GetActiveSEODoctorRun(ctx, req.ProjectID); err == nil {
		return active, false, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.SeoDoctorRun{}, false, err
	}
	now := s.now()
	if trigger == DoctorTriggerManual {
		count, err := s.Q.CountManualSEODoctorRunsSince(ctx, db.CountManualSEODoctorRunsSinceParams{
			ProjectID: req.ProjectID,
			CreatedAt: pgtype.Timestamptz{Time: now.Add(-manualDoctorRunRateWindow), Valid: true},
		})
		if err != nil {
			return db.SeoDoctorRun{}, false, err
		}
		if count >= manualDoctorRunLimit {
			return db.SeoDoctorRun{}, false, fmt.Errorf("manual seo doctor run limit reached")
		}
	}
	run, err := s.Q.CreateSEODoctorRun(ctx, db.CreateSEODoctorRunParams{
		ProjectID:       req.ProjectID,
		Trigger:         string(trigger),
		Status:          "queued",
		Stage:           string(DoctorStageQueued),
		ProgressPercent: 0,
		Message:         "Doctor is queued.",
		InputSnapshot: mustJSON(map[string]any{
			"site_url": strings.TrimSpace(req.SiteURL),
			"trigger":  trigger,
		}),
		CreatedByUserID: req.CreatedByUserID,
	})
	return run, err == nil, err
}

func (s Service) RunDoctor(ctx context.Context, projectID, runID uuid.UUID) (DoctorReport, error) {
	if s.Q == nil {
		return DoctorReport{}, errors.New("database unavailable")
	}
	run, err := s.Q.GetSEODoctorRun(ctx, db.GetSEODoctorRunParams{ID: runID, ProjectID: projectID})
	if err != nil {
		return DoctorReport{}, err
	}
	if run.Status == "completed" {
		return s.DoctorReport(ctx, run.ProjectID, run.ID)
	}

	startedAt := pgtype.Timestamptz{Time: s.now(), Valid: true}
	run, err = s.Q.UpdateSEODoctorRunProgress(ctx, db.UpdateSEODoctorRunProgressParams{
		ID:              run.ID,
		ProjectID:       run.ProjectID,
		Status:          "running",
		Stage:           string(DoctorStageDiscovering),
		ProgressPercent: int32(doctorStageStarts[DoctorStageDiscovering]),
		Message:         "Discovering site and existing SEO checks.",
		StartedAt:       startedAt,
	})
	if err != nil {
		return DoctorReport{}, err
	}

	siteURL := doctorRunSiteURL(run)
	prop, err := s.ensureProperty(ctx, run.ProjectID, siteURL)
	if err != nil {
		return s.failDoctorRun(ctx, run, DoctorStageDiscovering, "Could not load SEO property.", err)
	}
	siteURL = prop.SiteUrl
	checks, err := s.Q.ListLatestTechnicalChecks(ctx, db.ListLatestTechnicalChecksParams{
		ProjectID: run.ProjectID,
		LimitRows: 100,
	})
	if err != nil {
		return s.failDoctorRun(ctx, run, DoctorStageDiscovering, "Could not read technical checks.", err)
	}
	if len(checks) == 0 {
		run, _ = s.Q.UpdateSEODoctorRunProgress(ctx, db.UpdateSEODoctorRunProgressParams{
			ID:              run.ID,
			ProjectID:       run.ProjectID,
			Status:          "running",
			Stage:           string(DoctorStageCrawling),
			ProgressPercent: int32(doctorProgressPercent(DoctorStageCrawling, 0, 0)),
			Message:         "Running a bounded public technical crawl.",
			StartedAt:       startedAt,
		})
		if _, err := s.Sync(ctx, run.ProjectID, siteURL); err != nil {
			return s.failDoctorRun(ctx, run, DoctorStageCrawling, "Technical crawl failed.", err)
		}
		checks, err = s.Q.ListLatestTechnicalChecks(ctx, db.ListLatestTechnicalChecksParams{
			ProjectID: run.ProjectID,
			LimitRows: 100,
		})
		if err != nil {
			return s.failDoctorRun(ctx, run, DoctorStageChecking, "Could not read crawl results.", err)
		}
	}
	crawlerAccess, err := s.Q.ListLatestAICrawlerAccessSnapshots(ctx, run.ProjectID)
	if err != nil {
		return s.failDoctorRun(ctx, run, DoctorStageChecking, "Could not read crawler-access evidence.", err)
	}

	pages := int32(len(checks))
	run, err = s.Q.UpdateSEODoctorRunProgress(ctx, db.UpdateSEODoctorRunProgressParams{
		ID:              run.ID,
		ProjectID:       run.ProjectID,
		Status:          "running",
		Stage:           string(DoctorStageChecking),
		ProgressPercent: int32(doctorProgressPercent(DoctorStageChecking, len(checks), maxInt(len(checks), 1))),
		Message:         "Classifying technical SEO health signals.",
		PagesDiscovered: pages,
		PagesFetched:    pages,
		PagesChecked:    pages,
		StartedAt:       startedAt,
	})
	if err != nil {
		return DoctorReport{}, err
	}

	candidates, growthCandidates := doctorFindingCandidatesAndGrowthFromChecks(run.ProjectID, checks)
	candidates = append(candidates, doctorFindingCandidatesFromCrawlerAccess(run.ProjectID, crawlerAccess)...)
	for _, candidate := range growthCandidates {
		action := candidate.RecommendedAction
		impact := candidate.ExpectedImpact
		pageURL := strings.TrimSpace(candidate.PageURL)
		var pageURLPtr *string
		if pageURL != "" {
			pageURLPtr = &pageURL
		}
		if _, err := s.Q.UpsertSEOOpportunity(ctx, db.UpsertSEOOpportunityParams{
			ProjectID:         run.ProjectID,
			Type:              candidate.Type,
			Status:            "open",
			PriorityScore:     pgutil.Numeric(candidate.PriorityScore),
			Confidence:        pgutil.Numeric(candidate.Confidence),
			PageUrl:           pageURLPtr,
			NormalizedPageUrl: candidate.NormalizedPageURL,
			Evidence:          mustJSON(candidate.Evidence),
			RecommendedAction: &action,
			ExpectedImpact:    &impact,
			Effort:            candidate.Effort,
			RiskLevel:         candidate.RiskLevel,
			CreatedByRunID:    uuidToPG(run.ID),
		}); err != nil {
			return s.failDoctorRun(ctx, run, DoctorStageClassifying, "Could not route growth work to Opportunities.", err)
		}
	}
	seenKeys := make([]string, 0, len(candidates))
	now := pgtype.Timestamptz{Time: s.now(), Valid: true}
	for _, candidate := range candidates {
		candidate = candidate.withDefaults()
		finding, err := s.Q.UpsertSEODoctorFinding(ctx, candidate.upsertParams(run.ProjectID, run.ID, now))
		if err != nil {
			return s.failDoctorRun(ctx, run, DoctorStageClassifying, "Could not store Doctor findings.", err)
		}
		seenKeys = append(seenKeys, finding.FindingKey)
	}
	if err := s.Q.ResolveMissingSEODoctorFindings(ctx, db.ResolveMissingSEODoctorFindingsParams{
		ProjectID:  run.ProjectID,
		RunID:      run.ID,
		ResolvedAt: now,
		ActiveKeys: seenKeys,
	}); err != nil {
		return s.failDoctorRun(ctx, run, DoctorStageClassifying, "Could not resolve previous Doctor findings.", err)
	}

	score := doctorHealthScore(candidates)
	score32 := int32(score)
	issuesFound := int32(nonInfoIssueCount(candidates))
	human := buildDoctorHumanReport(score, candidates, len(checks))
	human.Status = doctorDisplayStatus(score, candidates)
	coverage := buildDoctorHealthyCoverage(checks, crawlerAccess)
	if human.Status == "healthy" && !doctorCoverageComplete(coverage) {
		human.Status = "needs_attention"
	}
	run, err = s.Q.CompleteSEODoctorRun(ctx, db.CompleteSEODoctorRunParams{
		ID:              run.ID,
		ProjectID:       run.ProjectID,
		Message:         "Doctor report is ready.",
		PagesDiscovered: pages,
		PagesFetched:    pages,
		PagesChecked:    pages,
		IssuesFound:     issuesFound,
		HealthScore:     &score32,
		OutputSummary: mustJSON(map[string]any{
			"human_report":     human,
			"status":           human.Status,
			"healthy_coverage": coverage,
			"growth_rerouted":  len(growthCandidates),
		}),
		HealthyCoverage: mustJSON(coverage),
		FinishedAt:      pgtype.Timestamptz{Time: s.now(), Valid: true},
	})
	if err != nil {
		return DoctorReport{}, err
	}
	return s.DoctorReport(ctx, run.ProjectID, run.ID)
}

func (s Service) DoctorLatest(ctx context.Context, projectID uuid.UUID) (DoctorReport, error) {
	run, err := s.Q.LatestSEODoctorRun(ctx, projectID)
	if err != nil {
		return DoctorReport{}, err
	}
	return s.DoctorReport(ctx, projectID, run.ID)
}

func (s Service) DoctorReport(ctx context.Context, projectID, runID uuid.UUID) (DoctorReport, error) {
	run, err := s.Q.GetSEODoctorRun(ctx, db.GetSEODoctorRunParams{ID: runID, ProjectID: projectID})
	if err != nil {
		return DoctorReport{}, err
	}
	findings, err := s.Q.ListSEODoctorFindingsForRun(ctx, db.ListSEODoctorFindingsForRunParams{
		ProjectID: projectID,
		RunID:     runID,
	})
	if err != nil {
		return DoctorReport{}, err
	}
	candidates := doctorCandidatesFromRows(findings)
	score := 100
	if run.HealthScore != nil {
		score = int(*run.HealthScore)
	} else {
		score = doctorHealthScore(candidates)
	}
	human := buildDoctorHumanReport(score, candidates, int(run.PagesChecked))
	human.Status = doctorDisplayStatus(score, candidates)
	var coverage []doctorCheckCoverage
	_ = json.Unmarshal(run.HealthyCoverage, &coverage)
	if human.Status == "healthy" && !doctorCoverageComplete(coverage) {
		human.Status = "needs_attention"
	}
	return DoctorReport{
		Run:      run,
		Findings: nonNilSlice(findings),
		Human:    human,
	}, nil
}

func (s Service) DismissDoctorFinding(ctx context.Context, projectID, findingID uuid.UUID) (db.SeoDoctorFinding, error) {
	return s.Q.DismissSEODoctorFinding(ctx, db.DismissSEODoctorFindingParams{ID: findingID, ProjectID: projectID})
}

func (s Service) failDoctorRun(ctx context.Context, run db.SeoDoctorRun, stage DoctorStage, message string, runErr error) (DoctorReport, error) {
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	_, _ = s.Q.FailSEODoctorRun(ctx, db.FailSEODoctorRunParams{
		ID:              run.ID,
		ProjectID:       run.ProjectID,
		Status:          "failed",
		Stage:           string(stage),
		ProgressPercent: int32(doctorProgressPercent(stage, 0, 0)),
		Message:         message,
		Error:           strPtr(errText),
		OutputSummary:   mustJSON(map[string]any{"error": errText}),
		FinishedAt:      pgtype.Timestamptz{Time: s.now(), Valid: true},
	})
	return DoctorReport{}, runErr
}

func doctorRunSiteURL(run db.SeoDoctorRun) string {
	var input map[string]any
	_ = json.Unmarshal(run.InputSnapshot, &input)
	if value, ok := input["site_url"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func doctorProgressPercent(stage DoctorStage, completed, total int) int {
	start, ok := doctorStageStarts[stage]
	if !ok {
		return 0
	}
	if stage == DoctorStageCompleted {
		return 100
	}
	next := 100
	for i, candidate := range doctorStageOrder {
		if candidate == stage && i+1 < len(doctorStageOrder) {
			next = doctorStageStarts[doctorStageOrder[i+1]]
			break
		}
	}
	span := next - start
	if span <= 0 {
		return start
	}
	if stage != DoctorStageCrawling && stage != DoctorStageChecking {
		return start
	}
	if total <= 0 {
		return minInt(start+span/2, next-1)
	}
	progress := float64(completed) / float64(total)
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	return minInt(start+int(math.Floor(float64(span)*progress)), next-1)
}

func doctorHealthScore(findings []doctorFindingCandidate) int {
	raw := 0.0
	activeP0 := false
	activeP1 := false
	for _, finding := range findings {
		if !isActiveFinding(finding.Status) || finding.Severity == "Info" {
			continue
		}
		base := severityDeduction(finding.Severity)
		if base == 0 {
			continue
		}
		importance := finding.ImportanceMultiplier
		if importance <= 0 {
			importance = 1
		}
		raw += base * importance * confidenceMultiplier(finding.Confidence)
		if finding.Severity == "P0" {
			activeP0 = true
		}
		if finding.Severity == "P1" {
			activeP1 = true
		}
	}
	score := int(math.Round(100 - math.Min(raw, 100)))
	if score < 0 {
		score = 0
	}
	if activeP0 && score > 69 {
		score = 69
	}
	if activeP1 && score > 84 {
		score = 84
	}
	return score
}

func doctorDisplayStatus(score int, findings []doctorFindingCandidate) string {
	hasP0 := false
	hasP1 := false
	for _, finding := range findings {
		if !isActiveFinding(finding.Status) {
			continue
		}
		hasP0 = hasP0 || finding.Severity == "P0"
		hasP1 = hasP1 || finding.Severity == "P1"
	}
	if hasP0 {
		return "blocked"
	}
	if score >= 90 && !hasP1 {
		return "healthy"
	}
	return "needs_attention"
}

func buildDoctorHumanReport(score int, findings []doctorFindingCandidate, checkedURLs int) DoctorHumanReport {
	counts := map[string]int{"P0": 0, "P1": 0, "P2": 0, "Info": 0}
	for _, finding := range findings {
		if !isActiveFinding(finding.Status) {
			continue
		}
		counts[finding.Severity]++
	}
	issueCount := counts["P0"] + counts["P1"] + counts["P2"]
	noun := "issues"
	if issueCount == 1 {
		noun = "issue"
	}
	return DoctorHumanReport{
		HealthScore: score,
		Status:      doctorDisplayStatus(score, findings),
		Summary:     fmt.Sprintf("%d %s found across %d checked URLs", issueCount, noun, checkedURLs),
		IssueCounts: counts,
		CheckedURLs: checkedURLs,
	}
}

func classifySoft404(input soft404Evidence) doctorFindingCandidate {
	twoXXCount := 0
	highSimilarityCount := 0
	mediumSimilarityCount := 0
	for _, probe := range input.Probes {
		if probe.StatusCode >= 200 && probe.StatusCode < 300 {
			twoXXCount++
		}
		if probe.Similarity >= 0.85 {
			highSimilarityCount++
		}
		if probe.Similarity >= 0.75 {
			mediumSimilarityCount++
		}
	}
	confidenceLabel := "low"
	severity := "P2"
	if len(input.Probes) >= 2 && twoXXCount == len(input.Probes) && highSimilarityCount == len(input.Probes) {
		confidenceLabel = "high"
		if input.CanonicalHost || input.GeneratedPath {
			severity = "P0"
		} else {
			severity = "P1"
		}
	} else if twoXXCount > 0 && mediumSimilarityCount > 0 {
		confidenceLabel = "medium"
		severity = "P1"
	}
	return doctorFindingCandidate{
		IssueType:       "soft_404",
		Severity:        severity,
		Category:        "http",
		Status:          "active",
		Confidence:      ConfidenceValue(confidenceLabel),
		ConfidenceLabel: confidenceLabel,
		Evidence: map[string]any{
			"expected_status": input.ExpectedStatus,
			"probes":          input.Probes,
			"method":          "soft404_v1_similarity",
		},
		WhyItMatters:          "Missing URLs that return successful pages create soft-404 signals and can waste crawl budget.",
		FixIntent:             "Return a real 404/410 for missing URLs, or redirect only to the closest canonical replacement.",
		DeveloperInstructions: "Update routing or middleware so unknown paths return a real not-found response instead of the homepage shell.",
		AcceptanceTests:       []string{"Request two random missing URLs and verify both return 404 or 410."},
	}
}

func ConfidenceValue(label string) int {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "high":
		return 90
	case "medium":
		return 70
	case "low":
		return 50
	default:
		return 50
	}
}

func doctorFindingKey(candidate doctorFindingCandidate) string {
	normalizedURLs := append([]string(nil), candidate.NormalizedURLs...)
	sort.Strings(normalizedURLs)
	parts := []string{
		strings.ToLower(strings.TrimSpace(candidate.ProjectID)),
		strings.ToLower(strings.TrimSpace(candidate.FindingKind)),
		strings.ToLower(strings.TrimSpace(candidate.IssueType)),
		strings.Join(normalizedURLs, ","),
		strings.ToLower(strings.TrimSpace(candidate.CanonicalTarget)),
		strings.ToLower(strings.TrimSpace(candidate.StructuralLocator)),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func doctorFindingCandidatesFromChecks(projectID uuid.UUID, checks []db.TechnicalCheck) []doctorFindingCandidate {
	findings, _ := doctorFindingCandidatesAndGrowthFromChecks(projectID, checks)
	return findings
}

func doctorFindingCandidatesAndGrowthFromChecks(projectID uuid.UUID, checks []db.TechnicalCheck) ([]doctorFindingCandidate, []actionableSEOOpportunityCandidate) {
	out := doctorBrokenFindingCandidatesFromChecks(projectID, checks)
	optimizations, growth := doctorOptimizationCandidatesFromChecks(projectID, checks)
	out = append(out, optimizations...)
	for i := range out {
		out[i] = out[i].withDefaults()
	}
	return out, growth
}

func doctorBrokenFindingCandidatesFromChecks(projectID uuid.UUID, checks []db.TechnicalCheck) []doctorFindingCandidate {
	out := make([]doctorFindingCandidate, 0)
	for _, check := range checks {
		base := doctorFindingCandidate{
			ProjectID:      projectID.String(),
			Status:         "active",
			AffectedURLs:   []string{check.PageUrl},
			NormalizedURLs: []string{check.NormalizedPageUrl},
			Evidence: map[string]any{
				"source":              "technical_checks",
				"page_url":            check.PageUrl,
				"normalized_page_url": check.NormalizedPageUrl,
				"raw_details":         jsonObject(check.RawDetails),
			},
			ImportanceLabel:      "important",
			ImportanceMultiplier: 1,
			Confidence:           80,
			ConfidenceLabel:      "high",
		}
		if check.HttpStatus != nil && (*check.HttpStatus < 200 || *check.HttpStatus >= 300) {
			status := int32(0)
			if check.HttpStatus != nil {
				status = *check.HttpStatus
			}
			out = append(out, base.withIssue("P0", "http", "broken_url", fmt.Sprintf("HTTP status %d blocks reliable indexing.", status), "Return a successful page for valid URLs or a real 404/410 for invalid URLs."))
		}
		switch statusValue(check.RobotsStatus) {
		case "noindex":
			out = append(out, base.withIssue("P0", "indexing", "noindex", "Important pages marked noindex cannot rank.", "Remove noindex from pages that should appear in search."))
		case "blocked", "disallowed":
			out = append(out, base.withIssue("P0", "indexing", "robots_blocked", "Important pages blocked by robots rules cannot be crawled reliably.", "Allow crawlers to access pages that should appear in search."))
		}
		if observedMissing(check.CanonicalStatus) {
			out = append(out, base.withIssue("P1", "canonical", "canonical_missing", "Missing canonical tags make URL consolidation less predictable.", "Add a self-referencing canonical URL or point to the preferred canonical page."))
		}
		if observedMissing(check.StructuredDataStatus) {
			out = append(out, base.withIssue("P1", "structured_data", "structured_data_missing", "Missing structured data reduces eligibility for rich understanding and previews.", "Add valid JSON-LD schema for the page type."))
		}
		if observedMissing(check.TitleStatus) {
			out = append(out, base.withIssue("P2", "metadata", "title_missing", "Missing titles weaken search result clarity.", "Add a concise, unique title tag."))
		}
		if observedMissing(check.MetaDescriptionStatus) {
			out = append(out, base.withIssue("P2", "metadata", "meta_description_missing", "Missing descriptions reduce control over search snippets.", "Add a relevant meta description."))
		}
		if observedMissing(check.H1Status) {
			out = append(out, base.withIssue("P2", "content", "h1_missing", "Missing H1s make page topic hierarchy less clear.", "Add one descriptive H1."))
		}
		if sitemapGapObserved(check.SitemapStatus) {
			candidate := base.withIssue("P2", "sitemap", "important_page_missing_from_sitemap", "Important canonical pages missing from the sitemap are harder for crawlers to discover.", "Include the canonical URL in the sitemap.")
			candidate.Evidence["sitemap_status"] = statusValue(check.SitemapStatus)
			out = append(out, candidate)
		}
		if check.InternalLinkCount != nil && *check.InternalLinkCount == 0 {
			out = append(out, base.withIssue("P2", "links", "internal_link_gap", "Pages without internal links are harder for crawlers and users to discover.", "Add relevant internal links to and from this page."))
		}
		if check.UnsafeMdxDetected {
			out = append(out, base.withIssue("P1", "structured_data", "unsafe_mdx_detected", "Unsafe MDX or template script content can break rendering or schema parsing.", "Move raw script/schema output to a safe rendering path."))
		}
	}
	return out
}

func doctorOptimizationCandidatesFromChecks(projectID uuid.UUID, checks []db.TechnicalCheck) ([]doctorFindingCandidate, []actionableSEOOpportunityCandidate) {
	out := make([]doctorFindingCandidate, 0)
	growth := make([]actionableSEOOpportunityCandidate, 0)
	titles := map[string][]db.TechnicalCheck{}
	for _, check := range checks {
		details := jsonObject(check.RawDetails)
		citation := evidenceObject(details["citation_readiness"])
		added := evidenceStrings(citation["added_propositions"])
		removed := evidenceStrings(citation["removed_propositions"])
		if len(added) > 0 || len(removed) > 0 {
			growth = append(growth, citationFactExpansionCandidate(check, citation))
			continue
		}
		if title := normalizedEvidenceText(details["page_title"]); title != "" {
			titles[strings.ToLower(title)] = append(titles[strings.ToLower(title)], check)
			if len([]rune(title)) > 70 || len([]rune(title)) < 10 {
				out = append(out, doctorOptimizationFinding(projectID, check, "metadata_readability", "metadata", details,
					"Make the existing metadata easier to read without changing the page's intent."))
			}
		}

		if len(citation) == 0 {
			continue
		}
		if normalizedEvidenceText(citation["supported_fact_extractability"]) == "needs_optimization" {
			out = append(out, doctorOptimizationFinding(projectID, check, "supported_fact_extractability", "citation_readiness", details,
				"Reformat existing supported facts into an extractable structure without adding claims."))
		}
		association := normalizedEvidenceText(citation["source_association_status"])
		if association != "" && association != "associated" && association != "healthy" {
			out = append(out, doctorOptimizationFinding(projectID, check, "source_association", "citation_readiness", details,
				"Associate existing propositions with their existing visible sources."))
		}
		entityName := normalizedEvidenceText(citation["entity_name"])
		canonicalName := normalizedEvidenceText(citation["canonical_entity_name"])
		if entityName != "" && canonicalName != "" && !strings.EqualFold(entityName, canonicalName) {
			out = append(out, doctorOptimizationFinding(projectID, check, "entity_naming_consistency", "citation_readiness", details,
				"Use the established entity name consistently without changing the page's proposition set."))
		}
	}
	for _, duplicates := range titles {
		if len(duplicates) < 2 {
			continue
		}
		for _, check := range duplicates {
			out = append(out, doctorOptimizationFinding(projectID, check, "duplicate_metadata_template", "metadata", jsonObject(check.RawDetails),
				"Differentiate the existing page metadata from the duplicated template without changing page intent."))
		}
	}
	return out, growth
}

func doctorOptimizationFinding(projectID uuid.UUID, check db.TechnicalCheck, issueType, category string, details map[string]any, fix string) doctorFindingCandidate {
	citation := evidenceObject(details["citation_readiness"])
	intent := normalizedEvidenceText(details["primary_intent"])
	preserved := evidenceStrings(citation["preserved_propositions"])
	if len(preserved) == 0 {
		preserved = evidenceStrings(details["existing_propositions"])
	}
	evidence := map[string]any{
		"source":                     "technical_checks",
		"page_url":                   check.PageUrl,
		"normalized_page_url":        check.NormalizedPageUrl,
		"finding_kind":               "optimization",
		"primary_intent_before":      intent,
		"primary_intent_after":       intent,
		"preserved_propositions":     preserved,
		"added_propositions":         []string{},
		"removed_propositions":       evidenceStrings(citation["removed_propositions"]),
		"source_association_changes": evidenceList(citation["source_association_changes"]),
	}
	return doctorFindingCandidate{
		ProjectID: projectID.String(), Severity: "P2", Category: category, IssueType: issueType,
		Status: "active", AffectedURLs: []string{check.PageUrl}, NormalizedURLs: []string{check.NormalizedPageUrl},
		Evidence: evidence, WhyItMatters: "The live page can be made clearer and more machine-readable without changing its intent or adding facts.",
		FixIntent: fix, StructuralLocator: issueType, FindingKind: "optimization",
		Confidence: 80, ConfidenceLabel: "high", ImportanceLabel: "important", ImportanceMultiplier: 1,
	}
}

func citationFactExpansionCandidate(check db.TechnicalCheck, citation map[string]any) actionableSEOOpportunityCandidate {
	evidence := actionableEvidence("doctor_citation_readiness", "citation_fact_expansion", check.NormalizedPageUrl, "",
		"growth_route = proposed citation optimization adds propositions not present on live page", "measurement_required",
		"The proposal requires new facts, so its value must be tested as delayed Growth work.",
		[]string{"technical_checks", "doctor_fail_closed"}, map[string]any{
			"preserved_propositions":     evidenceStrings(citation["preserved_propositions"]),
			"added_propositions":         evidenceStrings(citation["added_propositions"]),
			"removed_propositions":       evidenceStrings(citation["removed_propositions"]),
			"source_association_changes": evidenceList(citation["source_association_changes"]),
			"hypothesis":                 "Adding the supported facts may improve citation readiness after a measurement window.",
			"measurement_window_days":    28,
		})
	return actionableSEOOpportunityCandidate{
		Type: "citation_fact_expansion", PageURL: check.PageUrl, NormalizedPageURL: check.NormalizedPageUrl,
		PriorityScore: 62, Confidence: 65,
		RecommendedAction: "Test adding the supported facts as a measured citation-readiness growth hypothesis",
		ExpectedImpact:    "May improve citation readiness after the new propositions are reviewed and measured.",
		Effort:            3, RiskLevel: "medium", Evidence: evidence,
	}
}

func doctorFindingCandidatesFromCrawlerAccess(projectID uuid.UUID, snapshots []db.AiCrawlerAccessSnapshot) []doctorFindingCandidate {
	out := make([]doctorFindingCandidate, 0)
	seen := map[string]bool{}
	for _, snapshot := range snapshots {
		if !strings.EqualFold(snapshot.EvidenceType, "robots_static") || !strings.EqualFold(snapshot.RobotsState, "disallowed") || !strings.EqualFold(snapshot.Confidence, "high") || seen[snapshot.NormalizedPageUrl] {
			continue
		}
		seen[snapshot.NormalizedPageUrl] = true
		out = append(out, doctorFindingCandidate{
			ProjectID: projectID.String(), Severity: "P0", Category: "geo", IssueType: "geo_crawler_access_blocked",
			Status: "active", AffectedURLs: []string{snapshot.PageUrl}, NormalizedURLs: []string{snapshot.NormalizedPageUrl},
			Evidence:          map[string]any{"source": "geo_crawler_access_snapshot", "finding_kind": "broken", "target_user_agent": snapshot.TargetUserAgent, "robots_state": snapshot.RobotsState, "confidence": snapshot.Confidence},
			WhyItMatters:      "An authoritative robots rule blocks an AI/search crawler from the page.",
			FixIntent:         "Review the robots policy and allow the crawler when that matches the project's indexing policy.",
			StructuralLocator: "geo_crawler_access_blocked", FindingKind: "broken", Confidence: 90, ConfidenceLabel: "high", ImportanceLabel: "important", ImportanceMultiplier: 1,
		}.withDefaults())
	}
	return out
}

func (c doctorFindingCandidate) withIssue(severity, category, issueType, why, fix string) doctorFindingCandidate {
	c.Severity = severity
	c.Category = category
	c.IssueType = issueType
	c.WhyItMatters = why
	c.FixIntent = fix
	c.StructuralLocator = issueType
	c.DeveloperInstructions = developerInstructionForIssue(issueType)
	c.AcceptanceTests = acceptanceTestsForIssue(issueType)
	return c
}

func (c doctorFindingCandidate) withDefaults() doctorFindingCandidate {
	if c.Status == "" {
		c.Status = "active"
	}
	if c.Severity == "" {
		c.Severity = "P2"
	}
	if c.Category == "" {
		c.Category = "technical"
	}
	if c.IssueType == "" {
		c.IssueType = "technical_visibility_issue"
	}
	if c.FindingKind == "" {
		c.FindingKind = "broken"
	}
	if c.Confidence == 0 {
		c.Confidence = ConfidenceValue(c.ConfidenceLabel)
	}
	if c.ConfidenceLabel == "" {
		c.ConfidenceLabel = confidenceLabel(c.Confidence)
	}
	if c.ImportanceMultiplier <= 0 {
		c.ImportanceMultiplier = 1
	}
	if c.ImportanceLabel == "" {
		c.ImportanceLabel = "standard"
	}
	if c.RiskLevel == "" {
		c.RiskLevel = riskForSeverity(c.Severity)
	}
	if c.WhyItMatters == "" {
		c.WhyItMatters = "This technical SEO issue can reduce crawl, index, preview, or report reliability."
	}
	if c.FixIntent == "" {
		c.FixIntent = "Fix the technical SEO issue and rerun Doctor."
	}
	if c.DeveloperInstructions == "" {
		c.DeveloperInstructions = developerInstructionForIssue(c.IssueType)
	}
	if len(c.LikelyFilesOrSurfaces) == 0 {
		c.LikelyFilesOrSurfaces = []string{"routing", "page metadata", "sitemap", "structured data"}
	}
	if len(c.AcceptanceTests) == 0 {
		c.AcceptanceTests = acceptanceTestsForIssue(c.IssueType)
	}
	if c.Evidence == nil {
		c.Evidence = map[string]any{}
	}
	c.Evidence["confidence"] = c.Confidence
	c.Evidence["confidence_label"] = c.ConfidenceLabel
	c.Evidence["importance_label"] = c.ImportanceLabel
	c.Evidence["finding_kind"] = c.FindingKind
	if c.FindingKey == "" {
		c.FindingKey = doctorFindingKey(c)
	}
	return c
}

func (c doctorFindingCandidate) upsertParams(projectID, runID uuid.UUID, seenAt pgtype.Timestamptz) db.UpsertSEODoctorFindingParams {
	return db.UpsertSEODoctorFindingParams{
		ProjectID:             projectID,
		RunID:                 runID,
		FindingKey:            c.FindingKey,
		Severity:              c.Severity,
		Category:              c.Category,
		IssueType:             c.IssueType,
		AffectedUrls:          mustJSON(c.AffectedURLs),
		NormalizedUrls:        mustJSON(c.NormalizedURLs),
		Evidence:              mustJSON(c.Evidence),
		WhyItMatters:          c.WhyItMatters,
		FixIntent:             c.FixIntent,
		DeveloperInstructions: c.DeveloperInstructions,
		LikelyFilesOrSurfaces: mustJSON(c.LikelyFilesOrSurfaces),
		AcceptanceTests:       mustJSON(c.AcceptanceTests),
		RiskLevel:             c.RiskLevel,
		ReviewRequired:        c.Severity == "P0" || c.Severity == "P1" || c.ReviewRequired,
		AutofixEligible:       c.AutofixEligible,
		LinkedOpportunityID:   c.LinkedOpportunityID,
		LinkedContentActionID: c.LinkedContentActionID,
		SeenAt:                seenAt,
		FindingKind:           c.FindingKind,
	}
}

func doctorCandidatesFromRows(rows []db.SeoDoctorFinding) []doctorFindingCandidate {
	out := make([]doctorFindingCandidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, doctorFindingCandidate{
			FindingKey:           row.FindingKey,
			Severity:             row.Severity,
			Category:             row.Category,
			IssueType:            row.IssueType,
			Status:               row.Status,
			AffectedURLs:         jsonStringArray(row.AffectedUrls),
			NormalizedURLs:       jsonStringArray(row.NormalizedUrls),
			Evidence:             jsonObject(row.Evidence),
			Confidence:           confidenceFromFinding(row),
			ImportanceMultiplier: 1,
			FindingKind:          row.FindingKind,
		})
	}
	return out
}

func confidenceFromFinding(row db.SeoDoctorFinding) int {
	evidence := jsonObject(row.Evidence)
	switch value := evidence["confidence"].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		return ConfidenceValue(value)
	default:
		return 70
	}
}

func severityDeduction(severity string) float64 {
	switch severity {
	case "P0":
		return 20
	case "P1":
		return 8
	case "P2":
		return 2
	default:
		return 0
	}
}

func confidenceMultiplier(confidence int) float64 {
	switch {
	case confidence >= 80:
		return 1
	case confidence >= 60:
		return 0.75
	default:
		return 0.5
	}
}

func isActiveFinding(status string) bool {
	return status == "" || status == "active"
}

func nonInfoIssueCount(findings []doctorFindingCandidate) int {
	total := 0
	for _, finding := range findings {
		if isActiveFinding(finding.Status) && finding.Severity != "Info" {
			total++
		}
	}
	return total
}

func missingOrUnknown(value *string) bool {
	status := statusValue(value)
	return status == "" || status == "missing" || status == "unknown" || status == "invalid"
}

func observedMissing(value *string) bool {
	status := statusValue(value)
	return status == "missing" || status == "invalid"
}

func sitemapGapObserved(value *string) bool {
	return statusValue(value) == "missing"
}

func statusValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*value))
}

func normalizedEvidenceText(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}

func evidenceObject(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case json.RawMessage:
		return jsonObject(typed)
	case []byte:
		return jsonObject(json.RawMessage(typed))
	default:
		return map[string]any{}
	}
}

func evidenceStrings(value any) []string {
	values := make([]string, 0)
	switch typed := value.(type) {
	case []string:
		values = append(values, typed...)
	case []any:
		for _, item := range typed {
			if text := normalizedEvidenceText(item); text != "" && text != "<nil>" {
				values = append(values, text)
			}
		}
	}
	return values
}

func evidenceList(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case nil:
		return []any{}
	default:
		return []any{typed}
	}
}

func buildDoctorHealthyCoverage(checks []db.TechnicalCheck, crawlerAccess []db.AiCrawlerAccessSnapshot) []doctorCheckCoverage {
	type checkDefinition struct {
		name     string
		classify func(db.TechnicalCheck) string
	}
	statusCheck := func(value func(db.TechnicalCheck) *string) func(db.TechnicalCheck) string {
		return func(check db.TechnicalCheck) string {
			status := statusValue(value(check))
			switch status {
			case "present", "index", "indexed", "allowed", "included", "valid", "ok":
				return "passed"
			case "missing", "invalid", "noindex", "blocked", "disallowed":
				return "failed"
			default:
				return "skipped"
			}
		}
	}
	definitions := []checkDefinition{
		{name: "http_status", classify: func(check db.TechnicalCheck) string {
			if check.HttpStatus == nil {
				return "skipped"
			}
			if *check.HttpStatus >= 200 && *check.HttpStatus < 300 {
				return "passed"
			}
			return "failed"
		}},
		{name: "robots", classify: statusCheck(func(check db.TechnicalCheck) *string { return check.RobotsStatus })},
		{name: "canonical", classify: statusCheck(func(check db.TechnicalCheck) *string { return check.CanonicalStatus })},
		{name: "title", classify: statusCheck(func(check db.TechnicalCheck) *string { return check.TitleStatus })},
		{name: "meta_description", classify: statusCheck(func(check db.TechnicalCheck) *string { return check.MetaDescriptionStatus })},
		{name: "h1", classify: statusCheck(func(check db.TechnicalCheck) *string { return check.H1Status })},
		{name: "structured_data", classify: statusCheck(func(check db.TechnicalCheck) *string { return check.StructuredDataStatus })},
		{name: "sitemap", classify: statusCheck(func(check db.TechnicalCheck) *string { return check.SitemapStatus })},
		{name: "internal_links", classify: func(check db.TechnicalCheck) string {
			if check.InternalLinkCount == nil {
				return "skipped"
			}
			if *check.InternalLinkCount > 0 {
				return "passed"
			}
			return "failed"
		}},
	}
	out := make([]doctorCheckCoverage, 0, len(definitions)+1)
	for _, definition := range definitions {
		coverage := doctorCheckCoverage{Check: definition.name, CheckedURLs: []string{}, PassedURLs: []string{}, FailedURLs: []string{}, SkippedURLs: []string{}}
		for _, check := range checks {
			url := strings.TrimSpace(check.PageUrl)
			if url == "" {
				url = strings.TrimSpace(check.NormalizedPageUrl)
			}
			state := definition.classify(check)
			crawlState := strings.ToLower(normalizedEvidenceText(jsonObject(check.RawDetails)["crawl_status"]))
			if crawlState == "partial" || crawlState == "unchecked" || crawlState == "skipped" {
				state = "skipped"
			}
			switch state {
			case "passed":
				coverage.CheckedURLs = append(coverage.CheckedURLs, url)
				coverage.PassedURLs = append(coverage.PassedURLs, url)
			case "failed":
				coverage.CheckedURLs = append(coverage.CheckedURLs, url)
				coverage.FailedURLs = append(coverage.FailedURLs, url)
			default:
				coverage.SkippedURLs = append(coverage.SkippedURLs, url)
			}
		}
		normalizeDoctorCoverage(&coverage)
		out = append(out, coverage)
	}
	if len(crawlerAccess) > 0 {
		coverage := doctorCheckCoverage{Check: "geo_crawler_access", CheckedURLs: []string{}, PassedURLs: []string{}, FailedURLs: []string{}, SkippedURLs: []string{}}
		for _, snapshot := range crawlerAccess {
			url := snapshot.PageUrl
			authoritativeRobots := strings.EqualFold(snapshot.EvidenceType, "robots_static") && strings.EqualFold(snapshot.Confidence, "high")
			if !authoritativeRobots && (!strings.EqualFold(snapshot.Confidence, "high") || snapshot.Inferred) {
				coverage.SkippedURLs = append(coverage.SkippedURLs, url)
				continue
			}
			coverage.CheckedURLs = append(coverage.CheckedURLs, url)
			if strings.EqualFold(snapshot.RobotsState, "disallowed") || strings.EqualFold(snapshot.AccessState, "blocked") {
				coverage.FailedURLs = append(coverage.FailedURLs, url)
			} else {
				coverage.PassedURLs = append(coverage.PassedURLs, url)
			}
		}
		normalizeDoctorCoverage(&coverage)
		out = append(out, coverage)
	}
	return out
}

func normalizeDoctorCoverage(coverage *doctorCheckCoverage) {
	coverage.CheckedURLs = sortedUniqueStrings(coverage.CheckedURLs)
	coverage.PassedURLs = sortedUniqueStrings(coverage.PassedURLs)
	coverage.FailedURLs = sortedUniqueStrings(coverage.FailedURLs)
	coverage.SkippedURLs = sortedUniqueStrings(coverage.SkippedURLs)
	failed := stringSet(coverage.FailedURLs)
	skipped := stringSet(coverage.SkippedURLs)
	passed := coverage.PassedURLs[:0]
	for _, url := range coverage.PassedURLs {
		if !failed[url] && !skipped[url] {
			passed = append(passed, url)
		}
	}
	coverage.PassedURLs = passed
}

func doctorCoverageComplete(coverage []doctorCheckCoverage) bool {
	if len(coverage) == 0 {
		return false
	}
	for _, check := range coverage {
		if len(check.SkippedURLs) > 0 || len(check.FailedURLs) > 0 || len(check.CheckedURLs) == 0 {
			return false
		}
	}
	return true
}

func sortedUniqueStrings(values []string) []string {
	set := stringSet(values)
	out := make([]string, 0, len(set))
	for value := range set {
		if value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func developerInstructionForIssue(issueType string) string {
	switch issueType {
	case "broken_url":
		return "Inspect the route or deployment rewrite for the affected URL. Valid URLs should return 2xx; intentionally missing URLs should return 404 or 410."
	case "noindex":
		return "Find the robots meta tag or X-Robots-Tag source and remove noindex for pages intended to rank."
	case "canonical_missing", "canonical_mismatch":
		return "Add or correct the canonical tag so the page points to the preferred absolute canonical URL."
	case "structured_data_missing", "structured_data_invalid", "unsafe_mdx_detected":
		return "Validate JSON-LD output, remove template placeholders, and confirm Google's rich result parser can read the schema."
	case "internal_link_gap":
		return "Add contextual internal links from relevant pages and include this URL in navigational or sitemap discovery paths."
	case "soft_404":
		return "Update routing, middleware, or not-found handling so missing paths return 404/410 instead of a homepage-like 200 response."
	default:
		return "Fix the affected SEO surface, deploy the change, and rerun Doctor to verify resolution."
	}
}

func acceptanceTestsForIssue(issueType string) []string {
	switch issueType {
	case "broken_url":
		return []string{"Request the affected URL and verify the final response status matches the intended page state."}
	case "noindex":
		return []string{"Fetch the affected page and verify neither meta robots nor X-Robots-Tag contains noindex."}
	case "canonical_missing", "canonical_mismatch":
		return []string{"Fetch the affected page and verify exactly one absolute canonical URL points to the preferred URL."}
	case "structured_data_missing", "structured_data_invalid", "unsafe_mdx_detected":
		return []string{"Parse the rendered HTML and verify JSON-LD is valid JSON without template placeholders."}
	case "internal_link_gap":
		return []string{"Crawl internal links and verify the affected URL has at least one relevant internal inlink."}
	case "soft_404":
		return []string{"Request two random missing URLs and verify both return 404 or 410."}
	default:
		return []string{"Rerun Doctor and verify the finding is resolved."}
	}
}

func riskForSeverity(severity string) string {
	switch severity {
	case "P0":
		return "high"
	case "P1":
		return "medium"
	default:
		return "low"
	}
}

func confidenceLabel(confidence int) string {
	switch {
	case confidence >= 80:
		return "high"
	case confidence >= 60:
		return "medium"
	default:
		return "low"
	}
}

func jsonObject(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func jsonStringArray(raw json.RawMessage) []string {
	var values []string
	if len(raw) > 0 && json.Unmarshal(raw, &values) == nil {
		return values
	}
	var anyValues []any
	if len(raw) > 0 && json.Unmarshal(raw, &anyValues) == nil {
		for _, value := range anyValues {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
				values = append(values, text)
			}
		}
	}
	if values == nil {
		return []string{}
	}
	return values
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type activityJobDTO struct {
	ID        string             `json:"id"`
	Label     string             `json:"label"`
	Status    string             `json:"status"`
	Detail    string             `json:"detail,omitempty"`
	Href      string             `json:"href,omitempty"`
	RunID     *uuid.UUID         `json:"run_id,omitempty"`
	UpdatedAt pgtype.Timestamptz `json:"updated_at"`
}

type activityRunDTO struct {
	ID        uuid.UUID          `json:"id"`
	ProjectID uuid.UUID          `json:"project_id"`
	Agent     string             `json:"agent"`
	Model     *string            `json:"model"`
	Tokens    *int32             `json:"tokens"`
	CostUsd   pgtype.Numeric     `json:"cost_usd"`
	Status    string             `json:"status"`
	Error     *string            `json:"error"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

type activityInsightDTO struct {
	ProfileReady   bool           `json:"profile_ready"`
	InventoryCount int            `json:"inventory_count"`
	CrawlSummary   map[string]any `json:"crawl_summary,omitempty"`
	LastRunID      *uuid.UUID     `json:"last_run_id,omitempty"`
}

type activityCountsDTO struct {
	Topics        int `json:"topics"`
	PendingReview int `json:"pending_review"`
	Published     int `json:"published"`
}

type activityFailureDTO struct {
	RunID   uuid.UUID          `json:"run_id"`
	Agent   string             `json:"agent"`
	Error   string             `json:"error"`
	Href    string             `json:"href"`
	Created pgtype.Timestamptz `json:"created_at"`
}

type projectActivityDTO struct {
	Jobs             []activityJobDTO     `json:"jobs"`
	ActiveJobs       []activityJobDTO     `json:"active_jobs"`
	RecentRuns       []activityRunDTO     `json:"recent_runs"`
	RecentFailures   []activityFailureDTO `json:"recent_failures"`
	Insight          activityInsightDTO   `json:"insight"`
	Counts           activityCountsDTO    `json:"counts"`
	PublishingHealth publishingHealthDTO  `json:"publishing_health"`
}

func (s *Server) getProjectActivity(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	activity, err := s.projectActivity(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, activity)
}

func (s *Server) projectActivity(ctx context.Context, projectID uuid.UUID) (projectActivityDTO, error) {
	activity := projectActivityDTO{
		Jobs:       []activityJobDTO{},
		ActiveJobs: []activityJobDTO{},
		RecentRuns: []activityRunDTO{},
	}
	if s.Q == nil {
		return activity, nil
	}

	runs, err := s.Q.ListGenerationRuns(ctx, db.ListGenerationRunsParams{
		ProjectID:       projectID,
		CursorCreatedAt: pgtype.Timestamptz{},
		LimitRows:       20,
	})
	if err != nil {
		return activity, err
	}
	activity.RecentRuns = activityRunSummaries(runs, 8)
	activity.RecentFailures = activityFailures(runs, projectID)

	profileReady, profileUpdatedAt, err := s.activityProfileState(ctx, projectID)
	if err != nil {
		return activity, err
	}
	inventory, err := s.Q.ListInventory(ctx, projectID)
	if err != nil {
		return activity, err
	}
	topics, err := s.Q.ListTopics(ctx, projectID)
	if err != nil {
		return activity, err
	}
	pendingReview, err := s.Q.ListPendingReview(ctx, projectID)
	if err != nil {
		return activity, err
	}
	published, err := s.Q.ListArticlesByStatus(ctx, db.ListArticlesByStatusParams{ProjectID: projectID, Status: "published"})
	if err != nil {
		return activity, err
	}
	publishingHealth, err := s.publisherHealth(ctx, projectID)
	if err != nil {
		return activity, err
	}
	activity.PublishingHealth = publishingHealth
	activity.Counts = activityCountsDTO{Topics: len(topics), PendingReview: len(pendingReview), Published: len(published)}
	activity.Insight = activityInsightDTO{
		ProfileReady:   profileReady,
		InventoryCount: len(inventory),
	}

	latestInsight := latestRun(runs, "insight", "")
	profileRun := latestRun(runs, "insight", "profile")
	inventoryRun := latestRun(runs, "insight", "inventory")
	crawlRun := latestRun(runs, "insight", "crawl_summary")
	if crawlRun != nil {
		activity.Insight.LastRunID = &crawlRun.ID
		activity.Insight.CrawlSummary = runOutputObject(*crawlRun, "crawl_summary")
	} else if profileRun != nil {
		activity.Insight.LastRunID = &profileRun.ID
		activity.Insight.CrawlSummary = runOutputObject(*profileRun, "crawl_summary")
	} else if latestInsight != nil {
		activity.Insight.LastRunID = &latestInsight.ID
	}

	activity.Jobs = []activityJobDTO{
		crawlJob(projectID, profileRun, crawlRun, activity.Insight.CrawlSummary, profileReady),
		profileJob(projectID, profileRun, profileReady, profileUpdatedAt),
		inventoryJob(projectID, inventoryRun, profileReady, len(inventory)),
		strategistJob(projectID, latestRun(runs, "strategist", ""), profileReady, len(topics)),
		writerQAJob(projectID, latestRun(runs, "writer", ""), latestRun(runs, "qa", ""), len(topics), len(pendingReview)),
		publisherJob(projectID, publishingHealth),
	}
	return activity, nil
}

func (s *Server) activityProfileState(ctx context.Context, projectID uuid.UUID) (bool, pgtype.Timestamptz, error) {
	profile, err := s.Q.GetActiveProfile(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, pgtype.Timestamptz{}, nil
		}
		return false, pgtype.Timestamptz{}, err
	}
	return true, profile.UpdatedAt, nil
}

func activityRunSummaries(runs []db.GenerationRun, limit int) []activityRunDTO {
	if limit <= 0 || limit > len(runs) {
		limit = len(runs)
	}
	out := make([]activityRunDTO, 0, limit)
	for _, run := range runs[:limit] {
		out = append(out, activityRunDTO{
			ID:        run.ID,
			ProjectID: run.ProjectID,
			Agent:     run.Agent,
			Model:     run.Model,
			Tokens:    run.Tokens,
			CostUsd:   run.CostUsd,
			Status:    run.Status,
			Error:     run.Error,
			CreatedAt: run.CreatedAt,
		})
	}
	return out
}

func activityFailures(runs []db.GenerationRun, projectID uuid.UUID) []activityFailureDTO {
	failures := []activityFailureDTO{}
	for _, run := range runs {
		if run.Status != "error" || run.Error == nil || *run.Error == "" {
			continue
		}
		failures = append(failures, activityFailureDTO{
			RunID:   run.ID,
			Agent:   run.Agent,
			Error:   *run.Error,
			Href:    activityRunHref(projectID, run.Agent),
			Created: run.CreatedAt,
		})
		if len(failures) >= 3 {
			break
		}
	}
	return failures
}

func latestRun(runs []db.GenerationRun, agent string, step string) *db.GenerationRun {
	for i := range runs {
		run := &runs[i]
		if run.Agent != agent {
			continue
		}
		if step != "" && runInputStep(*run) != step {
			continue
		}
		return run
	}
	return nil
}

func runInputStep(run db.GenerationRun) string {
	var payload struct {
		Step string `json:"step"`
	}
	if len(run.Input) == 0 || json.Unmarshal(run.Input, &payload) != nil {
		return ""
	}
	return payload.Step
}

func runOutputObject(run db.GenerationRun, key string) map[string]any {
	var payload map[string]any
	if len(run.Output) == 0 || json.Unmarshal(run.Output, &payload) != nil {
		return nil
	}
	if key == "" {
		return payload
	}
	nested, ok := payload[key].(map[string]any)
	if !ok {
		return nil
	}
	return nested
}

func crawlJob(projectID uuid.UUID, latestInsight *db.GenerationRun, crawlRun *db.GenerationRun, crawlSummary map[string]any, profileReady bool) activityJobDTO {
	job := activityJobDTO{ID: "public_crawl", Label: "Public crawl", Status: "queued", Detail: "Run Insight to crawl the public site.", Href: activityProjectHref(projectID, "knowledge")}
	if latestInsight != nil && latestInsight.Status == "error" {
		job.Status = "failed"
		job.Detail = strValue(latestInsight.Error, "Insight crawl failed.")
		job.RunID = &latestInsight.ID
		job.UpdatedAt = latestInsight.CreatedAt
		return job
	}
	if crawlRun != nil {
		job.RunID = &crawlRun.ID
		job.UpdatedAt = crawlRun.CreatedAt
	}
	if crawlSummary != nil {
		job.Status = "succeeded"
		job.Detail = crawlSummaryDetail(crawlSummary)
		if boolValue(crawlSummary["truncated"]) || len(anySlice(crawlSummary["errors"])) > 0 {
			job.Status = "degraded"
		}
		return job
	}
	if profileReady {
		job.Status = "succeeded"
		job.Detail = "Public crawl has completed for the active profile."
	}
	return job
}

func profileJob(projectID uuid.UUID, latestInsight *db.GenerationRun, ready bool, updatedAt pgtype.Timestamptz) activityJobDTO {
	job := activityJobDTO{ID: "product_profile", Label: "Product profile", Status: "queued", Detail: "Run Insight to extract positioning, ICP, features, and evidence.", Href: activityProjectHref(projectID, "knowledge")}
	if latestInsight != nil && latestInsight.Status == "error" {
		job.Status = "failed"
		job.Detail = strValue(latestInsight.Error, "Profile extraction failed.")
		job.RunID = &latestInsight.ID
		job.UpdatedAt = latestInsight.CreatedAt
		return job
	}
	if ready {
		job.Status = "succeeded"
		job.Detail = "Active profile is ready."
		job.UpdatedAt = updatedAt
	}
	return job
}

func inventoryJob(projectID uuid.UUID, latestInsight *db.GenerationRun, profileReady bool, count int) activityJobDTO {
	job := activityJobDTO{ID: "content_inventory", Label: "Content inventory", Status: "waiting_for_permission", Detail: "Waiting for a product profile before extracting existing content.", Href: activityProjectHref(projectID, "knowledge")}
	if profileReady {
		job.Status = "degraded"
		job.Detail = "No inventory items captured yet."
	}
	if count > 0 {
		job.Status = "succeeded"
		job.Detail = plural(count, "inventory item", "inventory items") + " captured."
	}
	if latestInsight != nil {
		job.RunID = &latestInsight.ID
		job.UpdatedAt = latestInsight.CreatedAt
	}
	return job
}

func strategistJob(projectID uuid.UUID, latestStrategist *db.GenerationRun, profileReady bool, count int) activityJobDTO {
	job := activityJobDTO{ID: "strategist", Label: "Strategist", Status: "waiting_for_permission", Detail: "Waiting for a product profile before generating topics.", Href: activityProjectHref(projectID, "topics")}
	if profileReady {
		job.Status = "queued"
		job.Detail = "Ready to generate topic backlog."
	}
	if count > 0 {
		job.Status = "succeeded"
		job.Detail = plural(count, "topic", "topics") + " in backlog."
	}
	if latestStrategist != nil {
		job.RunID = &latestStrategist.ID
		job.UpdatedAt = latestStrategist.CreatedAt
		if latestStrategist.Status == "error" {
			job.Status = "failed"
			job.Detail = strValue(latestStrategist.Error, "Strategist failed.")
		}
	}
	return job
}

func writerQAJob(projectID uuid.UUID, latestWriter *db.GenerationRun, latestQA *db.GenerationRun, topicCount int, pendingReview int) activityJobDTO {
	job := activityJobDTO{ID: "writer_qa", Label: "Writer and QA", Status: "waiting_for_permission", Detail: "Generate topics before writing drafts.", Href: activityProjectHref(projectID, "review")}
	if topicCount > 0 {
		job.Status = "queued"
		job.Detail = "Ready to write and QA drafts from approved topics."
	}
	if pendingReview > 0 {
		job.Status = "succeeded"
		job.Detail = plural(pendingReview, "draft", "drafts") + " waiting for review."
	}
	for _, run := range []*db.GenerationRun{latestQA, latestWriter} {
		if run == nil {
			continue
		}
		if !run.CreatedAt.Time.IsZero() {
			job.UpdatedAt = run.CreatedAt
		}
		job.RunID = &run.ID
		if run.Status == "error" {
			job.Status = "failed"
			job.Detail = strValue(run.Error, run.Agent+" failed.")
			return job
		}
	}
	return job
}

func publisherJob(projectID uuid.UUID, health publishingHealthDTO) activityJobDTO {
	job := activityJobDTO{ID: "publisher", Label: "Publisher", Status: "waiting_for_permission", Detail: health.NextAction, Href: activityProjectHref(projectID, "publishing")}
	if health.Ready {
		job.Status = "succeeded"
		job.Detail = "Publisher is ready for automatic canonical publishing."
	} else if health.Status == "error" {
		job.Status = "failed"
	}
	return job
}

func activityRunHref(projectID uuid.UUID, agent string) string {
	switch agent {
	case "writer", "qa":
		return activityProjectHref(projectID, "review")
	case "publisher", "notification":
		return activityProjectHref(projectID, "publishing")
	case "seo", "geo":
		return activityProjectHref(projectID, "seo")
	case "insight":
		return activityProjectHref(projectID, "knowledge")
	case "strategist":
		return activityProjectHref(projectID, "topics")
	default:
		return activityProjectHref(projectID, "runs")
	}
}

func activityProjectHref(projectID uuid.UUID, section string) string {
	if section == "" || section == "dashboard" {
		return "/projects/" + projectID.String()
	}
	return "/projects/" + projectID.String() + "/" + section
}

func crawlSummaryDetail(summary map[string]any) string {
	discovered := intValue(summary["discovered_count"])
	fetched := intValue(summary["fetched_count"])
	inventory := intValue(summary["inventory_count"])
	detail := plural(fetched, "page", "pages") + " fetched"
	if discovered > 0 {
		detail += " from " + plural(discovered, "discovered URL", "discovered URLs")
	}
	if inventory > 0 {
		detail += "; " + plural(inventory, "inventory item", "inventory items") + " captured"
	}
	return detail + "."
}

func plural(count int, singular string, plural string) string {
	word := plural
	if count == 1 {
		word = singular
	}
	return strconv.Itoa(count) + " " + word
}

func strValue(value *string, fallback string) string {
	if value != nil && *value != "" {
		return *value
	}
	return fallback
}

func boolValue(value any) bool {
	v, _ := value.(bool)
	return v
}

func intValue(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}

func anySlice(value any) []any {
	if slice, ok := value.([]any); ok {
		return slice
	}
	return nil
}

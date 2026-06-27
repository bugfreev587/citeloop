package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestRefreshContextUsesConfiguredDomainAndMarksManualCrawl(t *testing.T) {
	projectID := uuid.New()
	called := make(chan insightInventoryInput, 1)
	store := &contextRefreshDBSpy{
		projectID: projectID,
		config:    projectConfigWithSiteURL("https://unipost.dev/"),
		profile:   json.RawMessage(`{"context_confirmed_at":"2026-06-20T00:00:00Z"}`),
	}
	srv := &Server{
		Q: db.New(store),
		InsightInventoryRunner: func(ctx context.Context, in insightInventoryInput) {
			called <- in
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/context/refresh", strings.NewReader(`{"landing_url":"https://evil.example"}`))
	rec := httptest.NewRecorder()

	srv.refreshContext(rec, withProjectID(req, projectID))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body = %s, want 202", rec.Code, rec.Body.String())
	}
	select {
	case got := <-called:
		if got.LandingURL != "https://unipost.dev/" {
			t.Fatalf("refresh landing url = %q, want configured project site", got.LandingURL)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("manual context refresh did not start background crawl")
	}
	if !strings.Contains(string(store.updatedProfile), `"context_crawl_source":"manual"`) {
		t.Fatalf("updated profile = %s, want manual crawl source", store.updatedProfile)
	}
	if !strings.Contains(string(store.updatedProfile), `"context_crawl_started_at"`) {
		t.Fatalf("updated profile = %s, want crawl start timestamp", store.updatedProfile)
	}
}

func TestRefreshContextRejectsManualCrawlWithin24Hours(t *testing.T) {
	projectID := uuid.New()
	recentManual := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	store := &contextRefreshDBSpy{
		projectID: projectID,
		config:    projectConfigWithSiteURL("https://unipost.dev/"),
		profile:   json.RawMessage(`{"context_confirmed_at":"2026-06-20T00:00:00Z","context_last_manual_crawled_at":"` + recentManual + `"}`),
	}
	srv := &Server{Q: db.New(store)}

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/context/refresh", nil)
	rec := httptest.NewRecorder()

	srv.refreshContext(rec, withProjectID(req, projectID))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body = %s, want 429", rec.Code, rec.Body.String())
	}
}

func TestRefreshContextRejectsActiveCrawl(t *testing.T) {
	projectID := uuid.New()
	store := &contextRefreshDBSpy{
		projectID: projectID,
		config:    projectConfigWithSiteURL("https://unipost.dev/"),
		profile:   json.RawMessage(`{"context_confirmed_at":"2026-06-20T00:00:00Z","context_crawl_started_at":"2026-06-27T10:00:00Z"}`),
	}
	srv := &Server{Q: db.New(store)}

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/context/refresh", nil)
	rec := httptest.NewRecorder()

	srv.refreshContext(rec, withProjectID(req, projectID))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s, want 409", rec.Code, rec.Body.String())
	}
}

func TestRunInsightRejectsDomainChangeAfterProjectDomainIsSet(t *testing.T) {
	projectID := uuid.New()
	store := &contextRefreshDBSpy{
		projectID: projectID,
		config:    projectConfigWithSiteURL("https://unipost.dev/"),
	}
	srv := &Server{Q: db.New(store)}

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/insight", strings.NewReader(`{"landing_url":"https://other.example/"}`))
	rec := httptest.NewRecorder()

	srv.runInsight(rec, withProjectID(req, projectID))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s, want 400", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "configured domain") {
		t.Fatalf("body = %s, want configured domain error", rec.Body.String())
	}
}

func projectConfigWithSiteURL(siteURL string) json.RawMessage {
	cfg := config.Default()
	cfg.SiteURL = siteURL
	return cfg.JSON()
}

func withProjectID(req *http.Request, projectID uuid.UUID) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("projectID", projectID.String())
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

type contextRefreshDBSpy struct {
	projectID      uuid.UUID
	config         json.RawMessage
	profile        json.RawMessage
	updatedProfile json.RawMessage
}

func (s *contextRefreshDBSpy) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (s *contextRefreshDBSpy) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (s *contextRefreshDBSpy) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	switch {
	case strings.Contains(query, "select id, owner_id, name, slug, config, created_at from projects"):
		return onboardingScanRow{values: []any{s.projectID, "owner", "UniPost", "unipost", s.config, pgtype.Timestamptz{}}}
	case strings.Contains(query, "select id, project_id, source_urls, profile, version, is_active, created_at, updated_at from product_profiles"):
		profile := s.profile
		if len(profile) == 0 {
			profile = json.RawMessage(`{"positioning":"done"}`)
		}
		return onboardingScanRow{values: []any{
			uuid.New(), s.projectID, json.RawMessage(`["https://unipost.dev/"]`), profile,
			int32(1), true, pgtype.Timestamptz{}, pgtype.Timestamptz{},
		}}
	case strings.Contains(query, "update product_profiles set profile"):
		s.updatedProfile = args[1].(json.RawMessage)
		return onboardingScanRow{values: []any{
			args[0].(uuid.UUID), s.projectID, args[2].(json.RawMessage), args[1].(json.RawMessage),
			int32(1), true, pgtype.Timestamptz{}, pgtype.Timestamptz{},
		}}
	default:
		return onboardingScanRow{err: pgx.ErrNoRows}
	}
}

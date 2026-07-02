package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestStartProjectOnboardingRunsAsDetachedBackgroundTask(t *testing.T) {
	projectID := uuid.New()
	called := make(chan projectOnboardingInput, 1)
	release := make(chan struct{})
	srv := &Server{
		OnboardingRunner: func(ctx context.Context, in projectOnboardingInput) {
			if err := ctx.Err(); err != nil {
				t.Errorf("background context should be detached from the request: %v", err)
			}
			called <- in
			<-release
		},
	}

	started := time.Now()
	srv.startProjectOnboarding(projectID, "https://unipost.dev")

	select {
	case got := <-called:
		if got.ProjectID != projectID {
			t.Fatalf("project id = %s, want %s", got.ProjectID, projectID)
		}
		if got.SiteURL != "https://unipost.dev" {
			t.Fatalf("site url = %q", got.SiteURL)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("background onboarding was not started")
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("startProjectOnboarding blocked for %s", elapsed)
	}
	close(release)
}

func TestStartProjectOnboardingSkipsEmptySiteURL(t *testing.T) {
	called := make(chan projectOnboardingInput, 1)
	srv := &Server{
		OnboardingRunner: func(ctx context.Context, in projectOnboardingInput) {
			called <- in
		},
	}

	srv.startProjectOnboarding(uuid.New(), " ")

	select {
	case <-called:
		t.Fatal("background onboarding should not start without a site URL")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCreateProjectPersistsSiteURLAndStartsOnboarding(t *testing.T) {
	projectID := uuid.New()
	called := make(chan projectOnboardingInput, 1)
	store := &projectCreateDBSpy{projectID: projectID}
	srv := &Server{
		Q: db.New(store),
		OnboardingRunner: func(ctx context.Context, in projectOnboardingInput) {
			called <- in
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(`{"site_url":"https://unipost.dev"}`))
	rec := httptest.NewRecorder()
	srv.createProject(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var project db.Project
	if err := json.NewDecoder(rec.Body).Decode(&project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	cfg, err := config.Parse(project.Config)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.SiteURL != "https://unipost.dev/" {
		t.Fatalf("config site_url = %q", cfg.SiteURL)
	}
	if store.seoSiteURL != cfg.SiteURL {
		t.Fatalf("seo site_url = %q, want %q", store.seoSiteURL, cfg.SiteURL)
	}

	select {
	case got := <-called:
		if got.ProjectID != projectID {
			t.Fatalf("onboarding project id = %s, want %s", got.ProjectID, projectID)
		}
		if got.SiteURL != cfg.SiteURL {
			t.Fatalf("onboarding site url = %q, want %q", got.SiteURL, cfg.SiteURL)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("background onboarding was not started")
	}
}

func TestStartInsightInventoryCrawlRunsAsDetachedBackgroundTask(t *testing.T) {
	projectID := uuid.New()
	called := make(chan insightInventoryInput, 1)
	release := make(chan struct{})
	srv := &Server{
		InsightInventoryRunner: func(ctx context.Context, in insightInventoryInput) {
			if err := ctx.Err(); err != nil {
				t.Errorf("inventory context should be detached from the request: %v", err)
			}
			called <- in
			<-release
		},
	}
	cfg := config.CrawlConfig{MaxPages: 5}

	started := time.Now()
	srv.startInsightInventoryCrawl(projectID, "https://unipost.dev", cfg)

	select {
	case got := <-called:
		if got.ProjectID != projectID {
			t.Fatalf("project id = %s, want %s", got.ProjectID, projectID)
		}
		if got.LandingURL != "https://unipost.dev" {
			t.Fatalf("landing url = %q", got.LandingURL)
		}
		if got.Crawl.MaxPages != 5 {
			t.Fatalf("crawl max pages = %d, want 5", got.Crawl.MaxPages)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("background inventory crawl was not started")
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("startInsightInventoryCrawl blocked for %s", elapsed)
	}
	close(release)
}

func TestStartInsightInventoryCrawlUsesBoundedOnboardingCrawlConfig(t *testing.T) {
	projectID := uuid.New()
	called := make(chan insightInventoryInput, 1)
	srv := &Server{
		InsightInventoryRunner: func(ctx context.Context, in insightInventoryInput) {
			called <- in
		},
	}
	cfg := config.CrawlConfig{
		MaxPages:         200,
		SitemapURLCap:    2000,
		RequestTimeoutMs: 8000,
		RateLimitRPS:     1,
	}

	srv.startInsightInventoryCrawl(projectID, "https://unipost.dev", cfg)

	select {
	case got := <-called:
		if got.Crawl.MaxPages != 20 {
			t.Fatalf("crawl max pages = %d, want 20", got.Crawl.MaxPages)
		}
		if got.Crawl.SitemapURLCap != 80 {
			t.Fatalf("sitemap url cap = %d, want 80", got.Crawl.SitemapURLCap)
		}
		if got.Crawl.RequestTimeoutMs != 5000 {
			t.Fatalf("request timeout = %d, want 5000", got.Crawl.RequestTimeoutMs)
		}
		if got.Crawl.RateLimitRPS != 3 {
			t.Fatalf("rate limit rps = %d, want 3", got.Crawl.RateLimitRPS)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("background inventory crawl was not started")
	}
}

func TestStartInsightInventoryCrawlSkipsEmptyLandingURL(t *testing.T) {
	called := make(chan insightInventoryInput, 1)
	srv := &Server{
		InsightInventoryRunner: func(ctx context.Context, in insightInventoryInput) {
			called <- in
		},
	}

	srv.startInsightInventoryCrawl(uuid.New(), " ", config.CrawlConfig{})

	select {
	case <-called:
		t.Fatal("background inventory should not start without a landing URL")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRunProjectOnboardingStartsIndependentTracksBeforeQuickProfileCompletes(t *testing.T) {
	projectID := uuid.New()
	landing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>UniPost</title></head><body><main>UniPost schedules social posts across platforms.</main></body></html>`))
	}))
	defer landing.Close()

	profileLLM := &blockingOnboardingLLM{started: make(chan struct{}, 1), release: make(chan struct{})}
	inventoryStarted := make(chan insightInventoryInput, 1)
	seoStarted := make(chan projectOnboardingInput, 1)
	doctorStarted := make(chan projectOnboardingInput, 1)
	releaseInventory := make(chan struct{})
	releaseSEO := make(chan struct{})
	releaseDoctor := make(chan struct{})
	srv := &Server{
		Q:   db.New(&onboardingDBSpy{projectID: projectID}),
		LLM: profileLLM,
		InsightInventoryRunner: func(ctx context.Context, in insightInventoryInput) {
			inventoryStarted <- in
			<-releaseInventory
		},
		SEOOnboardingRunner: func(ctx context.Context, in projectOnboardingInput) {
			seoStarted <- in
			<-releaseSEO
		},
		DoctorOnboardingRunner: func(ctx context.Context, in projectOnboardingInput) {
			doctorStarted <- in
			<-releaseDoctor
		},
	}

	done := make(chan struct{})
	go func() {
		srv.runProjectOnboarding(context.Background(), projectOnboardingInput{ProjectID: projectID, SiteURL: landing.URL})
		close(done)
	}()
	defer func() {
		close(profileLLM.release)
		close(releaseInventory)
		close(releaseSEO)
		close(releaseDoctor)
		<-done
	}()

	select {
	case <-profileLLM.started:
	case <-time.After(3 * time.Second):
		t.Fatal("quick profile did not start")
	}

	select {
	case got := <-inventoryStarted:
		if got.ProjectID != projectID {
			t.Fatalf("inventory project id = %s, want %s", got.ProjectID, projectID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("inventory crawl was not started after quick profile")
	}

	select {
	case got := <-seoStarted:
		if got.ProjectID != projectID {
			t.Fatalf("seo project id = %s, want %s", got.ProjectID, projectID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("seo onboarding should start while quick profile is still running")
	}

	select {
	case got := <-doctorStarted:
		if got.ProjectID != projectID {
			t.Fatalf("doctor project id = %s, want %s", got.ProjectID, projectID)
		}
		if got.SiteURL != landing.URL {
			t.Fatalf("doctor site url = %q, want %q", got.SiteURL, landing.URL)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("seo doctor onboarding should start for a URL-created project")
	}
}

type blockingOnboardingLLM struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingOnboardingLLM) Complete(ctx context.Context, req llm.CompletionReq) (llm.CompletionResp, error) {
	p.once.Do(func() { p.started <- struct{}{} })
	select {
	case <-p.release:
	case <-ctx.Done():
		return llm.CompletionResp{}, ctx.Err()
	}
	return llm.CompletionResp{
		Text:  `{"positioning":"Provisional profile","value_props":["Fast"],"features":["Landing"],"icp":["Operators"],"tone":"clear","key_terms":["profile"],"competitors":[],"differentiators":["Quick context"]}`,
		Model: "blocking-onboarding",
	}, nil
}

type onboardingDBSpy struct {
	projectID uuid.UUID
}

func (s *onboardingDBSpy) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

type projectCreateDBSpy struct {
	projectID  uuid.UUID
	seoSiteURL string
}

func (s *projectCreateDBSpy) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (s *projectCreateDBSpy) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (s *projectCreateDBSpy) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	switch {
	case strings.Contains(query, "insert into projects"):
		return onboardingScanRow{values: []any{
			s.projectID, args[0].(string), args[1].(string), args[2].(string), args[3].(json.RawMessage), pgtype.Timestamptz{}, pgtype.Timestamptz{},
		}}
	case strings.Contains(query, "insert into seo_properties"):
		s.seoSiteURL = args[1].(string)
		return onboardingScanRow{values: []any{
			uuid.New(), args[0].(uuid.UUID), args[1].(string), args[2].(*string), args[3].(*string), args[4].(json.RawMessage),
			args[5].(*string), args[6].(*string), pgtype.Timestamptz{}, pgtype.Timestamptz{},
		}}
	case strings.Contains(query, "insert into seo_integrations"):
		return onboardingScanRow{values: []any{
			uuid.New(), args[0].(uuid.UUID), args[1].(string), args[2].(string), args[3].(*string), args[4].(pgtype.Timestamptz),
			args[5].(*string), pgtype.Timestamptz{}, pgtype.Timestamptz{},
		}}
	default:
		return onboardingScanRow{err: pgx.ErrNoRows}
	}
}

func (s *onboardingDBSpy) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (s *onboardingDBSpy) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	switch {
	case strings.Contains(query, "select id, owner_id, name, slug, config, created_at, updated_at from projects"):
		return onboardingScanRow{values: []any{
			s.projectID, "owner", "UniPost", "unipost", config.Default().JSON(), pgtype.Timestamptz{}, pgtype.Timestamptz{},
		}}
	case strings.Contains(query, "insert into generation_runs"):
		return onboardingScanRow{values: []any{
			uuid.New(), args[0].(uuid.UUID), args[1].(string), args[2].([]byte), args[3].([]byte),
			args[4].(*string), args[5].(*int32), args[6].(pgtype.Numeric), args[7].(string), args[8].(*string), pgtype.Timestamptz{},
		}}
	case strings.Contains(query, "select coalesce(max(version)"):
		return onboardingScanRow{values: []any{int32(1)}}
	case strings.Contains(query, "insert into product_profiles"):
		return onboardingScanRow{values: []any{
			uuid.New(), args[0].(uuid.UUID), args[1].(json.RawMessage), args[2].(json.RawMessage),
			args[3].(int32), true, pgtype.Timestamptz{}, pgtype.Timestamptz{},
		}}
	default:
		return onboardingScanRow{err: pgx.ErrNoRows}
	}
}

type onboardingScanRow struct {
	values []any
	err    error
}

func (r onboardingScanRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		switch d := dest[i].(type) {
		case *uuid.UUID:
			*d = r.values[i].(uuid.UUID)
		case *string:
			*d = r.values[i].(string)
		case *[]byte:
			*d = r.values[i].([]byte)
		case **string:
			*d = r.values[i].(*string)
		case **int32:
			*d = r.values[i].(*int32)
		case *json.RawMessage:
			*d = r.values[i].(json.RawMessage)
		case *pgtype.Numeric:
			*d = r.values[i].(pgtype.Numeric)
		case *pgtype.Timestamptz:
			*d = r.values[i].(pgtype.Timestamptz)
		case *int32:
			*d = r.values[i].(int32)
		case *bool:
			*d = r.values[i].(bool)
		default:
			return pgx.ErrNoRows
		}
	}
	return nil
}

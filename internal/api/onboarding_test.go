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

func TestRunProjectOnboardingStartsInventoryBeforeSEOCompletes(t *testing.T) {
	projectID := uuid.New()
	landing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>UniPost</title></head><body><main>UniPost schedules social posts across platforms.</main></body></html>`))
	}))
	defer landing.Close()

	inventoryStarted := make(chan insightInventoryInput, 1)
	seoStarted := make(chan projectOnboardingInput, 1)
	releaseInventory := make(chan struct{})
	releaseSEO := make(chan struct{})
	srv := &Server{
		Q:   db.New(&onboardingDBSpy{projectID: projectID}),
		LLM: llm.NewMock(),
		InsightInventoryRunner: func(ctx context.Context, in insightInventoryInput) {
			inventoryStarted <- in
			<-releaseInventory
		},
		SEOOnboardingRunner: func(ctx context.Context, in projectOnboardingInput) {
			seoStarted <- in
			<-releaseSEO
		},
	}

	done := make(chan struct{})
	go func() {
		srv.runProjectOnboarding(context.Background(), projectOnboardingInput{ProjectID: projectID, SiteURL: landing.URL})
		close(done)
	}()
	defer func() {
		close(releaseInventory)
		close(releaseSEO)
		<-done
	}()

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
		t.Fatal("seo onboarding should start while inventory crawl is still running")
	}
}

type onboardingDBSpy struct {
	projectID uuid.UUID
}

func (s *onboardingDBSpy) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (s *onboardingDBSpy) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (s *onboardingDBSpy) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	switch {
	case strings.Contains(query, "select id, owner_id, name, slug, config, created_at from projects"):
		return onboardingScanRow{values: []any{
			s.projectID, "owner", "UniPost", "unipost", config.Default().JSON(), pgtype.Timestamptz{},
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

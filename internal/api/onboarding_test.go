package api

import (
	"context"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/google/uuid"
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

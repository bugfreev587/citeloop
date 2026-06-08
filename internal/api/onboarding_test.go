package api

import (
	"context"
	"testing"
	"time"

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

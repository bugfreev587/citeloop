package seo

import (
	"context"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
)

func TestProviderCanAttemptAfterRecoverableStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{status: "connected", want: true},
		{status: "backfilling", want: true},
		{status: "stale", want: true},
		{status: "error", want: true},
		{status: "expired", want: true},
		{status: "missing", want: false},
		{status: "property_selection_required", want: false},
		{status: "mismatch", want: false},
		{status: "revoked", want: false},
	}
	for _, tc := range cases {
		got := isProviderAttemptable([]db.SeoIntegration{{
			Provider: ProviderGSC,
			Status:   tc.status,
		}}, ProviderGSC)
		if got != tc.want {
			t.Fatalf("status %q attemptable = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestFinishRunContextSurvivesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	finishCtx, finishCancel := finishRunContext(ctx)
	defer finishCancel()

	if err := finishCtx.Err(); err != nil {
		t.Fatalf("finish context err = %v, want nil", err)
	}
}

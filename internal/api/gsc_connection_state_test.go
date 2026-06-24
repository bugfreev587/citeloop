package api

import (
	"testing"
	"time"
)

func TestDeriveGSCConnectionStatus(t *testing.T) {
	selected := "sc-domain:unipost.dev"
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	properties := []gscPropertyResponse{{SiteURL: selected}}

	tests := []struct {
		name string
		in   gscStatusInput
		want string
	}{
		{
			name: "needs property selection before data sync",
			in:   gscStatusInput{IntegrationStatus: "connected", Properties: properties},
			want: "property_selection_required",
		},
		{
			name: "selected property without data is backfilling",
			in:   gscStatusInput{IntegrationStatus: "connected", SelectedProperty: &selected, Properties: properties, DataDayCount: 0, HasDataDayCount: true, Now: now},
			want: "backfilling",
		},
		{
			name: "selected property with data is connected",
			in:   gscStatusInput{IntegrationStatus: "backfilling", SelectedProperty: &selected, Properties: properties, DataDayCount: 14, HasDataDayCount: true, Now: now},
			want: "connected",
		},
		{
			name: "old verification is stale",
			in: gscStatusInput{
				IntegrationStatus: "connected",
				SelectedProperty:  &selected,
				Properties:        properties,
				DataDayCount:      14,
				HasDataDayCount:   true,
				LastVerifiedAt:    now.Add(-73 * time.Hour),
				Now:               now,
			},
			want: "stale",
		},
		{
			name: "selected unauthorized property is mismatch",
			in: gscStatusInput{
				IntegrationStatus: "connected",
				SelectedProperty:  &selected,
				Properties:        []gscPropertyResponse{{SiteURL: "https://other.example/"}},
				DataDayCount:      14,
				HasDataDayCount:   true,
				Now:               now,
			},
			want: "mismatch",
		},
		{
			name: "revoked integration wins",
			in:   gscStatusInput{IntegrationStatus: "revoked", SelectedProperty: &selected, Properties: properties, DataDayCount: 14, HasDataDayCount: true, Now: now},
			want: "revoked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveGSCConnectionStatus(tt.in); got != tt.want {
				t.Fatalf("status = %q, want %q", got, tt.want)
			}
		})
	}
}

package seo

import (
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/google/uuid"
)

func TestSetupChecklistMarksPublisherWriteFromConnectionHealth(t *testing.T) {
	projectID := uuid.New()
	checklist, mode, ready := buildSetupChecklist(setupChecklistInput{
		Integrations: []db.SeoIntegration{
			{ProjectID: projectID, Provider: ProviderGSC, Status: "connected"},
		},
		PublisherConnections: []db.PublisherConnection{
			{
				ProjectID:     projectID,
				Kind:          publisher.ConnectionKindGitHubNextJS,
				Status:        "connected",
				CredentialRef: ptr("publisher_credential:" + uuid.New().String()),
			},
		},
		ColdStart: false,
	})

	if mode != "customer_site_connected" {
		t.Fatalf("mode = %q, want customer_site_connected", mode)
	}
	if !ready {
		t.Fatal("expected handoff to autopilot to be ready when search data and publisher are connected")
	}
	publisherItem := findChecklistItem(checklist, "publisher_write")
	if publisherItem == nil {
		t.Fatal("publisher_write item missing")
	}
	if publisherItem.Status != "connected" {
		t.Fatalf("publisher_write status = %q, want connected", publisherItem.Status)
	}
}

func TestSetupChecklistShowsPendingPublisherWhenCredentialMissing(t *testing.T) {
	checklist, mode, ready := buildSetupChecklist(setupChecklistInput{
		Integrations: []db.SeoIntegration{
			{Provider: ProviderGSC, Status: "connected"},
		},
		PublisherConnections: []db.PublisherConnection{
			{
				Kind:   publisher.ConnectionKindGitHubNextJS,
				Status: "missing",
			},
		},
		ColdStart: false,
	})

	if mode != "customer_site_pending_verification" {
		t.Fatalf("mode = %q, want customer_site_pending_verification", mode)
	}
	if ready {
		t.Fatal("expected handoff to autopilot to stay blocked without publisher credential")
	}
	publisherItem := findChecklistItem(checklist, "publisher_write")
	if publisherItem == nil {
		t.Fatal("publisher_write item missing")
	}
	if publisherItem.Status != "in_progress" {
		t.Fatalf("publisher_write status = %q, want in_progress", publisherItem.Status)
	}
}

func TestSetupChecklistKeepsPublicOnlyWhenSearchDataMissing(t *testing.T) {
	checklist, mode, ready := buildSetupChecklist(setupChecklistInput{
		PublisherConnections: []db.PublisherConnection{
			{
				Kind:          publisher.ConnectionKindGitHubNextJS,
				Status:        "connected",
				CredentialRef: ptr("env:GITHUB_TOKEN"),
			},
		},
		ColdStart: false,
	})

	if mode != "public_only" {
		t.Fatalf("mode = %q, want public_only", mode)
	}
	if ready {
		t.Fatal("expected handoff to autopilot to stay blocked without search data")
	}
	searchItem := findChecklistItem(checklist, "search_data")
	if searchItem == nil {
		t.Fatal("search_data item missing")
	}
	if searchItem.Status != "blocked" {
		t.Fatalf("search_data status = %q, want blocked", searchItem.Status)
	}
}

func findChecklistItem(items []SetupChecklistItem, key string) *SetupChecklistItem {
	for i := range items {
		if items[i].Key == key {
			return &items[i]
		}
	}
	return nil
}

func ptr(value string) *string {
	return &value
}

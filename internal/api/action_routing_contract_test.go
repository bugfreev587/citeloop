package api

import (
	"os"
	"strings"
	"testing"
)

func TestCreateSEOContentActionInfersMultiSurfaceAssetAndReviewOutput(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"inferContentActionAssetType",
		"defaultReviewRequiredForAssetType",
		"defaultOutputSnapshotForAction",
		"defaultDiffSnapshotForAction",
		"metadata_rewrite",
		"internal_link_patch",
		"schema_patch",
		"technical_fix",
		"direct_patch",
		"technical_task",
		"seo_geo_contribution",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("createSEOContentAction routing contract missing %q", want)
		}
	}
}

func TestPlanSEOContentActionCreatesTopicForManualDrafting(t *testing.T) {
	serverRaw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	if !strings.Contains(string(serverRaw), `r.Post("/actions/{actionID}/plan", s.planSEOContentAction)`) {
		t.Fatal("manual Content Plan drafting must expose a content action planning endpoint")
	}

	handlerRaw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(handlerRaw)
	for _, want := range []string{
		"func (s *Server) planSEOContentAction",
		"GetContentAction",
		"contentActionNeedsTopic",
		"CreateTopic",
		"SourceContentActionID",
		`Status:                string(topicstate.StatusBacklog)`,
		`Status:    "approved"`,
		"EnqueueWorkflowEvent",
		"workflow.EventContentPlanCreated",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("manual content action planning missing %q", want)
		}
	}
}

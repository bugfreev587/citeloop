package seo

import "testing"

func TestDoctorHealthScoreCapsActiveP0At69(t *testing.T) {
	findings := []doctorFindingCandidate{{
		IssueType:            "soft_404",
		Severity:             "P0",
		Status:               "active",
		Confidence:           90,
		ImportanceMultiplier: 1,
	}}

	score := doctorHealthScore(findings)

	if score > 69 {
		t.Fatalf("score = %d, want active P0 cap <= 69", score)
	}
	if status := doctorDisplayStatus(score, findings); status != "blocked" {
		t.Fatalf("display status = %q, want blocked", status)
	}
}

func TestDoctorHealthScoreCapsActiveP1At84(t *testing.T) {
	findings := []doctorFindingCandidate{{
		IssueType:            "structured_data_invalid",
		Severity:             "P1",
		Status:               "active",
		Confidence:           90,
		ImportanceMultiplier: 1,
	}}

	score := doctorHealthScore(findings)

	if score > 84 {
		t.Fatalf("score = %d, want active P1 cap <= 84", score)
	}
	if status := doctorDisplayStatus(score, findings); status != "needs_attention" {
		t.Fatalf("display status = %q, want needs_attention", status)
	}
}

func TestDoctorHumanReportIssueCountExcludesInfo(t *testing.T) {
	findings := []doctorFindingCandidate{
		{Severity: "P0", Status: "active"},
		{Severity: "P1", Status: "active"},
		{Severity: "P1", Status: "active"},
		{Severity: "P1", Status: "active"},
		{Severity: "P2", Status: "active"},
		{Severity: "P2", Status: "active"},
		{Severity: "Info", Status: "active"},
		{Severity: "Info", Status: "active"},
		{Severity: "Info", Status: "active"},
		{Severity: "Info", Status: "active"},
	}

	report := buildDoctorHumanReport(58, findings, 24)

	if report.Summary != "6 issues found across 24 checked URLs" {
		t.Fatalf("summary = %q", report.Summary)
	}
	if report.IssueCounts["Info"] != 4 {
		t.Fatalf("Info count = %d, want 4", report.IssueCounts["Info"])
	}
}

func TestDoctorProgressInterpolatesWithinCheckingStage(t *testing.T) {
	progress := doctorProgressPercent(DoctorStageChecking, 6, 12)

	if progress != 62 {
		t.Fatalf("progress = %d, want 62", progress)
	}
}

func TestDoctorProgressSequenceExcludesHandoffForNewRuns(t *testing.T) {
	for _, stage := range doctorStageOrder {
		if stage == DoctorStageHandoff {
			t.Fatal("new Doctor run progress sequence must not enter handoff")
		}
	}
	if _, ok := doctorStageStarts[DoctorStageHandoff]; ok {
		t.Fatal("new Doctor run progress map must not expose handoff")
	}
	if progress := doctorProgressPercent(DoctorStageHandoff, 0, 0); progress != 0 {
		t.Fatalf("legacy handoff progress = %d, want 0 for new-run interpolation", progress)
	}
}

func TestDoctorSoft404HighConfidenceCanBeP0(t *testing.T) {
	candidate := classifySoft404(soft404Evidence{
		CanonicalHost:  true,
		GeneratedPath:  true,
		ExpectedStatus: 404,
		Probes: []soft404Probe{
			{URL: "https://example.com/__missing-a", StatusCode: 200, Similarity: 0.91},
			{URL: "https://example.com/__missing-b", StatusCode: 200, Similarity: 0.89},
		},
	})

	if candidate.IssueType != "soft_404" {
		t.Fatalf("issue type = %q, want soft_404", candidate.IssueType)
	}
	if candidate.Severity != "P0" {
		t.Fatalf("severity = %q, want P0", candidate.Severity)
	}
	if candidate.Confidence != 90 || candidate.ConfidenceLabel != "high" {
		t.Fatalf("confidence = %d/%q, want 90/high", candidate.Confidence, candidate.ConfidenceLabel)
	}
}

func TestDoctorSoft404MediumConfidenceDefaultsBelowP0(t *testing.T) {
	candidate := classifySoft404(soft404Evidence{
		CanonicalHost:  true,
		GeneratedPath:  true,
		ExpectedStatus: 404,
		Probes: []soft404Probe{
			{URL: "https://example.com/__missing-a", StatusCode: 200, Similarity: 0.78},
			{URL: "https://example.com/__missing-b", StatusCode: 404, Similarity: 0.20},
		},
	})

	if candidate.Severity == "P0" {
		t.Fatalf("medium confidence soft 404 should not be P0: %#v", candidate)
	}
	if candidate.Confidence != 70 || candidate.ConfidenceLabel != "medium" {
		t.Fatalf("confidence = %d/%q, want 70/medium", candidate.Confidence, candidate.ConfidenceLabel)
	}
}

func TestDoctorFindingKeyKeepsEvidenceVariantsOutOfHash(t *testing.T) {
	a := doctorFindingCandidate{
		IssueType:         "canonical_mismatch",
		ProjectID:         "project-1",
		NormalizedURLs:    []string{"/en"},
		StructuralLocator: "head link[rel=canonical]",
		Evidence: map[string]any{
			"observed_url": "https://www.example.com/en",
		},
	}
	b := a
	b.Evidence = map[string]any{
		"observed_url": "http://example.com/en/",
	}

	if doctorFindingKey(a) != doctorFindingKey(b) {
		t.Fatalf("evidence variants should not change finding key: %q != %q", doctorFindingKey(a), doctorFindingKey(b))
	}
}

package autopilot

import "testing"

func TestClassifyRiskLowTrafficMetadata(t *testing.T) {
	got := ClassifyRisk(RiskInput{
		ActionType:        "metadata rewrite",
		PageType:          "blog",
		DiffScope:         "metadata-only",
		Clicks28D:         2,
		Impressions28D:    120,
		TrafficPercentile: 20,
		Confidence:        95,
	}, RiskPolicy{})

	if got.Level != RiskLow {
		t.Fatalf("level = %s, want low; reasons=%v", got.Level, got.Reasons)
	}
	if !got.LowTraffic {
		t.Fatal("expected low traffic")
	}
	if got.ClassifierVersion != DefaultRiskClassifierVersion {
		t.Fatalf("version = %q", got.ClassifierVersion)
	}
}

func TestClassifyRiskCriticalPageIsHigh(t *testing.T) {
	got := ClassifyRisk(RiskInput{
		ActionType:        "metadata rewrite",
		PageType:          "pricing",
		DiffScope:         "metadata-only",
		Clicks28D:         0,
		Impressions28D:    0,
		TrafficPercentile: 0,
	}, RiskPolicy{})

	if got.Level != RiskHigh {
		t.Fatalf("level = %s, want high", got.Level)
	}
	if got.LowTraffic {
		t.Fatal("critical page must not be low traffic")
	}
}

func TestClassifyRiskMergeNoindexDeleteIsHigh(t *testing.T) {
	for _, action := range []string{"merge pages", "noindex/prune/delete"} {
		t.Run(action, func(t *testing.T) {
			got := ClassifyRisk(RiskInput{ActionType: action, PageType: "blog"}, RiskPolicy{})
			if got.Level != RiskHigh {
				t.Fatalf("level = %s, want high", got.Level)
			}
		})
	}
}

func TestClassifyRiskLowConfidenceIsMedium(t *testing.T) {
	got := ClassifyRisk(RiskInput{
		ActionType:        "metadata rewrite",
		PageType:          "blog",
		DiffScope:         "metadata-only",
		Clicks28D:         1,
		Impressions28D:    100,
		TrafficPercentile: 10,
		Confidence:        60,
	}, RiskPolicy{MinConfidenceForAutoPublish: 75})

	if got.Level != RiskMedium {
		t.Fatalf("level = %s, want medium; reasons=%v", got.Level, got.Reasons)
	}
}

func TestClassifyRiskUsesMultiSurfaceInputs(t *testing.T) {
	policy := RiskPolicy{}

	comparison := ClassifyRisk(RiskInput{
		ActionType:         "create comparison page",
		AssetType:          "comparison_page",
		PublicationSurface: "blog",
		Confidence:         90,
	}, policy)
	if comparison.Level != RiskMedium {
		t.Fatalf("comparison pages should be medium risk, got %s reasons=%v", comparison.Level, comparison.Reasons)
	}

	community := ClassifyRisk(RiskInput{
		ActionType:           "external distribution",
		AssetType:            "blog_post",
		PublicationSurface:   "external",
		DistributionPlatform: "reddit",
		Confidence:           90,
	}, policy)
	if community.Level != RiskHigh {
		t.Fatalf("community auto distribution should be high risk, got %s reasons=%v", community.Level, community.Reasons)
	}

	schemaPatch := ClassifyRisk(RiskInput{
		ActionType:   "schema patch",
		AssetType:    "schema_patch",
		SchemaChange: true,
		Confidence:   90,
	}, policy)
	if schemaPatch.Level != RiskMedium {
		t.Fatalf("schema patch should be medium risk, got %s reasons=%v", schemaPatch.Level, schemaPatch.Reasons)
	}
}

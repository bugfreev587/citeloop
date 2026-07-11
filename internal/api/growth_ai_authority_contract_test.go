package api

import (
	"os"
	"strings"
	"testing"
)

func TestAllAPIGrowthArbitrationPathsUseProjectScopedAuthority(t *testing.T) {
	checks := []struct {
		file      string
		required  []string
		forbidden string
	}{
		{"handlers_seo.go", []string{"growthComparatorForProject", "GrowthAITriggerManual"}, "seoService().EnsureGrowthOpportunityReserved"},
		{"handlers_geo.go", []string{"growthComparatorForProject", "geoServiceForProject"}, "discovery.NewLLMSemanticComparator"},
		{"handlers_geo_pr2.go", []string{"geoServiceForProject", "GrowthAITriggerManual"}, ""},
		{"handlers_admin_discovery_review.go", []string{"growthComparatorForProject", "GrowthAITriggerManual", "AllowsDoctorAI"}, ""},
	}
	for _, check := range checks {
		raw, err := os.ReadFile(check.file)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, required := range check.required {
			if !strings.Contains(source, required) {
				t.Errorf("%s missing %q", check.file, required)
			}
		}
		if check.forbidden != "" && strings.Contains(source, check.forbidden) {
			t.Errorf("%s retains unauthorized path %q", check.file, check.forbidden)
		}
	}
}

func TestSEOServiceCannotConstructGrowthComparatorFromGlobalProvider(t *testing.T) {
	raw, err := os.ReadFile("../seo/service.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if strings.Contains(source, "discovery.NewLLMSemanticComparator") || !strings.Contains(source, "GrowthComparator") {
		t.Fatal("SEO service still turns a shared provider into Growth execution authority")
	}
}

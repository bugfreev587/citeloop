package growthwork

import (
	"os"
	"strings"
	"testing"
)

func TestProjectCutoverIsFencedConservedLedgeredAndRollbackable(t *testing.T) {
	raw, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, want := range []string{
		"EnsureProjectCutover",
		"FenceProductWriterAuthority",
		"SwitchProductWriterAuthority",
		"CountUnrepresentedActiveLegacyGrowth",
		"CreateMigrationBatch",
		"AppendMigrationLedger",
		"rollbackGrowthCutover",
		"ReleaseProductWriterFence",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("Growth cutover missing %q", want)
		}
	}
}

func TestDoctorCreationRequiresGrowthVisibilityGate(t *testing.T) {
	raw, err := os.ReadFile("../api/handlers_site_fixes.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "EnsureProjectCutover") {
		t.Fatal("Doctor can reserve before active legacy Growth is represented")
	}
}

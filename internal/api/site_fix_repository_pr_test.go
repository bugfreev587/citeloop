package api

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/google/uuid"
)

func TestCanonicalSiteFixPRUsesFencedPreparedMultiFilePatch(t *testing.T) {
	raw, err := os.ReadFile("handlers_site_fixes.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "func (s *Server) openCanonicalSiteFixGitHubPR")
	end := strings.Index(source, "func (s *Server) failCanonicalSiteFixPRClaim")
	if start < 0 || end <= start {
		t.Fatal("canonical Site Fix PR function boundaries unavailable")
	}
	openFlow := source[start:end]
	for _, required := range []string{"ReapplyRepositoryPreparedPatch", "PreserveRepositoryActualDiffMetadata", "SaveCanonicalSiteFixPreparedPatch", "CreateFileUpdatesPR", "BaseCommitSHA", "PrClaimAuthorityFingerprint"} {
		if !strings.Contains(openFlow, required) {
			t.Fatalf("repository-grounded PR flow missing %q", required)
		}
	}
	if strings.Index(openFlow, "SaveCanonicalSiteFixPreparedPatch") > strings.Index(openFlow, "CreateFileUpdatesPR") {
		t.Fatal("prepared patch must be fenced and persisted before GitHub mutation")
	}
	for _, forbidden := range []string{"fallbackCanonicalSiteFixManualApply", "GitHubNextJSTargetForSiteURL", "CreatePageUpdatePR", "siteFixMetadataRewriteContent"} {
		if strings.Contains(openFlow, forbidden) {
			t.Fatalf("repository-grounded PR flow retained legacy fallback %q", forbidden)
		}
	}
}

func TestCanonicalSiteFixPRFailureAndBranchValuesAreControlled(t *testing.T) {
	raw := errors.New("credential lookup failed: secret-token-value")
	code := safeCanonicalSiteFixPRFailureCode(raw)
	if code == "" || strings.Contains(code, "credential") || strings.Contains(code, "secret-token-value") {
		t.Fatalf("unsafe failure code %q", code)
	}
	id := uuid.MustParse("12345678-90ab-cdef-1234-567890abcdef")
	if branch := siteFixRepositoryWorkingBranch(id); branch != "citeloop/doctor-site-fix-1234567890ab" {
		t.Fatalf("working branch=%q", branch)
	}
	if conflictCode := safeCanonicalSiteFixPRFailureCode(fmt.Errorf("atomic apply: %w", publisher.ErrSourceConflict)); conflictCode != "repository_source_conflict" {
		t.Fatalf("typed source conflict code=%q", conflictCode)
	}
}

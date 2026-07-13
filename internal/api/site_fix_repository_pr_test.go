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

func TestCanonicalSiteFixPRTargetChangeRepreparesBeforeAnyNewTargetGitHubRead(t *testing.T) {
	raw, err := os.ReadFile("handlers_site_fixes.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "func (s *Server) openCanonicalSiteFixGitHubPR")
	end := strings.Index(source, "func (s *Server) resetCanonicalSiteFixPRForReprepare")
	if start < 0 || end <= start {
		t.Fatal("canonical Site Fix PR function boundaries unavailable")
	}
	openFlow := source[start:end]
	claimAt := strings.Index(openFlow, "ClaimCanonicalSiteFixGitHubPR")
	targetResetAt := strings.Index(openFlow, `"repository_target_changed"`)
	clientAt := strings.Index(openFlow, "publisher.NewGitHubPRClient")
	readAt := strings.Index(openFlow, "ReadBlobBounded")
	if claimAt < 0 || targetResetAt < 0 || clientAt < 0 || readAt < 0 {
		t.Fatalf("target-change flow incomplete: claim=%d reset=%d client=%d read=%d", claimAt, targetResetAt, clientAt, readAt)
	}
	if !(claimAt < targetResetAt && targetResetAt < clientAt && targetResetAt < readAt) {
		t.Fatalf("target mismatch must be claimed and reset before touching the new GitHub target: claim=%d reset=%d client=%d read=%d", claimAt, targetResetAt, clientAt, readAt)
	}
	createAt := strings.Index(openFlow, "createCanonicalSiteFixPRWithLegacyReconciliation(ctx, client")
	sourceResetAt := strings.LastIndex(openFlow, `"repository_source_conflict"`)
	if createAt < 0 || sourceResetAt < createAt {
		t.Fatal("typed repository source conflict must request one fresh preparation after the PR attempt")
	}
}

func TestCanonicalSiteFixPRReloadsFixAfterAtomicPRObservation(t *testing.T) {
	raw, err := os.ReadFile("handlers_site_fixes.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "func (s *Server) openCanonicalSiteFixGitHubPR")
	end := strings.Index(source, "func (s *Server) resetCanonicalSiteFixPRForReprepare")
	openFlow := source[start:end]
	markAt := strings.Index(openFlow, "MarkCanonicalSiteFixGitHubPR")
	reloadAt := strings.LastIndex(openFlow, "GetCanonicalSiteFix")
	if markAt < 0 || reloadAt < markAt {
		t.Fatal("open/closed/merged PR observation must reload the exact canonical fix before returning")
	}
}

func TestCanonicalSiteFixPRFailureAndBranchValuesAreControlled(t *testing.T) {
	raw := errors.New("credential lookup failed: secret-token-value")
	code := safeCanonicalSiteFixPRFailureCode(raw)
	if code == "" || strings.Contains(code, "credential") || strings.Contains(code, "secret-token-value") {
		t.Fatalf("unsafe failure code %q", code)
	}
	fixID := uuid.MustParse("12345678-90ab-cdef-1234-567890abcdef")
	applicationID := uuid.MustParse("abcdef12-3456-7890-abcd-ef1234567890")
	if branch := siteFixRepositoryWorkingBranch(fixID, applicationID); branch != "citeloop/doctor-site-fix-1234567890ab-abcdef123456" {
		t.Fatalf("working branch=%q", branch)
	}
	if siteFixRepositoryWorkingBranch(fixID, uuid.New()) == siteFixRepositoryWorkingBranch(fixID, applicationID) {
		t.Fatal("fresh application reused the previous deterministic working branch")
	}
	if branch := legacySiteFixRepositoryWorkingBranch(fixID); branch != "citeloop/doctor-site-fix-1234567890ab" {
		t.Fatalf("legacy working branch=%q", branch)
	}
	if conflictCode := safeCanonicalSiteFixPRFailureCode(fmt.Errorf("atomic apply: %w", publisher.ErrSourceConflict)); conflictCode != "repository_source_conflict" {
		t.Fatalf("typed source conflict code=%q", conflictCode)
	}
}

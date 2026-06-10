package agents

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnforceBannedClaimsBlocksLiteralMatch(t *testing.T) {
	out := &QAOutput{GeoScore: 0.9, SeoScore: 0.9}
	profile := json.RawMessage(`{"banned_claims":["SOC 2 certified","HIPAA compliant"]}`)

	enforceBannedClaims(out, profile, "UniPost is SOC 2 Certified and ships fast.")

	if !out.QABlocking {
		t.Fatal("a draft containing a banned claim must be blocked")
	}
	if !out.CanAutoFix {
		t.Fatal("banned-claim blocks should be auto-fixable so the editor loop can strip them")
	}
	if !strings.Contains(strings.ToLower(out.BlockingReason), "soc 2 certified") {
		t.Fatalf("blocking reason should name the banned claim, got %q", out.BlockingReason)
	}
	joined := strings.ToLower(strings.Join(out.Issues, " "))
	if !strings.Contains(joined, "banned claim present") {
		t.Fatalf("issues should record the banned claim, got %#v", out.Issues)
	}
}

func TestEnforceBannedClaimsAllowsCleanContent(t *testing.T) {
	out := &QAOutput{GeoScore: 0.9, SeoScore: 0.9}
	profile := json.RawMessage(`{"banned_claims":["SOC 2 certified"]}`)

	enforceBannedClaims(out, profile, "UniPost helps teams schedule posts across platforms.")

	if out.QABlocking {
		t.Fatal("clean content must not be blocked by the banned-claims gate")
	}
}

func TestEnforceBannedClaimsIgnoresEmptyAndMissing(t *testing.T) {
	out := &QAOutput{}
	// No banned_claims key at all.
	enforceBannedClaims(out, json.RawMessage(`{"positioning":"scheduler"}`), "anything")
	// Empty/whitespace entries are skipped.
	enforceBannedClaims(out, json.RawMessage(`{"banned_claims":["  ",""]}`), "anything")
	if out.QABlocking {
		t.Fatal("missing or empty banned_claims must not block")
	}
}

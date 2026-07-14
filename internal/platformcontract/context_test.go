package platformcontract

import (
	"testing"
	"time"
)

func TestPrepareTargetContextNormalizesAndVersionsRedditRules(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	prepared, err := PrepareTargetContext(ConfirmTargetContextInput{
		Platform: "Reddit", TargetKey: "https://reddit.com/r/SaaS/",
		SourceURL:        "https://www.reddit.com/r/SaaS/about/rules",
		RulesText:        "1. No spam\n2. Add the right flair",
		AllowedPostTypes: []string{"text", "link", "text"}, RequiredFlair: "Discussion",
		LinkPolicy: "source links allowed", SelfPromotionPolicy: "disclose affiliation",
		Verified: true,
	}, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Platform != "reddit" || prepared.TargetKey != "r/saas" || prepared.Version != 3 {
		t.Fatalf("identity = %+v", prepared)
	}
	if prepared.SourceKind != "user_pasted_rules" || prepared.ContentHash == "" {
		t.Fatalf("source/hash = %+v", prepared)
	}
	if !prepared.ExpiresAt.Equal(now.Add(30 * 24 * time.Hour)) {
		t.Fatalf("expires_at = %s", prepared.ExpiresAt)
	}
	if len(prepared.AllowedPostTypes) != 2 || prepared.AllowedPostTypes[0] != "community_post" || prepared.AllowedPostTypes[1] != "link_submission" {
		t.Fatalf("post types = %#v", prepared.AllowedPostTypes)
	}
}

func TestPrepareTargetContextRequiresExplicitVerificationAndCompleteRules(t *testing.T) {
	tests := []ConfirmTargetContextInput{
		{Platform: "reddit", TargetKey: "r/saas", RulesText: "No spam", AllowedPostTypes: []string{"text"}, LinkPolicy: "allowed", SelfPromotionPolicy: "disclose"},
		{Platform: "reddit", TargetKey: "r/saas", Verified: true, AllowedPostTypes: []string{"text"}, LinkPolicy: "allowed", SelfPromotionPolicy: "disclose"},
		{Platform: "reddit", TargetKey: "r/saas", Verified: true, RulesText: "No spam", LinkPolicy: "allowed", SelfPromotionPolicy: "disclose"},
		{Platform: "reddit", TargetKey: "r/saas", Verified: true, RulesText: "No spam", AllowedPostTypes: []string{"text"}, SelfPromotionPolicy: "disclose"},
	}
	for i, input := range tests {
		if _, err := PrepareTargetContext(input, 0, time.Now()); err == nil {
			t.Errorf("case %d expected error", i)
		}
	}
}

func TestTargetContextCurrentRequiresConfirmedUnexpiredRevision(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	if !TargetContextCurrent("confirmed", now.Add(time.Minute), now) {
		t.Fatal("confirmed future revision should be current")
	}
	if TargetContextCurrent("draft", now.Add(time.Minute), now) {
		t.Fatal("draft revision must not be current")
	}
	if TargetContextCurrent("confirmed", now, now) {
		t.Fatal("expired-at-now revision must not be current")
	}
}

func TestPrepareTargetContextSupportsConfirmedHashnodePublication(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	prepared, err := PrepareTargetContext(ConfirmTargetContextInput{
		Platform: "hashnode", TargetKey: "citeloop", SourceURL: "https://citeloop.hashnode.dev", Verified: true,
	}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Platform != "hashnode" || prepared.TargetKey != "citeloop" || prepared.SourceKind != "user_confirmed_rules" || len(prepared.AllowedPostTypes) != 1 {
		t.Fatalf("prepared hashnode context = %+v", prepared)
	}
}

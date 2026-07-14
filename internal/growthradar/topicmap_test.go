package growthradar

import "testing"

func TestTopicMapIdentitySeparatesIntentAudienceAndTarget(t *testing.T) {
	base := TopicIdentityInput{ProjectID: "p1", Cluster: "AI Visibility", Intent: "comparison", Audience: "SaaS", AssetType: "comparison_page", CanonicalTarget: "blog"}
	first := DedupeIdentity(base)
	if first == "" || first != DedupeIdentity(base) {
		t.Fatal("identity must be stable")
	}
	changed := base
	changed.Intent = "how_to"
	if first == DedupeIdentity(changed) {
		t.Fatal("different intent must not merge")
	}
	changed = base
	changed.CanonicalTarget = "docs"
	if first == DedupeIdentity(changed) {
		t.Fatal("different canonical target must not merge")
	}
}

func TestNearDuplicateRequiresSameIdentityFamily(t *testing.T) {
	left := TopicIdentityInput{ProjectID: "p", Cluster: "content contracts", Intent: "comparison", Audience: "developers", AssetType: "comparison_page", CanonicalTarget: "blog"}
	right := left
	if !SameIdentityFamily(left, right) {
		t.Fatal("exact identity family should match")
	}
	right.Audience = "marketers"
	if SameIdentityFamily(left, right) {
		t.Fatal("distinct audience is arbitration, not near duplicate")
	}
}

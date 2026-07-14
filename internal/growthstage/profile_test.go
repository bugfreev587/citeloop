package growthstage

import "testing"

func TestProfilesHaveApprovedWeightsAndThresholds(t *testing.T) {
	tests := []struct {
		stage       Stage
		weights     Weights
		opportunity int
		watchlist   int
	}{
		{Foundation, Weights{Demand: 10, Coverage: 30, Relevance: 20, Commercial: 10, Freshness: 10, Reuse: 10, Evidence: 10}, 70, 60},
		{Traction, Weights{Demand: 25, Coverage: 20, Relevance: 15, Commercial: 15, Freshness: 10, Reuse: 10, Evidence: 5}, 75, 60},
		{Scale, Weights{Demand: 20, Coverage: 15, Relevance: 10, Commercial: 20, Freshness: 10, Reuse: 20, Evidence: 5}, 78, 65},
		{Optimize, Weights{Demand: 20, Coverage: 10, Relevance: 10, Commercial: 20, Freshness: 25, Reuse: 5, Evidence: 10}, 75, 60},
	}
	for _, test := range tests {
		profile, err := ProfileFor(test.stage)
		if err != nil {
			t.Fatalf("ProfileFor(%s): %v", test.stage, err)
		}
		if profile.Weights != test.weights || profile.OpportunityThreshold != test.opportunity || profile.WatchlistThreshold != test.watchlist {
			t.Errorf("profile %s = %+v", test.stage, profile)
		}
		if profile.Weights.Total() != 100 {
			t.Errorf("profile %s totals %d", test.stage, profile.Weights.Total())
		}
	}
}

func TestApplyUsesDeterministicIntegerFlooring(t *testing.T) {
	profile, _ := ProfileFor(Foundation)
	weighted := Apply(Raw{Demand: 5, Coverage: 16, Relevance: 15, Commercial: 12, Freshness: 5, Reuse: 6, Evidence: 4}, profile)
	if weighted.Demand != 2 || weighted.Coverage != 24 || weighted.Relevance != 20 || weighted.Commercial != 8 || weighted.Freshness != 5 || weighted.Reuse != 6 || weighted.Evidence != 8 || weighted.Total() != 73 {
		t.Fatalf("weighted = %+v", weighted)
	}
}

func TestDefaultAndInvalidStage(t *testing.T) {
	setting := DefaultSetting()
	if setting.Stage != Foundation || !setting.IsDefaultUnconfirmed || setting.SettingVersion != 0 {
		t.Fatalf("default = %+v", setting)
	}
	if _, err := ProfileFor(Stage("unknown")); err == nil {
		t.Fatal("invalid stage accepted")
	}
}

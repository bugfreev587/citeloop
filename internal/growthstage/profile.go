package growthstage

import "fmt"

const ProfileVersion = "growth-stage-profile-v1"

type Stage string

const (
	Foundation Stage = "foundation"
	Traction   Stage = "traction"
	Scale      Stage = "scale"
	Optimize   Stage = "optimize"
)

type Weights struct {
	Demand     int `json:"demand"`
	Coverage   int `json:"coverage_gap"`
	Relevance  int `json:"relevance"`
	Commercial int `json:"commercial_value"`
	Freshness  int `json:"freshness"`
	Reuse      int `json:"reuse_potential"`
	Evidence   int `json:"evidence_quality"`
}

func (w Weights) Total() int {
	return w.Demand + w.Coverage + w.Relevance + w.Commercial + w.Freshness + w.Reuse + w.Evidence
}

type Profile struct {
	Stage                Stage   `json:"stage"`
	Version              string  `json:"version"`
	Description          string  `json:"description"`
	Weights              Weights `json:"weights"`
	OpportunityThreshold int     `json:"opportunity_threshold"`
	WatchlistThreshold   int     `json:"watchlist_threshold"`
}

type Setting struct {
	Stage                Stage  `json:"stage"`
	StageProfileVersion  string `json:"stage_profile_version"`
	SettingVersion       int64  `json:"setting_version"`
	IsDefaultUnconfirmed bool   `json:"is_default_unconfirmed"`
}

type Raw struct {
	Demand     int `json:"demand"`
	Coverage   int `json:"coverage_gap"`
	Relevance  int `json:"relevance"`
	Commercial int `json:"commercial_value"`
	Freshness  int `json:"freshness"`
	Reuse      int `json:"reuse_potential"`
	Evidence   int `json:"evidence_quality"`
}

type Weighted struct {
	Demand     int `json:"demand"`
	Coverage   int `json:"coverage_gap"`
	Relevance  int `json:"relevance"`
	Commercial int `json:"commercial_value"`
	Freshness  int `json:"freshness"`
	Reuse      int `json:"reuse_potential"`
	Evidence   int `json:"evidence_quality"`
}

func (w Weighted) Total() int {
	return w.Demand + w.Coverage + w.Relevance + w.Commercial + w.Freshness + w.Reuse + w.Evidence
}

func DefaultSetting() Setting {
	return Setting{Stage: Foundation, StageProfileVersion: ProfileVersion, SettingVersion: 0, IsDefaultUnconfirmed: true}
}

func AllProfiles() []Profile {
	stages := []Stage{Foundation, Traction, Scale, Optimize}
	result := make([]Profile, 0, len(stages))
	for _, stage := range stages {
		profile, _ := ProfileFor(stage)
		result = append(result, profile)
	}
	return result
}

func ProfileFor(stage Stage) (Profile, error) {
	profile := Profile{Stage: stage, Version: ProfileVersion}
	switch stage {
	case Foundation:
		profile.Description = "Build essential topic coverage and citable owned assets."
		profile.Weights = Weights{Demand: 10, Coverage: 30, Relevance: 20, Commercial: 10, Freshness: 10, Reuse: 10, Evidence: 10}
		profile.OpportunityThreshold, profile.WatchlistThreshold = 70, 60
	case Traction:
		profile.Description = "Act on emerging SEO and GEO demand."
		profile.Weights = Weights{Demand: 25, Coverage: 20, Relevance: 15, Commercial: 15, Freshness: 10, Reuse: 10, Evidence: 5}
		profile.OpportunityThreshold, profile.WatchlistThreshold = 75, 60
	case Scale:
		profile.Description = "Expand proven themes across high-value content and platforms."
		profile.Weights = Weights{Demand: 20, Coverage: 15, Relevance: 10, Commercial: 20, Freshness: 10, Reuse: 20, Evidence: 5}
		profile.OpportunityThreshold, profile.WatchlistThreshold = 78, 65
	case Optimize:
		profile.Description = "Refresh declining assets and respond to competitive change."
		profile.Weights = Weights{Demand: 20, Coverage: 10, Relevance: 10, Commercial: 20, Freshness: 25, Reuse: 5, Evidence: 10}
		profile.OpportunityThreshold, profile.WatchlistThreshold = 75, 60
	default:
		return Profile{}, fmt.Errorf("unsupported growth stage %q", stage)
	}
	return profile, nil
}

func Apply(raw Raw, profile Profile) Weighted {
	return Weighted{
		Demand:     scale(raw.Demand, 25, profile.Weights.Demand),
		Coverage:   scale(raw.Coverage, 20, profile.Weights.Coverage),
		Relevance:  scale(raw.Relevance, 15, profile.Weights.Relevance),
		Commercial: scale(raw.Commercial, 15, profile.Weights.Commercial),
		Freshness:  scale(raw.Freshness, 10, profile.Weights.Freshness),
		Reuse:      scale(raw.Reuse, 10, profile.Weights.Reuse),
		Evidence:   scale(raw.Evidence, 5, profile.Weights.Evidence),
	}
}

func scale(value, maximum, weight int) int {
	if value < 0 {
		value = 0
	}
	if value > maximum {
		value = maximum
	}
	return value * weight / maximum
}

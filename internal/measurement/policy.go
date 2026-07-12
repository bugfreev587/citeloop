package measurement

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type CheckpointRole string

const (
	RoleBaseline CheckpointRole = "baseline"
	RoleEarly    CheckpointRole = "early"
	RolePrimary  CheckpointRole = "primary"
	RoleFollowUp CheckpointRole = "follow_up"
)

type Policy struct {
	PolicyVersion                  string `json:"policy_version"`
	EarlySignalOffsetDays          int    `json:"early_signal_offset_days"`
	PrimaryCheckpointOffsetDays    int    `json:"primary_checkpoint_offset_days"`
	FollowUpOffsetsDays            []int  `json:"follow_up_offsets_days"`
	MaxFollowUpAttempts            int    `json:"max_follow_up_attempts"`
	MaxMeasuringDurationDays       int    `json:"max_measuring_duration_days"`
	TerminalizationGracePeriodDays int    `json:"terminalization_grace_period_days"`
}

type Checkpoint struct {
	Role    CheckpointRole `json:"role"`
	Day     int            `json:"day"`
	Attempt int            `json:"attempt"`
}

func Parse(raw json.RawMessage) (Policy, error) {
	var policy Policy
	if len(raw) == 0 || !json.Valid(raw) {
		return Policy{}, fmt.Errorf("measurement policy must be valid JSON")
	}
	if err := json.Unmarshal(raw, &policy); err != nil {
		return Policy{}, err
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func LegacyPolicy() Policy {
	return Policy{
		PolicyVersion: "legacy-measurement-v1", EarlySignalOffsetDays: 14,
		PrimaryCheckpointOffsetDays: 28, FollowUpOffsetsDays: []int{56, 90},
		MaxFollowUpAttempts: 2, MaxMeasuringDurationDays: 90,
		TerminalizationGracePeriodDays: 7,
	}
}

func (policy Policy) Validate() error {
	if strings.TrimSpace(policy.PolicyVersion) == "" {
		return fmt.Errorf("measurement policy version is required")
	}
	if policy.EarlySignalOffsetDays <= 0 || policy.PrimaryCheckpointOffsetDays <= policy.EarlySignalOffsetDays {
		return fmt.Errorf("measurement checkpoints must be positive and increasing")
	}
	if policy.MaxFollowUpAttempts < 0 || policy.MaxFollowUpAttempts > 4 || len(policy.FollowUpOffsetsDays) > policy.MaxFollowUpAttempts {
		return fmt.Errorf("measurement follow-up attempts must be bounded")
	}
	if policy.MaxMeasuringDurationDays < policy.PrimaryCheckpointOffsetDays || policy.MaxMeasuringDurationDays > 365 {
		return fmt.Errorf("measurement duration must include the primary checkpoint and be bounded")
	}
	previous := policy.PrimaryCheckpointOffsetDays
	for _, offset := range policy.FollowUpOffsetsDays {
		if offset <= previous || offset > policy.MaxMeasuringDurationDays {
			return fmt.Errorf("measurement follow-up offsets must be increasing and within the duration")
		}
		previous = offset
	}
	if policy.TerminalizationGracePeriodDays < 0 || policy.TerminalizationGracePeriodDays > 30 {
		return fmt.Errorf("measurement terminalization grace period must be bounded")
	}
	return nil
}

func (policy Policy) Checkpoints() []Checkpoint {
	checkpoints := []Checkpoint{
		{Role: RoleBaseline, Day: 0, Attempt: 1},
		{Role: RoleEarly, Day: policy.EarlySignalOffsetDays, Attempt: 1},
		{Role: RolePrimary, Day: policy.PrimaryCheckpointOffsetDays, Attempt: 1},
	}
	for index, offset := range policy.FollowUpOffsetsDays {
		checkpoints = append(checkpoints, Checkpoint{Role: RoleFollowUp, Day: offset, Attempt: index + 1})
	}
	return checkpoints
}

func (policy Policy) AbsoluteTerminalAt(start time.Time) time.Time {
	return start.UTC().AddDate(0, 0, policy.MaxMeasuringDurationDays+policy.TerminalizationGracePeriodDays)
}

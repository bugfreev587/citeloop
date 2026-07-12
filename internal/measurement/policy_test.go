package measurement

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPolicyProducesFiniteTypedCheckpoints(t *testing.T) {
	policy, err := Parse(json.RawMessage(`{
      "policy_version":"growth-measurement-v1",
      "early_signal_offset_days":28,
      "primary_checkpoint_offset_days":56,
      "follow_up_offsets_days":[70,84],
      "max_follow_up_attempts":2,
      "max_measuring_duration_days":84,
      "terminalization_grace_period_days":7
    }`))
	if err != nil {
		t.Fatal(err)
	}
	checkpoints := policy.Checkpoints()
	want := []Checkpoint{{Role: RoleBaseline, Day: 0, Attempt: 1}, {Role: RoleEarly, Day: 28, Attempt: 1}, {Role: RolePrimary, Day: 56, Attempt: 1}, {Role: RoleFollowUp, Day: 70, Attempt: 1}, {Role: RoleFollowUp, Day: 84, Attempt: 2}}
	if len(checkpoints) != len(want) {
		t.Fatalf("checkpoints = %#v", checkpoints)
	}
	for i := range want {
		if checkpoints[i] != want[i] {
			t.Fatalf("checkpoint[%d] = %#v, want %#v", i, checkpoints[i], want[i])
		}
	}
	start := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	if got := policy.AbsoluteTerminalAt(start); !got.Equal(start.AddDate(0, 0, 91)) {
		t.Fatalf("absolute terminal = %s", got)
	}
}

func TestPolicyRejectsUnboundedOrSlidingSchedules(t *testing.T) {
	tests := []json.RawMessage{
		json.RawMessage(`{"policy_version":"x","early_signal_offset_days":7,"primary_checkpoint_offset_days":28,"follow_up_offsets_days":[42],"max_follow_up_attempts":99,"max_measuring_duration_days":42,"terminalization_grace_period_days":7}`),
		json.RawMessage(`{"policy_version":"x","early_signal_offset_days":28,"primary_checkpoint_offset_days":14,"follow_up_offsets_days":[],"max_follow_up_attempts":0,"max_measuring_duration_days":14,"terminalization_grace_period_days":7}`),
		json.RawMessage(`{"policy_version":"x","early_signal_offset_days":7,"primary_checkpoint_offset_days":28,"follow_up_offsets_days":[60],"max_follow_up_attempts":1,"max_measuring_duration_days":56,"terminalization_grace_period_days":7}`),
	}
	for _, raw := range tests {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("invalid policy accepted: %s", raw)
		}
	}
}

func TestLegacyPolicyIsFinite(t *testing.T) {
	policy := LegacyPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	if policy.MaxMeasuringDurationDays > 90 || len(policy.FollowUpOffsetsDays) > policy.MaxFollowUpAttempts {
		t.Fatalf("legacy policy is not bounded: %#v", policy)
	}
}

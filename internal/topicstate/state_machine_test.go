package topicstate

import "testing"

func TestTransitionFollowsTopicLifecycle(t *testing.T) {
	tests := []struct {
		name  string
		from  Status
		event Event
		want  Status
	}{
		{name: "schedule backlog", from: StatusBacklog, event: EventSchedule, want: StatusScheduled},
		{name: "clear scheduled", from: StatusScheduled, event: EventClearSchedule, want: StatusBacklog},
		{name: "start generation from backlog", from: StatusBacklog, event: EventStartGeneration, want: StatusGenerating},
		{name: "start generation from scheduled", from: StatusScheduled, event: EventStartGeneration, want: StatusGenerating},
		{name: "mark drafted", from: StatusGenerating, event: EventMarkDrafted, want: StatusDrafted},
		{name: "reject drafted", from: StatusDrafted, event: EventRejectDraft, want: StatusBacklog},
		{name: "archive backlog", from: StatusBacklog, event: EventArchive, want: StatusArchived},
		{name: "archive drafted", from: StatusDrafted, event: EventArchive, want: StatusArchived},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Transition(tt.from, tt.event)
			if err != nil {
				t.Fatalf("Transition(%q, %q): %v", tt.from, tt.event, err)
			}
			if got != tt.want {
				t.Fatalf("Transition(%q, %q) = %q, want %q", tt.from, tt.event, got, tt.want)
			}
		})
	}
}

func TestTransitionRejectsInvalidTopicLifecycleJumps(t *testing.T) {
	tests := []struct {
		name  string
		from  Status
		event Event
	}{
		{name: "drafted cannot start generation", from: StatusDrafted, event: EventStartGeneration},
		{name: "done cannot be scheduled", from: StatusDone, event: EventSchedule},
		{name: "archived cannot be drafted", from: StatusArchived, event: EventMarkDrafted},
		{name: "generating cannot be archived", from: StatusGenerating, event: EventArchive},
		{name: "unknown status rejected", from: Status("unknown"), event: EventSchedule},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Transition(tt.from, tt.event); err == nil {
				t.Fatalf("Transition(%q, %q) succeeded, want error", tt.from, tt.event)
			}
		})
	}
}

func TestReconcileExistingDrafts(t *testing.T) {
	for _, from := range []Status{StatusBacklog, StatusScheduled, StatusGenerating} {
		t.Run(string(from), func(t *testing.T) {
			got, changed, err := ReconcileExistingDrafts(from)
			if err != nil {
				t.Fatalf("ReconcileExistingDrafts(%q): %v", from, err)
			}
			if !changed || got != StatusDrafted {
				t.Fatalf("ReconcileExistingDrafts(%q) = (%q, %v), want (drafted, true)", from, got, changed)
			}
		})
	}

	got, changed, err := ReconcileExistingDrafts(StatusDrafted)
	if err != nil {
		t.Fatalf("ReconcileExistingDrafts(drafted): %v", err)
	}
	if changed || got != StatusDrafted {
		t.Fatalf("ReconcileExistingDrafts(drafted) = (%q, %v), want (drafted, false)", got, changed)
	}
}

func TestGenerationFailureReturnsToPriorIntent(t *testing.T) {
	tests := []struct {
		name      string
		scheduled bool
		want      Status
	}{
		{name: "unscheduled topic", scheduled: false, want: StatusBacklog},
		{name: "scheduled topic", scheduled: true, want: StatusScheduled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerationFailureStatus(StatusGenerating, tt.scheduled)
			if err != nil {
				t.Fatalf("GenerationFailureStatus: %v", err)
			}
			if got != tt.want {
				t.Fatalf("GenerationFailureStatus = %q, want %q", got, tt.want)
			}
		})
	}
}

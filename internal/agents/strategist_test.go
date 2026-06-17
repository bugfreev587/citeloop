package agents

import (
	"encoding/json"
	"testing"
)

func TestStrategistTopicSpecAcceptsPriorityScoreAlias(t *testing.T) {
	var wrap struct {
		Topics []TopicSpec `json:"topics"`
	}

	err := extractJSON(`{"topics":[{"channel":"blog","title":"Demo","priority_score":80}]}`, &wrap)
	if err != nil {
		t.Fatalf("extractJSON: %v", err)
	}

	if got, want := wrap.Topics[0].Priority, 2; got != want {
		t.Fatalf("priority = %d, want %d", got, want)
	}
}

func TestStrategistTopicSpecPrefersPriorityScoreWhenPriorityNonPositive(t *testing.T) {
	var wrap struct {
		Topics []TopicSpec `json:"topics"`
	}

	err := extractJSON(`{"topics":[{"channel":"blog","title":"Demo","priority":0,"priority_score":80}]}`, &wrap)
	if err != nil {
		t.Fatalf("extractJSON: %v", err)
	}

	if got, want := wrap.Topics[0].Priority, 2; got != want {
		t.Fatalf("priority = %d, want %d (priority_score alias should win over non-positive priority)", got, want)
	}
}

func TestNormalizeTopicSpecsBackfillsMissingPriority(t *testing.T) {
	specs := normalizeTopicSpecs([]TopicSpec{
		{Title: "First", Priority: 0},
		{Title: "Second", Priority: -2},
		{Title: "Third", Priority: 7},
	})

	for i, spec := range specs {
		if spec.Priority <= 0 {
			t.Fatalf("spec %d priority = %d, want positive fallback", i, spec.Priority)
		}
	}
	if got, want := specs[2].Priority, 7; got != want {
		t.Fatalf("existing priority = %d, want %d", got, want)
	}
}

func TestTopicSpecUnmarshalAcceptsStringPriority(t *testing.T) {
	var spec TopicSpec
	if err := json.Unmarshal([]byte(`{"title":"Demo","priority":"high"}`), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got, want := spec.Priority, 1; got != want {
		t.Fatalf("priority = %d, want %d", got, want)
	}
}

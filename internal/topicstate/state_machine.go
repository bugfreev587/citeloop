package topicstate

import "fmt"

type Status string

const (
	StatusBacklog    Status = "backlog"
	StatusScheduled  Status = "scheduled"
	StatusGenerating Status = "generating"
	StatusDrafted    Status = "drafted"
	StatusDone       Status = "done"
	StatusArchived   Status = "archived"
)

type Event string

const (
	EventSchedule        Event = "schedule"
	EventClearSchedule   Event = "clear_schedule"
	EventStartGeneration Event = "start_generation"
	EventMarkDrafted     Event = "mark_drafted"
	EventRejectDraft     Event = "reject_draft"
	EventArchive         Event = "archive"
)

func Transition(from Status, event Event) (Status, error) {
	if !validStatus(from) {
		return "", fmt.Errorf("invalid topic status %q", from)
	}
	switch event {
	case EventSchedule:
		if from == StatusBacklog || from == StatusScheduled {
			return StatusScheduled, nil
		}
	case EventClearSchedule:
		if from == StatusBacklog || from == StatusScheduled {
			return StatusBacklog, nil
		}
	case EventStartGeneration:
		if from == StatusBacklog || from == StatusScheduled {
			return StatusGenerating, nil
		}
	case EventMarkDrafted:
		if from == StatusGenerating {
			return StatusDrafted, nil
		}
	case EventRejectDraft:
		if from == StatusDrafted || from == StatusGenerating || from == StatusBacklog || from == StatusScheduled {
			return StatusBacklog, nil
		}
	case EventArchive:
		if from == StatusBacklog || from == StatusScheduled || from == StatusDrafted || from == StatusDone {
			return StatusArchived, nil
		}
	default:
		return "", fmt.Errorf("invalid topic event %q", event)
	}
	return "", fmt.Errorf("invalid topic transition %q -> %q", from, event)
}

func ReconcileExistingDrafts(from Status) (Status, bool, error) {
	if !validStatus(from) {
		return "", false, fmt.Errorf("invalid topic status %q", from)
	}
	switch from {
	case StatusBacklog, StatusScheduled, StatusGenerating:
		return StatusDrafted, true, nil
	case StatusDrafted:
		return StatusDrafted, false, nil
	default:
		return from, false, nil
	}
}

func GenerationFailureStatus(from Status, scheduled bool) (Status, error) {
	if from != StatusGenerating {
		return "", fmt.Errorf("invalid topic transition %q -> generation_failure", from)
	}
	if scheduled {
		return StatusScheduled, nil
	}
	return StatusBacklog, nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusBacklog, StatusScheduled, StatusGenerating, StatusDrafted, StatusDone, StatusArchived:
		return true
	default:
		return false
	}
}

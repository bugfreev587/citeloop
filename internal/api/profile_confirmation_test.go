package api

import (
	"encoding/json"
	"testing"
)

func TestProfileHasContextConfirmationDetectsConfirmationTransitionPayload(t *testing.T) {
	if profileHasContextConfirmation(json.RawMessage(`{"positioning":"draft"}`)) {
		t.Fatal("unconfirmed profile should not trigger opportunity discovery")
	}
	if !profileHasContextConfirmation(json.RawMessage(`{"confirmed_at":"2026-06-12T00:00:00Z"}`)) {
		t.Fatal("confirmed_at should trigger opportunity discovery")
	}
	if !profileHasContextConfirmation(json.RawMessage(`{"context_confirmed_at":"2026-06-12T00:00:00Z"}`)) {
		t.Fatal("context_confirmed_at should trigger opportunity discovery")
	}
}

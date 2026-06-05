package db

import (
	"strings"
	"testing"
)

func TestInventoryUpdateCanEditEvidenceSnippets(t *testing.T) {
	if !strings.Contains(updateInventoryItem, "evidence_snippets = $6") {
		t.Fatal("UpdateInventoryItem must persist editable evidence_snippets")
	}
}

package api

import "testing"

func TestContractValidationBlocksOnlyExplicitFailures(t *testing.T) {
	if !contractValidationBlocks([]byte(`{"passed":false,"failures":[{"code":"canonical_required"}]}`)) {
		t.Fatal("explicit contract failure must block approval")
	}
	if contractValidationBlocks([]byte(`{"passed":true,"failures":[]}`)) {
		t.Fatal("passed contract must not block")
	}
	if contractValidationBlocks(nil) || contractValidationBlocks([]byte(`{}`)) {
		t.Fatal("legacy articles without a contract report must remain reviewable")
	}
}

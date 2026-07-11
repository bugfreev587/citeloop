package growthwork

import (
	"testing"

	"github.com/citeloop/citeloop/internal/discovery"
)

func TestClassifyDuplicateExecutionChain(t *testing.T) {
	tests := []struct {
		name     string
		owner    discovery.Owner
		actions  int64
		conflict int64
		want     executionChainDisposition
	}{
		{name: "no descendants", owner: discovery.OwnerDoctor, want: executionChainNone},
		{name: "safe growth repoint", owner: discovery.OwnerOpportunities, actions: 2, want: executionChainRepoint},
		{name: "doctor cannot own growth action chain", owner: discovery.OwnerDoctor, actions: 1, want: executionChainReview},
		{name: "canonical action conflict", owner: discovery.OwnerOpportunities, actions: 2, conflict: 1, want: executionChainReview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyDuplicateExecutionChain(tt.owner, tt.actions, tt.conflict); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

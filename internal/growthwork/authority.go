package growthwork

import (
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/citeloop/citeloop/internal/llm"
)

// ComparatorAuthority is the only place a shared provider becomes executable
// Growth arbitration authority. Callers must supply the project config and the
// exact trigger for the current operation.
type ComparatorAuthority struct {
	Provider llm.Provider
	Model    string
}

func (a ComparatorAuthority) ForConfig(projectConfig config.ProjectConfig, trigger config.GrowthAITrigger) discovery.SemanticComparator {
	if a.Provider == nil || !projectConfig.AllowsGrowthAI(trigger) {
		return nil
	}
	return discovery.NewLLMSemanticComparator(a.Provider, "tokengate", a.Model).WithPurpose(llm.PurposeDefault)
}

package discovery

import (
	"encoding/json"
	"fmt"
	"sort"
)

// DependencyClass is persisted for cross-line work that is intentionally
// distinct but shares a target/conflict surface. Exact duplicates never reach
// this classifier: they merge through the owner-neutral signature registry.
type DependencyClass string

const (
	DependencyHardBlocker DependencyClass = "hard_blocker"
	DependencySoft        DependencyClass = "soft_dependency"
)

type CrossLineDependency struct {
	BlockingWorkSignatureID   string
	Class                     DependencyClass
	Reason                    string
	OverlappingMutationFields []string
	ReassessmentTrigger       string
}

// ClassifyCrossLineDependency compares canonical signature inputs, not detector
// types or recommendation wording. A shared mutation field is a hard blocker;
// a shared target with disjoint mutation fields is an attribution confounder
// and therefore a soft dependency.
func ClassifyCrossLineDependency(candidate Candidate, active SnapshotWork) (CrossLineDependency, bool, error) {
	owner := ownerForCandidate(candidate)
	if active.Owner != OwnerDoctor && active.Owner != OwnerOpportunities {
		return CrossLineDependency{}, false, nil
	}
	if active.Owner == owner {
		return CrossLineDependency{}, false, nil
	}
	candidateIdentity, err := BuildIdentity(candidate)
	if err != nil {
		return CrossLineDependency{}, false, err
	}
	if candidateIdentity.ExactSignatureHash == active.ExactSignatureHash {
		return CrossLineDependency{}, false, nil
	}
	var existing signaturePayload
	if err := json.Unmarshal(active.SignaturePayload, &existing); err != nil {
		return CrossLineDependency{}, false, fmt.Errorf("decode active work signature: %w", err)
	}
	if existing.ProjectID == "" || existing.ChangeFamily == "" || len(existing.NormalizedTargetSet) == 0 || len(existing.NormalizedMutations) == 0 {
		return CrossLineDependency{}, false, fmt.Errorf("active work signature is incomplete")
	}
	var proposed signaturePayload
	if err := json.Unmarshal(candidateIdentity.SignaturePayload, &proposed); err != nil {
		return CrossLineDependency{}, false, fmt.Errorf("decode candidate work signature: %w", err)
	}
	if existing.ProjectID != proposed.ProjectID || !setsOverlap(existing.NormalizedTargetSet, proposed.NormalizedTargetSet) {
		return CrossLineDependency{}, false, nil
	}
	overlap := mutationFieldIntersection(existing.NormalizedMutations, proposed.NormalizedMutations)
	if len(overlap) > 0 {
		reason := "cross-line work mutates the same canonical field on an overlapping target"
		if active.Owner == OwnerDoctor && candidate.VerificationMode == VerificationDelayed &&
			containsString(overlap, "title") && normalizeToken(candidate.PrimarySuccessMetric) == "ctr" {
			reason = "Doctor title repair must verify first because a missing or invalid title makes the Growth CTR baseline unreliable"
		}
		return CrossLineDependency{
			BlockingWorkSignatureID: active.ID.String(), Class: DependencyHardBlocker,
			Reason:                    reason,
			OverlappingMutationFields: overlap, ReassessmentTrigger: "blocking_work_verified",
		}, true, nil
	}
	return CrossLineDependency{
		BlockingWorkSignatureID: active.ID.String(), Class: DependencySoft,
		Reason:                    "cross-line work shares a target but mutations do not overlap; preserve as an attribution confounder",
		OverlappingMutationFields: []string{}, ReassessmentTrigger: "attribution_reconcile",
	}, true, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func setsOverlap(left, right []string) bool {
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[normalizeTarget(value)] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[normalizeTarget(value)]; ok {
			return true
		}
	}
	return false
}

func mutationFieldIntersection(left, right []Mutation) []string {
	seen := make(map[string]struct{}, len(left))
	for _, mutation := range left {
		if field := normalizeToken(mutation.Field); field != "" {
			seen[field] = struct{}{}
		}
	}
	overlap := make(map[string]struct{})
	for _, mutation := range right {
		field := normalizeToken(mutation.Field)
		if _, ok := seen[field]; ok && field != "" {
			overlap[field] = struct{}{}
		}
	}
	out := make([]string, 0, len(overlap))
	for field := range overlap {
		out = append(out, field)
	}
	sort.Strings(out)
	return out
}

package scheduler

import (
	"os"
	"strings"
	"testing"
)

func TestMarkerLifecycleAppliedSharesVerificationTransaction(t *testing.T) {
	raw, err := os.ReadFile("sitefix_verify.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, bounds := range [][2]string{{"func (s *Scheduler) recordCanonicalVerificationPass", "func (s *Scheduler) recordCanonicalVerificationFailure"}, {"func (s *Scheduler) recordCanonicalVerificationFailure", "func canonicalRetryClassification"}} {
		start, end := strings.Index(source, bounds[0]), strings.Index(source, bounds[1])
		if start < 0 || end <= start {
			t.Fatalf("missing function bounds %v", bounds)
		}
		body := source[start:end]
		if !strings.Contains(body, "withCanonicalVerificationTx") || !strings.Contains(body, "markCanonicalAIReviewAppliedStrict") || !strings.Contains(body, "supersedeCanonicalAISiblingMarkers") {
			t.Fatalf("marker application is not inside verification transaction: %s", bounds[0])
		}
		applied := strings.Index(body, "markCanonicalAIReviewAppliedStrict")
		siblings := strings.Index(body, "supersedeCanonicalAISiblingMarkers")
		transition := strings.Index(body, "MarkCanonicalSiteFixVerified")
		if strings.Contains(bounds[0], "Failure") {
			transition = strings.Index(body, "MarkCanonicalSiteFixRetryable")
		}
		if applied < 0 || siblings < applied || transition < siblings {
			t.Fatalf("marker winner and siblings must be fenced before lifecycle transition: applied=%d siblings=%d transition=%d function=%s", applied, siblings, transition, bounds[0])
		}
	}
}

func TestAcceptedMarkerHasExplicitNonAIExitRejections(t *testing.T) {
	raw, err := os.ReadFile("sitefix_verify.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, reason := range []string{"production PageEvidence fetch failed", "deterministic acceptance evidence is sufficient", "verification lifecycle was not reopened"} {
		if !strings.Contains(source, reason) {
			t.Errorf("marker exit missing rejection %q", reason)
		}
	}
}

func TestPolicyDisabledExplicitlyRejectsActiveMarkers(t *testing.T) {
	raw, err := os.ReadFile("sitefix_verify.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"AllowsDoctorAI(config.DoctorAITriggerVerificationUser)",
		"Doctor AI policy is disabled",
		"rejectCanonicalAIMarkers",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("policy-off marker fencing missing %q", required)
		}
	}
}

func TestAcquireChecksCurrentPolicyBeforeReadingStoredConsumedResult(t *testing.T) {
	raw, err := os.ReadFile("sitefix_ai_trigger.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	policy := strings.Index(body, "AllowsDoctorAI(config.DoctorAITriggerVerificationUser)")
	reject := strings.Index(body, "rejectCanonicalAIMarkers")
	consumed := strings.Index(body, "GetDoctorAIOnDemandConsumedResult")
	if policy < 0 || reject < policy || consumed < reject {
		t.Fatalf("current policy must reject non-applied markers before consumed result read: policy=%d reject=%d consumed=%d", policy, reject, consumed)
	}
}

func TestLegacyConsumedMarkerReconcileRequiresReferencedVerification(t *testing.T) {
	raw, err := os.ReadFile("sitefix_verify.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"HasLifecycleReference",
		"RejectDoctorAIOnDemandConsumedWithoutLifecycleReference",
		"lifecycle_completed_without_this_ai_result",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("legacy marker reconciliation missing %q", required)
		}
	}
}

func TestRejectedRunningAICallsHaveBackgroundReconciler(t *testing.T) {
	raw, err := os.ReadFile("sitefix_verify.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{"ListRejectedDoctorAIRunningCalls", "FinishAICallRecordIfRunning"} {
		if !strings.Contains(source, required) {
			t.Errorf("rejected call reconciliation missing %q", required)
		}
	}
}

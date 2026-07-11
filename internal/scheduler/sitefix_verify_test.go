package scheduler

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
)

func TestCanonicalDoctorAcceptanceRequiresExecutableEvidence(t *testing.T) {
	fix := db.SiteFix{AcceptanceTests: json.RawMessage(`["title is live"]`)}
	fuzzy := db.SiteChangeApplication{ResolutionCriteria: json.RawMessage(`{"asset_type":"content_expansion"}`)}
	results, passed, executable := evaluateCanonicalAcceptanceTests(fix, fuzzy, canonicalPageEvidence{Body: "<html></html>", StatusCode: 200})
	if executable || passed || len(results) == 0 {
		t.Fatalf("fuzzy result executable=%v passed=%v results=%v", executable, passed, results)
	}

}

func TestCanonicalDoctorTypedAcceptanceExecutesDOMWithoutSubstringFalsePositive(t *testing.T) {
	fix := db.SiteFix{AcceptanceTests: json.RawMessage(`[
		{"type":"title_equals","expected":"New Title"},
		{"type":"meta_description_equals","expected":"New meta"},
		{"type":"canonical_equals","expected":"https://example.com/page"},
		{"type":"noindex_absent"},
		{"type":"json_ld_valid","expected_type":"Article"}
	]`)}
	app := db.SiteChangeApplication{TargetUrl: "https://example.com/page"}
	page := `<html><head><title>New Title</title>
	<meta name="description" content="New meta">
	<link rel="canonical" href="https://example.com/page">
	<script type="application/ld+json">{"@context":"https://schema.org","@type":"Article"}</script>
	</head><body>safe</body></html>`
	results, passed, executable := evaluateCanonicalAcceptanceTests(fix, app, canonicalPageEvidence{Body: page, StatusCode: 200})
	if !executable || !passed || len(results) != 5 {
		t.Fatalf("typed result executable=%v passed=%v results=%v", executable, passed, results)
	}

	// Expected text appearing in body/script is not DOM metadata evidence.
	trap := `<html><head><title>Old</title></head><body>New Title New meta https://example.com/page</body></html>`
	_, passed, executable = evaluateCanonicalAcceptanceTests(fix, app, canonicalPageEvidence{Body: trap, StatusCode: 200})
	if !executable || passed {
		t.Fatalf("substring trap executable=%v passed=%v", executable, passed)
	}
}

func TestCanonicalDoctorUnsupportedAcceptanceNeverPasses(t *testing.T) {
	fix := db.SiteFix{AcceptanceTests: json.RawMessage(`[{"type":"rank_improves"}]`)}
	_, passed, executable := evaluateCanonicalAcceptanceTests(fix, db.SiteChangeApplication{}, canonicalPageEvidence{Body: `<html></html>`, StatusCode: 200})
	if executable || passed {
		t.Fatalf("unsupported test executable=%v passed=%v", executable, passed)
	}
}

func TestCanonicalVerificationRetryClassificationMatchesSchema(t *testing.T) {
	for _, tc := range []struct {
		passed     bool
		retryCount int32
		maxRetries int32
		want       string
	}{
		{true, 0, 3, "not_applicable"},
		{false, 0, 3, "retryable"},
		{false, 2, 3, "retryable"},
		{false, 3, 3, "retry_exhausted"},
	} {
		if got := canonicalRetryClassification(tc.passed, tc.retryCount, tc.maxRetries); got != tc.want {
			t.Fatalf("classification(%v,%d,%d)=%q want %q", tc.passed, tc.retryCount, tc.maxRetries, got, tc.want)
		}
	}
}

func TestCanonicalVerifierRejectsCrossOriginAndUnsafeIPs(t *testing.T) {
	project, _ := url.Parse("https://example.com")
	for _, raw := range []string{"ftp://example.com/a", "https://evil.example/a", "http://127.0.0.1/a", "http://169.254.169.254/latest/meta-data", "http://10.0.0.1/a", "http://[::1]/a"} {
		target, _ := url.Parse(raw)
		if err := validateCanonicalVerificationURL(project, target); err == nil {
			t.Fatalf("expected %s rejected", raw)
		}
	}
	for _, rawIP := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "192.0.2.1", "198.18.0.1", "::1", "fc00::1", "fe80::1", "2001:db8::1"} {
		if safeCanonicalVerificationIP(net.ParseIP(rawIP)) {
			t.Fatalf("unsafe IP accepted: %s", rawIP)
		}
	}
}

func TestCanonicalVerifierAllowsHermeticInjectedFetcher(t *testing.T) {
	s := &Scheduler{siteFixVerifier: verifierStub{evidence: canonicalPageEvidence{Body: "<html>ok</html>", StatusCode: 200, Headers: http.Header{"X-Robots-Tag": {"index"}}}}}
	page, err := s.fetchCanonicalSiteFixPage(context.Background(), "https://example.com", "https://example.com/page")
	if err != nil || page.StatusCode != 200 || page.Body != "<html>ok</html>" || page.Headers.Get("X-Robots-Tag") != "index" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

type verifierStub struct {
	evidence canonicalPageEvidence
	err      error
}

func (v verifierStub) Fetch(context.Context, string, string) (canonicalPageEvidence, error) {
	return v.evidence, v.err
}

func TestCanonicalDoctorAcceptanceUsesHeadUniqueCanonicalAllRobotsAndStrictJSONLD(t *testing.T) {
	fix := db.SiteFix{AcceptanceTests: json.RawMessage(`[
		{"type":"canonical_equals","expected":"https://example.com:443/page#fragment"},
		{"type":"noindex_absent"},
		{"type":"json_ld_valid","expected_type":"Article"}
	]`)}
	page := canonicalPageEvidence{
		Body:       `<html><head><link rel="canonical" href="https://EXAMPLE.com/page"><script type="application/ld+json">{"@context":"https://schema.org/","@type":"Article"}</script></head><body><link rel="canonical" href="https://evil.example/"><meta name="robots" content="noindex"></body></html>`,
		StatusCode: http.StatusOK,
		Headers:    http.Header{"X-Robots-Tag": {"googlebot: index", "bingbot: follow"}},
	}
	_, passed, executable := evaluateCanonicalAcceptanceTests(fix, db.SiteChangeApplication{}, page)
	if !executable || !passed {
		t.Fatalf("valid head-scoped evidence executable=%v passed=%v", executable, passed)
	}

	for name, mutate := range map[string]func(*canonicalPageEvidence){
		"duplicate canonical": func(page *canonicalPageEvidence) {
			page.Body = strings.Replace(page.Body, `</head>`, `<link rel="canonical" href="https://example.com/page"></head>`, 1)
		},
		"relative canonical": func(page *canonicalPageEvidence) {
			page.Body = strings.Replace(page.Body, `https://EXAMPLE.com/page`, `/page`, 1)
		},
		"header noindex": func(page *canonicalPageEvidence) { page.Headers.Add("X-Robots-Tag", "noindex") },
		"robots none":    func(page *canonicalPageEvidence) { page.Headers.Add("X-Robots-Tag", "none") },
		"invalid jsonld": func(page *canonicalPageEvidence) {
			page.Body = strings.Replace(page.Body, `</head>`, `<script type="application/ld+json">{"@type":</script></head>`, 1)
		},
		"jsonld placeholder": func(page *canonicalPageEvidence) {
			page.Body = strings.Replace(page.Body, `"@type":"Article"`, `"@type":"Article","name":"{{TITLE}}"`, 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := canonicalPageEvidence{Body: page.Body, StatusCode: page.StatusCode, Headers: page.Headers.Clone()}
			mutate(&candidate)
			_, passed, executable := evaluateCanonicalAcceptanceTests(fix, db.SiteChangeApplication{}, candidate)
			if !executable || passed {
				t.Fatalf("unsafe evidence executable=%v passed=%v", executable, passed)
			}
		})
	}
}

func TestCanonicalDoctorAcceptanceRequiresHTTP200(t *testing.T) {
	fix := db.SiteFix{AcceptanceTests: json.RawMessage(`[{"type":"title_equals","expected":"Ready"}]`)}
	page := canonicalPageEvidence{Body: `<html><head><title>Ready</title></head></html>`, StatusCode: http.StatusNoContent}
	_, passed, executable := evaluateCanonicalAcceptanceTests(fix, db.SiteChangeApplication{}, page)
	if !executable || passed {
		t.Fatalf("non-200 response executable=%v passed=%v", executable, passed)
	}
}

func TestPageEmitsProposedMetadata(t *testing.T) {
	page := `<html><head><title>Unipost</title>
	<meta name="description" content="Unipost helps you publish &amp; manage posts across your channels."></head><body>hi</body></html>`

	if !pageEmitsProposedMetadata(page, "Unipost", "Unipost helps you publish & manage posts across your channels.") {
		t.Fatal("expected match when title and (entity-encoded) meta are present")
	}
	// Whitespace/case differences must not break the match.
	if !pageEmitsProposedMetadata(page, "  unipost  ", "") {
		t.Fatal("expected normalized title match")
	}
	// A proposed value not on the page fails.
	if pageEmitsProposedMetadata(page, "Different Title", "") {
		t.Fatal("expected no match for absent title")
	}
	// Both empty is never a trivial pass.
	if pageEmitsProposedMetadata(page, "", "") {
		t.Fatal("empty proposed values must not match")
	}
}

func TestSiteFixURLVerifiableAndProposedMetadata(t *testing.T) {
	app := db.SiteChangeApplication{
		ResolutionCriteria: json.RawMessage(`{"asset_type":"metadata_rewrite","target_url":"https://x.dev/"}`),
		PatchSnapshot:      json.RawMessage(`{"proposed_change":{"title":"New Title","meta_description":"New meta."}}`),
	}
	if !siteFixURLVerifiable(app) {
		t.Fatal("metadata_rewrite should be URL-verifiable")
	}
	title, meta := siteFixProposedMetadata(app)
	if title != "New Title" || meta != "New meta." {
		t.Fatalf("proposed metadata = %q / %q", title, meta)
	}

	fuzzy := db.SiteChangeApplication{ResolutionCriteria: json.RawMessage(`{"asset_type":"content_expansion"}`)}
	if siteFixURLVerifiable(fuzzy) {
		t.Fatal("content_expansion should not be URL-verifiable")
	}

	// Falls back to diff snapshot when patch snapshot lacks proposed_change.
	fallback := db.SiteChangeApplication{
		DiffSnapshot: json.RawMessage(`{"proposed_metadata":{"title":"T","meta_description":"M"}}`),
	}
	title, meta = siteFixProposedMetadata(fallback)
	if title != "T" || meta != "M" {
		t.Fatalf("fallback metadata = %q / %q", title, meta)
	}
}

func TestNextSiteFixVerifyPollAtStopsAtDeadline(t *testing.T) {
	merged := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	if next, ok := nextSiteFixVerifyPollAt(merged, merged); !ok || !next.Equal(merged.Add(5*time.Minute)) {
		t.Fatalf("first verify check = %v ok=%v", next, ok)
	}
	if _, ok := nextSiteFixVerifyPollAt(merged, merged.Add(siteFixVerifyDeadline)); ok {
		t.Fatal("expected verification give-up at the 24h deadline")
	}
}

func TestSiteFixVerifiedPublisherResultRecordsCompletedPRState(t *testing.T) {
	prNumber := int32(175)
	prURL := "https://github.com/bugfreev587/unipost/pull/175"
	repo := "bugfreev587/unipost"
	base := "main"
	verifiedAt := time.Date(2026, 7, 9, 21, 50, 0, 0, time.UTC)
	app := db.SiteChangeApplication{
		ID:             [16]byte{1},
		GithubPrNumber: &prNumber,
		GithubPrUrl:    &prURL,
		RepoFullName:   &repo,
		BaseBranch:     &base,
		TargetUrl:      "https://unipost.dev/",
	}

	var result map[string]any
	if err := json.Unmarshal(siteFixVerifiedPublisherResult(app, "auto_url_check", verifiedAt), &result); err != nil {
		t.Fatalf("decode verified publisher result: %v", err)
	}
	for key, want := range map[string]any{
		"status":              "verified",
		"github_pr_state":     "merged",
		"github_pr_url":       prURL,
		"verification_source": "auto_url_check",
		"verified_at":         "2026-07-09T21:50:00Z",
	} {
		if got := result[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
}

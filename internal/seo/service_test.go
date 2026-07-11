package seo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestProviderCanAttemptAfterRecoverableStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{status: "connected", want: true},
		{status: "backfilling", want: true},
		{status: "stale", want: true},
		{status: "error", want: true},
		{status: "expired", want: true},
		{status: "missing", want: false},
		{status: "property_selection_required", want: false},
		{status: "mismatch", want: false},
		{status: "revoked", want: false},
	}
	for _, tc := range cases {
		got := isProviderAttemptable([]db.SeoIntegration{{
			Provider: ProviderGSC,
			Status:   tc.status,
		}}, ProviderGSC)
		if got != tc.want {
			t.Fatalf("status %q attemptable = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestFinishRunContextSurvivesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	finishCtx, finishCancel := finishRunContext(ctx)
	defer finishCancel()

	if err := finishCtx.Err(); err != nil {
		t.Fatalf("finish context err = %v, want nil", err)
	}
}

func TestCheckURLExtractsRepairMetadataFacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
  <head>
    <title>UniPost | Unified Social Media Posting API for Developers</title>
    <link rel="canonical" href="/" />
    <link rel="icon" href="/brand/unipost-icon-dark.png" sizes="512x512" type="image/png" />
    <meta name="description" content="UniPost is a unified social media posting API for developers." />
    <meta property="og:site_name" content="UniPost" />
    <meta property="og:description" content="Build social publishing into your product with one API." />
  </head>
  <body><main><h1>Build the social layer once</h1></main></body>
</html>`))
	}))
	defer server.Close()

	result := Service{}.checkURL(context.Background(), server.URL, server.URL)

	if got := result.RawDetails["page_title"]; got != "UniPost | Unified Social Media Posting API for Developers" {
		t.Fatalf("page_title = %#v", got)
	}
	if got := result.RawDetails["meta_description"]; got != "UniPost is a unified social media posting API for developers." {
		t.Fatalf("meta_description = %#v", got)
	}
	if got := result.RawDetails["og_site_name"]; got != "UniPost" {
		t.Fatalf("og_site_name = %#v", got)
	}
	if got := result.RawDetails["html_lang"]; got != "en" {
		t.Fatalf("html_lang = %#v", got)
	}
	if got := result.RawDetails["canonical_url"]; got != server.URL+"/" {
		t.Fatalf("canonical_url = %#v", got)
	}
	logos, ok := result.RawDetails["logo_candidates"].([]string)
	if !ok || len(logos) != 1 || logos[0] != server.URL+"/brand/unipost-icon-dark.png" {
		t.Fatalf("logo_candidates = %#v", result.RawDetails["logo_candidates"])
	}
	if got := result.RawDetails["site_search_observed"]; got != false {
		t.Fatalf("site_search_observed = %#v, want false", got)
	}
}

func TestCheckURLProducesCitationReadinessEvidenceForDoctor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<!doctype html><html><head>
<meta name="application-name" content="Cite Loop"><meta property="og:site_name" content="CiteLoop">
</head><body><article><h1>Explain the existing Doctor product</h1><p>Doctor checks the existing live site for technical problems.<a rel="cite" href="https://docs.example.org/one">source</a></p>
<p>Each finding keeps the source evidence needed for verification.<cite><a href="https://docs.example.org/two">source</a></cite></p></article></body></html>`))
	}))
	defer server.Close()

	result := Service{}.checkURL(context.Background(), server.URL, server.URL)
	citation, ok := result.RawDetails["citation_readiness"].(map[string]any)
	if !ok {
		t.Fatalf("citation_readiness evidence = %#v", result.RawDetails["citation_readiness"])
	}
	if citation["supported_fact_extractability"] != "needs_optimization" || citation["source_association_status"] != "associated" {
		t.Fatalf("citation readiness = %#v", citation)
	}
	if citation["entity_name"] != "Cite Loop" || citation["canonical_entity_name"] != "CiteLoop" {
		t.Fatalf("entity evidence = %#v", citation)
	}
	if propositions, ok := citation["preserved_propositions"].([]string); !ok || len(propositions) != 2 {
		t.Fatalf("preserved propositions = %#v", citation["preserved_propositions"])
	}
}

func TestCheckSitemapStatusRequiresExactLocationMatch(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<urlset><url><loc>` + server.URL + `/foobar</loc></url></urlset>`))
	}))
	defer server.Close()
	if got := checkSitemapStatus(context.Background(), server.Client(), server.URL, server.URL+"/foo"); got != "missing" {
		t.Fatalf("prefix-only sitemap match = %q, want missing", got)
	}
}

func TestCheckURLHonorsXRobotsTagNoindex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		_, _ = w.Write([]byte("<html><head><title>Header robots</title></head><body><h1>Header robots</h1></body></html>"))
	}))
	defer server.Close()
	result := (Service{HTTPClient: server.Client()}).checkURL(context.Background(), server.URL+"/page", server.URL)
	if result.RobotsStatus != "noindex" {
		t.Fatalf("robots status = %q, want noindex from X-Robots-Tag", result.RobotsStatus)
	}
}

func TestRobotsStatusParsesDirectivesWithoutBodyFalsePositives(t *testing.T) {
	if got := robotsStatus(`<html><body>This article discusses noindex.</body></html>`); got != "index" {
		t.Fatalf("body text status = %q", got)
	}
	if got := robotsStatus(`<meta name="robots" content="noindex,follow">`); got != "noindex" {
		t.Fatalf("meta status = %q", got)
	}
	if got := robotsStatus(`<html></html>`, "OtherBot: noindex"); got != "index" {
		t.Fatalf("unrelated scoped header = %q", got)
	}
	if got := robotsStatus(`<html></html>`, "Googlebot: noindex"); got != "noindex" {
		t.Fatalf("applicable scoped header = %q", got)
	}
	if got := robotsStatus(`<html></html>`, "Googlebot: index, OtherBot: noindex"); got != "index" {
		t.Fatalf("mixed scoped header = %q", got)
	}
	if got := robotsStatus(`<meta name="robots" content="none">`); got != "noindex" {
		t.Fatalf("robots none status = %q", got)
	}
}

func TestCanonicalStatusValidatesExactSingleFinalURL(t *testing.T) {
	base := "https://example.com/final"
	for _, tc := range []struct{ name, html, want string }{
		{"exact", `<link rel="canonical" href="https://example.com/final">`, "present"},
		{"wrong", `<link rel="canonical" href="https://example.com/other">`, "mismatch"},
		{"duplicate", `<link rel="canonical" href="https://example.com/final"><link rel="canonical" href="https://example.com/final">`, "multiple"},
		{"malformed", `<link rel="canonical" href="://bad">`, "invalid"},
		{"unsupported scheme", `<link rel="canonical" href="javascript:alert(1)">`, "invalid"},
		{"missing", `<html></html>`, "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := classifyCanonical(tc.html, base, base, URLNormalizationConfig{})
			if status != tc.want {
				t.Fatalf("status=%q want=%q", status, tc.want)
			}
		})
	}
}

func TestCheckURLValidatesCanonicalAgainstRedirectFinalURL(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			_, _ = w.Write([]byte(`<urlset><url><loc>` + server.URL + `/final</loc></url></urlset>`))
		case "/start":
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/final":
			_, _ = w.Write([]byte(`<html><head><link rel="canonical" href="` + server.URL + `/final"></head></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result := (Service{HTTPClient: server.Client()}).checkURL(context.Background(), server.URL+"/start", server.URL)
	if result.CanonicalStatus != "present" {
		t.Fatalf("canonical status = %q, want present against redirect final URL", result.CanonicalStatus)
	}
	if got := result.RawDetails["final_url"]; got != server.URL+"/final" {
		t.Fatalf("final_url = %#v", got)
	}
}

func TestCheckURLMarksTruncatedBodyAsPartial(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			_, _ = w.Write([]byte(`<urlset><url><loc>` + server.URL + `/large</loc></url></urlset>`))
			return
		}
		_, _ = w.Write([]byte(`<html><head>` + strings.Repeat("x", (1<<20)+1) + `<title>Large page</title><link rel="canonical" href="` + server.URL + `/large"><meta name="description" content="after cutoff"><meta name="robots" content="noindex"><script type="application/ld+json">{}</script></head><body><h1>After cutoff</h1><a href="/inside">Inside</a></body></html>`))
	}))
	defer server.Close()

	result := (Service{HTTPClient: server.Client()}).checkURL(context.Background(), server.URL+"/large", server.URL)
	if got := result.RawDetails["crawl_status"]; got != "partial" {
		t.Fatalf("crawl_status = %#v, want partial", got)
	}
	for name, status := range map[string]string{
		"canonical":        result.CanonicalStatus,
		"robots":           result.RobotsStatus,
		"title":            result.TitleStatus,
		"meta_description": result.MetaDescriptionStatus,
		"h1":               result.H1Status,
		"structured_data":  result.StructuredDataStatus,
	} {
		if status != "unknown" {
			t.Fatalf("%s status = %q, want unknown for partial body", name, status)
		}
	}
	params := technicalCheckParams(uuid.New(), uuid.New(), server.URL+"/large", server.URL+"/large", pgtype.UUID{}, nil, false, result)
	if params.InternalLinkCount != nil || params.OutboundLinkCount != nil {
		t.Fatalf("partial body link counts must be nil: internal=%v outbound=%v", params.InternalLinkCount, params.OutboundLinkCount)
	}
	if findings := doctorFindingCandidatesFromChecks(uuid.New(), []db.TechnicalCheck{technicalCheckFromParams(params)}); len(findings) != 0 {
		t.Fatalf("partial body produced repair candidates: %#v", findings)
	}
}

func TestSitemapInventoryTraversesIndexOnceAndReusesExactLocations(t *testing.T) {
	requests := map[string]int{}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		switch r.URL.Path {
		case "/sitemap.xml":
			_, _ = w.Write([]byte(`<sitemapindex><sitemap><loc>` + server.URL + `/child.xml</loc></sitemap></sitemapindex>`))
		case "/child.xml":
			_, _ = w.Write([]byte(`<urlset><url><loc>` + server.URL + `/a</loc></url><url><loc>` + server.URL + `/b</loc></url></urlset>`))
		default:
			_, _ = w.Write([]byte(`<html><head><title>Page title</title></head><body><h1>Page</h1></body></html>`))
		}
	}))
	defer server.Close()
	inv := loadSitemapInventory(context.Background(), server.Client(), server.URL, URLNormalizationConfig{})
	for _, page := range []string{server.URL + "/a", server.URL + "/b"} {
		_ = (Service{HTTPClient: server.Client()}).checkURLWithSitemap(context.Background(), page, server.URL, URLNormalizationConfig{}, &inv)
	}
	if requests["/sitemap.xml"] != 1 || requests["/child.xml"] != 1 {
		t.Fatalf("requests=%v", requests)
	}
	if !inv.Contains(server.URL+"/a") || inv.Contains(server.URL+"/ab") {
		t.Fatalf("inventory=%+v", inv)
	}
}

func TestSitemapInventoryStopsAtDocumentBound(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			var body strings.Builder
			body.WriteString("<sitemapindex>")
			for i := 0; i < 40; i++ {
				fmt.Fprintf(&body, "<sitemap><loc>%s/child-%d.xml</loc></sitemap>", server.URL, i)
			}
			body.WriteString("</sitemapindex>")
			_, _ = w.Write([]byte(body.String()))
			return
		}
		_, _ = w.Write([]byte(`<urlset><url><loc>` + server.URL + `/page</loc></url></urlset>`))
	}))
	defer server.Close()

	inv := loadSitemapInventory(context.Background(), server.Client(), server.URL, URLNormalizationConfig{})
	if inv.Complete {
		t.Fatal("bounded sitemap inventory must be marked incomplete")
	}
	if inv.Documents > 32 {
		t.Fatalf("documents = %d, want at most 32", inv.Documents)
	}
}

func TestSitemapInventoryRejectsCrossOriginRedirects(t *testing.T) {
	externalRequests := 0
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		externalRequests++
		_, _ = w.Write([]byte(`<urlset><url><loc>https://example.com/leaked</loc></url></urlset>`))
	}))
	defer external.Close()

	property := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, external.URL+"/outside.xml", http.StatusFound)
	}))
	defer property.Close()

	inv := loadSitemapInventory(context.Background(), property.Client(), property.URL, URLNormalizationConfig{})
	if inv.Complete {
		t.Fatal("cross-origin sitemap redirect must make inventory incomplete")
	}
	if externalRequests != 0 {
		t.Fatalf("cross-origin redirect was followed %d times", externalRequests)
	}
	if len(inv.URLs) != 0 {
		t.Fatalf("cross-origin sitemap URLs were accepted: %#v", inv.URLs)
	}
}

func TestSitemapInventoryUsesPropertyNormalizationConfig(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<urlset><url><loc>` + server.URL + `/Product?id=1&amp;drop=source</loc></url></urlset>`))
	}))
	defer server.Close()

	configured := loadSitemapInventory(context.Background(), server.Client(), server.URL, URLNormalizationConfig{
		KeepQueryKeys: []string{"id"},
		LowercasePath: true,
		PreserveHTTP:  true,
	})
	if !configured.Contains(server.URL + "/PRODUCT?id=1&drop=request") {
		t.Fatal("configured inventory did not apply KeepQueryKeys and LowercasePath")
	}
	if configured.Contains(server.URL + "/product?id=2") {
		t.Fatal("configured inventory collapsed distinct kept query values")
	}
	httpsURL := "https://" + strings.TrimPrefix(server.URL, "http://") + "/product?id=1"
	if configured.Contains(httpsURL) {
		t.Fatal("configured inventory ignored PreserveHTTP")
	}

	defaults := loadSitemapInventory(context.Background(), server.Client(), server.URL, URLNormalizationConfig{})
	if !defaults.Contains(server.URL + "/Product?id=2") {
		t.Fatal("default inventory should drop unconfigured query keys")
	}
	if defaults.Contains(server.URL + "/PRODUCT?id=2") {
		t.Fatal("default inventory should preserve path case")
	}
	if !defaults.Contains("https://" + strings.TrimPrefix(server.URL, "http://") + "/Product?id=2") {
		t.Fatal("default inventory should normalize same-host HTTP to HTTPS")
	}
}

func TestCitationEvidenceRequiresExplicitPropositionSourceAssociation(t *testing.T) {
	unrelated := extractRepairMetadataFacts(`<article><p>This is a sufficiently long product proposition for testing.</p><p>Another sufficiently long product proposition for testing.</p><a href="https://source.example/doc">unrelated footer</a></article>`, mustParseURL(t, "https://example.com"))
	if _, ok := unrelated["citation_readiness"]; ok {
		t.Fatalf("unrelated link produced citation evidence: %#v", unrelated)
	}
	explicit := extractRepairMetadataFacts(`<article><p>First supported proposition is explicitly sourced.<a rel="cite" href="https://source.example/one">source</a></p><p>Second supported proposition is explicitly sourced.<cite><a href="https://source.example/two">source</a></cite></p></article>`, mustParseURL(t, "https://example.com"))
	citation, ok := explicit["citation_readiness"].(map[string]any)
	if !ok {
		t.Fatalf("explicit evidence=%#v", explicit)
	}
	changes, _ := citation["source_association_changes"].([]any)
	if len(changes) != 2 {
		t.Fatalf("changes=%#v", citation["source_association_changes"])
	}

	plain := extractRepairMetadataFacts(`<article><p>A supported proposition has a source in the same proposition element.<a href="https://source.example/plain">source</a></p></article>`, mustParseURL(t, "https://example.com"))
	plainCitation, ok := plain["citation_readiness"].(map[string]any)
	if !ok {
		t.Fatalf("same-proposition plain link produced no evidence: %#v", plain)
	}
	if got := plainCitation["source_association_status"]; got != "missing_visible_association" {
		t.Fatalf("source association status = %#v", got)
	}
	page := fullyCheckedDoctorPage("https://example.com/page", nil, nil, nil, plain)
	findings, growth := doctorFindingCandidatesAndGrowthFromChecks(uuid.New(), []db.TechnicalCheck{page})
	if len(growth) != 0 {
		t.Fatalf("association repair was routed to growth: %#v", growth)
	}
	foundAssociation := false
	for _, finding := range findings {
		foundAssociation = foundAssociation || finding.IssueType == "source_association"
	}
	if !foundAssociation {
		t.Fatalf("same-proposition plain source emitted no association optimization: %#v", findings)
	}
}

func mustParseURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestGA4IntegrationFailureRequiresReconnectForInsufficientScope(t *testing.T) {
	err := errors.New(`google api status 403: { "error": { "code": 403, "message": "Request had insufficient authentication scopes.", "status": "PERMISSION_DENIED", "details": [ { "reason": "ACCESS_TOKEN_SCOPE_INSUFFICIENT" } ] } }`)

	status, message, note := ga4IntegrationFailureForError(err)

	if status != "reconnect_required" {
		t.Fatalf("status = %q, want reconnect_required", status)
	}
	want := "Google Analytics permission is missing. Update Analytics access from Settings so CiteLoop can request Analytics read access, then run SEO sync again."
	if message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
	if note != "ga4_reconnect_required" {
		t.Fatalf("note = %q, want ga4_reconnect_required", note)
	}
}

func TestGA4IntegrationFailureRequiresPropertyAccessForPermissionDeniedProperty(t *testing.T) {
	err := errors.New(`google api status 403: { "error": { "code": 403, "message": "User does not have sufficient permissions for this property. To learn more about Property ID, see https://developers.google.com/analytics/devguides/reporting/data/v1/property-id.", "status": "PERMISSION_DENIED" } }`)

	status, message, note := ga4IntegrationFailureForError(err)

	if status != "property_access_required" {
		t.Fatalf("status = %q, want property_access_required", status)
	}
	want := "Google Analytics property access is missing. Confirm the numeric GA4 Property ID and grant the connected Google account Viewer access in GA4 Property Access Management, then run SEO sync again."
	if message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
	if note != "ga4_property_access_required" {
		t.Fatalf("note = %q, want ga4_property_access_required", note)
	}
}

func TestGA4IntegrationFailurePreservesOtherErrors(t *testing.T) {
	status, message, note := ga4IntegrationFailureForError(errors.New("google api status 404: property not found"))

	if status != "error" {
		t.Fatalf("status = %q, want error", status)
	}
	if message != "google api status 404: property not found" {
		t.Fatalf("message = %q", message)
	}
	if note != "ga4_error" {
		t.Fatalf("note = %q, want ga4_error", note)
	}
}

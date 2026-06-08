package geo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRobotsRulesAreStaticPerTargetAgent(t *testing.T) {
	body := `User-agent: *
Disallow: /private

User-agent: OAI-SearchBot
Disallow: /blocked

User-agent: PerplexityBot
Allow: /
`
	rules := ParseRobots(strings.NewReader(body))

	if got := rules.StateFor("OAI-SearchBot", "/blocked/page"); got != RobotsDisallowed {
		t.Fatalf("OAI-SearchBot /blocked = %s, want %s", got, RobotsDisallowed)
	}
	if got := rules.StateFor("PerplexityBot", "/blocked/page"); got != RobotsAllowed {
		t.Fatalf("PerplexityBot /blocked = %s, want %s", got, RobotsAllowed)
	}
	if got := rules.StateFor("Claude-SearchBot", "/private/page"); got != RobotsDisallowed {
		t.Fatalf("Claude-SearchBot /private = %s, want %s", got, RobotsDisallowed)
	}
}

func TestAuditorUsesHonestProbeUserAgent(t *testing.T) {
	var seenUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("User-agent: OAI-SearchBot\nDisallow: /blocked\n"))
			return
		}
		seenUA = r.UserAgent()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><head><title>x</title></head><body>hello</body></html>"))
	}))
	defer server.Close()

	auditor := Auditor{HTTPClient: server.Client()}
	results := auditor.Audit(context.Background(), AuditRequest{
		SiteURL:          server.URL,
		URLs:             []string{server.URL + "/blocked"},
		TargetUserAgents: []string{"OAI-SearchBot"},
	})

	if seenUA != HonestProbeUserAgent {
		t.Fatalf("probe UA = %q, want %q", seenUA, HonestProbeUserAgent)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].RobotsState != RobotsDisallowed || results[0].Confidence != ConfidenceHigh || !results[0].Inferred {
		t.Fatalf("result = %+v, want high-confidence inferred robots disallow", results[0])
	}
}

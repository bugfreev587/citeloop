package geo

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	seopkg "github.com/citeloop/citeloop/internal/seo"
)

const (
	HonestProbeUserAgent = "CiteLoop GEO crawler access auditor"

	EvidenceRobotsStatic       = "robots_static"
	EvidenceHonestProbe        = "honest_probe"
	EvidenceManualConfirmation = "manual_confirmation"

	AccessOK          = "ok"
	AccessBlocked     = "blocked"
	AccessChallenge   = "challenge"
	AccessRateLimited = "rate_limited"
	AccessTimeout     = "timeout"
	AccessError       = "error"

	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

type AuditRequest struct {
	SiteURL          string
	URLs             []string
	TargetUserAgents []string
}

type AuditResult struct {
	PageURL           string
	NormalizedPageURL string
	TargetUserAgent   string
	ProbeUserAgent    string
	EvidenceType      string
	RobotsState       RobotsState
	HTTPStatus        *int32
	AccessState       string
	Confidence        string
	Inferred          bool
	MetaRobotsState   string
	SitemapState      string
	BodyExtractable   bool
	RawDetails        map[string]any
}

type Auditor struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

type probeResult struct {
	status          *int32
	accessState     string
	metaRobotsState string
	bodyExtractable bool
	raw             map[string]any
}

func (a Auditor) Audit(ctx context.Context, req AuditRequest) []AuditResult {
	targets := req.TargetUserAgents
	if len(targets) == 0 {
		targets = DefaultTargetUserAgents()
	}
	robots := a.fetchRobots(ctx, req.SiteURL)
	probes := map[string]probeResult{}
	for _, rawURL := range uniqueStrings(req.URLs) {
		probes[rawURL] = a.probe(ctx, rawURL)
	}

	results := make([]AuditResult, 0, len(req.URLs)*len(targets))
	for _, rawURL := range uniqueStrings(req.URLs) {
		normalized := rawURL
		if n, err := seopkg.NormalizeURL(rawURL, req.SiteURL, seopkg.URLNormalizationConfig{}); err == nil {
			normalized = n
		}
		path := pathForRobots(rawURL, req.SiteURL)
		probe := probes[rawURL]
		for _, target := range targets {
			robotsState := robots.StateFor(target, path)
			result := AuditResult{
				PageURL:           rawURL,
				NormalizedPageURL: normalized,
				TargetUserAgent:   target,
				ProbeUserAgent:    HonestProbeUserAgent,
				EvidenceType:      EvidenceHonestProbe,
				RobotsState:       robotsState,
				HTTPStatus:        probe.status,
				AccessState:       probe.accessState,
				Confidence:        ConfidenceMedium,
				Inferred:          false,
				MetaRobotsState:   probe.metaRobotsState,
				SitemapState:      "unknown",
				BodyExtractable:   probe.bodyExtractable,
				RawDetails:        probe.raw,
			}
			if robotsState == RobotsDisallowed {
				result.EvidenceType = EvidenceRobotsStatic
				result.AccessState = AccessBlocked
				result.Confidence = ConfidenceHigh
				result.Inferred = true
			} else if probe.accessState != AccessOK {
				result.Confidence = ConfidenceMedium
				result.Inferred = true
			} else if robotsState == RobotsAllowed {
				result.EvidenceType = EvidenceRobotsStatic
				result.Confidence = ConfidenceHigh
				result.Inferred = true
			}
			results = append(results, result)
		}
	}
	return results
}

func DefaultTargetUserAgents() []string {
	return []string{"Googlebot", "Bingbot", "OAI-SearchBot", "PerplexityBot", "Perplexity-User", "Claude-SearchBot", "Claude-User"}
}

func (a Auditor) client() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return http.DefaultClient
}

func (a Auditor) fetchRobots(ctx context.Context, siteURL string) RobotsRules {
	base, err := url.Parse(strings.TrimSpace(siteURL))
	if err != nil {
		return RobotsRules{byAgent: map[string][]robotsRule{}}
	}
	if base.Scheme == "" {
		base.Scheme = "https"
	}
	base.Path = "/robots.txt"
	base.RawQuery = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return RobotsRules{byAgent: map[string][]robotsRule{}}
	}
	request.Header.Set("User-Agent", HonestProbeUserAgent)
	response, err := a.client().Do(request)
	if err != nil {
		return RobotsRules{byAgent: map[string][]robotsRule{}}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return RobotsRules{byAgent: map[string][]robotsRule{}}
	}
	return ParseRobots(io.LimitReader(response.Body, 1<<20))
}

func (a Auditor) probe(ctx context.Context, rawURL string) probeResult {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return failedProbe(err.Error())
	}
	request.Header.Set("User-Agent", HonestProbeUserAgent)
	response, err := a.client().Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return probeResult{accessState: AccessTimeout, raw: map[string]any{"error": ctx.Err().Error()}}
		}
		return failedProbe(err.Error())
	}
	defer response.Body.Close()
	status := int32(response.StatusCode)
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	bodyText := strings.ToLower(string(body))
	state := classifyAccess(response.StatusCode, bodyText)
	return probeResult{
		status:          &status,
		accessState:     state,
		metaRobotsState: metaRobotsState(bodyText),
		bodyExtractable: len(strings.TrimSpace(stripTagHints(bodyText))) > 0,
		raw: map[string]any{
			"status":     response.Status,
			"final_url":  response.Request.URL.String(),
			"body_bytes": len(body),
		},
	}
}

func failedProbe(message string) probeResult {
	return probeResult{accessState: AccessError, raw: map[string]any{"error": message}}
}

func classifyAccess(status int, body string) string {
	switch {
	case status == http.StatusTooManyRequests:
		return AccessRateLimited
	case status == http.StatusForbidden || status == http.StatusUnauthorized:
		if looksLikeChallenge(body) {
			return AccessChallenge
		}
		return AccessBlocked
	case looksLikeChallenge(body):
		return AccessChallenge
	case status >= 200 && status < 400:
		return AccessOK
	default:
		return AccessError
	}
}

func looksLikeChallenge(body string) bool {
	return strings.Contains(body, "captcha") ||
		strings.Contains(body, "cf-chl") ||
		strings.Contains(body, "cloudflare") ||
		strings.Contains(body, "js challenge")
}

func metaRobotsState(body string) string {
	if strings.Contains(body, "noindex") {
		return "noindex"
	}
	if strings.Contains(body, `name="robots"`) || strings.Contains(body, `name='robots'`) {
		return "present"
	}
	return "missing"
}

func stripTagHints(body string) string {
	body = strings.ReplaceAll(body, "<", " ")
	body = strings.ReplaceAll(body, ">", " ")
	body = strings.ReplaceAll(body, "/", " ")
	return body
}

func pathForRobots(rawURL, siteURL string) string {
	base, _ := url.Parse(siteURL)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "/"
	}
	if !parsed.IsAbs() && base != nil {
		parsed = base.ResolveReference(parsed)
	}
	if parsed.Path == "" {
		return "/"
	}
	return parsed.EscapedPath()
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

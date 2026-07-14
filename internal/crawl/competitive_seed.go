package crawl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// SeedURLEnrichment is a derived-facts report for a user-provided public seed
// URL. It intentionally avoids storing the full competitor HTML body; callers
// get the normalized URL, crawl/indexability facts, sitemap evidence, and
// archetype signals needed by competitive discovery.
type SeedURLEnrichment struct {
	URL                    string             `json:"url"`
	FinalURL               string             `json:"final_url"`
	CanonicalURL           string             `json:"canonical_url"`
	Host                   string             `json:"host"`
	StatusCode             int                `json:"status_code"`
	RobotsAllowed          bool               `json:"robots_allowed"`
	RobotsSitemaps         []string           `json:"robots_sitemaps"`
	Indexable              bool               `json:"indexable"`
	Title                  string             `json:"title"`
	SitemapIncluded        bool               `json:"sitemap_included"`
	SitemapURLSamples      []string           `json:"sitemap_url_samples"`
	SitemapTruncated       bool               `json:"sitemap_truncated"`
	SameArchetypeLinkCount int                `json:"same_archetype_link_count"`
	Archetypes             []SeedURLArchetype `json:"archetypes"`
	Signals                []string           `json:"signals"`
	FilterReasons          []string           `json:"filter_reasons,omitempty"`
}

type SeedURLArchetype struct {
	Archetype  string   `json:"archetype"`
	Confidence string   `json:"confidence"`
	Signals    []string `json:"signals"`
}

func (r SeedURLEnrichment) TopArchetype() SeedURLArchetype {
	if len(r.Archetypes) == 0 {
		return SeedURLArchetype{}
	}
	return r.Archetypes[0]
}

// EnrichSeedURL fetches a single user-provided URL and samples the domain's
// sitemap to produce competitive-discovery facts for repair/direct-seed flows.
func (c *Crawler) EnrichSeedURL(ctx context.Context, rawURL string) (*SeedURLEnrichment, error) {
	seed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid seed url: %w", err)
	}
	if seed.Scheme == "" || seed.Host == "" {
		return nil, fmt.Errorf("invalid seed url: missing scheme or host")
	}
	normalizedSeed, err := Normalize(seed.String())
	if err != nil {
		normalizedSeed = seed.String()
	}
	report := &SeedURLEnrichment{
		URL:           normalizedSeed,
		Host:          strings.ToLower(seed.Host),
		RobotsAllowed: true,
		Indexable:     true,
	}

	var rb *robots
	if c.cfg.RespectRobots {
		rb = fetchRobots(ctx, c.client, seed)
		report.RobotsSitemaps = append([]string{}, rb.sitemaps...)
		report.RobotsAllowed = rb.allowed(seed.Path)
		if !report.RobotsAllowed {
			report.FilterReasons = append(report.FilterReasons, "robots_disallowed")
			return report, nil
		}
	}

	c.wait()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, seed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	report.StatusCode = resp.StatusCode
	if resp.StatusCode >= 400 {
		report.FilterReasons = append(report.FilterReasons, "http_error")
		return report, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}
	htmlStr := string(body)

	report.FinalURL = resp.Request.URL.String()
	canonical := report.FinalURL
	if found := canonicalLink(htmlStr, resp.Request.URL); found != "" {
		canonical = found
	}
	if normalizedCanonical, err := Normalize(canonical); err == nil {
		canonical = normalizedCanonical
	}
	report.CanonicalURL = canonical
	report.Title = extractTitle(htmlStr)
	report.Indexable = !metaRobotsNoindex(htmlStr)
	if !report.Indexable {
		report.FilterReasons = append(report.FilterReasons, "noindex")
	}

	sitemapEntries, sitemapTruncated := c.seedSitemapEntries(ctx, seed, rb)
	report.SitemapTruncated = sitemapTruncated
	report.SitemapIncluded, report.SitemapURLSamples = sitemapSeedFacts(sitemapEntries, report.CanonicalURL, report.URL)
	if report.SitemapIncluded {
		report.Signals = append(report.Signals, "sitemap_included")
	}
	if sitemapHasRelatedComparisonOrScheduler(sitemapEntries) {
		report.Signals = append(report.Signals, "related_comparison_or_scheduler_pages")
	}

	links := extractHTMLLinks(htmlStr, resp.Request.URL)
	report.SameArchetypeLinkCount = sameArchetypeLinkCount(seed, links)
	if report.SameArchetypeLinkCount >= 20 {
		report.Signals = append(report.Signals, "many_same_archetype_links")
	}
	if hasFreeToolsLanguage(report.Title, htmlStr) {
		report.Signals = append(report.Signals, "free_tools_language")
	}
	report.Archetypes = seedURLArchetypes(seed, report.Signals, report.SameArchetypeLinkCount)
	report.Signals = dedupeStrings(report.Signals)
	return report, nil
}

func (c *Crawler) seedSitemapEntries(ctx context.Context, seed *url.URL, rb *robots) ([]sitemapEntry, bool) {
	sitemaps := []string{}
	if rb != nil {
		sitemaps = append(sitemaps, rb.sitemaps...)
	}
	if len(sitemaps) == 0 {
		su := *seed
		su.Path = "/sitemap.xml"
		su.RawQuery = ""
		sitemaps = append(sitemaps, su.String())
	}
	cap := c.cfg.SitemapURLCap
	if cap <= 0 {
		cap = 100
	}
	return c.collectSitemap(ctx, sitemaps, cap)
}

func sitemapSeedFacts(entries []sitemapEntry, canonicalURL, seedURL string) (bool, []string) {
	targets := map[string]bool{canonicalURL: true, seedURL: true}
	var samples []string
	included := false
	for _, entry := range entries {
		normalized, err := Normalize(entry.loc)
		if err != nil {
			normalized = entry.loc
		}
		if targets[normalized] {
			included = true
		}
		if len(samples) < 20 {
			samples = append(samples, normalized)
		}
	}
	return included, dedupeStrings(samples)
}

func sitemapHasRelatedComparisonOrScheduler(entries []sitemapEntry) bool {
	for _, entry := range entries {
		path := strings.ToLower(entry.loc)
		if strings.Contains(path, "/compare") || strings.Contains(path, "comparison") || strings.Contains(path, "scheduler") {
			return true
		}
	}
	return false
}

func sameArchetypeLinkCount(seed *url.URL, links []string) int {
	segment := firstPathSegment(seed.Path)
	if segment == "" {
		return 0
	}
	seen := map[string]bool{}
	for _, raw := range links {
		normalized, err := Normalize(raw)
		if err != nil {
			continue
		}
		u, err := url.Parse(normalized)
		if err != nil || !SameOrigin(seed, u) {
			continue
		}
		if firstPathSegment(u.Path) != segment || strings.EqualFold(strings.TrimRight(u.Path, "/"), strings.TrimRight(seed.Path, "/")) {
			continue
		}
		seen[normalized] = true
	}
	return len(seen)
}

func firstPathSegment(path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	if len(parts) == 0 {
		return ""
	}
	return strings.ToLower(parts[0])
}

func hasFreeToolsLanguage(title, htmlStr string) bool {
	text := strings.ToLower(title + " " + stripTags(htmlStr))
	return strings.Contains(text, "free") && strings.Contains(text, "tools")
}

func seedURLArchetypes(seed *url.URL, signals []string, sameArchetypeLinks int) []SeedURLArchetype {
	if firstPathSegment(seed.Path) != "tools" {
		return nil
	}
	confidence := "medium"
	if sameArchetypeLinks >= 100 &&
		hasSignal(signals, "free_tools_language") &&
		hasSignal(signals, "sitemap_included") {
		confidence = "high"
	}
	return []SeedURLArchetype{{
		Archetype:  "tools_hub",
		Confidence: confidence,
		Signals:    append([]string{}, signals...),
	}}
}

func hasSignal(signals []string, signal string) bool {
	for _, existing := range signals {
		if existing == signal {
			return true
		}
	}
	return false
}

func extractHTMLLinks(htmlStr string, base *url.URL) []string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					if abs := resolve(base, attr.Val); abs != "" {
						out = append(out, abs)
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return out
}

func metaRobotsNoindex(htmlStr string) bool {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return false
	}
	var noindex bool
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if noindex {
			return
		}
		if n.Type == html.ElementNode && n.Data == "meta" {
			var name, content string
			for _, attr := range n.Attr {
				switch strings.ToLower(attr.Key) {
				case "name":
					name = strings.ToLower(attr.Val)
				case "content":
					content = strings.ToLower(attr.Val)
				}
			}
			if (name == "robots" || name == "googlebot") && strings.Contains(content, "noindex") {
				noindex = true
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return noindex
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

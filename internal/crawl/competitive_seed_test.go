package crawl

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
)

func TestEnrichSeedURLDetectsPostSyncerToolsHubFixture(t *testing.T) {
	var baseURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("content-type", "text/plain")
			_, _ = fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: %s/sitemap.xml\n", baseURL)
		case "/sitemap.xml":
			w.Header().Set("content-type", "application/xml")
			var sb strings.Builder
			sb.WriteString(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
			fmt.Fprintf(&sb, "<url><loc>%s/tools</loc><lastmod>2026-07-01</lastmod></url>", baseURL)
			fmt.Fprintf(&sb, "<url><loc>%s/tools/social-media-caption-generator</loc><lastmod>2026-07-01</lastmod></url>", baseURL)
			fmt.Fprintf(&sb, "<url><loc>%s/tools/social-media-post-formatter</loc><lastmod>2026-07-01</lastmod></url>", baseURL)
			fmt.Fprintf(&sb, "<url><loc>%s/compare/buffer-alternative</loc><lastmod>2026-07-01</lastmod></url>", baseURL)
			fmt.Fprintf(&sb, "<url><loc>%s/social-media-scheduler</loc><lastmod>2026-07-01</lastmod></url>", baseURL)
			sb.WriteString(`</urlset>`)
			_, _ = w.Write([]byte(sb.String()))
		case "/tools":
			w.Header().Set("content-type", "text/html; charset=utf-8")
			var sb strings.Builder
			sb.WriteString(`<html><head><title>Free Social Media Tools</title>`)
			fmt.Fprintf(&sb, `<link rel="canonical" href="%s/tools">`, baseURL)
			sb.WriteString(`</head><body><h1>100+ free social media tools</h1>`)
			for i := 0; i < 120; i++ {
				fmt.Fprintf(&sb, `<a href="/tools/social-tool-%03d">Tool %03d</a>`, i, i)
			}
			sb.WriteString(`</body></html>`)
			_, _ = w.Write([]byte(sb.String()))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	baseURL = srv.URL

	c := New(config.CrawlConfig{
		RequestTimeoutMs: 1000,
		RateLimitRPS:     1000,
		RespectRobots:    true,
		SitemapURLCap:    20,
		MaxPages:         5,
		MaxDepth:         1,
		SameOriginOnly:   true,
	}, slog.Default())

	report, err := c.EnrichSeedURL(context.Background(), baseURL+"/tools")
	if err != nil {
		t.Fatalf("EnrichSeedURL error: %v", err)
	}

	if report.StatusCode != http.StatusOK || !report.RobotsAllowed || !report.Indexable || !report.SitemapIncluded {
		t.Fatalf("report basics = %+v, want 2xx, robots allowed, indexable, sitemap included", report)
	}
	if report.CanonicalURL != baseURL+"/tools" {
		t.Fatalf("canonical = %q, want %q", report.CanonicalURL, baseURL+"/tools")
	}
	if report.SameArchetypeLinkCount < 100 {
		t.Fatalf("same archetype link count = %d, want at least 100", report.SameArchetypeLinkCount)
	}
	if got := report.TopArchetype(); got.Archetype != "tools_hub" || got.Confidence != "high" {
		t.Fatalf("top archetype = %+v, want high-confidence tools_hub", got)
	}
	for _, want := range []string{"free_tools_language", "sitemap_included", "many_same_archetype_links", "related_comparison_or_scheduler_pages"} {
		if !slices.Contains(report.Signals, want) {
			t.Fatalf("signals = %#v, want %q", report.Signals, want)
		}
	}
}

func TestEnrichSeedURLDetectsAlternativesAndComparisonClusters(t *testing.T) {
	var baseURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("content-type", "text/plain")
			_, _ = fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: %s/sitemap.xml\n", baseURL)
		case "/sitemap.xml":
			w.Header().Set("content-type", "application/xml")
			var sb strings.Builder
			sb.WriteString(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
			fmt.Fprintf(&sb, "<url><loc>%s/alternatives</loc><lastmod>2026-07-01</lastmod></url>", baseURL)
			fmt.Fprintf(&sb, "<url><loc>%s/alternatives/buffer</loc><lastmod>2026-07-01</lastmod></url>", baseURL)
			fmt.Fprintf(&sb, "<url><loc>%s/alternatives/hootsuite</loc><lastmod>2026-07-01</lastmod></url>", baseURL)
			fmt.Fprintf(&sb, "<url><loc>%s/compare/buffer</loc><lastmod>2026-07-01</lastmod></url>", baseURL)
			fmt.Fprintf(&sb, "<url><loc>%s/compare/hootsuite</loc><lastmod>2026-07-01</lastmod></url>", baseURL)
			sb.WriteString(`</urlset>`)
			_, _ = w.Write([]byte(sb.String()))
		case "/alternatives":
			w.Header().Set("content-type", "text/html; charset=utf-8")
			var sb strings.Builder
			sb.WriteString(`<html><head><title>Best Social Media Scheduler Alternatives</title>`)
			fmt.Fprintf(&sb, `<link rel="canonical" href="%s/alternatives">`, baseURL)
			sb.WriteString(`</head><body><h1>Best social media scheduler alternatives</h1>`)
			for _, name := range []string{"buffer", "hootsuite", "sprout-social"} {
				fmt.Fprintf(&sb, `<a href="/alternatives/%s">%s alternatives</a>`, name, name)
			}
			sb.WriteString(`</body></html>`)
			_, _ = w.Write([]byte(sb.String()))
		case "/compare/buffer":
			w.Header().Set("content-type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<html><head><title>PostSyncer vs Buffer Comparison</title><link rel="canonical" href="%s/compare/buffer"></head><body><h1>PostSyncer vs Buffer</h1><a href="/compare/hootsuite">Compare Hootsuite</a></body></html>`, baseURL)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	baseURL = srv.URL

	c := New(config.CrawlConfig{
		RequestTimeoutMs: 1000,
		RateLimitRPS:     1000,
		RespectRobots:    true,
		SitemapURLCap:    20,
		MaxPages:         5,
		MaxDepth:         1,
		SameOriginOnly:   true,
	}, slog.Default())

	alternatives, err := c.EnrichSeedURL(context.Background(), baseURL+"/alternatives")
	if err != nil {
		t.Fatalf("EnrichSeedURL alternatives error: %v", err)
	}
	if got := alternatives.TopArchetype(); got.Archetype != "alternatives_cluster" || got.Confidence != "high" {
		t.Fatalf("alternatives top archetype = %+v, want high-confidence alternatives_cluster", got)
	}
	if !slices.Contains(alternatives.Signals, "alternatives_language") || !slices.Contains(alternatives.Signals, "sitemap_included") {
		t.Fatalf("alternatives signals = %#v, want alternatives language and sitemap", alternatives.Signals)
	}

	comparison, err := c.EnrichSeedURL(context.Background(), baseURL+"/compare/buffer")
	if err != nil {
		t.Fatalf("EnrichSeedURL comparison error: %v", err)
	}
	if got := comparison.TopArchetype(); got.Archetype != "comparison_cluster" || got.Confidence != "high" {
		t.Fatalf("comparison top archetype = %+v, want high-confidence comparison_cluster", got)
	}
	if !slices.Contains(comparison.Signals, "comparison_language") || !slices.Contains(comparison.Signals, "sitemap_included") {
		t.Fatalf("comparison signals = %#v, want comparison language and sitemap", comparison.Signals)
	}
}

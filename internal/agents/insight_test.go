package agents

import (
	"testing"

	"github.com/citeloop/citeloop/internal/crawl"
)

func TestSummarizeCrawlCapturesOperatorRelevantFields(t *testing.T) {
	summary := summarizeCrawl("https://example.com", &crawl.Result{
		Landing:    &crawl.Page{URL: "https://example.com", Title: "Home"},
		Discovered: []string{"https://example.com/blog/a", "https://example.com/blog/b"},
		Articles: []*crawl.Page{
			{URL: "https://example.com/blog/a", Title: "A"},
			{URL: "https://example.com/blog/b", Title: "B"},
		},
		Truncated: true,
		Errors:    []string{"skip https://example.com/blog/c: timeout"},
	})
	summary.InventoryCount = 2

	if summary.LandingURL != "https://example.com" {
		t.Fatalf("landing url = %q", summary.LandingURL)
	}
	if summary.DiscoveredCount != 2 || summary.FetchedCount != 2 || summary.InventoryCount != 2 {
		t.Fatalf("counts = discovered %d fetched %d inventory %d", summary.DiscoveredCount, summary.FetchedCount, summary.InventoryCount)
	}
	if !summary.Truncated {
		t.Fatal("summary must expose truncated crawls")
	}
	if len(summary.Errors) != 1 {
		t.Fatalf("errors len = %d, want 1", len(summary.Errors))
	}
	if len(summary.SampleURLs) != 2 || summary.SampleURLs[0] != "https://example.com/blog/a" {
		t.Fatalf("sample urls = %#v", summary.SampleURLs)
	}
}

func TestProfileSourceURLsLandingOnlyForQuickProfile(t *testing.T) {
	urls := profileSourceURLs("https://example.com/input", &crawl.Result{
		Landing: &crawl.Page{URL: "https://example.com"},
		Articles: []*crawl.Page{
			{URL: "https://example.com/blog/a"},
			{URL: "https://example.com/blog/b"},
		},
	}, false)

	if len(urls) != 1 || urls[0] != "https://example.com" {
		t.Fatalf("quick profile source urls = %#v, want landing only", urls)
	}
}

func TestProfileSourceURLsIncludesArticlesForFullProfile(t *testing.T) {
	urls := profileSourceURLs("https://example.com/input", &crawl.Result{
		Landing: &crawl.Page{URL: "https://example.com"},
		Articles: []*crawl.Page{
			nil,
			{URL: "https://example.com/blog/a"},
			{URL: ""},
			{URL: "https://example.com/blog/b"},
		},
	}, true)

	want := []string{"https://example.com", "https://example.com/blog/a", "https://example.com/blog/b"}
	if len(urls) != len(want) {
		t.Fatalf("source urls = %#v, want %#v", urls, want)
	}
	for i := range want {
		if urls[i] != want[i] {
			t.Fatalf("source urls = %#v, want %#v", urls, want)
		}
	}
}

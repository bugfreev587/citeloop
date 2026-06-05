package crawl

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
)

// The sitemap_url_cap boundary must be enforced during collection (§5.1): a huge
// sitemap should not be fully decoded past the cap.
func TestSitemapCapBoundsFetch(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&sb, "<url><loc>http://x/blog/p%d</loc></url>", i)
	}
	sb.WriteString(`</urlset>`)
	body := sb.String()

	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New(config.CrawlConfig{SitemapURLCap: 100, RateLimitRPS: 1000}, slog.Default())
	entries, truncated := c.collectSitemap(context.Background(), []string{srv.URL + "/sitemap.xml"}, 100)
	if !truncated {
		t.Fatal("expected truncated=true when over the cap")
	}
	if len(entries) != 100 {
		t.Fatalf("expected exactly 100 bounded entries, got %d", len(entries))
	}
}

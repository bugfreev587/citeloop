package crawl

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
)

func TestFetchLandingSkipsDiscovery(t *testing.T) {
	var landingHits int64
	var discoveryHits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			atomic.AddInt64(&landingHits, 1)
			_, _ = w.Write([]byte(`<html><head><title>Home</title></head><body><main>Product facts.</main><a href="/blog/a">A</a></body></html>`))
		case "/sitemap.xml", "/blog/a":
			atomic.AddInt64(&discoveryHits, 1)
			_, _ = w.Write([]byte(`<html><body>slow page</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(config.CrawlConfig{RequestTimeoutMs: 1000, RateLimitRPS: 1000}, slog.Default())
	res, err := c.FetchLanding(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchLanding: %v", err)
	}

	if res.Landing == nil {
		t.Fatal("landing page was not returned")
	}
	if got := atomic.LoadInt64(&landingHits); got != 1 {
		t.Fatalf("landing hits = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&discoveryHits); got != 0 {
		t.Fatalf("discovery hits = %d, want 0", got)
	}
	if len(res.Discovered) != 0 || len(res.Articles) != 0 {
		t.Fatalf("landing-only result should not include discovered/articles: %#v", res)
	}
}

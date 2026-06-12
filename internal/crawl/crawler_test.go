package crawl

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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

func TestRunFetchesArticlePagesConcurrently(t *testing.T) {
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	fastFetched := make(chan struct{})
	var baseURL string
	var slowOnce atomic.Bool
	var fastOnce atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><head><title>Home</title></head><body><main>Product facts.</main></body></html>`))
		case "/sitemap.xml":
			w.Header().Set("content-type", "application/xml")
			_, _ = w.Write([]byte(`<urlset><url><loc>` + baseURL + `/blog/slow</loc></url><url><loc>` + baseURL + `/blog/fast</loc></url></urlset>`))
		case "/blog/slow":
			if slowOnce.CompareAndSwap(false, true) {
				close(slowStarted)
			}
			<-releaseSlow
			_, _ = w.Write([]byte(`<html><head><title>Slow</title></head><body><main>Slow article.</main></body></html>`))
		case "/blog/fast":
			if fastOnce.CompareAndSwap(false, true) {
				close(fastFetched)
			}
			_, _ = w.Write([]byte(`<html><head><title>Fast</title></head><body><main>Fast article.</main></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	baseURL = srv.URL

	c := New(config.CrawlConfig{
		RequestTimeoutMs: 1000,
		RateLimitRPS:     1000,
		MaxPages:         2,
		MaxDepth:         1,
		SitemapURLCap:    10,
	}, slog.Default())
	done := make(chan *Result, 1)
	errs := make(chan error, 1)
	go func() {
		res, err := c.Run(context.Background(), srv.URL)
		done <- res
		errs <- err
	}()

	select {
	case <-slowStarted:
	case <-time.After(500 * time.Millisecond):
		close(releaseSlow)
		t.Fatal("slow article fetch did not start")
	}

	select {
	case <-fastFetched:
	case <-time.After(500 * time.Millisecond):
		close(releaseSlow)
		t.Fatal("fast article fetch waited behind the slow article")
	}

	close(releaseSlow)
	res := <-done
	if err := <-errs; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Articles) != 2 {
		t.Fatalf("articles = %d, want 2", len(res.Articles))
	}
}

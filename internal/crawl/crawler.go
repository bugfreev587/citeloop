// Package crawl implements the conservative, same-origin blog discovery and
// fetch described in PRD §5.1. It is deliberately bounded (max pages/depth,
// rate limit, robots, sitemap cap) — general-site robustness is SaaS P0 and
// explicitly out of scope (§2).
package crawl

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/config"
)

const userAgent = "CiteLoopBot/0.1 (+https://citeloop.dev/bot)"

// Page is a fetched and extracted article page.
type Page struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Text      string    `json:"text"` // readable main content
	HTML      string    `json:"-"`
	FetchedAt time.Time `json:"fetched_at"`
}

// Result is the outcome of a discovery+fetch run.
type Result struct {
	Landing   *Page    `json:"landing"`
	Articles  []*Page  `json:"articles"`
	Discovered []string `json:"discovered"`  // normalized article URLs found
	Truncated bool     `json:"truncated"`    // hit sitemap_url_cap (§5.1)
	Errors    []string `json:"errors"`       // per-page failures (non-fatal)
}

type Crawler struct {
	cfg    config.CrawlConfig
	client *http.Client
	limiter <-chan time.Time
	log    *slog.Logger
	now    func() time.Time
}

func New(cfg config.CrawlConfig, log *slog.Logger) *Crawler {
	if log == nil {
		log = slog.Default()
	}
	rps := cfg.RateLimitRPS
	if rps <= 0 {
		rps = 1
	}
	return &Crawler{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeoutMs) * time.Millisecond,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		limiter: time.Tick(time.Second / time.Duration(rps)),
		log:     log,
		now:     time.Now,
	}
}

// Run discovers articles for the given landing URL and fetches them, honoring
// all crawl boundaries. The landing page itself failing is fatal (§5.1); an
// individual article failing is logged and skipped.
func (c *Crawler) Run(ctx context.Context, landingURL string) (*Result, error) {
	base, err := url.Parse(landingURL)
	if err != nil {
		return nil, fmt.Errorf("invalid landing url: %w", err)
	}

	var rb *robots
	if c.cfg.RespectRobots {
		rb = fetchRobots(ctx, c.client, base)
	}

	landing, err := c.fetch(ctx, base, rb)
	if err != nil {
		return nil, fmt.Errorf("landing fetch failed (fatal): %w", err)
	}

	res := &Result{Landing: landing}

	urls, truncated := c.discover(ctx, base, rb)
	res.Discovered = urls
	res.Truncated = truncated

	for _, u := range urls {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		pu, err := url.Parse(u)
		if err != nil {
			continue
		}
		p, err := c.fetch(ctx, pu, rb)
		if err != nil {
			msg := fmt.Sprintf("skip %s: %v", u, err)
			c.log.Warn("article fetch failed", "url", u, "err", err)
			res.Errors = append(res.Errors, msg)
			continue
		}
		res.Articles = append(res.Articles, p)
	}
	return res, nil
}

// discover returns normalized, deduped, article-classified URLs within bounds.
// Strategy: robots→sitemap first; fall back to BFS over index paths (§5.1).
func (c *Crawler) discover(ctx context.Context, base *url.URL, rb *robots) ([]string, bool) {
	seen := map[string]bool{}
	var out []string
	truncated := false

	add := func(raw string) {
		n, err := Normalize(raw)
		if err != nil {
			return
		}
		u, err := url.Parse(n)
		if err != nil {
			return
		}
		if c.cfg.SameOriginOnly && !SameOrigin(base, u) {
			return
		}
		if rb != nil && !rb.allowed(u.Path) {
			return
		}
		if !looksLikeArticle(u) {
			return
		}
		if seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}

	// 1) sitemaps (from robots, else conventional location)
	sitemaps := []string{}
	if rb != nil {
		sitemaps = append(sitemaps, rb.sitemaps...)
	}
	if len(sitemaps) == 0 {
		su := *base
		su.Path = "/sitemap.xml"
		su.RawQuery = ""
		sitemaps = append(sitemaps, su.String())
	}
	// Bound collection at the cap so we never fetch/decode an unbounded sitemap.
	entries, capped := c.collectSitemap(ctx, sitemaps, c.cfg.SitemapURLCap)
	if capped {
		// Over the cap: take the most-recently-modified first within the bounded
		// set (§5.1). Top-N is approximate by design, since we stop fetching once
		// the cap is reached rather than reading every entry.
		sort.Slice(entries, func(i, j int) bool { return entries[i].lastmod > entries[j].lastmod })
		if len(entries) > c.cfg.SitemapURLCap {
			entries = entries[:c.cfg.SitemapURLCap]
		}
		truncated = true
	}
	for _, e := range entries {
		add(e.loc)
	}

	// 2) fallback BFS over listing roots when sitemap yielded little
	if len(out) < 3 {
		c.bfsDiscover(ctx, base, rb, add)
	}

	if len(out) > c.cfg.MaxPages {
		out = out[:c.cfg.MaxPages]
		truncated = true
	}
	return out, truncated
}

// bfsDiscover walks listing pages following same-origin links up to max_depth /
// max_pages, feeding article-looking URLs into add().
func (c *Crawler) bfsDiscover(ctx context.Context, base *url.URL, rb *robots, add func(string)) {
	type node struct {
		u     string
		depth int
	}
	visited := map[string]bool{}
	var queue []node
	for _, p := range indexPaths {
		su := *base
		su.Path = p
		su.RawQuery = ""
		queue = append(queue, node{su.String(), 0})
	}

	pages := 0
	for len(queue) > 0 && pages < c.cfg.MaxPages {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n := queue[0]
		queue = queue[1:]
		nn, err := Normalize(n.u)
		if err != nil || visited[nn] {
			continue
		}
		visited[nn] = true
		nu, _ := url.Parse(nn)
		if c.cfg.SameOriginOnly && !SameOrigin(base, nu) {
			continue
		}
		if rb != nil && !rb.allowed(nu.Path) {
			continue
		}
		links, ok := c.extractLinks(ctx, nu)
		pages++
		if !ok {
			continue
		}
		for _, l := range links {
			add(l) // article candidates
			if n.depth < c.cfg.MaxDepth {
				lu, err := url.Parse(l)
				if err == nil && (c.isListing(lu.Path)) {
					queue = append(queue, node{l, n.depth + 1})
				}
			}
		}
	}
}

func (c *Crawler) isListing(path string) bool {
	for _, idx := range indexPaths {
		if strings.HasPrefix(strings.ToLower(path), idx) {
			return true
		}
	}
	for _, re := range nonArticlePatterns {
		if re.MatchString(path) {
			return true // pagination pages are listings worth following
		}
	}
	return false
}

func (c *Crawler) wait() {
	if c.limiter != nil {
		<-c.limiter
	}
}

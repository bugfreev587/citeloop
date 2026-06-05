package crawl

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"strings"

	readability "github.com/go-shiori/go-readability"
	"golang.org/x/net/html"
)

const maxBodyBytes = 5 << 20 // 5MB cap per page

// fetch retrieves a page, follows <link rel=canonical> for URL identity, and
// extracts readable main content (§5.1 step 4).
func (c *Crawler) fetch(ctx context.Context, u *url.URL, rb *robots) (*Page, error) {
	if rb != nil && !rb.allowed(u.Path) {
		return nil, errDisallowed
	}
	c.wait()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, &httpError{resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}
	htmlStr := string(body)

	// Follow rel=canonical for the page's identity URL (§5.1).
	finalURL := resp.Request.URL.String()
	if canon := canonicalLink(htmlStr, resp.Request.URL); canon != "" {
		finalURL = canon
	}
	normURL, err := Normalize(finalURL)
	if err != nil {
		normURL = finalURL
	}

	article, rerr := readability.FromReader(strings.NewReader(htmlStr), resp.Request.URL)
	page := &Page{URL: normURL, HTML: htmlStr, FetchedAt: c.now()}
	if rerr == nil {
		page.Title = article.Title
		page.Text = strings.TrimSpace(article.TextContent)
	} else {
		page.Title = extractTitle(htmlStr)
		page.Text = stripTags(htmlStr)
	}
	return page, nil
}

// extractLinks fetches a listing page and returns absolute, same-host links.
func (c *Crawler) extractLinks(ctx context.Context, u *url.URL) ([]string, bool) {
	c.wait()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, false
	}
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, false
	}
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					if abs := resolve(resp.Request.URL, a.Val); abs != "" {
						out = append(out, abs)
					}
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return out, true
}

func resolve(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "javascript:") {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

// --- sitemap parsing (recursive sitemap index, §5.1) ---

type sitemapEntry struct {
	loc     string
	lastmod string
}

type xmlURLSet struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
	} `xml:"url"`
}

type xmlSitemapIndex struct {
	XMLName  xml.Name `xml:"sitemapindex"`
	Sitemaps []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
	} `xml:"sitemap"`
}

// collectSitemap walks sitemap indexes and url sets, stopping once `cap` entries
// have been collected so a huge sitemap is not fully fetched/decoded — the
// boundary is enforced during collection, not after (§5.1). When the cap is hit
// it returns truncated=true; the caller then takes the most-recent N by lastmod
// from the bounded set.
func (c *Crawler) collectSitemap(ctx context.Context, roots []string, cap int) (entries []sitemapEntry, truncated bool) {
	seen := map[string]bool{}
	queue := append([]string{}, roots...)
	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return entries, truncated
		default:
		}
		if cap > 0 && len(entries) >= cap {
			return entries, true // stop fetching further sitemap documents
		}
		s := queue[0]
		queue = queue[1:]
		if seen[s] {
			continue
		}
		seen[s] = true
		body := c.getBody(ctx, s)
		if body == nil {
			continue
		}
		var idx xmlSitemapIndex
		if err := xml.Unmarshal(body, &idx); err == nil && len(idx.Sitemaps) > 0 {
			for _, sm := range idx.Sitemaps {
				queue = append(queue, sm.Loc)
			}
			continue
		}
		var set xmlURLSet
		if err := xml.Unmarshal(body, &set); err == nil {
			for _, u := range set.URLs {
				entries = append(entries, sitemapEntry{loc: u.Loc, lastmod: u.LastMod})
				if cap > 0 && len(entries) >= cap {
					return entries, true
				}
			}
		}
	}
	return entries, truncated
}

func (c *Crawler) getBody(ctx context.Context, raw string) []byte {
	c.wait()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	return body
}

// --- small HTML helpers ---

func canonicalLink(htmlStr string, base *url.URL) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return ""
	}
	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "link" {
			var rel, href string
			for _, a := range n.Attr {
				switch a.Key {
				case "rel":
					rel = a.Val
				case "href":
					href = a.Val
				}
			}
			if strings.EqualFold(rel, "canonical") && href != "" {
				found = resolve(base, href)
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return found
}

func extractTitle(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return ""
	}
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if title != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
			title = strings.TrimSpace(n.FirstChild.Data)
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return title
}

func stripTags(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return ""
	}
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
			sb.WriteString(" ")
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return strings.Join(strings.Fields(sb.String()), " ")
}

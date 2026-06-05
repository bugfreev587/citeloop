package crawl

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"strings"
)

// robots is a minimal robots.txt model: the set of Disallow prefixes that apply
// to our user-agent (* group) plus any declared sitemaps. Good enough for the
// conservative same-origin crawl in §5.1; full RFC 9309 robustness is SaaS P0.
type robots struct {
	disallow []string
	sitemaps []string
}

func fetchRobots(ctx context.Context, c *http.Client, base *url.URL) *robots {
	r := &robots{}
	ru := *base
	ru.Path = "/robots.txt"
	ru.RawQuery = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ru.String(), nil)
	if err != nil {
		return r
	}
	resp, err := c.Do(req)
	if err != nil {
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return r
	}

	var appliesToUs bool
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := splitDirective(line)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "user-agent":
			appliesToUs = val == "*"
		case "disallow":
			if appliesToUs && val != "" {
				r.disallow = append(r.disallow, val)
			}
		case "sitemap":
			r.sitemaps = append(r.sitemaps, val)
		}
	}
	return r
}

func splitDirective(line string) (key, val string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}

// allowed reports whether path may be fetched given the disallow rules.
func (r *robots) allowed(path string) bool {
	if r == nil {
		return true
	}
	for _, d := range r.disallow {
		if d == "/" {
			return false
		}
		if strings.HasPrefix(path, d) {
			return false
		}
	}
	return true
}

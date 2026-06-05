package crawl

import (
	"net/url"
	"regexp"
	"strings"
)

// Normalize implements the URL normalization rules from PRD §5.1:
//   - lowercase host
//   - drop fragment
//   - drop tracking query params (keep meaningful ones)
//   - strip trailing slash (except root)
//
// Following <link rel=canonical> is handled at fetch time (see crawler.go),
// not here, because it requires the page body.
func Normalize(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawQuery = filterQuery(u.RawQuery)
	if u.Path != "/" {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

// trackingParams are dropped during normalization; everything else is kept
// because some blogs use ?p=123 style meaningful params.
var trackingParams = map[string]bool{
	"utm_source": true, "utm_medium": true, "utm_campaign": true,
	"utm_term": true, "utm_content": true, "gclid": true,
	"fbclid": true, "ref": true, "source": true,
}

func filterQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	vals, err := url.ParseQuery(rawQuery)
	if err != nil {
		return ""
	}
	for k := range vals {
		if trackingParams[strings.ToLower(k)] {
			vals.Del(k)
		}
	}
	return vals.Encode()
}

// SameOrigin reports whether candidate shares scheme+host with base.
func SameOrigin(base, candidate *url.URL) bool {
	return strings.EqualFold(base.Host, candidate.Host)
}

// nonArticlePatterns matches index/listing/taxonomy pages that §5.1 says to
// exclude from the "all articles" set (tag/author/pagination).
var nonArticlePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)/tag/`),
	regexp.MustCompile(`(?i)/tags/`),
	regexp.MustCompile(`(?i)/category/`),
	regexp.MustCompile(`(?i)/categories/`),
	regexp.MustCompile(`(?i)/author/`),
	regexp.MustCompile(`(?i)/authors/`),
	regexp.MustCompile(`(?i)/page/\d+`),
	regexp.MustCompile(`(?i)[?&]page=\d+`),
	regexp.MustCompile(`(?i)/feed/?$`),
	regexp.MustCompile(`(?i)\.(xml|rss|json|atom)$`),
}

// indexPaths are listing roots used as crawl seeds when no sitemap exists (§5.1).
var indexPaths = []string{"/blog", "/posts", "/articles", "/changelog", "/news", "/resources"}

// looksLikeArticle is a heuristic for the "is this a content article" decision.
// Listing/taxonomy URLs are rejected; a path with real depth is accepted.
func looksLikeArticle(u *url.URL) bool {
	p := u.Path
	for _, re := range nonArticlePatterns {
		if re.MatchString(u.String()) {
			return false
		}
	}
	// bare listing roots are not articles
	for _, idx := range indexPaths {
		if strings.EqualFold(strings.TrimRight(p, "/"), idx) {
			return false
		}
	}
	// must have a slug-bearing path segment
	segs := strings.FieldsFunc(p, func(r rune) bool { return r == '/' })
	return len(segs) >= 1 && len(p) > 1
}

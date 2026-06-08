// Package seo implements the SEO Operations Loop data contracts and workflow.
package seo

import (
	"net/url"
	"path"
	"sort"
	"strings"
)

// URLNormalizationConfig mirrors seo_properties.url_normalization_config.
type URLNormalizationConfig struct {
	KeepQueryKeys []string `json:"keep_query_keys,omitempty"`
	LowercasePath bool     `json:"lowercase_path,omitempty"`
	PreserveHTTP  bool     `json:"preserve_http,omitempty"`
}

// NormalizeURL canonicalizes URLs before joining GSC, GA4, crawl, and article data.
func NormalizeURL(rawURL, siteURL string, cfg URLNormalizationConfig) (string, error) {
	base, err := url.Parse(strings.TrimSpace(siteURL))
	if err != nil {
		return "", err
	}
	if base.Scheme == "" {
		base.Scheme = "https"
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	if !u.IsAbs() {
		u = base.ResolveReference(u)
	}
	if u.Scheme == "" {
		u.Scheme = base.Scheme
	}
	if u.Host == "" {
		u.Host = base.Host
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if !cfg.PreserveHTTP && sameHost(u.Host, base.Host) {
		u.Scheme = "https"
	}
	u.Fragment = ""
	u.RawQuery = normalizedQuery(u.Query(), cfg.KeepQueryKeys)

	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}
	p = "/" + strings.TrimPrefix(path.Clean(strings.ReplaceAll(p, "//", "/")), "/")
	if cfg.LowercasePath {
		p = strings.ToLower(p)
	}
	if p != "/" {
		p = strings.TrimRight(p, "/")
		if p == "" {
			p = "/"
		}
	}
	u.Path = p
	u.RawPath = ""
	return u.String(), nil
}

func sameHost(a, b string) bool {
	return strings.EqualFold(strings.TrimPrefix(a, "www."), strings.TrimPrefix(b, "www."))
}

func normalizedQuery(values url.Values, keep []string) string {
	if len(values) == 0 || len(keep) == 0 {
		return ""
	}
	allowed := make(map[string]struct{}, len(keep))
	for _, key := range keep {
		allowed[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}
	filtered := url.Values{}
	for key, vals := range values {
		if _, ok := allowed[strings.ToLower(key)]; !ok {
			continue
		}
		sort.Strings(vals)
		for _, value := range vals {
			filtered.Add(key, value)
		}
	}
	return filtered.Encode()
}

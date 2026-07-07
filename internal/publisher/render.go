package publisher

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/markdownutil"
)

type seoMeta struct {
	Title           string `json:"title"`
	MetaDescription string `json:"meta_description"`
	Slug            string `json:"slug"`
	H1              string `json:"h1"`
	CanonicalURL    string `json:"canonical_url"`
}

func parseSEO(a *db.Article) seoMeta {
	var m seoMeta
	_ = json.Unmarshal(a.SeoMeta, &m)
	return m
}

func title(a *db.Article) string {
	m := parseSEO(a)
	if m.Title != "" {
		return m.Title
	}
	if m.H1 != "" {
		return m.H1
	}
	return "Untitled"
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)
var scriptTagRe = regexp.MustCompile(`(?i)<\s*script\b`)
var mdxImportRe = regexp.MustCompile(`(?m)^\s*import\s+`)
var htmlEventHandlerRe = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)

const maxBlogSlugLength = 96

func slugOf(a *db.Article) string {
	if a.ResolvedSlug != nil && strings.TrimSpace(*a.ResolvedSlug) != "" {
		return NormalizeBlogSlug(*a.ResolvedSlug)
	}
	m := parseSEO(a)
	s := m.Slug
	if s == "" {
		s = strings.ToLower(title(a))
	}
	s = NormalizeBlogSlug(s)
	if s == "" {
		s = "post-" + a.ID.String()[:8]
	}
	return s
}

func NormalizeBlogSlug(s string) string {
	s = slugRe.ReplaceAllString(strings.ToLower(s), "-")
	s = strings.Trim(s, "-")
	if len(s) > maxBlogSlugLength {
		s = strings.Trim(s[:maxBlogSlugLength], "-")
	}
	return s
}

// renderMDX builds the MDX file with frontmatter for the canonical blog post.
func renderMDX(a *db.Article, slug, publicURL string, now time.Time) ([]byte, error) {
	content := markdownutil.NormalizeGeneratedEscapes(a.ContentMd)
	if err := validateGeneratedMDX(content); err != nil {
		return nil, err
	}
	m := parseSEO(a)
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("source: citeloop\n")
	fmt.Fprintf(&b, "citeloop_article_id: %q\n", a.ID.String())
	fmt.Fprintf(&b, "citeloop_topic_id: %q\n", a.TopicID.String())
	fmt.Fprintf(&b, "slug: %q\n", slug)
	fmt.Fprintf(&b, "title: %q\n", title(a))
	fmt.Fprintf(&b, "seo_title: %q\n", title(a))
	fmt.Fprintf(&b, "description: %q\n", m.MetaDescription)
	fmt.Fprintf(&b, "excerpt: %q\n", m.MetaDescription)
	fmt.Fprintf(&b, "published_at: %q\n", now.Format("2006-01-02"))
	fmt.Fprintf(&b, "updated_at: %q\n", now.Format("2006-01-02"))
	b.WriteString("author: \"UniPost\"\n")
	b.WriteString("category: \"Engineering\"\n")
	b.WriteString("keywords: []\n")
	fmt.Fprintf(&b, "canonical: %q\n", publicURL)
	b.WriteString("---\n\n")
	b.WriteString(content)
	b.WriteString("\n")
	return []byte(b.String()), nil
}

func validateGeneratedMDX(content string) error {
	switch {
	case scriptTagRe.MatchString(content):
		return fmt.Errorf("unsafe generated mdx: script tag is not allowed")
	case mdxImportRe.MatchString(content):
		return fmt.Errorf("unsafe generated mdx: import is not allowed")
	case htmlEventHandlerRe.MatchString(content):
		return fmt.Errorf("unsafe generated mdx: html event handler is not allowed")
	default:
		return nil
	}
}

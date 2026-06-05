package publisher

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
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

func slugOf(a *db.Article) string {
	m := parseSEO(a)
	s := m.Slug
	if s == "" {
		s = strings.ToLower(title(a))
	}
	s = slugRe.ReplaceAllString(strings.ToLower(s), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "post-" + a.ID.String()[:8]
	}
	return s
}

// renderMDX builds the MDX file with frontmatter for the canonical blog post.
func renderMDX(a *db.Article, slug, publicURL string, now time.Time) []byte {
	m := parseSEO(a)
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %q\n", title(a))
	fmt.Fprintf(&b, "description: %q\n", m.MetaDescription)
	fmt.Fprintf(&b, "slug: %q\n", slug)
	fmt.Fprintf(&b, "canonical: %q\n", publicURL)
	fmt.Fprintf(&b, "date: %q\n", now.Format(time.RFC3339))
	b.WriteString("---\n\n")
	b.WriteString(a.ContentMd)
	b.WriteString("\n")
	return []byte(b.String())
}

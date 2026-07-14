package publisher

import (
	"context"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/platform"
)

// CanonicalPlaceholder mirrors the Writer's token; backfilled at distribution
// unlock (§5.6). Kept in sync with internal/agents.canonicalPlaceholder.
const CanonicalPlaceholder = "{{CANONICAL_URL}}"

// composeURLs maps a platform to its "write a new post" page (§5.6 UI step).
var composeURLs = map[string]string{
	"dev_to":      "https://dev.to/new",
	"hashnode":    "https://hashnode.com/draft",
	"medium":      "https://medium.com/new-story",
	"linkedin":    "https://www.linkedin.com/article/new/",
	"reddit":      "https://www.reddit.com/submit?type=TEXT",
	"hacker_news": "https://news.ycombinator.com/submit",
}

// SemiManualPublisher is the V1 syndication lane (§5.6). It never auto-posts; it
// only produces a distribution-ready variant (canonical backfilled) and a
// compose link for the human to paste into.
type SemiManualPublisher struct {
	plat string
}

func NewSemiManual(plat string) *SemiManualPublisher { return &SemiManualPublisher{plat: plat} }

func (s *SemiManualPublisher) Platform() string { return s.plat }
func (s *SemiManualPublisher) Mode() Mode       { return SemiManual }
func (s *SemiManualPublisher) SupportsCanonical() bool {
	return platform.SupportsCanonical(platform.Platform(s.plat))
}

// Publish for the semi-manual lane returns the compose URL; the actual content
// backfill is done by RewriteForDistribution before this is surfaced in the UI.
func (s *SemiManualPublisher) Publish(_ context.Context, _ *db.Article) (Result, error) {
	compose := composeURLs[s.plat]
	return Result{
		Mode:       SemiManual,
		Detail:     "ready to distribute — copy variant and post manually",
		Distribute: compose,
	}, nil
}

// ComposeURL returns the "write a new post" page for a platform (§5.6 UI step),
// or "" if unknown.
func ComposeURL(plat string) string { return composeURLs[plat] }

// RewriteForDistribution backfills the real canonical URL into a variant's body
// at unlock time. Platforms supporting rel=canonical also get the value via
// canonical_url (handled by the caller); forum platforms only get the source
// link substituted in-body (§5.3/§5.6).
func RewriteForDistribution(content, realURL string) string {
	return strings.ReplaceAll(content, CanonicalPlaceholder, realURL)
}

// Package platform is the registry of syndication platforms and their
// capabilities. The key distinction (PRD §4 correction / §5.3 / §5.6):
// Medium/Dev.to/Hashnode/LinkedIn support rel=canonical; forum/aggregator
// platforms (Reddit/Hacker News) do NOT — they get a source link in the body.
package platform

// Platform identifies a syndication target.
type Platform string

const (
	Blog     Platform = "blog"
	DevTo    Platform = "dev_to"
	Hashnode Platform = "hashnode"
	Medium   Platform = "medium"
	LinkedIn Platform = "linkedin"
	Reddit   Platform = "reddit"
	HN       Platform = "hacker_news"
)

// supportsCanonical maps each platform to whether it honors rel=canonical.
var supportsCanonical = map[Platform]bool{
	Medium:   true,
	DevTo:    true,
	Hashnode: true,
	LinkedIn: true,
	Reddit:   false,
	HN:       false,
}

// SupportsCanonical reports whether p honors a rel=canonical tag.
func SupportsCanonical(p Platform) bool {
	return supportsCanonical[p]
}

// SyndicationTargets is the default V1 set of platforms a syndication topic is
// rewritten for. Kept small and conservative for the MVP.
var SyndicationTargets = []Platform{DevTo, Hashnode, Reddit}

func (p Platform) String() string { return string(p) }

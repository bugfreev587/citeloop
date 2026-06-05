// Package publisher implements the Publisher abstraction (PRD §4) and the two
// V1 lanes (§5.6): BlogPublisher (Auto — auto-commit MDX,真自动) and
// SemiManualPublisher (variants gated behind canonical publish).
package publisher

import (
	"context"

	"github.com/citeloop/citeloop/internal/db"
)

// Mode is the publish lane (PRD §4).
type Mode string

const (
	Auto       Mode = "auto"
	SemiManual Mode = "semi_manual"
)

// Result is returned by Publish and stored in articles.publish_result (§5.6).
type Result struct {
	URL        string `json:"url"` // real published URL (becomes canonical_url)
	Mode       Mode   `json:"mode"`
	Detail     string `json:"detail"`
	Path       string `json:"path,omitempty"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	Phase      string `json:"phase,omitempty"`
	DeployHook string `json:"deploy_hook,omitempty"`
	Distribute string `json:"distribute,omitempty"` // for semi-manual: target compose URL
}

// Publisher is the PRD §4 interface.
type Publisher interface {
	Platform() string
	Mode() Mode
	SupportsCanonical() bool
	Publish(ctx context.Context, a *db.Article) (Result, error)
}

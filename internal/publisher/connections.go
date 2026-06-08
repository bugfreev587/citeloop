package publisher

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
)

const (
	ConnectionKindGitHubNextJS = "github_nextjs"

	CredentialKindGitHubToken = "github_token"

	CapabilityCreateArticle  = "create_article"
	CapabilityUpdateArticle  = "update_article"
	CapabilityMetadataUpdate = "metadata_update"
	CapabilityCanonical      = "canonical"
	CapabilityMediaUpload    = "media_upload"
	CapabilityDraftMode      = "draft_mode"
	CapabilityPublishMode    = "publish_mode"
	CapabilityDelete         = "delete"
	CapabilityRollback       = "rollback"
)

const publisherCredentialPrefix = "publisher_credential:"

type Capabilities map[string]bool

type GitHubNextJSConfig struct {
	Repo        string `json:"repo"`
	Branch      string `json:"branch"`
	ContentDir  string `json:"content_dir"`
	BaseURL     string `json:"base_url"`
	PublishMode string `json:"publish_mode"`
}

func GitHubNextJSCapabilities() Capabilities {
	return Capabilities{
		CapabilityCreateArticle:  true,
		CapabilityUpdateArticle:  true,
		CapabilityMetadataUpdate: true,
		CapabilityCanonical:      true,
		CapabilityMediaUpload:    false,
		CapabilityDraftMode:      false,
		CapabilityPublishMode:    true,
		CapabilityDelete:         false,
		CapabilityRollback:       false,
	}
}

func ParseGitHubNextJSConfig(raw json.RawMessage) (GitHubNextJSConfig, error) {
	var cfg GitHubNextJSConfig
	if len(raw) == 0 {
		return cfg, errors.New("publisher config is empty")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	cfg.Repo = strings.TrimSpace(cfg.Repo)
	cfg.Branch = strings.TrimSpace(cfg.Branch)
	cfg.ContentDir = strings.TrimSpace(cfg.ContentDir)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.PublishMode = strings.TrimSpace(cfg.PublishMode)
	if cfg.Branch == "" {
		cfg.Branch = "citeloop-content"
	}
	if cfg.ContentDir == "" {
		cfg.ContentDir = "content/citeloop/blog"
	}
	if cfg.PublishMode == "" {
		cfg.PublishMode = "publish"
	}
	if cfg.Repo == "" || cfg.BaseURL == "" {
		return cfg, errors.New("repo and base_url are required")
	}
	return cfg, nil
}

func (c Capabilities) JSON() json.RawMessage {
	b, _ := json.Marshal(c)
	return b
}

func PublisherCredentialRef(id uuid.UUID) string {
	return publisherCredentialPrefix + id.String()
}

func ParsePublisherCredentialRef(ref string) (uuid.UUID, bool) {
	trimmed := strings.TrimSpace(ref)
	if !strings.HasPrefix(trimmed, publisherCredentialPrefix) {
		return uuid.UUID{}, false
	}
	id, err := uuid.Parse(strings.TrimPrefix(trimmed, publisherCredentialPrefix))
	if err != nil {
		return uuid.UUID{}, false
	}
	return id, true
}

func RedactCredentialValue(kind, value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 4 {
		return "****"
	}
	tail := trimmed[len(trimmed)-4:]
	switch kind {
	case CredentialKindGitHubToken:
		return "gh_****" + tail
	default:
		return "****" + tail
	}
}

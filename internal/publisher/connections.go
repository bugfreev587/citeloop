package publisher

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

const (
	ConnectionKindGitHubNextJS = "github_nextjs"
	ConnectionKindDevTo        = "dev_to"

	CredentialKindGitHubToken = "github_token"
	CredentialKindDevToAPIKey = "dev_to_api_key"

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

type GitHubNextJSTarget struct {
	Branch  string
	BaseURL string
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

func DevToCapabilities() Capabilities {
	return Capabilities{
		CapabilityCreateArticle:  true,
		CapabilityUpdateArticle:  false,
		CapabilityMetadataUpdate: true,
		CapabilityCanonical:      true,
		CapabilityMediaUpload:    false,
		CapabilityDraftMode:      true,
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
	cfg.BaseURL = normalizePublicBaseURL(cfg.BaseURL)
	cfg.PublishMode = strings.TrimSpace(cfg.PublishMode)
	if cfg.Branch == "" {
		if target, ok := GitHubNextJSTargetForBaseURL(cfg.BaseURL); ok {
			cfg.Branch = target.Branch
		} else {
			cfg.Branch = "citeloop-content"
		}
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

func GitHubNextJSTargetForSiteURL(raw string) (GitHubNextJSTarget, bool) {
	_, host, ok := parseURLHost(raw)
	if !ok {
		return GitHubNextJSTarget{}, false
	}
	branch, ok := unipostBranchForHost(host)
	if !ok {
		return GitHubNextJSTarget{}, false
	}
	return GitHubNextJSTarget{
		Branch:  branch,
		BaseURL: "https://" + host + "/blog",
	}, true
}

func GitHubNextJSTargetForBaseURL(raw string) (GitHubNextJSTarget, bool) {
	trimmed := normalizePublicBaseURL(raw)
	if trimmed == "" {
		return GitHubNextJSTarget{}, false
	}
	_, host, ok := parseURLHost(trimmed)
	if !ok {
		return GitHubNextJSTarget{}, false
	}
	branch, ok := unipostBranchForHost(host)
	if !ok {
		return GitHubNextJSTarget{}, false
	}
	return GitHubNextJSTarget{Branch: branch, BaseURL: trimmed}, true
}

func normalizePublicBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func parseURLHost(raw string) (*url.URL, string, bool) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return nil, "", false
	}
	candidate := trimmed
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Hostname() == "" {
		return nil, "", false
	}
	return parsed, strings.ToLower(parsed.Hostname()), true
}

func unipostBranchForHost(host string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "dev.unipost.dev":
		return "dev", true
	case "staging.unipost.dev":
		return "staging", true
	case "unipost.dev":
		return "main", true
	default:
		return "", false
	}
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

func IsEnvPublisherCredentialRef(ref string) bool {
	switch strings.ToUpper(strings.TrimSpace(ref)) {
	case "ENV:GITHUB_TOKEN", "GITHUB_TOKEN":
		return true
	default:
		return false
	}
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
	case CredentialKindDevToAPIKey:
		return "devto_****" + tail
	default:
		return "****" + tail
	}
}

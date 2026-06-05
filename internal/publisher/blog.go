package publisher

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
)

// BlogPublisher implements §8 option A: write MDX into the blog repo and
// auto-commit to a publish branch via the GitHub Contents API —真自动, no human
// merge. The app-internal approve is the only human gate (§1.5/§5.6).
//
// When the repo/token are not configured it runs in DryRun: it computes the
// real URL and logs, so the end-to-end pipeline is demonstrable without a live
// repo. DryRun is explicit in the result Detail so a real deploy never silently
// no-ops.
type BlogPublisher struct {
	Token   string
	Repo    string // "owner/name"
	Branch  string
	BaseURL string
	Log     *slog.Logger
	client  *http.Client
	now     func() time.Time
}

func NewBlog(token, repo, branch, baseURL string, log *slog.Logger) *BlogPublisher {
	if log == nil {
		log = slog.Default()
	}
	return &BlogPublisher{
		Token: token, Repo: repo, Branch: branch, BaseURL: strings.TrimRight(baseURL, "/"),
		Log: log, client: &http.Client{Timeout: 30 * time.Second}, now: time.Now,
	}
}

func (b *BlogPublisher) Platform() string        { return "blog" }
func (b *BlogPublisher) Mode() Mode              { return Auto }
func (b *BlogPublisher) SupportsCanonical() bool { return true }

func (b *BlogPublisher) dryRun() bool { return b.Token == "" || b.Repo == "" }

// Publish writes the article as MDX and commits it. Returns the real public URL
// to be backfilled as canonical_url (§5.6).
func (b *BlogPublisher) Publish(ctx context.Context, a *db.Article) (Result, error) {
	slug := slugOf(a)
	publicURL := b.BaseURL + "/" + slug
	mdx := renderMDX(a, slug, publicURL, b.now())
	path := "content/blog/" + slug + ".mdx"

	if b.dryRun() {
		b.Log.Warn("BlogPublisher dry-run (no repo/token configured)", "path", path, "url", publicURL)
		return Result{URL: publicURL, Mode: Auto, Detail: "dry-run: not committed (configure GITHUB_TOKEN + BLOG_REPO)"}, nil
	}
	if err := b.commitFile(ctx, path, mdx, "Publish: "+title(a)); err != nil {
		return Result{}, fmt.Errorf("commit mdx: %w", err)
	}
	b.Log.Info("blog published", "path", path, "url", publicURL, "branch", b.Branch)
	return Result{URL: publicURL, Mode: Auto, Detail: "committed to " + b.Repo + "@" + b.Branch}, nil
}

// commitFile creates or updates path on the publish branch via the Contents API.
func (b *BlogPublisher) commitFile(ctx context.Context, path string, content []byte, msg string) error {
	api := "https://api.github.com/repos/" + b.Repo + "/contents/" + path

	// Look up existing sha (update vs create) on the branch.
	sha := b.existingSHA(ctx, api)

	payload := map[string]any{
		"message": msg,
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  b.Branch,
	}
	if sha != "" {
		payload["sha"] = sha
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, api, bytes.NewReader(body))
	if err != nil {
		return err
	}
	b.auth(req)
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

func (b *BlogPublisher) existingSHA(ctx context.Context, api string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"?ref="+b.Branch, nil)
	if err != nil {
		return ""
	}
	b.auth(req)
	resp, err := b.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var meta struct {
		SHA string `json:"sha"`
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &meta)
	return meta.SHA
}

func (b *BlogPublisher) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+b.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

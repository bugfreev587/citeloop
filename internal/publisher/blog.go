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
	"net/url"
	"path"
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
	Token      string
	Repo       string // "owner/name"
	Branch     string
	ContentDir string
	BaseURL    string
	Log        *slog.Logger
	client     *http.Client
	now        func() time.Time
}

func NewBlog(token, repo, branch, baseURL, contentDir string, log *slog.Logger) *BlogPublisher {
	if log == nil {
		log = slog.Default()
	}
	if contentDir == "" {
		contentDir = "content/citeloop/blog"
	}
	return &BlogPublisher{
		Token: token, Repo: repo, Branch: branch, ContentDir: contentDir, BaseURL: strings.TrimRight(baseURL, "/"),
		Log: log, client: &http.Client{Timeout: 30 * time.Second}, now: time.Now,
	}
}

func (b *BlogPublisher) Platform() string        { return "blog" }
func (b *BlogPublisher) Mode() Mode              { return Auto }
func (b *BlogPublisher) SupportsCanonical() bool { return true }

func (b *BlogPublisher) dryRun() bool { return b.Token == "" || b.Repo == "" }

func (b *BlogPublisher) Resolve(a *db.Article) (slug, publishPath, publicURL string, err error) {
	slug = slugOf(a)
	publicURL = b.BaseURL + "/" + slug
	publishPath, err = contentPath(b.ContentDir, slug)
	return slug, publishPath, publicURL, err
}

// Publish writes the article as MDX and commits it. Returns the real public URL
// to be backfilled as canonical_url (§5.6).
func (b *BlogPublisher) Publish(ctx context.Context, a *db.Article) (Result, error) {
	slug, publishPath, publicURL, err := b.Resolve(a)
	if err != nil {
		return Result{}, err
	}
	mdx, err := renderMDX(a, slug, publicURL, b.now())
	if err != nil {
		return Result{}, err
	}

	if b.dryRun() {
		b.Log.Warn("BlogPublisher dry-run (no repo/token configured)", "path", publishPath, "url", publicURL)
		return Result{URL: publicURL, Mode: Auto, Detail: "dry-run: not committed (configure GITHUB_TOKEN + BLOG_REPO)", Path: publishPath, Phase: "pending_url_verification"}, nil
	}
	msg := fmt.Sprintf("CiteLoop publish: %s\n\nProject: %s\nArticle: %s", title(a), a.ProjectID, a.ID)
	commitSHA, err := b.commitFile(ctx, publishPath, mdx, msg)
	if err != nil {
		return Result{}, fmt.Errorf("commit mdx: %w", err)
	}
	b.Log.Info("blog published", "path", publishPath, "url", publicURL, "branch", b.Branch, "commit", commitSHA)
	return Result{URL: publicURL, Mode: Auto, Detail: "committed to " + b.Repo + "@" + b.Branch, Path: publishPath, CommitSHA: commitSHA, Phase: "pending_url_verification"}, nil
}

// commitFile creates or updates path on the publish branch via the Contents API.
func (b *BlogPublisher) commitFile(ctx context.Context, publishPath string, content []byte, msg string) (string, error) {
	api := "https://api.github.com/repos/" + b.Repo + "/contents/" + publishPath

	// Look up existing sha (update vs create) on the branch.
	sha, err := b.existingSHA(ctx, api)
	if err != nil {
		return "", err
	}

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
		return "", err
	}
	b.auth(req)
	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("github %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Commit.SHA, nil
}

func (b *BlogPublisher) existingSHA(ctx context.Context, api string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"?ref="+b.Branch, nil)
	if err != nil {
		return "", err
	}
	b.auth(req)
	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github lookup %d: %s", resp.StatusCode, string(raw))
	}
	var meta struct {
		SHA string `json:"sha"`
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &meta)
	return meta.SHA, nil
}

func (b *BlogPublisher) PublishedPathExists(ctx context.Context, publishPath string) error {
	cleanPath, err := safeRelativePath(publishPath, "publish path")
	if err != nil {
		return err
	}
	if b.dryRun() {
		return nil
	}
	api := "https://api.github.com/repos/" + b.Repo + "/contents/" + cleanPath
	sha, err := b.existingSHA(ctx, api)
	if err != nil {
		return err
	}
	if sha == "" {
		return fmt.Errorf("publish path missing: %s", cleanPath)
	}
	return nil
}

func (b *BlogPublisher) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+b.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

func contentPath(contentDir, slug string) (string, error) {
	cleanDir, err := safeRelativePath(contentDir, "content dir")
	if err != nil {
		return "", err
	}
	publishPath := cleanDir + "/" + slug + ".mdx"
	if !strings.HasPrefix(publishPath, cleanDir+"/") {
		return "", fmt.Errorf("publish path escapes content dir: %s", publishPath)
	}
	return publishPath, nil
}

func safeRelativePath(raw, label string) (string, error) {
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("invalid %s: %w", label, err)
	}
	decoded = strings.ReplaceAll(decoded, "\\", "/")
	if decoded == "" || strings.HasPrefix(decoded, "/") {
		return "", fmt.Errorf("invalid %s %q", label, raw)
	}
	trimmed := strings.Trim(decoded, "/")
	for _, part := range strings.Split(trimmed, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid %s %q", label, raw)
		}
	}
	return path.Clean(trimmed), nil
}

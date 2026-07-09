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
)

type GitHubPRClient struct {
	Token      string
	Repo       string
	BaseBranch string
	Log        *slog.Logger
	client     *http.Client
}

type GitHubPRInput struct {
	SourcePath        string
	WorkingBranch     string
	BaseFileSHA       string
	ProposedContentMD string
	CommitMessage     string
	Title             string
	Body              string
}

type GitHubPRResult struct {
	Number        int    `json:"number"`
	URL           string `json:"url"`
	State         string `json:"state"`
	WorkingBranch string `json:"working_branch"`
	BaseBranch    string `json:"base_branch"`
	HeadCommitSHA string `json:"head_commit_sha"`
	BaseFileSHA   string `json:"base_file_sha"`
	SourcePath    string `json:"source_file_path"`
}

func NewGitHubPRClient(token, repo, baseBranch string, log *slog.Logger) *GitHubPRClient {
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = "main"
	}
	return &GitHubPRClient{
		Token: strings.TrimSpace(token), Repo: strings.TrimSpace(repo), BaseBranch: strings.TrimSpace(baseBranch),
		Log: log, client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *GitHubPRClient) CreatePageUpdatePR(ctx context.Context, in GitHubPRInput) (GitHubPRResult, error) {
	if strings.TrimSpace(c.Token) == "" || strings.TrimSpace(c.Repo) == "" {
		return GitHubPRResult{}, fmt.Errorf("github token and repo are required for source-backed PR apply")
	}
	sourcePath, err := safeRelativePath(in.SourcePath, "source path")
	if err != nil {
		return GitHubPRResult{}, err
	}
	workingBranch := strings.TrimSpace(in.WorkingBranch)
	if workingBranch == "" {
		return GitHubPRResult{}, fmt.Errorf("working branch is required")
	}
	currentSHA, err := c.fileSHA(ctx, sourcePath, c.BaseBranch)
	if err != nil {
		return GitHubPRResult{}, err
	}
	if strings.TrimSpace(in.BaseFileSHA) != "" && currentSHA != strings.TrimSpace(in.BaseFileSHA) {
		return GitHubPRResult{}, fmt.Errorf("source file changed since draft approval: expected %s got %s", strings.TrimSpace(in.BaseFileSHA), currentSHA)
	}
	baseCommitSHA, err := c.baseCommitSHA(ctx)
	if err != nil {
		return GitHubPRResult{}, err
	}
	branchCreated, err := c.createBranch(ctx, workingBranch, baseCommitSHA)
	if err != nil {
		return GitHubPRResult{}, err
	}
	commitFileSHA := currentSHA
	if !branchCreated {
		commitFileSHA, err = c.fileSHA(ctx, sourcePath, workingBranch)
		if err != nil {
			return GitHubPRResult{}, err
		}
	}
	headCommitSHA, err := c.commitFile(ctx, sourcePath, []byte(in.ProposedContentMD), commitFileSHA, workingBranch, in.CommitMessage)
	if err != nil {
		return GitHubPRResult{}, err
	}
	number, url, state, err := c.openPullRequest(ctx, workingBranch, in.Title, in.Body)
	if err != nil {
		return GitHubPRResult{}, err
	}
	return GitHubPRResult{
		Number: number, URL: url, State: state, WorkingBranch: workingBranch, BaseBranch: c.BaseBranch,
		HeadCommitSHA: headCommitSHA, BaseFileSHA: currentSHA, SourcePath: sourcePath,
	}, nil
}

func (c *GitHubPRClient) ReadFile(ctx context.Context, sourcePath, ref string) (string, string, error) {
	sourcePath, err := safeRelativePath(sourcePath, "source path")
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(ref) == "" {
		ref = c.BaseBranch
	}
	api := "https://api.github.com/repos/" + c.Repo + "/contents/" + sourcePath + "?ref=" + ref
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", "", err
	}
	c.auth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("github content lookup %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		SHA      string `json:"sha"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(out.SHA) == "" {
		return "", "", fmt.Errorf("github content lookup returned empty sha")
	}
	if strings.TrimSpace(out.Encoding) != "" && strings.TrimSpace(out.Encoding) != "base64" {
		return "", "", fmt.Errorf("github content encoding %q is not supported", out.Encoding)
	}
	encoded := strings.NewReplacer("\n", "", "\r", "", " ", "").Replace(out.Content)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", err
	}
	return string(decoded), out.SHA, nil
}

func (c *GitHubPRClient) fileSHA(ctx context.Context, sourcePath, ref string) (string, error) {
	api := "https://api.github.com/repos/" + c.Repo + "/contents/" + sourcePath + "?ref=" + ref
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	c.auth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github content lookup %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.SHA) == "" {
		return "", fmt.Errorf("github content lookup returned empty sha")
	}
	return out.SHA, nil
}

func (c *GitHubPRClient) baseCommitSHA(ctx context.Context) (string, error) {
	api := "https://api.github.com/repos/" + c.Repo + "/git/ref/heads/" + c.BaseBranch
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	c.auth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github base ref lookup %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.Object.SHA) == "" {
		return "", fmt.Errorf("github base ref returned empty sha")
	}
	return out.Object.SHA, nil
}

func (c *GitHubPRClient) createBranch(ctx context.Context, branch, sha string) (bool, error) {
	payload := map[string]any{"ref": "refs/heads/" + branch, "sha": sha}
	body, _ := json.Marshal(payload)
	endpoint := "https://api.github.com/repos/" + c.Repo + "/git/refs"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	c.auth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusCreated {
		return true, nil
	}
	if resp.StatusCode == http.StatusUnprocessableEntity && strings.Contains(strings.ToLower(string(raw)), "reference already exists") {
		return false, nil
	}
	return false, fmt.Errorf("github %s %s %d: %s", http.MethodPost, endpoint, resp.StatusCode, string(raw))
}

func (c *GitHubPRClient) commitFile(ctx context.Context, sourcePath string, content []byte, fileSHA, branch, message string) (string, error) {
	payload := map[string]any{
		"message": strings.TrimSpace(message),
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  branch,
		"sha":     fileSHA,
	}
	var out struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := c.doJSON(ctx, http.MethodPut, "https://api.github.com/repos/"+c.Repo+"/contents/"+sourcePath, payload, &out, http.StatusOK, http.StatusCreated); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.Commit.SHA) == "" {
		return "", fmt.Errorf("github content update returned empty commit sha")
	}
	return out.Commit.SHA, nil
}

func (c *GitHubPRClient) openPullRequest(ctx context.Context, branch, title, body string) (int, string, string, error) {
	payload := map[string]any{"title": strings.TrimSpace(title), "head": branch, "base": c.BaseBranch, "body": body}
	var out struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		State   string `json:"state"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "https://api.github.com/repos/"+c.Repo+"/pulls", payload, &out, http.StatusCreated); err != nil {
		return 0, "", "", err
	}
	if out.Number == 0 || strings.TrimSpace(out.HTMLURL) == "" {
		return 0, "", "", fmt.Errorf("github pull request response missing number or url")
	}
	return out.Number, out.HTMLURL, out.State, nil
}

// GitHubPRState is a read-only snapshot of a pull request's merge status, used
// to reconcile the site-change apply ledger after an operator merges the PR.
type GitHubPRState struct {
	Number   int
	State    string // "open" | "closed"
	Merged   bool
	MergedAt *time.Time
	HTMLURL  string
}

// GetPullRequest reads a pull request's current state so the scheduler can
// detect a merge (or a close-without-merge) without a webhook.
func (c *GitHubPRClient) GetPullRequest(ctx context.Context, number int) (GitHubPRState, error) {
	if strings.TrimSpace(c.Token) == "" || strings.TrimSpace(c.Repo) == "" {
		return GitHubPRState{}, fmt.Errorf("github token and repo are required to read a pull request")
	}
	if number <= 0 {
		return GitHubPRState{}, fmt.Errorf("github pull request number is required")
	}
	var out struct {
		Number   int        `json:"number"`
		State    string     `json:"state"`
		Merged   bool       `json:"merged"`
		MergedAt *time.Time `json:"merged_at"`
		HTMLURL  string     `json:"html_url"`
	}
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d", c.Repo, number)
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &out, http.StatusOK); err != nil {
		return GitHubPRState{}, err
	}
	return GitHubPRState{Number: out.Number, State: out.State, Merged: out.Merged, MergedAt: out.MergedAt, HTMLURL: out.HTMLURL}, nil
}

func (c *GitHubPRClient) doJSON(ctx context.Context, method, endpoint string, payload any, out any, allowed ...int) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.auth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	ok := false
	for _, status := range allowed {
		if resp.StatusCode == status {
			ok = true
			break
		}
	}
	raw, _ := io.ReadAll(resp.Body)
	if !ok {
		return fmt.Errorf("github %s %s %d: %s", method, endpoint, resp.StatusCode, string(raw))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
	}
	return nil
}

func (c *GitHubPRClient) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

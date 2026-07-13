package publisher

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
)

// ErrSourceConflict marks an immutable base-ref or source-blob identity that
// no longer matches the repository snapshot used to prepare a patch.
var ErrSourceConflict = errors.New("repository source changed since preparation")

// ErrDivergentPullRequest marks a deterministic PR or branch head whose
// commit tree or base parent does not exactly match the prepared patch.
var ErrDivergentPullRequest = errors.New("existing pull request is divergent from the prepared change")

type GitHubPRClient struct {
	Token          string
	Repo           string
	BaseBranch     string
	Log            *slog.Logger
	client         *http.Client
	BeforeMutation func(context.Context) error
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

// GitHubTreeEntry is an existing blob in a recursively-listed repository
// tree. Mode is retained so an atomic update does not silently remove an
// executable bit or change another Git object type.
type GitHubTreeEntry struct {
	Path string
	Mode string
	Type string
	SHA  string
	Size int64
}

// GitHubFileUpdate describes one existing blob replacement. BaseBlobSHA pins
// the approved source snapshot and is checked again immediately before any
// Git ref is created.
type GitHubFileUpdate struct {
	Path        string
	BaseBlobSHA string
	Content     []byte
}

// GitHubFileUpdatesPRInput creates one commit and pull request for all Files.
// BaseCommitSHA pins the selected base ref as well as each file's blob SHA.
// CommitDate must come from persisted lifecycle data so a retry replays the
// identical Git commit object.
type GitHubFileUpdatesPRInput struct {
	WorkingBranch string
	BaseCommitSHA string
	Files         []GitHubFileUpdate
	CommitMessage string
	CommitDate    time.Time
	Title         string
	Body          string
}

const (
	gitHubCommitIdentityName   = "CiteLoop"
	gitHubCommitIdentityEmail  = "noreply@citeloop.dev"
	maxGitHubTreeResponseBytes = 8 << 20
	maxGitHubTreeEntries       = 25_000
	maxGitHubAPIResponseBytes  = 8 << 20
	maxGitHubPRCandidates      = 1_000
)

// GitHubAPIError exposes only the upstream status needed for controlled
// readiness invalidation. It deliberately omits the response body, request
// headers, credential, and endpoint from Error so callers and logs cannot
// leak GitHub-controlled or secret material.
type GitHubAPIError struct {
	status int
}

func (e *GitHubAPIError) Error() string {
	return fmt.Sprintf("GitHub request failed with status %d", e.status)
}

func (e *GitHubAPIError) StatusCode() int { return e.status }

func NewGitHubAPIError(status int) *GitHubAPIError { return &GitHubAPIError{status: status} }

type GitHubPRResult struct {
	Number        int    `json:"number"`
	URL           string `json:"url"`
	State         string `json:"state"`
	WorkingBranch string `json:"working_branch"`
	BaseBranch    string `json:"base_branch"`
	BaseCommitSHA string `json:"base_commit_sha,omitempty"`
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

// ListTree reads a complete recursive Git tree and returns only blobs. GitHub
// includes directory entries in recursive trees; filtering those here keeps
// non-file objects out of repository source selection.
func (c *GitHubPRClient) ListTree(ctx context.Context, ref string) ([]GitHubTreeEntry, error) {
	if strings.TrimSpace(c.Token) == "" || strings.TrimSpace(c.Repo) == "" {
		return nil, fmt.Errorf("github token and repo are required to list a repository tree")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("github tree ref is required")
	}
	endpoint := "https://api.github.com/repos/" + c.Repo + "/git/trees/" + url.PathEscape(ref) + "?recursive=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	c.auth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := readGitHubResponse(resp, maxGitHubTreeResponseBytes, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var out struct {
		Truncated bool `json:"truncated"`
		Tree      []struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
			Size int64  `json:"size"`
		} `json:"tree"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("github recursive tree response contains trailing data")
	}
	if out.Truncated {
		return nil, fmt.Errorf("github recursive tree is truncated")
	}
	if len(out.Tree) > maxGitHubTreeEntries {
		return nil, fmt.Errorf("github recursive tree exceeds %d entries", maxGitHubTreeEntries)
	}
	entries := make([]GitHubTreeEntry, 0, len(out.Tree))
	for _, entry := range out.Tree {
		if strings.TrimSpace(entry.Type) != "blob" {
			continue
		}
		path, err := validateRawGitTreePath(entry.Path, "tree entry path")
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(entry.Mode) == "" || strings.TrimSpace(entry.SHA) == "" {
			return nil, fmt.Errorf("github tree blob %q is missing mode or sha", path)
		}
		entries = append(entries, GitHubTreeEntry{
			Path: path, Mode: strings.TrimSpace(entry.Mode), Type: "blob",
			SHA: strings.TrimSpace(entry.SHA), Size: entry.Size,
		})
	}
	return entries, nil
}

func readBoundedGitHubBody(body io.Reader, maxBytes int) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, int64(maxBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxBytes {
		return nil, fmt.Errorf("github response exceeds %d bytes", maxBytes)
	}
	return raw, nil
}

func readGitHubResponse(resp *http.Response, maxBytes int, allowed ...int) ([]byte, error) {
	raw, err := readBoundedGitHubBody(resp.Body, maxBytes)
	if err != nil {
		return nil, err
	}
	for _, status := range allowed {
		if resp.StatusCode == status {
			return raw, nil
		}
	}
	return nil, &GitHubAPIError{status: resp.StatusCode}
}

func decodeGitHubJSONResponse(resp *http.Response, out any, maxBytes int, allowed ...int) error {
	raw, err := readGitHubResponse(resp, maxBytes, allowed...)
	if err != nil {
		return err
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(out); err != nil {
		return errors.New("GitHub response contained invalid JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("GitHub response contained trailing JSON data")
	}
	return nil
}

// ReadBlob reads content by immutable Git blob SHA rather than by a moving
// branch/path pair.
func (c *GitHubPRClient) ReadBlob(ctx context.Context, blobSHA string) ([]byte, error) {
	if strings.TrimSpace(c.Token) == "" || strings.TrimSpace(c.Repo) == "" {
		return nil, fmt.Errorf("github token and repo are required to read a repository blob")
	}
	blobSHA = strings.TrimSpace(blobSHA)
	if blobSHA == "" {
		return nil, fmt.Errorf("github blob sha is required")
	}
	endpoint := "https://api.github.com/repos/" + c.Repo + "/git/blobs/" + url.PathEscape(blobSHA)
	var out struct {
		SHA      string `json:"sha"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &out, http.StatusOK); err != nil {
		return nil, err
	}
	if returned := strings.TrimSpace(out.SHA); returned != "" && returned != blobSHA {
		return nil, fmt.Errorf("github blob lookup returned sha %s instead of %s", returned, blobSHA)
	}
	if strings.TrimSpace(out.Encoding) != "base64" {
		return nil, fmt.Errorf("github blob encoding %q is not supported", out.Encoding)
	}
	encoded := strings.NewReplacer("\n", "", "\r", "", " ", "").Replace(out.Content)
	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	return content, nil
}

// CreateFileUpdatesPR creates an atomic multi-file Git commit. All Git
// objects are prepared before the deterministic branch ref becomes visible.
// A retry may recreate immutable blobs/tree objects, but it reuses an exact
// orphan branch commit and never force-updates a divergent branch.
func (c *GitHubPRClient) CreateFileUpdatesPR(ctx context.Context, in GitHubFileUpdatesPRInput) (GitHubPRResult, error) {
	prepared, err := prepareGitHubFileUpdatesInput(in)
	if err != nil {
		return GitHubPRResult{}, err
	}
	if strings.TrimSpace(c.Token) == "" || strings.TrimSpace(c.Repo) == "" {
		return GitHubPRResult{}, fmt.Errorf("github token and repo are required for source-backed PR apply")
	}

	existingPRCandidates, err := c.findPullRequestsByHead(ctx, prepared.WorkingBranch)
	if err != nil {
		return GitHubPRResult{}, err
	}
	existingPRFound := len(existingPRCandidates) > 0

	baseCommitSHA := prepared.BaseCommitSHA
	if !existingPRFound {
		liveBaseCommitSHA, err := c.baseCommitSHA(ctx)
		if err != nil {
			return GitHubPRResult{}, err
		}
		if liveBaseCommitSHA != baseCommitSHA {
			return GitHubPRResult{}, fmt.Errorf("%w: base branch expected %s got %s", ErrSourceConflict, baseCommitSHA, liveBaseCommitSHA)
		}
	}
	baseCommit, err := c.gitCommit(ctx, baseCommitSHA)
	if err != nil {
		return GitHubPRResult{}, err
	}
	baseEntries, err := c.ListTree(ctx, baseCommit.TreeSHA)
	if err != nil {
		return GitHubPRResult{}, err
	}
	entriesByPath := make(map[string]GitHubTreeEntry, len(baseEntries))
	for _, entry := range baseEntries {
		entriesByPath[entry.Path] = entry
	}
	for _, file := range prepared.Files {
		entry, ok := entriesByPath[file.Path]
		if !ok || entry.Type != "blob" {
			return GitHubPRResult{}, fmt.Errorf("%w: source file %s is not an existing blob on %s", ErrSourceConflict, file.Path, c.BaseBranch)
		}
		if entry.SHA != file.BaseBlobSHA {
			return GitHubPRResult{}, fmt.Errorf("%w: source file %s expected blob %s got %s", ErrSourceConflict, file.Path, file.BaseBlobSHA, entry.SHA)
		}
	}

	treeEntries := make([]gitHubCreateTreeEntry, 0, len(prepared.Files))
	for _, file := range prepared.Files {
		newBlobSHA, err := c.createGitBlob(ctx, file.Content)
		if err != nil {
			return GitHubPRResult{}, err
		}
		baseEntry := entriesByPath[file.Path]
		treeEntries = append(treeEntries, gitHubCreateTreeEntry{
			Path: file.Path, Mode: baseEntry.Mode, Type: baseEntry.Type, SHA: newBlobSHA,
		})
	}
	desiredTreeSHA, err := c.createGitTree(ctx, baseCommit.TreeSHA, treeEntries)
	if err != nil {
		return GitHubPRResult{}, err
	}
	if existingPRFound {
		existingPR, found, err := c.exactPullRequestForTarget(ctx, existingPRCandidates, desiredTreeSHA, baseCommitSHA)
		if err != nil {
			return GitHubPRResult{}, err
		}
		if !found {
			return GitHubPRResult{}, fmt.Errorf("%w: head %s", ErrDivergentPullRequest, prepared.WorkingBranch)
		}
		existingPR.BaseCommitSHA = baseCommitSHA
		return existingPR, nil
	}

	headCommitSHA, branchExists, err := c.refCommitSHAIfExists(ctx, prepared.WorkingBranch)
	if err != nil {
		return GitHubPRResult{}, err
	}
	if branchExists {
		matches, err := c.gitCommitMatches(ctx, headCommitSHA, desiredTreeSHA, baseCommitSHA)
		if err != nil {
			return GitHubPRResult{}, err
		}
		if !matches {
			return GitHubPRResult{}, fmt.Errorf("%w: deterministic working branch %s; refusing to force update it", ErrDivergentPullRequest, prepared.WorkingBranch)
		}
	} else {
		headCommitSHA, err = c.createGitCommit(ctx, prepared.CommitMessage, desiredTreeSHA, baseCommitSHA, prepared.CommitDate)
		if err != nil {
			return GitHubPRResult{}, err
		}
		created, err := c.createBranch(ctx, prepared.WorkingBranch, headCommitSHA)
		if err != nil {
			return GitHubPRResult{}, err
		}
		if !created {
			existingHead, found, err := c.refCommitSHAIfExists(ctx, prepared.WorkingBranch)
			if err != nil {
				return GitHubPRResult{}, err
			}
			if !found {
				return GitHubPRResult{}, fmt.Errorf("github reported working branch already exists but it could not be read")
			}
			matches, err := c.gitCommitMatches(ctx, existingHead, desiredTreeSHA, baseCommitSHA)
			if err != nil {
				return GitHubPRResult{}, err
			}
			if !matches {
				return GitHubPRResult{}, fmt.Errorf("%w: deterministic working branch %s; refusing to force update it", ErrDivergentPullRequest, prepared.WorkingBranch)
			}
			headCommitSHA = existingHead
		}
	}

	number, prURL, state, err := c.openPullRequest(ctx, prepared.WorkingBranch, prepared.Title, prepared.Body)
	if err != nil {
		if candidates, reconcileErr := c.findPullRequestsByHead(ctx, prepared.WorkingBranch); reconcileErr == nil && len(candidates) > 0 {
			existing, found, matchErr := c.exactPullRequestForTarget(ctx, candidates, desiredTreeSHA, baseCommitSHA)
			if matchErr != nil {
				return GitHubPRResult{}, matchErr
			}
			if !found {
				return GitHubPRResult{}, fmt.Errorf("%w: head %s", ErrDivergentPullRequest, prepared.WorkingBranch)
			}
			existing.BaseCommitSHA = baseCommitSHA
			return existing, nil
		}
		return GitHubPRResult{}, err
	}
	return GitHubPRResult{
		Number: number, URL: prURL, State: normalizePullRequestState(state, nil),
		WorkingBranch: prepared.WorkingBranch, BaseBranch: c.BaseBranch,
		BaseCommitSHA: baseCommitSHA, HeadCommitSHA: headCommitSHA,
	}, nil
}

type gitHubCommit struct {
	SHA     string
	TreeSHA string
	Parents []string
}

type gitHubCreateTreeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

type gitHubCommitIdentity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

func prepareGitHubFileUpdatesInput(in GitHubFileUpdatesPRInput) (GitHubFileUpdatesPRInput, error) {
	in.WorkingBranch = strings.TrimSpace(in.WorkingBranch)
	in.BaseCommitSHA = strings.TrimSpace(in.BaseCommitSHA)
	in.CommitMessage = strings.TrimSpace(in.CommitMessage)
	in.Title = strings.TrimSpace(in.Title)
	if in.WorkingBranch == "" {
		return GitHubFileUpdatesPRInput{}, fmt.Errorf("working branch is required")
	}
	if strings.HasPrefix(in.WorkingBranch, "refs/") || strings.ContainsAny(in.WorkingBranch, " ~^:?*[\\") || strings.Contains(in.WorkingBranch, "..") || strings.Contains(in.WorkingBranch, "@{") || strings.HasPrefix(in.WorkingBranch, "/") || strings.HasSuffix(in.WorkingBranch, "/") {
		return GitHubFileUpdatesPRInput{}, fmt.Errorf("working branch is invalid")
	}
	if in.BaseCommitSHA == "" {
		return GitHubFileUpdatesPRInput{}, fmt.Errorf("base commit sha is required")
	}
	if in.CommitMessage == "" {
		return GitHubFileUpdatesPRInput{}, fmt.Errorf("commit message is required")
	}
	if in.CommitDate.IsZero() {
		return GitHubFileUpdatesPRInput{}, fmt.Errorf("deterministic commit date is required")
	}
	in.CommitDate = in.CommitDate.Round(0).UTC().Truncate(time.Second)
	if year := in.CommitDate.Year(); year < 1 || year > 9999 {
		return GitHubFileUpdatesPRInput{}, fmt.Errorf("commit date cannot be represented as RFC3339")
	}
	if in.Title == "" {
		return GitHubFileUpdatesPRInput{}, fmt.Errorf("pull request title is required")
	}
	if len(in.Files) == 0 {
		return GitHubFileUpdatesPRInput{}, fmt.Errorf("at least one file update is required")
	}
	files := make([]GitHubFileUpdate, len(in.Files))
	copy(files, in.Files)
	in.Files = files
	seen := make(map[string]struct{}, len(in.Files))
	for i := range in.Files {
		path, err := validateRawGitTreePath(in.Files[i].Path, "source path")
		if err != nil {
			return GitHubFileUpdatesPRInput{}, err
		}
		if _, exists := seen[path]; exists {
			return GitHubFileUpdatesPRInput{}, fmt.Errorf("duplicate source path %s", path)
		}
		seen[path] = struct{}{}
		in.Files[i].Path = path
		in.Files[i].BaseBlobSHA = strings.TrimSpace(in.Files[i].BaseBlobSHA)
		if in.Files[i].BaseBlobSHA == "" {
			return GitHubFileUpdatesPRInput{}, fmt.Errorf("base blob sha is required for %s", path)
		}
	}
	sort.Slice(in.Files, func(i, j int) bool { return in.Files[i].Path < in.Files[j].Path })
	return in, nil
}

// validateRawGitTreePath validates a path exactly as GitHub returned it. A
// percent sign is an ordinary Git filename byte here, not URL escaping, so
// this function must never unescape, clean, trim, or otherwise rewrite input.
func validateRawGitTreePath(raw, label string) (string, error) {
	if raw == "" || strings.HasPrefix(raw, "/") || strings.Contains(raw, "\\") {
		return "", fmt.Errorf("invalid %s %q", label, raw)
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("invalid %s %q", label, raw)
		}
	}
	for _, part := range strings.Split(raw, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid %s %q", label, raw)
		}
	}
	return raw, nil
}

func (c *GitHubPRClient) gitCommit(ctx context.Context, sha string) (gitHubCommit, error) {
	var out struct {
		SHA  string `json:"sha"`
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
		Parents []struct {
			SHA string `json:"sha"`
		} `json:"parents"`
	}
	endpoint := "https://api.github.com/repos/" + c.Repo + "/git/commits/" + url.PathEscape(strings.TrimSpace(sha))
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &out, http.StatusOK); err != nil {
		return gitHubCommit{}, err
	}
	if strings.TrimSpace(out.SHA) == "" {
		out.SHA = strings.TrimSpace(sha)
	}
	if strings.TrimSpace(out.Tree.SHA) == "" {
		return gitHubCommit{}, fmt.Errorf("github commit %s returned an empty tree sha", sha)
	}
	commit := gitHubCommit{SHA: strings.TrimSpace(out.SHA), TreeSHA: strings.TrimSpace(out.Tree.SHA), Parents: make([]string, 0, len(out.Parents))}
	for _, parent := range out.Parents {
		commit.Parents = append(commit.Parents, strings.TrimSpace(parent.SHA))
	}
	return commit, nil
}

func (c *GitHubPRClient) createGitBlob(ctx context.Context, content []byte) (string, error) {
	if err := c.beforeMutation(ctx); err != nil {
		return "", err
	}
	payload := map[string]string{"content": base64.StdEncoding.EncodeToString(content), "encoding": "base64"}
	var out struct {
		SHA string `json:"sha"`
	}
	endpoint := "https://api.github.com/repos/" + c.Repo + "/git/blobs"
	if err := c.doJSON(ctx, http.MethodPost, endpoint, payload, &out, http.StatusCreated); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.SHA) == "" {
		return "", fmt.Errorf("github blob creation returned empty sha")
	}
	return strings.TrimSpace(out.SHA), nil
}

func (c *GitHubPRClient) createGitTree(ctx context.Context, baseTreeSHA string, entries []gitHubCreateTreeEntry) (string, error) {
	if err := c.beforeMutation(ctx); err != nil {
		return "", err
	}
	payload := struct {
		BaseTree string                  `json:"base_tree"`
		Tree     []gitHubCreateTreeEntry `json:"tree"`
	}{BaseTree: baseTreeSHA, Tree: entries}
	var out struct {
		SHA string `json:"sha"`
	}
	endpoint := "https://api.github.com/repos/" + c.Repo + "/git/trees"
	if err := c.doJSON(ctx, http.MethodPost, endpoint, payload, &out, http.StatusCreated); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.SHA) == "" {
		return "", fmt.Errorf("github tree creation returned empty sha")
	}
	return strings.TrimSpace(out.SHA), nil
}

func (c *GitHubPRClient) createGitCommit(ctx context.Context, message, treeSHA, parentSHA string, commitDate time.Time) (string, error) {
	if err := c.beforeMutation(ctx); err != nil {
		return "", err
	}
	identity := gitHubCommitIdentity{
		Name: gitHubCommitIdentityName, Email: gitHubCommitIdentityEmail,
		Date: commitDate.UTC().Format(time.RFC3339),
	}
	payload := struct {
		Message   string               `json:"message"`
		Tree      string               `json:"tree"`
		Parents   []string             `json:"parents"`
		Author    gitHubCommitIdentity `json:"author"`
		Committer gitHubCommitIdentity `json:"committer"`
	}{Message: message, Tree: treeSHA, Parents: []string{parentSHA}, Author: identity, Committer: identity}
	var out struct {
		SHA string `json:"sha"`
	}
	endpoint := "https://api.github.com/repos/" + c.Repo + "/git/commits"
	if err := c.doJSON(ctx, http.MethodPost, endpoint, payload, &out, http.StatusCreated); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.SHA) == "" {
		return "", fmt.Errorf("github commit creation returned empty sha")
	}
	return strings.TrimSpace(out.SHA), nil
}

func (c *GitHubPRClient) refCommitSHAIfExists(ctx context.Context, branch string) (string, bool, error) {
	endpoint := c.gitRefEndpoint(branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", false, err
	}
	c.auth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		if _, err := readBoundedGitHubBody(resp.Body, maxGitHubAPIResponseBytes); err != nil {
			return "", false, err
		}
		return "", false, nil
	}
	var out struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := decodeGitHubJSONResponse(resp, &out, maxGitHubAPIResponseBytes, http.StatusOK); err != nil {
		return "", false, err
	}
	if strings.TrimSpace(out.Object.SHA) == "" {
		return "", false, fmt.Errorf("github working ref returned empty sha")
	}
	return strings.TrimSpace(out.Object.SHA), true, nil
}

func (c *GitHubPRClient) gitCommitMatches(ctx context.Context, commitSHA, treeSHA, parentSHA string) (bool, error) {
	commit, err := c.gitCommit(ctx, commitSHA)
	if err != nil {
		return false, err
	}
	return commit.TreeSHA == treeSHA && len(commit.Parents) == 1 && commit.Parents[0] == parentSHA, nil
}

func (c *GitHubPRClient) exactPullRequestForTarget(ctx context.Context, candidates []GitHubPRResult, treeSHA, parentSHA string) (GitHubPRResult, bool, error) {
	exact := make([]GitHubPRResult, 0, len(candidates))
	matchByHead := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		matches, checked := matchByHead[candidate.HeadCommitSHA]
		if !checked {
			var err error
			matches, err = c.gitCommitMatches(ctx, candidate.HeadCommitSHA, treeSHA, parentSHA)
			if err != nil {
				return GitHubPRResult{}, false, err
			}
			matchByHead[candidate.HeadCommitSHA] = matches
		}
		if matches {
			exact = append(exact, candidate)
		}
	}
	return preferredPullRequest(exact)
}

func (c *GitHubPRClient) beforeMutation(ctx context.Context) error {
	if c.BeforeMutation == nil {
		return nil
	}
	return c.BeforeMutation(ctx)
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
	headCommitSHA := ""
	if !branchCreated {
		var branchContent string
		branchContent, commitFileSHA, err = c.ReadFile(ctx, sourcePath, workingBranch)
		if err != nil {
			return GitHubPRResult{}, err
		}
		if branchContent == in.ProposedContentMD {
			headCommitSHA, err = c.refCommitSHA(ctx, workingBranch)
			if err != nil {
				return GitHubPRResult{}, err
			}
		}
	}
	if headCommitSHA == "" {
		headCommitSHA, err = c.commitFile(ctx, sourcePath, []byte(in.ProposedContentMD), commitFileSHA, workingBranch, in.CommitMessage)
		if err != nil {
			return GitHubPRResult{}, err
		}
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
	api := c.contentEndpoint(sourcePath, ref)
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
	var out struct {
		SHA      string `json:"sha"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := decodeGitHubJSONResponse(resp, &out, maxGitHubAPIResponseBytes, http.StatusOK); err != nil {
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
	api := c.contentEndpoint(sourcePath, ref)
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
	var out struct {
		SHA string `json:"sha"`
	}
	if err := decodeGitHubJSONResponse(resp, &out, maxGitHubAPIResponseBytes, http.StatusOK); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.SHA) == "" {
		return "", fmt.Errorf("github content lookup returned empty sha")
	}
	return out.SHA, nil
}

func (c *GitHubPRClient) baseCommitSHA(ctx context.Context) (string, error) {
	return c.refCommitSHA(ctx, c.BaseBranch)
}

func (c *GitHubPRClient) refCommitSHA(ctx context.Context, branch string) (string, error) {
	api := c.gitRefEndpoint(branch)
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
	var out struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := decodeGitHubJSONResponse(resp, &out, maxGitHubAPIResponseBytes, http.StatusOK); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.Object.SHA) == "" {
		return "", fmt.Errorf("github base ref returned empty sha")
	}
	return out.Object.SHA, nil
}

func (c *GitHubPRClient) gitRefEndpoint(branch string) string {
	return "https://api.github.com/repos/" + c.Repo + "/git/ref/heads/" + url.PathEscape(strings.TrimSpace(branch))
}

func (c *GitHubPRClient) contentEndpoint(sourcePath, ref string) string {
	query := url.Values{}
	query.Set("ref", ref)
	return "https://api.github.com/repos/" + c.Repo + "/contents/" + sourcePath + "?" + query.Encode()
}

func (c *GitHubPRClient) createBranch(ctx context.Context, branch, sha string) (bool, error) {
	if c.BeforeMutation != nil {
		if err := c.BeforeMutation(ctx); err != nil {
			return false, err
		}
	}
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
	raw, readErr := readBoundedGitHubBody(resp.Body, maxGitHubAPIResponseBytes)
	if readErr != nil {
		return false, readErr
	}
	if resp.StatusCode == http.StatusCreated {
		return true, nil
	}
	if resp.StatusCode == http.StatusUnprocessableEntity && strings.Contains(strings.ToLower(string(raw)), "reference already exists") {
		return false, nil
	}
	return false, &GitHubAPIError{status: resp.StatusCode}
}

func (c *GitHubPRClient) commitFile(ctx context.Context, sourcePath string, content []byte, fileSHA, branch, message string) (string, error) {
	if c.BeforeMutation != nil {
		if err := c.BeforeMutation(ctx); err != nil {
			return "", err
		}
	}
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
	if c.BeforeMutation != nil {
		if err := c.BeforeMutation(ctx); err != nil {
			return 0, "", "", err
		}
	}
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

// FindPullRequestByHead reconciles a deterministic working branch before any
// new GitHub mutation. This closes the lost-response window where the PR was
// created remotely but its identifiers were not persisted locally.
func (c *GitHubPRClient) FindPullRequestByHead(ctx context.Context, workingBranch string) (GitHubPRResult, bool, error) {
	candidates, err := c.findPullRequestsByHead(ctx, workingBranch)
	if err != nil {
		return GitHubPRResult{}, false, err
	}
	return preferredPullRequest(candidates)
}

func (c *GitHubPRClient) findPullRequestsByHead(ctx context.Context, workingBranch string) ([]GitHubPRResult, error) {
	workingBranch = strings.TrimSpace(workingBranch)
	parts := strings.SplitN(c.Repo, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || workingBranch == "" {
		return nil, fmt.Errorf("repo owner and working branch are required to reconcile a pull request")
	}
	query := url.Values{}
	query.Set("state", "all")
	query.Set("base", c.BaseBranch)
	query.Set("head", parts[0]+":"+workingBranch)
	endpoint := "https://api.github.com/repos/" + c.Repo + "/pulls?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	c.auth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var rows []struct {
		Number   int        `json:"number"`
		HTMLURL  string     `json:"html_url"`
		State    string     `json:"state"`
		Merged   bool       `json:"merged"`
		MergedAt *time.Time `json:"merged_at"`
		Head     struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := decodeGitHubJSONResponse(resp, &rows, maxGitHubAPIResponseBytes, http.StatusOK); err != nil {
		return nil, err
	}
	if len(rows) > maxGitHubPRCandidates {
		return nil, fmt.Errorf("GitHub PR reconciliation exceeds %d candidates", maxGitHubPRCandidates)
	}
	candidates := make([]GitHubPRResult, 0, len(rows))
	for _, row := range rows {
		state := normalizePullRequestState(row.State, row.MergedAt)
		if row.Merged {
			state = "merged"
		}
		candidates = append(candidates, GitHubPRResult{
			Number: row.Number, URL: row.HTMLURL, State: state, WorkingBranch: workingBranch,
			BaseBranch: c.BaseBranch, HeadCommitSHA: row.Head.SHA,
		})
	}
	return candidates, nil
}

// preferredPullRequest chooses exact candidates deterministically: an open PR
// wins over a merged PR, which wins over a closed PR. Within the same state,
// the greatest PR number wins; an exact tie retains API order.
func preferredPullRequest(candidates []GitHubPRResult) (GitHubPRResult, bool, error) {
	if len(candidates) == 0 {
		return GitHubPRResult{}, false, nil
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		candidatePriority := pullRequestStatePriority(candidate.State)
		bestPriority := pullRequestStatePriority(best.State)
		if candidatePriority < bestPriority || (candidatePriority == bestPriority && candidate.Number > best.Number) {
			best = candidate
		}
	}
	return best, true, nil
}

func pullRequestStatePriority(state string) int {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "open":
		return 0
	case "merged":
		return 1
	case "closed":
		return 2
	default:
		return 3
	}
}

func normalizePullRequestState(state string, mergedAt *time.Time) string {
	if mergedAt != nil && !mergedAt.IsZero() {
		return "merged"
	}
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "open":
		return "open"
	case "closed":
		return "closed"
	default:
		return strings.ToLower(strings.TrimSpace(state))
	}
}

func (c *GitHubPRClient) doJSON(ctx context.Context, method, endpoint string, payload any, out any, allowed ...int) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	c.auth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeGitHubJSONResponse(resp, out, maxGitHubAPIResponseBytes, allowed...)
}

func (c *GitHubPRClient) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

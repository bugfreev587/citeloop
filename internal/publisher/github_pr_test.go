package publisher

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

func TestGitHubPRClientCreatesBranchWritesExistingFileAndOpensPR(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	var calls []string
	var putBody map[string]any
	var prBody map[string]any
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.Method+" "+req.URL.Path+"?"+req.URL.RawQuery)
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/contents/content/citeloop/blog/evidence.mdx":
			if req.URL.Query().Get("ref") != "main" {
				t.Fatalf("content lookup ref = %s", req.URL.RawQuery)
			}
			encoded := base64.StdEncoding.EncodeToString([]byte("# Existing"))
			return jsonResponse(http.StatusOK, `{"sha":"base-file-sha","content":"`+encoded+`"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/ref/heads/main":
			return jsonResponse(http.StatusOK, `{"object":{"sha":"base-commit-sha"}}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/refs":
			return jsonResponse(http.StatusCreated, `{}`), nil
		case req.Method == http.MethodPut && req.URL.Path == "/repos/owner/unipost/contents/content/citeloop/blog/evidence.mdx":
			raw, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(raw, &putBody); err != nil {
				t.Fatalf("unmarshal PUT body: %v", err)
			}
			return jsonResponse(http.StatusOK, `{"commit":{"sha":"head-commit-sha"}}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/pulls":
			raw, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(raw, &prBody); err != nil {
				t.Fatalf("unmarshal PR body: %v", err)
			}
			return jsonResponse(http.StatusCreated, `{"number":42,"html_url":"https://github.com/owner/unipost/pull/42","state":"open"}`), nil
		default:
			t.Fatalf("unexpected request %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
			return nil, nil
		}
	})}

	result, err := client.CreatePageUpdatePR(context.Background(), GitHubPRInput{
		SourcePath:        "content/citeloop/blog/evidence.mdx",
		WorkingBranch:     "citeloop/unipost/page-update-abc123",
		BaseFileSHA:       "base-file-sha",
		ProposedContentMD: "# Existing\n\n## Evidence\n\nSource-backed proof.",
		CommitMessage:     "Improve evidence on existing page",
		Title:             "CiteLoop: strengthen evidence on existing page",
		Body:              "## What Changed\n\nAdded evidence.",
	})
	if err != nil {
		t.Fatalf("CreatePageUpdatePR returned error: %v", err)
	}

	if putBody["branch"] != "citeloop/unipost/page-update-abc123" {
		t.Fatalf("PUT branch = %#v", putBody["branch"])
	}
	if putBody["sha"] != "base-file-sha" {
		t.Fatalf("PUT sha = %#v", putBody["sha"])
	}
	decoded, err := base64.StdEncoding.DecodeString(putBody["content"].(string))
	if err != nil {
		t.Fatalf("PUT content is not base64: %v", err)
	}
	if !strings.Contains(string(decoded), "## Evidence") {
		t.Fatalf("PUT content missing evidence update:\n%s", string(decoded))
	}
	if prBody["head"] != "citeloop/unipost/page-update-abc123" || prBody["base"] != "main" {
		t.Fatalf("PR head/base = %#v/%#v", prBody["head"], prBody["base"])
	}
	if result.Number != 42 || result.URL != "https://github.com/owner/unipost/pull/42" || result.HeadCommitSHA != "head-commit-sha" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if strings.Join(calls, "\n") == "" {
		t.Fatal("expected GitHub API calls")
	}
}

func TestGitHubPRClientReadsFileContentAndSHA(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/repos/owner/unipost/contents/content/citeloop/blog/evidence.mdx" {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		if req.URL.Query().Get("ref") != "main" {
			t.Fatalf("content lookup ref = %s", req.URL.RawQuery)
		}
		encoded := base64.StdEncoding.EncodeToString([]byte("---\ntitle: \"Old\"\n---\n\nBody"))
		return jsonResponse(http.StatusOK, `{"sha":"base-file-sha","content":"`+encoded+`"}`), nil
	})}

	content, sha, err := client.ReadFile(context.Background(), "content/citeloop/blog/evidence.mdx", "main")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if sha != "base-file-sha" {
		t.Fatalf("sha = %q", sha)
	}
	if !strings.Contains(content, `title: "Old"`) || !strings.Contains(content, "Body") {
		t.Fatalf("content not decoded: %q", content)
	}
}

func TestGitHubPRClientRejectsBaseFileSHAMismatch(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected mutation request after sha mismatch: %s %s", req.Method, req.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"sha":"new-file-sha"}`), nil
	})}

	_, err := client.CreatePageUpdatePR(context.Background(), GitHubPRInput{
		SourcePath:        "content/citeloop/blog/evidence.mdx",
		WorkingBranch:     "citeloop/unipost/page-update-abc123",
		BaseFileSHA:       "base-file-sha",
		ProposedContentMD: "# Updated",
		CommitMessage:     "Improve evidence on existing page",
		Title:             "CiteLoop: strengthen evidence on existing page",
		Body:              "body",
	})
	if err == nil || !strings.Contains(err.Error(), "source file changed") {
		t.Fatalf("expected source file changed error, got %v", err)
	}
}

func TestGitHubPRClientReusesExistingDraftBranch(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	committed := false
	var putBody map[string]any
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/contents/content/citeloop/blog/evidence.mdx":
			if req.URL.Query().Get("ref") == "citeloop/unipost/page-update-abc123" {
				return jsonResponse(http.StatusOK, `{"sha":"branch-file-sha"}`), nil
			}
			return jsonResponse(http.StatusOK, `{"sha":"base-file-sha"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/ref/heads/main":
			return jsonResponse(http.StatusOK, `{"object":{"sha":"base-commit-sha"}}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/refs":
			return jsonResponse(http.StatusUnprocessableEntity, `{"message":"Reference already exists"}`), nil
		case req.Method == http.MethodPut && req.URL.Path == "/repos/owner/unipost/contents/content/citeloop/blog/evidence.mdx":
			raw, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(raw, &putBody); err != nil {
				t.Fatalf("unmarshal PUT body: %v", err)
			}
			committed = true
			return jsonResponse(http.StatusOK, `{"commit":{"sha":"head-commit-sha"}}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/pulls":
			return jsonResponse(http.StatusCreated, `{"number":42,"html_url":"https://github.com/owner/unipost/pull/42","state":"open"}`), nil
		default:
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})}

	_, err := client.CreatePageUpdatePR(context.Background(), GitHubPRInput{
		SourcePath:        "content/citeloop/blog/evidence.mdx",
		WorkingBranch:     "citeloop/unipost/page-update-abc123",
		BaseFileSHA:       "base-file-sha",
		ProposedContentMD: "# Updated",
		CommitMessage:     "Improve evidence on existing page",
		Title:             "CiteLoop: strengthen evidence on existing page",
		Body:              "body",
	})
	if err != nil {
		t.Fatalf("CreatePageUpdatePR should reuse existing branch, got %v", err)
	}
	if !committed {
		t.Fatal("expected client to continue and commit to existing draft branch")
	}
	if putBody["sha"] != "branch-file-sha" {
		t.Fatalf("PUT sha must use existing branch file SHA, got %#v", putBody["sha"])
	}
}

func TestGitHubPRClientReconcilesOrphanBranchWithoutDuplicateCommit(t *testing.T) {
	putCalls := 0
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/contents/page.md" && req.URL.Query().Get("ref") == "main":
			return jsonResponse(http.StatusOK, `{"sha":"base-file-sha"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/ref/heads/main":
			return jsonResponse(http.StatusOK, `{"object":{"sha":"base-commit-sha"}}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/refs":
			return jsonResponse(http.StatusUnprocessableEntity, `{"message":"Reference already exists"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/contents/page.md" && req.URL.Query().Get("ref") == "citeloop/orphan":
			return jsonResponse(http.StatusOK, `{"sha":"branch-file-sha","encoding":"base64","content":"IyBVcGRhdGVk"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/ref/heads/citeloop/orphan":
			return jsonResponse(http.StatusOK, `{"object":{"sha":"orphan-head-sha"}}`), nil
		case req.Method == http.MethodPut:
			putCalls++
			return jsonResponse(http.StatusOK, `{"commit":{"sha":"duplicate"}}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/pulls":
			return jsonResponse(http.StatusCreated, `{"number":43,"html_url":"https://github.com/owner/unipost/pull/43","state":"open"}`), nil
		default:
			t.Fatalf("unexpected request %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
			return nil, nil
		}
	})}
	result, err := client.CreatePageUpdatePR(context.Background(), GitHubPRInput{SourcePath: "page.md", WorkingBranch: "citeloop/orphan", BaseFileSHA: "base-file-sha", ProposedContentMD: "# Updated", CommitMessage: "fix", Title: "fix", Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	if putCalls != 0 || result.HeadCommitSHA != "orphan-head-sha" {
		t.Fatalf("putCalls=%d result=%+v", putCalls, result)
	}
}

func TestGitHubPRClientGetPullRequestReportsMerge(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/repos/owner/unipost/pulls/42" {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
		return jsonResponse(http.StatusOK, `{"number":42,"state":"closed","merged":true,"merged_at":"2026-07-09T10:00:00Z","html_url":"https://github.com/owner/unipost/pull/42"}`), nil
	})}

	pr, err := client.GetPullRequest(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetPullRequest returned error: %v", err)
	}
	if !pr.Merged || pr.State != "closed" || pr.Number != 42 {
		t.Fatalf("unexpected PR state: %#v", pr)
	}
	if pr.MergedAt == nil || pr.MergedAt.IsZero() {
		t.Fatalf("expected merged_at to be parsed, got %#v", pr.MergedAt)
	}
}

func TestGitHubPRClientGetPullRequestOpenNotMerged(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"number":7,"state":"open","merged":false,"merged_at":null,"html_url":"https://github.com/owner/unipost/pull/7"}`), nil
	})}

	pr, err := client.GetPullRequest(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetPullRequest returned error: %v", err)
	}
	if pr.Merged || pr.State != "open" {
		t.Fatalf("open PR should not report merged: %#v", pr)
	}
	if pr.MergedAt != nil {
		t.Fatalf("open PR should have nil merged_at, got %#v", pr.MergedAt)
	}
}

func TestGitHubPRClientGetPullRequestRequiresNumber(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	if _, err := client.GetPullRequest(context.Background(), 0); err == nil {
		t.Fatal("expected error for zero PR number")
	}
}

func TestGitHubPRClientFindsExistingPullRequestByDeterministicHead(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/repos/owner/unipost/pulls" || req.URL.Query().Get("head") != "owner:citeloop/doctor-site-fix-abc" {
			t.Fatalf("unexpected request %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
		}
		return jsonResponse(http.StatusOK, `[{"number":42,"html_url":"https://github.com/owner/unipost/pull/42","state":"open","head":{"sha":"head-sha"}}]`), nil
	})}
	result, found, err := client.FindPullRequestByHead(context.Background(), "citeloop/doctor-site-fix-abc")
	if err != nil || !found || result.Number != 42 || result.HeadCommitSHA != "head-sha" {
		t.Fatalf("result=%+v found=%v err=%v", result, found, err)
	}
}

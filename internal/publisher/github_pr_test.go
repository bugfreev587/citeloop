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

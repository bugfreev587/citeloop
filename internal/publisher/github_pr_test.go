package publisher

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func TestGitHubPRClientListsRecursiveTreeAndReadsPinnedBlob(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/trees/main":
			if req.URL.Query().Get("recursive") != "1" {
				t.Fatalf("tree request is not recursive: %s", req.URL.RawQuery)
			}
			return jsonResponse(http.StatusOK, `{
				"sha":"base-tree",
				"truncated":false,
				"tree":[
					{"path":"app","mode":"040000","type":"tree","sha":"dir-sha"},
					{"path":"app/page.tsx","mode":"100644","type":"blob","sha":"blob-a","size":123},
					{"path":"scripts/build.sh","mode":"100755","type":"blob","sha":"blob-b","size":45}
				]
			}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/blobs/blob-a":
			encoded := base64.StdEncoding.EncodeToString([]byte("export default function Page() {}"))
			return jsonResponse(http.StatusOK, `{"sha":"blob-a","encoding":"base64","content":"`+encoded+`","size":33}`), nil
		default:
			t.Fatalf("unexpected request %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
			return nil, nil
		}
	})}

	entries, err := client.ListTree(context.Background(), "main")
	if err != nil {
		t.Fatalf("ListTree returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ListTree must expose only blobs, got %+v", entries)
	}
	if entries[0] != (GitHubTreeEntry{Path: "app/page.tsx", Mode: "100644", Type: "blob", SHA: "blob-a", Size: 123}) {
		t.Fatalf("first tree entry = %+v", entries[0])
	}
	if entries[1].Mode != "100755" {
		t.Fatalf("executable mode was not preserved: %+v", entries[1])
	}

	content, err := client.ReadBlob(context.Background(), "blob-a")
	if err != nil {
		t.Fatalf("ReadBlob returned error: %v", err)
	}
	if string(content) != "export default function Page() {}" {
		t.Fatalf("ReadBlob content = %q", content)
	}
}

func TestGitHubPRClientRejectsTruncatedTree(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"sha":"tree","truncated":true,"tree":[{"path":"app/page.tsx","mode":"100644","type":"blob","sha":"blob-a"}]}`), nil
	})}

	_, err := client.ListTree(context.Background(), "main")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "truncated") {
		t.Fatalf("expected a truncated-tree error, got %v", err)
	}
}

func TestGitHubPRClientPreservesRawGitTreePath(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"sha":"tree","truncated":false,"tree":[{"path":"pages/a%2Fb.tsx","mode":"100644","type":"blob","sha":"blob-a"}]}`), nil
	})}

	entries, err := client.ListTree(context.Background(), "main")
	if err != nil {
		t.Fatalf("ListTree returned error: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "pages/a%2Fb.tsx" {
		t.Fatalf("raw Git tree path was rewritten: %+v", entries)
	}

	input := atomicFileUpdatesInput()
	input.Files = []GitHubFileUpdate{{Path: "pages/a%2Fb.tsx", BaseBlobSHA: "blob-a", Content: []byte("updated")}}
	prepared, err := prepareGitHubFileUpdatesInput(input)
	if err != nil {
		t.Fatalf("prepareGitHubFileUpdatesInput returned error: %v", err)
	}
	if prepared.Files[0].Path != "pages/a%2Fb.tsx" {
		t.Fatalf("prepared raw path = %q", prepared.Files[0].Path)
	}
}

func TestGitHubPRClientRejectsUnsafeRawGitTreePaths(t *testing.T) {
	for _, path := range []string{"/absolute.tsx", "pages/../secret.ts", "pages/./page.tsx", "pages\\page.tsx", "pages//page.tsx", "pages/\x00page.tsx", "pages/\x1fpage.tsx"} {
		t.Run(fmt.Sprintf("%q", path), func(t *testing.T) {
			input := atomicFileUpdatesInput()
			input.Files[0].Path = path
			if _, err := prepareGitHubFileUpdatesInput(input); err == nil {
				t.Fatalf("expected unsafe raw path %q to be rejected", path)
			}
		})
	}
}

func TestGitHubPRClientAtomicTreePayloadPreservesPercentEncodedFilename(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	var payloadPath string
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/pulls":
			return jsonResponse(http.StatusOK, `[]`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/ref/heads/main":
			return jsonResponse(http.StatusOK, `{"object":{"sha":"base-commit"}}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/commits/base-commit":
			return jsonResponse(http.StatusOK, `{"sha":"base-commit","tree":{"sha":"base-tree"}}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/trees/base-tree":
			return jsonResponse(http.StatusOK, `{"sha":"base-tree","truncated":false,"tree":[{"path":"pages/a%2Fb.tsx","mode":"100644","type":"blob","sha":"old-a"}]}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/blobs":
			return jsonResponse(http.StatusCreated, `{"sha":"new-a"}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/trees":
			var body struct {
				Tree []struct {
					Path string `json:"path"`
				} `json:"tree"`
			}
			decodeRequestJSON(t, req, &body)
			if len(body.Tree) != 1 {
				t.Fatalf("tree payload = %+v", body)
			}
			payloadPath = body.Tree[0].Path
			return jsonResponse(http.StatusCreated, `{"sha":"desired-tree"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/ref/heads/citeloop/doctor-site-fix-abc":
			return jsonResponse(http.StatusNotFound, `{}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/commits":
			return jsonResponse(http.StatusCreated, `{"sha":"desired-commit"}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/refs":
			return jsonResponse(http.StatusCreated, `{}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/pulls":
			return jsonResponse(http.StatusCreated, `{"number":53,"html_url":"https://github.com/owner/unipost/pull/53","state":"open"}`), nil
		default:
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})}
	input := atomicFileUpdatesInput()
	input.Files = []GitHubFileUpdate{{Path: "pages/a%2Fb.tsx", BaseBlobSHA: "old-a", Content: []byte("updated")}}

	if _, err := client.CreateFileUpdatesPR(context.Background(), input); err != nil {
		t.Fatalf("CreateFileUpdatesPR returned error: %v", err)
	}
	if payloadPath != "pages/a%2Fb.tsx" {
		t.Fatalf("tree payload path was rewritten: %q", payloadPath)
	}
}

func TestGitHubPRClientCreatesAtomicFileUpdatesPR(t *testing.T) {
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	counts := map[string]int{}
	var treeBody struct {
		BaseTree string `json:"base_tree"`
		Tree     []struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"tree"`
	}
	var commitBody struct {
		Message string   `json:"message"`
		Tree    string   `json:"tree"`
		Parents []string `json:"parents"`
	}
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/pulls":
			counts["find-pr"]++
			return jsonResponse(http.StatusOK, `[]`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/ref/heads/main":
			return jsonResponse(http.StatusOK, `{"object":{"sha":"base-commit"}}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/commits/base-commit":
			return jsonResponse(http.StatusOK, `{"sha":"base-commit","tree":{"sha":"base-tree"},"parents":[{"sha":"previous"}]}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/trees/base-tree":
			if req.URL.Query().Get("recursive") != "1" {
				t.Fatalf("base tree request is not recursive")
			}
			return jsonResponse(http.StatusOK, `{"sha":"base-tree","truncated":false,"tree":[
				{"path":"app/page.tsx","mode":"100644","type":"blob","sha":"old-a","size":10},
				{"path":"scripts/build.sh","mode":"100755","type":"blob","sha":"old-b","size":20}
			]}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/blobs":
			counts["blob"]++
			var body struct {
				Content  string `json:"content"`
				Encoding string `json:"encoding"`
			}
			decodeRequestJSON(t, req, &body)
			if body.Encoding != "base64" {
				t.Fatalf("blob encoding = %q", body.Encoding)
			}
			decoded, err := base64.StdEncoding.DecodeString(body.Content)
			if err != nil {
				t.Fatalf("decode blob body: %v", err)
			}
			switch string(decoded) {
			case "new page":
				return jsonResponse(http.StatusCreated, `{"sha":"new-a"}`), nil
			case "#!/bin/sh\necho fixed\n":
				return jsonResponse(http.StatusCreated, `{"sha":"new-b"}`), nil
			default:
				t.Fatalf("unexpected blob content %q", decoded)
				return nil, nil
			}
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/trees":
			counts["tree"]++
			decodeRequestJSON(t, req, &treeBody)
			return jsonResponse(http.StatusCreated, `{"sha":"desired-tree"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/ref/heads/citeloop/doctor-site-fix-abc":
			return jsonResponse(http.StatusNotFound, `{"message":"Not Found"}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/commits":
			counts["commit"]++
			decodeRequestJSON(t, req, &commitBody)
			return jsonResponse(http.StatusCreated, `{"sha":"desired-commit","tree":{"sha":"desired-tree"}}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/refs":
			counts["ref"]++
			var body map[string]string
			decodeRequestJSON(t, req, &body)
			if body["ref"] != "refs/heads/citeloop/doctor-site-fix-abc" || body["sha"] != "desired-commit" {
				t.Fatalf("unexpected ref body: %+v", body)
			}
			return jsonResponse(http.StatusCreated, `{"ref":"refs/heads/citeloop/doctor-site-fix-abc","object":{"sha":"desired-commit"}}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/pulls":
			counts["pr"]++
			return jsonResponse(http.StatusCreated, `{"number":51,"html_url":"https://github.com/owner/unipost/pull/51","state":"open","head":{"sha":"desired-commit"}}`), nil
		default:
			t.Fatalf("unexpected request %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
			return nil, nil
		}
	})}

	result, err := client.CreateFileUpdatesPR(context.Background(), atomicFileUpdatesInput())
	if err != nil {
		t.Fatalf("CreateFileUpdatesPR returned error: %v", err)
	}
	for key, want := range map[string]int{"blob": 2, "tree": 1, "commit": 1, "ref": 1, "pr": 1} {
		if counts[key] != want {
			t.Errorf("%s calls = %d, want %d", key, counts[key], want)
		}
	}
	if treeBody.BaseTree != "base-tree" || len(treeBody.Tree) != 2 {
		t.Fatalf("unexpected tree body: %+v", treeBody)
	}
	if treeBody.Tree[0].Path != "app/page.tsx" || treeBody.Tree[0].Mode != "100644" || treeBody.Tree[0].Type != "blob" || treeBody.Tree[0].SHA != "new-a" {
		t.Fatalf("page tree entry = %+v", treeBody.Tree[0])
	}
	if treeBody.Tree[1].Path != "scripts/build.sh" || treeBody.Tree[1].Mode != "100755" || treeBody.Tree[1].SHA != "new-b" {
		t.Fatalf("executable tree entry did not preserve mode: %+v", treeBody.Tree[1])
	}
	if commitBody.Tree != "desired-tree" || len(commitBody.Parents) != 1 || commitBody.Parents[0] != "base-commit" {
		t.Fatalf("commit is not based on current base: %+v", commitBody)
	}
	if result.Number != 51 || result.State != "open" || result.HeadCommitSHA != "desired-commit" || result.BaseBranch != "main" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGitHubPRClientAtomicFileUpdatesRejectsSHAMismatchWithoutMutation(t *testing.T) {
	client := newAtomicPreparationClient(t, "", func(req *http.Request, counts map[string]int) (*http.Response, error) {
		if req.Method == http.MethodPost {
			counts["mutation"]++
		}
		return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
	})
	input := atomicFileUpdatesInput()
	input.Files[0].BaseBlobSHA = "stale-a"

	_, err := client.CreateFileUpdatesPR(context.Background(), input)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "changed") {
		t.Fatalf("expected source-change error, got %v", err)
	}
	counts := clientTestCounts(client)
	if counts["mutation"] != 0 || counts["ref"] != 0 {
		t.Fatalf("validation failure mutated GitHub: %+v", counts)
	}
}

func TestGitHubPRClientAtomicPreparationFailureDoesNotMutateRef(t *testing.T) {
	for _, phase := range []string{"blob", "tree", "commit"} {
		t.Run(phase, func(t *testing.T) {
			client := newAtomicPreparationClient(t, phase, nil)
			_, err := client.CreateFileUpdatesPR(context.Background(), atomicFileUpdatesInput())
			if err == nil || !strings.Contains(err.Error(), "injected "+phase+" failure") {
				t.Fatalf("expected injected %s error, got %v", phase, err)
			}
			if got := clientTestCounts(client)["ref"]; got != 0 {
				t.Fatalf("%s failure mutated ref %d times", phase, got)
			}
		})
	}
}

func TestGitHubPRClientReusesAtomicOrphanBranchAfterLostResponse(t *testing.T) {
	client := newAtomicPreparationClient(t, "orphan", nil)
	result, err := client.CreateFileUpdatesPR(context.Background(), atomicFileUpdatesInput())
	if err != nil {
		t.Fatalf("CreateFileUpdatesPR returned error: %v", err)
	}
	counts := clientTestCounts(client)
	if counts["commit"] != 0 || counts["ref"] != 0 || counts["pr"] != 1 {
		t.Fatalf("orphan reconciliation calls = %+v", counts)
	}
	if result.HeadCommitSHA != "orphan-commit" || result.Number != 52 {
		t.Fatalf("unexpected reconciled result: %+v", result)
	}
}

func TestGitHubPRClientRejectsDivergentAtomicBranchWithoutRefUpdate(t *testing.T) {
	client := newAtomicPreparationClient(t, "divergent", nil)
	_, err := client.CreateFileUpdatesPR(context.Background(), atomicFileUpdatesInput())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "divergent") {
		t.Fatalf("expected divergent-branch error, got %v", err)
	}
	counts := clientTestCounts(client)
	if counts["commit"] != 0 || counts["ref"] != 0 || counts["pr"] != 0 {
		t.Fatalf("divergent branch was mutated: %+v", counts)
	}
}

func TestGitHubPRClientAtomicExistingPRMustMatchDesiredTarget(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mergedAt  string
		headTree  string
		wantState string
		wantError bool
	}{
		{name: "matching closed", mergedAt: "null", headTree: "desired-tree", wantState: "closed"},
		{name: "matching merged", mergedAt: `"2026-07-12T12:00:00Z"`, headTree: "desired-tree", wantState: "merged"},
		{name: "mismatching head", mergedAt: "null", headTree: "unrelated-tree", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
			calls := map[string]int{}
			client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/pulls":
					return jsonResponse(http.StatusOK, `[{"number":44,"html_url":"https://github.com/owner/unipost/pull/44","state":"closed","merged_at":`+tc.mergedAt+`,"head":{"sha":"pr-head"}}]`), nil
				case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/ref/heads/main":
					return jsonResponse(http.StatusOK, `{"object":{"sha":"base-commit"}}`), nil
				case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/commits/base-commit":
					return jsonResponse(http.StatusOK, `{"sha":"base-commit","tree":{"sha":"base-tree"}}`), nil
				case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/trees/base-tree":
					return jsonResponse(http.StatusOK, `{"sha":"base-tree","truncated":false,"tree":[{"path":"app/page.tsx","mode":"100644","type":"blob","sha":"old-a"},{"path":"scripts/build.sh","mode":"100755","type":"blob","sha":"old-b"}]}`), nil
				case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/blobs":
					calls["blob"]++
					return jsonResponse(http.StatusCreated, fmt.Sprintf(`{"sha":"new-%d"}`, calls["blob"])), nil
				case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/trees":
					calls["tree"]++
					return jsonResponse(http.StatusCreated, `{"sha":"desired-tree"}`), nil
				case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/commits/pr-head":
					calls["head-check"]++
					return jsonResponse(http.StatusOK, `{"sha":"pr-head","tree":{"sha":"`+tc.headTree+`"},"parents":[{"sha":"base-commit"}]}`), nil
				case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/refs":
					calls["ref"]++
					return jsonResponse(http.StatusCreated, `{}`), nil
				case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/pulls":
					calls["pr"]++
					return jsonResponse(http.StatusCreated, `{}`), nil
				default:
					t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
					return nil, nil
				}
			})}

			result, err := client.CreateFileUpdatesPR(context.Background(), atomicFileUpdatesInput())
			if tc.wantError {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), "divergent") {
					t.Fatalf("expected divergent existing PR error, got result=%+v err=%v", result, err)
				}
			} else {
				if err != nil {
					t.Fatalf("CreateFileUpdatesPR returned error: %v", err)
				}
				if result.State != tc.wantState || result.Number != 44 {
					t.Fatalf("result=%+v", result)
				}
			}
			if calls["head-check"] != 1 || calls["ref"] != 0 || calls["pr"] != 0 {
				t.Fatalf("existing PR reconciliation calls=%+v", calls)
			}
		})
	}
}

func TestGitHubPRClientAtomicInputHasStableFileOrderWithoutMutatingCaller(t *testing.T) {
	input := atomicFileUpdatesInput()
	input.Files[0], input.Files[1] = input.Files[1], input.Files[0]

	prepared, err := prepareGitHubFileUpdatesInput(input)
	if err != nil {
		t.Fatalf("prepareGitHubFileUpdatesInput returned error: %v", err)
	}
	if prepared.Files[0].Path != "app/page.tsx" || prepared.Files[1].Path != "scripts/build.sh" {
		t.Fatalf("prepared file order is not stable: %+v", prepared.Files)
	}
	if input.Files[0].Path != "scripts/build.sh" || input.Files[1].Path != "app/page.tsx" {
		t.Fatalf("input was mutated: %+v", input.Files)
	}
}

type atomicClientTestState struct {
	counts map[string]int
}

// atomicClientStates is scoped to the test process and indexed by the client
// pointer solely so the table-driven transport helper can expose call counts.
var atomicClientStates = map[*GitHubPRClient]*atomicClientTestState{}

func atomicFileUpdatesInput() GitHubFileUpdatesPRInput {
	return GitHubFileUpdatesPRInput{
		WorkingBranch: "citeloop/doctor-site-fix-abc",
		BaseCommitSHA: "base-commit",
		Files: []GitHubFileUpdate{
			{Path: "app/page.tsx", BaseBlobSHA: "old-a", Content: []byte("new page")},
			{Path: "scripts/build.sh", BaseBlobSHA: "old-b", Content: []byte("#!/bin/sh\necho fixed\n")},
		},
		CommitMessage: "fix: apply CiteLoop Doctor Site Fix",
		Title:         "Apply CiteLoop Doctor Site Fix",
		Body:          "Repository-grounded changes.",
	}
}

func newAtomicPreparationClient(t *testing.T, failurePhase string, unexpected func(*http.Request, map[string]int) (*http.Response, error)) *GitHubPRClient {
	t.Helper()
	client := NewGitHubPRClient("gh-token", "owner/unipost", "main", slog.Default())
	state := &atomicClientTestState{counts: map[string]int{}}
	atomicClientStates[client] = state
	t.Cleanup(func() { delete(atomicClientStates, client) })
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		counts := state.counts
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/pulls":
			return jsonResponse(http.StatusOK, `[]`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/ref/heads/main":
			return jsonResponse(http.StatusOK, `{"object":{"sha":"base-commit"}}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/commits/base-commit":
			return jsonResponse(http.StatusOK, `{"sha":"base-commit","tree":{"sha":"base-tree"}}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/trees/base-tree":
			return jsonResponse(http.StatusOK, `{"sha":"base-tree","truncated":false,"tree":[
				{"path":"app/page.tsx","mode":"100644","type":"blob","sha":"old-a"},
				{"path":"scripts/build.sh","mode":"100755","type":"blob","sha":"old-b"}
			]}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/blobs":
			counts["mutation"]++
			counts["blob"]++
			if failurePhase == "blob" {
				return jsonResponse(http.StatusInternalServerError, `{"message":"injected blob failure"}`), nil
			}
			return jsonResponse(http.StatusCreated, fmt.Sprintf(`{"sha":"new-%d"}`, counts["blob"])), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/trees":
			counts["mutation"]++
			counts["tree"]++
			if failurePhase == "tree" {
				return jsonResponse(http.StatusInternalServerError, `{"message":"injected tree failure"}`), nil
			}
			return jsonResponse(http.StatusCreated, `{"sha":"desired-tree"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/ref/heads/citeloop/doctor-site-fix-abc":
			switch failurePhase {
			case "orphan":
				return jsonResponse(http.StatusOK, `{"object":{"sha":"orphan-commit"}}`), nil
			case "divergent":
				return jsonResponse(http.StatusOK, `{"object":{"sha":"divergent-commit"}}`), nil
			default:
				return jsonResponse(http.StatusNotFound, `{"message":"Not Found"}`), nil
			}
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/commits/orphan-commit":
			return jsonResponse(http.StatusOK, `{"sha":"orphan-commit","tree":{"sha":"desired-tree"},"parents":[{"sha":"base-commit"}]}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/repos/owner/unipost/git/commits/divergent-commit":
			return jsonResponse(http.StatusOK, `{"sha":"divergent-commit","tree":{"sha":"other-tree"},"parents":[{"sha":"base-commit"}]}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/commits":
			counts["mutation"]++
			counts["commit"]++
			if failurePhase == "commit" {
				return jsonResponse(http.StatusInternalServerError, `{"message":"injected commit failure"}`), nil
			}
			return jsonResponse(http.StatusCreated, `{"sha":"desired-commit"}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/git/refs":
			counts["mutation"]++
			counts["ref"]++
			return jsonResponse(http.StatusCreated, `{}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/repos/owner/unipost/pulls":
			counts["mutation"]++
			counts["pr"]++
			return jsonResponse(http.StatusCreated, `{"number":52,"html_url":"https://github.com/owner/unipost/pull/52","state":"open"}`), nil
		default:
			if unexpected != nil {
				return unexpected(req, counts)
			}
			t.Fatalf("unexpected request %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
			return nil, nil
		}
	})}
	return client
}

func clientTestCounts(client *GitHubPRClient) map[string]int {
	return atomicClientStates[client].counts
}

func decodeRequestJSON(t *testing.T, req *http.Request, out any) {
	t.Helper()
	if err := json.NewDecoder(req.Body).Decode(out); err != nil {
		t.Fatalf("decode %s %s body: %v", req.Method, req.URL.Path, err)
	}
}

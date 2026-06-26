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

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBlogPublisherRejectsUnsafeGeneratedMDX(t *testing.T) {
	article := testArticle(t)
	article.ContentMd = "# Bad\n\n<script>alert('xss')</script>\n"
	blog := NewBlog("", "", "citeloop-content", "https://dev.unipost.dev/blog", "content/citeloop/blog", slog.Default())

	_, err := blog.Publish(context.Background(), article)

	if err == nil {
		t.Fatal("expected script tag to be rejected")
	}
	if !strings.Contains(err.Error(), "unsafe generated mdx") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBlogPublisherRejectsUnsafeContentDir(t *testing.T) {
	article := testArticle(t)
	blog := NewBlog("", "", "citeloop-content", "https://dev.unipost.dev/blog", "../outside", slog.Default())

	_, err := blog.Publish(context.Background(), article)

	if err == nil {
		t.Fatal("expected content dir traversal to be rejected")
	}
	if !strings.Contains(err.Error(), "content dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBlogPublisherRequiresConfiguredGitHubTarget(t *testing.T) {
	article := testArticle(t)
	blog := NewBlog("", "", "citeloop-content", "https://dev.unipost.dev/blog", "content/citeloop/blog", slog.Default())

	_, err := blog.Publish(context.Background(), article)

	if err == nil || !strings.Contains(err.Error(), "publisher credential") {
		t.Fatalf("expected configured publisher error, got %v", err)
	}
}

func TestBlogPublisherUsesContentDirAndUpdateSHA(t *testing.T) {
	article := testArticle(t)
	blog := NewBlog("gh-token", "owner/unipost", "citeloop-content", "https://dev.unipost.dev/blog", "content/citeloop/blog", slog.Default())

	var putBody map[string]any
	blog.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			if req.URL.Path != "/repos/owner/unipost/contents/content/citeloop/blog/my-post.mdx" {
				t.Fatalf("GET path = %s", req.URL.Path)
			}
			if req.URL.Query().Get("ref") != "citeloop-content" {
				t.Fatalf("GET ref = %s", req.URL.RawQuery)
			}
			return jsonResponse(http.StatusOK, `{"sha":"old-file-sha"}`), nil
		case http.MethodPut:
			if req.URL.Path != "/repos/owner/unipost/contents/content/citeloop/blog/my-post.mdx" {
				t.Fatalf("PUT path = %s", req.URL.Path)
			}
			raw, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(raw, &putBody); err != nil {
				t.Fatalf("unmarshal PUT body: %v", err)
			}
			return jsonResponse(http.StatusOK, `{"commit":{"sha":"new-commit-sha"}}`), nil
		default:
			t.Fatalf("unexpected method %s", req.Method)
			return nil, nil
		}
	})}

	result, err := blog.Publish(context.Background(), article)
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	if putBody["sha"] != "old-file-sha" {
		t.Fatalf("PUT sha = %#v", putBody["sha"])
	}
	if putBody["branch"] != "citeloop-content" {
		t.Fatalf("PUT branch = %#v", putBody["branch"])
	}
	message := putBody["message"].(string)
	if !strings.Contains(message, article.ProjectID.String()) || !strings.Contains(message, article.ID.String()) || !strings.Contains(message, "My Post") {
		t.Fatalf("commit message missing identifiers/title: %q", message)
	}
	decoded, err := base64.StdEncoding.DecodeString(putBody["content"].(string))
	if err != nil {
		t.Fatalf("content is not base64: %v", err)
	}
	if !strings.Contains(string(decoded), `citeloop_article_id: "`+article.ID.String()+`"`) {
		t.Fatalf("rendered MDX missing article id frontmatter:\n%s", string(decoded))
	}
	if result.Path != "content/citeloop/blog/my-post.mdx" {
		t.Fatalf("result path = %q", result.Path)
	}
	if result.CommitSHA != "new-commit-sha" {
		t.Fatalf("result commit sha = %q", result.CommitSHA)
	}
}

func TestBlogPublisherReusesResolvedSlugOnRetry(t *testing.T) {
	article := testArticle(t)
	article.ResolvedSlug = ptrString("first-slug")
	article.SeoMeta = json.RawMessage(`{"title":"Changed Post","meta_description":"Meta","slug":"changed-post","h1":"Changed Post"}`)
	blog := NewBlog("gh-token", "owner/unipost", "citeloop-content", "https://dev.unipost.dev/blog", "content/citeloop/blog", slog.Default())
	blog.client = stubGitHubPublishClient(t, "content/citeloop/blog/first-slug.mdx", "citeloop-content")

	result, err := blog.Publish(context.Background(), article)
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if result.Path != "content/citeloop/blog/first-slug.mdx" {
		t.Fatalf("result path = %q", result.Path)
	}
	if result.URL != "https://dev.unipost.dev/blog/first-slug" {
		t.Fatalf("result url = %q", result.URL)
	}
}

func TestBlogPublisherCapsSlugToUniPostRouteContract(t *testing.T) {
	article := testArticle(t)
	rawSlug := "white-label-social-publishing-adding-multi-platform-posting-to-your-saas-without-building-integrations"
	expectedSlug := "white-label-social-publishing-adding-multi-platform-posting-to-your-saas-without-building-integr"
	article.SeoMeta = json.RawMessage(`{"title":"White Label","meta_description":"Meta","slug":"` + rawSlug + `","h1":"White Label"}`)
	blog := NewBlog("gh-token", "owner/unipost", "dev", "https://dev.unipost.dev/blog", "content/citeloop/blog", slog.Default())
	blog.client = stubGitHubPublishClient(t, "content/citeloop/blog/"+expectedSlug+".mdx", "dev")

	result, err := blog.Publish(context.Background(), article)
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	if result.Path != "content/citeloop/blog/"+expectedSlug+".mdx" {
		t.Fatalf("result path = %q", result.Path)
	}
	if result.URL != "https://dev.unipost.dev/blog/"+expectedSlug {
		t.Fatalf("result url = %q", result.URL)
	}
}

func TestBlogPublisherResultMarksPendingURLVerification(t *testing.T) {
	article := testArticle(t)
	blog := NewBlog("gh-token", "owner/unipost", "citeloop-content", "https://dev.unipost.dev/blog", "content/citeloop/blog", slog.Default())
	blog.client = stubGitHubPublishClient(t, "content/citeloop/blog/my-post.mdx", "citeloop-content")

	result, err := blog.Publish(context.Background(), article)
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if result.Phase != "pending_url_verification" {
		t.Fatalf("result phase = %q", result.Phase)
	}
}

func TestBlogPublisherPublishedPathExistsRejectsMissingFile(t *testing.T) {
	blog := NewBlog("gh-token", "owner/unipost", "citeloop-content", "https://dev.unipost.dev/blog", "content/citeloop/blog", slog.Default())
	blog.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", req.Method)
		}
		if req.URL.Path != "/repos/owner/unipost/contents/content/citeloop/blog/missing.mdx" {
			t.Fatalf("path = %s", req.URL.Path)
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	})}

	err := blog.PublishedPathExists(context.Background(), "content/citeloop/blog/missing.mdx")

	if err == nil {
		t.Fatal("expected missing content path to fail")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBlogPublisherPublishedPathExistsRequiresConfiguredGitHubTarget(t *testing.T) {
	blog := NewBlog("", "", "citeloop-content", "https://dev.unipost.dev/blog", "content/citeloop/blog", slog.Default())

	err := blog.PublishedPathExists(context.Background(), "content/citeloop/blog/my-post.mdx")

	if err == nil || !strings.Contains(err.Error(), "publisher credential") {
		t.Fatalf("expected configured publisher error, got %v", err)
	}
}

func testArticle(t *testing.T) *db.Article {
	t.Helper()
	seo := json.RawMessage(`{"title":"My Post","meta_description":"Meta","slug":"my-post","h1":"My Post"}`)
	return &db.Article{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
		TopicID:   uuid.New(),
		Kind:      "canonical",
		ContentMd: "# My Post\n\nSafe body.",
		SeoMeta:   seo,
	}
}

func ptrString(s string) *string { return &s }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func stubGitHubPublishClient(t *testing.T, expectedPath, expectedRef string) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/"+expectedPath) {
			t.Fatalf("%s path = %s", req.Method, req.URL.Path)
		}
		switch req.Method {
		case http.MethodGet:
			if req.URL.Query().Get("ref") != expectedRef {
				t.Fatalf("GET ref = %s", req.URL.RawQuery)
			}
			return jsonResponse(http.StatusNotFound, `{}`), nil
		case http.MethodPut:
			return jsonResponse(http.StatusOK, `{"commit":{"sha":"new-commit-sha"}}`), nil
		default:
			t.Fatalf("unexpected method %s", req.Method)
			return nil, nil
		}
	})}
}

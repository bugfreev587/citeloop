package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/google/uuid"
)

func TestBlogPublisherForProjectWithoutQueryUsesFallbackPublisher(t *testing.T) {
	fallback := publisher.NewBlog("fallback-token", "fallback/repo", "fallback-branch", "https://fallback.example/blog", "fallback/content", slog.Default())
	s := &Scheduler{Blog: fallback}

	blog, err := s.blogPublisherForProject(context.Background(), nil, db.Project{ID: uuid.New()})
	if err != nil {
		t.Fatalf("blogPublisherForProject returned error: %v", err)
	}
	if blog != fallback {
		t.Fatal("expected missing query/connection path to use fallback publisher")
	}
}

func TestBlogPublisherFromConnectionOverridesEnvPublisherConfig(t *testing.T) {
	conn := db.PublisherConnection{
		ID:            uuid.New(),
		ProjectID:     uuid.New(),
		Kind:          publisher.ConnectionKindGitHubNextJS,
		Status:        "connected",
		CredentialRef: ptr("env:GITHUB_TOKEN"),
		Config: json.RawMessage(`{
			"repo":"customer/site",
			"branch":"staging-content",
			"content_dir":"content/blog",
			"base_url":"https://customer.example/blog"
		}`),
	}
	fallback := publisher.NewBlog("fallback-token", "fallback/repo", "fallback-branch", "https://fallback.example/blog", "fallback/content", slog.Default())

	blog, fromConnection, err := blogPublisherFromConnection(fallback, "connection-token", conn, slog.Default())
	if err != nil {
		t.Fatalf("blogPublisherFromConnection returned error: %v", err)
	}
	if !fromConnection {
		t.Fatal("expected publisher to come from connection")
	}
	if blog.Repo != "customer/site" || blog.Branch != "staging-content" || blog.ContentDir != "content/blog" || blog.BaseURL != "https://customer.example/blog" {
		t.Fatalf("blog publisher did not use connection config: %+v", blog)
	}
	if !blogConfiguredWithToken(blog) {
		t.Fatal("expected connection token to configure publisher")
	}
}

func TestBlogPublisherFromConnectionWithoutCredentialDoesNotUseFallbackToken(t *testing.T) {
	conn := db.PublisherConnection{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
		Kind:      publisher.ConnectionKindGitHubNextJS,
		Status:    "missing",
		Config: json.RawMessage(`{
			"repo":"customer/site",
			"base_url":"https://customer.example/blog"
		}`),
	}
	fallback := publisher.NewBlog("fallback-token", "fallback/repo", "fallback-branch", "https://fallback.example/blog", "fallback/content", slog.Default())

	blog, fromConnection, err := blogPublisherFromConnection(fallback, "", conn, slog.Default())
	if err != nil {
		t.Fatalf("blogPublisherFromConnection returned error: %v", err)
	}
	if !fromConnection {
		t.Fatal("expected publisher to come from connection")
	}
	if blog.Repo != "customer/site" {
		t.Fatalf("repo = %q", blog.Repo)
	}
	if blogConfiguredWithToken(blog) {
		t.Fatal("connection publisher without credential must not reuse fallback token")
	}
}

func blogConfiguredWithToken(blog *publisher.BlogPublisher) bool {
	return blog.Token != ""
}

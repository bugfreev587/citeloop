package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/secretbox"
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

func TestBlogPublisherForProjectUsesEncryptedPublisherCredential(t *testing.T) {
	secret := "test-secret"
	token := "ghp_customer_token"
	encrypted, err := secretbox.EncryptString(token, secret)
	if err != nil {
		t.Fatal(err)
	}
	projectID := uuid.New()
	connectionID := uuid.New()
	credentialID := uuid.New()
	store := &publisherConnectionStoreFake{
		conn: db.PublisherConnection{
			ID:            connectionID,
			ProjectID:     projectID,
			Kind:          publisher.ConnectionKindGitHubNextJS,
			Status:        "connected",
			CredentialRef: ptr(publisher.PublisherCredentialRef(credentialID)),
			Config: json.RawMessage(`{
				"repo":"customer/site",
				"branch":"content",
				"content_dir":"content/blog",
				"base_url":"https://customer.example/blog"
			}`),
		},
		cred: db.PublisherCredential{
			ID:             credentialID,
			ProjectID:      projectID,
			ConnectionID:   connectionID,
			Kind:           publisher.CredentialKindGitHubToken,
			EncryptedValue: encrypted,
			RedactedValue:  "gh_****oken",
		},
	}
	fallback := publisher.NewBlog("fallback-token", "fallback/repo", "fallback-branch", "https://fallback.example/blog", "fallback/content", slog.Default())
	s := &Scheduler{Blog: fallback, NotificationSecret: secret, Log: slog.Default()}

	blog, err := s.blogPublisherForProject(context.Background(), store, db.Project{ID: projectID})
	if err != nil {
		t.Fatalf("blogPublisherForProject returned error: %v", err)
	}
	if blog.Token != token {
		t.Fatalf("token = %q, want encrypted project credential", blog.Token)
	}
}

type publisherConnectionStoreFake struct {
	conn db.PublisherConnection
	cred db.PublisherCredential
}

func (f *publisherConnectionStoreFake) GetDefaultPublisherConnectionForProject(context.Context, db.GetDefaultPublisherConnectionForProjectParams) (db.PublisherConnection, error) {
	return f.conn, nil
}

func (f *publisherConnectionStoreFake) GetActivePublisherCredential(context.Context, db.GetActivePublisherCredentialParams) (db.PublisherCredential, error) {
	return f.cred, nil
}

func blogConfiguredWithToken(blog *publisher.BlogPublisher) bool {
	return blog.Token != ""
}

package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/secretbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestBlogPublisherForProjectWithoutConnectionStoreFailsClosed(t *testing.T) {
	fallback := publisher.NewBlog("fallback-token", "fallback/repo", "fallback-branch", "https://fallback.example/blog", "fallback/content", slog.Default())
	s := &Scheduler{Blog: fallback}

	_, err := s.blogPublisherForProject(context.Background(), nil, db.Project{ID: uuid.New()})
	if err == nil || !strings.Contains(err.Error(), "publisher connection store") {
		t.Fatalf("expected publisher connection store error, got %v", err)
	}
}

func TestBlogPublisherForProjectRequiresEnabledConnectionWhenStoreIsAvailable(t *testing.T) {
	projectID := uuid.New()
	store := &publisherConnectionStoreFake{noConnection: true}
	fallback := publisher.NewBlog("fallback-token", "bugfreev587/unipost", "citeloop-content", "https://unipost.dev/blog", "content/citeloop/blog", slog.Default())
	s := &Scheduler{Blog: fallback, Log: slog.Default()}

	_, err := s.blogPublisherForProject(context.Background(), store, db.Project{
		ID:     projectID,
		Config: json.RawMessage(`{"site_url":"https://dev.unipost.dev/"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "enabled publisher connection") {
		t.Fatalf("expected enabled publisher connection error, got %v", err)
	}
}

func TestGithubTokenForProjectSkipsWhenNoConnection(t *testing.T) {
	s := &Scheduler{Log: slog.Default()}

	token, err := s.githubTokenForProject(context.Background(), &publisherConnectionStoreFake{noConnection: true}, db.Project{ID: uuid.New()})
	if err != nil {
		t.Fatalf("no connection should skip silently, got err %v", err)
	}
	if token != "" {
		t.Fatalf("no connection should yield empty token, got %q", token)
	}
}

func TestPublisherCredentialTokenRejectsEnvFallback(t *testing.T) {
	conn := db.PublisherConnection{
		ID:            uuid.New(),
		ProjectID:     uuid.New(),
		Kind:          publisher.ConnectionKindGitHubNextJS,
		Status:        "connected",
		Enabled:       true,
		CredentialRef: ptr("env:GITHUB_TOKEN"),
	}
	fallback := publisher.NewBlog("fallback-token", "fallback/repo", "fallback-branch", "https://fallback.example/blog", "fallback/content", slog.Default())
	s := &Scheduler{Blog: fallback, Log: slog.Default()}

	token, err := s.publisherCredentialToken(context.Background(), &publisherConnectionStoreFake{}, conn)
	if err == nil || !strings.Contains(err.Error(), "project-scoped publisher credential") {
		t.Fatalf("expected env fallback rejection, token=%q err=%v", token, err)
	}
}

func TestBlogPublisherFromConnectionOverridesEnvPublisherConfig(t *testing.T) {
	credentialID := uuid.New()
	conn := db.PublisherConnection{
		ID:            uuid.New(),
		ProjectID:     uuid.New(),
		Kind:          publisher.ConnectionKindGitHubNextJS,
		Status:        "connected",
		Enabled:       true,
		CredentialRef: ptr(publisher.PublisherCredentialRef(credentialID)),
		Config: json.RawMessage(`{
			"repo":"customer/site",
			"branch":"staging-content",
			"content_dir":"content/blog",
			"base_url":"https://customer.example/blog"
		}`),
	}
	fallback := publisher.NewBlog("fallback-token", "fallback/repo", "fallback-branch", "https://fallback.example/blog", "fallback/content", slog.Default())

	blog, fromConnection, err := blogPublisherFromConnection(fallback, "connection-token", conn, slog.Default(), nil)
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

func TestBlogPublisherForProjectTargetOverridesConnectionEnvironment(t *testing.T) {
	projectID := uuid.New()
	secret := "test-secret"
	token := "ghp_customer_token"
	encrypted, err := secretbox.EncryptString(token, secret)
	if err != nil {
		t.Fatal(err)
	}
	connectionID := uuid.New()
	credentialID := uuid.New()
	store := &publisherConnectionStoreFake{
		conn: db.PublisherConnection{
			ID:            connectionID,
			ProjectID:     projectID,
			Kind:          publisher.ConnectionKindGitHubNextJS,
			Status:        "connected",
			Enabled:       true,
			CredentialRef: ptr(publisher.PublisherCredentialRef(credentialID)),
			Config: json.RawMessage(`{
				"repo":"bugfreev587/unipost",
				"branch":"main",
				"content_dir":"content/citeloop/blog",
				"base_url":"https://unipost.dev/blog"
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
	fallback := publisher.NewBlog("fallback-token", "bugfreev587/unipost", "main", "https://unipost.dev/blog", "content/citeloop/blog", slog.Default())
	s := &Scheduler{Blog: fallback, NotificationSecret: secret, Log: slog.Default()}

	blog, err := s.blogPublisherForProject(context.Background(), store, db.Project{
		ID:     projectID,
		Config: json.RawMessage(`{"site_url":"https://staging.unipost.dev"}`),
	})
	if err != nil {
		t.Fatalf("blogPublisherForProject returned error: %v", err)
	}
	if blog.Repo != "bugfreev587/unipost" || blog.Branch != "staging" || blog.BaseURL != "https://staging.unipost.dev/blog" {
		t.Fatalf("blog publisher did not use staging target: %+v", blog)
	}
}

func TestBlogPublisherForProjectRejectsDisabledConnection(t *testing.T) {
	projectID := uuid.New()
	store := &publisherConnectionStoreFake{
		disabledConnection: true,
		conn: db.PublisherConnection{
			ID:            uuid.New(),
			ProjectID:     projectID,
			Kind:          publisher.ConnectionKindGitHubNextJS,
			Status:        "connected",
			Enabled:       false,
			CredentialRef: ptr(publisher.PublisherCredentialRef(uuid.New())),
			Config: json.RawMessage(`{
				"repo":"customer/site",
				"base_url":"https://customer.example/blog"
			}`),
		},
	}
	fallback := publisher.NewBlog("fallback-token", "fallback/repo", "fallback-branch", "https://fallback.example/blog", "fallback/content", slog.Default())
	s := &Scheduler{Blog: fallback, Log: slog.Default()}

	_, err := s.blogPublisherForProject(context.Background(), store, db.Project{ID: projectID})
	if err == nil || !strings.Contains(err.Error(), "enabled publisher connection") {
		t.Fatalf("expected disabled connection to be ineligible, got %v", err)
	}
}

func TestBlogPublisherFromConnectionRequiresCredential(t *testing.T) {
	conn := db.PublisherConnection{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
		Kind:      publisher.ConnectionKindGitHubNextJS,
		Status:    "connected",
		Enabled:   true,
		Config: json.RawMessage(`{
			"repo":"customer/site",
			"base_url":"https://customer.example/blog"
		}`),
	}
	fallback := publisher.NewBlog("fallback-token", "fallback/repo", "fallback-branch", "https://fallback.example/blog", "fallback/content", slog.Default())

	blog, fromConnection, err := blogPublisherFromConnection(fallback, "", conn, slog.Default(), nil)
	if err == nil || !strings.Contains(err.Error(), "publisher credential") {
		t.Fatalf("expected missing credential error, blog=%+v fromConnection=%v err=%v", blog, fromConnection, err)
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
			Enabled:       true,
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
	conn               db.PublisherConnection
	cred               db.PublisherCredential
	noConnection       bool
	disabledConnection bool
}

func (f *publisherConnectionStoreFake) GetDefaultPublisherConnectionForProject(context.Context, db.GetDefaultPublisherConnectionForProjectParams) (db.PublisherConnection, error) {
	if f.noConnection {
		return db.PublisherConnection{}, pgx.ErrNoRows
	}
	return f.conn, nil
}

func (f *publisherConnectionStoreFake) GetEnabledPublisherConnectionForProject(context.Context, db.GetEnabledPublisherConnectionForProjectParams) (db.PublisherConnection, error) {
	if f.noConnection {
		return db.PublisherConnection{}, pgx.ErrNoRows
	}
	if f.disabledConnection || !f.conn.Enabled || f.conn.Status != "connected" {
		return db.PublisherConnection{}, pgx.ErrNoRows
	}
	return f.conn, nil
}

func (f *publisherConnectionStoreFake) GetActivePublisherCredential(context.Context, db.GetActivePublisherCredentialParams) (db.PublisherCredential, error) {
	return f.cred, nil
}

func blogConfiguredWithToken(blog *publisher.BlogPublisher) bool {
	return blog.Token != ""
}

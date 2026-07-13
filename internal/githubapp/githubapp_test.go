package githubapp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"testing"
)

func testKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(block))
}

func TestConfiguredRequiresAppKeyAndSlug(t *testing.T) {
	if (Config{}).Configured() {
		t.Fatal("empty config must not be configured")
	}
	full := Config{AppID: "123", Slug: "citeloop", PrivateKeyPEM: testKeyPEM(t)}
	if !full.Configured() {
		t.Fatal("complete config must be configured")
	}
	if (Config{AppID: "123", Slug: "citeloop"}).Configured() {
		t.Fatal("missing private key must not be configured")
	}
}

func TestInstallURL(t *testing.T) {
	s := New(Config{Slug: "citeloop-publisher"})
	got := s.InstallURL("proj-1")
	if got != "https://github.com/apps/citeloop-publisher/installations/new?state=proj-1" {
		t.Fatalf("install url = %q", got)
	}
	if New(Config{}).InstallURL("x") != "" {
		t.Fatal("no slug should yield empty install url")
	}

	// A slug mistakenly set to the full App page URL must still produce a valid,
	// non-doubled install URL (regression: GITHUB_APP_SLUG=https://github.com/apps/citeloop).
	for _, slug := range []string{
		"https://github.com/apps/citeloop",
		"github.com/apps/citeloop/",
		"  citeloop  ",
	} {
		got := New(Config{Slug: slug}).InstallURL("proj-1")
		if got != "https://github.com/apps/citeloop/installations/new?state=proj-1" {
			t.Fatalf("slug %q: install url = %q", slug, got)
		}
	}
}

func TestAppJWTSignsValidRS256(t *testing.T) {
	s := New(Config{AppID: "424242", Slug: "citeloop", PrivateKeyPEM: testKeyPEM(t)})
	tok, err := s.appJWT()
	if err != nil {
		t.Fatalf("appJWT: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt must have 3 segments, got %d", len(parts))
	}
	header, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var h map[string]string
	if err := json.Unmarshal(header, &h); err != nil || h["alg"] != "RS256" {
		t.Fatalf("header alg = %v (err %v)", h, err)
	}
	claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c map[string]any
	if err := json.Unmarshal(claims, &c); err != nil {
		t.Fatalf("claims: %v", err)
	}
	if c["iss"] != "424242" {
		t.Fatalf("iss = %v, want app id", c["iss"])
	}
	if _, ok := c["exp"]; !ok {
		t.Fatal("missing exp")
	}
}

func TestGitHubAppReadinessInstallationAccessDecodesTokenAndGrantedPermissions(t *testing.T) {
	s := New(Config{AppID: "424242", Slug: "citeloop", PrivateKeyPEM: testKeyPEM(t)})
	s.client = &http.Client{Transport: githubAppRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %q, want POST", req.Method)
		}
		if req.URL.Path != "/app/installations/12345/access_tokens" {
			t.Fatalf("path = %q", req.URL.Path)
		}
		if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
			t.Fatal("installation access request must use an App JWT")
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Status:     "201 Created",
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{
				"token":"installation-secret",
				"permissions":{"contents":"write","pull_requests":"write","metadata":"read"}
			}`)),
			Request: req,
		}, nil
	})}

	access, err := s.InstallationAccess(context.Background(), "12345")
	if err != nil {
		t.Fatalf("InstallationAccess returned error: %v", err)
	}
	if access.Token != "installation-secret" {
		t.Fatalf("token = %q", access.Token)
	}
	if access.Permissions["contents"] != "write" || access.Permissions["pull_requests"] != "write" || access.Permissions["metadata"] != "read" {
		t.Fatalf("permissions = %#v", access.Permissions)
	}
	body, err := json.Marshal(access)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "installation-secret") || !strings.Contains(string(body), `"permissions"`) {
		t.Fatalf("serialized installation access must expose permissions but not token: %s", body)
	}
}

func TestGitHubAppReadinessInstallationAccessErrorKeepsOnlyStatusMetadata(t *testing.T) {
	s := New(Config{AppID: "424242", Slug: "citeloop", PrivateKeyPEM: testKeyPEM(t)})
	s.client = &http.Client{Transport: githubAppRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Status:     "403 Forbidden",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"message":"ghp_secret Authorization: Bearer raw upstream body"}`)),
			Request:    req,
		}, nil
	})}

	_, err := s.InstallationAccess(context.Background(), "12345")
	if err == nil {
		t.Fatal("expected installation access error")
	}
	statusError, ok := err.(interface{ StatusCode() int })
	if !ok || statusError.StatusCode() != http.StatusForbidden {
		t.Fatalf("error = %#v, want status-bearing 403", err)
	}
	for _, unsafe := range []string{"ghp_secret", "Authorization", "Bearer", "raw upstream body"} {
		if strings.Contains(err.Error(), unsafe) {
			t.Fatalf("error leaked %q: %v", unsafe, err)
		}
	}
}

type githubAppRoundTripFunc func(*http.Request) (*http.Response, error)

func (f githubAppRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

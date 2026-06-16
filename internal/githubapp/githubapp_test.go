package githubapp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
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

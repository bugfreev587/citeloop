// Package githubapp implements the minimal GitHub App flow CiteLoop needs to
// publish to a customer's blog repo without a pasted PAT: mint an App JWT,
// exchange it for a short-lived installation access token, and list the repos an
// installation can reach. It signs RS256 with crypto/rsa to avoid a JWT dep.
package githubapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const apiBase = "https://api.github.com"

// Config holds the GitHub App credentials, sourced from the server environment.
type Config struct {
	AppID         string
	Slug          string
	ClientID      string
	ClientSecret  string
	PrivateKeyPEM string
}

// Configured reports whether the App is set up enough to mint tokens.
func (c Config) Configured() bool {
	return strings.TrimSpace(c.AppID) != "" && strings.TrimSpace(c.PrivateKeyPEM) != "" && strings.TrimSpace(c.Slug) != ""
}

type Service struct {
	cfg    Config
	client *http.Client
	now    func() time.Time
}

func New(cfg Config) *Service {
	return &Service{cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}, now: time.Now}
}

func (s *Service) Configured() bool { return s.cfg.Configured() }

// normalizeSlug tolerates GITHUB_APP_SLUG being set to the full App page URL
// (e.g. "https://github.com/apps/citeloop") instead of just the slug, which
// would otherwise produce a doubled, 404-ing install URL.
func normalizeSlug(raw string) string {
	slug := strings.TrimSpace(raw)
	for _, prefix := range []string{"https://github.com/apps/", "http://github.com/apps/", "github.com/apps/"} {
		if idx := strings.LastIndex(slug, prefix); idx >= 0 {
			slug = slug[idx+len(prefix):]
			break
		}
	}
	return strings.Trim(strings.TrimSpace(slug), "/")
}

// InstallURL is where the operator authorizes the App on their repo(s); GitHub
// redirects back to the App's configured callback with installation_id + state.
func (s *Service) InstallURL(state string) string {
	slug := normalizeSlug(s.cfg.Slug)
	if slug == "" {
		return ""
	}
	return fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s", slug, url.QueryEscape(state))
}

// Repo is the subset of a GitHub repository the picker needs.
type Repo struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

func (s *Service) appJWT() (string, error) {
	key, err := parseRSAPrivateKey(s.cfg.PrivateKeyPEM)
	if err != nil {
		return "", err
	}
	now := s.now().Add(-30 * time.Second) // tolerate small clock skew
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iat": now.Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": strings.TrimSpace(s.cfg.AppID),
	}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)
	signingInput := b64(headerJSON) + "." + b64(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign app jwt: %w", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// InstallationAccess is the short-lived credential and the permissions GitHub
// granted to it. Token is deliberately excluded from JSON so callers cannot
// accidentally expose it with the permission metadata.
type InstallationAccess struct {
	Token       string            `json:"-"`
	Permissions map[string]string `json:"permissions"`
}

type responseStatusError struct {
	operation string
	status    int
}

func (e responseStatusError) Error() string {
	return fmt.Sprintf("%s: GitHub returned status %d", e.operation, e.status)
}

func (e responseStatusError) StatusCode() int { return e.status }

// InstallationAccess exchanges the App JWT for a ~1h installation access token
// and the permission grants attached to that exact token.
func (s *Service) InstallationAccess(ctx context.Context, installationID string) (InstallationAccess, error) {
	jwt, err := s.appJWT()
	if err != nil {
		return InstallationAccess{}, err
	}
	endpoint := fmt.Sprintf("%s/app/installations/%s/access_tokens", apiBase, url.PathEscape(strings.TrimSpace(installationID)))
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return InstallationAccess{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return InstallationAccess{}, responseStatusError{operation: "github installation token", status: resp.StatusCode}
	}
	var payload struct {
		Token       string            `json:"token"`
		Permissions map[string]string `json:"permissions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return InstallationAccess{}, err
	}
	if payload.Token == "" {
		return InstallationAccess{}, errors.New("github returned an empty installation token")
	}
	return InstallationAccess{Token: payload.Token, Permissions: payload.Permissions}, nil
}

// InstallationToken preserves the original token-only API for publisher and
// scheduler callers that do not need permission metadata.
func (s *Service) InstallationToken(ctx context.Context, installationID string) (string, error) {
	access, err := s.InstallationAccess(ctx, installationID)
	if err != nil {
		return "", err
	}
	return access.Token, nil
}

// ListRepos returns the repositories an installation can access.
func (s *Service) ListRepos(ctx context.Context, installationID string) ([]Repo, error) {
	token, err := s.InstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	endpoint := apiBase + "/installation/repositories?per_page=100"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("github list repositories: %s", resp.Status)
	}
	var out struct {
		Repositories []Repo `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Repositories, nil
}

func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemStr)))
	if block == nil {
		return nil, errors.New("invalid GitHub App private key PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub App private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("GitHub App private key is not RSA")
	}
	return key, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

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

// InstallURL is where the operator authorizes the App on their repo(s); GitHub
// redirects back to the App's configured callback with installation_id + state.
func (s *Service) InstallURL(state string) string {
	if strings.TrimSpace(s.cfg.Slug) == "" {
		return ""
	}
	return fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s", s.cfg.Slug, url.QueryEscape(state))
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

// InstallationToken exchanges the App JWT for a ~1h installation access token.
func (s *Service) InstallationToken(ctx context.Context, installationID string) (string, error) {
	jwt, err := s.appJWT()
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("%s/app/installations/%s/access_tokens", apiBase, url.PathEscape(strings.TrimSpace(installationID)))
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("github installation token: %s", resp.Status)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", errors.New("github returned an empty installation token")
	}
	return out.Token, nil
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

package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type gscOAuthStateClaims struct {
	ProjectID   uuid.UUID `json:"project_id"`
	OwnerID     string    `json:"owner_id"`
	RedirectURI string    `json:"redirect_uri"`
	Nonce       string    `json:"nonce"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func signGSCOAuthState(secret string, claims gscOAuthStateClaims) (string, error) {
	if claims.ProjectID == uuid.Nil || strings.TrimSpace(claims.OwnerID) == "" || strings.TrimSpace(claims.RedirectURI) == "" || claims.ExpiresAt.IsZero() {
		return "", errors.New("oauth state claims are incomplete")
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	sig := signStatePayload(secret, encodedPayload)
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func parseGSCOAuthState(secret, state string, projectID uuid.UUID, ownerID string, now time.Time) (gscOAuthStateClaims, error) {
	var claims gscOAuthStateClaims
	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return claims, errors.New("oauth state is malformed")
	}
	gotSig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, errors.New("oauth state signature is malformed")
	}
	wantSig := signStatePayload(secret, parts[0])
	if !hmac.Equal(gotSig, wantSig) {
		return claims, errors.New("oauth state signature mismatch")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, errors.New("oauth state payload is malformed")
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, err
	}
	if claims.ProjectID != projectID {
		return claims, errors.New("oauth state project mismatch")
	}
	if claims.OwnerID != strings.TrimSpace(ownerID) {
		return claims, errors.New("oauth state owner mismatch")
	}
	if !claims.ExpiresAt.After(now) {
		return claims, errors.New("oauth state expired")
	}
	return claims, nil
}

func signStatePayload(secret, encodedPayload string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encodedPayload))
	return mac.Sum(nil)
}

package articleassets

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
)

type DatabaseStore struct {
	Q             *db.Queries
	PublicBaseURL string
}

func (s DatabaseStore) Put(ctx context.Context, key string, data []byte, mimeType string) (string, error) {
	if s.Q == nil || strings.TrimSpace(s.PublicBaseURL) == "" {
		return "", errors.New("stable article asset database store is not configured")
	}
	if len(data) == 0 || strings.TrimSpace(mimeType) == "" {
		return "", errors.New("article asset data and mime type are required")
	}
	if err := s.Q.PutArticleAssetObject(ctx, db.PutArticleAssetObjectParams{StorageKey: key, Data: data, MimeType: mimeType}); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString([]byte(key))
	return strings.TrimRight(s.PublicBaseURL, "/") + "/assets/" + token, nil
}

func DecodeStorageToken(token string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil || !strings.HasPrefix(string(decoded), "article-assets/") {
		return "", errors.New("invalid article asset token")
	}
	return string(decoded), nil
}

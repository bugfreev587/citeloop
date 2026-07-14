package db

import (
	"os"
	"strings"
	"testing"
)

func TestArticleAssetMigrationKeepsImagesOptionalStableAndCredentialsEncrypted(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0087_article_assets.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{"article_assets", "planned','generating','ready','failed", "unique(article_id, role, brief_hash, revision)", "article_asset_objects", "encrypted_api_key", "admin_image_credentials"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	if strings.Contains(sql, " api_key text") {
		t.Fatal("image API key must never be stored in plaintext")
	}
}

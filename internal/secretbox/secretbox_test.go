package secretbox

import (
	"strings"
	"testing"
)

func TestEncryptStringHidesAndRestoresPlaintext(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	plain := "ghp_test_secret_value"

	encrypted, err := EncryptString(plain, secret)
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "" {
		t.Fatal("encrypted value is empty")
	}
	if strings.Contains(encrypted, plain) || strings.Contains(encrypted, "ghp_test") {
		t.Fatalf("encrypted value leaked plaintext: %s", encrypted)
	}

	decrypted, err := DecryptString(encrypted, secret)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != plain {
		t.Fatalf("decrypted = %q", decrypted)
	}
}

func TestDecryptStringRejectsMalformedCiphertext(t *testing.T) {
	_, err := DecryptString("not-base64", "0123456789abcdef0123456789abcdef")
	if err == nil {
		t.Fatal("expected malformed ciphertext to fail")
	}
}

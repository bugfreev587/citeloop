package secretbox

import (
	"strings"
	"testing"
)

func TestEncryptDecryptStringRoundTrip(t *testing.T) {
	ciphertext, err := EncryptString("secret-token", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "secret-token" || ciphertext == "" {
		t.Fatalf("ciphertext was not encrypted: %q", ciphertext)
	}
	plaintext, err := DecryptString(ciphertext, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "secret-token" {
		t.Fatalf("plaintext = %q", plaintext)
	}
}

func TestDecryptStringRejectsWrongSecret(t *testing.T) {
	ciphertext, err := EncryptString("secret-token", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptString(ciphertext, "wrong-secret"); err == nil {
		t.Fatal("expected wrong secret to fail")
	}
}

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

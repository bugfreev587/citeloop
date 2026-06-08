package secretbox

import "testing"

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

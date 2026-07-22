package secretbox_test

import (
	"bytes"
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/secretbox"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Parallel()

	plain := []byte("-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----")
	sealed, err := secretbox.Encrypt("session-secret", plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(sealed, plain) {
		t.Fatal("ciphertext must differ from plaintext")
	}
	got, err := secretbox.Decrypt("session-secret", sealed)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q, want %q", got, plain)
	}
	if _, err := secretbox.Decrypt("other-secret", sealed); err == nil {
		t.Fatal("expected decrypt failure with wrong key")
	}
}

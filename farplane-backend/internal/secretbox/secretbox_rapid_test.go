package secretbox_test

import (
	"bytes"
	"testing"

	"pgregory.net/rapid"

	"github.com/farplane/farplane/farplane-backend/internal/secretbox"
)

func TestEncryptDecryptRapid(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		key := rapid.StringMatching(`[A-Za-z0-9_-]{8,64}`).Draw(t, "key")
		plain := rapid.SliceOf(rapid.Byte()).Draw(t, "plain")

		sealed, err := secretbox.Encrypt(key, plain)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}

		got, err := secretbox.Decrypt(key, sealed)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}

		if !bytes.Equal(got, plain) {
			t.Fatalf("round-trip mismatch")
		}

		wrongKey := key + "-x"
		if _, err := secretbox.Decrypt(wrongKey, sealed); err == nil {
			t.Fatal("expected decrypt failure with wrong key")
		}
	})
}

func TestDecryptRejectsShortCiphertextRapid(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		key := rapid.StringMatching(`[A-Za-z0-9_-]{8,32}`).Draw(t, "key")

		short := rapid.SliceOfN(rapid.Byte(), 0, 11).Draw(t, "short")
		if _, err := secretbox.Decrypt(key, short); err == nil {
			t.Fatal("expected error for short ciphertext")
		}
	})
}

func TestEncryptRejectsEmptyKey(t *testing.T) {
	t.Parallel()

	if _, err := secretbox.Encrypt("", []byte("x")); err == nil {
		t.Fatal("expected empty key error")
	}

	if _, err := secretbox.Decrypt("", []byte("0123456789ab")); err == nil {
		t.Fatal("expected empty key error")
	}
}

func FuzzEncryptDecrypt(f *testing.F) {
	f.Add("session-secret", []byte("hello"))
	f.Add("k", []byte{})
	f.Add("another-key", []byte{0, 1, 2, 255})
	f.Fuzz(func(t *testing.T, key string, plain []byte) {
		if key == "" {
			t.Skip("empty key is rejected by design")
		}

		sealed, err := secretbox.Encrypt(key, plain)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}

		got, err := secretbox.Decrypt(key, sealed)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}

		if !bytes.Equal(got, plain) {
			t.Fatalf("round-trip mismatch")
		}
	})
}

package portability

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncryptDecryptCredentialsPayload(t *testing.T) {
	type credential struct {
		Pattern string `json:"pattern"`
		Token   string `json:"token"`
	}

	input := []credential{
		{Pattern: "api.openai.com", Token: "secret-token"},
	}

	blob, err := EncryptCredentialsPayload("senha-segura", input)
	if err != nil {
		t.Fatalf("EncryptCredentialsPayload() error = %v", err)
	}
	if blob.Mode != credentialCipherMode {
		t.Fatalf("Mode = %q, want %q", blob.Mode, credentialCipherMode)
	}
	if blob.Algorithm != credentialCipherAlgorithm {
		t.Fatalf("Algorithm = %q, want %q", blob.Algorithm, credentialCipherAlgorithm)
	}

	var output []credential
	if err := DecryptCredentialsPayload("senha-segura", blob, &output); err != nil {
		t.Fatalf("DecryptCredentialsPayload() error = %v", err)
	}
	if len(output) != 1 || output[0].Token != "secret-token" {
		t.Fatalf("output = %#v, want decrypted payload", output)
	}
}

func TestDecryptCredentialsPayloadWrongPassword(t *testing.T) {
	blob, err := EncryptCredentialsPayload("senha-correta", map[string]string{"token": "abc"})
	if err != nil {
		t.Fatalf("EncryptCredentialsPayload() error = %v", err)
	}

	var output map[string]string
	if err := DecryptCredentialsPayload("senha-incorreta", blob, &output); err == nil {
		t.Fatal("DecryptCredentialsPayload() expected error for wrong password")
	}
}

func TestEncryptCredentialsPayloadRequiresPassword(t *testing.T) {
	if _, err := EncryptCredentialsPayload("   ", map[string]string{"token": "abc"}); err == nil {
		t.Fatal("EncryptCredentialsPayload() expected error for empty password")
	}
}

func TestDecryptCredentialsPayloadRejectsInvalidSaltShape(t *testing.T) {
	blob, err := EncryptCredentialsPayload("senha-correta", map[string]string{"token": "abc"})
	if err != nil {
		t.Fatalf("EncryptCredentialsPayload() error = %v", err)
	}

	blob.Salt = base64.StdEncoding.EncodeToString([]byte("short"))

	var output map[string]string
	if err := DecryptCredentialsPayload("senha-correta", blob, &output); err == nil {
		t.Fatal("DecryptCredentialsPayload() expected error for invalid salt")
	}
}

func TestDecryptCredentialsPayloadRejectsOversizedCiphertext(t *testing.T) {
	blob, err := EncryptCredentialsPayload("senha-correta", map[string]string{"token": "abc"})
	if err != nil {
		t.Fatalf("EncryptCredentialsPayload() error = %v", err)
	}

	blob.Ciphertext = strings.Repeat("A", base64.StdEncoding.EncodedLen(maxCredentialCiphertextBytes+1))

	var output map[string]string
	if err := DecryptCredentialsPayload("senha-correta", blob, &output); err == nil {
		t.Fatal("DecryptCredentialsPayload() expected error for oversized ciphertext")
	}
}

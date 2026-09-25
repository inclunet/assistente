package commandledger

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"assistente/internal/credentials"
)

func TestCredentialKeyProviderSignsUsingEncryptedManager(t *testing.T) {
	manager := credentials.NewManager(bytes.Repeat([]byte{7}, 32)) // Memória, sem persistência/keychain.
	key := bytes.Repeat([]byte{9}, 32)
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(key)); err != nil {
		t.Fatal(err)
	}
	provider, err := NewCredentialKeyProvider(manager)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	loaded, err := provider(ctx, "command-request-hmac:v1")
	if err != nil || !bytes.Equal(loaded, key) {
		t.Fatal("chave não carregada")
	}
	clear(loaded)
	again, err := provider(ctx, "command-request-hmac:v1")
	if err != nil || !bytes.Equal(again, key) {
		t.Fatal("provider expôs armazenamento mutável")
	}
	req := validRequest()
	req.ArgumentsFingerprint, req.RequestFingerprint, req.RequestFingerprintVersion = "", "", ""
	signed, err := SignLocalRead(ctx, req, "v1", provider)
	if err != nil {
		t.Fatal(err)
	}
	now := req.ReceivedAt
	store, _ := testStore(t, &now)
	if result, err := store.Reserve(ctx, signed); err != nil || !result.Created {
		t.Fatalf("reserva assinada: %v", err)
	}
	if _, err := provider(ctx, "command-request-hmac:v2"); !errors.Is(err, ErrFingerprintKeyUnavailable) {
		t.Fatal("fallback de versão indevido")
	}
}

func TestCredentialKeyProviderNeverUsesUserScopedSecret(t *testing.T) {
	manager := credentials.NewManager(bytes.Repeat([]byte{7}, 32))
	req := validRequest()
	if err := manager.RegisterStoredCredentialWithContext(context.Background(), credentials.StoredCredential{UserID: req.Owner.UserID, Pattern: "internal-auth:command-request-hmac:v1", Auth: &credentials.AuthConfig{Source: "static", Type: "secret", Token: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))}}); err != nil {
		t.Fatal(err)
	}
	provider, err := NewCredentialKeyProvider(manager)
	if err != nil {
		t.Fatal(err)
	}
	key, err := provider(context.Background(), "command-request-hmac:v1")
	if key != nil || !errors.Is(err, ErrFingerprintKeyUnavailable) {
		t.Fatal("credencial de usuário aceita como chave da instância")
	}
}

func TestCredentialKeyProviderRejectsInvalidSecrets(t *testing.T) {
	for _, tc := range []struct{ name, kind, token string }{
		{"missing", "secret", ""}, {"invalid", "secret", "not base64!"},
		{"short", "secret", base64.RawURLEncoding.EncodeToString([]byte("short"))},
		{"wrong_type", "bearer", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))},
		{"padded", "secret", base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := credentials.NewManager(bytes.Repeat([]byte{7}, 32))
			if err := manager.RegisterPattern("internal-auth:command-request-hmac:v1", &credentials.AuthConfig{Source: "static", Type: tc.kind, Token: tc.token}); err != nil {
				t.Fatal(err)
			}
			provider, err := NewCredentialKeyProvider(manager)
			if err != nil {
				t.Fatal(err)
			}
			if key, err := provider(context.Background(), "command-request-hmac:v1"); key != nil || !errors.Is(err, ErrFingerprintKeyUnavailable) {
				t.Fatal("segredo inválido aceito")
			}
		})
	}
}

func TestCredentialKeyProviderRejectsContextAndNames(t *testing.T) {
	if provider, err := NewCredentialKeyProvider(nil); provider != nil || !errors.Is(err, ErrFingerprintKeyUnavailable) {
		t.Fatal("manager nil aceito")
	}
	manager := credentials.NewManager(bytes.Repeat([]byte{7}, 32))
	provider, err := NewCredentialKeyProvider(manager)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"internal-auth:jwt-signing-key", "command-request-hmac:v0", "command-request-hmac:v01", "command-request-hmac:v1 ", "command-request-hmac:*"} {
		if key, err := provider(context.Background(), name); key != nil || !errors.Is(err, ErrInvalidRequest) {
			t.Fatal("nome inválido aceito")
		}
	}
	if _, err := provider(nil, "command-request-hmac:v1"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if key, err := provider(ctx, "command-request-hmac:v1"); key != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelamento ignorado")
	}
}

package credentials

import (
	"context"
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestWrapUnwrapDEK(t *testing.T) {
	dek := []byte("01234567890123456789012345678901")
	wrap, err := WrapDEK("senha-forte", dek, KeyWrapKindMaster)
	if err != nil {
		t.Fatalf("WrapDEK failed: %v", err)
	}

	unwrapped, err := UnwrapDEK("senha-forte", wrap)
	if err != nil {
		t.Fatalf("UnwrapDEK failed: %v", err)
	}

	if string(unwrapped) != string(dek) {
		t.Fatalf("DEK mismatch after unwrap")
	}
}

func TestUnwrapDEKWrongSecret(t *testing.T) {
	dek := []byte("01234567890123456789012345678901")
	wrap, err := WrapDEK("senha-correta", dek, KeyWrapKindMaster)
	if err != nil {
		t.Fatalf("WrapDEK failed: %v", err)
	}

	if _, err := UnwrapDEK("senha-incorreta", wrap); err == nil {
		t.Fatalf("expected error for wrong secret")
	}
}

func TestGenerateRecoveryKeyFormat(t *testing.T) {
	key, err := GenerateRecoveryKey()
	if err != nil {
		t.Fatalf("GenerateRecoveryKey failed: %v", err)
	}
	if len(key) < 20 {
		t.Fatalf("recovery key muito curta: %q", key)
	}
	for _, c := range key {
		if (c < 'A' || c > 'Z') && (c < '2' || c > '7') && c != '-' {
			t.Fatalf("recovery key contém caractere inválido %q em %q (esperado base32 + hífens)", string(c), key)
		}
	}
	if !containsDash(key) {
		t.Fatalf("recovery key deve conter separadores: %q", key)
	}
}

func TestSetupMasterKeyAdoptingKeychainRejectsExistingCredentialsWithoutKeyringDEK(t *testing.T) {
	store := &setupMasterKeyTestStore{
		credentials: []StoredCredential{{Pattern: "api.example.com", Auth: &AuthConfig{Type: "bearer", Token: "ciphertext"}}},
	}

	_, err := setupMasterKeyAdoptingKeychain(store, "senha-forte", func() ([]byte, error) {
		return nil, keyring.ErrNotFound
	}, func([]byte) error {
		t.Fatal("setup must not save a new DEK when credentials already exist")
		return nil
	})
	if !errors.Is(err, ErrExistingCredentialsWithoutDEK) {
		t.Fatalf("expected ErrExistingCredentialsWithoutDEK, got %v", err)
	}
	if len(store.wraps) != 0 {
		t.Fatalf("setup must not create wraps when existing credentials have no DEK, got %+v", store.wraps)
	}
}

func TestSetupMasterKeyAdoptingKeychainCreatesNewDEKWhenNoCredentialsAndNoKeyringDEK(t *testing.T) {
	store := &setupMasterKeyTestStore{}

	result, err := setupMasterKeyAdoptingKeychain(store, "senha-forte", func() ([]byte, error) {
		return nil, keyring.ErrNotFound
	}, func([]byte) error {
		return nil
	})
	if err != nil {
		t.Fatalf("setup should create new DEK for empty store: %v", err)
	}
	if len(result.DEK) != 32 {
		t.Fatalf("unexpected DEK length: %d", len(result.DEK))
	}
	if len(store.wraps) != 2 {
		t.Fatalf("expected master and recovery wraps, got %+v", store.wraps)
	}
}

type setupMasterKeyTestStore struct {
	credentials []StoredCredential
	wraps       []KeyWrap
}

func (s *setupMasterKeyTestStore) SaveCredential(context.Context, StoredCredential) error {
	return nil
}

func (s *setupMasterKeyTestStore) ListCredentials(context.Context) ([]StoredCredential, error) {
	return s.credentials, nil
}

func (s *setupMasterKeyTestStore) DeleteCredential(context.Context, string) error {
	return nil
}

func (s *setupMasterKeyTestStore) SaveKeyWrap(_ context.Context, wrap KeyWrap) error {
	s.wraps = append(s.wraps, wrap)
	return nil
}

func (s *setupMasterKeyTestStore) GetKeyWrap(context.Context, string) (*KeyWrap, error) {
	return nil, nil
}

func (s *setupMasterKeyTestStore) HasKeyWrap(context.Context, string) (bool, error) {
	return false, nil
}

func containsDash(value string) bool {
	for _, r := range value {
		if r == '-' {
			return true
		}
	}
	return false
}

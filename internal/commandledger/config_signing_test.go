package commandledger

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func configurationMutationFixture() ConfigurationMutationRequest {
	return ConfigurationMutationRequest{
		MutationID:         "01890f00-0000-7000-8000-000000000001",
		UserID:             "01890f00-0000-7000-8000-000000000002",
		SessionID:          "01890f00-0000-7000-8000-000000000003",
		AuthGeneration:     "auth:1",
		SecurityGeneration: "security:1",
		GenerationID:       "01890f00-0000-7000-8000-000000000004",
		Generation:         9223372036854775807,
		BeforeDocument:     `{"version":1,"enabled":true}`,
		AfterDocument:      " {\"enabled\":false,\"version\":1} \n",
	}
}

func TestSignConfigurationMutationIsDeterministicAndUsesIndependentHMAC(t *testing.T) {
	req := configurationMutationFixture()
	key := bytes.Repeat([]byte{'k'}, 32)
	var calls int
	provider := func(_ context.Context, name string) ([]byte, error) {
		calls++
		if name != "command-request-hmac:v1" {
			t.Fatalf("nome da chave = %q", name)
		}
		return key, nil
	}

	payload, err := canonicalConfigurationMutation(req)
	if err != nil {
		t.Fatal(err)
	}
	const wantPayload = `{"action":"binding_enabled","after_document":" {\"enabled\":false,\"version\":1} \n","auth_context_type":"local_session","auth_generation":"auth:1","before_document":"{\"version\":1,\"enabled\":true}","domain":"config","generation":"9223372036854775807","generation_id":"01890f00-0000-7000-8000-000000000004","mutation_id":"01890f00-0000-7000-8000-000000000001","scope":"global","security_generation":"security:1","session_id":"01890f00-0000-7000-8000-000000000003","user_id":"01890f00-0000-7000-8000-000000000002"}`
	if string(payload) != wantPayload {
		t.Fatalf("payload canônico = %s\nwant = %s", payload, wantPayload)
	}

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(wantPayload))
	want := "v1:" + hex.EncodeToString(mac.Sum(nil))
	got, err := SignConfigurationMutation(context.Background(), req, "v1", provider)
	if err != nil || got != want {
		t.Fatalf("assinatura = %q, %v; want %q", got, err, want)
	}
	repeated, err := SignConfigurationMutation(context.Background(), req, "v1", provider)
	if err != nil || repeated != got || calls != 2 {
		t.Fatalf("não determinística ou provider inesperado: %q %v calls=%d", repeated, err, calls)
	}
}

func TestSignConfigurationMutationEveryFieldInfluencesSignature(t *testing.T) {
	base := configurationMutationFixture()
	key := bytes.Repeat([]byte{0x42}, 32)
	keys := func(context.Context, string) ([]byte, error) { return key, nil }
	baseline, err := SignConfigurationMutation(context.Background(), base, "v1", keys)
	if err != nil {
		t.Fatal(err)
	}
	newUUID := func() string {
		u, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return u.String()
	}
	cases := map[string]func(*ConfigurationMutationRequest){
		"mutation_id":         func(r *ConfigurationMutationRequest) { r.MutationID = newUUID() },
		"user_id":             func(r *ConfigurationMutationRequest) { r.UserID = newUUID() },
		"session_id":          func(r *ConfigurationMutationRequest) { r.SessionID = newUUID() },
		"auth_generation":     func(r *ConfigurationMutationRequest) { r.AuthGeneration = "auth:2" },
		"security_generation": func(r *ConfigurationMutationRequest) { r.SecurityGeneration = "security:2" },
		"generation_id":       func(r *ConfigurationMutationRequest) { r.GenerationID = newUUID() },
		"generation":          func(r *ConfigurationMutationRequest) { r.Generation = 1 },
		"before_document":     func(r *ConfigurationMutationRequest) { r.BeforeDocument = `{"enabled":false}` },
		"after_document":      func(r *ConfigurationMutationRequest) { r.AfterDocument = `{"enabled":true}` },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			mutated := base
			mutate(&mutated)
			got, err := SignConfigurationMutation(context.Background(), mutated, "v1", keys)
			if err != nil {
				t.Fatal(err)
			}
			if got == baseline {
				t.Fatal("campo semântico não alterou a assinatura")
			}
		})
	}
}

func TestSignConfigurationMutationKeyVersionChangesSignature(t *testing.T) {
	req := configurationMutationFixture()
	keys := func(_ context.Context, name string) ([]byte, error) {
		if name == "command-request-hmac:v1" {
			return bytes.Repeat([]byte{1}, 32), nil
		}
		if name == "command-request-hmac:v2" {
			return bytes.Repeat([]byte{2}, 32), nil
		}
		return nil, errors.New("chave inesperada")
	}
	v1, err := SignConfigurationMutation(context.Background(), req, "v1", keys)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := SignConfigurationMutation(context.Background(), req, "v2", keys)
	if err != nil {
		t.Fatal(err)
	}
	if v1 == v2 || !strings.HasPrefix(v1, "v1:") || !strings.HasPrefix(v2, "v2:") {
		t.Fatalf("versão/chave não vinculada: %q %q", v1, v2)
	}
}

func TestSignConfigurationMutationRejectsInvalidRequests(t *testing.T) {
	valid := configurationMutationFixture()
	keys := func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{1}, 32), nil }
	cases := map[string]func(*ConfigurationMutationRequest){
		"noncanonical_uuid":       func(r *ConfigurationMutationRequest) { r.UserID = strings.ToUpper(r.UserID) },
		"wrong_uuid_version":      func(r *ConfigurationMutationRequest) { r.SessionID = "01890f00-0000-6000-8000-000000000003" },
		"empty_generation":        func(r *ConfigurationMutationRequest) { r.AuthGeneration = "" },
		"trimmed_generation":      func(r *ConfigurationMutationRequest) { r.SecurityGeneration = " security:1" },
		"invalid_utf8_generation": func(r *ConfigurationMutationRequest) { r.AuthGeneration = string([]byte{0xff}) },
		"long_generation":         func(r *ConfigurationMutationRequest) { r.SecurityGeneration = strings.Repeat("x", 257) },
		"zero_generation":         func(r *ConfigurationMutationRequest) { r.Generation = 0 },
		"negative_generation":     func(r *ConfigurationMutationRequest) { r.Generation = -1 },
		"document_array":          func(r *ConfigurationMutationRequest) { r.BeforeDocument = `[]` },
		"document_invalid_json":   func(r *ConfigurationMutationRequest) { r.AfterDocument = `{"enabled":}` },
		"document_invalid_utf8": func(r *ConfigurationMutationRequest) {
			r.BeforeDocument = string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
		},
		"document_too_large": func(r *ConfigurationMutationRequest) {
			r.AfterDocument = `{"x":"` + strings.Repeat("a", maxConfigurationDocumentBytes) + `"}`
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			req := valid
			mutate(&req)
			called := false
			provider := func(context.Context, string) ([]byte, error) {
				called = true
				return keys(context.Background(), "")
			}
			got, err := SignConfigurationMutation(context.Background(), req, "v1", provider)
			if !errors.Is(err, ErrInvalidRequest) || got != "" || called {
				t.Fatalf("resultado = %q, %v, provider=%v", got, err, called)
			}
		})
	}
}

func TestSignConfigurationMutationSanitizesProviderAndPreservesContext(t *testing.T) {
	req := configurationMutationFixture()
	key := bytes.Repeat([]byte{'s'}, 32)
	for name, provider := range map[string]FingerprintKeyProvider{
		"provider_error": func(context.Context, string) ([]byte, error) {
			return nil, errors.New("secret=top-secret path=C:\\vault")
		},
		"missing_key": func(context.Context, string) ([]byte, error) { return nil, nil },
		"short_key":   func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{1}, 31), nil },
	} {
		t.Run(name, func(t *testing.T) {
			got, err := SignConfigurationMutation(context.Background(), req, "v1", provider)
			if !errors.Is(err, ErrFingerprintKeyUnavailable) || got != "" {
				t.Fatalf("resultado = %q, %v", got, err)
			}
		})
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	got, err := SignConfigurationMutation(cancelled, req, "v1", func(context.Context, string) ([]byte, error) {
		called = true
		return key, nil
	})
	if !errors.Is(err, context.Canceled) || got != "" || called {
		t.Fatalf("cancelamento antes do provider = %q, %v, called=%v", got, err, called)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got, err = SignConfigurationMutation(ctx, req, "v1", func(context.Context, string) ([]byte, error) {
		cancel()
		return key, nil
	})
	if !errors.Is(err, context.Canceled) || got != "" {
		t.Fatalf("cancelamento depois do provider = %q, %v", got, err)
	}

	originalKey := append([]byte(nil), key...)
	if _, err := SignConfigurationMutation(context.Background(), req, "v1", func(context.Context, string) ([]byte, error) { return key, nil }); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key, originalKey) {
		t.Fatal("provider teve sua chave alterada")
	}
}

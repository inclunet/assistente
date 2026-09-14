package commandledger

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
)

var fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func testFingerprintKey() []byte {
	return bytes.Repeat([]byte{0x42}, 32)
}

func unsignedRequest() LocalReadRequest {
	req := validRequest()
	req.ArgumentsFingerprint = ""
	req.RequestFingerprint = ""
	req.RequestFingerprintVersion = ""
	return req
}

func signTestRequest(t *testing.T, req LocalReadRequest, version string, keys FingerprintKeyProvider) (LocalReadRequest, error) {
	t.Helper()
	return SignLocalRead(context.Background(), req, version, keys)
}

func TestSignLocalReadIsDeterministicAndFillsFingerprints(t *testing.T) {
	req := unsignedRequest()
	original := req
	key := testFingerprintKey()
	providerCalls := 0
	provider := func(_ context.Context, name string) ([]byte, error) {
		providerCalls++
		if name != "command-request-hmac:v1" {
			t.Fatalf("nome da chave = %q", name)
		}
		return key, nil
	}

	got, err := signTestRequest(t, req, "v1", provider)
	if err != nil {
		t.Fatal(err)
	}
	if req != original {
		t.Fatal("a entrada foi alterada")
	}
	if got.ArgumentsFingerprint == "" || got.RequestFingerprint == "" || got.RequestFingerprintVersion != "v1" {
		t.Fatalf("fingerprints incompletos: %+v", got)
	}
	if !fingerprintPattern.MatchString(got.ArgumentsFingerprint) || !fingerprintPattern.MatchString(got.RequestFingerprint) {
		t.Fatalf("fingerprint não é HMAC SHA-256 hexadecimal minúsculo: %+v", got)
	}

	repeated, err := signTestRequest(t, req, "v1", provider)
	if err != nil {
		t.Fatal(err)
	}
	if got != repeated {
		t.Fatalf("assinatura não determinística:\n%+v\n%+v", got, repeated)
	}
	if providerCalls != 2 {
		t.Fatalf("provider chamado %d vezes, want 2", providerCalls)
	}
}

func TestSignLocalReadEverySemanticFieldChangesRequestFingerprint(t *testing.T) {
	base := unsignedRequest()
	key := testFingerprintKey()
	keys := func(context.Context, string) ([]byte, error) { return key, nil }
	baseline, err := signTestRequest(t, base, "v1", keys)
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
	cases := map[string]func(*LocalReadRequest){
		"invocation_id":            func(r *LocalReadRequest) { r.InvocationID = newUUID() },
		"owner_user":               func(r *LocalReadRequest) { r.Owner.UserID = newUUID() },
		"owner_auth_context":       func(r *LocalReadRequest) { r.Owner.AuthContextID = newUUID() },
		"security_generation":      func(r *LocalReadRequest) { r.SecurityGeneration = "s2" },
		"registry_version":         func(r *LocalReadRequest) { r.RegistryVersion = "r2" },
		"global_config_generation": func(r *LocalReadRequest) { r.GlobalConfigGeneration = "g2" },
		"active_layers_generation": func(r *LocalReadRequest) { r.ActiveLayersGeneration = "l2" },
		"command_id":               func(r *LocalReadRequest) { r.CommandID = "workspace.write" },
		"source_type":              func(r *LocalReadRequest) { r.SourceType = "cli" },
		"correlation_id":           func(r *LocalReadRequest) { r.CorrelationID = "corr-2" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			mutated := base
			mutate(&mutated)
			got, err := signTestRequest(t, mutated, "v1", keys)
			if err != nil {
				t.Fatal(err)
			}
			if got.RequestFingerprint == baseline.RequestFingerprint {
				t.Fatal("mutação semântica não alterou request fingerprint")
			}
			if got.ArgumentsFingerprint != baseline.ArgumentsFingerprint {
				t.Fatal("mutação semântica alterou arguments fingerprint")
			}
		})
	}
}

func TestSignLocalReadExcludesAuthGenerationAndTimestamps(t *testing.T) {
	base := unsignedRequest()
	key := testFingerprintKey()
	keys := func(context.Context, string) ([]byte, error) { return key, nil }
	want, err := signTestRequest(t, base, "v1", keys)
	if err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*LocalReadRequest){
		"auth_generation": func(r *LocalReadRequest) { r.AuthGeneration = "rotated" },
		"received_at":     func(r *LocalReadRequest) { r.ReceivedAt = r.ReceivedAt.Add(time.Minute) },
		"expires_at":      func(r *LocalReadRequest) { r.ExpiresAt = r.ExpiresAt.Add(time.Hour) },
	} {
		t.Run(name, func(t *testing.T) {
			mutated := base
			mutate(&mutated)
			got, err := signTestRequest(t, mutated, "v1", keys)
			if err != nil {
				t.Fatal(err)
			}
			if got.RequestFingerprint != want.RequestFingerprint || got.ArgumentsFingerprint != want.ArgumentsFingerprint {
				t.Fatal("campo excluído alterou a assinatura")
			}
		})
	}
}

func TestSignLocalReadKeyRotationAndProviderVersion(t *testing.T) {
	req := unsignedRequest()
	keyV1 := testFingerprintKey()
	keyV2 := bytes.Repeat([]byte{0x24}, 32)
	provider := func(_ context.Context, name string) ([]byte, error) {
		switch name {
		case "command-request-hmac:v1":
			return keyV1, nil
		case "command-request-hmac:v2":
			return keyV2, nil
		default:
			return nil, errors.New("chave inesperada")
		}
	}
	v1, err := signTestRequest(t, req, "v1", provider)
	if err != nil {
		t.Fatal(err)
	}
	v1Again, err := signTestRequest(t, req, "v1", provider)
	if err != nil || v1 != v1Again {
		t.Fatalf("mesma versão/chave não manteve assinatura: %v", err)
	}
	v2, err := signTestRequest(t, req, "v2", provider)
	if err != nil {
		t.Fatal(err)
	}
	if v2.RequestFingerprint == v1.RequestFingerprint || v2.ArgumentsFingerprint == v1.ArgumentsFingerprint {
		t.Fatal("rotação de chave não alterou fingerprints")
	}
}

func TestSignLocalReadRejectsInvalidInputsAndProviderFailures(t *testing.T) {
	key := testFingerprintKey()
	validKeys := func(context.Context, string) ([]byte, error) { return key, nil }
	for name, setup := range map[string]func(LocalReadRequest) (LocalReadRequest, string, FingerprintKeyProvider, context.Context){
		"nil_context": func(r LocalReadRequest) (LocalReadRequest, string, FingerprintKeyProvider, context.Context) {
			return r, "v1", validKeys, nil
		},
		"nil_provider": func(r LocalReadRequest) (LocalReadRequest, string, FingerprintKeyProvider, context.Context) {
			return r, "v1", nil, context.Background()
		},
		"bad_version": func(r LocalReadRequest) (LocalReadRequest, string, FingerprintKeyProvider, context.Context) {
			return r, "version", validKeys, context.Background()
		},
		"client_request_fingerprint": func(r LocalReadRequest) (LocalReadRequest, string, FingerprintKeyProvider, context.Context) {
			r.RequestFingerprint = "client"
			return r, "v1", validKeys, context.Background()
		},
		"client_arguments_fingerprint": func(r LocalReadRequest) (LocalReadRequest, string, FingerprintKeyProvider, context.Context) {
			r.ArgumentsFingerprint = "client"
			return r, "v1", validKeys, context.Background()
		},
		"client_fingerprint_version": func(r LocalReadRequest) (LocalReadRequest, string, FingerprintKeyProvider, context.Context) {
			r.RequestFingerprintVersion = "v1"
			return r, "v1", validKeys, context.Background()
		},
		"invalid_utf8_generation": func(r LocalReadRequest) (LocalReadRequest, string, FingerprintKeyProvider, context.Context) {
			r.SecurityGeneration = string([]byte{0xff})
			return r, "v1", validKeys, context.Background()
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, version, provider, ctx := setup(unsignedRequest())
			got, err := SignLocalRead(ctx, r, version, provider)
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("erro = %v, want ErrInvalidRequest", err)
			}
			if got != (LocalReadRequest{}) {
				t.Fatal("retornou resultado parcial para entrada inválida")
			}
		})
	}

	for name, provider := range map[string]FingerprintKeyProvider{
		"missing":        func(context.Context, string) ([]byte, error) { return nil, nil },
		"provider_error": func(context.Context, string) ([]byte, error) { return nil, errors.New("segredo indisponível") },
		"short_key":      func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{1}, 31), nil },
	} {
		t.Run(name, func(t *testing.T) {
			got, err := signTestRequest(t, unsignedRequest(), "v1", provider)
			if !errors.Is(err, ErrFingerprintKeyUnavailable) {
				t.Fatalf("erro = %v, want ErrFingerprintKeyUnavailable", err)
			}
			if got != (LocalReadRequest{}) {
				t.Fatal("retornou resultado parcial sem chave utilizável")
			}
		})
	}
}

func TestSignLocalReadPreservesCancellationAndDoesNotEraseKey(t *testing.T) {
	req := unsignedRequest()
	key := testFingerprintKey()
	originalKey := append([]byte(nil), key...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := SignLocalRead(ctx, req, "v1", func(context.Context, string) ([]byte, error) {
		t.Fatal("provider não deveria ser chamado com contexto cancelado")
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) || got != (LocalReadRequest{}) {
		t.Fatalf("cancelamento = %+v, %v", got, err)
	}

	_, err = signTestRequest(t, req, "v1", func(context.Context, string) ([]byte, error) { return key, nil })
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key, originalKey) {
		t.Fatal("material da chave foi alterado ou apagado")
	}
}

package commandledger

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestCanonicalStringJCSVectors(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"€$\x0f\nA'B\"\\\\\"/", `"€$\u000f\nA'B\"\\\\\"/"`},
		{"<>&\u2028\u2029😀", "\"<>&\u2028\u2029😀\""},
		{"\x00\b\t\n\f\r\x1f", `"\u0000\b\t\n\f\r\u001f"`},
	} {
		got, err := canonicalString(tc.input)
		if err != nil || got != tc.want {
			t.Fatalf("got %q want %q err %v", got, tc.want, err)
		}
	}
	if _, err := canonicalString(string([]byte{0xed, 0xa0, 0x80})); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal("surrogate inválido aceito")
	}
	a, _ := canonicalString("é")
	b, _ := canonicalString("e\u0301")
	if a == b {
		t.Fatal("Unicode normalizado indevidamente")
	}
}

func TestCanonicalLocalReadGolden(t *testing.T) {
	req := validRequest()
	req.InvocationID = "01890f00-0000-7000-8000-000000000001"
	req.Owner = Owner{UserID: "01890f00-0000-7000-8000-000000000002", AuthContextID: "01890f00-0000-7000-8000-000000000003"}
	req.ArgumentsFingerprint = ""
	req.RequestFingerprint = ""
	req.RequestFingerprintVersion = ""
	const want = `{"active_layers_generation":"l1","actor_id":"01890f00-0000-7000-8000-000000000002","actor_type":"user","arguments":{},"auth_context_id":"01890f00-0000-7000-8000-000000000003","auth_context_type":"local_session","binding_ids":[],"command_id":"workspace.read","context_policy":"none","correlation_id":"corr","decision":"none","effect":"read","global_config_generation":"g1","invocation_id":"01890f00-0000-7000-8000-000000000001","registry_version":"r1","security_generation":"s1","session_id":"01890f00-0000-7000-8000-000000000003","source_type":"palette","user_id":"01890f00-0000-7000-8000-000000000002","version":1}`
	got, err := canonicalLocalRead(req)
	if err != nil || string(got) != want {
		t.Fatalf("canonical diferente: %s %v", got, err)
	}
	key := []byte(strings.Repeat("k", 32))
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(want))
	signed, err := SignLocalRead(context.Background(), req, "v1", func(context.Context, string) ([]byte, error) { return key, nil })
	if err != nil || signed.RequestFingerprint != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("HMAC divergente: %v", err)
	}
}

func TestSignedRequestReplayUsesRetainedKeyVersion(t *testing.T) {
	req := validRequest()
	req.ArgumentsFingerprint = ""
	req.RequestFingerprint = ""
	req.RequestFingerprintVersion = ""
	now := req.ReceivedAt
	s, _ := testStore(t, &now)
	ctx := context.Background()
	keyring := map[string][]byte{"command-request-hmac:v1": []byte(strings.Repeat("a", 32)), "command-request-hmac:v2": []byte(strings.Repeat("b", 32))}
	keys := func(_ context.Context, name string) ([]byte, error) {
		key, ok := keyring[name]
		if !ok {
			return nil, errors.New("missing")
		}
		return key, nil
	}
	first, err := SignLocalRead(ctx, req, "v1", keys)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := s.Reserve(ctx, first); err != nil || !res.Created {
		t.Fatalf("reserva: %+v %v", res, err)
	}
	stored, err := s.Get(ctx, req.Owner, req.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := SignLocalRead(ctx, req, stored.RequestFingerprintVersion, keys)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := s.Reserve(ctx, retry); err != nil || res.Created {
		t.Fatalf("replay v1: %+v %v", res, err)
	}
	wrong, err := SignLocalRead(ctx, req, "v2", keys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reserve(ctx, wrong); !errors.Is(err, ErrConflict) {
		t.Fatalf("versão nova indevida: %v", err)
	}
	delete(keyring, "command-request-hmac:v1")
	if _, err := SignLocalRead(ctx, req, stored.RequestFingerprintVersion, keys); !errors.Is(err, ErrFingerprintKeyUnavailable) {
		t.Fatalf("fallback indevido: %v", err)
	}
}

func TestSigningCancelsAfterKeyLookupWithoutPartialResult(t *testing.T) {
	req := validRequest()
	req.ArgumentsFingerprint, req.RequestFingerprint, req.RequestFingerprintVersion = "", "", ""
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got, err := SignLocalRead(ctx, req, "v1", func(context.Context, string) ([]byte, error) {
		cancel()
		return []byte(strings.Repeat("x", 32)), nil
	})
	if !errors.Is(err, context.Canceled) || got != (LocalReadRequest{}) {
		t.Fatalf("resultado após cancelamento: %+v %v", got, err)
	}
}

package credentials

import "testing"

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

func containsDash(value string) bool {
	for _, r := range value {
		if r == '-' {
			return true
		}
	}
	return false
}

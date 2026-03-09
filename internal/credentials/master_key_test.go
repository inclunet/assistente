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
		t.Fatalf("recovery key muito curta")
	}
	if key == "" || key[0] < 'A' || key[0] > 'Z' {
		t.Fatalf("recovery key deve estar em maiúsculas")
	}
	if !containsDash(key) {
		t.Fatalf("recovery key deve conter separadores")
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

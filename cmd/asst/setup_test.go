package main

import (
	"fmt"
	"strings"
	"testing"
)

// fakePasswordReader returns a passwordReader that serves passwords in order.
func fakePasswordReader(passwords ...string) passwordReader {
	idx := 0
	return func(prompt string) (string, error) {
		if idx >= len(passwords) {
			return "", fmt.Errorf("no more passwords")
		}
		p := passwords[idx]
		idx++
		return p, nil
	}
}

func TestReadMasterPassword_Success(t *testing.T) {
	readPwd := fakePasswordReader("senhasegura", "senhasegura")
	pwd, err := readMasterPassword(readPwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pwd != "senhasegura" {
		t.Errorf("expected 'senhasegura', got %q", pwd)
	}
}

func TestReadMasterPassword_TooShort(t *testing.T) {
	readPwd := fakePasswordReader("short", "short")
	_, err := readMasterPassword(readPwd)
	if err == nil {
		t.Fatal("expected error for short password")
	}
	if !strings.Contains(err.Error(), "pelo menos 8 caracteres") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadMasterPassword_Mismatch(t *testing.T) {
	readPwd := fakePasswordReader("senhasegura", "outrasenha!")
	_, err := readMasterPassword(readPwd)
	if err == nil {
		t.Fatal("expected error for mismatched passwords")
	}
	if !strings.Contains(err.Error(), "não coincidem") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadMasterPassword_ReadError(t *testing.T) {
	readPwd := func(prompt string) (string, error) {
		return "", fmt.Errorf("terminal closed")
	}
	_, err := readMasterPassword(readPwd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao ler senha") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// readProviderChoice
// ---------------------------------------------------------------------------

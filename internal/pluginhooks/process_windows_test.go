package pluginhooks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Só o job windows-installer habilita o processo real. O diretório pertence ao build.
func TestWindowsHookIsolation(t *testing.T) {
	parent := os.Getenv("ASSISTENTE_HOOK_TEST_ROOT")
	if parent == "" {
		t.Skip("integração exclusiva do windows-installer")
	}
	root, err := os.MkdirTemp(parent, "isolation-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	}()
	secret := filepath.Join(root, "private.txt")
	if err = os.WriteFile(secret, []byte("privado"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ASSISTENTE_HOOK_PRIVATE_VALUE", "privado")
	// O acesso ao fixture privado precisa ser negado pela DACL mesmo quando seu caminho é conhecido.
	user, err := currentUserPrivateFile(secret)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("fixture privado para", user)
	pathLiteral := "'" + strings.ReplaceAll(secret, "'", "''") + "'"
	script := `$ErrorActionPreference = 'Stop'
 $ev = [Console]::In.ReadToEnd() | ConvertFrom-Json
 if ($ev.tool_name -ne 'example') { exit 11 }
 if ($env:ASSISTENTE_HOOK_PRIVATE_VALUE) { exit 12 }
 if ([IO.File]::ReadAllText((Join-Path $PSScriptRoot 'rules.txt')) -ne 'deny') { exit 13 }
 try { [void][IO.File]::ReadAllText(` + pathLiteral + `); exit 14 } catch [UnauthorizedAccessException] {}
 try { [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'rules.txt'), 'allow'); exit 15 } catch [UnauthorizedAccessException] {}
 [Console]::Out.Write('veto')
 exit 2`
	files := map[string][]byte{"pre.ps1": []byte(script), "rules.txt": []byte("deny")}
	out, status, err := Run(t.Context(), root, files, "pre.ps1", []byte(`{"tool_name":"example"}`))
	if err != nil || status != 2 || string(out) != "veto" {
		t.Fatalf("status=%s saída=%q err=%v", strconv.FormatUint(uint64(status), 16), out, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 7*time.Second)
	defer cancel()
	out, _, err = Run(ctx, root, map[string][]byte{"wait.ps1": []byte("[Console]::Out.Write('iniciado'); [Console]::Out.Flush(); while ($true) { }")}, "wait.ps1", nil)
	if string(out) != "iniciado" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("prazo: %v, saída %q", err, out)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "private.txt" {
		t.Fatalf("limpeza: %v, %v", entries, err)
	}
}

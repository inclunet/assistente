package shell

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveProjectWorkDirAcceptsNestedRelativePath(t *testing.T) {
	root := t.TempDir()
	requested := filepath.Join("src", "module")
	if err := os.MkdirAll(filepath.Join(root, requested), 0700); err != nil {
		t.Fatal(err)
	}
	got, err := resolveProjectWorkDir(root, requested)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalRoot, "src", "module")
	if got != want {
		t.Fatalf("diretório = %q, esperado %q", got, want)
	}
}

func TestResolveProjectWorkDirReportsMissingNestedTarget(t *testing.T) {
	root := t.TempDir()
	requested := filepath.Join("not-created", "yet")

	got, err := resolveProjectWorkDir(root, requested)
	if got != "" || err == nil || !strings.Contains(err.Error(), "não existe") {
		t.Fatalf("diretório ausente: caminho=%q erro=%v", got, err)
	}
}

func TestRunCommandValidatesExistingWorkDirBeforeAuthorization(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "arquivo")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, project, requested, cause string
		prerequisite                    bool
	}{
		{"destino ausente", root, "ausente", "não existe", false},
		{"destino arquivo", root, "arquivo", "não é um diretório", false},
		{"raiz ausente", filepath.Join(root, "ausente"), "subdir", "", true},
		{"raiz arquivo", file, "subdir", "", true},
		{"raiz padrão ausente", filepath.Join(root, "ausente"), "", "", true},
		{"raiz padrão arquivo", file, "", "", true},
	} {
		for _, mode := range []string{"efêmero", "persistente"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				mgr := &MockSessionManager{}
				rc := NewRunCommand(mgr, func(context.Context, string, string) (bool, error) {
					t.Fatal("solicitou confirmação com diretório inválido")
					return false, nil
				}, nil, tc.project)
				params := map[string]any{"command": "pwd", "persistent": mode == "persistente"}
				if tc.requested != "" {
					params["working_directory"] = tc.requested
				}
				args, err := json.Marshal(params)
				if err != nil {
					t.Fatal(err)
				}
				result, err := rc.Execute(t.Context(), args)
				if err != nil || !result.IsError || strings.Contains(result.Content, root) {
					t.Fatalf("resultado=%+v, erro=%v", result, err)
				}
				if tc.prerequisite {
					if result.Failure == nil || result.Failure.Code != FailurePrerequisiteUnavailable {
						t.Fatalf("raiz indisponível: %+v", result)
					}
				} else if result.Failure != nil || !strings.HasPrefix(result.Content, "working_directory inválido: ") || !strings.Contains(result.Content, tc.cause) {
					t.Fatalf("diagnóstico do argumento: %+v", result)
				}
				if mgr.acquireCalls != 0 || mgr.runEphemeralCalls.Load() != 0 || mgr.runCommandCalls != 0 {
					t.Fatal("diretório indisponível chegou ao terminal")
				}
			})
		}
	}
}

func TestRunCommandRevalidatesDefaultRootAfterConsent(t *testing.T) {
	for _, mode := range []string{"efêmero", "persistente"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "projeto")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			mgr := &MockSessionManager{}
			prompts := 0
			rc := NewRunCommand(mgr, func(context.Context, string, string) (bool, error) {
				prompts++
				if err := os.Rename(root, root+"-movido"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(root, nil, 0600); err != nil {
					t.Fatal(err)
				}
				return true, nil
			}, nil, root)
			args, err := json.Marshal(map[string]any{"command": "pwd", "persistent": mode == "persistente"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := rc.Execute(t.Context(), args)
			if err != nil || result.Failure == nil || result.Failure.Code != FailurePrerequisiteUnavailable ||
				prompts != 1 || mgr.acquireCalls != 0 || mgr.runEphemeralCalls.Load() != 0 {
				t.Fatalf("resultado=%+v erro=%v confirmações=%d acquire=%d efêmero=%d", result, err, prompts, mgr.acquireCalls, mgr.runEphemeralCalls.Load())
			}
		})
	}
}

func TestResolveProjectWorkDirRejectsSymlinkOutsideWithMissingDescendant(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link-outside")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("permissão para symlink indisponível: %v", err)
		}
		t.Fatal(err)
	}

	if _, err := resolveProjectWorkDir(root, filepath.Join("link-outside", "not-created")); err == nil {
		t.Fatal("symlink para fora com descendente ausente foi aceito")
	}
	if _, err := resolveProjectWorkDir(root, "link-outside"); err == nil || !strings.Contains(err.Error(), "dentro do diretório do projeto") {
		t.Fatalf("link para diretório externo existente deveria falhar por sair da raiz: %v", err)
	}
}

func TestResolveProjectWorkDirRejectsDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(root, "does-not-exist"), link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("permissão para symlink indisponível: %v", err)
		}
		t.Fatal(err)
	}

	if _, err := resolveProjectWorkDir(root, filepath.Join("dangling", "child")); err == nil {
		t.Fatal("symlink dangling foi tratado como componente inexistente")
	}
}

func TestResolveProjectWorkDirRejectsEscapesAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	for _, requested := range []string{
		filepath.Join("..", "outside"),
		root,
		`C:\outside`,
		`\\server\share`,
	} {
		t.Run(requested, func(t *testing.T) {
			if _, err := resolveProjectWorkDir(root, requested); err == nil {
				t.Fatalf("esperava rejeição de %q", requested)
			}
		})
	}
}

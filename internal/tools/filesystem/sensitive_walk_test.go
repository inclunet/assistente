package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// prepararSymlinkSensivel cria um workDir com um .env e um symlink de nome
// inócuo apontando para ele, além de um arquivo normal de controle.
func prepararSymlinkSensivel(t *testing.T) (workDir string, link string) {
	t.Helper()

	workDir = t.TempDir()
	envFile := filepath.Join(workDir, ".env")
	if err := os.WriteFile(envFile, []byte("SECRET=valor"), 0o600); err != nil {
		t.Fatalf("escrever .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "normal.txt"), []byte("SECRET=valor"), 0o644); err != nil {
		t.Fatalf("escrever normal.txt: %v", err)
	}

	link = filepath.Join(workDir, "innocent.txt")
	if err := os.Symlink(envFile, link); err != nil {
		t.Fatalf("criar symlink: %v", err)
	}
	return workDir, link
}

// TestWalkers_SymlinkParaSensivelNaoVaza garante que list_dir, search_files e
// grep_search filtram pelo destino real: um symlink com nome inócuo apontando
// para arquivo sensível não pode aparecer nos resultados (AEP-0092 D-Q5).
func TestWalkers_SymlinkParaSensivelNaoVaza(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks podem requerer privilégios elevados no Windows")
	}

	workDir, _ := prepararSymlinkSensivel(t)
	ctx := context.Background()

	casos := []struct {
		nome string
		exec func() (string, bool)
	}{
		{
			nome: "list_dir",
			exec: func() (string, bool) {
				args, _ := json.Marshal(map[string]any{"path": "."})
				res, err := NewListDirectory(workDir).Execute(ctx, args)
				if err != nil {
					t.Fatalf("list_dir: %v", err)
				}
				return res.Content, res.IsError
			},
		},
		{
			nome: "list_dir_recursivo",
			exec: func() (string, bool) {
				args, _ := json.Marshal(map[string]any{"path": ".", "recursive": true, "max_depth": 3})
				res, err := NewListDirectory(workDir).Execute(ctx, args)
				if err != nil {
					t.Fatalf("list_dir recursivo: %v", err)
				}
				return res.Content, res.IsError
			},
		},
		{
			nome: "search_files",
			exec: func() (string, bool) {
				args, _ := json.Marshal(map[string]any{"pattern": "**/*.txt", "max_results": 50})
				res, err := NewSearchFiles(workDir).Execute(ctx, args)
				if err != nil {
					t.Fatalf("search_files: %v", err)
				}
				return res.Content, res.IsError
			},
		},
		{
			nome: "grep_search",
			exec: func() (string, bool) {
				args, _ := json.Marshal(map[string]any{"pattern": "SECRET"})
				res, err := NewGrepSearch(workDir).Execute(ctx, args)
				if err != nil {
					t.Fatalf("grep_search: %v", err)
				}
				return res.Content, res.IsError
			},
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			content, isErr := caso.exec()
			if isErr {
				t.Fatalf("execução retornou erro: %s", content)
			}
			if strings.Contains(content, "innocent.txt") {
				t.Errorf("symlink para arquivo sensível vazou no resultado:\n%s", content)
			}
			if strings.Contains(content, ".env") {
				t.Errorf("arquivo sensível vazou no resultado:\n%s", content)
			}
		})
	}
}

// TestFileOps_SymlinkParaSensivelBloqueado garante que copiar, remover e
// renomear também olham o destino real do link.
func TestFileOps_SymlinkParaSensivelBloqueado(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks podem requerer privilégios elevados no Windows")
	}

	workDir, link := prepararSymlinkSensivel(t)

	// Destinos distintos por operação: reaproveitar o mesmo destino faria o
	// move falhar por "destino já existe" e o teste passaria por engano.
	if _, err := CopyFileWithPolicy(link, filepath.Join(workDir, "copia.txt"), false, ToolPolicy()); err == nil {
		t.Error("copiar symlink para arquivo sensível deveria ser bloqueado")
	}
	if err := MoveFileWithPolicy(link, filepath.Join(workDir, "movido.txt"), false, ToolPolicy()); err == nil {
		t.Error("mover symlink para arquivo sensível deveria ser bloqueado")
	}
	if err := RemoveFileWithPolicy(link, ToolPolicy()); err == nil {
		t.Error("remover symlink para arquivo sensível deveria ser bloqueado")
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("symlink não deveria ter sido consumido pelas operações bloqueadas: %v", err)
	}
}

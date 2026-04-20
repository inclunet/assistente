package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestValidatePath_PathTraversal testa proteção contra path traversal attacks
func TestValidatePath_PathTraversal(t *testing.T) {
	workDir := t.TempDir()

	tests := []struct {
		name      string
		path      string
		shouldErr bool
		desc      string
		winOnly   bool
	}{
		// Path traversal attempts
		{"traversal_parent", "../../../etc/passwd", true, "deve bloquear ../", false},
		{"traversal_parent2", "..\\..\\.\\etc\\passwd", true, "deve bloquear ..\\", true},
		{"traversal_double_dot", "dir/../../etc/passwd", true, "deve bloquear traversal após subdir", false},

		// Valid paths within workDir
		{"valid_file", "file.txt", false, "arquivo simples", false},
		{"valid_subdir", "subdir/file.txt", false, "arquivo em subdiretório", false},
		{"valid_nested", "a/b/c/d/file.txt", false, "arquivo profundamente aninhado", false},

		// Edge cases
		{"dot_prefix", ".hidden/file.txt", false, "arquivo em diretório hidden", false},
		{"root_name", ".", false, "raiz do workDir", false},
		{"current_dir", "./file.txt", false, "arquivo com ./ prefixo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.winOnly && runtime.GOOS != "windows" {
				t.Skipf("backslash path traversal only applies on Windows")
			}
			fullPath := filepath.Join(workDir, tt.path)
			err := validatePath(fullPath, workDir)

			if tt.shouldErr && err == nil {
				t.Errorf("%s: esperado erro, got nil (path: %s)", tt.desc, fullPath)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("%s: esperado sucesso, got erro: %v (path: %s)", tt.desc, err, fullPath)
			}
		})
	}
}

// TestValidatePathWithPolicy_SensitiveFiles testa bloqueio de arquivos sensíveis
func TestValidatePathWithPolicy_SensitiveFiles(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()

	// Criar alguns arquivos sensíveis
	sensitiveFiles := []string{".env", ".env.local", "id_rsa", "server.key", "cert.pem"}
	for _, name := range sensitiveFiles {
		_ = os.WriteFile(filepath.Join(workDir, name), []byte("secret"), 0600)
	}

	tests := []struct {
		filename string
		policy   Policy
		op       string
		shouldErr bool
		desc     string
	}{
		// Tool policy (BlockSensitive = true)
		{".env", ToolPolicy(), "write", true, "ToolPolicy deve bloquear .env em write"},
		{".env.local", ToolPolicy(), "read", true, "ToolPolicy deve bloquear .env em read"},
		{"id_rsa", ToolPolicy(), "edit", true, "ToolPolicy deve bloquear id_rsa em edit"},
		{"server.key", ToolPolicy(), "move", true, "ToolPolicy deve bloquear server.key em move"},

		// Editor policy (BlockSensitive = false)
		{".env", EditorPolicy(), "write", false, "EditorPolicy deve permitir .env em write"},
		{"id_rsa", EditorPolicy(), "read", false, "EditorPolicy deve permitir id_rsa em read"},

		// Non-sensitive files
		{"main.go", ToolPolicy(), "read", false, "deve permitir read de arquivo normal"},
		{"config.yaml", ToolPolicy(), "write", false, "deve permitir write de arquivo normal"},
		{"data.json", EditorPolicy(), "edit", false, "deve permitir edit de arquivo normal"},
	}

	for _, tt := range tests {
		t.Run(tt.filename+"_"+tt.op, func(t *testing.T) {
			fullPath := filepath.Join(workDir, tt.filename)
			err := validatePathWithPolicy(ctx, fullPath, workDir, tt.policy, tt.op)

			if tt.shouldErr && err == nil {
				t.Errorf("%s: esperado erro, got nil", tt.desc)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("%s: esperado sucesso, got erro: %v", tt.desc, err)
			}
		})
	}
}

// TestValidatePath_HomeDirectory testa acesso ao diretório home
func TestValidatePath_HomeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("home dir validation test não é confiável no Windows")
	}

	workDir := t.TempDir()

	// Paths dentro de workDir devem ser válidos
	validPath := filepath.Join(workDir, "file.txt")
	if err := validatePath(validPath, workDir); err != nil {
		t.Errorf("path dentro de workDir deve ser válido: %v", err)
	}

	// Paths completamente fora devem ser inválidos
	outsidePath := "/outside/the/workdir/file.txt"
	if err := validatePath(outsidePath, workDir); err == nil {
		t.Error("path fora de workDir deve ser rejeitado")
	}
}

// TestValidatePath_InvalidInput testa validação de entrada
func TestValidatePath_InvalidInput(t *testing.T) {
	tests := []struct {
		fullPath string
		workDir  string
		shouldErr bool
		desc     string
	}{
		{"", "", true, "paths vazios"},
		{"/some/path", "", true, "workDir vazio"},
		{"", "/some/path", true, "fullPath vazio"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			err := validatePath(tt.fullPath, tt.workDir)
			if tt.shouldErr && err == nil {
				t.Errorf("%s: esperado erro, got nil", tt.desc)
			}
		})
	}
}

// TestNormalizeForComparison testa normalização de paths
func TestNormalizeForComparison(t *testing.T) {
	tests := []struct {
		input  string
		desc   string
	}{
		{"/path/to/../file.txt", "deve remover .."},
		{"/path/./to/file.txt", "deve remover ."},
		{"/path//to///file.txt", "deve colapsar barras múltiplas"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := normalizeForComparison(tt.input)
			// Não deve conter .. ou múltiplas barras
			if filepath.Clean(result) != result {
				t.Errorf("%s: normalização não completa: %s", tt.desc, result)
			}
		})
	}
}

// TestIsWithinRoot testa detecção de caminhos dentro de root
func TestIsWithinRoot(t *testing.T) {
	workDir := t.TempDir()

	tests := []struct {
		absPath   string
		absRoot   string
		expected  bool
		desc      string
	}{
		{filepath.Join(workDir, "file.txt"), workDir, true, "arquivo direto em root"},
		{filepath.Join(workDir, "sub", "file.txt"), workDir, true, "arquivo em subdir"},
		{workDir, workDir, true, "root path itself"},
		{filepath.Dir(workDir), workDir, false, "parent de root"},
		{"/etc/passwd", workDir, false, "path completamente fora"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := isWithinRoot(tt.absPath, tt.absRoot)
			if result != tt.expected {
				t.Errorf("%s: expected %v, got %v", tt.desc, tt.expected, result)
			}
		})
	}
}

// TestValidatePathWithPolicy_ErrorMessages testa mensagens de erro
func TestValidatePathWithPolicy_ErrorMessages(t *testing.T) {
	workDir := t.TempDir()
	ctx := context.Background()

	// Criar arquivo sensível
	_ = os.WriteFile(filepath.Join(workDir, ".env"), []byte("secret=value"), 0600)

	envPath := filepath.Join(workDir, ".env")

	operations := []string{"read", "write", "edit", "move"}
	expectedMsgs := map[string]string{
		"read":  "ler",
		"write": "escrever",
		"edit":  "editar",
		"move":  "mover",
	}

	for _, op := range operations {
		t.Run("error_msg_"+op, func(t *testing.T) {
			err := validatePathWithPolicy(ctx, envPath, workDir, ToolPolicy(), op)
			if err == nil {
				t.Fatal("esperado erro")
			}

			errMsg := err.Error()
			expectedPart := expectedMsgs[op]
			if !contains(errMsg, expectedPart) {
				t.Errorf("mensagem deveria conter '%s', got: %s", expectedPart, errMsg)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

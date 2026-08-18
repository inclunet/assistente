package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/tools"
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
		filename  string
		policy    Policy
		op        string
		shouldErr bool
		desc      string
	}{
		// Tool policy (BlockSensitive = true)
		{".env", ToolPolicy(), "write", true, "ToolPolicy deve bloquear .env em write"},
		{".env.local", ToolPolicy(), "read", true, "ToolPolicy deve bloquear .env em read"},
		{"id_rsa", ToolPolicy(), "edit", true, "ToolPolicy deve bloquear id_rsa em edit"},
		{"server.key", ToolPolicy(), "move_from", true, "ToolPolicy deve bloquear server.key em move_from"},

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

// TestBlockSensitiveForOperation_Mensagens fixa a mensagem por operação: cair
// no default genérico esconde do usuário o que exatamente foi negado.
func TestBlockSensitiveForOperation_Mensagens(t *testing.T) {
	dir := t.TempDir()
	casos := map[string]string{
		"delete":    "não é permitido excluir arquivos sensíveis",
		"mkdir":     "não é permitido criar diretórios sensíveis",
		"copy_from": "não é permitido copiar arquivos sensíveis",
		"copy_to":   "não é permitido copiar arquivos sensíveis",
		"move_from": "não é permitido mover/renomear arquivos sensíveis",
		"move_to":   "não é permitido mover/renomear arquivos sensíveis",
	}
	for operacao, esperado := range casos {
		t.Run(operacao, func(t *testing.T) {
			err := blockSensitiveForOperation(filepath.Join(dir, ".env"), operacao)
			if err == nil {
				t.Fatalf("%s de arquivo sensível deveria ser bloqueado", operacao)
			}
			if got := err.Error(); got != esperado {
				t.Fatalf("mensagem de %s inesperada: %q", operacao, got)
			}
		})
	}
}

// TestFileOps_MensagensConsistentesComPolicy garante que a operação direta e a
// validação prévia falam a mesma língua. Divergir aqui faz o usuário receber
// uma frase que não descreve o que ele pediu, e bagunça a telemetria.
func TestFileOps_MensagensConsistentesComPolicy(t *testing.T) {
	dir := t.TempDir()
	alvo := filepath.Join(dir, ".env")
	if err := os.WriteFile(alvo, []byte("x"), 0o600); err != nil {
		t.Fatalf("escrever .env: %v", err)
	}

	casos := []struct {
		nome     string
		operacao string
		executar func() error
	}{
		{
			nome:     "remover",
			operacao: "delete",
			executar: func() error { return RemoveFileWithPolicy(alvo, ToolPolicy()) },
		},
		{
			nome:     "copiar",
			operacao: "copy_from",
			executar: func() error {
				_, err := CopyFileWithPolicy(alvo, filepath.Join(dir, "copia.txt"), false, ToolPolicy())
				return err
			},
		},
		{
			nome:     "mover",
			operacao: "move_from",
			executar: func() error {
				return MoveFileWithPolicy(alvo, filepath.Join(dir, "movido.txt"), false, ToolPolicy())
			},
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			err := caso.executar()
			if err == nil {
				t.Fatalf("%s arquivo sensível deveria ser bloqueado", caso.nome)
			}
			esperado := blockSensitiveForOperation(alvo, caso.operacao).Error()
			if got := err.Error(); got != esperado {
				t.Fatalf("mensagem divergente: got %q, want %q", got, esperado)
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
		fullPath  string
		workDir   string
		shouldErr bool
		desc      string
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
		input string
		desc  string
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
		absPath  string
		absRoot  string
		expected bool
		desc     string
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

	operations := []string{"read", "write", "edit", "move_from"}
	expectedMsgs := map[string]string{
		"read":      "ler",
		"write":     "escrever",
		"edit":      "editar",
		"move_from": "mover",
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

// TestValidatePathWithPolicy_OpenEditorPaths_NoLongerBypass (AEP-0092): abrir no
// editor NÃO libera path fora do sandbox sem PathAuthorizer.
func TestValidatePathWithPolicy_OpenEditorPaths_NoLongerBypass(t *testing.T) {
	workDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "doc.txt")
	_ = os.WriteFile(outsideFile, []byte("conteúdo"), 0644)

	ctx := tools.WithOpenEditorPaths(context.Background(), []string{outsideFile})
	if err := validatePathWithPolicy(ctx, outsideFile, workDir, ToolPolicy(), "read"); err == nil {
		t.Fatal("arquivo fora do workDir não deve ser liberado só por estar aberto no editor")
	}
}

type stubPathAuthorizer struct {
	allow   bool
	err     error
	denyErr error
}

func (s stubPathAuthorizer) Authorize(ctx context.Context, absPath, operation string) error {
	if s.denyErr != nil {
		return s.denyErr
	}
	if s.err != nil {
		return s.err
	}
	if s.allow {
		return nil
	}
	return errOutsideAllowedDirs
}

func (s stubPathAuthorizer) DeniedResolved(ctx context.Context, resolvedPath, operation string) error {
	return s.denyErr
}

func TestValidatePathWithPolicy_DenyInsideSandbox(t *testing.T) {
	workDir := t.TempDir()
	inside := filepath.Join(workDir, "bloqueado.txt")
	if err := os.WriteFile(inside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	prev := pathAuthorizer
	t.Cleanup(func() { pathAuthorizer = prev })
	pathAuthorizer = stubPathAuthorizer{
		denyErr: fmt.Errorf("bloqueado pela denylist (escopo global)"),
	}

	err := validatePathWithPolicy(context.Background(), inside, workDir, ToolPolicy(), "read")
	if err == nil {
		t.Fatal("deny dentro do sandbox deveria bloquear")
	}
	if !strings.Contains(err.Error(), "denylist") {
		t.Fatalf("mensagem inesperada: %v", err)
	}
}

func TestValidatePathWithPolicy_DenyOutsideUsesAuthorizeOnce(t *testing.T) {
	workDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "doc.txt")
	if err := os.WriteFile(outsideFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	prev := pathAuthorizer
	t.Cleanup(func() { pathAuthorizer = prev })
	counter := &countingPathAuthorizer{
		denyErr: fmt.Errorf("bloqueado pela denylist (escopo global)"),
	}
	pathAuthorizer = counter

	err := validatePathWithPolicy(context.Background(), outsideFile, workDir, ToolPolicy(), "read")
	if err == nil {
		t.Fatal("deny fora do sandbox deveria bloquear")
	}
	if counter.deniedCalls != 1 {
		t.Fatalf("Denied deveria rodar 1 vez via Authorize, got %d", counter.deniedCalls)
	}
	if counter.authorizeCalls != 1 {
		t.Fatalf("Authorize deveria rodar 1 vez, got %d", counter.authorizeCalls)
	}
}

type countingPathAuthorizer struct {
	denyErr        error
	deniedCalls    int
	authorizeCalls int
}

func (c *countingPathAuthorizer) Authorize(ctx context.Context, absPath, operation string) error {
	c.authorizeCalls++
	// Fora do sandbox, o Authorize aplica a precedência de deny internamente.
	c.deniedCalls++
	return c.denyErr
}

func (c *countingPathAuthorizer) DeniedResolved(ctx context.Context, resolvedPath, operation string) error {
	c.deniedCalls++
	return c.denyErr
}

func TestValidatePathWithPolicy_PathAuthorizerAllowsOutside(t *testing.T) {
	workDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "doc.txt")
	_ = os.WriteFile(outsideFile, []byte("ok"), 0644)

	prev := pathAuthorizer
	t.Cleanup(func() { pathAuthorizer = prev })
	pathAuthorizer = stubPathAuthorizer{allow: true}

	if err := validatePathWithPolicy(context.Background(), outsideFile, workDir, ToolPolicy(), "read"); err != nil {
		t.Fatalf("authorizer allow deveria liberar: %v", err)
	}
}

func TestValidatePathWithPolicy_PathAuthorizerSensitiveStillBlocked(t *testing.T) {
	workDir := t.TempDir()
	outsideDir := t.TempDir()
	envFile := filepath.Join(outsideDir, ".env")
	_ = os.WriteFile(envFile, []byte("SECRET=value"), 0600)

	prev := pathAuthorizer
	t.Cleanup(func() { pathAuthorizer = prev })
	pathAuthorizer = stubPathAuthorizer{allow: true}

	if err := validatePathWithPolicy(context.Background(), envFile, workDir, ToolPolicy(), "read"); err == nil {
		t.Fatal("arquivo sensível deve continuar bloqueado mesmo com PathAuthorizer")
	}
	if err := validatePathWithPolicy(context.Background(), envFile, workDir, EditorPolicy(), "read"); err != nil {
		t.Fatalf("EditorPolicy deveria permitir após authorizer: %v", err)
	}
}

// TestValidatePathWithPolicy_OpenEditorInsideWorkDirUnchanged testa que arquivos dentro do
// workDir continuam funcionando normalmente (regressão).
func TestValidatePathWithPolicy_OpenEditorInsideWorkDirUnchanged(t *testing.T) {
	workDir := t.TempDir()

	insideFile := filepath.Join(workDir, "inside.txt")
	_ = os.WriteFile(insideFile, []byte("ok"), 0644)

	ctx := context.Background()

	// Sem open editor paths, arquivo dentro do workDir funciona normalmente
	if err := validatePathWithPolicy(ctx, insideFile, workDir, ToolPolicy(), "read"); err != nil {
		t.Errorf("arquivo dentro do workDir deveria ser permitido: %v", err)
	}
}

// TestValidatePathWithPolicy_OpenEditorInvalidWorkDirNotBypassed testa que erros de workDir
// inválido NÃO são bypassados.
func TestValidatePathWithPolicy_OpenEditorInvalidWorkDirNotBypassed(t *testing.T) {
	someFile := filepath.Join(t.TempDir(), "file.txt")
	_ = os.WriteFile(someFile, []byte("data"), 0644)

	ctx := tools.WithOpenEditorPaths(context.Background(), []string{someFile})

	if err := validatePathWithPolicy(ctx, someFile, "", ToolPolicy(), "read"); err == nil {
		t.Error("workDir inválido deveria retornar erro mesmo com open editor paths")
	}
}

// TestValidatePathWithPolicy_SymlinkToSensitiveBlockedInsideWorkDir garante que
// um symlink com nome inócuo apontando para arquivo sensível é bloqueado por
// ToolPolicy mesmo estando DENTRO do workDir (onde o path validation passaria)
// — fechando o bypass do sensitive check que olhava só o basename (AEP-0092 D-Q5).
func TestValidatePathWithPolicy_SymlinkToSensitiveBlockedInsideWorkDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks podem requerer privilégios elevados no Windows")
	}

	workDir := t.TempDir()

	envFile := filepath.Join(workDir, ".env")
	_ = os.WriteFile(envFile, []byte("SECRET=value"), 0600)

	linkFile := filepath.Join(workDir, "innocent.txt")
	if err := os.Symlink(envFile, linkFile); err != nil {
		t.Fatalf("falha ao criar symlink: %v", err)
	}

	ctx := context.Background()

	// O link mora dentro do workDir (path validation passa), mas resolve para
	// um arquivo sensível: ToolPolicy deve bloquear em qualquer operação.
	for _, op := range []string{"read", "write", "edit", "move_from"} {
		if err := validatePathWithPolicy(ctx, linkFile, workDir, ToolPolicy(), op); err == nil {
			t.Errorf("symlink %q → arquivo sensível deveria ser bloqueado (op=%s)", linkFile, op)
		}
	}

	// Arquivo normal dentro do workDir continua permitido (regressão).
	normalFile := filepath.Join(workDir, "normal.txt")
	_ = os.WriteFile(normalFile, []byte("ok"), 0644)
	if err := validatePathWithPolicy(ctx, normalFile, workDir, ToolPolicy(), "read"); err != nil {
		t.Errorf("arquivo normal dentro do workDir deveria ser permitido: %v", err)
	}
}

// TestValidatePathWithPolicy_DanglingSymlinkFailsClosed garante fail-closed: um
// symlink existente cujo alvo não resolve (EvalSymlinks falha) é tratado como
// sensível e bloqueado por ToolPolicy — sem reabrir bypass por falha de resolução.
func TestValidatePathWithPolicy_DanglingSymlinkFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks podem requerer privilégios elevados no Windows")
	}

	workDir := t.TempDir()

	// Alvo inexistente → symlink pendurado (dangling): EvalSymlinks falha, mas
	// Lstat confirma que é symlink → deve falhar fechado.
	target := filepath.Join(workDir, "does-not-exist")
	linkFile := filepath.Join(workDir, "innocent.txt")
	if err := os.Symlink(target, linkFile); err != nil {
		t.Fatalf("falha ao criar symlink: %v", err)
	}

	if err := validatePathWithPolicy(context.Background(), linkFile, workDir, ToolPolicy(), "read"); err == nil {
		t.Error("symlink pendurado deveria falhar fechado (bloqueado) sob ToolPolicy")
	}
}

// TestValidatePath_SymlinkEscapeBlocked garante que um symlink DENTRO do workDir
// apontando para fora (workDir/link -> outsideDir) não burla o sandbox: o path
// resolvido cai fora das raízes permitidas e, sem PathAuthorizer, é negado
// (errOutsideAllowedDirs) em vez de ser lido direto pelo os.ReadFile (AEP-0092).
func TestValidatePath_SymlinkEscapeBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks podem requerer privilégios elevados no Windows")
	}

	workDir := t.TempDir()
	outsideDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("x"), 0644)

	link := filepath.Join(workDir, "link")
	if err := os.Symlink(outsideDir, link); err != nil {
		t.Fatalf("falha ao criar symlink: %v", err)
	}

	// Arquivo existente através do link → destino real fora do sandbox.
	viaLink := filepath.Join(link, "secret.txt")
	if err := validatePath(viaLink, workDir); !errors.Is(err, errOutsideAllowedDirs) {
		t.Errorf("leitura via symlink para fora deveria cair em errOutsideAllowedDirs, obtido: %v", err)
	}

	// Arquivo novo (inexistente) através do link → ancestral resolvido fora.
	novoViaLink := filepath.Join(link, "novo.txt")
	if err := validatePath(novoViaLink, workDir); !errors.Is(err, errOutsideAllowedDirs) {
		t.Errorf("escrita via symlink para fora deveria cair em errOutsideAllowedDirs, obtido: %v", err)
	}

	// O próprio último componente é um symlink pendurado para um alvo externo
	// ainda inexistente. Lstat+Readlink precisa determinar esse alvo mesmo que
	// EvalSymlinks não consiga resolvê-lo.
	danglingTarget := filepath.Join(outsideDir, "ainda-inexistente.txt")
	danglingLink := filepath.Join(workDir, "dangling.txt")
	if err := os.Symlink(danglingTarget, danglingLink); err != nil {
		t.Fatalf("falha ao criar symlink pendurado: %v", err)
	}
	if err := validatePath(danglingLink, workDir); !errors.Is(err, errOutsideAllowedDirs) {
		t.Errorf("symlink final pendurado para fora deveria cair em errOutsideAllowedDirs, obtido: %v", err)
	}

	// Regressão: arquivo real dentro do workDir continua permitido.
	inside := filepath.Join(workDir, "inside.txt")
	_ = os.WriteFile(inside, []byte("ok"), 0644)
	if err := validatePath(inside, workDir); err != nil {
		t.Errorf("arquivo real dentro do workDir deveria ser permitido: %v", err)
	}
}

func TestValidatePath_SandboxRootFunc(t *testing.T) {
	bootDir := t.TempDir()
	active := t.TempDir()
	fileInActive := filepath.Join(active, "x.txt")
	_ = os.WriteFile(fileInActive, []byte("x"), 0644)

	prev := sandboxRootFunc
	t.Cleanup(func() { sandboxRootFunc = prev })
	sandboxRootFunc = func() string { return active }

	if err := validatePath(fileInActive, bootDir); err != nil {
		t.Fatalf("path no workspace ativo deveria passar: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "y.txt")
	_ = os.WriteFile(outside, []byte("y"), 0644)
	if err := validatePath(outside, bootDir); err == nil {
		t.Fatal("path fora do workspace ativo deveria falhar")
	}
}

func TestResolveFilePath_UsesActiveSandboxRoot(t *testing.T) {
	bootDir := t.TempDir()
	active := t.TempDir()

	prev := sandboxRootFunc
	t.Cleanup(func() { sandboxRootFunc = prev })
	sandboxRootFunc = func() string { return active }

	resolved, err := resolveFilePath(filepath.Join("docs", "note.md"), bootDir)
	if err != nil {
		t.Fatalf("resolveFilePath relativo: %v", err)
	}
	want := filepath.Join(active, "docs", "note.md")
	if normalizeForComparison(resolved) != normalizeForComparison(want) {
		t.Fatalf("path relativo deveria usar workspace ativo: got %q, want %q", resolved, want)
	}

	absolute := filepath.Join(bootDir, "absolute.txt")
	resolvedAbsolute, err := resolveFilePath(absolute, bootDir)
	if err != nil {
		t.Fatalf("resolveFilePath absoluto: %v", err)
	}
	if normalizeForComparison(resolvedAbsolute) != normalizeForComparison(absolute) {
		t.Fatalf("path absoluto não deveria ser rebased: got %q, want %q", resolvedAbsolute, absolute)
	}

	// Patterns relativos de skill precisam acompanhar a mesma raiz dinâmica.
	allowed := filepath.Join(active, "docs", "child.txt")
	if !filesystemPatternMatches(allowed, bootDir, filepath.Join("docs", "**")) {
		t.Fatalf("pattern relativo deveria casar path no workspace ativo: %q", allowed)
	}
	oldWorkspacePath := filepath.Join(bootDir, "docs", "child.txt")
	if filesystemPatternMatches(oldWorkspacePath, bootDir, filepath.Join("docs", "**")) {
		t.Fatalf("pattern relativo não deveria continuar casando o diretório de boot: %q", oldWorkspacePath)
	}
}

// TestFilesystemPatternMatches_RaizSymlinkada garante que um pattern de skill
// ancorado numa raiz que é symlink continua casando o path já resolvido — o
// caso real é macOS (/var -> /private/var), que viraria falso-negativo e
// bloqueio indevido se só um dos lados fosse resolvido.
func TestFilesystemPatternMatches_RaizSymlinkada(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks podem requerer privilégios elevados no Windows")
	}

	realDir := t.TempDir()
	docs := filepath.Join(realDir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	child := filepath.Join(docs, "child.txt")
	if err := os.WriteFile(child, []byte("ok"), 0o644); err != nil {
		t.Fatalf("escrever child: %v", err)
	}

	linkRoot := filepath.Join(t.TempDir(), "raiz-link")
	if err := os.Symlink(realDir, linkRoot); err != nil {
		t.Fatalf("criar symlink de raiz: %v", err)
	}

	workDir := realDir
	patterns := []string{
		filepath.Join(linkRoot, "**"),
		filepath.Join(linkRoot, "docs", "**"),
		filepath.Join(linkRoot, "docs", "*.txt"),
	}
	for _, pattern := range patterns {
		if !matchesAnyFilesystemPattern(child, workDir, []string{pattern}) {
			t.Errorf("pattern %q com raiz symlinkada deveria casar %q", pattern, child)
		}
	}

	fora := filepath.Join(t.TempDir(), "outro.txt")
	if matchesAnyFilesystemPattern(fora, workDir, []string{filepath.Join(linkRoot, "**")}) {
		t.Errorf("pattern não deveria casar path fora da raiz: %q", fora)
	}
}

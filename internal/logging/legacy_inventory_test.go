package logging

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	expectedLegacyFormatCount  = 781
	expectedLegacyFormatDigest = "d0ddcfc7f2b7a5843d6074ff287361ce619cfe0cf63ef290cf92ec846934f846"
)

// TestLegacyLoggingInventory mantém reproduzível o inventário da issue #675.
// Ele impede a reintrodução de call sites de Printf em produção e caracteriza
// os formatos que ainda dependem de normalizeLegacyMessage antes de sua
// remoção futura.
func TestLegacyLoggingInventory(t *testing.T) {
	root := repositoryRoot(t)
	var printfSites []string
	var legacyFormats []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		inventoryFile(t, root, path, &printfSites, &legacyFormats)
		return nil
	})
	if err != nil {
		t.Fatalf("percorrer fontes Go: %v", err)
	}

	sort.Strings(printfSites)
	if len(printfSites) != 0 {
		t.Fatalf("logging.Printf foi reintroduzido:\n%s", strings.Join(printfSites, "\n"))
	}

	sort.Strings(legacyFormats)
	digestBytes := sha256.Sum256([]byte(strings.Join(legacyFormats, "\n")))
	digest := hex.EncodeToString(digestBytes[:])
	t.Logf("inventário: logging.Printf=0; formatos alterados pela normalização=%d; sha256=%s",
		len(legacyFormats), digest)
	if len(legacyFormats) != expectedLegacyFormatCount || digest != expectedLegacyFormatDigest {
		t.Fatalf("inventário legado mudou; revise a migração e atualize a baseline conscientemente:\n%s",
			strings.Join(legacyFormats, "\n"))
	}
}

func TestInventoryFileDetectsPrintfInLoggingPackage(t *testing.T) {
	root := t.TempDir()
	packageDir := filepath.Join(root, "internal", "logging")
	if err := os.MkdirAll(packageDir, 0755); err != nil {
		t.Fatalf("criar pacote logging temporário: %v", err)
	}
	path := filepath.Join(packageDir, "sample.go")
	if err := os.WriteFile(path, []byte("package logging\nfunc sample() { Printf(\"mensagem\") }\n"), 0644); err != nil {
		t.Fatalf("criar fonte temporária: %v", err)
	}

	var printfSites []string
	var legacyFormats []string
	inventoryFile(t, root, path, &printfSites, &legacyFormats)

	if len(printfSites) != 1 {
		t.Fatalf("call sites de Printf = %v, want 1", printfSites)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	current, err := os.Getwd()
	if err != nil {
		t.Fatalf("obter diretório de trabalho: %v", err)
	}
	for {
		info, statErr := os.Stat(filepath.Join(current, "go.mod"))
		if statErr == nil && !info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("não foi possível localizar go.mod a partir de %s", current)
		}
		current = parent
	}
}

func inventoryFile(t *testing.T, root, path string, printfSites, legacyFormats *[]string) {
	t.Helper()
	ownPackage := filepath.Clean(filepath.Dir(path)) == filepath.Join(root, "internal", "logging")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse imports de %s: %v", path, err)
	}
	aliases := make(map[string]struct{})
	dotImport := false
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || importPath != "assistente/internal/logging" {
			continue
		}
		if spec.Name == nil {
			aliases["logging"] = struct{}{}
		} else if spec.Name.Name == "." {
			dotImport = true
		} else if spec.Name.Name != "_" {
			aliases[spec.Name.Name] = struct{}{}
		}
	}
	if len(aliases) == 0 && !dotImport && !ownPackage {
		return
	}

	file, err = parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("caminho relativo de %s: %v", path, err)
	}
	relative = filepath.ToSlash(relative)

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		method, ok := loggingMethod(call.Fun, aliases, dotImport || ownPackage)
		if !ok {
			return true
		}
		if method == "Printf" {
			position := fset.Position(call.Pos())
			*printfSites = append(*printfSites, relative+":"+
				strconv.Itoa(position.Line)+":"+strconv.Itoa(position.Column))
			return true
		}
		formatIndex, tracked := legacyFormatIndex(method)
		if !tracked || len(call.Args) <= formatIndex {
			return true
		}
		literal, ok := call.Args[formatIndex].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		format, err := strconv.Unquote(literal.Value)
		if err != nil || normalizeLegacyMessage(format) == strings.TrimSpace(format) {
			return true
		}
		componentIndex := 1
		if method == "Logf" {
			componentIndex = 2
		}
		component := literalString(call.Args, componentIndex)
		*legacyFormats = append(*legacyFormats, strings.Join([]string{
			relative, method, component, format,
		}, "|"))
		return true
	})
}

func loggingMethod(fun ast.Expr, aliases map[string]struct{}, dotImport bool) (string, bool) {
	if ident, ok := fun.(*ast.Ident); ok && dotImport {
		return ident.Name, true
	}
	selector, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	ident, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	_, ok = aliases[ident.Name]
	return selector.Sel.Name, ok
}

func legacyFormatIndex(method string) (int, bool) {
	switch method {
	case "Debugf", "Infof", "Warnf", "Errorf", "Fatalf", "Print", "Println":
		return 2, true
	case "Logf":
		return 3, true
	default:
		return 0, false
	}
}

func literalString(args []ast.Expr, index int) string {
	if len(args) <= index {
		return "<dynamic>"
	}
	literal, ok := args[index].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "<dynamic>"
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "<invalid>"
	}
	return value
}

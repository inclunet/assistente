package architecture

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// utf8BOM é a marca que alguns editores do Windows escrevem no começo do arquivo
// para dizer "isto é UTF-8". Nada no projeto precisa dela, e ela estraga o
// primeiro byte útil de quem lê o arquivo esperando o conteúdo.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Extensões em que a marca faz estrago silencioso: em Go ela quebra a
// compilação (essa dá para achar), e em CSS ela invalida o primeiro seletor do
// arquivo — a regra simplesmente não vale, sem erro em lugar nenhum. Em
// JSON/YAML ela derruba o parser em quem não a tolera.
var bomSensitiveExts = map[string]bool{
	".go":   true,
	".css":  true,
	".ts":   true,
	".tsx":  true,
	".js":   true,
	".mjs":  true,
	".json": true,
	".yml":  true,
	".yaml": true,
}

// Diretórios que não são nossos: dependência baixada e resultado de build.
var skippedDirs = map[string]bool{
	".git":              true,
	"node_modules":      true,
	"dist":              true,
	"build":             true,
	"coverage":          true,
	"playwright-report": true,
	"test-results":      true,
}

// A marca de UTF-8 no começo do arquivo já custou caro duas vezes nesta base: uma
// derrubando build de Go, outra invalidando a primeira regra de um CSS novo — e
// essa segunda passou por stylelint, tsc, CI e duas revisões, porque a única
// pista era um botão com a largura errada.
//
// Este teste é a rede: quem colar um arquivo com a marca descobre no CI, e não
// meses depois procurando por que uma regra de CSS "não pega".
func TestArquivosDoProjetoNaoComecamComMarcaDeUTF8(t *testing.T) {
	root := repoRoot(t)

	var offenders []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			// Diretório de ferramenta (.cursor, .vscode) não é código nosso; o
			// próprio .git já foi descartado acima.
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !bomSensitiveExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		head, err := readHead(path, len(utf8BOM))
		if err != nil {
			return err
		}
		if bytes.Equal(head, utf8BOM) {
			offenders = append(offenders, slashRel(root, path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("estes arquivos começam com a marca de UTF-8 (EF BB BF), que estraga o primeiro byte útil deles"+
			" — em CSS ela invalida o primeiro seletor e em Go ela quebra a compilação;"+
			" grave-os como UTF-8 sem marca:\n%s", strings.Join(offenders, "\n"))
	}
}

// readHead lê os primeiros n bytes do arquivo. Só o começo, porque é onde a marca
// mora e porque a varredura passa por todo o repositório.
func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, n)
	read, err := f.Read(head)
	if err != nil && read == 0 {
		// Arquivo vazio não tem marca nenhuma, e não é falha a relatar.
		return nil, nil
	}
	return head[:read], nil
}

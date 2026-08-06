package acpinstall

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/acp"
)

func TestDescribeMostraALinhaQueSeraExecutada(t *testing.T) {
	// O diálogo de confirmação mostra o que vai ser executado antes de qualquer
	// byte ser baixado (D3), e o que ele mostra é a linha de verdade — não uma
	// descrição do que o app faria.
	runtime := runtimeComNode()
	npm := NewNPM(runtime)

	linha := npm.Describe(filepath.Join("C:", "dados", "agents", codexID, codexVersao), codexPacote+"@"+codexVersao)

	if !strings.Contains(linha, "install") || !strings.Contains(linha, "--prefix") {
		t.Errorf("linha = %q, queria o `npm install --prefix`", linha)
	}
	if !strings.Contains(linha, codexPacote+"@"+codexVersao) {
		t.Errorf("linha = %q, queria o pacote com a versão fixada", linha)
	}
	// O npm é rodado pelo `node` com o `npm-cli.js`: no Windows o `npm` do PATH é
	// um arquivo de lote, e o app não cria processo a partir de arquivo de lote
	// (AEP-0084 D15) — cancelar não encerraria quem escreve no disco.
	if !strings.Contains(linha, "npm-cli.js") {
		t.Errorf("linha = %q, queria o npm-cli.js rodado pelo node", linha)
	}
	if strings.Contains(strings.ToLower(linha), ".cmd") {
		t.Errorf("linha = %q, não deveria passar por arquivo de lote", linha)
	}
	// Caminho com espaço sai entre aspas: é o caso comum no Windows, e a linha
	// existe para ser lida e copiada.
	if !strings.Contains(linha, `"`) {
		t.Errorf("linha = %q, queria o caminho com espaço entre aspas", linha)
	}
}

func TestDescribeVazioSemRuntime(t *testing.T) {
	if linha := NewNPM(runtimeSemNode()).Describe("dir", "pacote"); linha != "" {
		t.Errorf("linha = %q, queria vazio sem Node", linha)
	}
}

func TestInstallSemRuntimeDizQueFaltaONpm(t *testing.T) {
	err := NewNPM(runtimeSemNode()).Install(context.Background(), t.TempDir(), "pacote@1.0.0")
	if !errors.Is(err, ErrNoNPM) {
		t.Errorf("erro = %v, queria a falta do npm", err)
	}
}

func TestLimitedWriterGuardaOInicioEEngoleOResto(t *testing.T) {
	// O npm escreve muito quando falha, e o processo não pode travar por causa de
	// um cano que ninguém mais lê: o excesso é contado como escrito.
	var buf bytes.Buffer
	w := &limitedWriter{buf: &buf, limit: 8}

	n, err := w.Write([]byte("doze bytes a mais"))
	if err != nil {
		t.Fatalf("erro ao escrever: %v", err)
	}
	if n != len("doze bytes a mais") {
		t.Errorf("escreveu %d, queria dizer que aceitou tudo", n)
	}
	if buf.String() != "doze byt" {
		t.Errorf("guardou %q, queria os primeiros 8 bytes", buf.String())
	}
}

// TestInstallComNpmDeVerdade fala com o registro npm e por isso não roda no CI.
//
// Ela existe para provar, quando alguém quiser conferir, que o caminho real
// funciona: o `node` + `npm-cli.js` desta máquina instala um pacote de verdade em
// prefixo do app e o ponto de entrada sai do `bin` do manifesto instalado. O que
// ela não faz é o handshake: o pacote usado não é um agente ACP, e subir um
// processo de terceiro num teste é outra coisa.
//
// Para rodar: `ASSISTENTE_ACP_NPM_E2E=1 go test ./internal/acpinstall/ -run
// NpmDeVerdade -v`.
func TestInstallComNpmDeVerdade(t *testing.T) {
	if os.Getenv("ASSISTENTE_ACP_NPM_E2E") != "1" {
		t.Skip("precisa de rede e do npm: defina ASSISTENTE_ACP_NPM_E2E=1 para rodar")
	}
	if testing.Short() {
		t.Skip("teste de rede")
	}
	runtime := acp.FindNodeRuntime()
	if !runtime.Found {
		t.Skipf("sem Node nesta máquina; procurei em %v", runtime.Searched)
	}

	prefixo := t.TempDir()
	const pacote = "json"
	const versao = "11.0.0"
	if err := NewNPM(runtime).Install(context.Background(), prefixo, pacote+"@"+versao); err != nil {
		t.Fatalf("o npm de verdade não instalou %s@%s: %v", pacote, versao, err)
	}

	pkg, err := acp.NPMEntryPoint(prefixo, pacote)
	if err != nil {
		t.Fatalf("não resolveu o ponto de entrada do pacote instalado de verdade: %v", err)
	}
	if pkg.Version != versao {
		t.Errorf("versão instalada = %q, queria %q: a versão é a que o registro fixa", pkg.Version, versao)
	}
	if _, err := os.Stat(pkg.EntryPoint); err != nil {
		t.Errorf("o ponto de entrada resolvido não está no disco: %v", err)
	}
}

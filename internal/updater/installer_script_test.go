package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// O instalador NSIS é o outro lado da atualização automática: o updater fecha o
// app e o script precisa conseguir substituir o executável. Como não há como
// rodar makensis no CI, estes testes leem o script e cobram as invariantes que
// já quebraram a atualização em produção.

const caminhoScriptInstalador = "../../build/windows/installer/project.nsi"

// linhasEfetivas devolve as linhas do script sem comentários e sem espaços nas
// bordas. É o que o compilador do NSIS enxerga — e a diferença importa: um
// `!define` escrito depois de um `#` na mesma linha simplesmente não existe.
func linhasEfetivas(t *testing.T) []string {
	t.Helper()

	conteudo, err := os.ReadFile(filepath.FromSlash(caminhoScriptInstalador))
	if err != nil {
		t.Fatalf("falha ao ler %s: %v", caminhoScriptInstalador, err)
	}

	var efetivas []string
	for _, linha := range strings.Split(string(conteudo), "\n") {
		if corte := strings.IndexAny(linha, "#;"); corte >= 0 {
			linha = linha[:corte]
		}
		efetivas = append(efetivas, strings.TrimSpace(linha))
	}
	return efetivas
}

func contemPrefixo(linhas []string, prefixo string) bool {
	for _, linha := range linhas {
		if strings.HasPrefix(linha, prefixo) {
			return true
		}
	}
	return false
}

// defineAtivo confere o nome inteiro: `!define MUI_FINISHPAGE_RUN_TEXT` não
// pode passar por `!define MUI_FINISHPAGE_RUN`.
func defineAtivo(linhas []string, nome string) bool {
	for _, linha := range linhas {
		campos := strings.Fields(linha)
		if len(campos) >= 2 && campos[0] == "!define" && campos[1] == nome {
			return true
		}
	}
	return false
}

func contemTrecho(linhas []string, trecho string) bool {
	for _, linha := range linhas {
		if strings.Contains(linha, trecho) {
			return true
		}
	}
	return false
}

func TestInstaladorNaoUsaRenameParaDetectarAppAberto(t *testing.T) {
	linhas := linhasEfetivas(t)

	// O Windows permite renomear um executável em execução, então o rename
	// nunca provou que o app fechou. Pior: ele falha quando o destino já
	// existe, e o .old deixado por uma tentativa anterior travava toda
	// atualização seguinte com "o aplicativo não fechou a tempo".
	if contemTrecho(linhas, "Rename \"$INSTDIR\\${PRODUCT_EXECUTABLE}\"") {
		t.Error("o script voltou a usar Rename do executável como sonda de app aberto")
	}

	if !contemTrecho(linhas, "FileOpen $R1 \"$INSTDIR\\${PRODUCT_EXECUTABLE}\" a") {
		t.Error("a espera pelo fechamento do app deve sondar abrindo o executável para escrita")
	}

	if !contemPrefixo(linhas, "Delete \"$INSTDIR\\${PRODUCT_EXECUTABLE}.old\"") {
		t.Error("o script deve apagar um .old remanescente antes de esperar o app fechar")
	}
}

func TestInstaladorReabreAppSemElevacao(t *testing.T) {
	linhas := linhasEfetivas(t)

	// O instalador roda elevado; abrir o app direto daqui o deixaria como
	// administrador, junto de tudo que ele lança depois.
	if !contemTrecho(linhas, "explorer.exe\" \"$INSTDIR\\${PRODUCT_EXECUTABLE}\"") {
		t.Error("o app deve ser reaberto via explorer.exe para não herdar a elevação do instalador")
	}

	for _, linha := range linhas {
		if strings.HasPrefix(linha, "Exec ") && !strings.Contains(linha, "explorer.exe") {
			t.Errorf("Exec direto do app herdaria a elevação do instalador: %q", linha)
		}
	}
}

func TestInstaladorDefinePaginaFinal(t *testing.T) {
	linhas := linhasEfetivas(t)

	// Estes dois já estiveram no arquivo, mas grudados no fim de um comentário
	// da linha anterior — ou seja, comentados junto e sem efeito nenhum.
	for _, define := range []string{
		"MUI_FINISHPAGE_RUN",
		"MUI_FINISHPAGE_RUN_FUNCTION",
		"MUI_FINISHPAGE_RUN_TEXT",
		"MUI_ABORTWARNING",
	} {
		if !defineAtivo(linhas, define) {
			t.Errorf("!define %s não está ativo (linha própria, fora de comentário)", define)
		}
	}
}

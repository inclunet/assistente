package filesystem

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"assistente/internal/questionnaire"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

// QuestionnaireRequester abstrai o gerenciador de questionários para injeção de dependência.
type QuestionnaireRequester interface {
	RequestQuestionnaire(ctx context.Context, payload questionnaire.RequestPayload) (questionnaire.Response, error)
}

// editPolicy descreve o comportamento de confirmação de tools que modificam arquivos.
type editPolicy int

const (
	policyDirect          editPolicy = iota // grava direto, sem confirmação
	policyConfirmWithDiff                   // mostra Antes/Depois e pede confirmação antes de gravar
)

// resolveEditPolicy determina o comportamento de confirmação com base no contexto de invocação.
// Se a tool foi invocada de uma aba de editor com o arquivo ativo, exige confirmação (AEP-0032:
// alterações no documento aberto nunca são auto-aplicadas).
func resolveEditPolicy(ctx context.Context, fullPath string) editPolicy {
	inv, ok := invocationctx.Get(ctx)
	if !ok {
		return policyDirect
	}
	if inv.TabType == "editor" && inv.ActiveFilePath != "" && sameFilePath(inv.ActiveFilePath, fullPath) {
		return policyConfirmWithDiff
	}
	return policyDirect
}

// sameFilePath compara caminhos com a mesma normalização de IsOpenEditorFile
// (filepath.Clean e case-insensitive no Windows), para que capitalização ou
// separadores diferentes não burlem a política de confirmação.
func sameFilePath(a, b string) bool {
	ca := filepath.Clean(a)
	cb := filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}

// confirmBeforeAfter exibe um questionário com conteúdo Antes/Depois e aguarda confirmação do usuário.
// Retorna (true, zero) se aprovado, ou (false, errorResult) se rejeitado ou em caso de erro.
// Sem gerenciador de questionários (contextos não-UI: CLI/testes), aprova direto.
func confirmBeforeAfter(ctx context.Context, questMgr QuestionnaireRequester, title, displayPath, before, after string) (bool, tools.ToolResult) {
	if questMgr == nil {
		return true, tools.ToolResult{}
	}

	resp, err := questMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       title,
		Description: fmt.Sprintf("Revise a alteração em **%s** e clique em Aplicar para confirmar.", displayPath),
		Questions: []questionnaire.Question{
			{ID: "before", Type: "readonly_code", Prompt: "Antes", Content: before},
			{ID: "after", Type: "readonly_code", Prompt: "Depois", Content: after},
		},
		AllowCancel: true,
		SubmitLabel: "Aplicar",
		CancelLabel: "Rejeitar",
	})
	if err != nil {
		return false, tools.ToolResult{Content: fmt.Sprintf("Erro ao solicitar confirmação: %v", err), IsError: true}
	}
	if resp.Cancelled {
		return false, tools.ToolResult{Content: "Alteração rejeitada pelo usuário", IsError: true}
	}
	return true, tools.ToolResult{}
}

// Limites de truncamento dos previews de confirmação (write_file substitui o arquivo inteiro,
// então o conteúdo exibido precisa ser limitado).
const (
	previewMaxLines = 200
	previewMaxBytes = 8 * 1024
)

// previewTruncationMarker sinaliza que o preview foi cortado.
const previewTruncationMarker = "\n… (conteúdo truncado)"

// readFilePrefixForPreview lê apenas o prefixo necessário para montar o preview
// truncado (previewMaxBytes + 1 byte para detectar o corte), evitando carregar
// arquivos grandes inteiros em memória.
func readFilePrefixForPreview(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, previewMaxBytes+1)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	return string(buf[:n]), nil
}

// truncateForPreview limita o conteúdo a previewMaxLines linhas e previewMaxBytes bytes,
// anexando um marcador quando houver corte.
func truncateForPreview(content string) string {
	truncated := false

	if lines := strings.SplitAfterN(content, "\n", previewMaxLines+1); len(lines) > previewMaxLines {
		content = strings.Join(lines[:previewMaxLines], "")
		truncated = true
	}

	if len(content) > previewMaxBytes {
		content = content[:previewMaxBytes]
		// Evita cortar uma rune UTF-8 ao meio (no máximo 3 bytes de continuação).
		for i := 0; i < 3 && len(content) > 0; i++ {
			if r, size := utf8.DecodeLastRuneInString(content); r != utf8.RuneError || size > 1 {
				break
			}
			content = content[:len(content)-1]
		}
		truncated = true
	}

	if truncated {
		content += previewTruncationMarker
	}
	return content
}

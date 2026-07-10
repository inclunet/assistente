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

// sanitizeForDialogText remove CR/LF de valores interpolados em textos de diálogo.
// displayPath vem de input do modelo: quebras de linha permitiriam "injetar"
// linhas extras no diálogo de confirmação e confundir a revisão.
func sanitizeForDialogText(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// confirmDescriptionForPath monta a descrição padrão do diálogo de confirmação.
// Texto simples, sem Markdown: o QuestionnaireDialog renderiza description
// literalmente, então marcadores como ** apareceriam para o usuário.
func confirmDescriptionForPath(displayPath string) string {
	return fmt.Sprintf("Revise a alteração em %q e clique em Aplicar para confirmar.", sanitizeForDialogText(displayPath))
}

// confirmBeforeAfter exibe um questionário com conteúdo Antes/Depois e aguarda confirmação do usuário.
// Retorna (true, zero) se aprovado, ou (false, errorResult) se rejeitado ou em caso de erro.
// Sem gerenciador de questionários (contextos não-UI: CLI/testes), aprova direto.
func confirmBeforeAfter(ctx context.Context, questMgr QuestionnaireRequester, title, displayPath, before, after string) (bool, tools.ToolResult) {
	return confirmEditWithDiff(ctx, questMgr, title, confirmDescriptionForPath(displayPath), before, after)
}

// rejectReasonAnswerID é a chave usada no payload e nas answers para o motivo da rejeição.
const rejectReasonAnswerID = "reject_reason"

// rejectReasonMaxLen limita o tamanho do motivo repassado ao modelo, em runes,
// antes de anexar a elipse de truncamento (o texto final pode ter até
// rejectReasonMaxLen+1 runes).
const rejectReasonMaxLen = 2000

// extractRejectReason extrai e normaliza o motivo de rejeição informado pelo usuário.
// Retorna string vazia quando ausente ou em branco. Quebras de linha são preservadas
// (o motivo é conteúdo para o modelo, não texto de diálogo), mas o tamanho é limitado.
func extractRejectReason(answers map[string]any) string {
	raw, _ := answers[rejectReasonAnswerID].(string)
	reason := strings.TrimSpace(raw)
	if runes := []rune(reason); len(runes) > rejectReasonMaxLen {
		reason = string(runes[:rejectReasonMaxLen]) + "…"
	}
	return reason
}

// confirmEditWithDiff exibe um questionário Antes/Depois (Aplicar/Rejeitar) e aguarda
// a confirmação do usuário. Compartilhado por edit_file, write_file e text_edit.
// Retorna (true, zero) se aprovado, ou (false, errorResult) se rejeitado ou em erro.
// Sem gerenciador de questionários (contextos não-UI: CLI/testes), aprova direto.
func confirmEditWithDiff(ctx context.Context, questMgr QuestionnaireRequester, title, description, before, after string) (bool, tools.ToolResult) {
	if questMgr == nil {
		return true, tools.ToolResult{}
	}

	resp, err := questMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       title,
		Description: description,
		Questions: []questionnaire.Question{
			{ID: "before", Type: "readonly_code", Prompt: "Antes", Content: before},
			{ID: "after", Type: "readonly_code", Prompt: "Depois", Content: after},
		},
		AllowCancel: true,
		SubmitLabel: "Aplicar",
		CancelLabel: "Rejeitar",
		RejectReason: &questionnaire.RejectReasonConfig{
			ID:          rejectReasonAnswerID,
			Label:       "Motivo da rejeição (opcional)",
			Placeholder: "Explique o que deveria ser diferente para o assistente propor nova versão",
		},
	})
	if err != nil {
		return false, tools.ToolResult{Content: fmt.Sprintf("Erro ao solicitar confirmação: %v", err), IsError: true}
	}
	if resp.Cancelled {
		if reason := extractRejectReason(resp.Answers); reason != "" {
			return false, tools.ToolResult{Content: fmt.Sprintf("Alteração rejeitada pelo usuário. Motivo informado: %s", reason), IsError: true}
		}
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
	defer func() { _ = f.Close() }()

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

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
// AEP-0032: alterações no documento aberto nunca são auto-aplicadas. A proteção vale
// tanto para a aba de editor invocadora (arquivo ativo) quanto para escritas vindas
// de abas PARALELAS (chat ou outra superfície) em arquivo aberto em qualquer aba de
// editor — sem isso, a escrita direta dispararia o fluxo de "mudança externa" no
// editor como se outro aplicativo tivesse gravado o arquivo.
func resolveEditPolicy(ctx context.Context, fullPath string) editPolicy {
	if inv, ok := invocationctx.Get(ctx); ok {
		if inv.TabType == "editor" && inv.ActiveFilePath != "" && sameFilePath(inv.ActiveFilePath, fullPath) {
			return policyConfirmWithDiff
		}
	}
	// Arquivo aberto em alguma aba de editor (paths injetados via
	// tools.WithOpenEditorPaths): mesma confirmação com diff da aba invocadora.
	if tools.IsOpenEditorFile(ctx, fullPath) {
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

// editConfirmationTextKey é o assunto deste diálogo nas chaves de tradução
// (AEP-0085 D7). O texto pronto em pt-BR continua viajando como fallback: é ele
// que aparece se a chave faltar num locale, e é ele que serve às superfícies que
// não traduzem nada.
func editConfirmationTextKey(field string) string {
	return "app.questionnaire.editConfirmation." + field
}

// editConfirmTitle é o título de quem altera parte de um arquivo.
func editConfirmTitle() questionnaire.Text {
	return questionnaire.Keyed(editConfirmationTextKey("titleEdit"), "Confirmar edição")
}

// overwriteConfirmTitle é o título de quem substitui o arquivo inteiro. Vale
// dizer isso no título: quem só ouve o começo do diálogo decide sabendo que o
// "Depois" é o arquivo todo, e não um trecho dele.
func overwriteConfirmTitle() questionnaire.Text {
	return questionnaire.Keyed(editConfirmationTextKey("titleOverwrite"), "Confirmar sobrescrita")
}

// confirmDescriptionForPath monta a descrição padrão do diálogo de confirmação.
// Texto simples, sem Markdown: o QuestionnaireDialog renderiza description
// literalmente, então marcadores como ** apareceriam para o usuário.
//
// O caminho vai como parâmetro da tradução, e não na chave: não existe tradução
// para o nome de um arquivo (AEP-0085 D6). O fallback já vai com ele no lugar.
func confirmDescriptionForPath(displayPath string) questionnaire.Text {
	path := sanitizeForDialogText(displayPath)
	return questionnaire.KeyedWith(
		editConfirmationTextKey("description"),
		map[string]any{"path": path},
		fmt.Sprintf("Revise a alteração em %q e clique em Aplicar para confirmar.", path),
	)
}

// editConfirmDescription monta a descrição da confirmação do editor, que o
// modelo pode escrever no lugar da padrão e complementar com uma justificativa.
//
// Descrição e justificativa vindas do modelo são conteúdo, nunca chave (AEP-0085
// D6): a descrição substitui a frase inteira, então vai como texto puro; a
// justificativa se soma à frase padrão, então entra como parâmetro dela. As duas
// formas da frase padrão dividiriam um campo só, e por isso cada uma tem a sua
// chave — com uma só, quem traduz deixaria a justificativa de fora.
func editConfirmDescription(fromModel, displayPath, notes string) questionnaire.Text {
	if fromModel != "" {
		if notes != "" {
			return questionnaire.Plain(fromModel + "\n\n" + notes)
		}
		return questionnaire.Plain(fromModel)
	}

	padrao := confirmDescriptionForPath(displayPath)
	if notes == "" {
		return padrao
	}
	return questionnaire.KeyedWith(
		editConfirmationTextKey("descriptionNotes"),
		map[string]any{"path": padrao.Params["path"], "notes": notes},
		padrao.Fallback+"\n\n"+notes,
	)
}

// confirmBeforeAfter exibe um questionário com conteúdo Antes/Depois e aguarda confirmação do usuário.
// Retorna (true, zero) se aprovado, ou (false, errorResult) se rejeitado ou em caso de erro.
// Sem gerenciador de questionários (contextos não-UI: CLI/testes), aprova direto.
func confirmBeforeAfter(ctx context.Context, questMgr QuestionnaireRequester, title questionnaire.Text, displayPath, before, after string) (bool, tools.ToolResult) {
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

const (
	editDecisionApply  = "apply"
	editDecisionReject = "reject"
)

// rejectedEditResult monta o ToolResult de rejeição, com motivo opcional.
func rejectedEditResult(answers map[string]any) tools.ToolResult {
	if reason := extractRejectReason(answers); reason != "" {
		return tools.ToolResult{
			Content: fmt.Sprintf("Alteração rejeitada pelo usuário. Motivo informado: %s", reason),
			IsError: true,
		}
	}
	return tools.ToolResult{Content: "Alteração rejeitada pelo usuário", IsError: true}
}

// confirmEditWithDiff exibe uma decisão Antes/Depois (Aplicar/Rejeitar) e aguarda
// a confirmação do usuário (AEP-0091 kind=decision). Compartilhado por edit_file,
// write_file e text_edit.
// Retorna (true, zero) se aprovado, ou (false, errorResult) se rejeitado ou em erro.
// Sem gerenciador de questionários (contextos não-UI: CLI/testes), aprova direto.
// O conteúdo dos blocos é o texto do arquivo: vai cru, sem chave de tradução,
// porque não existe tradução para o conteúdo de um arquivo (AEP-0085 D6). O
// motivo da rejeição tem rótulo e placeholder do app, e esses se traduzem: é por
// eles que a pessoa entende que pode dizer ao assistente o que faltou.
func confirmEditWithDiff(ctx context.Context, questMgr QuestionnaireRequester, title, description questionnaire.Text, before, after string) (bool, tools.ToolResult) {
	if questMgr == nil {
		return true, tools.ToolResult{}
	}

	resp, err := questMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Kind:        questionnaire.KindDecision,
		Title:       title,
		Description: description,
		// readonly_code: o DecisionQuestionnaireHost renderiza Antes/Depois no body.
		Questions: []questionnaire.Question{
			{
				ID:      "before",
				Type:    "readonly_code",
				Prompt:  questionnaire.Keyed(editConfirmationTextKey("beforePrompt"), "Antes"),
				Content: before,
			},
			// AutoFocus no "Depois": o usuário quer ouvir primeiro como o texto
			// vai ficar (o host de decisão foca o body quando há conteúdo).
			{
				ID:        "after",
				Type:      "readonly_code",
				Prompt:    questionnaire.Keyed(editConfirmationTextKey("afterPrompt"), "Depois"),
				Content:   after,
				AutoFocus: true,
			},
		},
		AllowCancel: true,
		Actions: []questionnaire.DecisionAction{
			{
				ID:      editDecisionApply,
				Label:   questionnaire.Keyed(editConfirmationTextKey("submit"), "Aplicar"),
				Variant: "primary",
				Primary: true,
			},
			{
				ID:      editDecisionReject,
				Label:   questionnaire.Keyed(editConfirmationTextKey("cancel"), "Rejeitar"),
				Variant: "outline",
			},
		},
		RejectReason: &questionnaire.RejectReasonConfig{
			ID:    rejectReasonAnswerID,
			Label: questionnaire.Keyed(editConfirmationTextKey("rejectReasonLabel"), "Motivo da rejeição (opcional)"),
			Placeholder: questionnaire.Keyed(
				editConfirmationTextKey("rejectReasonPlaceholder"),
				"Explique o que deveria ser diferente para o assistente propor nova versão",
			),
			MaxLen: rejectReasonMaxLen,
		},
	})
	if err != nil {
		return false, tools.ToolResult{Content: fmt.Sprintf("Erro ao solicitar confirmação: %v", err), IsError: true}
	}
	// ESC / Fechar: rejeição com motivo opcional (mesmo contrato de reject).
	if resp.Cancelled {
		return false, rejectedEditResult(resp.Answers)
	}
	id, ok := questionnaire.DecisionActionID(resp)
	if !ok {
		return false, tools.ToolResult{Content: "Resposta inválida para confirmação de edição", IsError: true}
	}
	switch id {
	case editDecisionApply:
		return true, tools.ToolResult{}
	case editDecisionReject:
		return false, rejectedEditResult(resp.Answers)
	default:
		return false, tools.ToolResult{
			Content: fmt.Sprintf("Ação de decisão desconhecida: %q", id),
			IsError: true,
		}
	}
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

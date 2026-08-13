package portability

import (
	"errors"
	"fmt"
	"strconv"
)

// LocalizedMessage é como aviso, erro e motivo de conflito saem da
// importação: um código que diz qual é o caso, os parâmetros que o
// completam e o texto em português como reserva.
//
// O backend não sabe em que idioma a tela está — e o arquivo importado
// tampouco. Quem traduz é a UI, a partir do código; o texto viaja junto para
// que um app antigo, ou um código que a tradução ainda não conhece, continue
// mostrando alguma coisa em vez de uma lista vazia.
//
// Os parâmetros vêm do arquivo importado (id de provider, caminho de binário,
// pattern de credencial): são dados de quem exportou, e a UI os trata como
// texto puro.
type LocalizedMessage struct {
	Code    string            `json:"code"`
	Params  map[string]string `json:"params,omitempty"`
	Message string            `json:"message"`
}

// String devolve o texto de reserva, que é o que a CLI e os logs mostram.
func (m LocalizedMessage) String() string {
	return m.Message
}

// Códigos de aviso e erro da importação. A UI mapeia cada um para uma chave de
// tradução; mudar um valor aqui obriga a atualizar os três locales.
const (
	CodeUnsupportedResources    = "import.unsupportedResources"
	CodeEmptyConversations      = "import.emptyConversations"
	CodeConversationMissingID   = "conversation.missingId"
	CodeMessageMissingID        = "message.missingId"
	CodeMessageDuplicatedID     = "message.duplicatedId"
	CodeMessageInvalidParentID  = "message.invalidParentId"
	CodeMessageInvalidParentIdx = "message.invalidParentIndex"
	CodeMessageInvalidTurnID    = "message.invalidTurnId"
	CodeMessageInvalidTurnIdx   = "message.invalidTurnIndex"

	CodeProviderMissingID                 = "provider.missingId"
	CodeProviderMissingName               = "provider.missingName"
	CodeProviderMissingType               = "provider.missingType"
	CodeProviderMissingBaseURL            = "provider.missingBaseUrl"
	CodeProviderACPMissingCommand         = "provider.acpMissingCommand"
	CodeProviderACPOutsideACPFormat       = "provider.acpOutsideAcpFormat"
	CodeProviderACPCredentialEnvNoName    = "provider.acpCredentialEnvWithoutName"
	CodeProviderACPCredentialEnvBadName   = "provider.acpCredentialEnvInvalidName"
	CodeProviderACPCredentialEnvNoPattern = "provider.acpCredentialEnvWithoutPattern"

	CodeACPCommandNotFound   = "acp.commandNotFound"
	CodeACPCredentialMissing = "acp.credentialMissing"

	CodeMCPServerMissingSlug         = "mcpServer.missingSlug"
	CodeMCPServerStdioMissingCommand = "mcpServer.stdioMissingCommand"
	CodeMCPServerMissingURL          = "mcpServer.missingUrl"
	CodeMCPServerInvalidURL          = "mcpServer.invalidUrl"
	CodeMCPServerInvalidTransport    = "mcpServer.invalidTransport"

	CodeTaskListMissingID                = "taskList.missingId"
	CodeTaskListWorkflowMissingID        = "taskList.workflowMissingId"
	CodeTaskListWorkflowWithoutStatuses  = "taskList.workflowWithoutStatuses"
	CodeTaskListWorkflowInvalidStatus    = "taskList.workflowInvalidStatus"
	CodeTaskListWorkflowDuplicatedStatus = "taskList.workflowDuplicatedStatus"
	CodeTaskListWorkflowInitialUnknown   = "taskList.workflowInitialStatusUnknown"
	CodeTaskListWorkflowFromUnknown      = "taskList.workflowFromStatusUnknown"
	CodeTaskListWorkflowToUnknown        = "taskList.workflowToStatusUnknown"
	CodeTaskMissingID                    = "task.missingId"
	CodeTaskUnknownStatus                = "task.unknownStatus"
	CodeTaskNoteMissingID                = "taskNote.missingId"

	CodeMemoryRecordMissingID = "memoryRecord.missingId"

	CodeCredentialVaultUnavailableImport = "credential.vaultUnavailableForImport"
	CodeCredentialVaultUnavailableCheck  = "credential.vaultUnavailableForAnalysis"
	CodeCredentialPasswordRequired       = "credential.passwordRequiredForAnalysis"
	CodeCredentialAnalysisFailed         = "credential.analysisFailed"
	CodeCredentialManagedNotImportable   = "credential.managedNotImportable"
	CodeCredentialStrategyUnsupported    = "credential.strategyUnsupported"

	CodeConflictMCPServerSlug     = "conflict.mcpServerSlug"
	CodeConflictCredentialPattern = "conflict.credentialPattern"
)

// newMessage monta a mensagem com o texto de reserva já formatado.
func newMessage(code string, params map[string]string, format string, args ...any) LocalizedMessage {
	return LocalizedMessage{
		Code:    code,
		Params:  params,
		Message: fmt.Sprintf(format, args...),
	}
}

// params é açúcar para escrever o mapa de parâmetros na chamada.
func params(pairs ...string) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out[pairs[i]] = pairs[i+1]
	}
	return out
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

// CodedError é o erro que sabe se traduzir: carrega o mesmo par código +
// parâmetros da LocalizedMessage e continua sendo um `error` comum, com o
// texto em português no `Error()`.
type CodedError struct {
	Code   string
	Params map[string]string
	text   string
}

func (e *CodedError) Error() string {
	if e == nil {
		return ""
	}
	return e.text
}

// codedErrorf cria o erro traduzível. O formato é o texto de reserva.
func codedErrorf(code string, msgParams map[string]string, format string, args ...any) *CodedError {
	return &CodedError{
		Code:   code,
		Params: msgParams,
		text:   fmt.Sprintf(format, args...),
	}
}

// messageFromError traduz o erro para a lista da UI. Erro sem código — falha de
// banco, JSON inválido, o que o pacote não previu — entra sem código e a UI
// mostra o texto como veio.
func messageFromError(err error) LocalizedMessage {
	if err == nil {
		return LocalizedMessage{}
	}
	var coded *CodedError
	if errors.As(err, &coded) {
		return LocalizedMessage{Code: coded.Code, Params: coded.Params, Message: err.Error()}
	}
	return LocalizedMessage{Message: err.Error()}
}

// withMessageContext acrescenta a mensagem e a conversa ao erro de referência
// sem perder o código: a tradução precisa dos dois lados para dizer onde o
// arquivo está quebrado.
func withMessageContext(err error, index int, conversationTitle string) error {
	if err == nil {
		return nil
	}
	var coded *CodedError
	if !errors.As(err, &coded) {
		return fmt.Errorf("erro ao importar mensagem %d da conversa '%s': %w", index, conversationTitle, err)
	}
	merged := make(map[string]string, len(coded.Params)+2)
	for key, value := range coded.Params {
		merged[key] = value
	}
	merged["index"] = itoa(index)
	merged["conversation"] = conversationTitle
	return codedErrorf(
		coded.Code,
		merged,
		"erro ao importar mensagem %d da conversa '%s': %s",
		index, conversationTitle, coded.Error(),
	)
}

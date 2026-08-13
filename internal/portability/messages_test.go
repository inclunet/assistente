package portability

import (
	"errors"
	"fmt"
	"testing"
)

// messageCodes lista os códigos na ordem em que saíram, que é o que os testes
// comparam: o texto em português é reserva, e não contrato.
func messageCodes(messages []LocalizedMessage) []string {
	codes := make([]string, 0, len(messages))
	for _, message := range messages {
		codes = append(codes, message.Code)
	}
	return codes
}

func findMessageByCode(t *testing.T, messages []LocalizedMessage, code string) LocalizedMessage {
	t.Helper()
	for _, message := range messages {
		if message.Code == code {
			return message
		}
	}
	t.Fatalf("nenhuma mensagem com código %q em %v", code, messageCodes(messages))
	return LocalizedMessage{}
}

func requireParam(t *testing.T, message LocalizedMessage, name, want string) {
	t.Helper()
	if got := message.Params[name]; got != want {
		t.Errorf("parâmetro %s = %q, esperado %q (mensagem %q)", name, got, want, message.Code)
	}
}

func TestMensagemGuardaCodigoParametrosETextoDeReserva(t *testing.T) {
	message := newMessage(
		CodeACPCommandNotFound,
		params("providerId", "cursor", "command", "cursor-agent"),
		"Provider %q usa o agente %q.", "cursor", "cursor-agent",
	)

	if message.Code != CodeACPCommandNotFound {
		t.Errorf("código = %q", message.Code)
	}
	requireParam(t, message, "providerId", "cursor")
	requireParam(t, message, "command", "cursor-agent")
	if message.Message != `Provider "cursor" usa o agente "cursor-agent".` {
		t.Errorf("texto de reserva = %q", message.Message)
	}
	if message.String() != message.Message {
		t.Errorf("String() = %q, esperado o texto de reserva", message.String())
	}
}

// Par incompleto é engano de quem escreveu a mensagem. Melhor quebrar aqui do
// que entregar à UI um parâmetro faltando, que vira placeholder vazio na tela.
func TestParametroSemValorEntraEmPanico(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("esperado pânico com número ímpar de argumentos")
		}
	}()

	params("providerId", "cursor", "command")
}

// Erro que o pacote não previu — falha de banco, JSON quebrado — não pode
// sumir da lista só por não ter código: a UI mostra o texto como veio.
func TestErroSemCodigoViraMensagemComTextoOriginal(t *testing.T) {
	message := messageFromError(errors.New("erro ao criar conversa: disco cheio"))

	if message.Code != "" {
		t.Errorf("código = %q, esperado vazio", message.Code)
	}
	if message.Message != "erro ao criar conversa: disco cheio" {
		t.Errorf("texto = %q", message.Message)
	}
}

// O código sobrevive ao embrulho: quem trata o erro lá em cima acrescenta
// contexto ao texto sem apagar o que a UI usa para traduzir.
func TestCodigoSobreviveAoErroEmbrulhado(t *testing.T) {
	inner := codedErrorf(CodeProviderMissingBaseURL, params("providerId", "openai"), "provider %q sem baseUrl", "openai")
	wrapped := fmt.Errorf("erro ao importar provider: %w", inner)

	message := messageFromError(wrapped)
	if message.Code != CodeProviderMissingBaseURL {
		t.Errorf("código = %q", message.Code)
	}
	requireParam(t, message, "providerId", "openai")
	if message.Message != "erro ao importar provider: provider \"openai\" sem baseUrl" {
		t.Errorf("texto = %q", message.Message)
	}
}

func TestContextoDaMensagemMantemCodigoEAcrescentaParametros(t *testing.T) {
	inner := codedErrorf(CodeMessageInvalidParentID, params("reference", "msg-9"), "referência de pai inválida: id %q", "msg-9")

	message := messageFromError(withMessageContext(inner, 3, "Conversa"))
	if message.Code != CodeMessageInvalidParentID {
		t.Errorf("código = %q", message.Code)
	}
	requireParam(t, message, "reference", "msg-9")
	requireParam(t, message, "index", "3")
	requireParam(t, message, "conversation", "Conversa")
}

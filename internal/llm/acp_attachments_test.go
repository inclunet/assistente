package llm

import (
	"testing"

	"assistente/internal/acp"
)

// imagemPNG é uma imagem embutida como o pipeline a monta.
const imagemPNG = "data:image/png;base64,QUJD"

func mensagemComImagem(texto, url string) Message {
	return Message{Role: "user", Content: []ContentPart{
		{Type: "text", Text: texto},
		{Type: "image_url", ImageURL: &ImageURL{URL: url}},
	}}
}

func TestImagemVaiAoAgenteQueAceitaImagem(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "é um gráfico"}}}
	provider := providerComCapacidades(t, sessao, acp.Capabilities{PromptImage: true})
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{mensagemComImagem("o que tem nesta imagem?", imagemPNG)},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.erro != "" {
		t.Fatalf("turno falhou: %s", handler.erro)
	}
	turno := sessao.turnos()[0]
	if len(turno) != 2 {
		t.Fatalf("o agente recebeu %+v, quer o texto e a imagem", turno)
	}
	// A ordem é a da mensagem: a pergunta se refere à imagem que vem depois.
	if turno[0].Text != "o que tem nesta imagem?" {
		t.Errorf("primeiro bloco = %+v, quer o texto", turno[0])
	}
	if turno[1].ImageData != "QUJD" || turno[1].ImageMIME != "image/png" {
		t.Errorf("bloco de imagem = %+v, quer os dados e o tipo separados", turno[1])
	}
	if len(handler.avisos) != 0 {
		t.Errorf("avisos = %+v, o anexo foi enviado e não há o que avisar", handler.avisos)
	}
}

func TestAnexoRecusadoPeloAgenteNaoSomeEmSilencio(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "não recebi imagem"}}}
	// Agente sem promptCapabilities.image: o Cursor aceita imagem, outros não.
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{mensagemComImagem("o que tem nesta imagem?", imagemPNG)},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.erro != "" {
		t.Fatalf("turno falhou: %s", handler.erro)
	}
	// O turno segue com o texto…
	turno := sessao.turnos()[0]
	if len(turno) != 1 || turno[0].Text != "o que tem nesta imagem?" {
		t.Fatalf("o agente recebeu %+v, quer só o texto", turno)
	}
	// …e a pessoa fica sabendo do que ficou de fora, senão espera uma resposta
	// sobre uma imagem que o agente nunca viu.
	if len(handler.avisos) != 1 {
		t.Fatalf("avisos = %+v, quer um", handler.avisos)
	}
	if got := handler.avisos[0]; got.Kind != TurnNoticeAttachmentsNotSent || got.Count != 1 {
		t.Errorf("aviso = %+v, quer um anexo não enviado", got)
	}
}

func TestImagemQueNaoEstaEmbutidaNaoVaiEViraAviso(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "ok"}}}
	provider := providerComCapacidades(t, sessao, acp.Capabilities{PromptImage: true})
	handler := &espiao{}

	// O bloco do protocolo é base64 mais tipo MIME: um endereço remoto não tem
	// como ser embutido no pedido, mesmo com o agente aceitando imagem.
	provider.StreamChat(t.Context(),
		[]Message{mensagemComImagem("e nesta?", "https://exemplo.com/foto.png")},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if len(sessao.turnos()[0]) != 1 {
		t.Fatalf("o agente recebeu %+v, quer só o texto", sessao.turnos()[0])
	}
	if len(handler.avisos) != 1 || handler.avisos[0].Count != 1 {
		t.Errorf("avisos = %+v, quer um anexo não enviado", handler.avisos)
	}
}

func TestMensagemSoComAnexoRecusadoNaoViraTurnoVazio(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: []ContentPart{
		{Type: "image_url", ImageURL: &ImageURL{URL: imagemPNG}},
	}}}, ChatParams{ConversationID: "conversa-1"}, handler)

	// Sem texto e sem a imagem não sobra pedido nenhum: mandar assim seria
	// gastar um turno do agente com nada.
	if handler.erro == "" {
		t.Fatal("turno sem conteúdo nenhum precisa dizer o que aconteceu")
	}
	if len(sessao.turnos()) != 0 {
		t.Errorf("o agente recebeu %+v, não devia receber nada", sessao.turnos())
	}
}

func TestHandlerSemCanalDeAvisoAindaRecebeOTurno(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "ok"}}}
	provider := providerDeAgente(t, sessao)
	handler := &espiaoSurdo{}

	provider.StreamChat(t.Context(),
		[]Message{mensagemComImagem("e esta?", imagemPNG)},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if !handler.pronto || len(handler.chunks) == 0 {
		t.Errorf("o turno precisa seguir para quem não sabe receber aviso: %+v", handler)
	}
}

func TestSeparacaoDaImagemEmbutida(t *testing.T) {
	casos := []struct {
		nome string
		url  string
		data string
		mime string
		ok   bool
	}{
		{nome: "png em base64", url: imagemPNG, data: "QUJD", mime: "image/png", ok: true},
		{nome: "endereço remoto", url: "https://exemplo.com/foto.png"},
		{nome: "data url sem base64", url: "data:image/svg+xml,<svg/>"},
		{nome: "sem tipo", url: "data:;base64,QUJD"},
		{nome: "sem dados", url: "data:image/png;base64,"},
		{nome: "vazia", url: ""},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			data, mime, ok := inlineImage(ContentPart{Type: "image_url", ImageURL: &ImageURL{URL: caso.url}})
			if ok != caso.ok || data != caso.data || mime != caso.mime {
				t.Errorf("data=%q mime=%q ok=%t, quer %q, %q e %t", data, mime, ok, caso.data, caso.mime, caso.ok)
			}
		})
	}
	if _, _, ok := inlineImage(ContentPart{Type: "image_url"}); ok {
		t.Error("parte sem image_url não descreve imagem nenhuma")
	}
}

func TestPartesDestipadasViramOsMesmosBlocos(t *testing.T) {
	// A mensagem também chega destipada, quando veio de JSON.
	msg := Message{Role: "user", Content: []interface{}{
		map[string]interface{}{"type": "text", "text": "olha isto"},
		map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": imagemPNG}},
		map[string]interface{}{"type": "audio", "data": "..."},
	}}

	content, notSent := turnContent(msg, true)

	if notSent != 0 {
		t.Errorf("anexos não enviados = %d, quer 0", notSent)
	}
	if len(content) != 2 || content[0].Text != "olha isto" || content[1].ImageData != "QUJD" {
		t.Errorf("blocos = %+v, quer o texto e a imagem", content)
	}
}

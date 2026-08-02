package agent

import (
	"context"
	"testing"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/events"
	"assistente/internal/llm"
)

func novoHandlerDeAgente(t *testing.T, emitter *mockEmitter, fala *[]speechCall) *SimpleStreamHandler {
	t.Helper()
	cfg := ServiceConfig{Emitter: emitter, MsgRepo: &mockMsgRepo{}}
	if fala != nil {
		cfg.OnSpeechRequest = func(convID, msgID, role, text, origin, profileSlug string, interrupt bool) {
			*fala = append(*fala, speechCall{convID, msgID, role, text, origin, profileSlug, interrupt})
		}
	}
	svc := NewService(cfg)
	handler, err := svc.NewSimpleStreamHandler(context.Background(), "conversa-1", "turno-1", "perfil", nil)
	if err != nil {
		t.Fatalf("NewSimpleStreamHandler: %v", err)
	}
	return handler
}

func eventosPorNome(emitter *mockEmitter, nome string) []any {
	var encontrados []any
	for _, evento := range emitter.getEvents() {
		if evento.name == nome {
			encontrados = append(encontrados, evento.data)
		}
	}
	return encontrados
}

func TestFerramentaDoAgenteChegaMarcadaComoDele(t *testing.T) {
	emitter := &mockEmitter{}
	handler := novoHandlerDeAgente(t, emitter, nil)

	handler.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-1", Kind: "execute", Title: "npm test", Status: llm.AgentToolRunning})
	handler.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-1", Kind: "execute", Title: "npm test", Status: llm.AgentToolCompleted})

	inicios := eventosPorNome(emitter, "chat:tool_start")
	if len(inicios) != 1 {
		t.Fatalf("esperava 1 chat:tool_start, recebi %d", len(inicios))
	}
	inicio := inicios[0].(ports.ToolStartEvent)
	if inicio.Origin != OriginACPAgent {
		t.Errorf("origem=%q, esperava %q", inicio.Origin, OriginACPAgent)
	}
	if inicio.Name != "execute" {
		t.Errorf("nome=%q, esperava o kind do protocolo", inicio.Name)
	}
	if inicio.Summary != "npm test" {
		t.Errorf("resumo=%q, esperava o título do agente", inicio.Summary)
	}

	fins := eventosPorNome(emitter, "chat:tool_end")
	if len(fins) != 1 {
		t.Fatalf("esperava 1 chat:tool_end, recebi %d", len(fins))
	}
	fim := fins[0].(ports.ToolEndEvent)
	if fim.Origin != OriginACPAgent {
		t.Errorf("origem do fim=%q, esperava %q", fim.Origin, OriginACPAgent)
	}
	if fim.Status != "ok" {
		t.Errorf("status=%q, esperava ok — o mesmo valor dos outros emissores", fim.Status)
	}
	if fim.CallID != inicio.CallID {
		t.Errorf("callID do fim=%q, esperava o mesmo do início (%q)", fim.CallID, inicio.CallID)
	}
	if len(eventosPorNome(emitter, "chat:tool_failure")) != 0 {
		t.Error("ferramenta concluída não deve emitir falha")
	}
}

func TestFimDeFerramentaSemInicioAindaApareceNaTela(t *testing.T) {
	emitter := &mockEmitter{}
	handler := novoHandlerDeAgente(t, emitter, nil)

	handler.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-9", Kind: "read", Status: llm.AgentToolCompleted})

	if len(eventosPorNome(emitter, "chat:tool_start")) != 1 {
		t.Fatal("sem o início a UI não teria item para atualizar")
	}
	if len(eventosPorNome(emitter, "chat:tool_end")) != 1 {
		t.Fatal("esperava o fim da ferramenta")
	}
}

func TestFerramentaSemClassificacaoNaoFicaSemNome(t *testing.T) {
	emitter := &mockEmitter{}
	handler := novoHandlerDeAgente(t, emitter, nil)

	handler.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-2", Status: llm.AgentToolRunning})

	inicio := eventosPorNome(emitter, "chat:tool_start")[0].(ports.ToolStartEvent)
	if inicio.Name != llm.AgentToolKindOther {
		t.Errorf("nome=%q, esperava %q", inicio.Name, llm.AgentToolKindOther)
	}
}

func TestFerramentaSemIdentificadorNaoRoubaOItemDaOutra(t *testing.T) {
	emitter := &mockEmitter{}
	handler := novoHandlerDeAgente(t, emitter, nil)

	handler.OnAgentToolEvent(llm.AgentToolEvent{Kind: "read", Status: llm.AgentToolRunning})
	handler.OnAgentToolEvent(llm.AgentToolEvent{Kind: "search", Status: llm.AgentToolRunning})

	inicios := eventosPorNome(emitter, "chat:tool_start")
	if len(inicios) != 2 {
		t.Fatalf("esperava 2 chat:tool_start, recebi %d", len(inicios))
	}
	primeiro := inicios[0].(ports.ToolStartEvent).CallID
	segundo := inicios[1].(ports.ToolStartEvent).CallID
	if primeiro == segundo {
		t.Errorf("duas ferramentas dividiram o mesmo callID (%q)", primeiro)
	}
}

func TestFerramentaQueFalhaAvisaComFalhaSemPedirRetry(t *testing.T) {
	emitter := &mockEmitter{}
	handler := novoHandlerDeAgente(t, emitter, nil)

	handler.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-3", Kind: "edit", Status: llm.AgentToolRunning})
	handler.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-3", Kind: "edit", Status: llm.AgentToolFailed, Error: "permissão negada"})

	falhas := eventosPorNome(emitter, "chat:tool_failure")
	if len(falhas) != 1 {
		t.Fatalf("esperava 1 chat:tool_failure, recebi %d", len(falhas))
	}
	falha := falhas[0].(ports.ToolFailureEvent)
	if falha.Origin != OriginACPAgent {
		t.Errorf("origem=%q, esperava %q", falha.Origin, OriginACPAgent)
	}
	if falha.Retryable || falha.WillRetry {
		t.Error("o app não repete ferramenta do agente")
	}
	if falha.Message != "permissão negada" {
		t.Errorf("mensagem=%q, esperava o erro do agente", falha.Message)
	}
	fim := eventosPorNome(emitter, "chat:tool_end")[0].(ports.ToolEndEvent)
	if fim.Status != "error" {
		t.Errorf("status=%q, esperava error", fim.Status)
	}
}

func TestFerramentaCanceladaNaoViraAnuncioDeFalha(t *testing.T) {
	emitter := &mockEmitter{}
	handler := novoHandlerDeAgente(t, emitter, nil)

	handler.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-4", Kind: "execute", Status: llm.AgentToolRunning})
	handler.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-4", Kind: "execute", Status: llm.AgentToolCancelled})

	if len(eventosPorNome(emitter, "chat:tool_failure")) != 0 {
		t.Error("cancelar o turno não é falha da ferramenta")
	}
	fim := eventosPorNome(emitter, "chat:tool_end")[0].(ports.ToolEndEvent)
	if fim.Status != "error" {
		t.Errorf("status=%q, esperava error para a ferramenta interrompida", fim.Status)
	}
}

func TestSegmentoFechadoFalaOTextoSemEsperarOTurno(t *testing.T) {
	emitter := &mockEmitter{}
	var fala []speechCall
	handler := novoHandlerDeAgente(t, emitter, &fala)

	handler.OnChunk("primeira parte")
	handler.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-5", Kind: "read", Status: llm.AgentToolRunning})
	handler.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-5", Kind: "read", Status: llm.AgentToolCompleted})
	handler.OnSegmentDone()

	segmentos := eventosPorNome(emitter, "chat:segment_done")
	if len(segmentos) != 1 {
		t.Fatalf("esperava 1 chat:segment_done, recebi %d", len(segmentos))
	}
	segmento := segmentos[0].(ports.SegmentDoneEvent)
	if segmento.Content != "primeira parte" {
		t.Errorf("conteúdo=%q, esperava o bloco acumulado", segmento.Content)
	}
	if !segmento.HasMore {
		t.Error("segmento intermediário precisa de hasMore para a UI promovê-lo")
	}
	if len(segmento.ToolsInIteration) != 1 || segmento.ToolsInIteration[0].Origin != OriginACPAgent {
		t.Errorf("esperava a ferramenta do agente no resumo do segmento, recebi %+v", segmento.ToolsInIteration)
	}

	if len(fala) != 1 {
		t.Fatalf("esperava 1 pedido de fala, recebi %d", len(fala))
	}
	if fala[0].origin != "segment" {
		t.Errorf("origem da fala=%q, esperava segment", fala[0].origin)
	}
	if fala[0].interrupt {
		t.Error("segmento intermediário não interrompe a fala anterior")
	}
	if fala[0].text != "primeira parte" {
		t.Errorf("texto falado=%q, esperava o bloco do segmento", fala[0].text)
	}
}

func TestTextoJaPromovidoNaoVoltaNoStreamSeguinte(t *testing.T) {
	emitter := &mockEmitter{}
	handler := novoHandlerDeAgente(t, emitter, nil)

	handler.OnChunk("bloco um. ")
	handler.OnSegmentDone()
	handler.OnChunk("bloco dois.")

	// O primeiro chunk depois do corte cai no throttle de 50 ms.
	time.Sleep(120 * time.Millisecond)

	streams := eventosPorNome(emitter, "chat:stream")
	if len(streams) == 0 {
		t.Fatal("esperava ao menos um chat:stream")
	}
	ultimo := streams[len(streams)-1].(events.StreamEvent)
	if ultimo.Content != "bloco dois." {
		t.Errorf("stream=%q, esperava só o bloco novo — o anterior já virou segmento", ultimo.Content)
	}

	conteudo, _ := handler.Finalize()
	if conteudo != "bloco um. bloco dois." {
		t.Errorf("turno completo=%q, esperava os dois blocos", conteudo)
	}
}

func TestSegmentoSemTextoNemFerramentaNaoViraEvento(t *testing.T) {
	emitter := &mockEmitter{}
	var fala []speechCall
	handler := novoHandlerDeAgente(t, emitter, &fala)

	handler.OnSegmentDone()

	if len(eventosPorNome(emitter, "chat:segment_done")) != 0 {
		t.Error("segmento vazio não deve chegar à UI")
	}
	if len(fala) != 0 {
		t.Error("segmento vazio não deve pedir fala")
	}
}

func TestCadaSegmentoTemSuaIteracao(t *testing.T) {
	emitter := &mockEmitter{}
	handler := novoHandlerDeAgente(t, emitter, nil)

	handler.OnChunk("um")
	handler.OnSegmentDone()
	handler.OnChunk("dois")
	handler.OnSegmentDone()

	segmentos := eventosPorNome(emitter, "chat:segment_done")
	if len(segmentos) != 2 {
		t.Fatalf("esperava 2 segmentos, recebi %d", len(segmentos))
	}
	primeiro := segmentos[0].(ports.SegmentDoneEvent)
	segundo := segmentos[1].(ports.SegmentDoneEvent)
	if primeiro.Iteration != 0 || segundo.Iteration != 1 {
		t.Errorf("iterações=%d e %d, esperava 0 e 1", primeiro.Iteration, segundo.Iteration)
	}
}

func TestFerramentaContadaUmaVezSoNoSegmentoDela(t *testing.T) {
	emitter := &mockEmitter{}
	handler := novoHandlerDeAgente(t, emitter, nil)

	handler.OnChunk("texto")
	handler.OnAgentToolEvent(llm.AgentToolEvent{ID: "call-6", Kind: "search", Status: llm.AgentToolCompleted})
	handler.OnSegmentDone()
	handler.OnChunk("mais texto")
	handler.OnSegmentDone()

	segmentos := eventosPorNome(emitter, "chat:segment_done")
	if len(segmentos) != 2 {
		t.Fatalf("esperava 2 segmentos, recebi %d", len(segmentos))
	}
	if len(segmentos[1].(ports.SegmentDoneEvent).ToolsInIteration) != 0 {
		t.Error("a ferramenta do segmento anterior não pode aparecer de novo")
	}
}

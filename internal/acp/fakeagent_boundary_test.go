package acp

import (
	"bufio"
	"encoding/json"
	"testing"
)

type fakeBoundaryWriter func([]byte) (int, error)

func (write fakeBoundaryWriter) Write(data []byte) (int, error) { return write(data) }

// Observa a fronteira do fio sem depender de scheduling entre processos.
// O teste de integração continua provando a serialização do cliente.
func TestFakeAgentEncerraTurnoAntesDaRespostaTerminal(t *testing.T) {
	for _, script := range []string{scriptTurn, scriptEcho, scriptDuasConversas, scriptCancel} {
		t.Run(script, func(t *testing.T) {
			agent := &fakeAgent{script: script, cancelled: make(map[string]chan struct{})}
			terminalReplies := 0
			var nextCancel chan struct{}
			agent.out = bufio.NewWriter(fakeBoundaryWriter(func(data []byte) (int, error) {
				var message rpcMessage
				if err := json.Unmarshal(data, &message); err != nil {
					t.Errorf("resposta inválida: %v", err)
					return len(data), nil
				}
				if script == scriptCancel && message.Method == "session/update" {
					select {
					case agent.cancelChan(fakeSessionID) <- struct{}{}:
					default:
					}
				}
				if message.ID == nil || len(message.Result) == 0 {
					return len(data), nil
				}
				terminalReplies++
				var result struct {
					StopReason string `json:"stopReason"`
				}
				if err := json.Unmarshal(message.Result, &result); err != nil {
					t.Errorf("resultado inválido: %v", err)
				}
				want := "end_turn"
				if script == scriptCancel {
					want = "cancelled"
				}
				if result.StopReason != want {
					t.Errorf("stopReason = %q, esperado %q", result.StopReason, want)
				}
				// O próximo prompt chega antes de o Flush da fixture retornar.
				agent.beginTurn(fakeSessionID)
				nextCancel = agent.cancelChan(fakeSessionID)
				if agent.sawOverlap() {
					t.Error("resposta terminal publicada com o turno anterior ainda ativo")
				}
				return len(data), nil
			}))
			id := json.RawMessage("1")
			agent.runTurn(rpcMessage{ID: &id, Params: mustRaw(map[string]any{"sessionId": fakeSessionID})})
			if terminalReplies != 1 {
				t.Fatalf("respostas terminais = %d, esperado 1", terminalReplies)
			}
			if agent.cancelChan(fakeSessionID) != nextCancel || nextCancel == nil {
				t.Error("limpeza tardia do turno anterior apagou o cancelamento do próximo")
			}
		})
	}
}

func TestFakeAgentContinuaDetectandoSobreposicaoReal(t *testing.T) {
	agent := &fakeAgent{cancelled: make(map[string]chan struct{})}
	first := agent.beginTurn(fakeSessionID)
	second := agent.beginTurn(fakeSessionID)
	if !agent.sawOverlap() {
		t.Fatal("dois turnos ativos na mesma sessão não foram detectados")
	}
	agent.endTurn(fakeSessionID, first)
	if agent.cancelChan(fakeSessionID) != second {
		t.Fatal("o primeiro turno apagou o canal do segundo")
	}
	agent.endTurn(fakeSessionID, second)
	if agent.cancelChan(fakeSessionID) != nil {
		t.Fatal("o último turno não liberou a sessão")
	}
}

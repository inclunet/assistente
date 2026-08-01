package acp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// O agente falso é um processo de verdade falando ACP no fio: o binário de
// teste se relança com estas variáveis e vira agente. Testar contra um
// io.Pipe esconderia exatamente o que costuma quebrar — spawn, framing por
// linha e morte do processo.
const (
	fakeAgentEnv  = "ASSISTENTE_ACP_FAKE_AGENT"
	fakeScriptEnv = "ASSISTENTE_ACP_FAKE_SCRIPT"

	// Roteiros do agente falso.
	scriptTurn          = "turn"       // um turno completo com texto, raciocínio e ferramenta
	scriptPermission    = "permission" // pede permissão e conta o que foi decidido
	scriptCancel        = "cancel"     // só termina quando recebe session/cancel
	scriptDie           = "die"        // morre no meio do turno
	scriptCustom        = "custom"     // usa um método de extensão fora do padrão
	scriptEcho          = "echo"       // devolve o que recebeu no prompt
	scriptStall         = "stall"      // sobe, mas nunca responde ao handshake
	scriptStuck         = "stuck"      // aceita o turno e nunca responde, nem ao cancelamento
	scriptTeimoso       = "teimoso"    // ignora o cancelamento e segue falando para sempre
	scriptDuasConversas = "duas"       // fala em pedaços, assinando cada um com a conversa
	scriptIDSujo        = "idsujo"     // abre a conversa com identificador cheio de sujeira

	fakeSessionID = "sess-falsa-1"
	// fakeMuteValue faz o agente aceitar a troca e responder com um conjunto de
	// opções do qual nada é aproveitável.
	fakeMuteValue = "modelo-mudo"
	// fakeDirtySessionID imita um agente que emite identificador com quebra de
	// linha, como o Cursor fez com toolCallId na sonda do AEP-0084, e ainda com
	// espaço nas pontas — que é o que pega quem apara antes de rotear.
	fakeDirtySessionID = " sess-falsa\n1\t"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeAgentEnv) == "1" {
		runFakeAgent(os.Getenv(fakeScriptEnv))
		return
	}
	os.Exit(m.Run())
}

type rpcMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type fakeAgent struct {
	script string

	writeMu sync.Mutex
	out     *bufio.Writer

	mu       sync.Mutex
	nextID   int
	sessions int
	first    string
	pending  map[string]chan rpcMessage
	// cancelled é por sessão, como no agente de verdade: um cancelamento
	// global acordaria o turno da conversa errada e esconderia justamente o
	// erro de roteamento que os testes de duas conversas procuram.
	cancelled map[string]chan struct{}
	// inTurn é por sessão: dois turnos ao mesmo tempo em sessões diferentes é
	// uso normal do agente, o que não pode acontecer é na mesma sessão.
	inTurn  map[string]bool
	overlap bool
}

func runFakeAgent(script string) {
	agent := &fakeAgent{
		script:    script,
		out:       bufio.NewWriter(os.Stdout),
		nextID:    9000,
		pending:   make(map[string]chan rpcMessage),
		inTurn:    make(map[string]bool),
		cancelled: make(map[string]chan struct{}),
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var msg rpcMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Method == "" && msg.ID != nil {
			agent.resolve(msg)
			continue
		}
		agent.handle(msg)
	}
}

func (a *fakeAgent) send(msg any) {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	_, _ = a.out.Write(payload)
	_ = a.out.WriteByte('\n')
	_ = a.out.Flush()
}

func (a *fakeAgent) reply(id json.RawMessage, result any) {
	raw, err := json.Marshal(result)
	if err != nil {
		return
	}
	a.send(rpcMessage{JSONRPC: "2.0", ID: &id, Result: raw})
}

func (a *fakeAgent) replyError(id json.RawMessage, code int, message string) {
	a.send(rpcMessage{JSONRPC: "2.0", ID: &id, Error: &rpcError{Code: code, Message: message}})
}

func (a *fakeAgent) resolve(msg rpcMessage) {
	a.mu.Lock()
	ch := a.pending[string(*msg.ID)]
	delete(a.pending, string(*msg.ID))
	a.mu.Unlock()
	if ch != nil {
		ch <- msg
	}
}

// request manda um pedido ao cliente e espera a resposta.
func (a *fakeAgent) request(method string, params any) rpcMessage {
	a.mu.Lock()
	a.nextID++
	id := json.RawMessage(fmt.Sprintf("%d", a.nextID))
	ch := make(chan rpcMessage, 1)
	a.pending[string(id)] = ch
	a.mu.Unlock()

	a.send(rpcMessage{JSONRPC: "2.0", ID: &id, Method: method, Params: mustRaw(params)})

	select {
	case resp := <-ch:
		return resp
	case <-time.After(20 * time.Second):
		return rpcMessage{Error: &rpcError{Code: -1, Message: "cliente não respondeu"}}
	}
}

func (a *fakeAgent) handle(msg rpcMessage) {
	switch msg.Method {
	case "initialize":
		if a.script == scriptStall {
			return
		}
		a.reply(*msg.ID, map[string]any{
			"protocolVersion": 1,
			"agentInfo":       map[string]any{"name": "agente-falso", "version": "0.1.0"},
			"agentCapabilities": map[string]any{
				"loadSession":         true,
				"promptCapabilities":  map[string]any{"image": true, "audio": false, "embeddedContext": false},
				"sessionCapabilities": map[string]any{"close": map[string]any{}},
			},
			"authMethods": []any{
				map[string]any{"id": "login_falso", "name": "Entrar", "description": "rode o login no terminal"},
			},
		})

	case "session/new":
		a.reply(*msg.ID, map[string]any{
			"sessionId":     a.newSessionID(),
			"configOptions": []any{fakeModelOption("modelo-a")},
			"modes": map[string]any{
				"currentModeId":  "agent",
				"availableModes": []any{map[string]any{"id": "agent", "name": "Agente"}},
			},
		})

	case "session/close":
		if a.script == scriptStuck || a.script == scriptIDSujo {
			// Vivo, mas surdo: é o agente que penduraria o fechamento do app.
			return
		}
		a.reply(*msg.ID, map[string]any{})

	case "session/load":
		a.reply(*msg.ID, map[string]any{"configOptions": []any{fakeModelOption("modelo-b")}})

	case "session/set_config_option":
		var params struct {
			SessionId string `json:"sessionId"`
			ConfigId  string `json:"configId"`
			Value     string `json:"value"`
		}
		_ = json.Unmarshal(msg.Params, &params)
		if params.SessionId != fakeSessionID || params.ConfigId != "model" {
			a.replyError(*msg.ID, -32602, "parâmetros inesperados: "+string(msg.Params))
			return
		}
		if params.Value == fakeMuteValue {
			// Confirma a troca sem devolver nada que o cliente saiba ler, como
			// faria um agente cujas opções sejam todas de um tipo que ainda não
			// modelamos.
			a.reply(*msg.ID, map[string]any{"configOptions": []any{}})
			return
		}
		a.reply(*msg.ID, map[string]any{"configOptions": []any{fakeModelOption(params.Value)}})

	case "session/prompt":
		go a.runTurn(msg)

	case "session/cancel":
		var params struct {
			SessionId string `json:"sessionId"`
		}
		_ = json.Unmarshal(msg.Params, &params)
		if ch := a.cancelChan(params.SessionId); ch != nil {
			select {
			case ch <- struct{}{}:
			default:
			}
		}

	default:
		if msg.ID != nil {
			a.replyError(*msg.ID, -32601, "método desconhecido: "+msg.Method)
		}
	}
}

// newSessionID dá um identificador por conversa. A primeira continua sendo o
// fakeSessionID para não mexer nos testes de sessão única.
func (a *fakeAgent) newSessionID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessions++
	switch {
	case a.script == scriptIDSujo && a.sessions == 1:
		a.first = fakeDirtySessionID
	case a.sessions == 1:
		a.first = fakeSessionID
	default:
		return fmt.Sprintf("sess-falsa-%d", a.sessions)
	}
	return a.first
}

// firstID é a conversa para onde vão as atualizações dos roteiros de uma
// conversa só.
func (a *fakeAgent) firstID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.first == "" {
		return fakeSessionID
	}
	return a.first
}

func (a *fakeAgent) beginTurn(sid string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.inTurn[sid] {
		a.overlap = true
	}
	a.inTurn[sid] = true
	a.cancelled[sid] = make(chan struct{}, 1)
}

func (a *fakeAgent) endTurn(sid string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.inTurn, sid)
	delete(a.cancelled, sid)
}

func (a *fakeAgent) cancelChan(sid string) chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cancelled[sid]
}

func (a *fakeAgent) sawOverlap() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.overlap
}

func (a *fakeAgent) runTurn(msg rpcMessage) {
	var params struct {
		SessionId string `json:"sessionId"`
		Prompt    []struct {
			Text string `json:"text"`
		} `json:"prompt"`
	}
	_ = json.Unmarshal(msg.Params, &params)

	a.beginTurn(params.SessionId)
	defer a.endTurn(params.SessionId)

	// Dois turnos ao mesmo tempo na mesma sessão é justamente o que o cliente
	// deve impedir; o agente falso denuncia pelo fio quando acontece.
	if a.sawOverlap() {
		a.chunk("agent_message_chunk", "CONCORRENTE")
	}

	switch a.script {
	case scriptDuasConversas:
		// Fala devagar e em pedaços, alternando com a outra conversa, e assina
		// cada pedaço com o texto que recebeu. Se o transporte trocar as bolas,
		// o pedaço de uma aba aparece na outra.
		var texto string
		if len(params.Prompt) > 0 {
			texto = params.Prompt[0].Text
		}
		for i := range 5 {
			a.updateFor(params.SessionId, map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": fmt.Sprintf("%s-%d ", texto, i)},
			})
			time.Sleep(10 * time.Millisecond)
		}
		a.reply(*msg.ID, map[string]any{"stopReason": "end_turn"})
		return

	case scriptStuck:
		// Nunca responde: nem ao turno, nem ao cancelamento.
		return

	case scriptTeimoso:
		// Ignora o cancelamento e segue trabalhando. Nunca responde ao turno.
		for {
			a.chunk("agent_message_chunk", "ainda-falando ")
			time.Sleep(20 * time.Millisecond)
		}

	case scriptDie:
		os.Exit(3)

	case scriptEcho:
		a.chunk("agent_message_chunk", string(msg.Params))

	case scriptCancel:
		a.chunkOf(params.SessionId, "agent_message_chunk", "trabalhando")
		select {
		case <-a.cancelChan(params.SessionId):
		case <-time.After(20 * time.Second):
		}
		// Um agente real ainda emite alguma coisa enquanto se recolhe.
		a.chunkOf(params.SessionId, "agent_message_chunk", "depois-do-cancelamento")
		a.reply(*msg.ID, map[string]any{"stopReason": "cancelled"})
		return

	case scriptIDSujo:
		// A extensão vem assinada com o próprio identificador sujo: se o
		// transporte aparar isso antes de procurar a conversa, a pergunta não
		// chega a ninguém e o agente ouve que a conversa acabou.
		resp := a.request("cursor/ask_question", map[string]any{
			"sessionId": params.SessionId,
			"question":  "Prosseguir?",
		})
		if resp.Error != nil {
			a.chunkOf(params.SessionId, "agent_message_chunk", fmt.Sprintf("erro:%d", resp.Error.Code))
		} else {
			a.chunkOf(params.SessionId, "agent_message_chunk", "resposta:"+string(resp.Result))
		}
		a.reply(*msg.ID, map[string]any{"stopReason": "end_turn"})
		return

	case scriptPermission:
		a.toolCall("chamada-1\nfc-2", "execute", "Get-ChildItem", "pending")
		resp := a.request("session/request_permission", map[string]any{
			"sessionId": fakeSessionID,
			"toolCall":  map[string]any{"toolCallId": "chamada-1\nfc-2", "title": "Get-ChildItem", "kind": "execute"},
			"options": []any{
				map[string]any{"optionId": "allow-once", "name": "Permitir uma vez", "kind": "allow_once"},
				map[string]any{"optionId": "allow-always", "name": "Sempre permitir", "kind": "allow_always"},
				map[string]any{"optionId": "reject-once", "name": "Negar", "kind": "reject_once"},
			},
		})
		a.chunk("agent_message_chunk", "decisão: "+describeOutcome(resp))

	case scriptCustom:
		resp := a.request("cursor/ask_question", map[string]any{
			"sessionId": fakeSessionID,
			"question":  "Prosseguir?",
		})
		if resp.Error != nil {
			a.chunk("agent_message_chunk", fmt.Sprintf("erro:%d", resp.Error.Code))
		} else {
			a.chunk("agent_message_chunk", "resposta:"+string(resp.Result))
		}

	default: // scriptTurn
		a.chunk("agent_thought_chunk", "pensando")
		a.chunk("agent_message_chunk", "olá ")
		a.toolCall("chamada-1\nfc-2", "search", "grep por TODO", "pending")
		a.toolUpdate("chamada-1\nfc-2", "completed")
		a.chunk("agent_message_chunk", "mundo")
		a.update(map[string]any{"sessionUpdate": "current_mode_update", "currentModeId": "plan"})
		a.update(map[string]any{"sessionUpdate": "session_info_update", "title": "Listar arquivos"})
		// O agente troca de modelo sozinho (fallback de limite, por exemplo) e
		// avisa com o conjunto completo de opções.
		a.update(map[string]any{
			"sessionUpdate": "config_option_update",
			"configOptions": []any{fakeModelOption("modelo-b")},
		})
		// Um turno instantâneo não provaria nada sobre serialização.
		time.Sleep(40 * time.Millisecond)
	}

	a.reply(*msg.ID, map[string]any{"stopReason": "end_turn"})
}

func (a *fakeAgent) chunk(kind, text string) {
	a.chunkOf(a.firstID(), kind, text)
}

func (a *fakeAgent) chunkOf(sid, kind, text string) {
	a.updateFor(sid, map[string]any{
		"sessionUpdate": kind,
		"content":       map[string]any{"type": "text", "text": text},
	})
}

func (a *fakeAgent) toolCall(id, kind, title, status string) {
	a.update(map[string]any{
		"sessionUpdate": "tool_call",
		"toolCallId":    id,
		"kind":          kind,
		"title":         title,
		"status":        status,
	})
}

func (a *fakeAgent) toolUpdate(id, status string) {
	a.update(map[string]any{
		"sessionUpdate": "tool_call_update",
		"toolCallId":    id,
		"status":        status,
	})
}

func (a *fakeAgent) update(update map[string]any) {
	a.updateFor(a.firstID(), update)
}

func (a *fakeAgent) updateFor(sid string, update map[string]any) {
	a.send(map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params":  map[string]any{"sessionId": sid, "update": update},
	})
}

func describeOutcome(resp rpcMessage) string {
	if resp.Error != nil {
		return fmt.Sprintf("erro:%d", resp.Error.Code)
	}
	var payload struct {
		Outcome struct {
			Outcome  string `json:"outcome"`
			OptionId string `json:"optionId"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal(resp.Result, &payload); err != nil {
		return "ilegível"
	}
	if payload.Outcome.Outcome == "selected" {
		return payload.Outcome.OptionId
	}
	return payload.Outcome.Outcome
}

func fakeModelOption(current string) map[string]any {
	return map[string]any{
		"type":         "select",
		"id":           "model",
		"name":         "Modelo",
		"category":     "model",
		"currentValue": current,
		"options": []any{
			map[string]any{"value": "modelo-a", "name": "Modelo A"},
			map[string]any{"value": "modelo-b", "name": "Modelo B"},
		},
	}
}

func mustRaw(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

// fakeConfig monta a configuração que relança o binário de teste como agente.
func fakeConfig(t *testing.T, script string) Config {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("descobrir o binário de teste: %v", err)
	}
	return Config{
		Command:       executable,
		Env:           map[string]string{fakeAgentEnv: "1", fakeScriptEnv: script},
		WorkDir:       t.TempDir(),
		ClientName:    "assistente-test",
		ClientVersion: "0.0.1",
	}
}

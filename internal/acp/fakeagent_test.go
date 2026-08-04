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
	// scriptSemSessao é a extensão como o Cursor a manda de verdade: sem
	// sessionId no corpo, só com o toolCallId (AEP-0084, descobertas
	// empíricas).
	scriptSemSessao = "semsessao"
	// scriptDescoberta conta, em cada sessão nova, quantas ele já abriu e
	// quantas foram fechadas, e dá a cada uma um modelo corrente diferente. É
	// como o teste vê de fora se a descoberta reperguntou ao agente ou serviu o
	// que já tinha, e se sobrou sessão pendurada nele (AEP-0084 D6).
	scriptDescoberta = "descoberta"
	// scriptLegado é o agente anterior ao configOptions: desconhece
	// session/set_config_option e só entende os seletores de antes.
	scriptLegado = "legado"
	// scriptModelo responde ao turno dizendo em que modelo ele está, que é como
	// o teste confere que a troca valeu para o turno seguinte.
	scriptModelo        = "modelo"
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
	// fakeUnmodeledOption é uma opção que o agente tem e que este pacote não
	// sabe representar — o caso de quem dirige a opção pelo escape hatch.
	fakeUnmodeledOption = "opcao-nao-modelada"
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

	// closes conta os session/close atendidos e model guarda o modelo corrente
	// de cada sessão. Os dois viram resposta no fio: o agente roda em outro
	// processo, e contador que o teste não pode ler não prova nada.
	closes int
	model  map[string]string
}

func runFakeAgent(script string) {
	agent := &fakeAgent{
		script:    script,
		out:       bufio.NewWriter(os.Stdout),
		nextID:    9000,
		pending:   make(map[string]chan rpcMessage),
		inTurn:    make(map[string]bool),
		cancelled: make(map[string]chan struct{}),
		model:     make(map[string]string),
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
		sid := a.newSessionID()
		option := fakeModelOption(a.startModel(sid))
		if a.script == scriptDescoberta {
			// O nome carrega a escrituração do agente: quantas sessões ele
			// abriu e quantas foram fechadas. É o único jeito de o teste, que
			// roda em outro processo, conferir que a descoberta não deixou
			// sessão pendurada.
			option["name"] = a.bookkeeping()
		}
		a.reply(*msg.ID, map[string]any{
			"sessionId":     sid,
			"configOptions": []any{option},
			"modes": map[string]any{
				"currentModeId":  "agent",
				"availableModes": []any{map[string]any{"id": "agent", "name": "Agente"}, map[string]any{"id": "plan", "name": "Plano"}},
			},
		})

	case "session/close":
		if a.script == scriptStuck || a.script == scriptIDSujo {
			// Vivo, mas surdo: é o agente que penduraria o fechamento do app.
			return
		}
		a.countClose()
		a.reply(*msg.ID, map[string]any{})

	case legacySetModelMethod:
		// Só o agente legado conhece o seletor de antes; nos outros ele cai no
		// método desconhecido, como num agente que já migrou.
		if a.script != scriptLegado {
			a.replyError(*msg.ID, -32601, "método desconhecido: "+msg.Method)
			return
		}
		var params struct {
			SessionId string `json:"sessionId"`
			ModelId   string `json:"modelId"`
		}
		_ = json.Unmarshal(msg.Params, &params)
		if params.SessionId == "" || params.ModelId == "" {
			a.replyError(*msg.ID, -32602, "parâmetros inesperados: "+string(msg.Params))
			return
		}
		a.setModel(params.SessionId, params.ModelId)
		// O seletor legado não devolve estado nenhum: é justamente o caso em
		// que o app tem de anotar por conta própria o que pediu.
		a.reply(*msg.ID, nil)

	case "session/set_mode":
		if a.script != scriptLegado {
			a.replyError(*msg.ID, -32601, "método desconhecido: "+msg.Method)
			return
		}
		var params struct {
			SessionId string `json:"sessionId"`
			ModeId    string `json:"modeId"`
		}
		_ = json.Unmarshal(msg.Params, &params)
		if params.SessionId == "" || params.ModeId == "" {
			a.replyError(*msg.ID, -32602, "parâmetros inesperados: "+string(msg.Params))
			return
		}
		a.reply(*msg.ID, nil)

	case "session/load":
		a.reply(*msg.ID, map[string]any{"configOptions": []any{fakeModelOption("modelo-b")}})

	case "session/set_config_option":
		if a.script == scriptLegado {
			// Agente anterior ao formato estável: só conhece os seletores de
			// antes, e é este erro que o app usa para tentar o outro caminho.
			a.replyError(*msg.ID, -32601, "método desconhecido: "+msg.Method)
			return
		}
		var params struct {
			SessionId string `json:"sessionId"`
			ConfigId  string `json:"configId"`
			Value     string `json:"value"`
		}
		_ = json.Unmarshal(msg.Params, &params)
		if a.script == scriptModelo {
			if params.SessionId == "" || params.ConfigId != "model" || params.Value == "" {
				a.replyError(*msg.ID, -32602, "parâmetros inesperados: "+string(msg.Params))
				return
			}
			a.setModel(params.SessionId, params.Value)
			a.reply(*msg.ID, map[string]any{"configOptions": []any{fakeModelOption(params.Value)}})
			return
		}
		if params.SessionId != fakeSessionID {
			a.replyError(*msg.ID, -32602, "parâmetros inesperados: "+string(msg.Params))
			return
		}
		if params.ConfigId == fakeUnmodeledOption {
			// Opção que existe no agente e que o cliente não sabe representar:
			// a troca vale, e o que volta não vira seletor nenhum.
			a.reply(*msg.ID, map[string]any{"configOptions": []any{}})
			return
		}
		if params.ConfigId != "model" {
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

// startModel é o modelo em que uma sessão nasce. No roteiro de descoberta cada
// sessão nasce num modelo diferente: é o que permite distinguir a lista que veio
// do agente agora da que estava guardada.
func (a *fakeAgent) startModel(sid string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	model := "modelo-a"
	if a.script == scriptDescoberta {
		model = fmt.Sprintf("modelo-%d", a.sessions)
	}
	a.model[sid] = model
	return model
}

func (a *fakeAgent) setModel(sid, model string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.model[sid] = model
}

func (a *fakeAgent) currentModel(sid string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model[sid]
}

func (a *fakeAgent) countClose() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closes++
}

// bookkeeping é o que o agente sabe de si: sessões abertas e fechadas até agora,
// e qual processo está contando. O processo entra porque duas execuções do mesmo
// roteiro contam igual — sem ele, uma lista servida do cache de um processo
// morto seria indistinguível da resposta de um processo novo.
func (a *fakeAgent) bookkeeping() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return fmt.Sprintf("abertas=%d fechadas=%d processo=%d", a.sessions, a.closes, os.Getpid())
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

	case scriptDescoberta:
		if len(params.Prompt) > 0 && params.Prompt[0].Text == "morra" {
			// Morrer a pedido do turno é como o processo cai no uso real: no
			// meio de uma conversa, com o cache do processo já povoado.
			os.Exit(3)
		}
		// Fora disso, o agente troca de modelo por conta própria e anuncia.
		a.updateFor(params.SessionId, map[string]any{
			"sessionUpdate": "config_option_update",
			"configOptions": []any{fakeModelOption("modelo-b")},
		})

	case scriptModelo, scriptLegado:
		// O turno diz em que modelo o agente está. É o que prova que a troca
		// pedida pelo app valeu para o turno seguinte, e não só para a tela.
		a.chunkOf(params.SessionId, "agent_message_chunk", "modelo="+a.currentModel(params.SessionId))

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

	case scriptSemSessao:
		resp := a.request("cursor/ask_question", map[string]any{
			"toolCallId": "chamada-1\nfc-2",
			"title":      "Prosseguir?",
		})
		if resp.Error != nil {
			a.chunk("agent_message_chunk", fmt.Sprintf("erro:%d", resp.Error.Code))
		} else {
			a.chunk("agent_message_chunk", "resposta:"+string(resp.Result))
		}

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
	values := []any{
		map[string]any{"value": "modelo-a", "name": "Modelo A"},
		map[string]any{"value": "modelo-b", "name": "Modelo B"},
	}
	// Um agente sempre oferece o modelo em que está. Sem isso, um roteiro que
	// nasce fora da dupla padrão anunciaria um corrente que ele não oferece —
	// estado que existe no mundo real, mas não é o que estes roteiros contam.
	if current != "modelo-a" && current != "modelo-b" {
		values = append(values, map[string]any{"value": current, "name": current})
	}
	return map[string]any{
		"type":         "select",
		"id":           "model",
		"name":         "Modelo",
		"category":     "model",
		"currentValue": current,
		"options":      values,
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

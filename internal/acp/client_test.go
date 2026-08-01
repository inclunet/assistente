package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
)

// testTimeout limita cada teste: um transporte com defeito trava, e travar o
// CI é pior do que falhar nele.
const testTimeout = 30 * time.Second

type collector struct {
	mu      sync.Mutex
	updates []Update
}

func (c *collector) sink(update Update) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updates = append(c.updates, update)
}

func (c *collector) snapshot() []Update {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Update(nil), c.updates...)
}

func (c *collector) textOfKind(kind UpdateKind) string {
	var builder strings.Builder
	for _, update := range c.snapshot() {
		if update.Kind == kind {
			builder.WriteString(update.Text)
		}
	}
	return builder.String()
}

func (c *collector) tools(kind UpdateKind) []ToolCall {
	var out []ToolCall
	for _, update := range c.snapshot() {
		if update.Kind == kind && update.Tool != nil {
			out = append(out, *update.Tool)
		}
	}
	return out
}

func findOption(options []ConfigOption, category string) *ConfigOption {
	for i := range options {
		if options[i].Category == category {
			return &options[i]
		}
	}
	return nil
}

type scriptedHandler struct {
	mu       sync.Mutex
	requests []PermissionRequest

	decide   func(context.Context, PermissionRequest) PermissionOutcome
	custom   func(ctx context.Context, method string, params json.RawMessage) (any, bool)
	fallback func(method string) (any, bool)
}

func (h *scriptedHandler) RequestPermission(ctx context.Context, req PermissionRequest) PermissionOutcome {
	h.mu.Lock()
	h.requests = append(h.requests, req)
	h.mu.Unlock()
	if h.decide == nil {
		return PermissionOutcome{}
	}
	return h.decide(ctx, req)
}

func (h *scriptedHandler) HandleCustom(ctx context.Context, method string, params json.RawMessage) (any, bool) {
	if h.custom == nil {
		return nil, false
	}
	return h.custom(ctx, method, params)
}

func (h *scriptedHandler) CustomFallback(method string) (any, bool) {
	if h.fallback == nil {
		return nil, false
	}
	return h.fallback(method)
}

func (h *scriptedHandler) seen() []PermissionRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]PermissionRequest(nil), h.requests...)
}

func newTestClient(t *testing.T, script string, handler RequestHandler) Client {
	t.Helper()
	client, err := New(fakeConfig(t, script), handler)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	t.Cleanup(cancel)
	return ctx
}

func startSession(t *testing.T, client Client, ctx context.Context) Session {
	t.Helper()
	sess, err := client.NewSession(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("abrir sessão: %v", err)
	}
	return sess
}

func TestTurnoCompletoEntregaTextoRaciocinioEFerramentas(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, ctx)

	col := &collector{}
	stop, err := sess.Prompt(ctx, []Content{TextContent("liste os arquivos")}, col.sink)
	if err != nil {
		t.Fatalf("turno falhou: %v", err)
	}
	if stop != StopEndTurn {
		t.Fatalf("stopReason = %q, esperado %q", stop, StopEndTurn)
	}

	if got := col.textOfKind(UpdateText); got != "olá mundo" {
		t.Errorf("texto da resposta = %q, esperado %q", got, "olá mundo")
	}
	if got := col.textOfKind(UpdateThought); got != "pensando" {
		t.Errorf("raciocínio = %q, esperado %q", got, "pensando")
	}

	started := col.tools(UpdateToolStart)
	if len(started) != 1 {
		t.Fatalf("esperava 1 ferramenta iniciada, obtive %d", len(started))
	}
	// O identificador vem com quebra de linha no meio, como o Cursor emitiu na
	// sonda do AEP-0084; virar chave ou texto anunciado assim seria um bug.
	if started[0].ID != "chamada-1 fc-2" {
		t.Errorf("identificador da ferramenta = %q, esperado normalizado", started[0].ID)
	}
	if started[0].Kind != "search" || started[0].Title != "grep por TODO" {
		t.Errorf("ferramenta inesperada: %+v", started[0])
	}

	progress := col.tools(UpdateToolProgress)
	if len(progress) != 1 || progress[0].Status != "completed" {
		t.Errorf("atualização de ferramenta inesperada: %+v", progress)
	}
}

func TestTurnoInformaModoTituloETrocaDeModeloFeitaPeloAgente(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, ctx)

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("oi")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	var modo, titulo string
	var modelo string
	for _, update := range col.snapshot() {
		switch update.Kind {
		case UpdateMode:
			modo = update.Mode
		case UpdateTitle:
			titulo = update.Title
		case UpdateConfigOptions:
			if len(update.ConfigOptions) > 0 {
				modelo = update.ConfigOptions[0].CurrentValue
			}
		}
	}
	if modo != "plan" {
		t.Errorf("modo corrente = %q, esperado %q", modo, "plan")
	}
	if titulo != "Listar arquivos" {
		t.Errorf("título = %q", titulo)
	}
	if modelo != "modelo-b" {
		t.Errorf("modelo corrente = %q, esperado %q", modelo, "modelo-b")
	}
	// A troca feita pelo agente precisa ficar visível na sessão, senão a pessoa
	// segue achando que fala com outro modelo.
	if got := findOption(sess.ConfigOptions(), "model"); got == nil || got.CurrentValue != "modelo-b" {
		t.Errorf("sessão não refletiu a troca do agente: %+v", sess.ConfigOptions())
	}
}

func TestSessaoNovaExpoeModeloEModo(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, ctx)

	options := sess.ConfigOptions()
	if len(options) != 2 {
		t.Fatalf("esperava opções de modelo e modo, obtive %d: %+v", len(options), options)
	}
	model := options[0]
	if model.Category != "model" || model.CurrentValue != "modelo-a" || len(model.Values) != 2 {
		t.Errorf("opção de modelo inesperada: %+v", model)
	}
	// O formato legado de modos vira ConfigOption para o app ter um caminho só.
	if options[1].Category != "mode" || options[1].CurrentValue != "agent" {
		t.Errorf("opção de modo inesperada: %+v", options[1])
	}
}

func TestTrocaDeModeloDevolveEstadoCompleto(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, ctx)

	options, err := sess.SetConfigOption(ctx, "model", "modelo-b")
	if err != nil {
		t.Fatalf("trocar modelo: %v", err)
	}
	modelo := findOption(options, "model")
	if modelo == nil || modelo.CurrentValue != "modelo-b" {
		t.Fatalf("estado devolvido inesperado: %+v", options)
	}
	// O agente no formato legado responde só com o que ele chama de
	// configOptions. O seletor de modo não pode sumir da conversa só porque a
	// pessoa trocou de modelo.
	if findOption(options, modeCategory) == nil {
		t.Errorf("o modo sumiu ao trocar de modelo: %+v", options)
	}
	if got := findOption(sess.ConfigOptions(), "model"); got == nil || got.CurrentValue != "modelo-b" {
		t.Errorf("sessão não guardou o novo estado: %+v", sess.ConfigOptions())
	}

	// O estado devolvido é cópia: mexer nele não pode trocar o modelo da sessão
	// por fora, sem passar pelo agente.
	modelo.CurrentValue = "adulterado"
	if got := findOption(sess.ConfigOptions(), "model"); got.CurrentValue != "modelo-b" {
		t.Errorf("mexer no retorno alterou o estado da sessão: %+v", got)
	}
}

func TestPedidoDePermissaoChegaAoHandlerEADecisaoVoltaAoAgente(t *testing.T) {
	ctx := testContext(t)
	handler := &scriptedHandler{
		decide: func(context.Context, PermissionRequest) PermissionOutcome {
			return PermissionOutcome{OptionID: "allow-once"}
		},
	}
	client := newTestClient(t, scriptPermission, handler)
	sess := startSession(t, client, ctx)

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("rode um comando")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	if got := col.textOfKind(UpdateText); !strings.Contains(got, "decisão: allow-once") {
		t.Errorf("agente não recebeu a decisão: %q", got)
	}

	seen := handler.seen()
	if len(seen) != 1 {
		t.Fatalf("esperava 1 pedido de permissão, obtive %d", len(seen))
	}
	if seen[0].ToolCall.ID != "chamada-1 fc-2" {
		t.Errorf("identificador não normalizado no pedido: %q", seen[0].ToolCall.ID)
	}
	if len(seen[0].Options) != 3 || seen[0].Options[0].Kind != "allow_once" {
		t.Errorf("opções do pedido inesperadas: %+v", seen[0].Options)
	}
}

// O ACP exige que quem cancela o turno responda "cancelado" aos pedidos de
// permissão pendentes. Responder recusa seria mentira sobre uma decisão que
// ninguém tomou, e deixar o diálogo aberto pediria à pessoa que decidisse sobre
// um turno que ela mesma abortou.
func TestCancelarOTurnoCancelaOPedidoDePermissaoPendente(t *testing.T) {
	ctx := testContext(t)
	perguntou := make(chan struct{})
	dialogoFechou := make(chan struct{})
	handler := &scriptedHandler{
		decide: func(hctx context.Context, _ PermissionRequest) PermissionOutcome {
			close(perguntou)
			// O diálogo na tela só some quando o contexto do pedido morre.
			<-hctx.Done()
			close(dialogoFechou)
			return PermissionOutcome{}
		},
	}
	client := newTestClient(t, scriptPermission, handler)
	sess := startSession(t, client, ctx)

	turno, desistir := context.WithCancel(ctx)
	defer desistir()

	col := &collector{}
	resultado := make(chan error, 1)
	go func() {
		_, err := sess.Prompt(turno, []Content{TextContent("rode um comando")}, col.sink)
		resultado <- err
	}()

	select {
	case <-perguntou:
	case <-time.After(testTimeout):
		t.Fatal("o pedido de permissão nunca chegou ao handler")
	}
	desistir()

	select {
	case <-dialogoFechou:
	case <-time.After(testTimeout):
		t.Fatal("o diálogo de permissão não foi fechado pelo cancelamento")
	}
	select {
	case <-resultado:
	case <-time.After(testTimeout):
		t.Fatal("o turno cancelado nunca voltou")
	}

	if got := col.textOfKind(UpdateText); !strings.Contains(got, "decisão: cancelled") {
		t.Errorf("agente deveria ter recebido desfecho cancelado, recebeu: %q", got)
	}
}

// Excluir a conversa enquanto o agente pergunta algo não pode deixar o agente
// esperando resposta nem o diálogo aberto na tela.
func TestFecharASessaoCancelaOPedidoDePermissaoPendente(t *testing.T) {
	ctx := testContext(t)
	perguntou := make(chan struct{})
	dialogoFechou := make(chan struct{})
	handler := &scriptedHandler{
		decide: func(hctx context.Context, _ PermissionRequest) PermissionOutcome {
			close(perguntou)
			<-hctx.Done()
			close(dialogoFechou)
			return PermissionOutcome{}
		},
	}
	client := newTestClient(t, scriptPermission, handler)
	sess := startSession(t, client, ctx)

	col := &collector{}
	resultado := make(chan error, 1)
	go func() {
		_, err := sess.Prompt(ctx, []Content{TextContent("rode um comando")}, col.sink)
		resultado <- err
	}()

	select {
	case <-perguntou:
	case <-time.After(testTimeout):
		t.Fatal("o pedido de permissão nunca chegou ao handler")
	}
	if err := sess.Close(ctx); err != nil {
		t.Fatalf("encerrar sessão: %v", err)
	}

	select {
	case <-dialogoFechou:
	case <-time.After(testTimeout):
		t.Fatal("o diálogo de permissão sobreviveu ao fim da sessão")
	}
	select {
	case <-resultado:
	case <-time.After(testTimeout):
		t.Fatal("o turno da sessão encerrada nunca voltou")
	}
}

// Enquanto o agente não confirma o cancelamento, a vez continua ocupada — dois
// turnos no mesmo sessionId se atropelariam, e com um agente de código isso é
// gente editando o mesmo repositório duas vezes. O que não vale é deixar o
// próximo turno esperando no escuro: ele é recusado dizendo o motivo.
func TestCancelamentoSemConfirmacaoRecusaOProximoTurnoDizendoOMotivo(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptStuck, nil)
	sess := startSession(t, client, ctx)
	sess.(*session).grace = 300 * time.Millisecond

	turno, desistir := context.WithCancel(ctx)
	primeiro := make(chan error, 1)
	go func() {
		_, err := sess.Prompt(turno, []Content{TextContent("comece algo demorado")}, nil)
		primeiro <- err
	}()
	time.Sleep(200 * time.Millisecond)
	desistir()

	select {
	case err := <-primeiro:
		if !errors.Is(err, ErrCancelNotConfirmed) {
			t.Fatalf("esperava cancelamento não confirmado, obtive: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("o turno cancelado nunca voltou")
	}

	inicio := time.Now()
	_, err := sess.Prompt(ctx, []Content{TextContent("e agora?")}, nil)
	if !errors.Is(err, ErrCancelNotConfirmed) {
		t.Fatalf("o turno seguinte deveria explicar a recusa, obtive: %v", err)
	}
	if levou := time.Since(inicio); levou > 2*time.Second {
		t.Errorf("a recusa deveria ser imediata, levou %s", levou)
	}
	var falha *PromptError
	if !errors.As(err, &falha) || falha.Accepted {
		t.Errorf("o turno recusado não chegou ao agente: %+v", falha)
	}
}

// Quem já estava na fila quando o prazo estourou precisa ser acordado com o
// motivo. Sem isso, o barge-in do app fica esperando calado por um turno que
// talvez nunca volte — e o contexto de quem pediu pode nem ter prazo.
func TestQuemEsperavaNaFilaAcordaQuandoOCancelamentoNaoEhConfirmado(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptStuck, nil)
	sess := startSession(t, client, ctx)
	sess.(*session).grace = 500 * time.Millisecond

	turno, desistir := context.WithCancel(ctx)
	go func() {
		_, _ = sess.Prompt(turno, []Content{TextContent("comece algo demorado")}, nil)
	}()
	time.Sleep(200 * time.Millisecond)

	// Sem prazo de propósito: quem tira essa espera do escuro tem de ser a
	// sessão, não o relógio de quem chamou.
	naFila := make(chan error, 1)
	go func() {
		_, err := sess.Prompt(context.Background(), []Content{TextContent("e agora?")}, nil)
		naFila <- err
	}()
	time.Sleep(200 * time.Millisecond)
	desistir()

	select {
	case err := <-naFila:
		if !errors.Is(err, ErrCancelNotConfirmed) {
			t.Fatalf("esperava cancelamento não confirmado, obtive: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("quem esperava na fila nunca foi acordado")
	}
}

// Quem chega com o contexto já cancelado não pode botar o agente para
// trabalhar: com a fila livre, o turno sairia e só depois se descobriria que
// ninguém está mais esperando por ele.
func TestTurnoComContextoJaCanceladoNaoVaiAoAgente(t *testing.T) {
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, testContext(t))

	morto, cancelar := context.WithCancel(context.Background())
	cancelar()

	col := &collector{}
	_, err := sess.Prompt(morto, []Content{TextContent("oi")}, col.sink)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava contexto cancelado, obtive: %v", err)
	}
	var falha *PromptError
	if !errors.As(err, &falha) || falha.Accepted {
		t.Fatalf("o agente não chegou a aceitar o turno: %+v", falha)
	}
	if got := col.textOfKind(UpdateText); got != "" {
		t.Errorf("nada deveria ter sido entregue: %q", got)
	}
}

// Encerrar a conversa durante a saída do app costuma chegar com o contexto já
// cancelado. Mandar o agente parar não pode depender disso: o que está em jogo
// é um agente de código continuar mexendo no disco.
func TestFecharASessaoCancelaOTurnoMesmoComContextoMorto(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptCancel, nil)
	sess := startSession(t, client, ctx)

	col := &collector{}
	parou := make(chan StopReason, 1)
	go func() {
		stop, _ := sess.Prompt(ctx, []Content{TextContent("trabalhe")}, col.sink)
		parou <- stop
	}()

	prazo := time.Now().Add(testTimeout)
	for !strings.Contains(col.textOfKind(UpdateText), "trabalhando") && time.Now().Before(prazo) {
		time.Sleep(10 * time.Millisecond)
	}

	morto, cancelar := context.WithCancel(context.Background())
	cancelar()
	// O session/close também não pode depender do contexto de saída: a sessão
	// sumiria daqui e continuaria aberta no agente.
	if err := sess.Close(morto); err != nil {
		t.Fatalf("encerrar sessão com contexto morto: %v", err)
	}

	// O agente falso desiste sozinho depois de 20s; um limite bem menor separa
	// o cancelamento entregue da desistência por tédio.
	select {
	case stop := <-parou:
		if stop != StopCancelled {
			t.Fatalf("esperava turno cancelado, obtive %q", stop)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o cancelamento não chegou ao agente com o contexto de saída já morto")
	}
}

// Um turno que esperava a vez na fila não pode ser enviado depois de a sessão
// ter sido encerrada.
func TestTurnoNaFilaNaoRodaEmSessaoEncerrada(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptCancel, nil)
	sess := startSession(t, client, ctx)

	primeiro := &collector{}
	go func() {
		_, _ = sess.Prompt(ctx, []Content{TextContent("primeiro")}, primeiro.sink)
	}()

	// O segundo turno só entra na fila depois que o primeiro está de fato em
	// andamento no agente.
	prazo := time.Now().Add(testTimeout)
	for !strings.Contains(primeiro.textOfKind(UpdateText), "trabalhando") && time.Now().Before(prazo) {
		time.Sleep(10 * time.Millisecond)
	}

	naFila := make(chan error, 1)
	go func() {
		_, err := sess.Prompt(ctx, []Content{TextContent("segundo")}, nil)
		naFila <- err
	}()
	time.Sleep(200 * time.Millisecond)

	if err := sess.Close(ctx); err != nil {
		t.Fatalf("encerrar sessão: %v", err)
	}

	select {
	case err := <-naFila:
		if !errors.Is(err, ErrSessionClosed) {
			t.Fatalf("esperava sessão encerrada, obtive: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("o turno na fila nunca voltou")
	}
}

func TestSemHandlerOPedidoDePermissaoEhNegadoPontualmente(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptPermission, nil)
	sess := startSession(t, client, ctx)

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("rode um comando")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	// Nega a ação em vez de cancelar o turno inteiro, e nega uma vez em vez de
	// calar o agente para sempre.
	if got := col.textOfKind(UpdateText); !strings.Contains(got, "decisão: reject-once") {
		t.Errorf("desfecho negativo inesperado: %q", got)
	}
}

func TestHandlerQueEntraEmPanicoNaoPenduraOAgente(t *testing.T) {
	ctx := testContext(t)
	handler := &scriptedHandler{
		decide: func(context.Context, PermissionRequest) PermissionOutcome {
			panic("handler quebrado")
		},
	}
	client := newTestClient(t, scriptPermission, handler)
	sess := startSession(t, client, ctx)

	col := &collector{}
	stop, err := sess.Prompt(ctx, []Content{TextContent("rode um comando")}, col.sink)
	if err != nil {
		t.Fatalf("turno falhou: %v", err)
	}
	if stop != StopEndTurn {
		t.Fatalf("turno não terminou normalmente: %q", stop)
	}
	if got := col.textOfKind(UpdateText); !strings.Contains(got, "decisão: reject-once") {
		t.Errorf("pânico deveria virar negativa, obtive: %q", got)
	}
}

func TestMetodoDesconhecidoRecebeRespostaDeProtocolo(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptCustom, nil)
	sess := startSession(t, client, ctx)

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("pergunte algo")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	// Sem tratamento, o agente precisa receber "método não encontrado" — e não
	// silêncio, que o deixaria esperando para sempre.
	if got := col.textOfKind(UpdateText); !strings.Contains(got, "erro:-32601") {
		t.Errorf("esperava método não encontrado, obtive: %q", got)
	}
}

func TestExtensaoTratadaPeloHandlerRespondeAoAgente(t *testing.T) {
	ctx := testContext(t)
	var vistos []string
	handler := &scriptedHandler{
		custom: func(_ context.Context, method string, _ json.RawMessage) (any, bool) {
			vistos = append(vistos, method)
			return map[string]any{"answer": "sim"}, true
		},
	}
	client := newTestClient(t, scriptCustom, handler)
	sess := startSession(t, client, ctx)

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("pergunte algo")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	if got := col.textOfKind(UpdateText); !strings.Contains(got, `"answer":"sim"`) {
		t.Errorf("resposta da extensão não chegou ao agente: %q", got)
	}
	// Extensões do Cursor não começam com "_" e seriam recusadas pelo cliente
	// pronto do SDK antes de o app ver o pedido.
	if len(vistos) != 1 || vistos[0] != "cursor/ask_question" {
		t.Errorf("métodos vistos pelo handler: %v", vistos)
	}
}

// Um handler quebrado é defeito do app, não falta de suporte à extensão:
// responder "método não encontrado" faria o agente riscar a extensão da lista.
func TestExtensaoComHandlerEmPanicoRespondeErroInternoENaoFaltaDeSuporte(t *testing.T) {
	ctx := testContext(t)
	handler := &scriptedHandler{
		custom: func(context.Context, string, json.RawMessage) (any, bool) {
			panic("handler quebrado")
		},
	}
	client := newTestClient(t, scriptCustom, handler)
	sess := startSession(t, client, ctx)

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("pergunte algo")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	esperado := fmt.Sprintf("erro:%d", sdk.NewInternalError(nil).Code)
	if got := col.textOfKind(UpdateText); !strings.Contains(got, esperado) {
		t.Errorf("esperava %s, obtive: %q", esperado, got)
	}
}

// As extensões bloqueantes do Cursor pertencem ao turno: cancelá-lo precisa
// fechar a pergunta na tela e devolver ao agente um desfecho de protocolo.
func TestCancelarOTurnoCancelaAExtensaoBloqueante(t *testing.T) {
	ctx := testContext(t)
	perguntou := make(chan struct{})
	dialogoFechou := make(chan struct{})
	handler := &scriptedHandler{
		custom: func(hctx context.Context, _ string, _ json.RawMessage) (any, bool) {
			close(perguntou)
			<-hctx.Done()
			close(dialogoFechou)
			return nil, false
		},
	}
	client := newTestClient(t, scriptCustom, handler)
	sess := startSession(t, client, ctx)

	turno, desistir := context.WithCancel(ctx)
	defer desistir()

	col := &collector{}
	resultado := make(chan error, 1)
	go func() {
		_, err := sess.Prompt(turno, []Content{TextContent("pergunte algo")}, col.sink)
		resultado <- err
	}()

	select {
	case <-perguntou:
	case <-time.After(testTimeout):
		t.Fatal("a extensão nunca chegou ao handler")
	}
	desistir()

	select {
	case <-dialogoFechou:
	case <-time.After(testTimeout):
		t.Fatal("a pergunta da extensão sobreviveu ao cancelamento")
	}
	select {
	case <-resultado:
	case <-time.After(testTimeout):
		t.Fatal("o turno cancelado nunca voltou")
	}

	// "Método não encontrado" faria o agente concluir que o app não suporta a
	// extensão; o que houve foi o turno acabar.
	esperado := fmt.Sprintf("erro:%d", sdk.NewRequestCancelled(nil).Code)
	if got := col.textOfKind(UpdateText); !strings.Contains(got, esperado) {
		t.Errorf("esperava %s, obtive: %q", esperado, got)
	}
}

func TestCancelarOTurnoChegaAoAgenteEEncerraComoCancelado(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptCancel, nil)
	sess := startSession(t, client, ctx)

	turnCtx, cancelTurn := context.WithCancel(ctx)
	col := &collector{}

	go func() {
		// Espera o turno começar de fato antes de cancelar.
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if col.textOfKind(UpdateText) != "" {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		cancelTurn()
	}()

	stop, err := sess.Prompt(turnCtx, []Content{TextContent("trabalhe")}, col.sink)
	if err != nil {
		t.Fatalf("cancelamento deveria encerrar o turno normalmente: %v", err)
	}
	// O agente confirmou: o turno acabou de verdade, não só a nossa espera.
	if stop != StopCancelled {
		t.Fatalf("stopReason = %q, esperado %q", stop, StopCancelled)
	}
	// O rastro que o agente emite enquanto se recolhe continua chegando. É por
	// ele que a pessoa fica sabendo o que o agente alcançou a fazer antes de
	// parar; engolir isso esconderia trabalho que aconteceu de verdade.
	if got := col.textOfKind(UpdateText); !strings.Contains(got, "depois-do-cancelamento") {
		t.Errorf("o rastro do agente após o cancelamento não chegou: %q", got)
	}
}

// O agente anuncia o modo pelo formato legado e, no mesmo turno, manda o
// conjunto de opções falando só de modelo. O seletor de modo não pode sumir da
// conversa por causa disso, nem ficar mostrando o modo anterior.
func TestModoDaSessaoSobreviveAoTurnoEAcompanhaATroca(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, ctx)

	if _, err := sess.Prompt(ctx, []Content{TextContent("oi")}, func(Update) {}); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	options := sess.ConfigOptions()
	modo := findOption(options, modeCategory)
	modelo := findOption(options, "model")
	if modelo == nil || modelo.CurrentValue != "modelo-b" {
		t.Errorf("o modelo trocado pelo agente não ficou no estado: %+v", options)
	}
	if modo == nil {
		t.Fatalf("o modo sumiu das opções depois do turno: %+v", options)
	}
	if modo.CurrentValue != "plan" {
		t.Errorf("o modo ficou desatualizado: %+v", *modo)
	}
}

// Duas abas de chat no mesmo agente são duas sessões no mesmo processo. Elas
// respondem ao mesmo tempo, e cada uma só pode receber o que é dela: o
// transporte separa pelo sessionId que vem em toda atualização.
func TestDuasConversasNoMesmoProcessoNaoSeMisturam(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptDuasConversas, nil)

	primeira := startSession(t, client, ctx)
	segunda := startSession(t, client, ctx)
	if primeira.ID() == segunda.ID() {
		t.Fatalf("as duas conversas ficaram com o mesmo identificador: %q", primeira.ID())
	}

	colA, colB := &collector{}, &collector{}
	fim := make(chan error, 2)
	go func() {
		_, err := primeira.Prompt(ctx, []Content{TextContent("aba-a")}, colA.sink)
		fim <- err
	}()
	go func() {
		_, err := segunda.Prompt(ctx, []Content{TextContent("aba-b")}, colB.sink)
		fim <- err
	}()
	for range 2 {
		select {
		case err := <-fim:
			if err != nil {
				t.Fatalf("turno falhou: %v", err)
			}
		case <-time.After(testTimeout):
			t.Fatal("as conversas não terminaram")
		}
	}

	textoA := colA.textOfKind(UpdateText)
	textoB := colB.textOfKind(UpdateText)
	if !strings.Contains(textoA, "aba-a") || strings.Contains(textoA, "aba-b") {
		t.Errorf("a primeira conversa recebeu texto da outra: %q", textoA)
	}
	if !strings.Contains(textoB, "aba-b") || strings.Contains(textoB, "aba-a") {
		t.Errorf("a segunda conversa recebeu texto da outra: %q", textoB)
	}
	// Cada uma recebeu a resposta inteira: separar não pode significar perder
	// pedaço no caminho.
	for i := range 5 {
		pedaco := fmt.Sprintf("aba-a-%d", i)
		if !strings.Contains(textoA, pedaco) {
			t.Errorf("faltou %q na primeira conversa: %q", pedaco, textoA)
		}
	}
}

// O agente confirmou a troca; o que ele respondeu depois não muda isso. Se o
// conjunto de opções vier sem nada que saibamos ler, a tela não pode continuar
// anunciando o modelo antigo para uma troca que aconteceu de verdade.
func TestTrocaConfirmadaComRespostaIlegivelAindaVale(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, ctx)

	opcoes, err := sess.SetConfigOption(ctx, "model", "modelo-mudo")
	if err != nil {
		t.Fatalf("trocar de modelo: %v", err)
	}
	modelo := findOption(opcoes, "model")
	if modelo == nil || modelo.CurrentValue != "modelo-mudo" {
		t.Fatalf("a troca confirmada não apareceu no retorno: %+v", opcoes)
	}
	if modelo := findOption(sess.ConfigOptions(), "model"); modelo.CurrentValue != "modelo-mudo" {
		t.Errorf("o estado da sessão ficou no modelo antigo: %+v", *modelo)
	}
	// E a lista de modelos sobrevive: uma resposta que não trouxe nada não é
	// motivo para o seletor sumir da tela.
	if len(modelo.Values) == 0 {
		t.Error("as opções disponíveis se perderam")
	}
}

// Trocar de modelo no meio da resposta é previsto no ACP — "whether the Agent
// is idle or generating a response" — e é o caso real de quem percebe, ouvindo,
// que pediu ao modelo errado. Enfileirar isso atrás do turno faria a troca só
// valer quando a resposta acabasse, que é quando ela já não serve.
func TestTrocarDeModeloNoMeioDoTurnoFuncionaENaoAtrapalha(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptCancel, nil)
	sess := startSession(t, client, ctx)

	turno, desistir := context.WithCancel(ctx)
	defer desistir()
	col := &collector{}
	fim := make(chan promptOutcome, 1)
	go func() {
		stop, err := sess.Prompt(turno, []Content{TextContent("trabalhe")}, col.sink)
		fim <- promptOutcome{stop: stop, err: err}
	}()

	prazo := time.Now().Add(5 * time.Second)
	for !strings.Contains(col.textOfKind(UpdateText), "trabalhando") {
		if time.Now().After(prazo) {
			t.Fatal("o turno não começou")
		}
		time.Sleep(10 * time.Millisecond)
	}

	opcoes, err := sess.SetConfigOption(ctx, "model", "modelo-b")
	if err != nil {
		t.Fatalf("trocar de modelo com o turno em andamento: %v", err)
	}
	if modelo := findOption(opcoes, "model"); modelo == nil || modelo.CurrentValue != "modelo-b" {
		t.Fatalf("a troca não valeu: %+v", opcoes)
	}
	if modelo := findOption(sess.ConfigOptions(), "model"); modelo.CurrentValue != "modelo-b" {
		t.Errorf("o estado da sessão ficou para trás: %+v", *modelo)
	}

	// E o turno segue de pé: a troca no meio do caminho não pode atropelar a
	// resposta que estava sendo escrita.
	desistir()
	select {
	case out := <-fim:
		if out.err != nil || out.stop != StopCancelled {
			t.Fatalf("o turno terminou mal depois da troca: stop=%q err=%v", out.stop, out.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("o turno não voltou depois da troca de modelo")
	}
	if got := col.textOfKind(UpdateText); !strings.Contains(got, "depois-do-cancelamento") {
		t.Errorf("o rastro final do turno se perdeu: %q", got)
	}
}

// Parar a resposta numa aba não pode parar a da outra. O cancelamento viaja
// com o sessionId, e trocar as bolas aqui derrubaria o trabalho de uma conversa
// que ninguém mandou parar.
func TestCancelarUmaConversaNaoParaAOutra(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptCancel, nil)
	primeira := startSession(t, client, ctx)
	segunda := startSession(t, client, ctx)

	turnoA, pararA := context.WithCancel(ctx)
	defer pararA()
	colA, colB := &collector{}, &collector{}
	fimA := make(chan promptOutcome, 1)
	fimB := make(chan error, 1)
	go func() {
		stop, err := primeira.Prompt(turnoA, []Content{TextContent("aba-a")}, colA.sink)
		fimA <- promptOutcome{stop: stop, err: err}
	}()
	go func() {
		_, err := segunda.Prompt(ctx, []Content{TextContent("aba-b")}, colB.sink)
		fimB <- err
	}()

	// As duas precisam estar trabalhando antes do cancelamento, senão o teste
	// passaria sem que houvesse a outra conversa para atrapalhar.
	esperar := func(col *collector) {
		t.Helper()
		prazo := time.Now().Add(5 * time.Second)
		for !strings.Contains(col.textOfKind(UpdateText), "trabalhando") {
			if time.Now().After(prazo) {
				t.Fatal("a conversa não começou a responder")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	esperar(colA)
	esperar(colB)

	pararA()
	select {
	case out := <-fimA:
		if out.err != nil {
			t.Fatalf("a conversa cancelada devolveu erro: %v", out.err)
		}
		// O agente confirmou o cancelamento dessa conversa, e só dela.
		if out.stop != StopCancelled {
			t.Fatalf("stopReason = %q, esperado %q", out.stop, StopCancelled)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a conversa cancelada não voltou")
	}

	select {
	case err := <-fimB:
		t.Fatalf("a outra conversa parou junto: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	if got := colB.textOfKind(UpdateText); strings.Contains(got, "depois-do-cancelamento") {
		t.Errorf("o cancelamento vazou para a outra conversa: %q", got)
	}
}

func TestTurnosDaMesmaSessaoNaoSeAtropelam(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, ctx)

	col := &collector{}
	var wg sync.WaitGroup
	erros := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := sess.Prompt(ctx, []Content{TextContent("oi")}, col.sink); err != nil {
				erros <- err
			}
		}()
	}
	wg.Wait()
	close(erros)
	for err := range erros {
		t.Fatalf("turno falhou: %v", err)
	}

	// O agente falso denuncia sobreposição pelo próprio fio.
	if strings.Contains(col.textOfKind(UpdateText), "CONCORRENTE") {
		t.Error("dois session/prompt correram ao mesmo tempo na mesma sessão")
	}
}

func TestMorteDoProcessoPerdeASessaoEPermiteReconectar(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptDie, nil)
	sess := startSession(t, client, ctx)

	_, err := sess.Prompt(ctx, []Content{TextContent("morra")}, func(Update) {})
	if !errors.Is(err, ErrSessionLost) {
		t.Fatalf("esperava sessão perdida, obtive: %v", err)
	}

	// A classificação é conservadora de propósito: o pedido chegou a sair, e
	// repetir sozinho poderia refazer edições e comandos (AEP-0084 D4).
	var promptErr *PromptError
	if !errors.As(err, &promptErr) {
		t.Fatalf("erro deveria dizer se o turno foi aceito: %v", err)
	}
	if !promptErr.Accepted {
		t.Error("turno enviado antes da queda deveria contar como aceito")
	}

	// A sessão morreu com o processo, mas o cliente sobe outro no próximo uso.
	if _, err := client.NewSession(ctx, t.TempDir()); err != nil {
		t.Fatalf("cliente deveria reconectar: %v", err)
	}
}

func TestSessaoPerdidaRecusaNovosTurnos(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptDie, nil)
	sess := startSession(t, client, ctx)

	if _, err := sess.Prompt(ctx, []Content{TextContent("morra")}, func(Update) {}); err == nil {
		t.Fatal("o primeiro turno deveria falhar")
	}
	if _, err := sess.Prompt(ctx, []Content{TextContent("de novo")}, func(Update) {}); !errors.Is(err, ErrSessionLost) {
		t.Fatalf("sessão morta deveria recusar o turno: %v", err)
	}
	if err := sess.Cancel(ctx); !errors.Is(err, ErrSessionLost) {
		t.Fatalf("cancelar sessão morta deveria informar a perda: %v", err)
	}
}

func TestConteudoMultimodalChegaComoBlocosNaOrdem(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptEcho, nil)
	sess := startSession(t, client, ctx)

	col := &collector{}
	_, err := sess.Prompt(ctx, []Content{
		TextContent("olhe isto"),
		ImageContent("ZmFrZQ==", "image/png"),
	}, col.sink)
	if err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	var enviado struct {
		SessionId string `json:"sessionId"`
		Prompt    []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Data     string `json:"data"`
			MimeType string `json:"mimeType"`
		} `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(col.textOfKind(UpdateText)), &enviado); err != nil {
		t.Fatalf("eco ilegível (%v): %s", err, col.textOfKind(UpdateText))
	}
	if enviado.SessionId != fakeSessionID {
		t.Errorf("sessão enviada = %q", enviado.SessionId)
	}
	if len(enviado.Prompt) != 2 {
		t.Fatalf("esperava 2 blocos, obtive %d: %+v", len(enviado.Prompt), enviado.Prompt)
	}
	if enviado.Prompt[0].Type != "text" || enviado.Prompt[0].Text != "olhe isto" {
		t.Errorf("primeiro bloco inesperado: %+v", enviado.Prompt[0])
	}
	if enviado.Prompt[1].Type != "image" || enviado.Prompt[1].MimeType != "image/png" {
		t.Errorf("segundo bloco inesperado: %+v", enviado.Prompt[1])
	}
}

func TestHandshakeExpoeCapacidadesDoAgente(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)

	caps, err := client.Capabilities(ctx)
	if err != nil {
		t.Fatalf("obter capacidades: %v", err)
	}
	if !caps.LoadSession || !caps.PromptImage || caps.PromptAudio {
		t.Errorf("capacidades inesperadas: %+v", caps)
	}
	if caps.AgentName != "agente-falso" {
		t.Errorf("nome do agente = %q", caps.AgentName)
	}
	if len(caps.AuthMethods) != 1 || caps.AuthMethods[0].ID != "login_falso" {
		t.Fatalf("métodos de autenticação inesperados: %+v", caps.AuthMethods)
	}
	// Sem "type" no payload, o método é do tipo que o próprio agente conduz —
	// o caso do Cursor, que pede um login pelo terminal.
	if caps.AuthMethods[0].Kind != AuthKindAgent {
		t.Errorf("tipo de autenticação = %q", caps.AuthMethods[0].Kind)
	}
}

func TestRetomarSessaoUsaOIdentificadorExistente(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)

	sess, err := client.LoadSession(ctx, fakeSessionID, t.TempDir())
	if err != nil {
		t.Fatalf("retomar sessão: %v", err)
	}
	if sess.ID() != fakeSessionID {
		t.Errorf("sessão retomada = %q", sess.ID())
	}
	options := sess.ConfigOptions()
	if len(options) != 1 || options[0].CurrentValue != "modelo-b" {
		t.Errorf("estado da sessão retomada: %+v", options)
	}
}

func TestChamadaCruaAlcancaMetodosNaoTipados(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)

	// A saída crua existe porque o seletor legado e as extensões do Cursor não
	// são tipados pelo SDK (AEP-0084 D2).
	raw, err := client.Call(ctx, "session/set_config_option", map[string]any{
		"sessionId": fakeSessionID,
		"configId":  "model",
		"value":     "modelo-b",
	})
	if err != nil {
		t.Fatalf("chamada crua falhou: %v", err)
	}
	if !strings.Contains(string(raw), "modelo-b") {
		t.Errorf("resposta crua inesperada: %s", raw)
	}
}

func TestClienteFechadoRecusaNovasChamadas(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)
	if _, err := client.Capabilities(ctx); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if _, err := client.NewSession(ctx, t.TempDir()); !errors.Is(err, ErrClosed) {
		t.Fatalf("esperava cliente encerrado, obtive: %v", err)
	}
}

func TestAgenteInexistenteFalhaComErroAcionavelERespeitaOBackoff(t *testing.T) {
	ctx := testContext(t)
	client, err := New(Config{Command: "binario-de-agente-que-nao-existe", WorkDir: t.TempDir()}, nil)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	_, first := client.Capabilities(ctx)
	if first == nil || !strings.Contains(first.Error(), "binario-de-agente-que-nao-existe") {
		t.Fatalf("erro deveria nomear o comando: %v", first)
	}

	// A segunda tentativa imediata não pode tentar spawnar de novo: um binário
	// quebrado viraria uma tempestade de processos.
	_, second := client.Capabilities(ctx)
	if second == nil || !strings.Contains(second.Error(), "nova tentativa em") {
		t.Fatalf("esperava espera de backoff, obtive: %v", second)
	}
	// A espera precisa aparecer legível para quem lê o erro na tela.
	if !strings.Contains(second.Error(), "nova tentativa em 1s") {
		t.Errorf("a espera deveria estar formatada como duração: %v", second)
	}
}

// Fechar o app enquanto o agente ainda está subindo não pode esperar o
// handshake: o usuário mandou sair, e meio minuto de janela travada é o que ele
// sentiria.
func TestFecharOClienteNaoEsperaOHandshakeDoAgente(t *testing.T) {
	client, err := New(fakeConfig(t, scriptStall), nil)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}

	falhou := make(chan error, 1)
	go func() {
		_, err := client.Capabilities(context.Background())
		falhou <- err
	}()

	// Tempo para o processo subir e o handshake ficar pendurado; sem isso o
	// teste passaria sem nunca ter havido um dial em andamento.
	time.Sleep(500 * time.Millisecond)

	inicio := time.Now()
	if err := client.Close(); err != nil {
		t.Fatalf("fechar cliente: %v", err)
	}
	if levou := time.Since(inicio); levou > 5*time.Second {
		t.Fatalf("Close esperou o handshake: levou %s", levou)
	}

	select {
	case err := <-falhou:
		if err == nil {
			t.Fatal("o handshake pendurado deveria ter sido interrompido")
		}
	case <-time.After(testTimeout):
		t.Fatal("a chamada presa no handshake nunca voltou")
	}
}

// Um binário quebrado é algo que o usuário tenta de novo até acertar a
// configuração, e cada tentativa não pode deixar uma goroutine para trás.
func TestSpawnQueFalhaNaoDeixaGoroutineParaTras(t *testing.T) {
	tentar := func() {
		client, err := New(Config{Command: "binario-de-agente-que-nao-existe", WorkDir: t.TempDir()}, nil)
		if err != nil {
			t.Fatalf("criar cliente: %v", err)
		}
		_, _ = client.Capabilities(context.Background())
		_ = client.Close()
	}

	tentar()
	time.Sleep(200 * time.Millisecond)
	antes := runtime.NumGoroutine()

	const tentativas = 20
	for range tentativas {
		tentar()
	}

	// As goroutines mortas somem em algum momento, não na hora; o teto de folga
	// absorve o ruído sem absorver um vazamento por tentativa.
	prazo := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > antes+5 && time.Now().Before(prazo) {
		time.Sleep(50 * time.Millisecond)
	}
	if depois := runtime.NumGoroutine(); depois > antes+5 {
		t.Fatalf("goroutines vazaram: %d antes, %d depois de %d spawns falhos", antes, depois, tentativas)
	}
}

func TestFecharSessaoEncerraNoAgenteERecusaNovosTurnos(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, ctx)

	if err := sess.Close(ctx); err != nil {
		t.Fatalf("encerrar sessão: %v", err)
	}
	// Fechar duas vezes é o caso comum de conversa excluída durante o
	// encerramento do app, e não pode virar erro.
	if err := sess.Close(ctx); err != nil {
		t.Fatalf("encerrar de novo: %v", err)
	}

	_, err := sess.Prompt(ctx, []Content{TextContent("oi")}, func(Update) {})
	if !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("sessão encerrada deveria recusar turno: %v", err)
	}
	// Nem turno, nem troca de modelo: quem ainda segura a sessão não fala mais
	// com o agente sobre ela.
	if _, err := sess.SetConfigOption(ctx, "model", "modelo-b"); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("sessão encerrada deveria recusar troca de modelo: %v", err)
	}
}

// O que sai no Update é de quem escuta; o que fica na sessão é da sessão. Sem
// essa separação, quem guardasse o Update trocaria o modelo da conversa sem
// passar pelo agente — e a troca de modo, que mexe nas opções no lugar,
// alteraria por baixo dos panos um Update já entregue.
func TestOEstadoDaSessaoNaoCompartilhaMemoriaComQuemEscuta(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, ctx)

	var recebidas []ConfigOption
	_, err := sess.Prompt(ctx, []Content{TextContent("oi")}, func(update Update) {
		if update.Kind == UpdateConfigOptions {
			recebidas = update.ConfigOptions
		}
	})
	if err != nil {
		t.Fatalf("turno falhou: %v", err)
	}
	if len(recebidas) == 0 {
		t.Fatal("o turno não entregou opções de configuração")
	}

	for i := range recebidas {
		recebidas[i].CurrentValue = "adulterado"
		for j := range recebidas[i].Values {
			recebidas[i].Values[j].Value = "adulterado"
		}
	}
	got := findOption(sess.ConfigOptions(), "model")
	if got == nil || got.CurrentValue != "modelo-b" {
		t.Fatalf("mexer no que foi entregue alterou o estado da sessão: %+v", sess.ConfigOptions())
	}
	for _, value := range got.Values {
		if value.Value == "adulterado" {
			t.Errorf("a lista de modelos da sessão foi alterada por fora: %+v", got.Values)
		}
	}
}

// O identificador da conversa é chave de roteamento, não texto: vai e volta
// exatamente como o agente mandou. Limpá-lo aqui — como se faz com o da
// ferramenta, que é só para exibir — mandaria de volta um identificador que o
// agente não reconhece e faria toda atualização dele cair no vazio. Quem for
// exibir isso limpa na hora de exibir (AEP-0084 D11).
func TestIdentificadorSujoDaConversaContinuaRoteando(t *testing.T) {
	ctx := testContext(t)
	handler := &scriptedHandler{
		custom: func(context.Context, string, json.RawMessage) (any, bool) {
			return map[string]any{"answer": "sim"}, true
		},
	}
	client := newTestClient(t, scriptIDSujo, handler)
	sess := startSession(t, client, ctx)

	if sess.ID() != fakeDirtySessionID {
		t.Fatalf("identificador da conversa = %q, esperado o que o agente mandou %q",
			sess.ID(), fakeDirtySessionID)
	}

	col := &collector{}
	stop, err := sess.Prompt(ctx, []Content{TextContent("liste os arquivos")}, col.sink)
	if err != nil {
		t.Fatalf("turno falhou: %v", err)
	}
	if stop != StopEndTurn {
		t.Fatalf("stopReason = %q, esperado %q", stop, StopEndTurn)
	}
	// A pergunta do agente vem assinada com o mesmo identificador sujo:
	// procurar a conversa por uma versão aparada dele faria a pergunta morrer
	// como "conversa encerrada" sem nunca chegar a quem decide.
	if got := col.textOfKind(UpdateText); !strings.Contains(got, `"answer":"sim"`) {
		t.Errorf("a resposta da extensão não voltou ao agente: %q", got)
	}

	// Cru no protocolo, escapado no texto: um identificador com quebra de linha
	// dentro de uma mensagem de erro forja linha de log e atrapalha quem lê.
	sess.(*session).closeWait = 200 * time.Millisecond
	err = sess.Close(ctx)
	if err == nil {
		t.Fatal("o agente não respondeu à despedida e isso deveria virar erro")
	}
	if strings.ContainsAny(err.Error(), "\n\r") {
		t.Errorf("o erro carrega quebra de linha vinda do agente: %q", err.Error())
	}
	if !strings.Contains(err.Error(), `sess-falsa\n1`) {
		t.Errorf("o erro deveria citar o identificador escapado, obtive: %s", err.Error())
	}
}

// Um sink que encerra a própria conversa ao ver um evento não pode travar: ele
// roda dentro da entrega, e esperar a entrega terminar seria esperar por si
// mesmo. É o caminho de "erro fatal no meio da resposta, feche isso aqui".
func TestSinkPodeEncerrarAPropriaConversaSemTravar(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTeimoso, nil)
	sess := startSession(t, client, ctx)
	sess.(*session).closeWait = 200 * time.Millisecond

	fechou := make(chan error, 1)
	var umaVez sync.Once
	voltou := make(chan error, 1)
	go func() {
		_, err := sess.Prompt(ctx, []Content{TextContent("comece")}, func(Update) {
			umaVez.Do(func() { fechou <- sess.Close(context.Background()) })
		})
		voltou <- err
	}()

	select {
	case <-fechou:
	case <-time.After(5 * time.Second):
		t.Fatal("o sink travou ao encerrar a própria conversa")
	}
	select {
	case err := <-voltou:
		if !errors.Is(err, ErrSessionClosed) {
			t.Fatalf("esperava conversa encerrada, obtive: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o turno não voltou depois do encerramento")
	}
}

// Excluir a conversa no meio de um turno em andamento devolve quem chamou na
// hora, mesmo que o agente ignore o pedido de parada e nunca responda.
func TestExcluirAConversaDevolveOTurnoEmAndamento(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptStuck, nil)
	sess := startSession(t, client, ctx)
	sess.(*session).closeWait = 200 * time.Millisecond

	voltou := make(chan error, 1)
	go func() {
		// Contexto vivo de propósito: quem chamou não desistiu, a conversa é
		// que deixou de existir.
		_, err := sess.Prompt(ctx, []Content{TextContent("comece algo demorado")}, nil)
		voltou <- err
	}()
	time.Sleep(200 * time.Millisecond)

	go func() { _ = sess.Close(ctx) }()

	select {
	case err := <-voltou:
		if !errors.Is(err, ErrSessionClosed) {
			t.Fatalf("esperava conversa encerrada, obtive: %v", err)
		}
		var falha *PromptError
		if errors.As(err, &falha) && !falha.Accepted {
			t.Error("o turno saiu para o agente e deveria constar como aceito")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o turno ficou preso numa conversa que já não existe")
	}
}

// Excluir a conversa enquanto o turno cancelado espera a confirmação do agente
// não pode deixar quem chamou preso pelo resto do prazo: a conversa acabou, e é
// isso que ele precisa ouvir.
func TestExcluirAConversaAcordaOTurnoQueEsperavaAConfirmacao(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptStuck, nil)
	sess := startSession(t, client, ctx)
	// Prazo longo de propósito: se o encerramento não acordar quem espera, o
	// teste estoura no limite de tempo em vez de passar por acaso.
	sess.(*session).grace = 20 * time.Second
	sess.(*session).closeWait = 200 * time.Millisecond

	turno, desistir := context.WithCancel(ctx)
	voltou := make(chan error, 1)
	go func() {
		_, err := sess.Prompt(turno, []Content{TextContent("comece algo demorado")}, nil)
		voltou <- err
	}()
	time.Sleep(200 * time.Millisecond)
	desistir()
	time.Sleep(200 * time.Millisecond)

	go func() { _ = sess.Close(ctx) }()

	select {
	case err := <-voltou:
		if !errors.Is(err, ErrSessionClosed) {
			t.Fatalf("esperava conversa encerrada, obtive: %v", err)
		}
		// O turno chegou a sair: repetir por conta própria mexeria no disco de
		// novo.
		var falha *PromptError
		if errors.As(err, &falha) && !falha.Accepted {
			t.Error("o turno saiu para o agente e deveria constar como aceito")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o turno ficou preso esperando por uma conversa que já não existe")
	}
}

// O sink pertence à chamada de Prompt. Durante o prazo de graça a entrega
// continua, porque é ali que o agente conta o que alcançou fazer; depois que o
// turno volta, um agente teimoso que segue falando não escreve mais na tela.
// Sem essa fronteira, o rastro de um turno morto acabaria no meio do próximo.
func TestDepoisQueOTurnoVoltaOAgenteTeimosoNaoEscreveMaisNaTela(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTeimoso, nil)
	sess := startSession(t, client, ctx)
	sess.(*session).grace = 300 * time.Millisecond

	col := &collector{}
	turno, desistir := context.WithCancel(ctx)
	voltou := make(chan error, 1)
	go func() {
		_, err := sess.Prompt(turno, []Content{TextContent("comece algo demorado")}, col.sink)
		voltou <- err
	}()
	time.Sleep(200 * time.Millisecond)
	desistir()

	select {
	case err := <-voltou:
		if !errors.Is(err, ErrCancelNotConfirmed) {
			t.Fatalf("esperava cancelamento não confirmado, obtive: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("o turno cancelado nunca voltou")
	}
	// Até aqui o agente falou, e isso tinha de chegar: é o que ele fez antes de
	// parar.
	if col.textOfKind(UpdateText) == "" {
		t.Fatal("o rastro do agente durante o cancelamento não chegou")
	}

	entregue := len(col.snapshot())
	time.Sleep(300 * time.Millisecond)
	if agora := len(col.snapshot()); agora != entregue {
		t.Errorf("o agente continuou escrevendo depois do turno: %d atualizações viraram %d", entregue, agora)
	}
}

// Handler que nunca decide não pode pendurar o agente: o contexto que o SDK
// entrega para o pedido não tem prazo nenhum, então o teto é nosso. O agente
// precisa receber um desfecho que o método aceita e seguir a vida.
func TestHandlerQueNuncaDecideNaoPenduraOAgente(t *testing.T) {
	ctx := testContext(t)
	preso := make(chan struct{})
	t.Cleanup(func() { close(preso) })

	handler := &scriptedHandler{
		decide: func(hctx context.Context, _ PermissionRequest) PermissionOutcome {
			<-preso
			return PermissionOutcome{OptionID: "allow-once"}
		},
	}
	client := newTestClient(t, scriptPermission, handler)
	sess := startSession(t, client, ctx)
	sess.(*session).cn.backstop = 300 * time.Millisecond

	col := &collector{}
	stop, err := sess.Prompt(ctx, []Content{TextContent("rode algo")}, col.sink)
	if err != nil {
		t.Fatalf("turno falhou: %v", err)
	}
	if stop != StopEndTurn {
		t.Fatalf("stopReason = %q, esperado %q", stop, StopEndTurn)
	}
	// Sem decisão, negamos: é o desfecho que o pedido de permissão aceita e que
	// deixa o agente seguir, em vez de um erro que derrubaria o turno.
	if got := col.textOfKind(UpdateText); !strings.Contains(got, "decisão: reject-once") {
		t.Errorf("o agente não recebeu a recusa por falta de decisão: %q", got)
	}
}

// Extensão bloqueante sem decisão precisa receber o "não" que ela entende, e
// não um erro de cliente: o agente que ouve erro conclui que o app quebrou, em
// vez de seguir sem a resposta (AEP-0084 D9). Quem monta esse desfecho é quem
// implementa a extensão — o transporte não conhece o formato de cada método e
// uma resposta de forma errada correria o risco de virar decisão de verdade.
func TestExtensaoSemDecisaoRecebeODesfechoQueElaEntende(t *testing.T) {
	ctx := testContext(t)
	preso := make(chan struct{})
	t.Cleanup(func() { close(preso) })

	pedidos := make(chan string, 4)
	handler := &scriptedHandler{
		custom: func(context.Context, string, json.RawMessage) (any, bool) {
			<-preso
			return map[string]any{"answer": "tarde demais"}, true
		},
		fallback: func(method string) (any, bool) {
			pedidos <- method
			return map[string]any{"skipped": true}, true
		},
	}
	client := newTestClient(t, scriptCustom, handler)
	sess := startSession(t, client, ctx)
	sess.(*session).cn.backstop = 300 * time.Millisecond

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("pergunte algo")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	if got := col.textOfKind(UpdateText); !strings.Contains(got, `"skipped":true`) {
		t.Errorf("o agente não recebeu o desfecho da extensão: %q", got)
	}
	close(pedidos)
	var metodos []string
	for method := range pedidos {
		metodos = append(metodos, method)
	}
	if len(metodos) != 1 || metodos[0] != "cursor/ask_question" {
		t.Errorf("o desfecho foi pedido para: %v", metodos)
	}
}

// Sem desfecho de quem implementa a extensão, sobra o erro interno: é a única
// resposta honesta, porque inventar o formato arriscaria o agente ler um "não"
// como decisão de verdade.
func TestExtensaoSemDesfechoConhecidoRecebeErroInterno(t *testing.T) {
	ctx := testContext(t)
	preso := make(chan struct{})
	t.Cleanup(func() { close(preso) })

	handler := &scriptedHandler{
		custom: func(context.Context, string, json.RawMessage) (any, bool) {
			<-preso
			return nil, true
		},
	}
	client := newTestClient(t, scriptCustom, handler)
	sess := startSession(t, client, ctx)
	sess.(*session).cn.backstop = 300 * time.Millisecond

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("pergunte algo")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	esperado := fmt.Sprintf("erro:%d", sdk.NewInternalError(nil).Code)
	if got := col.textOfKind(UpdateText); !strings.Contains(got, esperado) {
		t.Errorf("esperava %s, obtive: %q", esperado, got)
	}
}

// canoCheio é o agente vivo que parou de ler a entrada: toda escrita para ele
// fica pendurada, que é o caso em que nem contexto salva — o SDK confere o
// cancelamento antes de escrever e depois entra num Write que não olha mais
// nada.
type canoCheio struct{ liberado chan struct{} }

func (w canoCheio) Write(p []byte) (int, error) {
	<-w.liberado
	return len(p), nil
}

func (w canoCheio) Read([]byte) (int, error) {
	<-w.liberado
	return 0, io.EOF
}

// O encerramento do app não pode ficar preso na escrita para um agente que
// parou de ler a entrada.
func TestFecharASessaoNaoTravaQuandoOAgenteParaDeLerAEntrada(t *testing.T) {
	cano := canoCheio{liberado: make(chan struct{})}
	t.Cleanup(func() { close(cano.liberado) })

	derrubado := make(chan struct{})
	cn := &conn{
		handler:  denyAll{},
		caps:     Capabilities{CloseSession: true},
		kill:     sync.OnceFunc(func() { close(derrubado) }),
		sessions: map[string]*session{},
		dead:     make(chan struct{}),
	}
	cn.rpc = sdk.NewConnection(cn.handleInbound, cano, cano)
	sess := cn.registerSession("sess-travada", t.TempDir(), nil)
	sess.closeWait = 200 * time.Millisecond

	done := make(chan error, 1)
	go func() { done <- sess.Close(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("encerrar deveria acusar que o agente não confirmou")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close ficou preso escrevendo para um agente que não lê")
	}

	// Um processo que não aceita mais entrada não serve às outras conversas:
	// cada chamada delas ficaria pendurada até o contexto de quem chamou
	// morrer. Derrubar é o que devolve o app ao caminho de recuperação.
	if !cn.isDead() {
		t.Error("a conexão continuou viva depois de o agente parar de aceitar pedidos")
	}
	select {
	case <-derrubado:
	default:
		t.Error("o processo do agente não foi derrubado")
	}
}

// Agente vivo que não responde à despedida não pode prender quem fechou a
// conversa: no encerramento do app isso seria uma janela que não fecha.
func TestFecharASessaoNaoEsperaParaSempreUmAgenteQueNaoResponde(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptStuck, nil)
	sess := startSession(t, client, ctx)
	sess.(*session).closeWait = 300 * time.Millisecond

	done := make(chan error, 1)
	go func() { done <- sess.Close(ctx) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("encerrar deveria acusar que o agente não confirmou")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close ficou preso esperando o agente")
	}

	// Independentemente da resposta do agente, a sessão morreu para o app.
	if _, err := sess.Prompt(ctx, []Content{TextContent("oi")}, func(Update) {}); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("sessão encerrada deveria recusar turno: %v", err)
	}
}

// Encerrar a conversa tira da fila quem esperava a vez, mesmo que o turno preso
// no agente nunca volte e o contexto de quem espera não tenha prazo.
func TestFecharASessaoTiraDaFilaQuemEsperavaAVez(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptStuck, nil)
	sess := startSession(t, client, ctx)

	go func() {
		_, _ = sess.Prompt(ctx, []Content{TextContent("comece algo demorado")}, nil)
	}()
	time.Sleep(200 * time.Millisecond)

	naFila := make(chan error, 1)
	go func() {
		_, err := sess.Prompt(context.Background(), []Content{TextContent("e agora?")}, nil)
		naFila <- err
	}()
	time.Sleep(200 * time.Millisecond)

	// Encerrar em paralelo e cobrar a fila logo em seguida: soltar quem espera é
	// decisão local e não pode ficar atrás da confirmação de um agente surdo.
	go func() { _ = sess.Close(ctx) }()

	select {
	case err := <-naFila:
		if !errors.Is(err, ErrSessionClosed) {
			t.Fatalf("esperava sessão encerrada, obtive: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("quem esperava na fila ficou preso na conversa excluída")
	}
}

func TestFalhaDeTurnoSempreDizSeOAgenteAceitou(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptTurn, nil)
	sess := startSession(t, client, ctx)

	// Turno sem conteúdo nem chega a sair: quem retentar pode fazê-lo à vontade.
	_, err := sess.Prompt(ctx, nil, func(Update) {})
	var promptErr *PromptError
	if !errors.As(err, &promptErr) {
		t.Fatalf("todo erro de turno deveria ser *PromptError: %v", err)
	}
	if promptErr.Accepted {
		t.Error("turno que nem foi enviado não pode contar como aceito")
	}
}

func TestVariavelDoProviderVenceAHerdadaDoApp(t *testing.T) {
	ctx := testContext(t)
	// O agente herda o ambiente do app para achar PATH e credenciais, mas o que
	// a configuração do provider define precisa prevalecer. Aqui o app diz um
	// roteiro e a configuração diz outro.
	t.Setenv(fakeScriptEnv, scriptTurn)

	client := newTestClient(t, scriptEcho, nil)
	sess := startSession(t, client, ctx)

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("oi")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}
	if got := col.textOfKind(UpdateText); !strings.Contains(got, `"sessionId"`) {
		t.Errorf("o agente rodou o roteiro herdado do app em vez do configurado: %q", got)
	}
}

func TestConfiguracaoSemComandoEhRejeitada(t *testing.T) {
	if _, err := New(Config{}, nil); err == nil {
		t.Fatal("cliente sem comando deveria falhar na criação")
	}
}

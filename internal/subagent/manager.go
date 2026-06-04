package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"assistente/internal/database"
	"assistente/internal/eventctx"
	"assistente/internal/messaging"
	"assistente/internal/toolinvocations"
)

// maxResultSummary limita o tamanho do result_summary persistido para evitar
// crescimento excessivo da tabela sub_agent_runs.
const maxResultSummary = 16 * 1024

// completion carrega o resultado entregue pelo callback in-process do notifier.
type completion struct {
	response           string
	assistantMessageID string
}

// outcome representa o desfecho da espera por um run.
type outcome struct {
	status             string
	summary            string
	assistantMessageID string
	errMsg             string
}

// activeRun rastreia um run em andamento para permitir cancelamento.
type activeRun struct {
	childConversationID string
	cancelCh            chan struct{}
	cancelOnce          sync.Once
}

func (a *activeRun) cancel() {
	a.cancelOnce.Do(func() { close(a.cancelCh) })
}

// Manager orquestra runs de sub-agente (AEP-0068). É a única porta de entrada
// para criar/continuar sub-conversas; reusa o pipeline oficial via SendFunc e
// detecta conclusão por callback in-process (ResponseNotifier).
type Manager struct {
	repo       Repository
	notifier   *messaging.ResponseNotifier
	send       SendFunc
	delivery   ParentDelivery
	cancelStrm func(conversationID string)
	now        func() time.Time

	mu          sync.Mutex
	active      map[string]*activeRun // runID -> run ativo
	activeConvs map[string]struct{}   // childConversationID com run ativo (fail-fast resume)
	parentMu    map[string]*sync.Mutex
}

// ManagerConfig agrupa as dependências do Manager.
type ManagerConfig struct {
	Repo     Repository
	Notifier *messaging.ResponseNotifier
	Send     SendFunc
	// Delivery entrega o aviso de conclusão de runs em background ao pai
	// (auto-wake). Pode ser nil (ex.: contextos sem pai); então o aviso é
	// apenas persistido no run.
	Delivery ParentDelivery
	// CancelStream cancela o streaming LLM de uma conversa (barge-in). Usado
	// para interromper um sub-agente em background. Pode ser nil em testes.
	CancelStream func(conversationID string)
	// Now é injetável para testes; nil usa time.Now.
	Now func() time.Time
}

// NewManager cria um Manager com as dependências injetadas.
func NewManager(cfg ManagerConfig) *Manager {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Manager{
		repo:       cfg.Repo,
		notifier:   cfg.Notifier,
		send:       cfg.Send,
		delivery:   cfg.Delivery,
		cancelStrm: cfg.CancelStream,
		now:         now,
		active:      make(map[string]*activeRun),
		activeConvs: make(map[string]struct{}),
		parentMu:    make(map[string]*sync.Mutex),
	}
}

// Run executa um sub-agente. Com Background=false é síncrono (Fase 1): espera a
// conclusão e devolve o resultado. Com Background=true (Fase 2) retorna o handle
// imediatamente e executa em goroutine, entregando o aviso de conclusão ao pai.
//
// Retorna o RunResult sempre que o run foi criado; error não-nil é reservado a
// falhas de pré-condição (validação, sem dono no ctx, falha ao criar a
// sub-conversa/run).
func (m *Manager) Run(ctx context.Context, p RunParams) (RunResult, error) {
	if m == nil || m.send == nil || m.repo == nil || m.notifier == nil {
		return RunResult{}, fmt.Errorf("subagent manager não configurado")
	}
	if strings.TrimSpace(p.Prompt) == "" {
		return RunResult{}, fmt.Errorf("prompt é obrigatório para iniciar um sub-agente")
	}
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return RunResult{}, err
	}

	// 1. Resolve a sub-conversa: cria nova ou reusa uma existente
	// (resume/clear — Fase 3), preservando o contexto da sub-conversa.
	childConvID, turnIndex, err := m.resolveChildConversation(ctx, p)
	if err != nil {
		return RunResult{}, err
	}

	// Fail-fast (AEP-0068): impede DOIS runs concorrentes na MESMA sub-conversa.
	// O ResponseNotifier é indexado só por conversationID e Notify()/Cancel()
	// atuam sobre TODOS os callbacks pendentes da conversa; dois runs simultâneos
	// no mesmo childConversationID se confundiriam (a 1ª conclusão concluiria
	// ambos, ou um cancelamento atingiria o run errado). A reserva é feita ANTES
	// do Create e liberada quando o run deixa de estar ativo (unregisterActive).
	if err := m.reserveConversation(childConvID); err != nil {
		return RunResult{}, err
	}

	// 2. Persiste o run (queued) com proveniência (anti-runaway, AEP-0067/0001).
	prov := deriveProvenance(ctx, "")
	chainHistoryJSON := encodeChainHistory(prov.ChainHistory)
	run := &database.SubAgentRun{
		UserID:               userID,
		ParentConversationID: p.ParentConversationID,
		ParentTurnID:         p.ParentTurnID,
		ChildConversationID:  childConvID,
		TurnIndex:            turnIndex,
		Status:               database.SubAgentRunStatusQueued,
		Background:           p.Background,
		ChainID:              prov.ChainID,
		ChainHistory:         chainHistoryJSON,
	}
	if err := m.repo.Create(ctx, run); err != nil {
		m.releaseConversation(childConvID)
		return RunResult{}, fmt.Errorf("erro ao registrar run de sub-agente: %w", err)
	}

	result := RunResult{ConversationID: childConvID, RunID: run.ID, Status: run.Status}

	// 3. Registra o callback de conclusão e o run ativo ANTES de enviar.
	done := make(chan completion, 1)
	m.notifier.Register(childConvID, messaging.ResponseCallback{
		Channel: Source,
		TraceID: run.ID,
		ChatID:  childConvID,
		Callback: func(response, assistantMessageID string) {
			select {
			case done <- completion{response: response, assistantMessageID: assistantMessageID}:
			default:
			}
		},
	})
	ar := &activeRun{childConversationID: childConvID, cancelCh: make(chan struct{})}
	m.registerActive(run.ID, ar)

	// 4. Marca running e dispara o envio pelo pipeline oficial.
	//    Persiste a transição num ctx desacoplado de cancelamento (como em
	//    finish): o estado não pode ficar preso em queued enquanto o loop roda.
	//    Se nem isso persistir, aborta ANTES de enviar para não deixar trabalho
	//    órfão e reporta a falha (não descarta o erro silenciosamente).
	startedAt := m.now()
	run.Status = database.SubAgentRunStatusRunning
	run.StartedAt = &startedAt
	result.Status = run.Status
	if err := m.repo.Update(context.WithoutCancel(ctx), run); err != nil {
		m.notifier.Cancel(childConvID)
		m.unregisterActive(run.ID)
		log.Printf("[Subagent] erro ao marcar run %s (conversa %s) como running: %v", run.ID, childConvID, err)
		o := outcome{status: database.SubAgentRunStatusFailed, errMsg: fmt.Sprintf("erro ao persistir estado running: %v", err)}
		finished := m.finish(ctx, run, &result, o)
		if p.Background {
			m.deliver(ctx, run, p)
		}
		return finished, nil
	}

	// Encadeia as sub-invocações da sub-conversa à invocação da tool `subagent`.
	// Contexto de envio cancelável e ESCOPADO a este run: o pipeline oficial
	// dispara o agentic loop da sub-conversa em background sob um ctx derivado
	// deste (SendMessageUseCase.Execute → context.WithCancel). Ao concluir
	// (timeout, cancel, ctx.Done() ou sucesso) cancelamos cancelSend para não
	// deixar o loop rodando/custando após o desfecho. cancelSend é privado deste
	// run (sem cancelamento cruzado) e nunca cancela o ctx-pai.
	//   - Síncrono: o base herda o ctx da tool (cancelamento do pai propaga); o
	//     cancel é disparado via defer ao retornar (logo após a espera inline).
	//   - Background: o base é desacoplado (WithoutCancel) para o run sobreviver
	//     ao fim do turno-pai; o cancel é disparado na goroutine ao concluir a
	//     espera (timeout/cancel via wait), alcançando o loop daquele run.
	sendBase := toolinvocations.WithParentInvocationID(ctx, p.ParentInvocationID)
	if p.Background {
		sendBase = context.WithoutCancel(sendBase)
	}
	sendCtx, cancelSend := context.WithCancel(sendBase)
	if _, err := m.send(sendCtx, SendParams{
		ConversationID: childConvID,
		Prompt:         p.Prompt,
		Media:          p.Media,
		ProfileSlug:    p.ProfileSlug,
		Model:          p.Model,
		Source:         Source,
	}); err != nil {
		cancelSend()
		m.notifier.Cancel(childConvID)
		m.unregisterActive(run.ID)
		// Um cancel/timeout do ctx (usuário cancela a tool, deadline do executor)
		// pode se manifestar como erro de send. Classificamos pelo estado do
		// ctx/erro para não reportar cancelled/timed_out como failed (telemetria).
		// Vale para o caminho síncrono e o de background (send é compartilhado
		// antes do branch p.Background).
		status, errMsg := classifySendError(ctx, err)
		o := outcome{status: status, errMsg: errMsg}
		finished := m.finish(ctx, run, &result, o)
		if p.Background {
			m.deliver(ctx, run, p)
		}
		return finished, nil
	}

	if p.Background {
		// Background real: goroutine com ctx desacoplado de cancelamento, mas
		// preservando o userID (WithoutCancel mantém valores do ctx — AEP-0052).
		bgCtx := context.WithoutCancel(ctx)
		// Cópia local do RunResult para a goroutine: o Run retorna `result`
		// (handle imediato) logo abaixo, então a goroutine NÃO pode escrever no
		// mesmo struct (corrida detectada por -race).
		bgResult := result
		go func() {
			// Cancela o loop da sub-conversa ao concluir a espera (timeout/cancel/
			// sucesso), interrompendo trabalho em background — escopado a este run.
			defer cancelSend()
			o := m.wait(bgCtx, childConvID, done, ar, p.Timeout, true)
			m.finish(bgCtx, run, &bgResult, o)
			m.unregisterActive(run.ID)
			m.deliver(bgCtx, run, p)
		}()
		return result, nil
	}

	// Síncrono (Fase 1): espera inline e cancela o loop ao concluir.
	defer cancelSend()
	o := m.wait(ctx, childConvID, done, ar, p.Timeout, false)
	m.unregisterActive(run.ID)
	return m.finish(ctx, run, &result, o), nil
}

// resolveChildConversation decide a sub-conversa alvo do run e o índice do turno:
//   - sem ConversationID → cria uma sub-conversa nova (Kind=subagent).
//   - com ConversationID → reusa a existente (resume), validando posse e Kind;
//     com Clear=true, reseta histórico e resumo antes do envio (AEP-0068 F3).
//
// O TurnIndex é incremental por sub-conversa (último run + 1), preservando a
// telemetria de turnos mesmo após um clear.
func (m *Manager) resolveChildConversation(ctx context.Context, p RunParams) (string, int, error) {
	if strings.TrimSpace(p.ConversationID) == "" {
		title := strings.TrimSpace(p.Title)
		if title == "" {
			title = deriveTitle(p.Prompt)
		}
		conv, err := database.CreateSubAgentConversationWithContext(ctx, title, p.ParentConversationID)
		if err != nil {
			return "", 0, fmt.Errorf("erro ao criar sub-conversa: %w", err)
		}
		return conv.ID, 0, nil
	}

	// Resume: a sub-conversa precisa existir, pertencer ao usuário (escopo
	// AEP-0052, garantido por GetConversationInfoWithContext) e ser de sub-agente.
	conv, err := database.GetConversationInfoWithContext(ctx, p.ConversationID)
	if err != nil {
		return "", 0, fmt.Errorf("sub-conversa não encontrada ou sem acesso: %w", err)
	}
	if conv.Kind != database.ConversationKindSubagent {
		return "", 0, fmt.Errorf("conversa %s não é uma sub-conversa de sub-agente", p.ConversationID)
	}

	if p.Clear {
		// clear = reset + envio: limpa histórico e resumo, depois envia o novo
		// prompt na mesma chamada (a continuidade de contexto é descartada).
		if err := database.DeleteAllMessagesWithContext(ctx, conv.ID); err != nil {
			return "", 0, fmt.Errorf("erro ao limpar histórico da sub-conversa: %w", err)
		}
		if err := database.UpdateConversationSummaryWithContext(ctx, conv.ID, "", ""); err != nil {
			return "", 0, fmt.Errorf("erro ao limpar resumo da sub-conversa: %w", err)
		}
	}

	turnIndex := 0
	if latest, lerr := m.repo.GetLatestByChildConversation(ctx, conv.ID); lerr == nil && latest != nil {
		turnIndex = latest.TurnIndex + 1
	}
	return conv.ID, turnIndex, nil
}

// resolveTimeout escolhe o timeout efetivo conforme o modo. Um Timeout explícito
// (>0) é respeitado em ambos os modos. Sem Timeout: o síncrono usa
// DefaultSyncTimeout (curto, cabe no executor de tools) e o background usa
// DefaultBackgroundTimeout (backstop anti-runaway bem maior — AEP-0068). Assim,
// background:true sem Timeout NÃO expira nos 5min do síncrono, mas mantém o
// backstop de "timeout por run" exigido pelos Riscos da AEP.
func resolveTimeout(timeout time.Duration, background bool) time.Duration {
	if timeout > 0 {
		return timeout
	}
	if background {
		return DefaultBackgroundTimeout
	}
	return DefaultSyncTimeout
}

// wait bloqueia até a conclusão, timeout, cancelamento explícito ou
// cancelamento do ctx. O background distingue o default de timeout (ver
// resolveTimeout): background:true sem Timeout usa o backstop longo, não o
// DefaultSyncTimeout.
func (m *Manager) wait(ctx context.Context, childConvID string, done chan completion, ar *activeRun, timeout time.Duration, background bool) outcome {
	timeout = resolveTimeout(timeout, background)
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	// O select de Go não prioriza cases: se `done` ficar pronto quase junto com
	// cancelCh/timer/ctx, o desfecho poderia virar cancelado/timed_out mesmo
	// havendo resposta. Por isso, antes de persistir cancel/timeout, re-checamos
	// `done` de forma não-bloqueante e damos prioridade à conclusão bem-sucedida.
	select {
	case c := <-done:
		return successOutcome(c)
	case <-ar.cancelCh:
		if c, ok := pollDone(done); ok {
			return successOutcome(c)
		}
		m.notifier.Cancel(childConvID)
		return outcome{status: database.SubAgentRunStatusCancelled, errMsg: "cancelado"}
	case <-timer.C:
		if c, ok := pollDone(done); ok {
			return successOutcome(c)
		}
		m.notifier.Cancel(childConvID)
		return outcome{status: database.SubAgentRunStatusTimedOut, errMsg: "tempo limite excedido aguardando o sub-agente"}
	case <-ctx.Done():
		if c, ok := pollDone(done); ok {
			return successOutcome(c)
		}
		m.notifier.Cancel(childConvID)
		// Distingue timed_out (deadline do executor) de cancelled (cancelamento
		// explícito), em vez de tratar todo ctx.Done() como cancelled.
		status, errMsg := classifyCtxErr(ctx.Err())
		return outcome{status: status, errMsg: errMsg}
	}
}

// pollDone lê `done` de forma não-bloqueante, retornando a conclusão se já
// estiver disponível. Usado para dar prioridade à resposta bem-sucedida quando
// `done` e cancel/timer/ctx ficam prontos quase simultaneamente (evita
// cancelled/timed_out indevido). Caminhos síncrono e background compartilham
// este helper via wait().
func pollDone(done chan completion) (completion, bool) {
	select {
	case c := <-done:
		return c, true
	default:
		return completion{}, false
	}
}

// classifySendError mapeia um erro de m.send para o status correto do run
// conforme o enum do AEP-0068 ("Retorno da tool"). Um cancelamento/timeout do
// contexto que apareça como erro de envio deve refletir cancelled/timed_out, e
// não failed. Prioriza a classificação do próprio erro (errors.Is) e, em
// seguida, o estado do ctx. A mensagem real do erro é sempre preservada.
func classifySendError(ctx context.Context, err error) (status, errMsg string) {
	switch {
	case errors.Is(err, context.Canceled):
		return database.SubAgentRunStatusCancelled, err.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return database.SubAgentRunStatusTimedOut, err.Error()
	case ctx.Err() != nil:
		return classifyCtxErr(ctx.Err())
	default:
		return database.SubAgentRunStatusFailed, err.Error()
	}
}

// classifyCtxErr mapeia o erro de um contexto encerrado para o status do run:
// context.DeadlineExceeded → timed_out; cancelamento (ou qualquer outro) →
// cancelled. Compartilhado pelo caminho ctx.Done() (wait) e pela classificação
// de erro de send, mantendo a distinção timed_out vs cancelled.
func classifyCtxErr(err error) (status, errMsg string) {
	if errors.Is(err, context.DeadlineExceeded) {
		return database.SubAgentRunStatusTimedOut, err.Error()
	}
	return database.SubAgentRunStatusCancelled, err.Error()
}

// successOutcome monta o desfecho de sucesso a partir da conclusão recebida.
func successOutcome(c completion) outcome {
	return outcome{status: database.SubAgentRunStatusSucceeded, summary: c.response, assistantMessageID: c.assistantMessageID}
}

// Status retorna o estado atual de um run (prompt omitido). Resolve por run_id
// quando informado; senão pelo run mais recente da sub-conversa.
func (m *Manager) Status(ctx context.Context, conversationID, runID string) (StatusResult, error) {
	run, err := m.resolveRun(ctx, conversationID, runID)
	if err != nil {
		return StatusResult{}, err
	}
	return StatusResult{
		ConversationID:     run.ChildConversationID,
		RunID:              run.ID,
		Status:             run.Status,
		ResultSummary:      run.ResultSummary,
		AssistantMessageID: run.AssistantMessageID,
		Error:              run.Error,
	}, nil
}

// Cancel cancela um run em andamento. Se havia run ativo, retorna
// Cancelled=true com Status=cancelled; se o run já era terminal/inexistente, é
// no-op (Cancelled=false) retornando o status real (AEP-0068).
func (m *Manager) Cancel(ctx context.Context, conversationID, runID string) (CancelResult, error) {
	run, err := m.resolveRun(ctx, conversationID, runID)
	if err != nil {
		return CancelResult{}, err
	}
	res := CancelResult{ConversationID: run.ChildConversationID, RunID: run.ID, Status: run.Status}

	ar := m.lookupActive(run.ID)
	if ar == nil || isTerminal(run.Status) {
		// No-op: nada ativo para cancelar.
		res.Cancelled = false
		res.Message = "nenhum run ativo para cancelar; status atual mantido"
		return res, nil
	}

	// Interrompe o streaming do sub-agente e sinaliza o waiter.
	if m.cancelStrm != nil {
		m.cancelStrm(run.ChildConversationID)
	}
	m.notifier.Cancel(run.ChildConversationID)
	ar.cancel()

	res.Status = database.SubAgentRunStatusCancelled
	res.Cancelled = true
	res.Message = "run cancelado"
	return res, nil
}

// resolveRun encontra o run alvo por run_id (validando que pertence à conversa)
// ou pelo run mais recente da conversa.
func (m *Manager) resolveRun(ctx context.Context, conversationID, runID string) (*database.SubAgentRun, error) {
	if strings.TrimSpace(runID) != "" {
		run, err := m.repo.Get(ctx, runID)
		if err != nil {
			return nil, fmt.Errorf("run não encontrado: %w", err)
		}
		if strings.TrimSpace(conversationID) != "" && run.ChildConversationID != conversationID {
			return nil, fmt.Errorf("run %s não pertence à conversa %s", runID, conversationID)
		}
		return run, nil
	}
	if strings.TrimSpace(conversationID) == "" {
		return nil, fmt.Errorf("conversation_id ou run_id é obrigatório")
	}
	run, err := m.repo.GetLatestByChildConversation(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("nenhum run encontrado para a conversa %s: %w", conversationID, err)
	}
	return run, nil
}

// finish atualiza o run com o desfecho e preenche o RunResult.
func (m *Manager) finish(ctx context.Context, run *database.SubAgentRun, result *RunResult, o outcome) RunResult {
	completedAt := m.now()
	run.Status = o.status
	run.ResultSummary = truncate(o.summary, maxResultSummary)
	run.AssistantMessageID = o.assistantMessageID
	run.Error = o.errMsg
	run.CompletedAt = &completedAt

	persistCtx := context.WithoutCancel(ctx)
	if err := m.repo.Update(persistCtx, run); err != nil {
		// Best-effort: não propaga (o desfecho do run já foi decidido), mas
		// loga para não falhar silenciosamente — evita run preso sem sinal.
		log.Printf("[Subagent] erro (best-effort) ao persistir estado final do run %s (status=%s): %v", run.ID, o.status, err)
	}

	result.Status = o.status
	result.ResultSummary = run.ResultSummary
	result.AssistantMessageID = o.assistantMessageID
	result.Error = o.errMsg
	return *result
}

// deliver entrega o aviso de conclusão ao pai (auto-wake), serializado por
// conversa-pai e idempotente por run_id.
func (m *Manager) deliver(ctx context.Context, run *database.SubAgentRun, p RunParams) {
	if m.delivery == nil || strings.TrimSpace(run.ParentConversationID) == "" {
		return
	}

	// Fila serializada por conversa-pai (evita corrida no StreamingManager).
	lock := m.parentLock(run.ParentConversationID)
	lock.Lock()
	defer lock.Unlock()

	persistCtx := context.WithoutCancel(ctx)

	// Idempotência por run_id: recarrega o run e não entrega duas vezes.
	if fresh, err := m.repo.Get(persistCtx, run.ID); err == nil && fresh != nil {
		if fresh.DeliveredAt != nil {
			return
		}
		run = fresh
	}

	notice := ParentNotice{
		ParentConversationID: run.ParentConversationID,
		ParentTurnID:         run.ParentTurnID,
		RunID:                run.ID,
		ChildConversationID:  run.ChildConversationID,
		Status:               run.Status,
		Summary:              run.ResultSummary,
		AssistantMessageID:   run.AssistantMessageID,
		Error:                run.Error,
	}

	// Proveniência propagada para o auto-wake (backstop anti-runaway).
	prov := deriveProvenance(ctx, run.ChainID)
	prov.ChainHistory = appendChain(prov.ChainHistory, run.ID)
	dctx := eventctx.With(persistCtx, prov)

	if err := m.delivery.Deliver(dctx, notice); err != nil {
		// Não marca DeliveredAt em falha — permite reentrega futura.
		return
	}

	now := m.now()
	run.DeliveredAt = &now
	_ = m.repo.Update(persistCtx, run)
}

// ---- registro de runs ativos / locks por pai ----

// reserveConversation marca a sub-conversa como tendo um run ativo. Retorna
// erro (fail-fast) se já existir um run ativo para o mesmo childConversationID,
// evitando dois runs concorrentes na mesma sub-conversa (limitação do
// ResponseNotifier, indexado por conversationID — AEP-0068).
func (m *Manager) reserveConversation(childConversationID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, busy := m.activeConvs[childConversationID]; busy {
		return fmt.Errorf("já existe um sub-agente ativo nesta sub-conversa (%s); aguarde a conclusão ou cancele o run atual antes de continuar", childConversationID)
	}
	m.activeConvs[childConversationID] = struct{}{}
	return nil
}

// releaseConversation libera a reserva de uma sub-conversa. Usado no caminho de
// falha antes de o run virar ativo (o caminho normal libera em unregisterActive).
func (m *Manager) releaseConversation(childConversationID string) {
	m.mu.Lock()
	delete(m.activeConvs, childConversationID)
	m.mu.Unlock()
}

func (m *Manager) registerActive(runID string, ar *activeRun) {
	m.mu.Lock()
	m.active[runID] = ar
	m.mu.Unlock()
}

func (m *Manager) unregisterActive(runID string) {
	m.mu.Lock()
	if ar := m.active[runID]; ar != nil {
		delete(m.activeConvs, ar.childConversationID)
	}
	delete(m.active, runID)
	m.mu.Unlock()
}

func (m *Manager) lookupActive(runID string) *activeRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active[runID]
}

func (m *Manager) parentLock(parentConversationID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	lock, ok := m.parentMu[parentConversationID]
	if !ok {
		lock = &sync.Mutex{}
		m.parentMu[parentConversationID] = lock
	}
	return lock
}

// ---- proveniência / utilitários ----

func deriveProvenance(ctx context.Context, existingChainID string) eventctx.Provenance {
	prov, ok := eventctx.From(ctx)
	if !ok {
		prov = eventctx.Provenance{Source: Source}
	}
	if strings.TrimSpace(prov.ChainID) == "" {
		if strings.TrimSpace(existingChainID) != "" {
			prov.ChainID = existingChainID
		}
	}
	return prov
}

func appendChain(history []string, id string) []string {
	if id == "" {
		return history
	}
	return append(history, id)
}

func encodeChainHistory(history []string) string {
	if len(history) == 0 {
		return ""
	}
	b, err := json.Marshal(history)
	if err != nil {
		return ""
	}
	return string(b)
}

// deriveTitle gera um título curto a partir do prompt.
func deriveTitle(prompt string) string {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return "Sub-agente"
	}
	if idx := strings.IndexAny(p, "\n\r"); idx >= 0 {
		p = strings.TrimSpace(p[:idx])
	}
	const maxTitle = 60
	if utf8.RuneCountInString(p) > maxTitle {
		runes := []rune(p)
		p = strings.TrimSpace(string(runes[:maxTitle])) + "…"
	}
	return p
}

// truncate corta uma string em no máximo n bytes, respeitando limites de runas.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

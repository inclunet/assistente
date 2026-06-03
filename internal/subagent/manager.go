package subagent

import (
	"context"
	"encoding/json"
	"fmt"
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

// DefaultMaxChainDepth é o teto de profundidade de cadeia (backstop anti-runaway,
// AEP-0068). Espelha jobs.DefaultMaxChainDepth para coerência entre os dois
// caminhos que compartilham proveniência via eventctx. Não é o gate de
// profundidade (esse é o profile) — é só um circuit-breaker contra recursão
// descontrolada (sub-agente acordando o pai que delega de novo, etc.).
const DefaultMaxChainDepth = 10

// DefaultMaxConcurrentPerUser é o teto global de sub-agentes simultâneos por
// usuário (AEP-0068 F5). Protege contra custo/concorrência descontrolados
// quando muitos runs em background são disparados ao mesmo tempo.
const DefaultMaxConcurrentPerUser = 4

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
	repo          Repository
	notifier      *messaging.ResponseNotifier
	send          SendFunc
	delivery      ParentDelivery
	lister        ConversationLister
	cancelStrm    func(conversationID string)
	now           func() time.Time
	maxChainDepth int
	maxConcurrent int

	mu           sync.Mutex
	active       map[string]*activeRun // runID -> run ativo
	activeByUser map[string]int        // userID -> nº de runs ativos (teto de concorrência)
	parentMu     map[string]*sync.Mutex
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
	// Lister fornece metadados/custo das sub-conversas para a UI (AEP-0068 F5).
	// Pode ser nil (ex.: testes que não exercitam a listagem).
	Lister ConversationLister
	// CancelStream cancela o streaming LLM de uma conversa (barge-in). Usado
	// para interromper um sub-agente em background. Pode ser nil em testes.
	CancelStream func(conversationID string)
	// Now é injetável para testes; nil usa time.Now.
	Now func() time.Time
	// MaxChainDepth é o teto de profundidade de cadeia (backstop anti-runaway).
	// <=0 usa DefaultMaxChainDepth.
	MaxChainDepth int
	// MaxConcurrentPerUser é o teto global de sub-agentes simultâneos por
	// usuário. <=0 usa DefaultMaxConcurrentPerUser.
	MaxConcurrentPerUser int
}

// NewManager cria um Manager com as dependências injetadas.
func NewManager(cfg ManagerConfig) *Manager {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	maxChainDepth := cfg.MaxChainDepth
	if maxChainDepth <= 0 {
		maxChainDepth = DefaultMaxChainDepth
	}
	maxConcurrent := cfg.MaxConcurrentPerUser
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrentPerUser
	}
	return &Manager{
		repo:          cfg.Repo,
		notifier:      cfg.Notifier,
		send:          cfg.Send,
		delivery:      cfg.Delivery,
		lister:        cfg.Lister,
		cancelStrm:    cfg.CancelStream,
		now:           now,
		maxChainDepth: maxChainDepth,
		maxConcurrent: maxConcurrent,
		active:        make(map[string]*activeRun),
		activeByUser:  make(map[string]int),
		parentMu:      make(map[string]*sync.Mutex),
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

	// Backstop anti-runaway (AEP-0068/0067): a cadeia de proveniência
	// compartilhada com jobs limita a profundidade de delegação. Verifica ANTES
	// de criar qualquer sub-conversa/run para não deixar lixo.
	prov := deriveProvenance(ctx, "")
	if len(prov.ChainHistory) >= m.maxChainDepth {
		return RunResult{}, fmt.Errorf("limite de profundidade de cadeia atingido (%d): possível runaway de sub-agentes/jobs", m.maxChainDepth)
	}

	// Teto global de concorrência por usuário (AEP-0068 F5): reserva uma vaga
	// antes de criar qualquer sub-conversa/run; cada caminho terminal libera.
	if err := m.acquireSlot(userID); err != nil {
		return RunResult{}, err
	}

	// 1. Resolve a sub-conversa: cria nova ou reusa uma existente
	// (resume/clear — Fase 3), preservando o contexto da sub-conversa.
	childConvID, turnIndex, err := m.resolveChildConversation(ctx, p)
	if err != nil {
		m.releaseSlot(userID)
		return RunResult{}, err
	}

	// 2. Persiste o run (queued) com proveniência (anti-runaway, AEP-0067/0001).
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
		m.releaseSlot(userID)
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
	startedAt := m.now()
	run.Status = database.SubAgentRunStatusRunning
	run.StartedAt = &startedAt
	_ = m.repo.Update(ctx, run)
	result.Status = run.Status

	sendCtx := toolinvocations.WithParentInvocationID(ctx, p.ParentInvocationID)
	if _, err := m.send(sendCtx, SendParams{
		ConversationID: childConvID,
		Prompt:         p.Prompt,
		Media:          p.Media,
		ProfileSlug:    p.ProfileSlug,
		Model:          p.Model,
		Source:         Source,
	}); err != nil {
		m.notifier.Cancel(childConvID)
		m.unregisterActive(run.ID)
		o := outcome{status: database.SubAgentRunStatusFailed, errMsg: err.Error()}
		finished := m.finish(ctx, run, &result, o)
		if p.Background {
			m.deliver(ctx, run, p)
		}
		m.releaseSlot(userID)
		return finished, nil
	}

	if p.Background {
		// Background real: goroutine com ctx desacoplado de cancelamento, mas
		// preservando o userID (WithoutCancel mantém valores do ctx — AEP-0052).
		bgCtx := context.WithoutCancel(ctx)
		go func() {
			o := m.wait(bgCtx, childConvID, done, ar, p.Timeout)
			m.finish(bgCtx, run, &result, o)
			m.unregisterActive(run.ID)
			m.deliver(bgCtx, run, p)
			m.releaseSlot(userID)
		}()
		return result, nil
	}

	// Síncrono (Fase 1): espera inline.
	o := m.wait(ctx, childConvID, done, ar, p.Timeout)
	m.unregisterActive(run.ID)
	defer m.releaseSlot(userID)
	return m.finish(ctx, run, &result, o), nil
}

// acquireSlot reserva uma vaga de concorrência para o usuário; falha se o teto
// já foi atingido. releaseSlot devolve a vaga (idempotente em zero).
func (m *Manager) acquireSlot(userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeByUser[userID] >= m.maxConcurrent {
		return fmt.Errorf("limite de %d sub-agentes simultâneos atingido para este usuário; aguarde a conclusão de um run ou cancele um existente", m.maxConcurrent)
	}
	m.activeByUser[userID]++
	return nil
}

func (m *Manager) releaseSlot(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeByUser[userID] > 0 {
		m.activeByUser[userID]--
	}
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

// wait bloqueia até a conclusão, timeout, cancelamento explícito ou
// cancelamento do ctx.
func (m *Manager) wait(ctx context.Context, childConvID string, done chan completion, ar *activeRun, timeout time.Duration) outcome {
	if timeout <= 0 {
		timeout = DefaultSyncTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case c := <-done:
		return outcome{status: database.SubAgentRunStatusSucceeded, summary: c.response, assistantMessageID: c.assistantMessageID}
	case <-ar.cancelCh:
		m.notifier.Cancel(childConvID)
		return outcome{status: database.SubAgentRunStatusCancelled, errMsg: "cancelado"}
	case <-timer.C:
		m.notifier.Cancel(childConvID)
		return outcome{status: database.SubAgentRunStatusTimedOut, errMsg: "tempo limite excedido aguardando o sub-agente"}
	case <-ctx.Done():
		m.notifier.Cancel(childConvID)
		return outcome{status: database.SubAgentRunStatusCancelled, errMsg: ctx.Err().Error()}
	}
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

// ReconcileOrphans marca como failed os runs deixados em queued/running por um
// encerramento abrupto do app (AEP-0068 F4). Após um restart não há goroutine
// viva para concluí-los nem entrada no mapa `active`, então qualquer run não
// terminal persistido é órfão. Espelha a reconciliação de jobs no startup.
// Retorna quantos runs foram reconciliados.
func (m *Manager) ReconcileOrphans(ctx context.Context) (int64, error) {
	if m == nil || m.repo == nil {
		return 0, nil
	}
	return m.repo.ReconcileOrphans(ctx, m.now())
}

// ListSubConversations retorna a visão das sub-conversas do usuário para a UI
// (AEP-0068 F5): identidade, vínculo com o pai, status do run mais recente,
// contagem de runs e custo agregado (tokens). Combina os metadados da conversa
// (via Lister) com os runs persistidos (via Repository), tudo escopado por
// usuário.
func (m *Manager) ListSubConversations(ctx context.Context) ([]SubConversationSummary, error) {
	if m == nil || m.repo == nil || m.lister == nil {
		return []SubConversationSummary{}, nil
	}
	metas, err := m.lister.ListSubAgentConversations(ctx)
	if err != nil {
		return nil, err
	}
	runs, err := m.repo.ListByUser(ctx)
	if err != nil {
		return nil, err
	}

	// runs vem ordenado do mais recente para o mais antigo: o primeiro visto por
	// child_conversation_id é o mais recente.
	type runAgg struct {
		latest *database.SubAgentRun
		count  int
	}
	byConv := make(map[string]*runAgg, len(metas))
	for i := range runs {
		r := &runs[i]
		agg, ok := byConv[r.ChildConversationID]
		if !ok {
			agg = &runAgg{}
			byConv[r.ChildConversationID] = agg
		}
		if agg.latest == nil {
			agg.latest = r
		}
		agg.count++
	}

	out := make([]SubConversationSummary, 0, len(metas))
	for _, meta := range metas {
		s := SubConversationSummary{
			ConversationID:       meta.ConversationID,
			Title:                meta.Title,
			ParentConversationID: meta.ParentConversationID,
			MessageCount:         meta.MessageCount,
			PromptTokens:         meta.PromptTokens,
			CompletionTokens:     meta.CompletionTokens,
			TotalTokens:          meta.TotalTokens,
			CreatedAt:            meta.CreatedAt,
			UpdatedAt:            meta.UpdatedAt,
		}
		if agg, ok := byConv[meta.ConversationID]; ok && agg.latest != nil {
			s.LatestStatus = agg.latest.Status
			s.RunCount = agg.count
			s.Background = agg.latest.Background
			s.LastError = agg.latest.Error
		}
		out = append(out, s)
	}
	return out, nil
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
	_ = m.repo.Update(persistCtx, run)

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

func (m *Manager) registerActive(runID string, ar *activeRun) {
	m.mu.Lock()
	m.active[runID] = ar
	m.mu.Unlock()
}

func (m *Manager) unregisterActive(runID string) {
	m.mu.Lock()
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

package subagent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"assistente/internal/database"
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

// Manager orquestra runs de sub-agente (AEP-0068). É a única porta de entrada
// para criar/continuar sub-conversas; reusa o pipeline oficial via SendFunc e
// detecta conclusão por callback in-process (ResponseNotifier).
type Manager struct {
	repo     Repository
	notifier *messaging.ResponseNotifier
	send     SendFunc
	now      func() time.Time
}

// ManagerConfig agrupa as dependências do Manager.
type ManagerConfig struct {
	Repo     Repository
	Notifier *messaging.ResponseNotifier
	Send     SendFunc
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
		repo:     cfg.Repo,
		notifier: cfg.Notifier,
		send:     cfg.Send,
		now:      now,
	}
}

// Run executa um sub-agente de forma SÍNCRONA (background:false, Fase 1):
// cria a sub-conversa, dispara o envio pelo pipeline oficial e espera a
// conclusão (callback in-process) ou timeout/cancelamento.
//
// Retorna o RunResult sempre que o run foi criado (mesmo em falha/timeout), com
// Status refletindo o desfecho. O error não-nil é reservado para falhas de
// pré-condição (validação, sem dono no ctx, falha ao criar a sub-conversa).
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

	// 1. Cria a sub-conversa (Kind=subagent, vinculada ao pai).
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = deriveTitle(p.Prompt)
	}
	conv, err := database.CreateSubAgentConversationWithContext(ctx, title, p.ParentConversationID)
	if err != nil {
		return RunResult{}, fmt.Errorf("erro ao criar sub-conversa: %w", err)
	}

	// 2. Persiste o run (queued).
	run := &database.SubAgentRun{
		UserID:               userID,
		ParentConversationID: p.ParentConversationID,
		ParentTurnID:         p.ParentTurnID,
		ChildConversationID:  conv.ID,
		TurnIndex:            0,
		Status:               database.SubAgentRunStatusQueued,
		Background:           false,
	}
	if err := m.repo.Create(ctx, run); err != nil {
		return RunResult{}, fmt.Errorf("erro ao registrar run de sub-agente: %w", err)
	}

	result := RunResult{ConversationID: conv.ID, RunID: run.ID}

	// 3. Registra o callback de conclusão ANTES de enviar (evita corrida com
	//    um agentic loop muito rápido).
	done := make(chan completion, 1)
	m.notifier.Register(conv.ID, messaging.ResponseCallback{
		Channel: Source,
		TraceID: run.ID,
		ChatID:  conv.ID,
		Callback: func(response, assistantMessageID string) {
			select {
			case done <- completion{response: response, assistantMessageID: assistantMessageID}:
			default:
			}
		},
	})

	// 4. Marca running e dispara o envio pelo pipeline oficial.
	//    Persiste a transição num ctx desacoplado de cancelamento (como em
	//    finish): o estado não pode ficar preso em queued enquanto o loop roda.
	//    Se nem isso persistir, aborta ANTES de enviar para não deixar trabalho
	//    órfão e reporta a falha (não descarta o erro silenciosamente).
	startedAt := m.now()
	run.Status = database.SubAgentRunStatusRunning
	run.StartedAt = &startedAt
	if err := m.repo.Update(context.WithoutCancel(ctx), run); err != nil {
		m.notifier.Cancel(conv.ID)
		log.Printf("[Subagent] erro ao marcar run %s (conversa %s) como running: %v", run.ID, conv.ID, err)
		return m.finish(ctx, run, &result, database.SubAgentRunStatusFailed, "", "", fmt.Sprintf("erro ao persistir estado running: %v", err)), nil
	}

	// Encadeia as sub-invocações da sub-conversa à invocação da tool `subagent`.
	sendCtx := toolinvocations.WithParentInvocationID(ctx, p.ParentInvocationID)
	if _, err := m.send(sendCtx, SendParams{
		ConversationID: conv.ID,
		Prompt:         p.Prompt,
		Media:          p.Media,
		ProfileSlug:    p.ProfileSlug,
		Model:          p.Model,
		Source:         Source,
	}); err != nil {
		m.notifier.Cancel(conv.ID)
		// Um cancel/timeout do ctx (usuário cancela a tool, deadline do executor)
		// pode se manifestar como erro de send. Classificamos pelo estado do
		// ctx/erro para não reportar cancelled/timed_out como failed (telemetria).
		status, errMsg := classifySendError(ctx, err)
		return m.finish(ctx, run, &result, status, "", "", errMsg), nil
	}

	// 5. Espera conclusão / timeout / cancelamento.
	status, summary, assistantMessageID, errMsg := m.waitForCompletion(ctx, conv.ID, done, p.Timeout)
	return m.finish(ctx, run, &result, status, summary, assistantMessageID, errMsg), nil
}

// waitForCompletion bloqueia até a conclusão (`done`), timeout ou cancelamento
// do ctx, e devolve o desfecho (status + dados). É extraído do Run para ser
// testável de forma determinística (injetando `done`).
//
// O select de Go não prioriza cases: se `done` ficar pronto quase junto com
// timer.C/ctx.Done(), o desfecho poderia virar timed_out/cancelled mesmo
// havendo resposta. Por isso, antes de persistir timeout/cancel, re-checamos
// `done` de forma não-bloqueante (pollDone) e priorizamos succeeded.
func (m *Manager) waitForCompletion(ctx context.Context, childConvID string, done chan completion, timeout time.Duration) (status, summary, assistantMessageID, errMsg string) {
	if timeout <= 0 {
		timeout = DefaultSyncTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case c := <-done:
		return database.SubAgentRunStatusSucceeded, c.response, c.assistantMessageID, ""
	case <-timer.C:
		if c, ok := pollDone(done); ok {
			return database.SubAgentRunStatusSucceeded, c.response, c.assistantMessageID, ""
		}
		m.notifier.Cancel(childConvID)
		return database.SubAgentRunStatusTimedOut, "", "", "tempo limite excedido aguardando o sub-agente"
	case <-ctx.Done():
		if c, ok := pollDone(done); ok {
			return database.SubAgentRunStatusSucceeded, c.response, c.assistantMessageID, ""
		}
		m.notifier.Cancel(childConvID)
		// Distingue timed_out (deadline do executor) de cancelled (cancelamento
		// explícito), em vez de tratar todo ctx.Done() como cancelled.
		status, errMsg := classifyCtxErr(ctx.Err())
		return status, "", "", errMsg
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
// cancelled. Compartilhado pelo caminho ctx.Done() (waitForCompletion) e pela
// classificação de erro de send, mantendo a distinção timed_out vs cancelled.
func classifyCtxErr(err error) (status, errMsg string) {
	if errors.Is(err, context.DeadlineExceeded) {
		return database.SubAgentRunStatusTimedOut, err.Error()
	}
	return database.SubAgentRunStatusCancelled, err.Error()
}

// pollDone lê `done` de forma não-bloqueante, retornando a conclusão se já
// estiver disponível. Usado para dar prioridade à resposta bem-sucedida quando
// `done` e timer/ctx ficam prontos quase simultaneamente (evita timed_out/
// cancelled indevido). Na F2+ o caminho de background compartilha o mesmo
// helper via wait().
func pollDone(done chan completion) (completion, bool) {
	select {
	case c := <-done:
		return c, true
	default:
		return completion{}, false
	}
}

// finish atualiza o run com o desfecho e preenche o RunResult. A persistência
// usa um ctx desacoplado de cancelamento para registrar o estado final mesmo
// quando o run foi cancelado.
func (m *Manager) finish(ctx context.Context, run *database.SubAgentRun, result *RunResult, status, summary, assistantMessageID, errMsg string) RunResult {
	completedAt := m.now()
	run.Status = status
	run.ResultSummary = truncate(summary, maxResultSummary)
	run.AssistantMessageID = assistantMessageID
	run.Error = errMsg
	run.CompletedAt = &completedAt

	persistCtx := context.WithoutCancel(ctx)
	if err := m.repo.Update(persistCtx, run); err != nil {
		// Best-effort: não propaga (o desfecho do run já foi decidido), mas
		// loga para não falhar silenciosamente — evita run preso sem sinal.
		log.Printf("[Subagent] erro (best-effort) ao persistir estado final do run %s (status=%s): %v", run.ID, status, err)
	}

	result.Status = status
	result.ResultSummary = run.ResultSummary
	result.AssistantMessageID = assistantMessageID
	result.Error = errMsg
	return *result
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

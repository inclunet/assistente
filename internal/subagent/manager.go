package subagent

import (
	"context"
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
		return m.finish(ctx, run, &result, database.SubAgentRunStatusFailed, "", "", err.Error()), nil
	}

	// 5. Espera conclusão / timeout / cancelamento.
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultSyncTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case c := <-done:
		return m.finish(ctx, run, &result, database.SubAgentRunStatusSucceeded, c.response, c.assistantMessageID, ""), nil
	case <-timer.C:
		m.notifier.Cancel(conv.ID)
		return m.finish(ctx, run, &result, database.SubAgentRunStatusTimedOut, "", "", "tempo limite excedido aguardando o sub-agente"), nil
	case <-ctx.Done():
		m.notifier.Cancel(conv.ID)
		return m.finish(ctx, run, &result, database.SubAgentRunStatusCancelled, "", "", ctx.Err().Error()), nil
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
	_ = m.repo.Update(persistCtx, run)

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

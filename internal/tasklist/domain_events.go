package tasklist

import (
	"context"
	"log"
	"time"

	"assistente/internal/database"
	"assistente/internal/eventctx"
)

// DomainEventSink publica eventos de domínio normalizados no EventBus de jobs
// (AEP-0067). É satisfeito estruturalmente por *jobs.Manager. Mantido como
// interface local para não acoplar o pacote tasklist ao pacote jobs.
type DomainEventSink interface {
	PublishDomainEvent(ctx context.Context, name string, payload map[string]any) error
	// HasDomainListener informa se há job inscrito no evento; permite pular a
	// montagem de payloads (e queries extras) quando ninguém escuta.
	HasDomainListener(name string) bool
}

// wantsDomain indica se vale a pena montar o payload e publicar o evento.
// Falso quando não há sink ou nenhum job inscrito — preservando custo ~zero.
func (s *Service) wantsDomain(name string) bool {
	return s.domain != nil && s.domain.HasDomainListener(name)
}

// publishDomain injeta proveniência (do ctx do run, quando houver) e publica.
func (s *Service) publishDomain(ctx context.Context, name string, payload map[string]any) {
	if s.domain == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	applyProvenance(ctx, payload)
	// Política best-effort: eventos de domínio não devem quebrar a mutação que
	// os originou. Mas só chegamos aqui quando há listener (wantsDomain), então
	// uma falha (event bus não inicializado, ctx sem user_id, etc.) silenciaria
	// um evento que alguém espera — logamos para não esconder o problema.
	if err := s.domain.PublishDomainEvent(ctx, name, payload); err != nil {
		log.Printf("[tasklist] falha ao publicar evento de domínio %q: %v", name, err)
	}
}

// applyProvenance copia a proveniência carimbada no ctx (por um run de job) para
// o payload. Sem carimbo, o PublishDomainEvent assume _source="user".
func applyProvenance(ctx context.Context, payload map[string]any) {
	p, ok := eventctx.From(ctx)
	if !ok {
		return
	}
	if p.Source != "" {
		payload["_source"] = p.Source
	}
	if p.SourceJobID != "" {
		payload["_source_job_id"] = p.SourceJobID
	}
	if p.ChainID != "" {
		payload["_chain_id"] = p.ChainID
	}
	if len(p.ChainHistory) > 0 {
		payload["_chain_history"] = p.ChainHistory
	}
}

// taskListSlug resolve o slug da lista (best-effort) para enriquecer payloads.
func (s *Service) taskListSlug(ctx context.Context, taskListID string) string {
	if taskListID == "" {
		return ""
	}
	tl, err := s.store.GetTaskList(ctx, taskListID)
	if err != nil || tl == nil {
		return ""
	}
	return tl.Slug
}

// listRefForTask resolve (task_list_id, slug) a partir do id de um card (best-effort).
func (s *Service) listRefForTask(ctx context.Context, taskID string) (string, string) {
	if taskID == "" {
		return "", ""
	}
	t, err := s.store.GetTask(ctx, taskID)
	if err != nil || t == nil {
		return "", ""
	}
	return t.TaskListID, s.taskListSlug(ctx, t.TaskListID)
}

func rfc3339OrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func parentOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// taskEventPayload monta o payload-base de um evento de card a partir do snapshot
// mais recente disponível (best-effort): prefere `task` (recarregado pós-mutação) e
// cai para `fallback` (snapshot pré-mutação) quando a recarga falha. Devolve nil
// quando nenhum snapshot está disponível, sinalizando para não publicar — evitando
// deref de task nil ao resolver task_list_id/slug.
func (s *Service) taskEventPayload(ctx context.Context, task, fallback *database.Task) map[string]any {
	t := task
	if t == nil {
		t = fallback
	}
	if t == nil {
		return nil
	}
	return taskPayload(t, s.taskListSlug(ctx, t.TaskListID))
}

// taskPayload normaliza os campos-base de um card (AEP-0067).
func taskPayload(t *database.Task, slug string) map[string]any {
	if t == nil {
		return map[string]any{}
	}
	return map[string]any{
		"task_id":        t.ID,
		"task_list_id":   t.TaskListID,
		"task_list_slug": slug,
		"code":           t.Code,
		"title":          t.Title,
		"status_id":      t.StatusID,
		"parent_id":      parentOrEmpty(t.ParentID),
		"assignee_id":    t.AssigneeID,
		"assignee_name":  t.AssigneeName,
		"creator_id":     t.CreatorID,
		"due_date":        rfc3339OrEmpty(t.DueDate),
		"completed_at":    rfc3339OrEmpty(t.CompletedAt),
		"link":            t.Link,
		"conversation_id": parentOrEmpty(t.ConversationID),
	}
}

// changedTaskFields retorna os nomes (snake_case, alinhados ao payload) dos
// campos-base que mudaram entre old e new. Best-effort: vazio se old for nil.
func changedTaskFields(old, updated *database.Task) []string {
	changed := []string{}
	if old == nil || updated == nil {
		return changed
	}
	if old.Title != updated.Title {
		changed = append(changed, "title")
	}
	if old.Description != updated.Description {
		changed = append(changed, "description")
	}
	if old.Code != updated.Code {
		changed = append(changed, "code")
	}
	if old.Link != updated.Link {
		changed = append(changed, "link")
	}
	if old.StatusID != updated.StatusID {
		changed = append(changed, "status_id")
	}
	if parentOrEmpty(old.ParentID) != parentOrEmpty(updated.ParentID) {
		changed = append(changed, "parent_id")
	}
	if old.AssigneeID != updated.AssigneeID {
		changed = append(changed, "assignee_id")
	}
	if old.AssigneeName != updated.AssigneeName {
		changed = append(changed, "assignee_name")
	}
	if old.CreatorID != updated.CreatorID {
		changed = append(changed, "creator_id")
	}
	if parentOrEmpty(old.ConversationID) != parentOrEmpty(updated.ConversationID) {
		changed = append(changed, "conversation_id")
	}
	return changed
}

// notePayload normaliza os campos-base de uma nota (AEP-0067).
func notePayload(n *database.TaskNote, taskListID, slug string) map[string]any {
	if n == nil {
		return map[string]any{}
	}
	return map[string]any{
		"note_id":        n.ID,
		"task_id":        n.TaskID,
		"task_list_id":   taskListID,
		"task_list_slug": slug,
		"note_type":      int(n.Type),
		"source":         n.ExternalSource,
		"external_id":    n.ExternalID,
		"author_id":      n.AuthorID,
	}
}

// workflowEventPayload normaliza os campos-base de um workflow (AEP-0067).
func (s *Service) workflowEventPayload(ctx context.Context, taskListID string, wf *database.TaskListWorkflow) map[string]any {
	p := map[string]any{
		"task_list_id":   taskListID,
		"task_list_slug": s.taskListSlug(ctx, taskListID),
	}
	if wf != nil {
		p["initial_status_id"] = wf.InitialStatusID
	}
	return p
}

// listPayload normaliza os campos-base de uma lista (AEP-0067).
func listPayload(tl *database.TaskList) map[string]any {
	if tl == nil {
		return map[string]any{}
	}
	return map[string]any{
		"task_list_id":    tl.ID,
		"task_list_slug":  tl.Slug,
		"title":           tl.Title,
		"conversation_id": parentOrEmpty(tl.ConversationID),
	}
}

// listEventPayload monta o payload-base de um evento de lista preferindo o
// snapshot recarregado (tl). Se a recarga falhou (tl==nil), garante ao menos
// task_list_id a partir do id conhecido — alinhando com os fallbacks de note/task
// e mantendo o contrato do evento mesmo no pior caso. O slug é best-effort
// (pode ficar vazio se a lista não puder ser recarregada).
func (s *Service) listEventPayload(ctx context.Context, tl *database.TaskList, id string) map[string]any {
	if tl != nil {
		return listPayload(tl)
	}
	return map[string]any{
		"task_list_id":   id,
		"task_list_slug": s.taskListSlug(ctx, id),
		"title":          "",
	}
}

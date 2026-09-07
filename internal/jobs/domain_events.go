package jobs

import "sync"

// Catálogo estático de eventos de domínio (AEP-0067).
//
// O EventBus só conhece nomes de eventos derivados dos jobs existentes
// (ListKnownEvents). Para que os eventos de domínio publicados pelas superfícies
// (a tasklist é o primeiro produtor) apareçam no picker do JobBuilder mesmo sem
// nenhum job referenciando-os, mantemos aqui o catálogo canônico + um schema
// estático usado como fallback por InferEventSchema.
//
// Extensível para superfícies futuras (chat.*, workspace.tab.*, terminal.*,
// editor.*) — basta adicionar entradas em newDomainEventCatalog().

// provenanceFields são os campos de proveniência anti-loop presentes em todo
// evento publicado via Manager.PublishDomainEvent.
func provenanceFields() map[string]any {
	return map[string]any{
		"_source":        "user", // "user" (humano) | "job" (automação)
		"_source_job_id": "",
		"_chain_id":      "",
		"_chain_history": []string{},
	}
}

func taskEventBase() map[string]any {
	return map[string]any{
		"task_id":        "",
		"task_list_id":   "",
		"task_list_slug": "",
		"code":           "", // ≡ external id
		"title":          "",
		"status_id":      0,
		"parent_id":      "",
		"assignee_id":    "",
		"assignee_name":  "",
		"creator_id":     "",
		"due_date":       "",
		"completed_at":   "",
		"link":           "",
	}
}

func noteEventBase() map[string]any {
	return map[string]any{
		"note_id":        "",
		"task_id":        "",
		"task_list_id":   "",
		"task_list_slug": "",
		"note_type":      0, // 1 interna / 2 cliente / 3 agente / 4 sistema
		"source":         "",
		"external_id":    "",
		"author_id":      "",
	}
}

func listEventBase() map[string]any {
	return map[string]any{
		"task_list_id":   "",
		"task_list_slug": "",
		"title":          "",
	}
}

func workflowEventBase() map[string]any {
	return map[string]any{
		"task_list_id":      "",
		"task_list_slug":    "",
		"initial_status_id": 0,
	}
}

type domainEvent struct {
	name   string
	schema map[string]any
}

// mergeSchema combina base + extras + proveniência num único schema de exemplo.
func mergeSchema(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra)+4)
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	for k, v := range provenanceFields() {
		out[k] = v
	}
	return out
}

// O catálogo é conceitualmente estático: construímos uma vez (sync.Once) e
// indexamos por nome para lookup O(1), evitando reconstruir os schemas
// (map[string]any) a cada chamada de KnownDomainEvents/DomainEventSchema.
var (
	domainCatalogOnce sync.Once
	cachedCatalog     []domainEvent
	cachedNames       []string
	cachedSchemas     map[string]map[string]any
)

func buildDomainCatalog() {
	cachedCatalog = newDomainEventCatalog()
	cachedNames = make([]string, len(cachedCatalog))
	cachedSchemas = make(map[string]map[string]any, len(cachedCatalog))
	for i, ev := range cachedCatalog {
		cachedNames[i] = ev.name
		cachedSchemas[ev.name] = ev.schema
	}
}

// newDomainEventCatalog é a lista canônica dos eventos de domínio, em ordem de
// declaração (agrupada por entidade) apenas para legibilidade deste arquivo.
// Nota: o picker do JobBuilder consome Manager.ListKnownEvents(), que ordena
// alfabeticamente — a ordem declarada aqui não é a ordem exibida na UI.
// Chamada uma única vez por buildDomainCatalog.
func newDomainEventCatalog() []domainEvent {
	return []domainEvent{
		{"tasklist.task.created", mergeSchema(taskEventBase(), nil)},
		{"tasklist.task.updated", mergeSchema(taskEventBase(), map[string]any{"changed_fields": []string{}})},
		{"tasklist.task.status_changed", mergeSchema(taskEventBase(), map[string]any{"from_status_id": 0})},
		{"tasklist.task.assignee_changed", mergeSchema(taskEventBase(), map[string]any{"from_assignee_id": ""})},
		{"tasklist.task.moved", mergeSchema(taskEventBase(), map[string]any{"from_task_list_id": ""})},
		{"tasklist.task.reparented", mergeSchema(taskEventBase(), map[string]any{"from_parent_id": ""})},
		// reordered é um evento de lista (não de card): carrega só a ordenação
		// da raia, então usa schema próprio em vez de taskEventBase().
		{"tasklist.task.reordered", mergeSchema(map[string]any{
			"task_list_id":   "",
			"task_list_slug": "",
			"status_id":      0,
			"ordered_ids":    []string{},
		}, nil)},
		{"tasklist.task.completed", mergeSchema(taskEventBase(), nil)},
		{"tasklist.task.deleted", mergeSchema(taskEventBase(), nil)},
		{"tasklist.note.added", mergeSchema(noteEventBase(), nil)},
		{"tasklist.note.updated", mergeSchema(noteEventBase(), nil)},
		{"tasklist.note.deleted", mergeSchema(noteEventBase(), nil)},
		{"tasklist.list.created", mergeSchema(listEventBase(), nil)},
		{"tasklist.list.cloned", mergeSchema(listEventBase(), map[string]any{"source_task_list_id": ""})},
		{"tasklist.list.updated", mergeSchema(listEventBase(), nil)},
		{"tasklist.list.cleared", mergeSchema(listEventBase(), nil)},
		{"tasklist.list.deleted", mergeSchema(listEventBase(), nil)},
		{"tasklist.list.refresh_requested", mergeSchema(listEventBase(), nil)},
		{"tasklist.workflow.updated", mergeSchema(workflowEventBase(), nil)},
		{"tasklist.item.opened", mergeSchema(taskEventBase(), nil)},
	}
}

// KnownDomainEvents retorna os nomes canônicos dos eventos de domínio do catálogo,
// na ordem de declaração. (ListKnownEvents reordena alfabeticamente ao montar o
// picker, então esta ordem não chega à UI.) Devolve uma cópia para que o caller
// não mute o slice cacheado.
func KnownDomainEvents() []string {
	domainCatalogOnce.Do(buildDomainCatalog)
	names := make([]string, len(cachedNames))
	copy(names, cachedNames)
	return names
}

// DomainEventSchema retorna o schema estático de um evento de domínio do
// catálogo, ou nil se não for um evento conhecido. Usado como fallback por
// InferEventSchema quando nenhum job emissor tem output. Devolve uma cópia (rasa)
// do schema para evitar mutação acidental do catálogo compartilhado.
func DomainEventSchema(name string) map[string]any {
	domainCatalogOnce.Do(buildDomainCatalog)
	schema, ok := cachedSchemas[name]
	if !ok {
		return nil
	}
	out := make(map[string]any, len(schema))
	for k, v := range schema {
		out[k] = v
	}
	return out
}

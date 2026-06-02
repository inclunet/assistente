package tasklist

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/eventctx"
)

type recordedEvent struct {
	name    string
	payload map[string]any
}

type fakeSink struct {
	listening map[string]bool
	events    []recordedEvent
}

func (f *fakeSink) PublishDomainEvent(_ context.Context, name string, payload map[string]any) error {
	f.events = append(f.events, recordedEvent{name: name, payload: payload})
	return nil
}

func (f *fakeSink) HasDomainListener(name string) bool {
	return f.listening[name]
}

func (f *fakeSink) find(name string) (map[string]any, bool) {
	for _, e := range f.events {
		if e.name == name {
			return e.payload, true
		}
	}
	return nil, false
}

type noopEmitter struct{}

func (noopEmitter) Emit(string, any) {}

// recordingEmitter registra os nomes dos eventos Wails emitidos, para verificar
// que mutações disparam os eventos que o frontend escuta.
type recordingEmitter struct {
	events []string
}

func (r *recordingEmitter) Emit(name string, _ any) { r.events = append(r.events, name) }

func (r *recordingEmitter) has(name string) bool {
	for _, e := range r.events {
		if e == name {
			return true
		}
	}
	return false
}

// fakeStore embute a interface (métodos não sobrescritos retornam panic se chamados,
// o que mantém os testes focados nos caminhos exercidos).
type fakeStore struct {
	TaskListRepository
	tasks                  map[string]*database.Task
	notes                  map[string]*database.TaskNote
	getTaskCalls           int
	reloadFailsAfterUpdate bool
	listReloadFails        bool
}

func (s *fakeStore) GetTask(_ context.Context, id string) (*database.Task, error) {
	s.getTaskCalls++
	t := s.tasks[id]
	if t == nil {
		return nil, errors.New("task not found")
	}
	cp := *t
	return &cp, nil
}

func (s *fakeStore) GetTaskList(_ context.Context, id string) (*database.TaskList, error) {
	if s.listReloadFails {
		return nil, errors.New("task list not found")
	}
	tl := &database.TaskList{Title: "Suporte", Slug: "suporte"}
	tl.ID = id
	return tl, nil
}

// UpdateTaskList é no-op; o foco do teste de fallback é a recarga pós-update
// (GetTaskList) falhar — controlada por listReloadFails.
func (s *fakeStore) UpdateTaskList(_ context.Context, id, title, description string) error {
	return nil
}

func (s *fakeStore) UpdateTaskListFull(_ context.Context, id, title, description, preferredViewMode string, slug *string) error {
	return nil
}

func (s *fakeStore) UpdateTaskStatus(_ context.Context, id string, newStatusID int) error {
	t := s.tasks[id]
	if t == nil {
		return errors.New("task not found")
	}
	t.StatusID = newStatusID
	if newStatusID == 3 && t.CompletedAt == nil {
		now := time.Now()
		t.CompletedAt = &now
	}
	if s.reloadFailsAfterUpdate {
		delete(s.tasks, id)
	}
	return nil
}

func (s *fakeStore) UpdateTaskFull(_ context.Context, id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	t := s.tasks[id]
	if t == nil {
		return errors.New("task not found")
	}
	t.Title, t.Code, t.Link = title, code, link
	t.AssigneeName, t.AssigneeID = assigneeName, assigneeID
	t.CreatorName, t.CreatorID = creatorName, creatorID
	if s.reloadFailsAfterUpdate {
		delete(s.tasks, id)
	}
	return nil
}

// UpdateTask aplica a mutação e, quando reloadFailsAfterUpdate, remove o card do
// store para simular a recarga pós-update retornando nil (caminho best-effort).
func (s *fakeStore) UpdateTask(_ context.Context, id, title, description, code, link string) error {
	t := s.tasks[id]
	if t == nil {
		return errors.New("task not found")
	}
	t.Title, t.Code, t.Link = title, code, link
	if s.reloadFailsAfterUpdate {
		delete(s.tasks, id)
	}
	return nil
}

func (s *fakeStore) CreateTask(_ context.Context, taskListID, title, description, code, link string, parentID *string) (*database.Task, error) {
	t := &database.Task{TaskListID: taskListID, Title: title, Code: code, Link: link, StatusID: 1, ParentID: parentID}
	t.ID = "new-task"
	s.tasks[t.ID] = t
	return t, nil
}

func (s *fakeStore) CreateTaskNote(_ context.Context, taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	n := &database.TaskNote{TaskID: taskID, Type: noteType, Content: content, AuthorName: authorName, AuthorID: authorID}
	n.ID = "new-note"
	if s.notes == nil {
		s.notes = map[string]*database.TaskNote{}
	}
	s.notes[n.ID] = n
	return n, nil
}

func newServiceWithSink(store *fakeStore, sink *fakeSink) *Service {
	return NewService(ServiceConfig{Store: store, Emitter: noopEmitter{}, DomainEvents: sink})
}

func seedTask(store *fakeStore, id, listID string, statusID int) {
	if store.tasks == nil {
		store.tasks = map[string]*database.Task{}
	}
	t := &database.Task{TaskListID: listID, Title: "Card", StatusID: statusID}
	t.ID = id
	store.tasks[id] = t
}

func TestUpdateTaskStatusEmitsStatusChangedAndCompleted(t *testing.T) {
	store := &fakeStore{}
	seedTask(store, "t1", "L1", 1)
	sink := &fakeSink{listening: map[string]bool{
		"tasklist.task.status_changed": true,
		"tasklist.task.completed":      true,
	}}
	svc := newServiceWithSink(store, sink)

	if err := svc.UpdateTaskStatus(context.Background(), "t1", 3); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}

	sc, ok := sink.find("tasklist.task.status_changed")
	if !ok {
		t.Fatal("status_changed not emitted")
	}
	if sc["from_status_id"] != 1 {
		t.Fatalf("from_status_id = %v, want 1", sc["from_status_id"])
	}
	if sc["status_id"] != 3 {
		t.Fatalf("status_id = %v, want 3", sc["status_id"])
	}
	if sc["task_list_slug"] != "suporte" {
		t.Fatalf("task_list_slug = %v, want suporte", sc["task_list_slug"])
	}
	if _, ok := sink.find("tasklist.task.completed"); !ok {
		t.Fatal("completed not emitted on transition to done")
	}
}

// TestUpdateTaskListFallsBackToIDWhenReloadFails garante que, se a recarga da
// lista pós-update falhar (GetTaskList -> nil), o evento tasklist.list.updated
// ainda carrega task_list_id (a partir do parâmetro) em vez de sair vazio. O slug
// é best-effort (vazio quando a lista não recarrega). Regressão do review #171.
func TestUpdateTaskListFallsBackToIDWhenReloadFails(t *testing.T) {
	store := &fakeStore{listReloadFails: true}
	sink := &fakeSink{listening: map[string]bool{"tasklist.list.updated": true}}
	svc := newServiceWithSink(store, sink)

	if err := svc.UpdateTaskList(context.Background(), "L-9", "Novo título", ""); err != nil {
		t.Fatalf("UpdateTaskList: %v", err)
	}

	p, ok := sink.find("tasklist.list.updated")
	if !ok {
		t.Fatal("tasklist.list.updated não emitido")
	}
	if p["task_list_id"] != "L-9" {
		t.Fatalf("task_list_id = %v, want L-9 (fallback do parâmetro)", p["task_list_id"])
	}
	if p["task_list_slug"] != "" {
		t.Fatalf("task_list_slug = %v, want vazio (best-effort sem recarga)", p["task_list_slug"])
	}
}

// TestUpdateTaskFullEmitsAssigneeChanged garante que trocar o responsável via
// UpdateTaskFull também dispara tasklist.task.assignee_changed (com
// from_assignee_id), não só tasklist.task.updated. Regressão do review #171.
func TestUpdateTaskFullEmitsAssigneeChanged(t *testing.T) {
	store := &fakeStore{}
	seedTask(store, "t1", "L1", 1)
	store.tasks["t1"].AssigneeID = "ana"
	store.tasks["t1"].AssigneeName = "Ana"
	sink := &fakeSink{listening: map[string]bool{
		"tasklist.task.updated":          true,
		"tasklist.task.assignee_changed": true,
	}}
	svc := newServiceWithSink(store, sink)

	if err := svc.UpdateTaskFull(context.Background(), "t1", "Card", "", "", "", "Bruno", "bruno", "", ""); err != nil {
		t.Fatalf("UpdateTaskFull: %v", err)
	}

	ac, ok := sink.find("tasklist.task.assignee_changed")
	if !ok {
		t.Fatal("tasklist.task.assignee_changed não emitido por UpdateTaskFull")
	}
	if ac["from_assignee_id"] != "ana" {
		t.Fatalf("from_assignee_id = %v, want ana", ac["from_assignee_id"])
	}
	if ac["assignee_id"] != "bruno" {
		t.Fatalf("assignee_id = %v, want bruno", ac["assignee_id"])
	}
}

// TestUpdateTaskFullNoAssigneeChangeSkipsEvent garante que UpdateTaskFull sem
// troca de responsável não emite assignee_changed.
func TestUpdateTaskFullNoAssigneeChangeSkipsEvent(t *testing.T) {
	store := &fakeStore{}
	seedTask(store, "t1", "L1", 1)
	store.tasks["t1"].AssigneeID = "ana"
	sink := &fakeSink{listening: map[string]bool{"tasklist.task.assignee_changed": true}}
	svc := newServiceWithSink(store, sink)

	if err := svc.UpdateTaskFull(context.Background(), "t1", "Novo título", "", "", "", "", "ana", "", ""); err != nil {
		t.Fatalf("UpdateTaskFull: %v", err)
	}
	if _, ok := sink.find("tasklist.task.assignee_changed"); ok {
		t.Fatal("assignee_changed não deveria ser emitido sem troca de responsável")
	}
}

// TestUpdateTaskStatusSkipsNoOp garante que UpdateTaskStatus não emite
// status_changed quando o status não muda (from == to). Regressão do review #171.
func TestUpdateTaskStatusSkipsNoOp(t *testing.T) {
	store := &fakeStore{}
	seedTask(store, "t1", "L1", 2)
	sink := &fakeSink{listening: map[string]bool{"tasklist.task.status_changed": true}}
	svc := newServiceWithSink(store, sink)

	if err := svc.UpdateTaskStatus(context.Background(), "t1", 2); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if _, ok := sink.find("tasklist.task.status_changed"); ok {
		t.Fatal("status_changed não deveria ser emitido em no-op (from == to)")
	}
}

// TestUpdateTaskStatusFallsBackOnReloadFail garante que, se a recarga do card
// falhar (task==nil), status_changed ainda é emitido best-effort a partir do
// snapshot anterior, com status_id refletindo o novo valor do input e
// from_status_id o anterior. Regressão do review #171.
func TestUpdateTaskStatusFallsBackOnReloadFail(t *testing.T) {
	store := &fakeStore{reloadFailsAfterUpdate: true}
	seedTask(store, "t1", "L1", 1)
	sink := &fakeSink{listening: map[string]bool{"tasklist.task.status_changed": true}}
	svc := newServiceWithSink(store, sink)

	if err := svc.UpdateTaskStatus(context.Background(), "t1", 2); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	sc, ok := sink.find("tasklist.task.status_changed")
	if !ok {
		t.Fatal("status_changed deveria ser emitido best-effort mesmo com reload falho")
	}
	if sc["status_id"] != 2 {
		t.Fatalf("status_id = %v, want 2 (novo valor do input)", sc["status_id"])
	}
	if sc["from_status_id"] != 1 {
		t.Fatalf("from_status_id = %v, want 1 (snapshot anterior)", sc["from_status_id"])
	}
	if sc["task_id"] != "t1" {
		t.Fatalf("task_id = %v, want t1 (fallback do snapshot)", sc["task_id"])
	}
}

// TestUpdateTaskListFullEmitsTaskListUpdated garante que UpdateTaskListFull emite
// o evento Wails taskList:updated (que o frontend escuta para invalidar cache),
// como os demais updates de lista. Regressão do review #171.
func TestUpdateTaskListFullEmitsTaskListUpdated(t *testing.T) {
	store := &fakeStore{}
	emitter := &recordingEmitter{}
	svc := NewService(ServiceConfig{Store: store, Emitter: emitter, DomainEvents: &fakeSink{listening: map[string]bool{}}})

	if err := svc.UpdateTaskListFull(context.Background(), "L-1", "Título", "", "kanban", nil); err != nil {
		t.Fatalf("UpdateTaskListFull: %v", err)
	}
	if !emitter.has("taskList:updated") {
		t.Fatalf("taskList:updated não emitido; emitidos: %v", emitter.events)
	}
}

func TestWantsDomainGatingAvoidsExtraReads(t *testing.T) {
	store := &fakeStore{}
	seedTask(store, "t1", "L1", 1)
	sink := &fakeSink{listening: map[string]bool{}} // ninguém escutando
	svc := newServiceWithSink(store, sink)

	if err := svc.UpdateTaskStatus(context.Background(), "t1", 2); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if len(sink.events) != 0 {
		t.Fatalf("expected no domain events, got %d", len(sink.events))
	}
	// Apenas o GetTask pós-update (para o Emit Wails) deve rodar; o pré-update é gated.
	if store.getTaskCalls != 1 {
		t.Fatalf("getTaskCalls = %d, want 1 (no pre-read when no listener)", store.getTaskCalls)
	}
}

func TestProvenanceFromContextIsPropagated(t *testing.T) {
	store := &fakeStore{tasks: map[string]*database.Task{}}
	sink := &fakeSink{listening: map[string]bool{"tasklist.task.created": true}}
	svc := newServiceWithSink(store, sink)

	ctx := eventctx.With(context.Background(), eventctx.Provenance{
		Source:       "job",
		SourceJobID:  "sync-jira",
		ChainID:      "chain-1",
		ChainHistory: []string{"sync-jira"},
	})

	if _, err := svc.CreateTask(ctx, "L1", "Novo", "", "PROJ-1", "", nil); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	p, ok := sink.find("tasklist.task.created")
	if !ok {
		t.Fatal("task.created not emitted")
	}
	if p["_source"] != "job" {
		t.Fatalf("_source = %v, want job", p["_source"])
	}
	if p["_source_job_id"] != "sync-jira" {
		t.Fatalf("_source_job_id = %v, want sync-jira", p["_source_job_id"])
	}
	if p["_chain_id"] != "chain-1" {
		t.Fatalf("_chain_id = %v, want chain-1", p["_chain_id"])
	}
	if p["code"] != "PROJ-1" {
		t.Fatalf("code = %v, want PROJ-1", p["code"])
	}
}

// TestUpdateTaskFallsBackToOldSnapshotWhenReloadFails garante que, se a recarga
// pós-update retornar nil, o evento ainda é publicado a partir do snapshot
// pré-mutação (sem panic ao resolver task_list_id/slug). Regressão do review #171.
func TestUpdateTaskFallsBackToOldSnapshotWhenReloadFails(t *testing.T) {
	store := &fakeStore{reloadFailsAfterUpdate: true}
	seedTask(store, "t1", "L1", 1)
	sink := &fakeSink{listening: map[string]bool{"tasklist.task.updated": true}}
	svc := newServiceWithSink(store, sink)

	if err := svc.UpdateTask(context.Background(), "t1", "Novo título", "", "PROJ-9", ""); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	p, ok := sink.find("tasklist.task.updated")
	if !ok {
		t.Fatal("task.updated não emitido com fallback para o snapshot antigo")
	}
	if p["task_id"] != "t1" {
		t.Fatalf("task_id = %v, want t1 (fallback)", p["task_id"])
	}
	if p["task_list_id"] != "L1" {
		t.Fatalf("task_list_id = %v, want L1 (fallback)", p["task_list_id"])
	}
	if p["task_list_slug"] != "suporte" {
		t.Fatalf("task_list_slug = %v, want suporte (fallback)", p["task_list_slug"])
	}
	// Sem snapshot recarregado, changed_fields é omitido (não vazio) para não
	// induzir o consumidor a achar que nada mudou. Regressão do review #171.
	if _, ok := p["changed_fields"]; ok {
		t.Fatalf("changed_fields não deveria estar presente no fallback (task==nil), veio %v", p["changed_fields"])
	}
}

func TestCreateTaskNoteEmitsNoteAdded(t *testing.T) {
	store := &fakeStore{}
	seedTask(store, "t1", "L1", 1)
	sink := &fakeSink{listening: map[string]bool{"tasklist.note.added": true}}
	svc := newServiceWithSink(store, sink)

	if _, err := svc.CreateTaskNote(context.Background(), "t1", int(database.TaskNoteCustomer), "oi", "Cliente", "cli-1"); err != nil {
		t.Fatalf("CreateTaskNote: %v", err)
	}

	p, ok := sink.find("tasklist.note.added")
	if !ok {
		t.Fatal("note.added not emitted")
	}
	if p["task_list_id"] != "L1" {
		t.Fatalf("task_list_id = %v, want L1", p["task_list_id"])
	}
	if p["note_type"] != int(database.TaskNoteCustomer) {
		t.Fatalf("note_type = %v, want %d", p["note_type"], int(database.TaskNoteCustomer))
	}
	if p["note_id"] != "new-note" {
		t.Fatalf("note_id = %v, want new-note", p["note_id"])
	}
}

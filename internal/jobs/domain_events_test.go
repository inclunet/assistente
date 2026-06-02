package jobs

import (
	"context"
	"testing"
	"time"
)

// TestPublishDomainEventReachesSubscriber verifica que PublishDomainEvent publica
// no EventBus, enriquece com proveniencia padrao e o subscriber recebe o payload.
func TestPublishDomainEventReachesSubscriber(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(mgr.Stop)

	received := make(chan map[string]any, 1)
	mgr.eventBus.Subscribe("tasklist.list.refresh_requested", "test", func(_ context.Context, _ string, payload map[string]any) {
		received <- payload
	})

	if err := mgr.PublishDomainEvent(userA, "tasklist.list.refresh_requested", map[string]any{
		"task_list_id":   "list-1",
		"task_list_slug": "suporte",
	}); err != nil {
		t.Fatalf("publish domain event: %v", err)
	}

	select {
	case payload := <-received:
		if payload["task_list_id"] != "list-1" {
			t.Fatalf("task_list_id = %v, want list-1", payload["task_list_id"])
		}
		if payload["_source"] != "user" {
			t.Fatalf("_source = %v, want user (default)", payload["_source"])
		}
		if jid, ok := payload["_source_job_id"]; !ok || jid != "" {
			t.Fatalf("_source_job_id = %v (present=%v), want \"\" sempre presente", payload["_source_job_id"], ok)
		}
		if id, _ := payload["_chain_id"].(string); id == "" {
			t.Fatal("_chain_id should be set by PublishDomainEvent")
		}
		if hist, ok := payload["_chain_history"].([]string); !ok || len(hist) != 1 || hist[0] != "tasklist.list.refresh_requested" {
			t.Fatalf("_chain_history = %#v, want [tasklist.list.refresh_requested]", payload["_chain_history"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber never received the domain event")
	}
}

// TestPublishDomainEventPreservesExplicitProvenance garante que proveniencia ja
// presente no payload (ex.: vinda de um job) nao é sobrescrita.
func TestPublishDomainEventPreservesExplicitProvenance(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(mgr.Stop)

	received := make(chan map[string]any, 1)
	mgr.eventBus.Subscribe("tasklist.task.updated", "test", func(_ context.Context, _ string, payload map[string]any) {
		received <- payload
	})

	if err := mgr.PublishDomainEvent(userA, "tasklist.task.updated", map[string]any{
		"task_id":   "t-1",
		"_source":   "job",
		"_chain_id": "chain-xyz",
	}); err != nil {
		t.Fatalf("publish domain event: %v", err)
	}

	select {
	case payload := <-received:
		if payload["_source"] != "job" {
			t.Fatalf("_source = %v, want job (preserved)", payload["_source"])
		}
		if payload["_chain_id"] != "chain-xyz" {
			t.Fatalf("_chain_id = %v, want chain-xyz (preserved)", payload["_chain_id"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber never received the domain event")
	}
}

// TestPublishDomainEventRequiresUserID garante que, sem user_id resolvivel
// (nem no ctx nem no Manager), a publicacao é rejeitada; e que nome vazio falha.
func TestPublishDomainEventRequiresUserID(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)

	// Manager sem ContextProvider: nenhum user_id é forcado, entao um ctx pelado falha.
	noUserMgr := NewManager(ManagerConfig{Repository: repo})
	if err := noUserMgr.PublishDomainEvent(context.Background(), "tasklist.list.refresh_requested", nil); err == nil {
		t.Fatal("expected error when no user_id is resolvable")
	}

	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	if err := mgr.PublishDomainEvent(userA, "", nil); err == nil {
		t.Fatal("expected error for empty event name")
	}
}

// TestListKnownEventsIncludesDomainCatalog garante que o catalogo estatico aparece
// mesmo sem nenhum job referenciando os eventos.
func TestListKnownEventsIncludesDomainCatalog(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(mgr.Stop)

	events := mgr.ListKnownEvents()
	want := []string{"tasklist.list.refresh_requested", "tasklist.task.status_changed", "tasklist.note.added"}
	for _, w := range want {
		if !contains(events, w) {
			t.Fatalf("ListKnownEvents missing %q; got %#v", w, events)
		}
	}
}

// TestInferEventSchemaFallsBackToCatalog garante o fallback de schema estatico.
func TestInferEventSchemaFallsBackToCatalog(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(mgr.Stop)

	schema := mgr.InferEventSchema("tasklist.task.status_changed")
	if schema == nil {
		t.Fatal("expected catalog schema fallback, got nil")
	}
	if _, ok := schema["from_status_id"]; !ok {
		t.Fatalf("schema missing from_status_id: %#v", schema)
	}
	if _, ok := schema["_source"]; !ok {
		t.Fatalf("schema missing provenance _source: %#v", schema)
	}

	if mgr.InferEventSchema("totally.unknown.event") != nil {
		t.Fatal("unknown event should return nil schema")
	}
}

func contains(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

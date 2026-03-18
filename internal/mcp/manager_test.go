package mcp

import (
	"context"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/tools"
)

// TestNewManager testa criação de novo manager
func TestNewManager(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	emitFunc := func(event string, data any) {}

	manager := NewManager(registry, credMgr, emitFunc)

	if manager == nil {
		t.Fatal("NewManager retornou nil")
	}

	if manager.registry != registry {
		t.Error("registry não foi atribuído corretamente")
	}

	if manager.credMgr != credMgr {
		t.Error("credMgr não foi atribuído corretamente")
	}

	if len(manager.servers) != 0 {
		t.Errorf("esperado 0 servers, got %d", len(manager.servers))
	}

	if len(manager.connections) != 0 {
		t.Errorf("esperado 0 connections, got %d", len(manager.connections))
	}
}

// TestSetSamplingHandler testa configuração do handler de sampling
func TestSetSamplingHandler(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Handler customizado
	handlerCalled := false
	customHandler := func(ctx context.Context, req SamplingRequest) (string, error) {
		handlerCalled = true
		return "response", nil
	}

	manager.SetSamplingHandler(customHandler)

	// Verifica que handler foi atribuído
	if manager.llmHandler == nil {
		t.Fatal("handler não foi configurado")
	}

	// Testa invocação do handler
	resp, err := manager.llmHandler(context.Background(), SamplingRequest{})
	if err != nil {
		t.Errorf("handler retornou erro: %v", err)
	}
	if resp != "response" {
		t.Errorf("esperado 'response', got %q", resp)
	}
	if !handlerCalled {
		t.Error("handler não foi chamado")
	}
}

// TestSetWorkspaceRoots testa configuração de workspace roots
func TestSetWorkspaceRoots(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	roots := []Root{
		{URI: "file:///home/user/project", Name: "my-project"},
		{URI: "file:///home/user/other", Name: "other"},
	}

	err := manager.SetWorkspaceRoots(roots)
	if err != nil {
		t.Errorf("SetWorkspaceRoots retornou erro: %v", err)
	}

	// Verifica que roots foram armazenados
	retrievedRoots := manager.GetWorkspaceRoots()
	if len(retrievedRoots) != len(roots) {
		t.Errorf("esperado %d roots, got %d", len(roots), len(retrievedRoots))
	}

	for i, root := range retrievedRoots {
		if root.URI != roots[i].URI || root.Name != roots[i].Name {
			t.Errorf("root %d não bate: %+v vs %+v", i, root, roots[i])
		}
	}
}

// TestGetWorkspaceRoots testa recuperação de workspace roots
func TestGetWorkspaceRoots(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Sem roots configurados
	roots := manager.GetWorkspaceRoots()
	if roots == nil {
		roots = []Root{}
	}
	if len(roots) != 0 {
		t.Errorf("esperado 0 roots inicialmente, got %d", len(roots))
	}

	// Depois de configurar
	newRoots := []Root{{URI: "file:///test", Name: "test"}}
	manager.SetWorkspaceRoots(newRoots)

	roots = manager.GetWorkspaceRoots()
	if len(roots) != 1 {
		t.Errorf("esperado 1 root, got %d", len(roots))
	}
}

// TestManagerConcurrency testa acesso concorrente ao manager
func TestManagerConcurrency(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Executa operações concorrentes
	done := make(chan bool, 10)

	// Goroutine 1: Set roots
	go func() {
		manager.SetWorkspaceRoots([]Root{
			{URI: "file:///test1", Name: "test1"},
		})
		done <- true
	}()

	// Goroutine 2: Get roots
	go func() {
		manager.GetWorkspaceRoots()
		done <- true
	}()

	// Goroutine 3: Set handler
	go func() {
		manager.SetSamplingHandler(func(ctx context.Context, req SamplingRequest) (string, error) {
			return "", nil
		})
		done <- true
	}()

	// Goroutine 4-10: More gets
	for i := 0; i < 7; i++ {
		go func() {
			manager.GetWorkspaceRoots()
			done <- true
		}()
	}

	// Aguarda todas as goroutinas
	for i := 0; i < 10; i++ {
		<-done
	}

	// Se chegou aqui sem deadlock/panic, test passou
}

// TestEmitEvent testa emissão de eventos
func TestEmitEvent(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)

	eventChan := make(chan struct { name string; data any }, 10)
	emitFunc := func(event string, data any) {
		eventChan <- struct {
			name string
			data any
		}{event, data}
	}

	manager := NewManager(registry, credMgr, emitFunc)

	// SetWorkspaceRoots deve emitir evento
	manager.SetWorkspaceRoots([]Root{
		{URI: "file:///test", Name: "test"},
	})

	// Tenta receber evento (non-blocking)
	select {
	case event := <-eventChan:
		if event.name != "mcp:roots_changed" {
			t.Errorf("esperado 'mcp:roots_changed', got %q", event.name)
		}
	default:
		t.Error("nenhum evento foi emitido")
	}
}

// TestManagerCancel testa cancelamento de context
func TestManagerCancel(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Context deve estar válido inicialmente
	select {
	case <-manager.ctx.Done():
		t.Fatal("context estava cancelado no início")
	default:
		// OK
	}

	// Cancelar
	manager.cancel()

	// Context deve estar cancelado agora
	select {
	case <-manager.ctx.Done():
		// OK
	default:
		t.Fatal("context não foi cancelado após Cancel()")
	}
}

// TestManagerStateConsistency testa consistência de estado
func TestManagerStateConsistency(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Configura roots
	roots1 := []Root{{URI: "file:///test1", Name: "test1"}}
	manager.SetWorkspaceRoots(roots1)

	// Obtém valores
	retrieved1 := manager.GetWorkspaceRoots()
	if len(retrieved1) != 1 || retrieved1[0].URI != "file:///test1" {
		t.Error("state inconsistency: roots1 não foram recuperados corretamente")
	}

	// Atualiza roots
	roots2 := []Root{
		{URI: "file:///test2", Name: "test2"},
		{URI: "file:///test3", Name: "test3"},
	}
	manager.SetWorkspaceRoots(roots2)

	// Obtém valores novamente
	retrieved2 := manager.GetWorkspaceRoots()
	if len(retrieved2) != 2 {
		t.Errorf("state inconsistency: esperado 2 roots, got %d", len(retrieved2))
	}

	// Verifica que são os novos
	if retrieved2[0].URI != "file:///test2" {
		t.Errorf("state inconsistency: esperado test2, got %s", retrieved2[0].URI)
	}
}

// TestSetMultipleHandlers testa múltiplas configurações de handler
func TestSetMultipleHandlers(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Handler 1
	manager.SetSamplingHandler(func(ctx context.Context, req SamplingRequest) (string, error) {
		return "handler1", nil
	})

	resp1, _ := manager.llmHandler(context.Background(), SamplingRequest{})
	if resp1 != "handler1" {
		t.Errorf("esperado 'handler1', got %q", resp1)
	}

	// Handler 2 (substitui)
	manager.SetSamplingHandler(func(ctx context.Context, req SamplingRequest) (string, error) {
		return "handler2", nil
	})

	resp2, _ := manager.llmHandler(context.Background(), SamplingRequest{})
	if resp2 != "handler2" {
		t.Errorf("esperado 'handler2', got %q", resp2)
	}
}

package app

import (
	"errors"
	"testing"
)

// TestRuntimeReloadResult_AddCollectsPerSubsystem garante que a coleta de
// falhas por subsistema acumula apenas erros reais (err != nil), preservando
// subsistema + mensagem — o coração da correção da issue #250 (antes só havia
// log.Printf e o usuário não sabia que o runtime subiu parcialmente).
func TestRuntimeReloadResult_AddCollectsPerSubsystem(t *testing.T) {
	result := &runtimeReloadResult{}

	if result.hasFailures() {
		t.Fatal("resultado recém-criado não deveria ter falhas")
	}

	// add com err nil é no-op (caminho feliz não polui o resultado).
	result.add(runtimeSubsystemMCP, nil)
	if result.hasFailures() {
		t.Fatal("add(nil) não deveria registrar falha")
	}

	result.add(runtimeSubsystemMCP, errors.New("falha ao carregar MCP"))
	result.add(runtimeSubsystemJobs, errors.New("falha ao iniciar jobs"))
	result.add(runtimeSubsystemToolInvocations, errors.New("falha ao limpar invocations"))

	if !result.hasFailures() {
		t.Fatal("esperava falhas coletadas")
	}
	if len(result.failures) != 3 {
		t.Fatalf("esperava 3 falhas, obteve %d", len(result.failures))
	}

	bySubsystem := map[string]string{}
	for _, f := range result.failures {
		bySubsystem[f.Subsystem] = f.Error
	}
	for _, sub := range []string{runtimeSubsystemMCP, runtimeSubsystemJobs, runtimeSubsystemToolInvocations} {
		if _, ok := bySubsystem[sub]; !ok {
			t.Errorf("subsistema %q ausente nas falhas coletadas", sub)
		}
	}
	if bySubsystem[runtimeSubsystemMCP] != "falha ao carregar MCP" {
		t.Errorf("mensagem de erro do MCP não preservada: %q", bySubsystem[runtimeSubsystemMCP])
	}
}

// TestEmitRuntimePartialInit_EmitsWhenFailures garante que o evento
// runtime:partial-init é emitido com o payload tipado listando os subsistemas
// falhos — é o que o frontend escuta para o aviso não-bloqueante.
func TestEmitRuntimePartialInit_EmitsWhenFailures(t *testing.T) {
	em := &testEmitter{}
	app := &App{emitter: em}

	result := runtimeReloadResult{}
	result.add(runtimeSubsystemMCP, errors.New("boom mcp"))
	result.add(runtimeSubsystemTimeout, errors.New("context deadline exceeded"))

	app.emitRuntimePartialInit(result)

	emitted := em.find(RuntimePartialInitEventName)
	if len(emitted) != 1 {
		t.Fatalf("esperava 1 evento %q, obteve %d", RuntimePartialInitEventName, len(emitted))
	}

	payload, ok := emitted[0].data.(RuntimePartialInitPayload)
	if !ok {
		t.Fatalf("payload com tipo inesperado: %T", emitted[0].data)
	}
	if len(payload.Subsystems) != 2 {
		t.Fatalf("esperava 2 subsistemas no payload, obteve %d", len(payload.Subsystems))
	}
	if payload.Subsystems[0].Subsystem != runtimeSubsystemMCP {
		t.Errorf("primeiro subsistema inesperado: %q", payload.Subsystems[0].Subsystem)
	}
}

// TestEmitRuntimePartialInit_NoopWhenNoFailures garante que o login saudável
// (sem falhas) não dispara aviso algum — evita ruído/false positive na UI.
func TestEmitRuntimePartialInit_NoopWhenNoFailures(t *testing.T) {
	em := &testEmitter{}
	app := &App{emitter: em}

	app.emitRuntimePartialInit(runtimeReloadResult{})

	if em.count() != 0 {
		t.Fatalf("não deveria emitir evento sem falhas, emitiu %d", em.count())
	}
}

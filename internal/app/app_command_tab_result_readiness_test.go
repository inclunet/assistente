package app

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"assistente/internal/database"
	"github.com/google/uuid"
)

func TestCreatedTabResultRemainsReadableWithoutPublishedMap(t *testing.T) {
	for _, commandID := range []string{
		"workspace.tab.chat.create",
		"workspace.tab.editor.create",
		"workspace.tab.tasklist.create",
	} {
		t.Run(commandID, func(t *testing.T) {
			a := readyCommandProduct(t)
			reservation := beginUICommand(t, a, commandID)
			handoff := takeUICommandFor(t, a, reservation.Ticket, commandID)
			if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
				t.Fatal(err)
			}
			if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" {
				t.Fatalf("criação não confirmada: %+v", result)
			}
			if err := a.resetCommandLifecycleIfConfigured(context.Background(), "test-tab-result-only"); err != nil {
				t.Fatal(err)
			}
			result, err := a.GetUICommandResult(reservation.Ticket)
			if err != nil || result.Status != "succeeded" || result.InvocationID != reservation.InvocationID {
				t.Fatalf("resultado confirmado ficou inacessível durante retirada do mapa: %+v %v", result, err)
			}
			if _, err := a.BeginUICommand(commandID); err == nil {
				t.Fatal("consulta de resultado não pode habilitar nova execução")
			}
		})
	}
}

func TestCreatedTabResultRejectsWhenReadAuthorityIsGone(t *testing.T) {
	tests := []struct {
		name          string
		unknownTicket bool
		before        func(t *testing.T, a *App, ticket string)
	}{
		{
			name: "sessao-revogada",
			before: func(t *testing.T, a *App, _ string) {
				sessionID := a.commandProduct.Load().principal.SessionID
				if err := database.DB().Model(&database.Session{}).Where("id = ?", sessionID).Update("revoked_at", time.Now().UTC()).Error; err != nil {
					t.Fatalf("revogar sessão: %v", err)
				}
			},
		},
		{
			name: "outra-sessao-local",
			before: func(t *testing.T, a *App, _ string) {
				a.authMu.Lock()
				a.currentAuthUser.SessionID = uuid.NewString()
				a.authMu.Unlock()
			},
		},
		{
			name: "cofre-bloqueado",
			before: func(t *testing.T, a *App, _ string) {
				if err := a.commandProduct.Load().host.SetVaultUnlocked(context.Background(), false); err != nil {
					t.Fatalf("bloquear cofre: %v", err)
				}
			},
		},
		{
			name: "sessao-do-so-bloqueada",
			before: func(t *testing.T, a *App, _ string) {
				if err := a.commandProduct.Load().host.SetOSSessionState(context.Background(), true, true); err != nil {
					t.Fatalf("bloquear sessão do SO: %v", err)
				}
			},
		},
		{
			name: "sessao-do-so-desconhecida",
			before: func(t *testing.T, a *App, _ string) {
				if err := a.commandProduct.Load().host.SetOSSessionState(context.Background(), false, false); err != nil {
					t.Fatalf("marcar sessão do SO desconhecida: %v", err)
				}
			},
		},
		{
			name:          "ticket-desconhecido",
			unknownTicket: true,
		},
		{
			name: "ticket-expirado",
			before: func(t *testing.T, a *App, ticket string) {
				p := a.commandProduct.Load()
				p.mu.Lock()
				p.uiRuns[ticket].expiresAt = time.Now().Add(-time.Second)
				p.mu.Unlock()
			},
		},
		{
			name: "runtime-fechado",
			before: func(t *testing.T, a *App, _ string) {
				p := a.commandProduct.Load()
				p.mu.Lock()
				p.closed = true
				p.mu.Unlock()
			},
		},
		{
			name: "runtime-substituido-apos-troca-de-workspace",
			before: func(t *testing.T, a *App, _ string) {
				second, err := a.workspaceMgr.Create("tab-result-runtime-switch")
				if err != nil {
					t.Fatalf("criar workspace para troca: %v", err)
				}
				if _, err := a.workspaceMgr.Switch(second.ID); err != nil {
					t.Fatalf("trocar workspace: %v", err)
				}
				if err := a.mountCommandProduct(context.Background()); err != nil {
					t.Fatalf("remontar runtime após troca de workspace: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := readyCommandProduct(t)
			reservation := beginUICommand(t, a, "workspace.tab.chat.create")
			handoff := takeUICommandFor(t, a, reservation.Ticket, reservation.CommandID)
			if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
				t.Fatalf("confirmar criação da aba: %v", err)
			}
			if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" {
				t.Fatalf("criação não confirmada: %+v", result)
			}

			if tt.before != nil {
				tt.before(t, a, reservation.Ticket)
			}
			ticket := reservation.Ticket
			if tt.unknownTicket {
				ticket += "-unknown"
			}
			if result, err := a.GetUICommandResult(ticket); err == nil {
				t.Fatalf("resultado exposto sem autoridade/ticket válido: %+v", result)
			}
		})
	}
}

func TestPendingCreatedTabResultRechecksAuthorityAfterCompletion(t *testing.T) {
	a := readyCommandProduct(t)
	reservation := beginUICommand(t, a, "workspace.tab.chat.create")
	handoff := takeUICommandFor(t, a, reservation.Ticket, reservation.CommandID)
	p := a.commandProduct.Load()
	p.mu.Lock()
	run := p.uiRuns[reservation.Ticket]
	p.mu.Unlock()
	if run == nil {
		t.Fatal("reserva UI não foi retida")
	}

	type queryResult struct {
		result CommandExecutionResult
		err    error
	}
	resultCh := make(chan queryResult, 1)
	queryGoroutineID := make(chan uint64, 1)
	go func() {
		stack := make([]byte, 128)
		n := runtime.Stack(stack, false)
		var id uint64
		if _, err := fmt.Sscanf(string(stack[:n]), "goroutine %d", &id); err != nil {
			queryGoroutineID <- 0
			resultCh <- queryResult{err: fmt.Errorf("capturar goroutine da consulta: %w", err)}
			return
		}
		queryGoroutineID <- id
		result, err := a.GetUICommandResult(reservation.Ticket)
		resultCh <- queryResult{result: result, err: err}
	}()
	queryID := <-queryGoroutineID
	if queryID == 0 {
		result := <-resultCh
		t.Fatalf("%v", result.err)
	}
	// Observe o método no topo do select: aguardar só a query SQL ou um prazo
	// fixo não prova que a leitura já passou a autorização e espera por run.done.
	stackMarker := fmt.Sprintf("goroutine %d [select]:\nassistente/internal/app.(*App).GetUICommandResult(", queryID)
	stack := make([]byte, 4<<20)
	deadline := time.Now().Add(uiCommandTestTimeout)
	for {
		n := runtime.Stack(stack, true)
		if bytes.Contains(stack[:n], []byte(stackMarker)) {
			break
		}
		select {
		case result := <-resultCh:
			t.Fatalf("consulta terminou antes de aguardar run.done: %+v %v", result.result, result.err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("goroutine da consulta não chegou ao select de espera em GetUICommandResult")
		}
		time.Sleep(time.Millisecond)
	}

	// Troca apenas a identidade local em memória: a sessão original permanece
	// válida no banco, então a conclusão controlada não vira erro/cancelamento.
	a.authMu.Lock()
	a.currentAuthUser.SessionID = uuid.NewString()
	a.authMu.Unlock()
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
		t.Fatal("fachada Commit aceitou a sessão local substituta")
	}
	if err := p.ui.Complete(p.uiOwner(), reservation.Ticket, handoff.HandoffID, "succeeded"); err != nil {
		t.Fatalf("concluir handoff UI controladamente: %v", err)
	}

	select {
	case <-run.done:
	case <-time.After(uiCommandTestTimeout):
		t.Fatal("run UI não terminou após concluir o handoff")
	}
	p.mu.Lock()
	runErr, runResult := run.err, run.result
	p.mu.Unlock()
	if runErr != nil || runResult.Status != "succeeded" || runResult.InvocationID != reservation.InvocationID {
		t.Fatalf("run deveria ter resultado succeeded antes da revalidação pós-wait: %+v err=%v", runResult, runErr)
	}

	select {
	case result := <-resultCh:
		if result.err == nil {
			t.Fatalf("consulta expôs resultado após troca de sessão: %+v", result.result)
		}
	case <-time.After(uiCommandTestTimeout):
		t.Fatal("consulta não terminou após run.done")
	}
}

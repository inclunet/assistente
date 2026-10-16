package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"github.com/google/uuid"
)

func TestCommandHandlerInheritedOriginRequiresResolverAndPreservesChain(t *testing.T) {
	root := CommandJobOrigin{RootOriginType: "internal_event", RootOriginID: "event-42"}
	cases := []struct {
		name    string
		resolve func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)
		wantOK  bool
	}{
		{name: "resolver ausente", resolve: nil},
		{name: "erro do resolver", resolve: func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error) {
			return CommandJobOrigin{}, errors.New("origem indisponível")
		}},
		{name: "origem unknown", resolve: func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error) {
			return CommandJobOrigin{RootOriginType: "unknown", RootOriginID: "event-42"}, nil
		}},
		{name: "origem externa não elegível", resolve: func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error) {
			return CommandJobOrigin{RootOriginType: "external_event", RootOriginID: "event-42"}, nil
		}},
		{name: "origem válida", resolve: func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error) { return root, nil }, wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCommandHandlerFixture(t, nil)
			in := inheritedCommandJobInvocation(t, f)
			definition := f.definition
			contract := commandcatalog.HandlerContract{Effect: commandcatalog.Destructive, HasMutableTarget: true, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerJob}
			handler, err := f.manager.CommandHandler(CommandHandlerConfig{Definition: definition, Contract: contract, Target: CommandJobTarget{DatabaseID: f.job.DatabaseID, Slug: f.job.ID, DefinitionFingerprint: f.definitionFingerprint}, Authorize: f.authorize, ResolveOrigin: tc.resolve})
			if err != nil {
				t.Fatal(err)
			}
			handle, err := handler.Start(f.ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			outcome := <-handle.Done
			if !tc.wantOK {
				if outcome.Status == commandledger.Succeeded || f.calls.Load() != 0 {
					t.Fatalf("cadeia inválida executou: outcome=%+v calls=%d", outcome, f.calls.Load())
				}
				return
			}
			if outcome.Status != commandledger.Succeeded {
				t.Fatalf("cadeia herdada recusada: %+v", outcome)
			}
			var metadata map[string]string
			if err := json.Unmarshal(outcome.Result, &metadata); err != nil {
				t.Fatal(err)
			}
			run, err := f.repo.GetRun(f.ctx, f.job.ID, metadata["run_id"])
			if err != nil {
				t.Fatal(err)
			}
			if run.RootOriginType != root.RootOriginType || run.RootOriginID != root.RootOriginID {
				t.Fatalf("raiz alterada: %q/%q", run.RootOriginType, run.RootOriginID)
			}
			var provenance map[string]any
			encodedProvenance, err := json.Marshal(run.Provenance)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encodedProvenance, &provenance); err != nil {
				t.Fatal(err)
			}
			if provenance["_chain_id"] != "parent-chain" {
				t.Fatalf("chain id perdido: %#v", provenance["_chain_id"])
			}
			if got, ok := provenance["_chain_history"].([]any); !ok || len(got) < 1 || got[0] != "parent-run" {
				t.Fatalf("chain history perdida: %#v", provenance["_chain_history"])
			}
			var history []commandcontract.CommandChainEntry
			raw, _ := json.Marshal(provenance["command_chain_history"])
			if err := json.Unmarshal(raw, &history); err != nil || len(history) != 2 || history[0].CommandID != "parent.command" || history[1].CommandID != f.definition.ID || history[1].InvocationID != f.invocationID {
				t.Fatalf("command chain não preservada: %s", raw)
			}
		})
	}
}

func TestCommandHandlerInheritedOriginMustStayStableAcrossRetry(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	f.tool.retryFirst = true
	f.job.ErrorPolicy = ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 1, RetryDelay: "1ms"}
	if err := f.repo.SaveJob(f.ctx, f.job); err != nil {
		t.Fatal(err)
	}
	f.job, _ = f.repo.GetJob(f.ctx, f.job.ID)
	f.definitionFingerprint = mustDefinitionFingerprint(t, f.job)
	var changed atomic.Bool
	f.tool.onCall = func() { changed.Store(true) }
	resolver := func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error) {
		if changed.Load() {
			return CommandJobOrigin{RootOriginType: "internal_event", RootOriginID: "different-root"}, nil
		}
		return CommandJobOrigin{RootOriginType: "internal_event", RootOriginID: "event-42"}, nil
	}
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Destructive, HasMutableTarget: true, Route: f.definition.HandlerRoute, Classification: commandcatalog.HandlerJob}
	handler, err := f.manager.CommandHandler(CommandHandlerConfig{Definition: f.definition, Contract: contract, Target: CommandJobTarget{DatabaseID: f.job.DatabaseID, Slug: f.job.ID, DefinitionFingerprint: f.definitionFingerprint}, Authorize: f.authorize, ResolveOrigin: resolver})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := handler.Start(f.ctx, inheritedCommandJobInvocation(t, f))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-handle.Done:
		if outcome.Status == commandledger.Succeeded {
			t.Fatalf("raiz trocada permitiu retry: %+v", outcome)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout aguardando recusa da raiz trocada")
	}
	if got := f.calls.Load(); got != 1 {
		t.Fatalf("efeitos após troca da raiz: %d", got)
	}
}

func TestCommandHandlerSameJobDescendantDoesNotInheritParentDispatch(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	var revalidateCalls atomic.Int32
	parent := commandEventOrigin{userID: f.userID, rootType: "internal_event", rootID: "event-42", chainID: "parent-chain", history: []string{"parent-run", f.job.ID}, commandHistory: []byte(`[]`), hasCommandHistory: true}
	ctx := context.WithValue(f.ctx, commandEventOriginKey{}, parent)
	ctx = context.WithValue(ctx, commandJobDispatchKey{}, commandJobDispatch{
		jobID:      f.job.DatabaseID,
		chainDepth: 1,
		revalidate: func(context.Context) error {
			revalidateCalls.Add(1)
			return ErrCommandJobDenied
		},
	})
	trigger := &TriggerContext{Type: TriggerManual, ChainID: parent.chainID, ChainHistory: append([]string(nil), parent.history...), EventPayload: map[string]any{}}
	run := f.manager.executor.Execute(ctx, f.job, trigger)
	if run == nil || run.Status != RunStatusFailed || !strings.Contains(run.Error, "event loop detected") {
		t.Fatalf("circuito anti-loop não recusou o descendente: %+v", run)
	}
	if f.calls.Load() != 0 {
		t.Fatalf("tool executada apesar do loop: %d", f.calls.Load())
	}
	if revalidateCalls.Load() != 0 {
		t.Fatal("descendente reutilizou autorização do dispatch pai")
	}
}

func inheritedCommandJobInvocation(t *testing.T, f *commandHandlerFixture) commandexecution.Invocation {
	t.Helper()
	in := f.invocation(f.definition, f.job)
	commandHistory := []map[string]any{
		{"command_id": "parent.command", "invocation_id": uuid.Must(uuid.NewV7()).String(), "layer_refs": []string{"parent.layer"}},
		{"command_id": f.definition.ID, "invocation_id": f.invocationID, "layer_refs": []string{}},
	}
	raw, err := json.Marshal(map[string]any{"version": 1, "_chain_id": "parent-chain", "_chain_history": []string{"parent-run"}, "command_chain_history": commandHistory})
	if err != nil {
		t.Fatal(err)
	}
	provenance := json.RawMessage(raw)
	in.Envelope.Provenance = &provenance
	return in
}

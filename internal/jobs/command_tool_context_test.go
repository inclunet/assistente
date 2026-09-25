package jobs

import (
	"context"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/eventctx"
)

func TestCommandToolContextDirectBindsInvocationAndCleansUp(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	in := f.invocation(f.definition, f.job)
	toolCtx, release, err := f.manager.CommandToolContext(f.ctx, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if release == nil || toolCtx == nil {
		t.Fatal("contexto ou cleanup ausente")
	}
	if _, ok := toolCtx.Value(commandJobDispatchKey{}).(commandJobDispatch); ok {
		t.Fatal("CommandToolContext instalou dispatch de job")
	}
	if err := f.manager.ValidateCommandToolContext(toolCtx, in); err != nil {
		t.Fatalf("contexto direto não revalidou: %v", err)
	}
	if _, _, err := f.manager.CommandToolContext(toolCtx, in, nil); err == nil {
		t.Fatal("segundo comando herdou autorização do primeiro")
	}
	release()
	release()
	if err := f.manager.ValidateCommandToolContext(toolCtx, in); err == nil {
		t.Fatal("cleanup não invalidou o contexto")
	}
}

func TestCommandToolContextReactiveRevalidatesRootAndChain(t *testing.T) {
	f := newCommandHandlerFixture(t, nil)
	in := inheritedCommandJobInvocation(t, f)
	root := CommandJobOrigin{RootOriginType: "internal_event", RootOriginID: "event-42"}
	currentRoot := root
	resolverCalls := 0
	resolver := func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error) {
		resolverCalls++
		return currentRoot, nil
	}
	toolCtx, release, err := f.manager.CommandToolContext(f.ctx, in, resolver)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	origin, ok := toolCtx.Value(commandEventOriginKey{}).(commandEventOrigin)
	if !ok || origin.rootType != root.RootOriginType || origin.rootID != root.RootOriginID || origin.chainID != "parent-chain" || len(origin.history) != 1 || !origin.hasCommandHistory {
		t.Fatalf("origem privada incompleta: %+v", origin)
	}
	if err := f.manager.ValidateCommandToolContext(toolCtx, in); err != nil {
		t.Fatalf("origem reativa válida recusada: %v", err)
	}
	if resolverCalls < 2 {
		t.Fatalf("resolver não foi usado na preparação e revalidação: %d", resolverCalls)
	}
	changed := in
	commandID := "different-command"
	changed.Envelope = func() *commandcontract.Envelope {
		clone := in.Envelope.Clone()
		clone.CommandID = &commandID
		return &clone
	}()
	if err := f.manager.ValidateCommandToolContext(toolCtx, changed); err == nil {
		t.Fatal("invocação diferente reutilizou binding privado")
	}
	currentRoot = CommandJobOrigin{RootOriginType: "internal_event", RootOriginID: "other-event"}
	if err := f.manager.ValidateCommandToolContext(toolCtx, in); err == nil {
		t.Fatal("revogação da raiz capturada foi aceita")
	}
}

func TestCommandToolContextRejectsOwnerRuntimeInheritedAndOldSource(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*commandHandlerFixture, *commandexecution.Invocation) (context.Context, func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error))
	}{
		{name: "owner estrangeiro", setup: func(f *commandHandlerFixture, in *commandexecution.Invocation) (context.Context, func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)) {
			foreign := "foreign-user"
			in.Principal.UserID = foreign
			in.Envelope.UserID = &foreign
			in.Envelope.ActorID = foreign
			return database.WithUserID(context.Background(), foreign), nil
		}},
		{name: "runtime divergente", setup: func(f *commandHandlerFixture, in *commandexecution.Invocation) (context.Context, func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)) {
			in.Envelope.SecurityGeneration = "stale-generation"
			return f.ctx, nil
		}},
		{name: "origem herdada sem resolver", setup: func(f *commandHandlerFixture, in *commandexecution.Invocation) (context.Context, func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)) {
			return f.ctx, nil
		}},
		{name: "proveniência pública", setup: func(f *commandHandlerFixture, in *commandexecution.Invocation) (context.Context, func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)) {
			return eventctx.With(f.ctx, eventctx.Provenance{Source: "job", SourceJobID: "foreign-job"}), nil
		}},
		{name: "binding privado existente", setup: func(f *commandHandlerFixture, in *commandexecution.Invocation) (context.Context, func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)) {
			return context.WithValue(f.ctx, commandToolOriginBindingKey{}, commandToolOriginBinding{}), nil
		}},
		{name: "source antigo", setup: func(f *commandHandlerFixture, in *commandexecution.Invocation) (context.Context, func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)) {
			old := commandcatalog.Source("old")
			in.Source = old
			source := commandcontract.SourceType(old)
			in.Envelope.SourceType = &source
			return f.ctx, nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCommandHandlerFixture(t, nil)
			in := f.invocation(f.definition, f.job)
			if tc.name == "origem herdada sem resolver" {
				in = inheritedCommandJobInvocation(t, f)
			}
			ctx, resolver := tc.setup(f, &in)
			if _, release, err := f.manager.CommandToolContext(ctx, in, resolver); err == nil {
				if release != nil {
					release()
				}
				t.Fatal("contexto inválido aceito")
			}
		})
	}
}

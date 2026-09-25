package jobs

import (
	"context"
	"reflect"
	"sync"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/eventctx"
)

type commandToolOriginBindingKey struct{}

type commandToolOriginBinding struct {
	manager       *Manager
	invocation    commandexecution.Invocation
	origin        commandEventOrigin
	resolveOrigin func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)
	inherited     bool
}

// CommandToolContext autentica uma invocação de comando para uma tool sem
// criar job, run ou dispatch. O contexto devolvido carrega somente a origem
// privada necessária para a execução da tool e para seus descendentes de
// proveniência; a autorização do comando não é herdada por outro comando.
func (m *Manager) CommandToolContext(ctx context.Context, in commandexecution.Invocation, resolveOrigin func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)) (context.Context, func(), error) {
	if m == nil || ctx == nil || ctx.Err() != nil || in.Envelope == nil || in.ID == "" || in.CommandID == "" || in.Source == "" {
		return nil, nil, ErrCommandJobDenied
	}
	if in.Envelope.InvocationID != in.ID || in.Envelope.CommandID == nil || *in.Envelope.CommandID != in.CommandID {
		return nil, nil, ErrCommandJobDenied
	}
	if _, inherited := ctx.Value(commandEventOriginKey{}).(commandEventOrigin); inherited {
		return nil, nil, ErrCommandJobDenied
	}
	if _, dispatched := ctx.Value(commandJobDispatchKey{}).(commandJobDispatch); dispatched {
		return nil, nil, ErrCommandJobDenied
	}
	if _, bound := ctx.Value(commandToolOriginBindingKey{}).(commandToolOriginBinding); bound {
		return nil, nil, ErrCommandJobDenied
	}
	if _, public := eventctx.From(ctx); public {
		return nil, nil, ErrCommandJobDenied
	}
	if !validCommandToolSource(in.Source) || !validCommandJobInvocation(ctx, in) {
		return nil, nil, ErrCommandJobDenied
	}
	marked, _, release, inherited, err := m.commandJobContext(ctx, in, resolveOrigin)
	if err != nil {
		if release != nil {
			release()
		}
		return nil, nil, err
	}
	if marked == nil || release == nil {
		if release != nil {
			release()
		}
		return nil, nil, ErrCommandJobDenied
	}
	marked, cancel := context.WithCancel(marked)
	origin, ok := marked.Value(commandEventOriginKey{}).(commandEventOrigin)
	if !ok || origin.runtimeIdentity == nil || origin.runtimeGuard == nil {
		cancel()
		release()
		return nil, nil, ErrCommandJobDenied
	}
	snapshot := in
	cloned := in.Envelope.Clone()
	snapshot.Envelope = &cloned
	binding := commandToolOriginBinding{manager: m, invocation: snapshot, origin: cloneCommandToolOrigin(origin), resolveOrigin: resolveOrigin, inherited: inherited}
	bound := context.WithValue(marked, commandToolOriginBindingKey{}, binding)
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			cancel()
			release()
		})
	}
	return bound, cleanup, nil
}

// ValidateCommandToolContext revalida a ligação preparada para uma tool sem
// emitir uma nova origem. A invocação precisa ser a mesma e a sessão/runtime
// ainda precisam estar vivos; cadeias herdadas consultam novamente sua raiz.
func (m *Manager) ValidateCommandToolContext(ctx context.Context, in commandexecution.Invocation) error {
	if m == nil || ctx == nil || ctx.Err() != nil || in.Envelope == nil {
		return ErrCommandJobDenied
	}
	binding, ok := ctx.Value(commandToolOriginBindingKey{}).(commandToolOriginBinding)
	if !ok || binding.manager != m || binding.invocation.Envelope == nil {
		return ErrCommandJobDenied
	}
	if binding.inherited {
		if binding.resolveOrigin == nil {
			return ErrCommandJobDenied
		}
	}
	if in.Envelope.InvocationID != in.ID || in.Envelope.CommandID == nil || *in.Envelope.CommandID != in.CommandID || !validCommandToolSource(in.Source) || !validCommandJobInvocation(ctx, in) || !reflect.DeepEqual(binding.invocation, in) {
		return ErrCommandJobDenied
	}
	origin, ok := ctx.Value(commandEventOriginKey{}).(commandEventOrigin)
	if !ok || origin.userID != binding.origin.userID || origin.userID != in.Principal.UserID || origin.rootType != binding.origin.rootType || origin.rootID != binding.origin.rootID || origin.chainID != binding.origin.chainID || origin.hasCommandHistory != binding.origin.hasCommandHistory || !origin.hasCommandHistory || len(origin.commandHistory) == 0 || !reflect.DeepEqual(origin.history, binding.origin.history) || !reflect.DeepEqual(origin.commandHistory, binding.origin.commandHistory) {
		return ErrCommandJobDenied
	}
	if origin.runtimeIdentity == nil || binding.origin.runtimeIdentity == nil || origin.runtimeGuard == nil || !reflect.DeepEqual(origin.runtimeIdentity, binding.origin.runtimeIdentity) || !origin.runtimeGuard(ctx, *origin.runtimeIdentity) || ctx.Err() != nil {
		return ErrCommandJobDenied
	}
	if binding.inherited {
		resolved, err := binding.resolveOrigin(ctx, in)
		if err != nil || !validCommandJobOrigin(resolved) || resolved.RootOriginType != binding.origin.rootType || resolved.RootOriginID != binding.origin.rootID {
			return ErrCommandJobDenied
		}
		if ctx.Err() != nil || !origin.runtimeGuard(ctx, *origin.runtimeIdentity) || ctx.Err() != nil {
			return ErrCommandJobDenied
		}
	}
	return nil
}

func cloneCommandToolOrigin(origin commandEventOrigin) commandEventOrigin {
	clone := origin
	clone.history = append([]string(nil), origin.history...)
	clone.commandHistory = append([]byte(nil), origin.commandHistory...)
	if origin.runtimeIdentity != nil {
		identity := *origin.runtimeIdentity
		clone.runtimeIdentity = &identity
	}
	return clone
}

func validCommandToolSource(source commandcatalog.Source) bool {
	switch source {
	case commandcatalog.KeyboardLocal,
		commandcatalog.KeyboardGlobal,
		commandcatalog.StreamDeck,
		commandcatalog.Palette,
		commandcatalog.UI,
		commandcatalog.Chat,
		commandcatalog.CLI:
		return true
	default:
		return false
	}
}

package jobs

import (
	"context"
	"encoding/json"

	"assistente/internal/commandjobactivation"
	"assistente/internal/database"
)

// A raiz viaja fora do payload público. O executor a propaga após queued e
// o sink autenticado de tasklist cria raízes internas; eventos legados
// continuam sem autoridade de ativação.
type commandEventOriginKey struct{}

type commandEventOrigin struct {
	userID, rootType, rootID, chainID string
	history                           []string
	commandHistory                    []byte
	hasCommandHistory                 bool
	runtimeIdentity                   *commandjobactivation.RuntimeIdentity
	runtimeGuard                      func(context.Context, commandjobactivation.RuntimeIdentity) bool
}

// O snapshot é JSON privado e profundo: o payload do EventBus nunca é fonte
// autorizada, e uma cadeia inválida continua presente para a rejeição
// canônica downstream, em vez de ser confundida com ausência.
func commandEventCommandHistorySnapshot(run *RunLog) ([]byte, bool) {
	if run == nil || run.Provenance == nil {
		return nil, false
	}
	value, ok := run.Provenance["command_chain_history"]
	if !ok {
		return nil, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		// Um valor não serializável continua marcado como presente e será
		// rejeitado pelo commandRunProvenance downstream.
		return []byte("null"), true
	}
	return append([]byte(nil), raw...), true
}

func (e *JobExecutor) withCommandEventOrigin(ctx context.Context, job *Job, trigger *TriggerContext, run *RunLog) context.Context {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return context.WithValue(ctx, commandEventOriginKey{}, commandEventOrigin{})
	}
	provenance := e.runProvenance(job, trigger, run)
	commandHistory, hasCommandHistory := commandEventCommandHistorySnapshot(run)
	origin := commandEventOrigin{
		userID: userID, rootType: run.RootOriginType, rootID: run.RootOriginID,
		chainID: provenance.ChainID, history: append([]string(nil), provenance.ChainHistory...),
		commandHistory:    append([]byte(nil), commandHistory...),
		hasCommandHistory: hasCommandHistory,
	}
	// A raiz autenticada continua vinculada à sessão que a publicou, mesmo
	// depois que um job intermediário terminou e emitiu outro evento.
	if parent, ok := ctx.Value(commandEventOriginKey{}).(commandEventOrigin); ok && parent.runtimeIdentity != nil {
		identity := *parent.runtimeIdentity
		origin.runtimeIdentity = &identity
		origin.runtimeGuard = parent.runtimeGuard
	}
	return context.WithValue(ctx, commandEventOriginKey{}, origin)
}

func inheritCommandEventOrigin(ctx context.Context, trigger *TriggerContext) {
	if ctx == nil || trigger == nil {
		return
	}
	origin, ok := ctx.Value(commandEventOriginKey{}).(commandEventOrigin)
	userID, err := database.RequireUserID(ctx)
	if !ok {
		return
	}
	if err != nil || userID != origin.userID || origin.userID == "" {
		// Um marker presente, mas foreign ou sem sessão, não pode ser
		// reinterpretado como a raiz manual recém-criada pelo chamador.
		trigger.RootOriginType, trigger.RootOriginID = "unknown", ""
		trigger.ChainID = ""
		trigger.ChainHistory = nil
		if trigger.Provenance != nil {
			delete(trigger.Provenance, "command_chain_history")
		}
		return
	}
	if origin.runtimeIdentity != nil && (origin.runtimeGuard == nil || !origin.runtimeGuard(context.WithoutCancel(ctx), *origin.runtimeIdentity)) {
		trigger.RootOriginType, trigger.RootOriginID = "unknown", ""
		trigger.ChainID = ""
		trigger.ChainHistory = nil
		if trigger.Provenance != nil {
			delete(trigger.Provenance, "command_chain_history")
		}
		return
	}
	// Inclusive unknown/external: um intermediário não pode lavar a raiz.
	trigger.RootOriginType, trigger.RootOriginID = origin.rootType, origin.rootID
	trigger.ChainID = origin.chainID
	if origin.history == nil {
		trigger.ChainHistory = nil
	} else {
		trigger.ChainHistory = append([]string{}, origin.history...)
	}
	if trigger.Provenance == nil {
		trigger.Provenance = make(map[string]any)
	}
	delete(trigger.Provenance, "command_chain_history")
	if origin.hasCommandHistory {
		var commandHistory any
		if err := json.Unmarshal(origin.commandHistory, &commandHistory); err != nil {
			commandHistory = nil
		}
		trigger.Provenance["command_chain_history"] = commandHistory
	}
}

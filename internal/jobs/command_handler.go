package jobs

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/tools"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrCommandJobDenied = errors.New("delegação de comando para job não autorizada")

// CommandHandlerConfig pertence ao bootstrap confiável. Authorize reconsulta
// política e grants exatos para a definição atual, fora do DispatchGate. Não
// pode conceder autorização interativa nova nem confiar apenas no envelope.
// Target é fixado pela montagem autoritativa, nunca pelo argumento público.
// Substituir Target exige reconstruir/publicar a versão do catálogo/handler.
// O grupo job_* do envelope descreve a origem job_service (run já existente),
// não é utilizado para inventar a identidade de um run que ainda não começou.
type CommandHandlerConfig struct {
	Definition commandcatalog.Definition
	Contract   commandcatalog.HandlerContract
	Target     CommandJobTarget
	// ProfileTarget is the exact slug prepared by the trusted host before its
	// decision. Templates must still resolve to this target on every attempt.
	ProfileTarget string
	Authorize     func(context.Context, commandexecution.Invocation, *Job) error
	// ResolveOrigin valida a raiz privada de uma invocação herdada. A callback
	// é chamada no worker, fora do DispatchGate, e novamente antes de cada
	// tentativa de efeito.
	ResolveOrigin func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)
	// Política da tool delegada, não pointers relativos ao argumento job_id.
	ToolSensitivePaths commandcatalog.SensitivePaths
}

// CommandJobOrigin é a raiz autenticada de uma cadeia reativa. O handler não
// transforma uma cadeia herdada em manual: a origem precisa ser resolvida por
// uma fonte autoritativa do App.
type CommandJobOrigin struct {
	RootOriginType string
	RootOriginID   string
}

type CommandJobTarget struct {
	DatabaseID            string
	Slug                  string
	DefinitionFingerprint string
}

type commandJobDispatchKey struct{}
type commandJobDispatch struct {
	jobID         string
	chainDepth    int
	paths         commandcatalog.SensitivePaths
	profileTarget string
	revalidate    func(context.Context) error
}

// CommandHandler conecta o executor comum ao runtime real. Start apenas
// entrega um handle; resolução SQL, autorização e execução ficam no worker.
// Não registra rota Wails, não publica catálogo e não altera grants/jobs.
func (m *Manager) CommandHandler(config CommandHandlerConfig) (commandexecution.Handler, error) {
	if m == nil || m.cfg.Repository == nil || m.cfg.CommandRuntimeIdentity == nil || config.Authorize == nil ||
		config.Contract.Classification != commandcatalog.HandlerJob || config.Contract.Effect != commandcatalog.Destructive || !config.Contract.HasMutableTarget || config.Definition.Decision != commandcatalog.Interactive ||
		commandcatalog.ValidateSensitivePaths(config.ToolSensitivePaths) != nil ||
		commandcatalog.ValidateDefinitionComplete(config.Definition, config.Contract) != nil {
		return commandexecution.Handler{}, ErrCommandJobDenied
	}
	target := config.Target
	digest, err := hex.DecodeString(target.DefinitionFingerprint)
	if !validJobDefinitionDatabaseID(target.DatabaseID) || target.Slug == "" || strings.TrimSpace(target.Slug) != target.Slug || err != nil || len(digest) != 32 || hex.EncodeToString(digest) != target.DefinitionFingerprint {
		return commandexecution.Handler{}, ErrCommandJobDenied
	}
	// Congela a política de redação sem reter slices mutáveis do bootstrap.
	paths := commandcatalog.SensitivePaths{Input: append([]string(nil), config.ToolSensitivePaths.Input...), Output: append([]string(nil), config.ToolSensitivePaths.Output...)}
	commandID := config.Definition.ID
	allowedSources := make(map[commandcatalog.Source]bool, len(config.Definition.AllowedSources))
	for _, source := range config.Definition.AllowedSources {
		allowedSources[source] = true
	}
	return commandexecution.Handler{Contract: config.Contract, Start: func(ctx context.Context, in commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
		if ctx == nil || ctx.Err() != nil || !allowedSources[in.Source] || in.Envelope == nil || in.CommandID != commandID || in.Envelope.CommandID == nil || *in.Envelope.CommandID != commandID || in.ID != in.Envelope.InvocationID {
			return commandexecution.ExecutionHandle{}, ErrCommandJobDenied
		}
		// keyboard.global só pode chegar ao handler por um adapter confiável que
		// preparou a combinação contra o job atual. A presença do snapshot é
		// verificada antes de ligar o worker; sua autoridade continua sendo
		// revalidada no commandJobContext, já dentro do contexto autenticado.
		if in.Source == commandcatalog.KeyboardGlobal {
			if _, ok := ctx.Value(preparedHotkeyDispatchKey{}).(preparedHotkeyBinding); !ok {
				return commandexecution.ExecutionHandle{}, ErrCommandJobDenied
			}
		}
		// A cópia que atravessa a goroutine não mantém aliases do chamador.
		raw, err := json.Marshal(in.Envelope)
		if err != nil {
			return commandexecution.ExecutionHandle{}, ErrCommandJobDenied
		}
		var envelope commandcontract.Envelope
		if json.Unmarshal(raw, &envelope) != nil {
			return commandexecution.ExecutionHandle{}, ErrCommandJobDenied
		}
		in.Envelope = &envelope
		if !validCommandJobInvocation(ctx, in) {
			return commandexecution.ExecutionHandle{}, ErrCommandJobDenied
		}
		work, cancel := context.WithCancel(ctx)
		done := make(chan commandexecution.Outcome, 1)
		var once sync.Once
		go func() {
			defer cancel()
			defer close(done)
			defer func() {
				if recover() != nil {
					done <- commandexecution.Outcome{Status: commandledger.OutcomeUnknown}
				}
			}()
			outcome := commandexecution.Outcome{Status: commandledger.Failed}
			job, err := m.PrepareCommandJob(work, target.DatabaseID)
			if err != nil {
				done <- outcome
				return
			}
			version, err := DefinitionFingerprint(job)
			if err != nil || version != target.DefinitionFingerprint || job.ID != target.Slug || config.Authorize(work, in, job) != nil {
				done <- outcome
				return
			}
			// O callback recebe uma cópia; mudanças concorrentes são relidas antes
			// do efeito. Não executamos o objeto que um authorizer poderia alterar.
			job, err = m.PrepareCommandJob(work, target.DatabaseID)
			if err != nil {
				done <- outcome
				return
			}
			current, err := DefinitionFingerprint(job)
			if err != nil || current != version {
				done <- outcome
				return
			}
			originCtx, trigger, release, inherited, err := m.commandJobContextForJob(work, in, job, config.ResolveOrigin)
			if release != nil {
				defer release()
			}
			if err != nil {
				done <- outcome
				return
			}
			revalidate := func(checkCtx context.Context) error {
				origin, ok := checkCtx.Value(commandEventOriginKey{}).(commandEventOrigin)
				if checkCtx.Err() != nil || !ok || origin.runtimeIdentity == nil || origin.runtimeGuard == nil || !origin.runtimeGuard(checkCtx, *origin.runtimeIdentity) {
					return ErrCommandJobDenied
				}
				latest, err := m.PrepareCommandJob(checkCtx, target.DatabaseID)
				if err != nil {
					if errors.Is(err, ErrCommandJobDenied) || errors.Is(err, gorm.ErrRecordNotFound) {
						return ErrCommandJobDenied
					}
					// Sem repetir efeitos quando a autoridade não pode ser lida,
					// mas sem classificar indisponibilidade SQL como grant negado.
					return permanentAttemptFailure(errors.New("job revalidation unavailable"), tools.ErrorKindUnavailable, "command_job_revalidation_unavailable")
				}
				latestVersion, err := DefinitionFingerprint(latest)
				if err != nil || latestVersion != version || config.Authorize(checkCtx, in, latest) != nil {
					return ErrCommandJobDenied
				}
				if inherited && config.ResolveOrigin != nil {
					resolved, err := config.ResolveOrigin(checkCtx, in)
					if err != nil || !validCommandJobOrigin(resolved) || resolved.RootOriginType != origin.rootType || resolved.RootOriginID != origin.rootID {
						return ErrCommandJobDenied
					}
				}
				// Authorize/ResolveOrigin podem atravessar uma revogação; confira
				// novamente antes de entregar o controle ao runtime.
				if checkCtx.Err() != nil || !origin.runtimeGuard(checkCtx, *origin.runtimeIdentity) {
					return ErrCommandJobDenied
				}
				if in.Source == commandcatalog.KeyboardGlobal {
					binding, ok := checkCtx.Value(preparedHotkeyDispatchKey{}).(preparedHotkeyBinding)
					if !ok || m.revalidatePreparedHotkeyForTarget(checkCtx, binding, target) != nil || !hotkeyWhenAllows(binding) {
						return ErrCommandJobDenied
					}
				}
				return nil
			}
			originCtx = context.WithValue(originCtx, commandJobDispatchKey{}, commandJobDispatch{jobID: job.DatabaseID, chainDepth: len(trigger.ChainHistory), paths: paths, profileTarget: config.ProfileTarget, revalidate: revalidate})
			run := m.executor.Execute(originCtx, job, trigger)
			outcome.Status = commandledger.OutcomeUnknown
			if work.Err() == nil && run != nil {
				if run.Status == RunStatusFailed && len(run.RunEvents) == 0 {
					// Rejeição de identidade/política anterior até mesmo à fila.
					outcome.Status = commandledger.Failed
				} else if persisted, err := m.cfg.Repository.GetRun(work, job.ID, run.RunID); err == nil && work.Err() == nil && persisted != nil && persisted.Status == run.Status {
					switch persisted.Status {
					case RunStatusCompleted:
						outcome.Status = commandledger.Succeeded
						outcome.Result, _ = json.Marshal(map[string]string{"job_id": job.DatabaseID, "run_id": run.RunID, "status": run.Status})
					case RunStatusFailed, RunStatusSkipped:
						outcome.Status = commandledger.Failed
					}
				}
			}
			if work.Err() != nil {
				outcome = commandexecution.Outcome{Status: commandledger.OutcomeUnknown}
			}
			done <- outcome
		}()
		return commandexecution.ExecutionHandle{ID: in.ID, Done: done, Cancel: func() { once.Do(cancel) }}, nil
	}}, nil
}

func validCommandJobInvocation(ctx context.Context, in commandexecution.Invocation) bool {
	e := in.Envelope
	owner, err := database.RequireUserID(ctx)
	if err != nil || owner != in.Principal.UserID || e.UserID == nil || *e.UserID != owner ||
		e.AuthContextType != commandcontract.AuthLocalSession || e.AuthContextID != in.Principal.SessionID ||
		e.ActorType != commandcontract.ActorUser || e.ActorID != owner || e.AuthGeneration == "" || e.SecurityGeneration == "" || e.AuthorizationDecisionID == nil || *e.AuthorizationDecisionID == "" ||
		e.SourceType == nil || string(*e.SourceType) != string(in.Source) || in.Source == commandcatalog.System || in.Source == commandcatalog.Event ||
		e.JobID != nil || e.JobSlug != nil || e.JobDefinitionFingerprint != nil || e.RunID != nil {
		return false
	}
	id, err := uuid.Parse(in.ID)
	if err != nil || id.Version() != 7 || id.Variant() != uuid.RFC4122 || id.String() != in.ID {
		return false
	}
	decision, err := uuid.Parse(*e.AuthorizationDecisionID)
	return err == nil && decision.Version() == 7 && decision.Variant() == uuid.RFC4122 && decision.String() == *e.AuthorizationDecisionID
}

func (m *Manager) commandJobContext(ctx context.Context, in commandexecution.Invocation, resolveOrigin func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)) (context.Context, *TriggerContext, func(), bool, error) {
	return m.commandJobContextForJob(ctx, in, nil, resolveOrigin)
}

func (m *Manager) commandJobContextForJob(ctx context.Context, in commandexecution.Invocation, job *Job, resolveOrigin func(context.Context, commandexecution.Invocation) (CommandJobOrigin, error)) (context.Context, *TriggerContext, func(), bool, error) {
	if _, inherited := ctx.Value(commandEventOriginKey{}).(commandEventOrigin); inherited {
		return nil, nil, nil, false, ErrCommandJobDenied
	}
	e := in.Envelope
	identity, watch, release, err := m.captureTasklistRuntimeIdentity(ctx, *e.UserID)
	expected := commandjobactivation.RuntimeIdentity{UserID: *e.UserID, AuthContextType: string(e.AuthContextType), AuthContextID: e.AuthContextID, AuthGeneration: e.AuthGeneration, SecurityGeneration: e.SecurityGeneration}
	if err != nil || watch == nil || watch.Err() != nil || !sameTasklistRuntimeIdentity(identity, expected) {
		return nil, nil, release, false, ErrCommandJobDenied
	}
	if e.Provenance == nil {
		return nil, nil, release, false, ErrCommandJobDenied
	}
	var prepared preparedHotkeyBinding
	if in.Source == commandcatalog.KeyboardGlobal {
		if job == nil {
			return nil, nil, release, false, ErrCommandJobDenied
		}
		var ok bool
		prepared, ok = ctx.Value(preparedHotkeyDispatchKey{}).(preparedHotkeyBinding)
		if !ok {
			return nil, nil, release, false, ErrCommandJobDenied
		}
		current, revalidateErr := m.revalidateHotkeyBinding(ctx, prepared)
		currentFingerprint, fingerprintErr := DefinitionFingerprint(current)
		if revalidateErr != nil || current == nil || fingerprintErr != nil || currentFingerprint != prepared.definitionFingerprint || current.DatabaseID != job.DatabaseID || current.ID != job.ID || !hotkeyWhenAllows(prepared) {
			return nil, nil, release, false, ErrCommandJobDenied
		}
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(*e.Provenance, &doc) != nil {
		return nil, nil, release, false, ErrCommandJobDenied
	}
	history, err := commandcontract.DecodeCommandChainHistory(doc["command_chain_history"])
	if err != nil || len(history) == 0 || history[len(history)-1].CommandID != in.CommandID || history[len(history)-1].InvocationID != in.ID {
		return nil, nil, release, false, ErrCommandJobDenied
	}
	var chainID string
	var ancestors []string
	if json.Unmarshal(doc["_chain_id"], &chainID) != nil || !validCommandOriginString(chainID) || json.Unmarshal(doc["_chain_history"], &ancestors) != nil || ancestors == nil {
		return nil, nil, release, false, ErrCommandJobDenied
	}
	for _, ancestor := range ancestors {
		if !validCommandOriginString(ancestor) {
			return nil, nil, release, false, ErrCommandJobDenied
		}
	}
	inherited := chainID != in.ID || len(ancestors) != 0 || len(history) != 1
	rootType, rootID := "manual", in.ID
	if in.Source == commandcatalog.KeyboardLocal || in.Source == commandcatalog.KeyboardGlobal || in.Source == commandcatalog.StreamDeck {
		rootType = "user_hotkey"
	}
	if inherited {
		if resolveOrigin == nil {
			return nil, nil, release, false, ErrCommandJobDenied
		}
		resolved, err := resolveOrigin(ctx, in)
		if err != nil || !validCommandJobOrigin(resolved) {
			return nil, nil, release, false, ErrCommandJobDenied
		}
		rootType, rootID = resolved.RootOriginType, resolved.RootOriginID
	}
	origin := commandEventOrigin{userID: *e.UserID, rootType: rootType, rootID: rootID, chainID: chainID, history: append([]string(nil), ancestors...), commandHistory: append([]byte(nil), doc["command_chain_history"]...), hasCommandHistory: true, runtimeIdentity: &expected, runtimeGuard: m.tasklistRuntimeGuard(*e.UserID, expected)}
	marked := context.WithValue(ctx, commandEventOriginKey{}, origin)
	trigger := &TriggerContext{Type: TriggerManual, EventPayload: map[string]any{}}
	if in.Source == commandcatalog.KeyboardGlobal {
		trigger.Type = TriggerHotkey
		trigger.Keys = prepared.keys
		trigger.When = prepared.when
	}
	inheritCommandEventOrigin(marked, trigger)
	trigger.ChainID = chainID
	trigger.ChainHistory = append([]string(nil), ancestors...)
	if trigger.RootOriginType != rootType {
		return nil, nil, release, false, ErrCommandJobDenied
	}
	return marked, trigger, release, inherited, nil
}

func (m *Manager) revalidatePreparedHotkeyForTarget(ctx context.Context, binding preparedHotkeyBinding, target CommandJobTarget) error {
	current, err := m.revalidateHotkeyBinding(ctx, binding)
	if err != nil || current == nil || current.DatabaseID != target.DatabaseID || current.ID != target.Slug {
		return ErrCommandJobDenied
	}
	fingerprint, err := DefinitionFingerprint(current)
	if err != nil || fingerprint != target.DefinitionFingerprint {
		return ErrCommandJobDenied
	}
	return nil
}

// hotkeyWhenAllows avalia a condição capturada pelo adapter. A expressão não
// vem do envelope: o snapshot privado já foi conferido contra o trigger
// persistido. O helper é chamado no ingresso e novamente antes de cada
// tentativa, incluindo retries.
func hotkeyWhenAllows(binding preparedHotkeyBinding) bool {
	if binding.when == "" {
		return true
	}
	ok, err := EvaluateCondition(binding.when, &TemplateContext{Event: map[string]any{}, Now: time.Now()})
	return err == nil && ok
}

func validCommandOriginString(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
}

func validCommandJobOrigin(origin CommandJobOrigin) bool {
	if !validCommandOriginString(origin.RootOriginID) {
		return false
	}
	switch origin.RootOriginType {
	case "manual", "cron", "interval", "user_hotkey", "internal_event":
		return true
	default:
		return false
	}
}

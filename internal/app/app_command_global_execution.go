package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"assistente/controllers"
	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandforeground"
	"assistente/internal/commandledger"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"github.com/google/uuid"
)

type commandGlobalOccurrence struct {
	ctx           context.Context
	current       func() bool
	validate      func(context.Context) error
	binding       commandGlobalBinding
	versions      commandexecution.Versions
	originVersion string
	foreground    *commandforeground.Snapshot
	handler       commandexecution.Handler
	decisionTitle string
	admission     chan bool
	admitted      bool
}

type commandGlobalExecution struct {
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	service     *commandexecution.Service
	instanceID  string
	occurrences map[string]*commandGlobalOccurrence
}

func (p *commandProductRuntime) globalOccurrence(id string) (*commandGlobalOccurrence, bool) {
	if p == nil || p.globalExecution == nil || id == "" {
		return nil, false
	}
	s := p.globalExecution
	s.mu.Lock()
	o := s.occurrences[id]
	s.mu.Unlock()
	return o, s.ctx.Err() == nil && o != nil && o.ctx != nil && o.ctx.Err() == nil && o.current != nil && o.current()
}

func (p *commandProductRuntime) addGlobalOccurrence(id string, o *commandGlobalOccurrence) error {
	s := p.globalExecution
	if s == nil {
		return commandexecution.ErrDenied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil || len(s.occurrences) >= 64 || s.occurrences[id] != nil || o == nil || o.ctx == nil || o.current == nil || o.ctx.Err() != nil || !o.current() {
		return commandexecution.ErrStale
	}
	s.occurrences[id] = o
	return nil
}

func (p *commandProductRuntime) removeGlobalOccurrence(id string) {
	if p.globalExecution == nil {
		return
	}
	p.globalExecution.mu.Lock()
	delete(p.globalExecution.occurrences, id)
	p.globalExecution.mu.Unlock()
}

func (a *App) newCommandGlobalExecutor(p *commandProductRuntime, base commandexecution.Config, host *commandexecution.HostState) (*commandGlobalExecution, error) {
	instance, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	state := &commandGlobalExecution{instanceID: instance.String(), occurrences: map[string]*commandGlobalOccurrence{}}
	state.ctx, state.cancel = context.WithCancel(context.Background())
	config := base
	config.Source = commandcatalog.KeyboardGlobal
	envelope := *base.Envelope
	envelope.Snapshot = func(ctx context.Context, owner auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
		e, err := base.Envelope.Snapshot(ctx, owner, candidate)
		if err != nil {
			return e, err
		}
		o, ok := p.globalOccurrence(candidate.InvocationID)
		if !ok || owner != p.principal || candidate.TriggerType != string(commandcatalog.KeyboardGlobal) || string(candidate.TriggerSpec) != string(o.binding.Trigger) {
			return commandcontract.Envelope{}, commandexecution.ErrDenied
		}
		current, err := host.Snapshot(ctx, owner)
		if err != nil || current != o.versions || !current.Unlocked {
			return commandcontract.Envelope{}, commandexecution.ErrStale
		}
		e.SourceInstanceID = commandStringPointer(state.instanceID)
		e.SourceEventID = commandStringPointer(candidate.InvocationID)
		e.ObserverType = commandStringPointer(string(commandcatalog.KeyboardGlobal))
		e.ObservedTriggerType = commandStringPointer(string(commandcatalog.KeyboardGlobal))
		e.ForegroundSnapshot, err = foregroundSummaryRaw(o.foreground)
		if err != nil {
			return commandcontract.Envelope{}, err
		}
		return e, nil
	}
	envelope.Authorize = func(ctx context.Context, principal auth.LocalSessionPrincipal, e commandcontract.Envelope, d commandcatalog.Definition) error {
		o, ok := p.globalOccurrence(e.InvocationID)
		if !ok || principal != p.principal || d.ID != o.binding.CommandID || !d.AllowsSource(commandcatalog.KeyboardGlobal) ||
			e.SourceType == nil || *e.SourceType != commandcontract.SourceKeyboardGlobal || !p.dependenciesMatch(a) {
			return commandexecution.ErrDenied
		}
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, principal)
		if err != nil || current != principal {
			return commandexecution.ErrDenied
		}
		var role string
		if err := database.DB().WithContext(ctx).Table("users").Select("role").Where("id = ? AND is_active = ?", principal.UserID, true).Scan(&role).Error; err != nil || (role != database.UserRoleUser && role != database.UserRoleAdmin) {
			return commandexecution.ErrDenied
		}
		return nil
	}
	envelope.AuthorizeLookup = func(context.Context, auth.LocalSessionPrincipal, commandledger.FullRecord) error {
		return commandexecution.ErrDenied
	}
	envelope.DecisionBody = func(d commandcatalog.Definition, e commandcontract.Envelope) (string, error) {
		o, ok := p.globalOccurrence(e.InvocationID)
		if !ok || d.ID != commandGlobalJobID || o.binding.CommandID != d.ID {
			return "", commandexecution.ErrDenied
		}
		metadata := d.Presentation.Locales[p.getDeckLocale()]
		if metadata.Name == "" {
			metadata = d.Presentation.Locales["en"]
		}
		return metadata.Name + "\n" + metadata.Description + "\n\n“" + o.decisionTitle + "”", nil
	}
	config.Envelope = &envelope
	state.service, err = a.newCommandDesktopExecutor(config, host)
	if err != nil {
		state.cancel()
	}
	return state, err
}

func (p *commandProductRuntime) prepareGlobalOccurrence(ctx context.Context, binding commandGlobalBinding, current func() bool) (*commandGlobalOccurrence, error) {
	if ctx == nil || ctx.Err() != nil || current == nil || !current() || p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) {
		return nil, commandexecution.ErrDenied
	}
	if err := p.refreshCommandJobProjection(ctx); err != nil {
		return nil, err
	}
	contextual := false
	var captured commandOriginContext
	read := func() (commandexecution.Versions, string, bool) {
		configuration, _, versions, err := p.host.ResolutionSnapshot(ctx, p.principal)
		if err != nil || !versions.Unlocked {
			return versions, "", false
		}
		required := configuration.RequiredFacts(binding.Identity)
		contextual = len(required) > 0
		originContext, factsErr := p.commandOriginFacts(ctx, commandcatalog.KeyboardGlobal, required)
		if factsErr != nil {
			return versions, "", false
		}
		captured = originContext
		resolved, err := configuration.Resolve(binding.Identity, originContext.facts, nil)
		return versions, originContext.version, err == nil && globalBindingMatches(binding, resolved)
	}
	versions, originVersion, ok := read()
	// A condition becoming false is not stale configuration. Never rebuild
	// or persist a projection merely because this physical press does not match.
	if !ok && contextual {
		return nil, commandexecution.ErrDenied
	}
	if !ok {
		p.projectionMu.Lock()
		err := p.app.rebuildCommandLifecycleProjection(ctx, false)
		p.projectionMu.Unlock()
		if err != nil {
			return nil, err
		}
		versions, originVersion, ok = read()
	}
	if !ok || !current() {
		return nil, commandexecution.ErrDenied
	}
	return &commandGlobalOccurrence{ctx: ctx, current: current, binding: binding, versions: versions, originVersion: originVersion, foreground: cloneForegroundSnapshot(captured.foreground), admission: make(chan bool, 1)}, nil
}

func globalBindingMatches(binding commandGlobalBinding, result commandbindings.Result) bool {
	if result.Status != commandbindings.Selected || result.CommandID != binding.CommandID || result.ArgumentsKey != string(binding.Arguments) || result.ExecutionScopeKey != "global" {
		return false
	}
	for _, id := range result.BindingIDs {
		if id == binding.ID {
			return true
		}
	}
	return false
}

func (a *App) startGlobalCommandJob(ctx context.Context, invocation commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
	p := a.commandProduct.Load()
	o, ok := p.globalOccurrence(invocation.ID)
	if !ok || invocation.Source != commandcatalog.KeyboardGlobal || invocation.CommandID != commandGlobalJobID || o.handler.Start == nil {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrDenied
	}
	return o.handler.Start(ctx, invocation)
}

// This API can only veto/admit a native occurrence already minted privately.
// It cannot construct a trigger, select a target, or create execution authority.
func (a *App) AdmitGlobalCommandOccurrence(invocationID string, allowed bool) bool {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return false
	}
	o, ok := p.globalOccurrence(invocationID)
	if !ok || o.binding.CommandID != commandGlobalJobID {
		return false
	}
	p.globalExecution.mu.Lock()
	defer p.globalExecution.mu.Unlock()
	if o.admitted {
		return false
	}
	o.admitted = true
	o.admission <- allowed
	return true
}

func (a *App) dispatchCommandJobHotkey(ctx context.Context, occurrence jobs.CommandHotkeyOccurrence) error {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return fmt.Errorf("global job authentication: %w", err)
	}
	if ctx == nil || p.globalExecution == nil {
		return commandexecution.ErrDenied
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(p.globalExecution.ctx, cancel)
	defer stop()
	ctx, err = occurrence.Context(database.WithUserID(ctx, p.principal.UserID))
	if err != nil {
		return fmt.Errorf("global job occurrence: %w", err)
	}
	b := occurrence.Binding()
	binding, err := newCommandGlobalBinding(commandGlobalJobID, b.Keys, b.BindingFingerprint, map[string]string{"job_id": b.JobDatabaseID})
	if err != nil {
		return err
	}
	o, err := p.prepareGlobalOccurrence(ctx, binding, occurrence.Current)
	if err != nil {
		return fmt.Errorf("global job projection: %w", err)
	}
	definition, exists := p.registry.Lookup(commandGlobalJobID)
	if !exists {
		return commandexecution.ErrDenied
	}
	contract := commandcatalog.HandlerContract{Effect: definition.Effect, HasMutableTarget: definition.HasMutableTarget, Classification: definition.HandlerClassification, Route: definition.HandlerRoute}
	o.handler, err = a.newCommandJobHandler(ctx, definition, contract, b.JobDatabaseID, commandcatalog.SensitivePaths{})
	if err != nil {
		return fmt.Errorf("global job handler preparation: %w", err)
	}
	job, err := a.jobMgr.PrepareCommandJob(ctx, b.JobDatabaseID)
	if err != nil {
		return err
	}
	o.decisionTitle = job.Name
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	invocationID := id.String()
	if err := p.addGlobalOccurrence(invocationID, o); err != nil {
		return err
	}
	defer p.removeGlobalOccurrence(invocationID)
	if a.emitter == nil {
		return commandexecution.ErrDenied
	}
	a.emitter.Emit("command:global-job-admission", map[string]string{"invocationId": invocationID})
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case allowed := <-o.admission:
		if !allowed {
			return commandexecution.ErrDenied
		}
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return commandexecution.ErrDenied
	}
	if err := occurrence.Validate(ctx); err != nil {
		return err
	}
	_, err = p.globalExecution.service.ExecuteEnvelope(ctx, "", commandexecution.EnvelopeCandidate{
		InvocationID: invocationID, CorrelationID: invocationID, TriggerType: string(commandcatalog.KeyboardGlobal), TriggerSpec: binding.Trigger, Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		return fmt.Errorf("global job execution: %w", err)
	}
	return err
}

func (a *App) dispatchCommandProfileHotkey(ctx context.Context, occurrence controllers.ProfileHotkeyOccurrence) error {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return err
	}
	if ctx == nil || p.globalExecution == nil {
		return commandexecution.ErrDenied
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(p.globalExecution.ctx, cancel)
	defer stop()
	ctx = database.WithUserID(ctx, p.principal.UserID)
	if err := occurrence.Validate(ctx); err != nil {
		return err
	}
	b := occurrence.Binding()
	binding, err := newCommandGlobalBinding(commandGlobalVoiceID, b.Hotkey, b.Fingerprint, map[string]any{"profile_slug": b.ProfileSlug, "trigger_type": b.TriggerType, "bring_to_front": b.BringToFront})
	if err != nil {
		return err
	}
	captured, err := p.workspaceMgr.CommandSnapshot()
	if err != nil {
		return err
	}
	current := func() bool {
		actual, err := p.workspaceMgr.CommandSnapshot()
		return err == nil && actual == captured && occurrence.Current()
	}
	o, err := p.prepareGlobalOccurrence(ctx, binding, current)
	if err != nil {
		return err
	}
	o.validate = occurrence.Validate
	var invocationID string
	reservation, err := a.beginCommandUI(p, commandGlobalVoiceID, func(res commandui.Reservation) (commandexecution.EnvelopeCandidate, error) {
		invocationID = res.InvocationID
		if err := p.addGlobalOccurrence(invocationID, o); err != nil {
			return commandexecution.EnvelopeCandidate{}, err
		}
		return commandexecution.EnvelopeCandidate{InvocationID: invocationID, CorrelationID: invocationID, TriggerType: string(commandcatalog.KeyboardGlobal), TriggerSpec: binding.Trigger, Arguments: json.RawMessage(`{}`)}, nil
	}, func(run context.Context, candidate commandexecution.EnvelopeCandidate) (commandledger.FullRecord, error) {
		defer p.removeGlobalOccurrence(candidate.InvocationID)
		stop := context.AfterFunc(ctx, func() { cancelGlobalVoiceInvocation(p, candidate.InvocationID) })
		defer stop()
		return p.globalExecution.service.ExecuteEnvelope(run, "", candidate)
	}, current)
	if err != nil {
		p.removeGlobalOccurrence(invocationID)
		return err
	}
	if a.emitter == nil {
		_ = a.CancelUICommand(reservation.Ticket)
		return commandexecution.ErrDenied
	}
	a.emitter.Emit("command:global-ui-reservation", map[string]any{"ticket": reservation.Ticket, "invocationId": reservation.InvocationID, "commandId": reservation.CommandID,
		"profile_slug": b.ProfileSlug, "trigger_type": b.TriggerType, "bring_to_front": b.BringToFront})
	result, err := a.GetUICommandResult(reservation.Ticket)
	if err == nil && result.Status == string(commandledger.Succeeded) && b.Global && b.BringToFront && a.windowPort != nil {
		a.windowPort.Show()
	}
	return err
}

func cancelGlobalVoiceInvocation(p *commandProductRuntime, id string) {
	p.mu.Lock()
	var selected *commandUIRun
	for _, run := range p.uiRuns {
		if run.reservation.InvocationID == id {
			selected = run
			break
		}
	}
	p.mu.Unlock()
	if selected != nil {
		selected.cancel()
		_ = p.ui.Cancel(p.uiOwner(), selected.reservation.Ticket)
	}
}

// GlobalVoiceHandoff binds the UI target to the admitted native occurrence.
// Event metadata is only a hint for capturing a surface before an async wait.
type GlobalVoiceHandoff struct {
	commandui.Handoff
	ProfileSlug  string `json:"profile_slug"`
	TriggerType  string `json:"trigger_type"`
	BringToFront bool   `json:"bring_to_front"`
}

func (a *App) TakeGlobalVoiceCommand(ticket string) (GlobalVoiceHandoff, error) {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return GlobalVoiceHandoff{}, err
	}
	if run.reservation.CommandID != commandGlobalVoiceID {
		return GlobalVoiceHandoff{}, commandexecution.ErrDenied
	}
	o, ok := p.globalOccurrence(run.reservation.InvocationID)
	if !ok || o.binding.CommandID != commandGlobalVoiceID {
		return GlobalVoiceHandoff{}, commandexecution.ErrDenied
	}
	if o.validate == nil || o.validate(o.ctx) != nil {
		cancelGlobalVoiceInvocation(p, run.reservation.InvocationID)
		return GlobalVoiceHandoff{}, commandexecution.ErrStale
	}
	var result GlobalVoiceHandoff
	if err := json.Unmarshal(o.binding.Arguments, &result); err != nil {
		return GlobalVoiceHandoff{}, err
	}
	result.Handoff, err = a.TakeUICommand(ticket)
	if err != nil {
		return GlobalVoiceHandoff{}, err
	}
	if _, ok := p.globalOccurrence(run.reservation.InvocationID); !ok || o.validate(o.ctx) != nil {
		cancelGlobalVoiceInvocation(p, run.reservation.InvocationID)
		return GlobalVoiceHandoff{}, commandexecution.ErrStale
	}
	return result, nil
}

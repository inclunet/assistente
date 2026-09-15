package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandexecution"
	"assistente/internal/commandruntime"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
)

type appCommandLifecycleRuntime struct {
	app    *App
	inputs CommandLifecycleMountInputs
	epochs *commandsecurity.EpochService

	mu          sync.Mutex
	generations map[string]commandsecurity.EpochSnapshot
	published   map[string]commandruntime.Projection
	enabled     commandruntime.Generation
	readiness   commandruntime.Snapshot
}

func newAppCommandLifecycleRuntime(a *App, inputs CommandLifecycleMountInputs) (*appCommandLifecycleRuntime, error) {
	if a == nil || inputs.Host == nil || inputs.Execution.Epochs == nil || inputs.Host.Epochs() != inputs.Execution.Epochs {
		return nil, commandruntime.ErrInvalidConfiguration
	}
	return &appCommandLifecycleRuntime{
		app:         a,
		inputs:      inputs,
		epochs:      inputs.Execution.Epochs,
		generations: make(map[string]commandsecurity.EpochSnapshot),
		published:   make(map[string]commandruntime.Projection),
	}, nil
}

func (r *appCommandLifecycleRuntime) config() commandruntime.Config {
	return commandruntime.Config{
		Authenticator:  r,
		Recovery:       r,
		Projector:      r,
		Publisher:      r,
		Inputs:         r,
		Core:           r,
		Generations:    r,
		Readiness:      appCommandLifecycleReadiness{runtime: r},
		CleanupTimeout: time.Second,
	}
}

func (r *appCommandLifecycleRuntime) Authenticate(ctx context.Context) error {
	_, err := r.currentPrincipal(ctx)
	return err
}

func (r *appCommandLifecycleRuntime) Recover(context.Context, commandruntime.Generation) error {
	// I14.3 ligará receipts/outbox/reconciliação durável. Por enquanto a porta é
	// explícita e inerte: não mascara indisponibilidade como mapa pronto.
	return nil
}

func (r *appCommandLifecycleRuntime) Project(ctx context.Context, generation commandruntime.Generation) (commandruntime.Projection, error) {
	snapshot, err := r.snapshot(generation)
	if err != nil {
		return commandruntime.Projection{}, err
	}
	principal := auth.LocalSessionPrincipal{UserID: snapshot.UserID, SessionID: snapshot.SessionID}
	versions, err := r.inputs.Host.Snapshot(ctx, principal)
	if err != nil {
		return commandruntime.Projection{}, err
	}
	if !versions.Unlocked || versions.Registry == "" || versions.GlobalConfig == "" || versions.ActiveLayers == "" {
		return commandruntime.Projection{}, commandruntime.ErrNotReady
	}
	entries := len(r.inputs.Execution.Handlers)
	if entries <= 0 {
		return commandruntime.Projection{}, commandruntime.ErrNotReady
	}
	return commandruntime.Projection{
		Generation: generation,
		Entries:    entries,
		Value: appCommandLifecycleProjection{
			Principal: principal,
			Versions:  versions,
			Source:    string(r.inputs.Execution.Source),
		},
	}, nil
}

func (r *appCommandLifecycleRuntime) Publish(_ context.Context, projection commandruntime.Projection) error {
	if projection.Entries <= 0 || projection.Generation.Value == "" {
		return commandruntime.ErrNotReady
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.generations[projection.Generation.Value]; !ok {
		return commandruntime.ErrTransition
	}
	r.published[projection.Generation.Value] = projection
	return nil
}

func (r *appCommandLifecycleRuntime) Clear(_ context.Context, generation commandruntime.Generation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.published, generation.Value)
	if r.enabled == generation {
		r.enabled = commandruntime.Generation{}
	}
	return nil
}

func (r *appCommandLifecycleRuntime) SetEnabled(ctx context.Context, generation commandruntime.Generation, enabled bool) error {
	if !enabled {
		r.mu.Lock()
		if r.enabled == generation {
			r.enabled = commandruntime.Generation{}
		}
		r.mu.Unlock()
		return nil
	}
	if err := r.ValidateCurrent(ctx, generation, commandruntime.BoundaryBeforeEnable); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.published[generation.Value]; !ok {
		return commandruntime.ErrNotReady
	}
	r.enabled = generation
	return nil
}

func (r *appCommandLifecycleRuntime) ValidateCurrent(ctx context.Context, generation commandruntime.Generation, _ commandruntime.Boundary) error {
	snapshot, err := r.snapshot(generation)
	if err != nil {
		return err
	}
	return r.revalidate(ctx, snapshot)
}

func (r *appCommandLifecycleRuntime) Authorize(ctx context.Context, generation commandruntime.Generation, boundary commandruntime.Boundary) error {
	return r.ValidateCurrent(ctx, generation, boundary)
}

func (r *appCommandLifecycleRuntime) Commit(ctx context.Context, generation commandruntime.Generation, commit func() error) error {
	if commit == nil {
		return commandruntime.ErrInvalidConfiguration
	}
	snapshot, err := r.snapshot(generation)
	if err != nil {
		return err
	}
	return r.epochs.Admit(ctx, snapshot, func(ctx context.Context) error {
		return r.revalidate(ctx, snapshot)
	}, commit)
}

func (r *appCommandLifecycleRuntime) Begin(ctx context.Context) (commandruntime.Generation, error) {
	snapshot, err := r.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		principal, err := r.currentPrincipal(ctx)
		if err != nil {
			return "", "", err
		}
		return principal.UserID, principal.SessionID, nil
	})
	if err != nil {
		return commandruntime.Generation{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return commandruntime.Generation{}, err
	}
	generation := commandruntime.Generation{Value: id.String()}
	r.mu.Lock()
	r.generations[generation.Value] = snapshot
	r.mu.Unlock()
	return generation, nil
}

func (r *appCommandLifecycleRuntime) Invalidate(_ context.Context, generation commandruntime.Generation, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.generations, generation.Value)
	delete(r.published, generation.Value)
	if r.enabled == generation {
		r.enabled = commandruntime.Generation{}
	}
	return nil
}

type appCommandLifecycleReadiness struct {
	runtime *appCommandLifecycleRuntime
}

func (p appCommandLifecycleReadiness) Publish(_ context.Context, snapshot commandruntime.Snapshot) error {
	if p.runtime == nil {
		return commandruntime.ErrInvalidConfiguration
	}
	p.runtime.mu.Lock()
	p.runtime.readiness = snapshot
	p.runtime.mu.Unlock()
	return nil
}

func (r *appCommandLifecycleRuntime) currentPrincipal(ctx context.Context) (auth.LocalSessionPrincipal, error) {
	if r == nil || r.app == nil || ctx == nil {
		return auth.LocalSessionPrincipal{}, commandruntime.ErrInvalidConfiguration
	}
	if err := ctx.Err(); err != nil {
		return auth.LocalSessionPrincipal{}, err
	}
	r.app.authMu.RLock()
	defer r.app.authMu.RUnlock()
	current := r.app.currentAuthUser
	if current == nil || r.app.currentUserID == "" || current.UserID != r.app.currentUserID || current.SessionID == "" {
		return auth.LocalSessionPrincipal{}, commandruntime.ErrNotReady
	}
	return auth.LocalSessionPrincipal{UserID: current.UserID, SessionID: current.SessionID}, nil
}

func (r *appCommandLifecycleRuntime) snapshot(generation commandruntime.Generation) (commandsecurity.EpochSnapshot, error) {
	if generation.Value == "" {
		return commandsecurity.EpochSnapshot{}, commandruntime.ErrInvalidConfiguration
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot, ok := r.generations[generation.Value]
	if !ok {
		return commandsecurity.EpochSnapshot{}, commandsecurity.ErrStaleEpoch
	}
	return snapshot, nil
}

func (r *appCommandLifecycleRuntime) revalidate(ctx context.Context, snapshot commandsecurity.EpochSnapshot) error {
	if ctx == nil {
		return commandruntime.ErrInvalidConfiguration
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	principal := auth.LocalSessionPrincipal{UserID: snapshot.UserID, SessionID: snapshot.SessionID}
	current, err := r.currentPrincipal(ctx)
	if err != nil {
		return err
	}
	if current != principal {
		return commandsecurity.ErrStaleEpoch
	}
	versions, err := r.inputs.Host.Snapshot(ctx, principal)
	if err != nil {
		return err
	}
	if !versions.Unlocked {
		return commandruntime.ErrNotReady
	}
	return nil
}

type appCommandLifecycleProjection struct {
	Principal auth.LocalSessionPrincipal
	Versions  commandexecution.Versions
	Source    string
}

func (p appCommandLifecycleProjection) MarshalJSON() ([]byte, error) {
	type projection appCommandLifecycleProjection
	if p.Principal.UserID == "" || p.Principal.SessionID == "" || p.Source == "" {
		return nil, errors.New("projeção de comandos incompleta")
	}
	return json.Marshal(projection(p))
}

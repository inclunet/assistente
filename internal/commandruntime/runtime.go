// Package commandruntime orquestra o ciclo de vida do runtime de comandos.
//
// A implementação é deliberadamente agnóstica ao App e aos domínios de
// autenticação, configuração e dispositivos. As portas abaixo devem ser
// adaptadas pelo bootstrap confiável; não há catálogo, mapa ou identidade
// implícita neste pacote.
package commandruntime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"time"
)

var (
	ErrInvalidConfiguration = errors.New("configuração do runtime de comandos inválida")
	ErrNotReady             = errors.New("runtime de comandos não está pronto")
	ErrStopped              = errors.New("runtime de comandos encerrado")
	ErrTransition           = errors.New("transição do runtime de comandos falhou")
)

type State string

const (
	StateCold          State = "cold"
	StateBootstrapping State = "bootstrapping"
	StateReady         State = "ready"
	StateFailed        State = "failed"
	StateStopping      State = "stopping"
	StateStopped       State = "stopped"
)

// Generation é fornecida pela autoridade de segurança do processo. O runtime
// só compara o valor exato e nunca gera epochs, contadores ou mutexes próprios.
type Generation struct{ Value string }

func (g Generation) valid() bool { return g.Value != "" }

// Projection é o resultado detached do carregador/projetor. Entries é uma
// prova produzida pelo projetor confiável: Publish nunca considera uma
// projeção vazia pronta. Value transporta a configuração concreta do domínio
// sem permitir que este pacote a interprete ou autorize.
type Projection struct {
	Generation Generation
	Entries    int
	Value      any
}

type Snapshot struct {
	State            State
	Generation       Generation
	Published        bool
	PublishedEntries int
	LastError        string
}

type Boundary string

const (
	BoundaryAfterRecovery   Boundary = "after-recovery"
	BoundaryAfterProjection Boundary = "after-projection"
	BoundaryBeforePublish   Boundary = "before-publish"
	BoundaryAfterPublish    Boundary = "after-publish"
	BoundaryBeforeEnable    Boundary = "before-enable"
	BoundaryBeforeReady     Boundary = "before-ready"
)

// AuthenticationPort autentica/revalida o contexto corrente. Não recebe IDs
// do payload e não devolve uma identidade projetada para o runtime.
type AuthenticationPort interface {
	Authenticate(context.Context) error
}

type RecoveryPort interface {
	Recover(context.Context, Generation) error
}

type ProjectionPort interface {
	Project(context.Context, Generation) (Projection, error)
}

type PublicationPort interface {
	// Publish deve associar a publicação à geração recebida e revalidá-la no
	// adapter real; uma publicação fora do gate compartilhado não é suficiente
	// para declarar I14.2/I14.3 concluídos.
	Publish(context.Context, Projection) error
	Clear(context.Context, Generation) error
}

type InputPort interface {
	// enabled=true deve apenas preparar a entrada vinculada à geração; o core
	// confirma a geração efetiva em Commit. O adapter real deve recusar geração
	// obsoleta sob o mesmo gate usado pela publicação.
	SetEnabled(context.Context, Generation, bool) error
}

// CorePort adapta o mesmo gate/serviço de segurança usado pela execução. A
// validação é curta e local; adapters não devem chamar o runtime de dentro de
// seus callbacks. Commit é a confirmação final atômica da geração no core.
type CorePort interface {
	ValidateCurrent(context.Context, Generation, Boundary) error
	Authorize(context.Context, Generation, Boundary) error
	// Commit valida a geração sob o gate compartilhado e executa callback curto
	// enquanto a decisão continua válida. O callback não pode fazer I/O, UI,
	// cofre, rede ou reentrar no runtime.
	Commit(context.Context, Generation, func() error) error
}

// GenerationPort deve adaptar a autoridade de segurança já existente no
// processo. O runtime não cria epochs, mutexes, contadores ou geração local.
type GenerationPort interface {
	Begin(context.Context) (Generation, error)
	Invalidate(context.Context, Generation, string) error
}

// ReadinessPort é observabilidade, não autorização. Uma falha ao publicar o
// estado impede que a transição seja anunciada como pronta. Publish deve ser
// curto e não reentrante: não pode chamar Bootstrap, Reset, Stop ou WaitReady.
type ReadinessPort interface {
	Publish(context.Context, Snapshot) error
}

type Config struct {
	Authenticator  AuthenticationPort
	Recovery       RecoveryPort
	Projector      ProjectionPort
	Publisher      PublicationPort
	Inputs         InputPort
	Core           CorePort
	Generations    GenerationPort
	Readiness      ReadinessPort
	CleanupTimeout time.Duration
}

func (c Config) validate() error {
	if nilPort(c.Authenticator) || nilPort(c.Recovery) || nilPort(c.Projector) ||
		nilPort(c.Publisher) || nilPort(c.Inputs) || nilPort(c.Core) ||
		nilPort(c.Generations) || nilPort(c.Readiness) {
		return ErrInvalidConfiguration
	}
	if c.CleanupTimeout < 0 {
		return ErrInvalidConfiguration
	}
	return nil
}

func nilPort(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

type operation struct {
	ctx    context.Context
	kind   operationKind
	resp   chan error
	reason string
}

type operationKind uint8

const (
	opBootstrap operationKind = iota + 1
	opReset
	opStop
)

type status struct {
	Snapshot
	change chan struct{}
}

// Controller serializa todas as transições em um único worker. Nenhuma porta
// é chamada enquanto um lock de estado estiver retido; UI, cofre e rede podem
// bloquear ou cancelar sem deadlock com o observador de readiness.
type Controller struct {
	config           Config
	rootCtx          context.Context
	cancel           context.CancelFunc
	ops              chan operation
	done             chan struct{}
	state            atomic.Pointer[status]
	stopOnce         atomic.Bool
	activeCancel     atomic.Value // context.CancelFunc
	activeGeneration atomic.Value // Generation
}

func New(config Config) (*Controller, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	root, cancel := context.WithCancel(context.Background())
	c := &Controller{
		config:  config,
		rootCtx: root,
		cancel:  cancel,
		ops:     make(chan operation),
		done:    make(chan struct{}),
	}
	c.state.Store(&status{Snapshot: Snapshot{State: StateCold}, change: make(chan struct{})})
	c.activeCancel.Store(context.CancelFunc(func() {}))
	c.activeGeneration.Store(Generation{})
	go c.loop()
	return c, nil
}

func (c *Controller) loop() {
	defer close(c.done)
	for op := range c.ops {
		var err error
		switch op.kind {
		case opBootstrap:
			err = c.bootstrap(op.ctx)
		case opReset:
			err = c.reset(op.ctx, op.reason)
		case opStop:
			err = c.stop(op.ctx)
			op.resp <- err
			return
		default:
			err = ErrTransition
		}
		op.resp <- err
	}
}

func (c *Controller) submit(ctx context.Context, kind operationKind, reason string) error {
	if c == nil || ctx == nil {
		return ErrInvalidConfiguration
	}
	if c.stopOnce.Load() && kind != opStop {
		return ErrStopped
	}
	response := make(chan error, 1)
	op := operation{ctx: ctx, kind: kind, reason: reason, resp: response}
	select {
	case c.ops <- op:
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return ErrStopped
	}
	select {
	case err := <-response:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		// Stop publica a resposta antes de fechar done. Drenar o canal evita
		// transformar uma parada bem-sucedida em erro por seleção aleatória
		// entre dois cases já prontos.
		select {
		case err := <-response:
			return err
		default:
			return ErrStopped
		}
	}
}

func (c *Controller) Bootstrap(ctx context.Context) error { return c.submit(ctx, opBootstrap, "") }

// Reset invalida o snapshot atual e apaga somente a publicação em memória.
// É usado para logout/troca de identidade e não exclui persistência.
func (c *Controller) Reset(ctx context.Context, reason string) error {
	if c == nil || ctx == nil {
		return ErrInvalidConfiguration
	}
	if reason == "" {
		reason = "reset"
	}
	// O cancelamento e a invalidação ocorrem antes de enfileirar o reset. Isso
	// impede que um projetor bloqueado publique a geração anterior enquanto a
	// operação de logout aguarda a fila.
	c.cancelActive()
	var immediate error
	if generation := c.currentActiveGeneration(); generation.valid() {
		cleanup, cancelCleanup := c.cleanupContext()
		immediate = c.config.Generations.Invalidate(cleanup, generation, reason)
		cancelCleanup()
	}
	queued := c.submit(ctx, opReset, reason)
	if immediate != nil && queued != nil {
		return errors.Join(immediate, queued)
	}
	if queued != nil {
		return queued
	}
	return immediate
}

func (c *Controller) Stop(ctx context.Context) error {
	if c == nil || ctx == nil {
		return ErrInvalidConfiguration
	}
	if c.stopOnce.CompareAndSwap(false, true) {
		// Cancela imediatamente a operação em andamento. O worker ainda
		// executará a limpeza final sem depender do contexto já cancelado.
		c.cancelActive()
		c.cancel()
	}
	return c.submit(ctx, opStop, "shutdown")
}

func (c *Controller) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{State: StateStopped, LastError: ErrInvalidConfiguration.Error()}
	}
	return c.state.Load().Snapshot
}

func (c *Controller) WaitReady(ctx context.Context) error {
	if c == nil || ctx == nil {
		return ErrInvalidConfiguration
	}
	for {
		current := c.state.Load()
		if current.State == StateReady {
			return nil
		}
		if current.State == StateStopped {
			return ErrStopped
		}
		if current.State == StateFailed {
			if current.LastError == "" {
				return ErrNotReady
			}
			return fmt.Errorf("%w: %s", ErrNotReady, current.LastError)
		}
		select {
		case <-current.change:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *Controller) setState(ctx context.Context, next Snapshot) error {
	return c.setStateWithSafety(ctx, next, false)
}

func (c *Controller) setStateWithSafety(ctx context.Context, next Snapshot, safety bool) error {
	if next.State == StateReady && !safety {
		return c.setReadyState(ctx, next)
	}
	previous := c.state.Load()
	if err := c.config.Readiness.Publish(ctx, next); err != nil && !safety {
		return err
	} else if err != nil {
		nextState := &status{Snapshot: next, change: make(chan struct{})}
		c.state.Store(nextState)
		close(previous.change)
		return err
	}
	nextState := &status{Snapshot: next, change: make(chan struct{})}
	c.state.Store(nextState)
	close(previous.change)
	return nil
}

func (c *Controller) setReadyState(ctx context.Context, next Snapshot) error {
	if err := c.config.Readiness.Publish(ctx, next); err != nil {
		return err
	}
	// O callback pode ignorar cancelamento. Nunca converter seu retorno em
	// Ready sem confirmar o contexto ainda ativo e a geração no core.
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.config.Core.ValidateCurrent(ctx, next.Generation, BoundaryBeforeReady); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.config.Core.Commit(ctx, next.Generation, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.storeState(next)
		return nil
	})
}

func (c *Controller) storeState(next Snapshot) {
	previous := c.state.Load()
	nextState := &status{Snapshot: next, change: make(chan struct{})}
	c.state.Store(nextState)
	close(previous.change)
}

func (c *Controller) bootstrap(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalidConfiguration
	}
	ctx, cancel := c.operationContext(ctx)
	defer cancel()
	c.setActiveCancel(cancel)
	defer c.clearActiveCancel(cancel)
	previous := c.state.Load().Generation
	if err := c.setState(ctx, Snapshot{State: StateBootstrapping}); err != nil {
		return c.fail(ctx, Generation{}, err)
	}
	if previous.valid() {
		if err := c.config.Generations.Invalidate(ctx, previous, "transition"); err != nil {
			return c.fail(ctx, previous, err)
		}
	}
	generation, err := c.config.Generations.Begin(ctx)
	if err != nil || !generation.valid() {
		if err == nil {
			err = ErrInvalidConfiguration
		}
		return c.fail(ctx, generation, err)
	}
	c.setActiveGeneration(generation)
	defer c.clearActiveGeneration(generation)
	if err = c.config.Authenticator.Authenticate(ctx); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = ctx.Err(); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = c.config.Recovery.Recover(ctx, generation); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = ctx.Err(); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = c.config.Core.ValidateCurrent(ctx, generation, BoundaryAfterRecovery); err != nil {
		return c.fail(ctx, generation, err)
	}
	projection, err := c.config.Projector.Project(ctx, generation)
	if err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = ctx.Err(); err != nil {
		return c.fail(ctx, generation, err)
	}
	if projection.Generation != generation || projection.Entries <= 0 || nilPort(projection.Value) {
		return c.fail(ctx, generation, fmt.Errorf("%w: projeção vazia ou de geração divergente", ErrNotReady))
	}
	if err = c.config.Core.ValidateCurrent(ctx, generation, BoundaryAfterProjection); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = c.config.Core.Authorize(ctx, generation, BoundaryBeforePublish); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = c.config.Publisher.Publish(ctx, projection); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = ctx.Err(); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = c.config.Core.ValidateCurrent(ctx, generation, BoundaryAfterPublish); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = c.config.Core.Authorize(ctx, generation, BoundaryBeforeEnable); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = c.config.Inputs.SetEnabled(ctx, generation, true); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = ctx.Err(); err != nil {
		return c.fail(ctx, generation, err)
	}
	if err = c.setState(ctx, Snapshot{State: StateReady, Generation: generation, Published: true, PublishedEntries: projection.Entries}); err != nil {
		cleanup, cancelCleanup := c.cleanupContext()
		defer cancelCleanup()
		_ = c.config.Inputs.SetEnabled(cleanup, generation, false)
		_ = c.config.Publisher.Clear(cleanup, generation)
		_ = c.config.Generations.Invalidate(cleanup, generation, "readiness-publication")
		return c.fail(cleanup, generation, err)
	}
	return nil
}

func (c *Controller) reset(ctx context.Context, reason string) error {
	if ctx == nil {
		return ErrInvalidConfiguration
	}
	operationCtx, cancel := c.operationContext(ctx)
	defer cancel()
	// A limpeza de reset é best-effort e usa contexto bounded independente;
	// observar operationCtx preserva a vinculação ao cancelamento do App sem
	// transformar cancelamento em retenção de publicação antiga.
	_ = operationCtx.Err()
	current := c.state.Load().Generation
	cleanup, cancelCleanup := c.cleanupContext()
	defer cancelCleanup()
	if err := c.config.Inputs.SetEnabled(cleanup, current, false); err != nil {
		return c.fail(cleanup, current, err)
	}
	if err := c.config.Publisher.Clear(cleanup, current); err != nil {
		return c.fail(cleanup, current, err)
	}
	if current.valid() {
		if err := c.config.Generations.Invalidate(cleanup, current, reason); err != nil {
			return c.fail(cleanup, current, err)
		}
	}
	return c.setStateWithSafety(cleanup, Snapshot{State: StateCold}, true)
}

func (c *Controller) stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	current := c.state.Load().Generation
	first := c.setStateWithSafety(ctx, Snapshot{State: StateStopping, Generation: current}, true)
	cleanup, cancelCleanup := c.cleanupContext()
	defer cancelCleanup()
	if err := c.config.Inputs.SetEnabled(cleanup, current, false); err != nil {
		if first == nil {
			first = err
		}
	}
	if err := c.config.Publisher.Clear(cleanup, current); err != nil && first == nil {
		first = err
	}
	if current.valid() {
		if err := c.config.Generations.Invalidate(cleanup, current, "shutdown"); err != nil && first == nil {
			first = err
		}
	}
	final := Snapshot{State: StateStopped}
	if first != nil {
		final.LastError = first.Error()
	}
	if err := c.setStateWithSafety(cleanup, final, true); err != nil && first == nil {
		first = err
	}
	c.cancel()
	return first
}

func (c *Controller) fail(ctx context.Context, generation Generation, cause error) error {
	cleanup, cancelCleanup := c.cleanupContext()
	defer cancelCleanup()
	_ = c.config.Inputs.SetEnabled(cleanup, generation, false)
	_ = c.config.Publisher.Clear(cleanup, generation)
	if generation.valid() {
		_ = c.config.Generations.Invalidate(cleanup, generation, "failure")
	}
	if cause == nil {
		cause = ErrTransition
	}
	_ = c.setStateWithSafety(cleanup, Snapshot{State: StateFailed, LastError: cause.Error()}, true)
	return cause
}

func (c *Controller) cleanupContext() (context.Context, context.CancelFunc) {
	timeout := c.config.CleanupTimeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	return context.WithTimeout(context.Background(), timeout)
}

func (c *Controller) operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	derived, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.rootCtx, cancel)
	return derived, func() {
		stop()
		cancel()
	}
}

func (c *Controller) setActiveCancel(cancel context.CancelFunc) {
	c.activeCancel.Store(cancel)
}

func (c *Controller) clearActiveCancel(context.CancelFunc) {
	// O worker é serializado: a próxima operação só pode começar depois que a
	// operação atual limpou este slot. A função recebida não é comparável em Go.
	c.activeCancel.Store(context.CancelFunc(func() {}))
}

func (c *Controller) cancelActive() {
	if value := c.activeCancel.Load(); value != nil {
		value.(context.CancelFunc)()
	}
}

func (c *Controller) setActiveGeneration(generation Generation) {
	c.activeGeneration.Store(generation)
}

func (c *Controller) clearActiveGeneration(generation Generation) {
	if current := c.currentActiveGeneration(); current == generation {
		c.activeGeneration.Store(Generation{})
	}
}

func (c *Controller) currentActiveGeneration() Generation {
	if value := c.activeGeneration.Load(); value != nil {
		return value.(Generation)
	}
	return Generation{}
}

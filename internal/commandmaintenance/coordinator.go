// Package commandmaintenance coordena a manutenção da instância. Ele não
// conhece jobs, claims ou eventos concretos: cada domínio fornece uma porta
// interna autenticada e bounded. Isso evita um segundo loop e mantém a ordem
// outbox -> jobs exigida por D8/D11/AEP-0074-B.
package commandmaintenance

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrInvalid            = errors.New("coordenador de manutenção inválido")
	ErrInvalidBatchResult = errors.New("resultado de lote de manutenção inválido")
	ErrAlreadyRunning     = errors.New("manutenção da instância já está em execução")
)

const DefaultBatchSize = 128

// Policy é fornecida pelo dono da configuração. O pacote não lê config.json
// nem cria uma fonte paralela de settings.
type Policy struct {
	JobRetention          time.Duration
	RunsPerJobKeep        int
	ChatRetention         time.Duration
	VacuumMinFreeBytes    int64
	InvocationRetention   time.Duration
	InvocationsPerUser    int
	InvocationsSystemKeep int
	ActivationRetention   time.Duration
	ActivationsPerUser    int
	LeaseDuration         time.Duration
	BatchSize             int
}

// Validate exige uma política completa quando o coordinator está habilitado.
// Os zeros canônicos da política legada continuam sendo aceitos para o cap de
// runs e para o cap opcional de chat. Não há semântica zero para retenção de
// invocações/ativações, lease ou cap de ativações: esses campos são positivos.
func (p Policy) Validate() error {
	if p.JobRetention < 0 || p.RunsPerJobKeep < 0 || p.ChatRetention < 0 ||
		p.InvocationRetention <= 0 || p.ActivationRetention <= 0 || p.LeaseDuration <= 0 ||
		p.VacuumMinFreeBytes < 0 || p.InvocationsPerUser <= 0 || p.InvocationsSystemKeep <= 0 ||
		p.ActivationsPerUser <= 0 || p.BatchSize < 0 || p.BatchSize > DefaultBatchSize {
		return ErrInvalid
	}
	return nil
}

// BatchResult é o contrato de operações canceláveis. More não autoriza um
// loop interno: indica apenas que uma próxima chamada agendada ainda é útil.
type BatchResult struct {
	Processed int
	More      bool
}

// OutboxPort é implementada pelo consumidor de eventos. Drain deve respeitar
// ctx e não deve iniciar goroutine/loop paralelo pertencente ao coordenador.
type OutboxPort interface {
	RequeueExpiredLeases(context.Context, int) (int, bool, error)
	Drain(context.Context, int) (BatchResult, error)
}

// RecoveryPort representa uma única fatia bounded de recuperação de um
// domínio. A implementação deve comprovar internamente o owner/ciclo; não há
// callback booleano de segurança nesta interface.
type RecoveryPort interface {
	Recover(context.Context, int) (BatchResult, error)
}

// RetentionPort remove somente dados autorizados pela política do seu domínio.
type RetentionPort interface {
	Retain(context.Context, Policy) (int64, error)
}

// ToolRetentionPort mantém a ordem D5/AEP-0074-B dentro do único coordinator.
// A implementação concreta deve executar cada operação no banco escopado e
// respeitar o contexto; não há chamada alternativa agregada que possa pular
// uma limpeza legada.
type ToolRetentionPort interface {
	CleanOldDryRuns(context.Context, Policy) (int64, error)
	CleanOrphanChat(context.Context, Policy) (int64, error)
	CleanOldChat(context.Context, Policy) (int64, error)
}

// CompactionPort encapsula a manutenção física global do SQLite.
type CompactionPort interface {
	Compact(context.Context, int64) error
}

type Ports struct {
	Outbox       OutboxPort
	Decisions    RecoveryPort
	Invocations  RecoveryPort
	Claims       RecoveryPort
	Jobs         RetentionPort
	Tools        ToolRetentionPort
	InvocationDB RetentionPort
	Activations  RetentionPort
	Compaction   CompactionPort
}

type Report struct {
	OutboxRequeued     int
	OutboxDrained      bool
	MoreOutbox         bool
	Recovered          int
	MoreRecovery       bool
	JobsDeleted        int64
	ToolsDeleted       int64
	InvocationsDeleted int64
	ActivationsDeleted int64
	Compacted          bool
}

// Coordinator é o único dono da sequência de manutenção. Run é uma única
// passagem; uma chamada posterior pode continuar um lote que retornou More.
type Coordinator struct {
	ports   Ports
	mu      sync.Mutex
	running bool
}

func New(ports Ports) (*Coordinator, error) {
	if ports.Outbox == nil || ports.Decisions == nil || ports.Invocations == nil || ports.Claims == nil || ports.Jobs == nil || ports.Tools == nil || ports.InvocationDB == nil || ports.Activations == nil || ports.Compaction == nil {
		return nil, ErrInvalid
	}
	return &Coordinator{ports: ports}, nil
}

func (c *Coordinator) Run(ctx context.Context, policy Policy) (Report, error) {
	if c == nil || ctx == nil {
		return Report{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if err := policy.Validate(); err != nil {
		return Report{}, err
	}
	batch := policy.BatchSize
	if batch == 0 {
		batch = DefaultBatchSize
	}
	if batch > DefaultBatchSize {
		return Report{}, ErrInvalid
	}
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return Report{}, ErrAlreadyRunning
	}
	c.running = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	var report Report
	// Essa ordem é deliberada: a retenção de jobs só ocorre depois que a
	// barreira de replay foi reencaminhada e drenada.
	if n, more, err := c.ports.Outbox.RequeueExpiredLeases(ctx, batch); err != nil {
		return report, err
	} else {
		if err := validateBatchResult(BatchResult{Processed: n, More: more}, batch); err != nil {
			return report, err
		}
		report.OutboxRequeued = n
		report.MoreOutbox = more
	}
	outboxResult, err := c.ports.Outbox.Drain(ctx, batch)
	if err != nil {
		return report, err
	}
	if err := validateBatchResult(outboxResult, batch); err != nil {
		return report, err
	}
	report.MoreOutbox = report.MoreOutbox || outboxResult.More
	report.OutboxDrained = !report.MoreOutbox

	for _, port := range []RecoveryPort{c.ports.Decisions, c.ports.Invocations, c.ports.Claims} {
		batchResult, err := port.Recover(ctx, batch)
		if err != nil {
			return report, err
		}
		if err := validateBatchResult(batchResult, batch); err != nil {
			return report, err
		}
		report.Recovered += batchResult.Processed
		report.MoreRecovery = report.MoreRecovery || batchResult.More
	}
	// Nenhuma exclusão/compactação pode ocorrer enquanto um domínio ainda
	// tiver lote pendente. A próxima passagem retoma pelo cursor próprio.
	if report.MoreOutbox || report.MoreRecovery {
		return report, nil
	}

	if deleted, err := c.ports.Jobs.Retain(ctx, policy); err != nil {
		return report, err
	} else {
		report.JobsDeleted = deleted
	}
	for _, clean := range []func(context.Context, Policy) (int64, error){
		c.ports.Tools.CleanOldDryRuns,
		c.ports.Tools.CleanOrphanChat,
		c.ports.Tools.CleanOldChat,
	} {
		deleted, err := clean(ctx, policy)
		if err != nil {
			return report, err
		}
		report.ToolsDeleted += deleted
	}
	for _, item := range []struct {
		port RetentionPort
		dest *int64
	}{
		{c.ports.InvocationDB, &report.InvocationsDeleted},
		{c.ports.Activations, &report.ActivationsDeleted},
	} {
		deleted, err := item.port.Retain(ctx, policy)
		if err != nil {
			return report, err
		}
		*item.dest = deleted
	}
	if err := c.ports.Compaction.Compact(ctx, policy.VacuumMinFreeBytes); err != nil {
		return report, err
	}
	report.Compacted = true
	return report, nil
}

func validateBatchResult(result BatchResult, limit int) error {
	if limit <= 0 || result.Processed < 0 || result.Processed > limit {
		return ErrInvalidBatchResult
	}
	return nil
}

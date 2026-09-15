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
	ErrInvalid                = errors.New("coordenador de manutenção inválido")
	ErrInvalidBatchResult     = errors.New("resultado de lote de manutenção inválido")
	ErrInvalidRetentionResult = errors.New("resultado de retenção inválido")
	ErrAlreadyRunning         = errors.New("manutenção da instância já está em execução")
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

// HeartbeatPort compartilha o owner, a política e a cadência da manutenção.
// Implementações executam uma fatia, sem timers ou releitura de settings.
type HeartbeatPort interface {
	Heartbeat(context.Context, Policy) (BatchResult, error)
}

// RetentionPort remove somente dados autorizados pela política do seu domínio.
type RetentionPort interface {
	Retain(context.Context, Policy) (int64, error)
}

// RetentionResult é o resultado de uma retenção bounded. More indica que a
// porta encontrou trabalho adicional para uma passagem futura; não autoriza
// um loop interno no coordinator.
type RetentionResult struct {
	Deleted int64
	More    bool
}

// BoundedRetentionPort é opcional. Portas novas podem expor a operação
// bounded sem quebrar implementações legadas de RetentionPort.
type BoundedRetentionPort interface {
	RetainBatch(context.Context, Policy) (RetentionResult, error)
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

// BoundedToolRetentionPort é opcional para adapters que precisam paginar o
// escopo de usuários. Cada operação mantém seu próprio cursor e pode informar
// que a próxima passagem ainda é necessária, sem criar um loop no coordinator.
type BoundedToolRetentionPort interface {
	CleanOldDryRunsBatch(context.Context, Policy) (RetentionResult, error)
	CleanOrphanChatBatch(context.Context, Policy) (RetentionResult, error)
	CleanOldChatBatch(context.Context, Policy) (RetentionResult, error)
}

// CompactionPort encapsula a manutenção física global do SQLite.
type CompactionPort interface {
	Compact(context.Context, int64) error
}

type Ports struct {
	Heartbeat    HeartbeatPort // opcional para montagens sem claims de job
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
	HeartbeatProcessed int
	MoreHeartbeat      bool
	OutboxRequeued     int
	OutboxDrained      bool
	MoreOutbox         bool
	Recovered          int
	MoreRecovery       bool
	MoreRetention      bool
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
	if c.ports.Heartbeat != nil {
		result, err := c.ports.Heartbeat.Heartbeat(ctx, policy)
		if validationErr := validateBatchResult(result, batch); validationErr != nil {
			return report, validationErr
		}
		report.HeartbeatProcessed, report.MoreHeartbeat = result.Processed, result.More
		if err != nil {
			report.MoreHeartbeat = true
			return report, err
		}
	}
	// Essa ordem é deliberada: a retenção de jobs só ocorre depois que a
	// barreira de replay foi reencaminhada e drenada.
	if err := ctx.Err(); err != nil {
		return report, err
	}
	n, more, err := c.ports.Outbox.RequeueExpiredLeases(ctx, batch)
	result := BatchResult{Processed: n, More: more}
	if validationErr := validateBatchResult(result, batch); validationErr != nil {
		return report, validationErr
	}
	report.OutboxRequeued = n
	report.MoreOutbox = more
	if err != nil {
		report.MoreOutbox = true
		return report, err
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	outboxResult, err := c.ports.Outbox.Drain(ctx, batch)
	if validationErr := validateBatchResult(outboxResult, batch); validationErr != nil {
		return report, validationErr
	}
	report.MoreOutbox = report.MoreOutbox || outboxResult.More
	report.OutboxDrained = !report.MoreOutbox
	if err != nil {
		report.OutboxDrained = false
		report.MoreOutbox = true
		return report, err
	}

	for _, port := range []RecoveryPort{c.ports.Decisions, c.ports.Invocations, c.ports.Claims} {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		batchResult, err := port.Recover(ctx, batch)
		if validationErr := validateBatchResult(batchResult, batch); validationErr != nil {
			return report, validationErr
		}
		report.Recovered += batchResult.Processed
		report.MoreRecovery = report.MoreRecovery || batchResult.More
		if err != nil {
			report.MoreRecovery = true
			return report, err
		}
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	// Nenhuma exclusão/compactação pode ocorrer enquanto um domínio ainda
	// tiver lote pendente. A próxima passagem retoma pelo cursor próprio.
	if report.MoreHeartbeat || report.MoreOutbox || report.MoreRecovery {
		return report, nil
	}

	if err := ctx.Err(); err != nil {
		return report, err
	}
	jobsResult, err := retain(ctx, c.ports.Jobs, policy)
	report.JobsDeleted = jobsResult.Deleted
	report.MoreRetention = jobsResult.More
	if err != nil {
		report.MoreRetention = true
		return report, err
	}
	for _, clean := range []func(context.Context, ToolRetentionPort, Policy) (RetentionResult, error){
		cleanOldDryRuns,
		cleanOrphanChat,
		cleanOldChat,
	} {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		result, err := clean(ctx, c.ports.Tools, policy)
		if validationErr := validateDeletedCount(result.Deleted); validationErr != nil {
			return report, validationErr
		}
		report.ToolsDeleted += result.Deleted
		report.MoreRetention = report.MoreRetention || result.More
		if err != nil {
			report.MoreRetention = true
			return report, err
		}
	}
	for _, item := range []struct {
		port RetentionPort
		dest *int64
	}{
		{c.ports.InvocationDB, &report.InvocationsDeleted},
		{c.ports.Activations, &report.ActivationsDeleted},
	} {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		result, err := retain(ctx, item.port, policy)
		*item.dest = result.Deleted
		report.MoreRetention = report.MoreRetention || result.More
		if err != nil {
			report.MoreRetention = true
			return report, err
		}
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if report.MoreRetention {
		return report, nil
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if err := c.ports.Compaction.Compact(ctx, policy.VacuumMinFreeBytes); err != nil {
		return report, err
	}
	report.Compacted = true
	return report, nil
}

func cleanOldDryRuns(ctx context.Context, port ToolRetentionPort, policy Policy) (RetentionResult, error) {
	if bounded, ok := port.(BoundedToolRetentionPort); ok {
		result, err := bounded.CleanOldDryRunsBatch(ctx, policy)
		if validationErr := validateDeletedCount(result.Deleted); validationErr != nil {
			return RetentionResult{}, validationErr
		}
		if err != nil {
			result.More = true
		}
		return result, err
	}
	deleted, err := port.CleanOldDryRuns(ctx, policy)
	if validationErr := validateDeletedCount(deleted); validationErr != nil {
		return RetentionResult{}, validationErr
	}
	result := RetentionResult{Deleted: deleted}
	if err != nil {
		result.More = true
	}
	return result, err
}

func cleanOrphanChat(ctx context.Context, port ToolRetentionPort, policy Policy) (RetentionResult, error) {
	if bounded, ok := port.(BoundedToolRetentionPort); ok {
		result, err := bounded.CleanOrphanChatBatch(ctx, policy)
		if validationErr := validateDeletedCount(result.Deleted); validationErr != nil {
			return RetentionResult{}, validationErr
		}
		if err != nil {
			result.More = true
		}
		return result, err
	}
	deleted, err := port.CleanOrphanChat(ctx, policy)
	if validationErr := validateDeletedCount(deleted); validationErr != nil {
		return RetentionResult{}, validationErr
	}
	result := RetentionResult{Deleted: deleted}
	if err != nil {
		result.More = true
	}
	return result, err
}

func cleanOldChat(ctx context.Context, port ToolRetentionPort, policy Policy) (RetentionResult, error) {
	if bounded, ok := port.(BoundedToolRetentionPort); ok {
		result, err := bounded.CleanOldChatBatch(ctx, policy)
		if validationErr := validateDeletedCount(result.Deleted); validationErr != nil {
			return RetentionResult{}, validationErr
		}
		if err != nil {
			result.More = true
		}
		return result, err
	}
	deleted, err := port.CleanOldChat(ctx, policy)
	if validationErr := validateDeletedCount(deleted); validationErr != nil {
		return RetentionResult{}, validationErr
	}
	result := RetentionResult{Deleted: deleted}
	if err != nil {
		result.More = true
	}
	return result, err
}

func validateBatchResult(result BatchResult, limit int) error {
	if limit <= 0 || result.Processed < 0 || result.Processed > limit {
		return ErrInvalidBatchResult
	}
	return nil
}

func retain(ctx context.Context, port RetentionPort, policy Policy) (RetentionResult, error) {
	if bounded, ok := port.(BoundedRetentionPort); ok {
		result, err := bounded.RetainBatch(ctx, policy)
		if validationErr := validateDeletedCount(result.Deleted); validationErr != nil {
			return RetentionResult{}, validationErr
		}
		if err != nil {
			result.More = true
		}
		return result, err
	}
	deleted, err := port.Retain(ctx, policy)
	if validationErr := validateDeletedCount(deleted); validationErr != nil {
		return RetentionResult{}, validationErr
	}
	result := RetentionResult{Deleted: deleted}
	if err != nil {
		result.More = true
	}
	return result, err
}

func validateDeletedCount(deleted int64) error {
	if deleted < 0 {
		return ErrInvalidRetentionResult
	}
	return nil
}

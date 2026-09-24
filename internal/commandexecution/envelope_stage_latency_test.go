package commandexecution

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"assistente/internal/commandledger"
	"gorm.io/gorm/logger"
)

// TestExecuteEnvelopeStageLatency mede caminhos reais do executor e do ledger
// sem impor metas de tempo. Ative explicitamente com
// COMMAND_EXECUTION_STAGE_LATENCY=1. SQLite, autenticação, executor, gate e
// ledger são reais; Snapshot/Resolve/Authorize/AwaitQueue/Start são ports
// controladas da fixture, não implementação produtiva do resolvedor. As
// amostras isoladas do ledger não são fases exatas da chamada integrada.
// Não há scheduler/GUI: fila e handler são imediatos; renderização não é medida.
func TestExecuteEnvelopeStageLatency(t *testing.T) {
	if os.Getenv("COMMAND_EXECUTION_STAGE_LATENCY") != "1" {
		t.Skip("qualificação opt-in: COMMAND_EXECUTION_STAGE_LATENCY=1")
	}
	const warmup, samples = 20, 200
	ctx := context.Background()
	f := newEnvelopePipelineFixture(t)
	f.db.Logger = logger.Default.LogMode(logger.Silent)
	sqlDB, err := f.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(4)
	configureLatencySQLite(t, sqlDB, 4)

	measurements := make([]time.Duration, samples)
	for i := 0; i < warmup+samples; i++ {
		candidate := f.trigger(newTestUUID())
		start := time.Now()
		record, err := f.service.ExecuteEnvelope(ctx, f.token, candidate)
		elapsed := time.Since(start)
		if err != nil || record.Status != commandledger.Succeeded {
			t.Fatalf("execução integrada %d: status=%s err=%v", i, record.Status, err)
		}
		if i >= warmup {
			measurements[i-warmup] = elapsed
		}
	}
	if got := int(f.startCalls.Load()); got != warmup+samples {
		t.Fatalf("handler Start calls=%d; esperado %d", got, warmup+samples)
	}
	reportDurations(t, "executor_end_to_end", measurements)

	// Reserva e CAS do ledger são medidos sobre o SQLite real. A preparação
	// autenticada/assinada ocorre fora da janela para não atribuí-la ao ledger.
	ledgerSamples := make([]time.Duration, samples)
	for i := range ledgerSamples {
		candidate := f.candidate(newTestUUID(), "pipe.read", `{}`)
		prepared, prior, err := f.service.prepareEnvelope(ctx, f.token, candidate)
		if err != nil || prior != nil {
			t.Fatalf("preparação da reserva %d: prior=%v err=%v", i, prior != nil, err)
		}
		request := commandledger.EnvelopeRequest{
			Envelope: prepared.envelope, Mode: prepared.mode,
			ArgumentsFingerprint: prepared.argumentsFingerprint,
			InputFingerprint:     prepared.inputFingerprint, ExpiresAt: prepared.expires,
			Risk: string(prepared.definition.Risk),
		}
		start := time.Now()
		reservation, err := f.store.ReserveEnvelope(ctx, request)
		if err == nil && reservation.Created {
			var changed bool
			for _, transition := range [][2]commandledger.Status{
				{commandledger.Evaluating, commandledger.Queued},
				{commandledger.Queued, commandledger.Running},
				{commandledger.Running, commandledger.Succeeded},
			} {
				changed, err = f.store.CompareAndSwapEnvelope(ctx, prepared.owner, candidate.InvocationID, transition[0], transition[1])
				if err != nil {
					break
				}
				if !changed {
					err = commandledger.ErrConflict
					break
				}
			}
		} else if err == nil {
			err = commandledger.ErrConflict
		}
		ledgerSamples[i] = time.Since(start)
		if err != nil {
			t.Fatalf("reserva/CAS do ledger %d: %v", i, err)
		}
		// Verificação fora da janela: não atribuir a consulta ao custo dos CAS.
		terminal, err := f.store.GetEnvelopeByID(ctx, prepared.owner, candidate.InvocationID)
		if err != nil || terminal.Status != commandledger.Succeeded {
			t.Fatalf("estado terminal da amostra %d: status=%s err=%v", i, terminal.Status, err)
		}
	}
	reportDurations(t, "sqlite_ledger_reserve_plus_3_cas", ledgerSamples)
}

func reportDurations(t *testing.T, name string, samples []time.Duration) {
	t.Helper()
	if len(samples) == 0 {
		t.Fatalf("sem amostras para %s", name)
	}
	ordered := append([]time.Duration(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	nearestRank := func(percent int) time.Duration {
		index := (percent*len(ordered)+99)/100 - 1
		return ordered[index]
	}
	t.Logf("latency stage=%s n=%d p50=%s p95=%s p99=%s max=%s", name, len(ordered), nearestRank(50), nearestRank(95), nearestRank(99), ordered[len(ordered)-1])
}

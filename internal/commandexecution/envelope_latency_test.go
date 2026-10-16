package commandexecution

import (
	"context"
	"database/sql"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandledger"
	"gorm.io/gorm/logger"
)

// TestExecuteEnvelopeLatency é um benchmark integrado opt-in, executado com:
// COMMAND_EXECUTION_LATENCY=1 go test -run '^TestExecuteEnvelopeLatency$' -v -count=1
// Usa SQLite em arquivo temporário, autenticação real e o handler pipe.read.
// A janela medida inclui ExecuteEnvelope inteiro até o CAS terminal; exclui
// setup, UUIDs, warmup e consultas de verificação. Não mede teclado/UI/foco.
// A mutação concorrente atualiza um contador de teste independente sob o MESMO
// EpochService/DispatchGate e SQLite, sem invalidar o comando medido.
func TestExecuteEnvelopeLatency(t *testing.T) {
	if os.Getenv("COMMAND_EXECUTION_LATENCY") != "1" {
		t.Skip("benchmark integrado opt-in: COMMAND_EXECUTION_LATENCY=1")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	const warmup, samples = 20, 200
	policy := os.Getenv("COMMAND_EXECUTION_LATENCY_SQLITE")
	if policy == "" {
		policy = "bootstrap"
	}
	if policy != "bootstrap" && policy != "fixture" {
		t.Fatal("COMMAND_EXECUTION_LATENCY_SQLITE deve ser bootstrap ou fixture")
	}
	connections := 4
	if raw := os.Getenv("COMMAND_EXECUTION_LATENCY_CONNECTIONS"); raw != "" {
		var err error
		connections, err = strconv.Atoi(raw)
		if err != nil || connections < 1 || connections > 4 {
			t.Fatal("COMMAND_EXECUTION_LATENCY_CONNECTIONS deve estar entre 1 e 4")
		}
	}
	for _, contention := range []bool{false, true} {
		name := "serial"
		if contention {
			name = "sqlite_mutation"
		}
		t.Run(name, func(t *testing.T) {
			f := newEnvelopePipelineFixture(t)
			f.db.Logger = logger.Default.LogMode(logger.Silent)
			// Pool explícito: inclui espera pelo SQLite serializado. Não comprova
			// comportamento com múltiplos writers/conexões concorrentes.
			sqlDB, err := f.db.DB()
			if err != nil {
				t.Fatal(err)
			}
			sqlDB.SetMaxOpenConns(connections)
			ctx := context.Background()
			if policy == "bootstrap" {
				configureLatencySQLite(t, sqlDB, connections)
			}
			t.Logf("SQLite policy=%s connections=%d", policy, connections)
			var journal string
			var synchronous int
			if err := f.db.Raw("PRAGMA journal_mode").Scan(&journal).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.db.Raw("PRAGMA synchronous").Scan(&synchronous).Error; err != nil {
				t.Fatal(err)
			}
			stop := func() {}
			execute := func() time.Duration {
				candidate := f.candidate(newTestUUID(), "pipe.read", `{}`)
				start := time.Now()
				record, err := f.service.ExecuteEnvelope(ctx, f.token, candidate)
				elapsed := time.Since(start)
				if err != nil || record.Status != commandledger.Succeeded {
					stop()
					var pair struct{ LedgerStatus, InvocationStatus string }
					diagnostic := f.db.Raw("SELECT l.status AS ledger_status, i.status AS invocation_status FROM command_idempotency_keys l LEFT JOIN command_invocations i ON i.invocation_id = l.invocation_id WHERE l.invocation_id = ?", candidate.InvocationID).Scan(&pair)
					t.Logf("após parar mutações: rows=%d ledger=%s invocation=%s diagnostic_error=%v", diagnostic.RowsAffected, pair.LedgerStatus, pair.InvocationStatus, diagnostic.Error)
					t.Fatalf("execução não concluída: status=%s err=%v", record.Status, err)
				}
				return elapsed
			}
			for range warmup {
				execute()
			}
			var mutations atomic.Int64
			if contention {
				if err := f.db.Exec("CREATE TABLE latency_mutation_fixture (id INTEGER PRIMARY KEY, generation INTEGER NOT NULL)").Error; err != nil {
					t.Fatal(err)
				}
				if err := f.db.Exec("INSERT INTO latency_mutation_fixture VALUES (1, 0)").Error; err != nil {
					t.Fatal(err)
				}
				_, _, epoch, err := f.service.captureEnvelopeIdentity(ctx, f.token)
				if err != nil {
					t.Fatal(err)
				}
				stopCh, ready, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
				go func() {
					first := true
					for {
						select {
						case <-stopCh:
							done <- nil
							return
						default:
						}
						err := f.epochs.AdmitMutation(ctx, epoch, func(ctx context.Context) error { return ctx.Err() }, func() error {
							return f.db.Exec("UPDATE latency_mutation_fixture SET generation = generation + 1 WHERE id = 1").Error
						})
						if first {
							close(ready)
							first = false
						}
						if err != nil {
							done <- err
							return
						}
						mutations.Add(1)
						runtime.Gosched()
					}
				}()
				stopped := false
				stop = func() {
					if !stopped {
						stopped = true
						close(stopCh)
						if err := <-done; err != nil {
							t.Errorf("mutação concorrente: %v", err)
						}
					}
				}
				t.Cleanup(stop)
				<-ready
			}
			before := mutations.Load()
			latencies := make([]time.Duration, samples)
			for i := range latencies {
				latencies[i] = execute()
			}
			stop()
			if contention && mutations.Load() <= before {
				t.Fatal("nenhuma mutação durante a janela medida")
			}
			for _, table := range []string{"command_invocations", "command_idempotency_keys"} {
				var count int64
				if err := f.db.Table(table).Where("status = ?", commandledger.Succeeded).Count(&count).Error; err != nil || count != samples+warmup {
					t.Fatalf("%s: succeeded=%d err=%v", table, count, err)
				}
			}
			if f.startCalls.Load() != samples+warmup {
				t.Fatalf("handler calls=%d", f.startCalls.Load())
			}
			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			// Nearest-rank: ceil(p*N/100)-1; conjunto completo, sem amostragem.
			percentile := func(p int) time.Duration { return latencies[(p*samples+99)/100-1] }
			t.Logf("go=%s os=%s arch=%s GOMAXPROCS=%d SQLite=file max_connections=%d journal=%s synchronous=%d warmup=%d n=%d mutation_commits=%d p50=%s p95=%s p99=%s max=%s", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0), connections, journal, synchronous, warmup, samples, mutations.Load()-before, percentile(50), percentile(95), percentile(99), latencies[len(latencies)-1])
		})
	}
}

// Replica os pragmas de database.Init/sqlite_policy.go sem chamar Init nem
// resolver diretórios pessoais. Como a fixture já abriu o banco sem DSN,
// mantém TODAS as conexões configuradas no pool (idle=connections, enquanto
// produto usa idle=2 e busy_timeout via DSN). Nenhuma conexão nova é necessária.
func configureLatencySQLite(t *testing.T, db *sql.DB, count int) {
	t.Helper()
	ctx := context.Background()
	db.SetMaxIdleConns(count)
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	var held []*sql.Conn
	defer func() {
		for _, conn := range held {
			if err := conn.Close(); err != nil {
				t.Error(err)
			}
		}
	}()
	for range count {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, conn)
		for _, pragma := range []string{"PRAGMA synchronous=NORMAL", "PRAGMA busy_timeout=100"} {
			if _, err := conn.ExecContext(ctx, pragma); err != nil {
				t.Fatal(err)
			}
		}
		var mode string
		var syncMode, busy int
		if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&syncMode); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
			t.Fatal(err)
		}
		if mode != "wal" || syncMode != 1 || busy != 100 {
			t.Fatalf("pragmas: mode=%s sync=%d busy=%d", mode, syncMode, busy)
		}
	}
}

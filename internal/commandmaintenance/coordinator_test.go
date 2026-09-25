package commandmaintenance

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

type maintenanceOutbox struct {
	mu           sync.Mutex
	order        *[]string
	requeued     int
	requeueLimit *int
}

type maintenanceOutboxWithPurge struct {
	maintenanceOutbox
	purged int
	more   bool
}

func (p *maintenanceOutboxWithPurge) PurgeExpired(context.Context, int) (int, bool, error) {
	*p.order = append(*p.order, "purge")
	p.purged++
	return p.purged, p.more, nil
}

func (p *maintenanceOutbox) RequeueExpiredLeases(_ context.Context, limit int) (int, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	*p.order = append(*p.order, "requeue")
	if p.requeueLimit != nil {
		*p.requeueLimit = limit
	}
	return p.requeued, false, nil
}
func (p *maintenanceOutbox) Drain(context.Context, int) (BatchResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	*p.order = append(*p.order, "drain")
	return BatchResult{}, nil
}

func (p *maintenanceOutbox) PurgeExpired(context.Context, int) (int, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return 0, false, nil
}

func TestCoordinatorUsesOptionalBoundedOutboxPurgeBeforeRetention(t *testing.T) {
	order := []string{}
	outbox := &maintenanceOutboxWithPurge{maintenanceOutbox: maintenanceOutbox{order: &order}}
	coordinator, err := New(Ports{
		Outbox: outbox, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{},
		Jobs: maintenanceRetention{name: "jobs", order: &order}, Tools: maintenanceTools{order: &order},
		InvocationDB: maintenanceRetention{name: "invocations", order: &order}, Activations: maintenanceRetention{name: "activations", order: &order},
		Compaction: maintenanceCompact{order: &order},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(2))
	if err != nil || report.OutboxPurged != 1 || !report.Compacted {
		t.Fatalf("purga=%+v err=%v", report, err)
	}
	if !reflect.DeepEqual(order[:4], []string{"requeue", "drain", "purge", "jobs"}) {
		t.Fatalf("ordem inicial=%v", order)
	}
}

type maintenanceRecovery struct {
	n    int
	more bool
}

func (p maintenanceRecovery) Recover(context.Context, int) (BatchResult, error) {
	return BatchResult{Processed: p.n, More: p.more}, nil
}

type maintenanceRetention struct {
	name  string
	order *[]string
}

func (p maintenanceRetention) Retain(context.Context, Policy) (int64, error) {
	*p.order = append(*p.order, p.name)
	return 1, nil
}

type boundedMaintenanceRetention struct {
	name     string
	order    *[]string
	result   RetentionResult
	gotBatch *int
}

func (p boundedMaintenanceRetention) Retain(context.Context, Policy) (int64, error) {
	*p.order = append(*p.order, p.name+"-legacy")
	return 0, nil
}

func (p boundedMaintenanceRetention) RetainBatch(_ context.Context, policy Policy) (RetentionResult, error) {
	*p.order = append(*p.order, p.name+"-batch")
	if p.gotBatch != nil {
		*p.gotBatch = policy.BatchSize
	}
	return p.result, nil
}

type maintenanceTools struct{ order *[]string }

func (p maintenanceTools) CleanOldDryRuns(context.Context, Policy) (int64, error) {
	*p.order = append(*p.order, "dry-runs")
	return 1, nil
}
func (p maintenanceTools) CleanOrphanChat(context.Context, Policy) (int64, error) {
	*p.order = append(*p.order, "orphan-chat")
	return 1, nil
}
func (p maintenanceTools) CleanOldChat(context.Context, Policy) (int64, error) {
	*p.order = append(*p.order, "old-chat")
	return 1, nil
}

type maintenanceCompact struct{ order *[]string }

func (p maintenanceCompact) Compact(context.Context, int64) error {
	*p.order = append(*p.order, "compact")
	return nil
}

func validMaintenancePolicy(batch int) Policy {
	return Policy{
		JobRetention: time.Hour, InvocationRetention: time.Hour,
		ActivationRetention: time.Hour, LeaseDuration: time.Minute,
		BatchSize: batch, InvocationsPerUser: 1, InvocationsSystemKeep: 1,
		ActivationsPerUser: 1,
	}
}

func TestCoordinatorOutboxBeforeJobsAndUmaPassagem(t *testing.T) {
	var order []string
	var gotRequeueLimit int
	outbox := &maintenanceOutbox{order: &order, requeued: 2, requeueLimit: &gotRequeueLimit}
	coordinator, err := New(Ports{
		Outbox:       outbox,
		Decisions:    maintenanceRecovery{},
		Invocations:  maintenanceRecovery{},
		Claims:       maintenanceRecovery{},
		Jobs:         maintenanceRetention{name: "jobs", order: &order},
		Tools:        maintenanceTools{order: &order},
		InvocationDB: maintenanceRetention{name: "invocations", order: &order},
		Activations:  maintenanceRetention{name: "activations", order: &order},
		Compaction:   maintenanceCompact{order: &order},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(2))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"requeue", "drain", "jobs", "dry-runs", "orphan-chat", "old-chat", "invocations", "activations", "compact"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("ordem=%v, want %v", order, want)
	}
	if report.OutboxRequeued != 2 || !report.OutboxDrained || report.Recovered != 0 || report.MoreRecovery || !report.Compacted {
		t.Fatalf("relatório inesperado: %+v", report)
	}
	if gotRequeueLimit != 2 {
		t.Fatalf("limite de requeue=%d, want 2", gotRequeueLimit)
	}
}

func TestCoordinatorUsaRetencaoBoundedEImpedeCompactacao(t *testing.T) {
	var order []string
	var gotJobsBatch int
	coordinator, err := New(Ports{
		Outbox: &maintenanceOutbox{order: &order}, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{},
		Jobs:         boundedMaintenanceRetention{name: "jobs", order: &order, result: RetentionResult{Deleted: 1, More: true}, gotBatch: &gotJobsBatch},
		Tools:        maintenanceTools{order: &order},
		InvocationDB: boundedMaintenanceRetention{name: "invocations", order: &order, result: RetentionResult{Deleted: 4}},
		Activations:  boundedMaintenanceRetention{name: "activations", order: &order, result: RetentionResult{Deleted: 1}},
		Compaction:   maintenanceCompact{order: &order},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(2))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"requeue", "drain", "jobs-batch", "dry-runs", "orphan-chat", "old-chat", "invocations-batch", "activations-batch"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("ordem=%v, want %v", order, want)
	}
	if gotJobsBatch != 2 {
		t.Fatalf("batch jobs=%d, want 2", gotJobsBatch)
	}
	if report.JobsDeleted != 1 || report.InvocationsDeleted != 4 || report.ActivationsDeleted != 1 || !report.MoreRetention || report.Compacted {
		t.Fatalf("relatório bounded inesperado: %+v", report)
	}
}

func TestCoordinatorRecusaRetencaoBoundedInconsistente(t *testing.T) {
	for _, test := range []struct {
		name   string
		result RetentionResult
	}{{name: "negative", result: RetentionResult{Deleted: -1}}} {
		t.Run(test.name, func(t *testing.T) {
			var order []string
			coordinator, err := New(Ports{
				Outbox: &maintenanceOutbox{order: &order}, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{},
				Jobs: boundedMaintenanceRetention{name: "jobs", order: &order, result: test.result}, Tools: maintenanceTools{order: &order},
				InvocationDB: maintenanceRetention{order: &order}, Activations: maintenanceRetention{order: &order}, Compaction: maintenanceCompact{order: &order},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := coordinator.Run(context.Background(), validMaintenancePolicy(2)); err != ErrInvalidRetentionResult {
				t.Fatalf("retenção inconsistente aceita: %+v, err=%v", test.result, err)
			}
			if len(order) != 3 || order[2] != "jobs-batch" {
				t.Fatalf("efeitos após retenção inconsistente: %v", order)
			}
		})
	}
}

func TestCoordinatorRevalidaCancelamentoEntreCadaPorta(t *testing.T) {
	for _, cancelAt := range []string{
		"requeue", "drain", "decisions", "invocations", "claims", "jobs",
		"dry-runs", "orphan-chat", "old-chat", "invocation-db", "activations",
	} {
		t.Run(cancelAt, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls []string
			outbox := &cancelingOutbox{calls: &calls, cancelAt: cancelAt, cancel: cancel}
			recovery := func(name string) *cancelingRecovery {
				return &cancelingRecovery{calls: &calls, name: name, cancelAt: cancelAt, cancel: cancel}
			}
			retention := func(name string) *cancelingRetention {
				return &cancelingRetention{calls: &calls, name: name, cancelAt: cancelAt, cancel: cancel}
			}
			coordinator, err := New(Ports{
				Outbox: outbox, Decisions: recovery("decisions"), Invocations: recovery("invocations"), Claims: recovery("claims"),
				Jobs: retention("jobs"), Tools: &cancelingTools{calls: &calls, cancelAt: cancelAt, cancel: cancel},
				InvocationDB: retention("invocation-db"), Activations: retention("activations"),
				Compaction: &cancelingCompaction{calls: &calls},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := coordinator.Run(ctx, validMaintenancePolicy(1)); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelamento após %s não propagado: %v", cancelAt, err)
			}
			if len(calls) == 0 || calls[len(calls)-1] != cancelAt {
				t.Fatalf("portas após cancelamento em %s: %v", cancelAt, calls)
			}
		})
	}
}

func TestCoordinatorRejeitaContagemNegativaLegadaETools(t *testing.T) {
	for _, test := range []struct {
		name         string
		negativeJob  bool
		negativeTool string
		wantCalls    []string
	}{
		{name: "legacy retention", negativeJob: true, wantCalls: []string{"requeue", "drain", "jobs"}},
		{name: "tools dry-runs", negativeTool: "dry-runs", wantCalls: []string{"requeue", "drain", "jobs", "dry-runs"}},
		{name: "tools orphan-chat", negativeTool: "orphan-chat", wantCalls: []string{"requeue", "drain", "jobs", "dry-runs", "orphan-chat"}},
		{name: "tools old-chat", negativeTool: "old-chat", wantCalls: []string{"requeue", "drain", "jobs", "dry-runs", "orphan-chat", "old-chat"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var order []string
			var jobs RetentionPort = maintenanceRetention{name: "jobs", order: &order}
			if test.negativeJob {
				jobs = negativeRetention{order: &order}
			}
			var tools ToolRetentionPort = maintenanceTools{order: &order}
			if test.negativeTool != "" {
				tools = negativeTools{order: &order, negative: test.negativeTool}
			}
			coordinator, err := New(Ports{
				Outbox: &maintenanceOutbox{order: &order}, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{},
				Jobs: jobs, Tools: tools, InvocationDB: maintenanceRetention{order: &order}, Activations: maintenanceRetention{order: &order},
				Compaction: maintenanceCompact{order: &order},
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = coordinator.Run(context.Background(), validMaintenancePolicy(1))
			if !errors.Is(err, ErrInvalidRetentionResult) {
				t.Fatalf("contagem negativa aceita: %v", err)
			}
			if !reflect.DeepEqual(order, test.wantCalls) {
				t.Fatalf("ordem=%v, want %v", order, test.wantCalls)
			}
		})
	}
}

func TestCoordinatorNaoRetemEnquantoOutboxTemMaisLotes(t *testing.T) {
	var order []string
	var gotLimit int
	coordinator, err := New(Ports{
		Outbox: &maintenanceOutboxWithMore{order: &order, limit: &gotLimit, drainMore: true}, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{},
		Jobs: maintenanceRetention{name: "jobs", order: &order}, Tools: maintenanceTools{order: &order}, InvocationDB: maintenanceRetention{order: &order},
		Activations: maintenanceRetention{order: &order}, Compaction: maintenanceCompact{order: &order},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(1))
	if err != nil {
		t.Fatal(err)
	}
	if !report.MoreOutbox || report.OutboxDrained || report.JobsDeleted != 0 || report.Compacted {
		t.Fatalf("retenção indevida ou relatório inconsistente: %+v", report)
	}
	if gotLimit != 1 {
		t.Fatalf("limite de drain=%d, want 1", gotLimit)
	}
	if want := []string{"requeue", "drain"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("ordem=%v, want %v", order, want)
	}
}

func TestCoordinatorMoreOutboxFromRequeueImpedeRetencao(t *testing.T) {
	var order []string
	coordinator, err := New(Ports{
		Outbox: &maintenanceOutboxWithMore{order: &order, requeueMore: true}, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{},
		Jobs: maintenanceRetention{name: "jobs", order: &order}, Tools: maintenanceTools{order: &order}, InvocationDB: maintenanceRetention{order: &order},
		Activations: maintenanceRetention{order: &order}, Compaction: maintenanceCompact{order: &order},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(1))
	if err != nil {
		t.Fatal(err)
	}
	if !report.MoreOutbox || report.OutboxDrained || report.JobsDeleted != 0 || report.Compacted {
		t.Fatalf("requeue pendente não bloqueou retenção: %+v", report)
	}
}

func TestCoordinatorRecusaResultadoDeLoteInconsistente(t *testing.T) {
	tests := []struct {
		name      string
		processed int
	}{
		{name: "negative", processed: -1},
		{name: "above limit", processed: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var order []string
			coordinator, err := New(Ports{
				Outbox: &maintenanceOutboxWithResult{order: &order, result: BatchResult{Processed: test.processed}}, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{},
				Jobs: maintenanceRetention{name: "jobs", order: &order}, Tools: maintenanceTools{order: &order}, InvocationDB: maintenanceRetention{order: &order},
				Activations: maintenanceRetention{order: &order}, Compaction: maintenanceCompact{order: &order},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := coordinator.Run(context.Background(), validMaintenancePolicy(1)); err != ErrInvalidBatchResult {
				t.Fatalf("resultado inválido aceito: %v", err)
			}
			if len(order) != 2 {
				t.Fatalf("recuperação/retenção executada após resultado inválido: %v", order)
			}
		})
	}
}

func TestCoordinatorNaoRetemEnquantoRecoveryTemMaisLotes(t *testing.T) {
	var order []string
	coordinator, err := New(Ports{
		Outbox: &maintenanceOutbox{order: &order}, Decisions: maintenanceRecovery{n: 1, more: true},
		Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{}, Jobs: maintenanceRetention{name: "jobs", order: &order},
		Tools: maintenanceTools{order: &order}, InvocationDB: maintenanceRetention{order: &order},
		Activations: maintenanceRetention{order: &order}, Compaction: maintenanceCompact{order: &order},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(1))
	if err != nil {
		t.Fatal(err)
	}
	if !report.MoreRecovery || report.JobsDeleted != 0 || report.Compacted || len(order) != 2 {
		t.Fatalf("retenção indevida: report=%+v order=%v", report, order)
	}
}

func TestCoordinatorNaoRodaSemOutboxOuCompactor(t *testing.T) {
	if _, err := New(Ports{Jobs: maintenanceRetention{}}); err != ErrInvalid {
		t.Fatalf("New sem portas críticas: %v", err)
	}
}

func TestCoordinatorCancelamentoAntesDaPassagemENaoSegundoLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var called int
	outbox := &maintenanceOutbox{order: new([]string)}
	jobs := maintenanceRetention{order: new([]string)}
	coordinator, err := New(Ports{Outbox: outbox, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{}, Jobs: jobs, Tools: maintenanceTools{order: new([]string)}, InvocationDB: maintenanceRetention{order: new([]string)}, Activations: maintenanceRetention{order: new([]string)}, Compaction: maintenanceCompact{order: new([]string)}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = coordinator.Run(ctx, validMaintenancePolicy(0))
	if err == nil {
		t.Fatal("cancelamento deveria ser retornado")
	}
	_ = called
	// Uma chamada nova é permitida depois que a passagem anterior terminou.
	if _, err := coordinator.Run(context.Background(), validMaintenancePolicy(0)); err != nil {
		t.Fatal(err)
	}
}

func TestCoordinatorRecusaConcorrenciaSemDuplicarPassagem(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	outbox := blockingOutbox{started: started, release: release}
	coordinator, err := New(Ports{Outbox: &outbox, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{}, Jobs: maintenanceRetention{order: new([]string)}, Tools: maintenanceTools{order: new([]string)}, InvocationDB: maintenanceRetention{order: new([]string)}, Activations: maintenanceRetention{order: new([]string)}, Compaction: maintenanceCompact{order: new([]string)}})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, e := coordinator.Run(context.Background(), validMaintenancePolicy(0)); result <- e }()
	<-started
	if _, err := coordinator.Run(context.Background(), validMaintenancePolicy(0)); err != ErrAlreadyRunning {
		t.Fatalf("corrida concorrente=%v", err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestCoordinatorRecusaPoliticaIncompletaAntesDoOutbox(t *testing.T) {
	for _, mutate := range []func(*Policy){
		func(p *Policy) { p.InvocationRetention = 0 },
		func(p *Policy) { p.ActivationRetention = 0 },
		func(p *Policy) { p.LeaseDuration = 0 },
		func(p *Policy) { p.ActivationsPerUser = 0 },
	} {
		var order []string
		coordinator, err := New(Ports{
			Outbox: &maintenanceOutbox{order: &order}, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{},
			Jobs: maintenanceRetention{order: &order}, Tools: maintenanceTools{order: &order}, InvocationDB: maintenanceRetention{order: &order},
			Activations: maintenanceRetention{order: &order}, Compaction: maintenanceCompact{order: &order},
		})
		if err != nil {
			t.Fatal(err)
		}
		policy := validMaintenancePolicy(1)
		mutate(&policy)
		if _, err := coordinator.Run(context.Background(), policy); err != ErrInvalid {
			t.Fatalf("policy incompleta aceita: %+v err=%v", policy, err)
		}
		if len(order) != 0 {
			t.Fatalf("outbox executado antes da validação: %v", order)
		}
	}
}

func TestPolicyValidateMantemZerosCanonicosLegados(t *testing.T) {
	policy := validMaintenancePolicy(0)
	policy.RunsPerJobKeep = 0
	policy.ChatRetention = 0
	if err := policy.Validate(); err != nil {
		t.Fatalf("zero canônico legado rejeitado: %v", err)
	}
}

type blockingOutbox struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockingOutbox) PurgeExpired(context.Context, int) (int, bool, error) {
	return 0, false, nil
}

func (p *blockingOutbox) RequeueExpiredLeases(context.Context, int) (int, bool, error) {
	close(p.started)
	<-p.release
	return 0, false, nil
}
func (p *blockingOutbox) Drain(_ context.Context, _ int) (BatchResult, error) {
	return BatchResult{}, nil
}

type maintenanceOutboxWithMore struct {
	order       *[]string
	limit       *int
	requeueMore bool
	drainMore   bool
}

func (p *maintenanceOutboxWithMore) RequeueExpiredLeases(_ context.Context, _ int) (int, bool, error) {
	*p.order = append(*p.order, "requeue")
	return 0, p.requeueMore, nil
}

func (p *maintenanceOutboxWithMore) Drain(_ context.Context, limit int) (BatchResult, error) {
	*p.order = append(*p.order, "drain")
	if p.limit != nil {
		*p.limit = limit
	}
	return BatchResult{Processed: 1, More: p.drainMore}, nil
}

func (p *maintenanceOutboxWithMore) PurgeExpired(context.Context, int) (int, bool, error) {
	return 0, false, nil
}

type maintenanceOutboxWithResult struct {
	order  *[]string
	result BatchResult
}

func (p *maintenanceOutboxWithResult) RequeueExpiredLeases(context.Context, int) (int, bool, error) {
	*p.order = append(*p.order, "requeue")
	return 0, false, nil
}

func (p *maintenanceOutboxWithResult) Drain(_ context.Context, _ int) (BatchResult, error) {
	*p.order = append(*p.order, "drain")
	return p.result, nil
}

func (p *maintenanceOutboxWithResult) PurgeExpired(context.Context, int) (int, bool, error) {
	return 0, false, nil
}

type cancelingOutbox struct {
	calls    *[]string
	cancelAt string
	cancel   context.CancelFunc
}

func (p *cancelingOutbox) mark(name string) {
	*p.calls = append(*p.calls, name)
	if name == p.cancelAt {
		p.cancel()
	}
}

func (p *cancelingOutbox) RequeueExpiredLeases(context.Context, int) (int, bool, error) {
	p.mark("requeue")
	return 0, false, nil
}

func (p *cancelingOutbox) Drain(context.Context, int) (BatchResult, error) {
	p.mark("drain")
	return BatchResult{}, nil
}

func (p *cancelingOutbox) PurgeExpired(context.Context, int) (int, bool, error) {
	p.mark("purge")
	return 0, false, nil
}

type cancelingRecovery struct {
	calls    *[]string
	name     string
	cancelAt string
	cancel   context.CancelFunc
}

func (p *cancelingRecovery) Recover(context.Context, int) (BatchResult, error) {
	*p.calls = append(*p.calls, p.name)
	if p.name == p.cancelAt {
		p.cancel()
	}
	return BatchResult{}, nil
}

type cancelingRetention struct {
	calls    *[]string
	name     string
	cancelAt string
	cancel   context.CancelFunc
}

func (p *cancelingRetention) Retain(context.Context, Policy) (int64, error) {
	*p.calls = append(*p.calls, p.name)
	if p.name == p.cancelAt {
		p.cancel()
	}
	return 0, nil
}

type cancelingTools struct {
	calls    *[]string
	cancelAt string
	cancel   context.CancelFunc
}

func (p *cancelingTools) clean(name string) (int64, error) {
	*p.calls = append(*p.calls, name)
	if name == p.cancelAt {
		p.cancel()
	}
	return 0, nil
}

func (p *cancelingTools) CleanOldDryRuns(context.Context, Policy) (int64, error) {
	return p.clean("dry-runs")
}

func (p *cancelingTools) CleanOrphanChat(context.Context, Policy) (int64, error) {
	return p.clean("orphan-chat")
}

func (p *cancelingTools) CleanOldChat(context.Context, Policy) (int64, error) {
	return p.clean("old-chat")
}

type cancelingCompaction struct {
	calls *[]string
}

func (p *cancelingCompaction) Compact(context.Context, int64) error {
	*p.calls = append(*p.calls, "compact")
	return nil
}

type negativeRetention struct {
	order *[]string
}

func (p negativeRetention) Retain(context.Context, Policy) (int64, error) {
	*p.order = append(*p.order, "jobs")
	return -1, nil
}

type negativeTools struct {
	order    *[]string
	negative string
}

func (p negativeTools) clean(name string) (int64, error) {
	*p.order = append(*p.order, name)
	if name == p.negative {
		return -1, nil
	}
	return 0, nil
}

func (p negativeTools) CleanOldDryRuns(context.Context, Policy) (int64, error) {
	return p.clean("dry-runs")
}

func (p negativeTools) CleanOrphanChat(context.Context, Policy) (int64, error) {
	return p.clean("orphan-chat")
}

func (p negativeTools) CleanOldChat(context.Context, Policy) (int64, error) {
	return p.clean("old-chat")
}

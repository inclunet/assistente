package jobs

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler gerencia triggers temporais (cron e interval) para jobs.
type Scheduler struct {
	mu       sync.Mutex
	cron     *cron.Cron
	entries  map[string][]cron.EntryID     // jobID -> lista de entry IDs
	timers   map[string]*time.Ticker       // jobID -> ticker para interval
	cancelFn map[string]context.CancelFunc // jobID -> cancel para goroutines de interval
	execFunc func(ctx context.Context, job *Job, trigCtx *TriggerContext)
	started  bool
}

// NewScheduler cria um scheduler com a funcao de execucao fornecida.
func NewScheduler(execFunc func(ctx context.Context, job *Job, trigCtx *TriggerContext)) *Scheduler {
	return &Scheduler{
		cron:     newJobCron(),
		entries:  make(map[string][]cron.EntryID),
		timers:   make(map[string]*time.Ticker),
		cancelFn: make(map[string]context.CancelFunc),
		execFunc: execFunc,
	}
}

func newJobCron() *cron.Cron {
	return cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)))
}

// Schedule registra os triggers temporais de um job.
func (s *Scheduler) Schedule(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove schedule anterior se existia
	s.removeJobLocked(job.ID)

	if !job.Enabled {
		return nil
	}

	for _, t := range job.Triggers {
		switch t.Type {
		case TriggerCron:
			if err := s.scheduleCron(job, t); err != nil {
				return fmt.Errorf("schedule cron for %s: %w", job.ID, err)
			}
		case TriggerInterval:
			if err := s.scheduleInterval(job, t); err != nil {
				return fmt.Errorf("schedule interval for %s: %w", job.ID, err)
			}
		}
	}

	return nil
}

// Unschedule remove todos os triggers temporais de um job.
func (s *Scheduler) Unschedule(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeJobLocked(jobID)
}

// Start inicia o cron scheduler.
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return
	}

	s.cron.Start()
	s.started = true
	log.Printf("[Jobs] Scheduler started")
}

// Stop para o scheduler e cancela todos os timers.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return
	}

	ctx := s.cron.Stop()
	<-ctx.Done()

	for id, cancel := range s.cancelFn {
		cancel()
		delete(s.cancelFn, id)
	}

	for id, ticker := range s.timers {
		ticker.Stop()
		delete(s.timers, id)
	}
	s.cron = newJobCron()
	s.entries = make(map[string][]cron.EntryID)

	s.started = false
	log.Printf("[Jobs] Scheduler stopped")
}

// Reschedule atualiza os triggers de um job (remove e re-adiciona).
func (s *Scheduler) Reschedule(job *Job) error {
	return s.Schedule(job)
}

func (s *Scheduler) scheduleCron(job *Job, t Trigger) error {
	jobCopy := *job
	entryID, err := s.cron.AddFunc(t.Expression, func() {
		if s.execFunc != nil {
			ctx := context.Background()
			s.safeExec(ctx, &jobCopy, &TriggerContext{
				Type:       TriggerCron,
				Expression: t.Expression,
			})
		}
	})
	if err != nil {
		return err
	}

	s.entries[job.ID] = append(s.entries[job.ID], entryID)
	log.Printf("[Jobs] Scheduled cron for %s: %s", job.ID, t.Expression)
	return nil
}

func (s *Scheduler) scheduleInterval(job *Job, t Trigger) error {
	duration, err := parseInterval(t.Every)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(duration)
	s.timers[job.ID] = ticker

	ctx, cancel := context.WithCancel(context.Background())
	s.cancelFn[job.ID] = cancel

	jobCopy := *job
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if s.execFunc != nil {
					s.safeExec(ctx, &jobCopy, &TriggerContext{
						Type:  TriggerInterval,
						Every: t.Every,
					})
				}
			}
		}
	}()

	log.Printf("[Jobs] Scheduled interval for %s: every %s", job.ID, t.Every)
	return nil
}

// safeExec executa execFunc com recover para evitar que um panic mate a goroutine
// do interval ticker permanentemente.
func (s *Scheduler) safeExec(ctx context.Context, job *Job, trigCtx *TriggerContext) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Jobs] PANIC recovered in interval execution for %q: %v", job.ID, r)
		}
	}()
	s.execFunc(ctx, job, trigCtx)
}

func (s *Scheduler) removeJobLocked(jobID string) {
	// Remove cron entries
	for _, entryID := range s.entries[jobID] {
		s.cron.Remove(entryID)
	}
	delete(s.entries, jobID)

	// Remove interval timer
	if cancel, ok := s.cancelFn[jobID]; ok {
		cancel()
		delete(s.cancelFn, jobID)
	}
	if ticker, ok := s.timers[jobID]; ok {
		ticker.Stop()
		delete(s.timers, jobID)
	}
}

// ScheduledJobs retorna os IDs dos jobs com schedule ativo.
func (s *Scheduler) ScheduledJobs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := make(map[string]bool)
	for id := range s.entries {
		seen[id] = true
	}
	for id := range s.timers {
		seen[id] = true
	}

	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	return result
}

package jobs

import (
	"fmt"
	"sync"
	"time"
)

// Limites padrao do circuit breaker.
const (
	DefaultMaxChainDepth  = 10
	DefaultMaxRunsPerHour = 60
)

// CircuitBreaker protege contra loops infinitos e execucao excessiva.
type CircuitBreaker struct {
	mu             sync.Mutex
	maxChainDepth  int
	maxRunsPerHour int

	// Contador de execucoes por job na ultima hora (sliding window)
	runCounts map[string][]time.Time
}

// NewCircuitBreaker cria um circuit breaker com limites padrao.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		maxChainDepth:  DefaultMaxChainDepth,
		maxRunsPerHour: DefaultMaxRunsPerHour,
		runCounts:      make(map[string][]time.Time),
	}
}

// SetMaxChainDepth altera o limite de profundidade de cadeia.
func (cb *CircuitBreaker) SetMaxChainDepth(depth int) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.maxChainDepth = depth
}

// SetMaxRunsPerHour altera o limite de execucoes por hora por job.
func (cb *CircuitBreaker) SetMaxRunsPerHour(max int) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.maxRunsPerHour = max
}

// CheckRateLimit verifica se um job pode executar (nao excedeu max runs/hora).
// jobLimit permite override per-job; se <= 0, usa o default global.
func (cb *CircuitBreaker) CheckRateLimit(jobID string, jobLimit int) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.pruneOldRuns(jobID)

	limit := cb.maxRunsPerHour
	if jobLimit > 0 {
		limit = jobLimit
	}

	count := len(cb.runCounts[jobID])
	if count >= limit {
		return fmt.Errorf("circuit breaker: job %q exceeded rate limit (%d runs/hour)", jobID, limit)
	}

	return nil
}

// RecordRun registra uma execucao para controle de rate limiting.
func (cb *CircuitBreaker) RecordRun(jobID string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.runCounts[jobID] = append(cb.runCounts[jobID], time.Now())
}

// MaxChainDepth retorna o limite atual de profundidade de cadeia.
func (cb *CircuitBreaker) MaxChainDepth() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.maxChainDepth
}

// DetectLoop verifica se disparar um evento criaria um loop (A -> B -> A).
// Recebe o historico de jobs na cadeia atual e o proximo job.
func (cb *CircuitBreaker) DetectLoop(chainHistory []string, nextJobID string) error {
	for _, prev := range chainHistory {
		if prev == nextJobID {
			return fmt.Errorf("circuit breaker: event loop detected: %s -> ... -> %s",
				nextJobID, nextJobID)
		}
	}
	return nil
}

// pruneOldRuns remove registros com mais de 1 hora (sem lock, chamador deve travar).
func (cb *CircuitBreaker) pruneOldRuns(jobID string) {
	cutoff := time.Now().Add(-1 * time.Hour)
	runs := cb.runCounts[jobID]

	i := 0
	for i < len(runs) && runs[i].Before(cutoff) {
		i++
	}

	if i > 0 {
		cb.runCounts[jobID] = runs[i:]
	}
}

// Reset limpa todos os contadores (util para testes).
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.runCounts = make(map[string][]time.Time)
}

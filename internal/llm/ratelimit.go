package llm

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Rate limiting das chamadas ao provedor LLM (Issue #27 / AEP-0065).
//
// Motivação: sem controle de taxa, loops de agentes, retries em cascata ou
// uso abusivo podem disparar centenas de chamadas ao provedor em segundos,
// gerando custos inesperados e estourando cotas. Este limitador aplica um
// token bucket por usuário (chave = userID resolvido do contexto) usando
// golang.org/x/time/rate, conforme proposto no issue.
//
// O escopo é por usuário (AEP-0052 já carimba o userID no contexto). Quando
// o contexto não tem userID (fluxos internos sem escopo), o limitador usa uma
// chave global única, de modo que nenhuma chamada escape do controle.

const (
	// DefaultRateLimitRPM é o teto sustentado de requisições por minuto por
	// usuário. 60 rpm (1/s sustentado) é generoso para conversas normais e
	// para a maioria dos loops agênticos, mas barra rajadas patológicas.
	DefaultRateLimitRPM = 60

	// DefaultRateLimitBurst é a rajada instantânea permitida. Precisa ser
	// >= MaxAgenticIterations (default 25) para não interromper um único loop
	// agêntico legítimo que dispare várias iterações em sequência rápida.
	DefaultRateLimitBurst = 30

	// DefaultNearLimitThreshold é a fração da rajada (burst) de tokens
	// restantes abaixo da qual um alerta de "próximo do limite" é emitido.
	DefaultNearLimitThreshold = 0.2

	// globalRateLimitKey é a chave usada quando o contexto não carrega um
	// userID. Garante que chamadas sem escopo ainda sejam contabilizadas.
	globalRateLimitKey = "__global__"

	// Variáveis de ambiente para configuração em runtime.
	envRateLimitEnabled = "ASSISTENTE_LLM_RATE_LIMIT_ENABLED"
	envRateLimitRPM     = "ASSISTENTE_LLM_RATE_LIMIT_RPM"
	envRateLimitBurst   = "ASSISTENTE_LLM_RATE_LIMIT_BURST"
)

// RateLimitConfig descreve os parâmetros configuráveis do limitador.
type RateLimitConfig struct {
	// Enabled liga/desliga o rate limiting. Quando false, NewRateLimiter
	// retorna nil e o decorator vira um passthrough.
	Enabled bool
	// RequestsPerMinute é o teto sustentado por usuário. <= 0 usa o default.
	RequestsPerMinute int
	// Burst é a rajada instantânea por usuário. <= 0 usa o default.
	Burst int
	// NearLimitThreshold é a fração (0..1) da rajada abaixo da qual o alerta
	// de proximidade é disparado. <= 0 usa o default.
	NearLimitThreshold float64
}

// DefaultRateLimitConfig retorna a configuração padrão (habilitada).
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Enabled:            true,
		RequestsPerMinute:  DefaultRateLimitRPM,
		Burst:              DefaultRateLimitBurst,
		NearLimitThreshold: DefaultNearLimitThreshold,
	}
}

// RateLimitConfigFromEnv parte dos defaults e sobrescreve com variáveis de
// ambiente quando presentes. Valores inválidos são ignorados (mantém default)
// com log de aviso, para nunca desabilitar o controle por erro de digitação.
func RateLimitConfigFromEnv() RateLimitConfig {
	cfg := DefaultRateLimitConfig()

	if raw, ok := os.LookupEnv(envRateLimitEnabled); ok {
		if v, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil {
			cfg.Enabled = v
		} else {
			logging.Infof(context.Background(), "llm.ratelimit", "[llm/ratelimit] valor inválido em %s=%q; mantendo enabled=%v", envRateLimitEnabled, raw, cfg.Enabled)
		}
	}
	if raw, ok := os.LookupEnv(envRateLimitRPM); ok {
		if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && v > 0 {
			cfg.RequestsPerMinute = v
		} else {
			logging.Infof(context.Background(), "llm.ratelimit", "[llm/ratelimit] valor inválido em %s=%q; mantendo rpm=%d", envRateLimitRPM, raw, cfg.RequestsPerMinute)
		}
	}
	if raw, ok := os.LookupEnv(envRateLimitBurst); ok {
		if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && v > 0 {
			cfg.Burst = v
		} else {
			logging.Errorf(context.Background(), "llm.ratelimit", "[llm/ratelimit] valor inválido em %s=%q; mantendo burst=%d", envRateLimitBurst, raw, cfg.Burst)
		}
	}
	return cfg
}

// RateLimitError sinaliza que a chamada foi barrada pelo limitador. Carrega o
// tempo sugerido de espera para retry (RetryAfter), permitindo backoff no
// caller sem travar a UI.
type RateLimitError struct {
	Key        string
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e == nil {
		return "limite de requisições ao provedor LLM atingido"
	}
	wait := e.RetryAfter
	if wait < time.Second {
		wait = time.Second
	}
	return fmt.Sprintf(
		"Limite de requisições ao provedor LLM atingido. Aguarde aproximadamente %s antes de tentar novamente.",
		wait.Round(time.Second),
	)
}

// IsRateLimitError informa se err é (ou embrulha) um RateLimitError.
func IsRateLimitError(err error) bool {
	var rl *RateLimitError
	return errors.As(err, &rl)
}

// RateLimiter aplica throttling por chave (userID) com um token bucket por
// chave. Usa golang.org/x/time/rate internamente. Seguro para uso concorrente.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	limit    rate.Limit
	burst    int
	nearFrac float64

	// nearAlerted guarda, por chave, se o alerta de proximidade já foi
	// disparado enquanto a chave permanece abaixo do limiar. Usado para tornar
	// o alerta edge-triggered (dispara só na transição acima→abaixo) e evitar
	// spam de log/telemetria. Protegido por mu.
	nearAlerted map[string]bool

	// onNearLimit é o callback opcional de proximidade. Protegido por mu:
	// escrito por SetNearLimitHandler e lido (copiado para uma var local) em
	// maybeNearLimit sob o lock, sendo invocado fora do lock.
	onNearLimit func(key string, remaining float64)
}

// NewRateLimiter cria um RateLimiter a partir da configuração. Retorna nil
// quando o rate limiting está desabilitado — callers tratam nil como "sem
// limite" (o decorator vira passthrough).
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	if !cfg.Enabled {
		return nil
	}
	rpm := cfg.RequestsPerMinute
	if rpm <= 0 {
		rpm = DefaultRateLimitRPM
	}
	burst := cfg.Burst
	if burst <= 0 {
		burst = DefaultRateLimitBurst
	}
	nearFrac := cfg.NearLimitThreshold
	if nearFrac <= 0 || nearFrac >= 1 {
		nearFrac = DefaultNearLimitThreshold
	}
	return &RateLimiter{
		limiters:    make(map[string]*rate.Limiter),
		nearAlerted: make(map[string]bool),
		limit:       rate.Limit(float64(rpm) / 60.0),
		burst:       burst,
		nearFrac:    nearFrac,
	}
}

// SetNearLimitHandler registra um callback opcional disparado quando, após
// permitir uma chamada, os tokens restantes CRUZAM o limiar (transição
// acima→abaixo). É edge-triggered por chave: não realerta enquanto a chave
// continua abaixo do limiar e volta a poder alertar quando ela retorna acima.
// Seguro para chamar concorrentemente com Allow: a escrita do handler é
// protegida pelo mutex do limitador.
func (l *RateLimiter) SetNearLimitHandler(fn func(key string, remaining float64)) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.onNearLimit = fn
	l.mu.Unlock()
}

func (l *RateLimiter) limiterFor(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.limiters[key]
	if !ok {
		lim = rate.NewLimiter(l.limit, l.burst)
		l.limiters[key] = lim
	}
	return lim
}

// Allow consome 1 token do bucket de `key`. Retorna nil quando permitido, ou
// *RateLimitError com RetryAfter quando barrado. Quando o limitador é nil
// (desabilitado), sempre permite.
func (l *RateLimiter) Allow(key string) error {
	if l == nil {
		return nil
	}
	return l.allowAt(key, time.Now())
}

// allowAt é a variante com tempo explícito, usada internamente por Allow e
// nos testes para exercitar reabastecimento de tokens de forma determinística.
//
// Usa uma única observação do limiter (uma chamada ReserveN) para decidir e,
// quando barrado, calcular o RetryAfter — evitando o estado inconsistente que
// surgiria ao combinar AllowN + ReserveN sob concorrência. A reserva só é
// cancelada quando há atraso (>0), preservando o contrato "permitido consome 1
// token; bloqueado não consome".
func (l *RateLimiter) allowAt(key string, now time.Time) error {
	if l == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = globalRateLimitKey
	}
	lim := l.limiterFor(key)

	r := lim.ReserveN(now, 1)
	if !r.OK() {
		// burst insuficiente para 1 token (não deveria ocorrer com burst >= 1).
		return &RateLimitError{Key: key}
	}
	if delay := r.DelayFrom(now); delay > 0 {
		// Estourou: devolve o token reservado e sinaliza o tempo de espera.
		// Usa CancelAt(now) (e não Cancel(), que usa time.Now()) para manter o
		// cancelamento determinístico e consistente com o `now` passado a
		// ReserveN — essencial nos testes que usam tempo artificial.
		r.CancelAt(now)
		return &RateLimitError{Key: key, RetryAfter: delay}
	}

	// Permitido (token consumido). Avalia o alerta de proximidade.
	l.maybeNearLimit(key, lim, now)
	return nil
}

// maybeNearLimit dispara o callback de proximidade de forma edge-triggered:
// apenas na transição acima→abaixo do limiar, por chave. Enquanto a chave
// continua abaixo do limiar, não realerta; quando volta acima, o estado é
// resetado para permitir um novo alerta no próximo cruzamento. Mantém o
// disparo do callback fora do lock para evitar reentrância/deadlock.
func (l *RateLimiter) maybeNearLimit(key string, lim *rate.Limiter, now time.Time) {
	// remaining/below dependem apenas de campos imutáveis (burst, nearFrac) e do
	// próprio rate.Limiter (thread-safe), então podem ser calculados sem o lock.
	remaining := lim.TokensAt(now)
	below := remaining <= float64(l.burst)*l.nearFrac

	// Sob o lock: copia o handler e lê/atualiza o estado edge-triggered por chave.
	l.mu.Lock()
	handler := l.onNearLimit
	alreadyAlerted := l.nearAlerted[key]
	crossed := below && !alreadyAlerted
	switch {
	case crossed:
		l.nearAlerted[key] = true
	case !below && alreadyAlerted:
		delete(l.nearAlerted, key)
	}
	l.mu.Unlock()

	// Invoca o callback FORA do lock (evita segurar o mutex durante código de
	// usuário) e somente quando há handler e houve cruzamento do limiar.
	if crossed && handler != nil {
		handler(key, remaining)
	}
}

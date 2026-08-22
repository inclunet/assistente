package llm

import (
	"errors"
	"fmt"
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
// token bucket por usuário e perfil usando
// golang.org/x/time/rate, conforme proposto no issue.
//
// O escopo é por usuário e perfil (AEP-0052 já carimba o userID no contexto).
// Quando
// o contexto não tem userID (fluxos internos sem escopo), o limitador usa uma
// chave global única, de modo que nenhuma chamada escape do controle.

const (
	// DefaultRateLimitRPM é o teto sustentado de requisições por minuto por
	// usuário e perfil. 60 rpm (1/s sustentado) é generoso para conversas normais e
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

// RateLimiter aplica throttling por chave (userID + perfil) com um token bucket por
// chave. Usa golang.org/x/time/rate internamente. Seguro para uso concorrente.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rateLimiterEntry
	defaults RateLimitConfig

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

type rateLimiterEntry struct {
	limiter    *rate.Limiter
	rpm        int
	burst      int
	nearFrac   float64
	generation uint64
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
		limiters:    make(map[string]*rateLimiterEntry),
		nearAlerted: make(map[string]bool),
		defaults: RateLimitConfig{
			Enabled:            true,
			RequestsPerMinute:  rpm,
			Burst:              burst,
			NearLimitThreshold: nearFrac,
		},
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

func normalizeRateLimitConfig(cfg RateLimitConfig) RateLimitConfig {
	if cfg.RequestsPerMinute <= 0 {
		cfg.RequestsPerMinute = DefaultRateLimitRPM
	}
	if cfg.Burst <= 0 {
		cfg.Burst = DefaultRateLimitBurst
	}
	if cfg.NearLimitThreshold <= 0 || cfg.NearLimitThreshold >= 1 {
		cfg.NearLimitThreshold = DefaultNearLimitThreshold
	}
	return cfg
}

func (l *RateLimiter) limiterFor(key string, cfg RateLimitConfig, now time.Time) (*rateLimiterEntry, uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.limiters[key]
	if !ok {
		entry = &rateLimiterEntry{
			limiter:    rate.NewLimiter(rate.Limit(float64(cfg.RequestsPerMinute)/60.0), cfg.Burst),
			rpm:        cfg.RequestsPerMinute,
			burst:      cfg.Burst,
			nearFrac:   cfg.NearLimitThreshold,
			generation: 1,
		}
		l.limiters[key] = entry
		return entry, entry.generation
	}
	changed := false
	if entry.rpm != cfg.RequestsPerMinute {
		entry.limiter.SetLimitAt(now, rate.Limit(float64(cfg.RequestsPerMinute)/60.0))
		entry.rpm = cfg.RequestsPerMinute
		changed = true
	}
	if entry.burst != cfg.Burst {
		entry.limiter.SetBurstAt(now, cfg.Burst)
		entry.burst = cfg.Burst
		changed = true
	}
	if entry.nearFrac != cfg.NearLimitThreshold {
		entry.nearFrac = cfg.NearLimitThreshold
		changed = true
	}
	if changed {
		entry.generation++
		delete(l.nearAlerted, key)
	}
	return entry, entry.generation
}

// Allow consome 1 token do bucket de `key`. Retorna nil quando permitido, ou
// *RateLimitError com RetryAfter quando barrado. Quando o limitador é nil
// (desabilitado), sempre permite.
func (l *RateLimiter) Allow(key string) error {
	if l == nil {
		return nil
	}
	return l.allowAtWithConfig(key, time.Now(), l.defaults)
}

// AllowWithConfig aplica a política informada ao bucket da chave. Alterações
// atualizam taxa e rajada in-place, preservando os tokens já consumidos.
func (l *RateLimiter) AllowWithConfig(key string, cfg RateLimitConfig) error {
	if l == nil || !cfg.Enabled {
		return nil
	}
	return l.allowAtWithConfig(key, time.Now(), cfg)
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
	return l.allowAtWithConfig(key, now, l.defaults)
}

func (l *RateLimiter) allowAtWithConfig(key string, now time.Time, cfg RateLimitConfig) error {
	if l == nil || !cfg.Enabled {
		return nil
	}
	cfg = normalizeRateLimitConfig(cfg)
	key = strings.TrimSpace(key)
	if key == "" {
		key = globalRateLimitKey
	}
	entry, generation := l.limiterFor(key, cfg, now)
	lim := entry.limiter

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
	l.maybeNearLimit(key, entry, generation, cfg, now)
	return nil
}

// maybeNearLimit dispara o callback de proximidade de forma edge-triggered:
// apenas na transição acima→abaixo do limiar, por chave. Enquanto a chave
// continua abaixo do limiar, não realerta; quando volta acima, o estado é
// resetado para permitir um novo alerta no próximo cruzamento. Mantém o
// disparo do callback fora do lock para evitar reentrância/deadlock.
func (l *RateLimiter) maybeNearLimit(key string, entry *rateLimiterEntry, generation uint64, cfg RateLimitConfig, now time.Time) {
	// O rate.Limiter é thread-safe e cfg é o snapshot desta chamada.
	remaining := entry.limiter.TokensAt(now)
	below := remaining <= float64(cfg.Burst)*cfg.NearLimitThreshold

	// Sob o lock: copia o handler e lê/atualiza o estado edge-triggered por chave.
	l.mu.Lock()
	if current := l.limiters[key]; current != entry || entry.generation != generation {
		l.mu.Unlock()
		return
	}
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

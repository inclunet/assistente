package httpapi

import (
	"assistente/internal/logging"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// jwksCacheEntry guarda o JSON serializado e o ETag do JWKSet em uso. O
// payload é imutável até a próxima rotação do signer; o cache evita lock
// no signer a cada request público em /.well-known/jwks.json (B20).
type jwksCacheEntry struct {
	payload []byte
	etag    string
	expires time.Time
}

// jwksCacheTTL é deliberadamente curto. JWKS é fonte de verdade pública e
// downstream verifiers (gateways, libs JWT) cacheiam por horas — o cache
// aqui só absorve picos de tráfego sem segurar referência stale por
// muito tempo. 5min combina com a granularidade típica de rotação de
// chaves quando ela for implementada (B22).
const jwksCacheTTL = 5 * time.Minute

// rateBucket implementa um token bucket per-key (chave = IP no caso de
// uso atual) sem dependência externa: x/time/rate seria leve mas trazer
// um pacote para 30 linhas de código é overkill aqui e o comportamento
// é mais previsível.
type rateBucket struct {
	tokens     float64
	lastRefill time.Time
}

// rateLimiter aplica throttling por chave em endpoints públicos da API.
// Cada chave tem um bucket independente que regenera `rate` tokens por
// segundo até `burst`. Sem ttl/cleanup ainda — a quantidade de IPs
// distintos atingindo /auth/* em um servidor local é finita; quando
// implementarmos a API em deploys públicos vale acrescentar GC.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	rate    float64
	burst   float64
}

func newRateLimiter(rate, burst float64) *rateLimiter {
	return &rateLimiter{
		buckets: make(map[string]*rateBucket),
		rate:    rate,
		burst:   burst,
	}
}

// allow consome 1 token do bucket de `key` e retorna se a operação foi
// permitida. Se o bucket não existe, é criado cheio (burst). M21 do
// review: limita brute-force de login/refresh e DoS em jwks.
func (l *rateLimiter) allow(key string) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &rateBucket{tokens: l.burst, lastRefill: now}
		l.buckets[key] = b
	}
	delta := now.Sub(b.lastRefill).Seconds() * l.rate
	if delta > 0 {
		b.tokens += delta
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.lastRefill = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// rateLimit aplica o rate limit em um handler usando o IP do client como
// chave. Suporta X-Forwarded-For quando configurado por proxy de
// confiança — o caller é responsável por validar a origem do header
// antes de habilitar.
func (s *Server) rateLimit(limiter *rateLimiter, op string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if limiter == nil {
			next(w, r)
			return
		}
		key := clientIP(r)
		if !limiter.allow(key) {
			w.Header().Set("Retry-After", "1")
			logging.Errorf(context.Background(), "httpapi.middleware", "[httpapi] rate-limit excedido op=%s ip=%s", op, key)
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

// clientIP extrai o IP do request preferindo RemoteAddr (sem dependência
// de X-Forwarded-For que pode ser forjado quando a API não estiver
// atrás de proxy de confiança). Quando o deploy mudar para usar proxy,
// a função pode ser estendida para honrar uma allowlist de proxies.
func clientIP(r *http.Request) string {
	addr := r.RemoteAddr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// writeInternalErr (B21 do review) loga o erro real e devolve uma
// mensagem genérica ao cliente. Antes vários handlers chamavam
// writeError com o err cru, vazando paths de filesystem, mensagens do
// signer/keyring, etc. Isso é especialmente sensível em /auth/login
// onde a borda da API é escutada por atacantes.
func (s *Server) writeInternalErr(w http.ResponseWriter, op string, err error) {
	logging.Errorf(context.Background(), "httpapi.middleware", "[httpapi] op=%s err=%v", op, err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"error": "erro interno",
	})
}

// writeAuthErr aplica a mesma normalização para erros 401/403: mensagens
// idênticas e sem hint sobre estrutura interna do servidor. Logamos a
// distinção para investigação posterior (combina com M2 do bloco 1 que
// já mitigou timing attacks em AuthenticateLocal).
func (s *Server) writeAuthErr(w http.ResponseWriter, op string, status int, err error) {
	if err != nil {
		logging.Errorf(context.Background(), "httpapi.middleware", "[httpapi] op=%s status=%d err=%v", op, status, err)
	}
	msg := "credenciais inválidas"
	if status == http.StatusForbidden {
		msg = "acesso negado"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// errSessionUnavailable sinaliza que o session service ainda não foi
// inicializado (vault trancado, app em bootstrap). Caller distingue
// entre 503 e 500 para que o cliente saiba retry-able.
var errSessionUnavailable = errors.New("serviço de sessão indisponível")

// jwksFromCacheOrSigner devolve um snapshot serializado do JWKSet
// (cacheado por jwksCacheTTL). Single-flight não é necessário porque o
// custo de gerar o JWKS é baixo (encode JSON de 1-N chaves) e a janela
// de stampede é desprezível.
func (s *Server) jwksFromCacheOrSigner() (*jwksCacheEntry, error) {
	if entry := s.jwksCache.Load(); entry != nil && time.Now().Before(entry.expires) {
		return entry, nil
	}
	session := s.sessionService()
	if session == nil {
		return nil, errSessionUnavailable
	}
	set := session.JWKSet()
	payload, err := json.Marshal(set)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(payload)
	entry := &jwksCacheEntry{
		payload: payload,
		etag:    `W/"` + base64.RawURLEncoding.EncodeToString(hash[:8]) + `"`,
		expires: time.Now().Add(jwksCacheTTL),
	}
	s.jwksCache.Store(entry)
	return entry, nil
}

// NOTE: ao implementar B22 (rotação de chave), adicione um método
// (s *Server) InvalidateJWKSCache() que chame s.jwksCache.Store(nil) e
// invoque-o de dentro do signer logo após a troca. Função removida hoje
// para satisfazer o linter `unused` — sem caller real ainda.

// extractClientLabel sanitiza o ClientLabel recebido em /auth/login.
// Mi4 do bloco 1 pediu cap em ~256 chars; aplicamos aqui antes de
// chegar ao SessionService.
func extractClientLabel(raw string) string {
	label := strings.TrimSpace(raw)
	const maxLabel = 256
	if len(label) > maxLabel {
		label = label[:maxLabel]
	}
	return label
}

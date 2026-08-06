package acpregistry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"assistente/internal/acp"
	"assistente/internal/configdir"
	"assistente/internal/logging"
	httpclient "assistente/internal/tools/http"
)

// IndexURL é o índice do registro. É o único endereço que este pacote busca, e
// é `https` (D9): não há configuração que aponte o catálogo para outro lugar.
const IndexURL = "https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json"

const (
	// DefaultTTL é a idade a partir da qual o catálogo é revalidado. Curto o
	// bastante para acompanhar o cron horário do registro e longo o bastante
	// para não bater na CDN a cada abertura de tela (D2).
	DefaultTTL = 30 * time.Minute

	// requestTimeout limita a busca. O índice tem dezenas de kilobytes; o que
	// este teto evita é uma conexão pendurada segurando a primeira execução, que
	// é a única que espera pela rede.
	requestTimeout = 15 * time.Second

	// maxIndexBytes é o teto de leitura da resposta. Um servidor hostil não
	// enche a memória do app: o corpo é lido por `io.LimitReader` e o que passa
	// do teto é recusado sem ser parseado.
	maxIndexBytes = 4 << 20

	// DefaultRetryAfter é quanto tempo a primeira execução espera antes de
	// tentar buscar de novo depois de falhar. Sem catálogo guardado a busca é
	// síncrona, e sem essa janela quem está offline pagaria o timeout inteiro a
	// cada abertura de tela — a mesma espera, pelo mesmo motivo já conhecido.
	DefaultRetryAfter = time.Minute

	// userAgent segue o formato dos outros clientes HTTP do app.
	userAgent = "Assistente/1.0 (ACP Registry)"
)

const serviceComponent = "acpregistry.service"

// Doer é o mínimo que este pacote precisa do cliente HTTP compartilhado do app
// (`internal/tools/http`), que é quem sabe de timeout, proxy e política de rede.
type Doer interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}

// Config monta o serviço.
type Config struct {
	// HTTP é o cliente compartilhado do app. Vazio monta o cliente da casa com
	// o timeout deste pacote.
	HTTP Doer
	// Dir é o diretório do cache. Vazio usa `~/.assistente/acp-registry`.
	Dir string
	// TTL é a idade a partir da qual o catálogo é revalidado. Vazio usa DefaultTTL.
	TTL time.Duration
	// RetryAfter é a espera entre buscas síncronas quando não há catálogo
	// nenhum para servir. Vazio usa DefaultRetryAfter.
	RetryAfter time.Duration
}

// Service serve o catálogo de agentes do registro ACP.
//
// Ele guarda em memória o índice que está servindo e o carimbo da coleta, e o
// espelha em disco para a tela abrir sem rede. Uma instância atende vários
// leitores ao mesmo tempo; o catálogo devolvido é para leitura, porque é o mesmo
// índice servido a todos.
type Service struct {
	http       Doer
	url        string
	path       string
	ttl        time.Duration
	retryAfter time.Duration
	now        func() time.Time
	maxBytes   int64

	mu           sync.RWMutex
	index        Index
	stamp        time.Time
	reason       string
	reasonCode   Reason
	reasonDetail string
	loaded       bool
	// failedAt é quando a última busca falhou sem haver catálogo para servir.
	// É o que faz a espera pela rede acontecer uma vez, e não a cada abertura.
	failedAt time.Time

	// revalidating garante uma revalidação em segundo plano por vez: várias
	// aberturas de tela seguidas não viram várias buscas na CDN.
	revalidating atomic.Bool
	wg           sync.WaitGroup
}

// Catalog é o que o serviço entrega: o catálogo, quando ele foi coletado e, se
// for o caso, por que ele está velho ou vazio.
type Catalog struct {
	// Version é o `version` do documento, já validado.
	Version string
	// Agents é o catálogo. Vazio quando não houve como carregá-lo.
	Agents []Agent
	// FetchedAt é o carimbo da coleta. Zero quando não há catálogo.
	FetchedAt time.Time
	// Age é a idade do que está sendo servido.
	Age time.Duration
	// FromCache diz que o catálogo veio do que estava guardado, e não de uma
	// busca que acabou de acontecer.
	FromCache bool
	// Stale diz que a idade passou do TTL — o conteúdo continua útil, e é o que
	// a tela informa em texto.
	Stale bool
	// Reason explica, em texto, por que o catálogo está vazio ou por que a
	// última atualização não aconteceu. Vazio quando não há nada a explicar.
	//
	// É o texto de quem lê log e de quem consome este pacote em Go. O que
	// atravessa para a tela é o ReasonCode: a frase precisa existir nos três
	// locales do app, e uma sentença montada aqui só existiria em um.
	Reason string

	// ReasonCode é o mesmo motivo como vocabulário fechado, para a tela dizê-lo
	// no idioma de quem lê. Vazio quando não há nada a explicar.
	ReasonCode Reason

	// ReasonDetail é a parte variável do motivo, quando ele tem uma: o erro de
	// transporte que a tela mostra junto da frase traduzida. Já saneado.
	ReasonDetail string
}

// Reason é o vocabulário dos motivos pelos quais o catálogo está vazio ou
// desatualizado (D2). Cada valor tem ação diferente para quem lê a tela, e é por
// isso que ele não é um booleano de falha.
type Reason string

const (
	// ReasonUnsupportedVersion é o documento com major que este app não lê. A
	// ação é atualizar o app; o cache anterior continua valendo.
	ReasonUnsupportedVersion Reason = "unsupported_version"

	// ReasonMalformedIndex é a origem respondendo algo que não é um índice.
	// Não há ação de quem usa: é problema do outro lado, e o app segue com o que
	// tinha.
	ReasonMalformedIndex Reason = "malformed_index"

	// ReasonCanceled é a busca interrompida — o app fechando no meio dela.
	ReasonCanceled Reason = "canceled"

	// ReasonTimeout é o registro não respondendo no tempo esperado.
	ReasonTimeout Reason = "timeout"

	// ReasonUnreachable é não ter dado para falar com o registro: sem rede,
	// DNS, proxy, TLS. É o desfecho da primeira execução offline.
	ReasonUnreachable Reason = "unreachable"
)

// New monta o serviço.
func New(cfg Config) *Service {
	client := cfg.HTTP
	if client == nil {
		client = httpclient.New(&httpclient.Config{Timeout: requestTimeout}, map[string]string{})
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	retryAfter := cfg.RetryAfter
	if retryAfter <= 0 {
		retryAfter = DefaultRetryAfter
	}
	dir := cfg.Dir
	if dir == "" {
		if home := configdir.GetHomeDir(); home != "" {
			dir = filepath.Join(home, cacheSubdir)
		}
	}
	path := ""
	if dir != "" {
		path = filepath.Join(dir, cacheFileName)
	}

	return &Service{
		http:       client,
		url:        IndexURL,
		path:       path,
		ttl:        ttl,
		retryAfter: retryAfter,
		now:        time.Now,
		maxBytes:   maxIndexBytes,
	}
}

// Catalog devolve o catálogo na hora.
//
// Havendo o que servir, serve — e dispara a revalidação em segundo plano quando
// o carimbo passou do TTL, sem esperar por ela. Sem nada guardado, busca de
// forma síncrona: é a primeira execução, e não há alternativa a esperar. Se essa
// busca falhar, o catálogo volta vazio com o motivo (D2), e a espera não se
// repete enquanto a janela de nova tentativa não vencer: quem está sem rede
// pagaria o timeout de novo a cada abertura de tela para receber o mesmo motivo.
func (s *Service) Catalog(ctx context.Context) Catalog {
	s.ensureLoaded(ctx)

	s.mu.RLock()
	index, stamp, failedAt := s.index, s.stamp, s.failedAt
	reason, reasonCode, reasonDetail := s.reason, s.reasonCode, s.reasonDetail
	s.mu.RUnlock()

	explain := func(catalog Catalog) Catalog {
		catalog.Reason, catalog.ReasonCode, catalog.ReasonDetail = reason, reasonCode, reasonDetail
		return catalog
	}

	if stamp.IsZero() {
		if !failedAt.IsZero() && s.now().Sub(failedAt) < s.retryAfter {
			return explain(s.catalogFrom(Index{}, time.Time{}, true))
		}
		// Primeira execução, ou janela vencida: não há o que servir enquanto
		// isso, e o Refresh já devolve o motivo dentro do catálogo quando a
		// busca não acontece.
		catalog, _ := s.Refresh(ctx)
		return catalog
	}

	catalog := explain(s.catalogFrom(index, stamp, true))
	if catalog.Stale {
		s.revalidate(ctx)
	}
	return catalog
}

// Refresh busca o índice, valida, saneia e grava o cache.
//
// Falha deixa intacto o que estava servindo — em memória e em disco —, e é isso
// que faz um índice malformado, ou um documento de major desconhecido, não
// custar o catálogo que já funcionava (D2). Nesse caso o Catalog devolvido é o
// anterior, com o motivo da falha, e o erro vem junto para quem precisa dele.
func (s *Service) Refresh(ctx context.Context) (Catalog, error) {
	s.ensureLoaded(ctx)

	index, err := s.fetch(ctx)
	if err != nil {
		code, detail := reasonCodeFor(err)
		reason := reasonFor(err)
		logging.Warnf(ctx, serviceComponent, "não foi possível atualizar o índice do registro ACP: %v", err)

		s.mu.Lock()
		s.reason, s.reasonCode, s.reasonDetail = reason, code, detail
		current, stamp := s.index, s.stamp
		if stamp.IsZero() {
			// Sem catálogo para servir, a falha é o que a próxima abertura
			// precisa saber para não esperar pela rede outra vez.
			s.failedAt = s.now()
		}
		s.mu.Unlock()

		catalog := s.catalogFrom(current, stamp, true)
		catalog.Reason, catalog.ReasonCode, catalog.ReasonDetail = reason, code, detail
		return catalog, err
	}

	stamp := s.now()
	s.mu.Lock()
	s.index, s.stamp, s.failedAt = index, stamp, time.Time{}
	s.reason, s.reasonCode, s.reasonDetail = "", "", ""
	s.mu.Unlock()

	if err := saveCache(s.path, index, stamp); err != nil {
		// Cache que não gravou não invalida o catálogo que já está em memória: a
		// tela desta sessão funciona, e a próxima abertura busca de novo.
		logging.Warnf(ctx, serviceComponent, "índice do registro ACP carregado, mas não foi possível gravar o cache: %v", err)
	}

	logging.Infof(ctx, serviceComponent, "índice do registro ACP atualizado: versão %s, %d agentes", index.Version, len(index.Agents))
	return s.catalogFrom(index, stamp, false), nil
}

// ensureLoaded carrega o cache do disco uma vez. Cache ilegível é tratado como
// ausente: gravar em cima dele é o desfecho normal da próxima busca.
func (s *Service) ensureLoaded(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return
	}
	s.loaded = true

	index, stamp, err := loadCache(ctx, s.path)
	if err != nil {
		if errors.Is(err, errCacheUnusable) {
			logging.Warnf(ctx, serviceComponent, "cache do índice do registro ACP será tratado como ausente: %v", err)
		}
		return
	}
	s.index, s.stamp = index, stamp
}

// revalidate dispara uma revalidação em segundo plano, uma por vez.
//
// O contexto de quem abriu a tela não serve aqui: ele morre quando a chamada
// termina, e a revalidação existe justamente para durar mais que ela. O que se
// mantém do contexto original são os valores; o cancelamento fica de fora, e o
// tempo é o do próprio pacote.
func (s *Service) revalidate(ctx context.Context) {
	if !s.revalidating.CompareAndSwap(false, true) {
		return
	}
	background := context.WithoutCancel(ctx)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.revalidating.Store(false)
		// O erro já foi registrado e guardado como motivo pelo Refresh; aqui não
		// há ninguém esperando resposta.
		_, _ = s.Refresh(background)
	}()
}

// fetch busca e valida o documento. Não toca no estado do serviço.
func (s *Service) fetch(ctx context.Context) (Index, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return Index{}, fmt.Errorf("não foi possível montar a busca do índice do registro ACP: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := s.http.Do(ctx, req)
	if err != nil {
		return Index{}, fmt.Errorf("não foi possível falar com o registro ACP: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Só o código: o texto da linha de status é escrito pelo servidor, e
		// mensagem de erro do app acaba em tela e em anúncio.
		return Index{}, fmt.Errorf("o registro ACP respondeu HTTP %d", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); !isJSONContentType(contentType) {
		return Index{}, fmt.Errorf("%w: o registro ACP respondeu com Content-Type %q", ErrMalformedIndex, acp.SanitizeLabel(contentType))
	}

	// io.LimitReader com um byte de folga: dá para saber que passou do teto sem
	// ter lido o excesso, e um servidor hostil não enche a memória do app.
	body, err := io.ReadAll(io.LimitReader(resp.Body, s.maxBytes+1))
	if err != nil {
		return Index{}, fmt.Errorf("não foi possível ler o índice do registro ACP: %w", err)
	}
	if int64(len(body)) > s.maxBytes {
		return Index{}, fmt.Errorf("%w: a resposta passou do limite de %d bytes", ErrMalformedIndex, s.maxBytes)
	}
	return ParseIndex(ctx, body)
}

// catalogFrom monta o valor servido, com a idade calculada na hora.
func (s *Service) catalogFrom(index Index, stamp time.Time, fromCache bool) Catalog {
	catalog := Catalog{
		Version:   index.Version,
		Agents:    index.Agents,
		FetchedAt: stamp,
		// Sem carimbo não há catálogo, e o que não existe não veio do cache.
		FromCache: fromCache && !stamp.IsZero(),
	}
	if stamp.IsZero() {
		return catalog
	}
	catalog.Age = s.now().Sub(stamp)
	if catalog.Age < 0 {
		// Relógio que andou para trás, ou carimbo no futuro: idade negativa não
		// existe, e tratar como recém-buscado é o desfecho inofensivo.
		catalog.Age = 0
	}
	catalog.Stale = catalog.Age >= s.ttl
	return catalog
}

// isJSONContentType aceita o que de fato é JSON. Cabeçalho ausente passa: a CDN
// é quem responde, e recusar por falta de cabeçalho quebraria a busca por um
// detalhe que não muda o conteúdo.
func isJSONContentType(contentType string) bool {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(mediaType)
	return mediaType == "application/json" || mediaType == "text/json" || strings.HasSuffix(mediaType, "+json")
}

// reasonCodeFor classifica a falha. Cada desfecho tem ação diferente, então
// "falha ao carregar" não serve — nem como código, nem como frase.
//
// O detalhe só vem preenchido no desfecho que tem um: o erro de transporte, que
// é a única parte do motivo que este pacote não sabe redigir de antemão. Ele é
// saneado como qualquer outro texto que chega à tela, porque um erro de
// transporte pode carregar o que o outro lado escreveu.
func reasonCodeFor(err error) (Reason, string) {
	switch {
	case err == nil:
		return "", ""
	case errors.Is(err, ErrUnsupportedVersion):
		return ReasonUnsupportedVersion, ""
	case errors.Is(err, ErrMalformedIndex):
		return ReasonMalformedIndex, ""
	case errors.Is(err, context.Canceled):
		return ReasonCanceled, ""
	case errors.Is(err, context.DeadlineExceeded):
		return ReasonTimeout, ""
	default:
		return ReasonUnreachable, acp.SanitizeLabel(err.Error())
	}
}

// reasonFor é o motivo em texto, para o log e para quem consome este pacote em
// Go. A tela não usa esta frase: ela recebe o código e o detalhe, e diz a frase
// dela nos três locales do app.
func reasonFor(err error) string {
	code, detail := reasonCodeFor(err)
	switch code {
	case ReasonUnsupportedVersion:
		return "o índice do registro ACP está num formato que este app ainda não conhece; atualize o app"
	case ReasonMalformedIndex:
		return "o registro ACP respondeu algo que não é um índice válido"
	case ReasonCanceled:
		return "a busca do índice do registro ACP foi interrompida"
	case ReasonTimeout:
		return "o registro ACP não respondeu no tempo esperado"
	case ReasonUnreachable:
		return "não foi possível buscar o índice do registro ACP: " + detail
	default:
		return ""
	}
}

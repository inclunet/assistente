package acpregistry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOIndiceSoEBuscadoNaCDNDoRegistroPorHTTPS(t *testing.T) {
	parsed, err := url.Parse(IndexURL)
	if err != nil {
		t.Fatalf("IndexURL não parseia: %v", err)
	}
	if parsed.Scheme != "https" {
		t.Errorf("esquema = %q, quer https (D9)", parsed.Scheme)
	}
	if parsed.Host != "cdn.agentclientprotocol.com" {
		t.Errorf("host = %q, quer a CDN do registro (D9)", parsed.Host)
	}
	if got := New(Config{Dir: t.TempDir()}).url; got != IndexURL {
		t.Errorf("url do serviço = %q, quer %q", got, IndexURL)
	}
}

func TestUmIndiceBomViraCatalogoTipadoEVaiParaODisco(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	dir := t.TempDir()
	servico, _ := novoServico(t, servidor.URL, dir)

	catalogo := servico.Catalog(context.Background())

	if catalogo.Version != "1.0.0" {
		t.Errorf("versão = %q, quer 1.0.0", catalogo.Version)
	}
	if len(catalogo.Agents) != 3 {
		t.Fatalf("agentes = %d, quer 3", len(catalogo.Agents))
	}
	if catalogo.FromCache {
		t.Error("FromCache = true numa busca que acabou de acontecer")
	}
	if catalogo.Stale {
		t.Error("Stale = true num catálogo recém-buscado")
	}
	if catalogo.Age != 0 {
		t.Errorf("idade = %v, quer 0", catalogo.Age)
	}
	if catalogo.Reason != "" {
		t.Errorf("motivo = %q, quer vazio", catalogo.Reason)
	}
	if catalogo.FetchedAt.IsZero() {
		t.Error("FetchedAt zerado num catálogo carregado")
	}
	if servidor.buscas() != 1 {
		t.Errorf("buscas = %d, quer 1", servidor.buscas())
	}

	if _, err := os.Stat(filepath.Join(dir, cacheFileName)); err != nil {
		t.Fatalf("o cache não foi gravado: %v", err)
	}
	index, carimbo, err := loadCache(context.Background(), filepath.Join(dir, cacheFileName))
	if err != nil {
		t.Fatalf("o cache gravado não volta: %v", err)
	}
	if len(index.Agents) != 3 || carimbo.IsZero() {
		t.Errorf("cache = %d agentes, carimbo %v", len(index.Agents), carimbo)
	}
}

func TestOIndiceMalformadoNaoDerrubaOCacheBom(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	dir := t.TempDir()
	servico, _ := novoServico(t, servidor.URL, dir)

	if _, err := servico.Refresh(context.Background()); err != nil {
		t.Fatalf("primeira carga falhou: %v", err)
	}

	servidor.serve(indiceMalformado)
	catalogo, err := servico.Refresh(context.Background())
	if !errors.Is(err, ErrMalformedIndex) {
		t.Fatalf("erro = %v, quer ErrMalformedIndex", err)
	}
	if len(catalogo.Agents) != 3 {
		t.Errorf("agentes = %d, quer os 3 do cache bom", len(catalogo.Agents))
	}
	if !catalogo.FromCache {
		t.Error("FromCache = false num catálogo que veio do que estava guardado")
	}
	if catalogo.Reason == "" {
		t.Error("motivo vazio depois de uma atualização recusada")
	}

	depois := servico.Catalog(context.Background())
	if len(depois.Agents) != 3 {
		t.Errorf("agentes depois = %d, quer 3", len(depois.Agents))
	}

	// E o disco também: o índice ruim não pode ter passado por cima do bom.
	index, _, err := loadCache(context.Background(), filepath.Join(dir, cacheFileName))
	if err != nil || len(index.Agents) != 3 {
		t.Fatalf("cache em disco = %d agentes, erro %v", len(index.Agents), err)
	}
}

func TestSemRedeOCacheAnteriorEServidoComAIdadeDele(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	dir := t.TempDir()
	servico, relogio := novoServico(t, servidor.URL, dir)

	if _, err := servico.Refresh(context.Background()); err != nil {
		t.Fatalf("primeira carga falhou: %v", err)
	}
	servidor.derruba()
	relogio.avanca(3 * time.Hour)

	catalogo := servico.Catalog(context.Background())
	if len(catalogo.Agents) != 3 {
		t.Fatalf("agentes = %d, quer os 3 do cache", len(catalogo.Agents))
	}
	if !catalogo.FromCache {
		t.Error("FromCache = false servindo cache")
	}
	if catalogo.Age != 3*time.Hour {
		t.Errorf("idade = %v, quer 3h", catalogo.Age)
	}
	if !catalogo.Stale {
		t.Error("Stale = false com idade acima do TTL")
	}

	// A revalidação disparada pela abertura falha em segundo plano, e o motivo
	// fica dito para a próxima leitura — sem trocar o que está servindo.
	servico.wg.Wait()
	depois := servico.Catalog(context.Background())
	if len(depois.Agents) != 3 {
		t.Errorf("agentes depois da revalidação falha = %d, quer 3", len(depois.Agents))
	}
	if depois.Reason == "" {
		t.Error("motivo vazio depois de uma revalidação que não conseguiu falar com o registro")
	}
}

func TestPrimeiraExecucaoSemRedeDevolveCatalogoVazioComOMotivo(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	servidor.derruba()
	dir := t.TempDir()
	servico, _ := novoServico(t, servidor.URL, dir)

	catalogo := servico.Catalog(context.Background())

	if len(catalogo.Agents) != 0 {
		t.Errorf("agentes = %d, quer 0", len(catalogo.Agents))
	}
	if !catalogo.FetchedAt.IsZero() {
		t.Errorf("FetchedAt = %v, quer zero", catalogo.FetchedAt)
	}
	if catalogo.Reason == "" {
		t.Fatal("catálogo vazio sem motivo: a tela não teria o que explicar")
	}
	if catalogo.FromCache {
		t.Error("FromCache = true num catálogo que não veio de cache algum")
	}
	if _, err := os.Stat(filepath.Join(dir, cacheFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cache criado sem índice algum: %v", err)
	}
}

func TestSemCatalogoAEsperaPelaRedeNaoSeRepeteACadaAbertura(t *testing.T) {
	// Sem nada guardado a busca é síncrona, e quem está offline esperaria o
	// timeout inteiro a cada abertura de tela para receber o motivo que já
	// conhece.
	servidor := novoServidor(t, indiceBom)
	servidor.status(http.StatusInternalServerError)
	servico, relogio := novoServico(t, servidor.URL, t.TempDir())

	primeiro := servico.Catalog(context.Background())
	if primeiro.Reason == "" {
		t.Fatal("catálogo vazio sem motivo na primeira abertura")
	}
	if servidor.buscas() != 1 {
		t.Fatalf("buscas na primeira abertura = %d, quer 1", servidor.buscas())
	}

	segundo := servico.Catalog(context.Background())
	if servidor.buscas() != 1 {
		t.Errorf("buscas na segunda abertura = %d, quer 1: a espera não deveria se repetir", servidor.buscas())
	}
	if segundo.Reason != primeiro.Reason {
		t.Errorf("motivo = %q, quer o mesmo da primeira abertura (%q)", segundo.Reason, primeiro.Reason)
	}
	if len(segundo.Agents) != 0 || segundo.FromCache {
		t.Errorf("catálogo = %d agentes, FromCache %v; quer vazio e não vindo de cache", len(segundo.Agents), segundo.FromCache)
	}

	// Passada a janela, tentar de novo é o que traz o catálogo de volta quando
	// a rede volta.
	relogio.avanca(DefaultRetryAfter)
	servidor.serve(indiceBom)
	terceiro := servico.Catalog(context.Background())
	if len(terceiro.Agents) != 3 {
		t.Errorf("agentes depois da janela = %d, quer 3", len(terceiro.Agents))
	}
	if terceiro.Reason != "" {
		t.Errorf("motivo = %q depois de uma busca que deu certo", terceiro.Reason)
	}
}

func TestVersaoDeMajorDesconhecidoERecusadaEOCacheAnteriorPermanece(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	dir := t.TempDir()
	servico, _ := novoServico(t, servidor.URL, dir)

	if _, err := servico.Refresh(context.Background()); err != nil {
		t.Fatalf("primeira carga falhou: %v", err)
	}

	servidor.serve(indiceDeMajorDesconhecido)
	catalogo, err := servico.Refresh(context.Background())
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("erro = %v, quer ErrUnsupportedVersion", err)
	}
	if len(catalogo.Agents) != 3 || catalogo.Version != "1.0.0" {
		t.Errorf("catálogo = versão %q com %d agentes, quer o anterior intacto", catalogo.Version, len(catalogo.Agents))
	}
	if !strings.Contains(catalogo.Reason, "atualize o app") {
		t.Errorf("motivo = %q, quer dizer que o app precisa ser atualizado", catalogo.Reason)
	}

	index, _, err := loadCache(context.Background(), filepath.Join(dir, cacheFileName))
	if err != nil || index.Version != "1.0.0" {
		t.Fatalf("cache em disco = versão %q, erro %v", index.Version, err)
	}
}

func TestIndiceSemAgentesNaoSubstituiOCacheBom(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	dir := t.TempDir()
	servico, _ := novoServico(t, servidor.URL, dir)

	if _, err := servico.Refresh(context.Background()); err != nil {
		t.Fatalf("primeira carga falhou: %v", err)
	}

	servidor.serve(indiceSemAgentes)
	catalogo, err := servico.Refresh(context.Background())
	if !errors.Is(err, ErrMalformedIndex) {
		t.Fatalf("erro = %v, quer ErrMalformedIndex", err)
	}
	if len(catalogo.Agents) != 3 {
		t.Errorf("agentes = %d, quer os 3 do cache bom", len(catalogo.Agents))
	}
}

func TestCacheCorrompidoEmDiscoEhTratadoComoAusente(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, cacheFileName), []byte("{isso não é json"), 0o600); err != nil {
		t.Fatalf("não foi possível preparar o cache corrompido: %v", err)
	}
	servico, _ := novoServico(t, servidor.URL, dir)

	catalogo := servico.Catalog(context.Background())
	if len(catalogo.Agents) != 3 {
		t.Fatalf("agentes = %d, quer 3 (buscados porque o cache não servia)", len(catalogo.Agents))
	}
	if catalogo.FromCache {
		t.Error("FromCache = true com cache corrompido no disco")
	}

	// E o arquivo bom passou por cima do corrompido.
	if _, _, err := loadCache(context.Background(), filepath.Join(dir, cacheFileName)); err != nil {
		t.Errorf("o cache não foi regravado: %v", err)
	}
}

func TestCancelamentoPorContextoNaoTrocaOCatalogo(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	dir := t.TempDir()
	servico, _ := novoServico(t, servidor.URL, dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	catalogo, err := servico.Refresh(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, quer context.Canceled", err)
	}
	if len(catalogo.Agents) != 0 {
		t.Errorf("agentes = %d, quer 0", len(catalogo.Agents))
	}
	if !strings.Contains(catalogo.Reason, "interrompida") {
		t.Errorf("motivo = %q, quer dizer que a busca foi interrompida", catalogo.Reason)
	}
	if _, err := os.Stat(filepath.Join(dir, cacheFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cache gravado a partir de uma busca cancelada: %v", err)
	}
}

// Um servidor hostil não pode encher a memória do app: o corpo é lido com teto e
// o que passa dele é recusado antes de qualquer desserialização.
func TestRespostaGiganteEhRecusadaSemDerrubarOCacheBom(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	dir := t.TempDir()
	servico, _ := novoServico(t, servidor.URL, dir)

	if _, err := servico.Refresh(context.Background()); err != nil {
		t.Fatalf("primeira carga falhou: %v", err)
	}

	servico.maxBytes = 64
	servidor.serve(indiceBom)
	catalogo, err := servico.Refresh(context.Background())
	if !errors.Is(err, ErrMalformedIndex) {
		t.Fatalf("erro = %v, quer ErrMalformedIndex", err)
	}
	if !strings.Contains(err.Error(), "limite") {
		t.Errorf("erro = %v, quer mencionar o limite", err)
	}
	if len(catalogo.Agents) != 3 {
		t.Errorf("agentes = %d, quer os 3 do cache bom", len(catalogo.Agents))
	}
}

func TestOContentTypeInesperadoEhRecusado(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	servidor.serveComo(indiceBom, "text/html; charset=utf-8")
	servico, _ := novoServico(t, servidor.URL, t.TempDir())

	catalogo, err := servico.Refresh(context.Background())
	if !errors.Is(err, ErrMalformedIndex) {
		t.Fatalf("erro = %v, quer ErrMalformedIndex", err)
	}
	if len(catalogo.Agents) != 0 {
		t.Errorf("agentes = %d, quer 0", len(catalogo.Agents))
	}
}

func TestOsContentTypesAceitosSaoOsQueSaoJSON(t *testing.T) {
	casos := map[string]bool{
		"":                                true,
		"application/json":                true,
		"application/json; charset=utf-8": true,
		"Application/JSON":                true,
		"text/json":                       true,
		"application/vnd.acp+json":        true,
		"text/html":                       false,
		"application/octet-stream":        false,
		"text/plain":                      false,
		"isso não é media type":           false,
	}
	for contentType, quer := range casos {
		t.Run(contentType, func(t *testing.T) {
			if got := isJSONContentType(contentType); got != quer {
				t.Errorf("isJSONContentType(%q) = %v, quer %v", contentType, got, quer)
			}
		})
	}
}

func TestORegistroQueRespondeErroHTTPNaoTrocaOCatalogo(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	servico, _ := novoServico(t, servidor.URL, t.TempDir())

	if _, err := servico.Refresh(context.Background()); err != nil {
		t.Fatalf("primeira carga falhou: %v", err)
	}

	servidor.status(http.StatusServiceUnavailable)
	catalogo, err := servico.Refresh(context.Background())
	if err == nil {
		t.Fatal("erro nulo para resposta 503")
	}
	if len(catalogo.Agents) != 3 {
		t.Errorf("agentes = %d, quer os 3 anteriores", len(catalogo.Agents))
	}
}

func TestARevalidacaoEmSegundoPlanoNaoBloqueiaEAtualizaOCatalogo(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	servico, relogio := novoServico(t, servidor.URL, t.TempDir())

	if len(servico.Catalog(context.Background()).Agents) != 3 {
		t.Fatal("primeira carga não trouxe os 3 agentes")
	}

	relogio.avanca(DefaultTTL + time.Minute)
	servidor.serve(indiceComOutroAgente)

	// A abertura da tela serve o que está guardado na hora, sem esperar a rede.
	imediato := servico.Catalog(context.Background())
	if len(imediato.Agents) != 3 || !imediato.FromCache {
		t.Errorf("catálogo imediato = %d agentes, FromCache %v; quer os 3 do cache", len(imediato.Agents), imediato.FromCache)
	}

	servico.wg.Wait()
	depois := servico.Catalog(context.Background())
	if len(depois.Agents) != 1 || depois.Agents[0].ID != "kimi" {
		t.Fatalf("catálogo depois da revalidação = %+v, quer só o kimi", depois.Agents)
	}
	if depois.Stale {
		t.Error("Stale = true logo depois de uma revalidação bem sucedida")
	}
}

func TestVariosLeitoresConcorrentesRecebemOCatalogoInteiro(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	servico, relogio := novoServico(t, servidor.URL, t.TempDir())

	if _, err := servico.Refresh(context.Background()); err != nil {
		t.Fatalf("primeira carga falhou: %v", err)
	}
	relogio.avanca(DefaultTTL + time.Minute)

	const leitores = 8
	var grupo sync.WaitGroup
	erros := make(chan int, leitores)
	for i := 0; i < leitores; i++ {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			if n := len(servico.Catalog(context.Background()).Agents); n != 3 {
				erros <- n
			}
		}()
	}
	grupo.Wait()
	servico.wg.Wait()
	close(erros)
	for n := range erros {
		t.Errorf("um leitor concorrente recebeu %d agentes, quer 3", n)
	}

	// Vários leitores ao mesmo tempo não viram várias buscas na CDN.
	if buscas := servidor.buscas(); buscas > 2 {
		t.Errorf("buscas = %d, quer no máximo 2 (a inicial e uma revalidação)", buscas)
	}
}

// Um leitor que mexe no que recebeu não estraga a leitura do próximo: o
// catálogo entregue é cópia, e não o índice guardado pelo serviço.
func TestOQueUmLeitorMexeNaoAlcancaOsOutros(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	servico, _ := novoServico(t, servidor.URL, t.TempDir())

	primeiro := servico.Catalog(context.Background())
	if len(primeiro.Agents) != 3 {
		t.Fatalf("catálogo = %d agentes, quer 3", len(primeiro.Agents))
	}
	agente := agentePorID(t, primeiro.Agents, "goose")
	nomeOriginal := agente.Name
	alvoOriginal := agente.Distribution.Binary["windows-x86_64"]

	for i := range primeiro.Agents {
		primeiro.Agents[i].Name = "nome trocado"
		for j := range primeiro.Agents[i].Authors {
			primeiro.Agents[i].Authors[j] = "autor inventado"
		}
		for alvo := range primeiro.Agents[i].Distribution.Binary {
			primeiro.Agents[i].Distribution.Binary[alvo] = BinaryTarget{Cmd: "comando trocado"}
		}
		if npx := primeiro.Agents[i].Distribution.NPX; npx != nil {
			npx.Package = "pacote trocado"
		}
	}

	segundo := servico.Catalog(context.Background())
	depois := agentePorID(t, segundo.Agents, "goose")
	if depois.Name != nomeOriginal {
		t.Errorf("nome = %q, quer %q", depois.Name, nomeOriginal)
	}
	if got := depois.Distribution.Binary["windows-x86_64"]; got.Cmd != alvoOriginal.Cmd || got.SHA256 != alvoOriginal.SHA256 {
		t.Errorf("alvo binário = %+v, quer %+v", got, alvoOriginal)
	}
	if len(segundo.Agents) != 3 {
		t.Errorf("catálogo seguinte = %d agentes, quer 3", len(segundo.Agents))
	}
}

// A busca manda os cabeçalhos que identificam o app e o que ele espera receber.
func TestABuscaSeIdentificaEPedeJSON(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	servico, _ := novoServico(t, servidor.URL, t.TempDir())

	if _, err := servico.Refresh(context.Background()); err != nil {
		t.Fatalf("busca falhou: %v", err)
	}
	cabecalhos := servidor.ultimosCabecalhos()
	if got := cabecalhos.Get("User-Agent"); got != userAgent {
		t.Errorf("User-Agent = %q, quer %q", got, userAgent)
	}
	if got := cabecalhos.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q, quer application/json", got)
	}
}

// O motivo é texto de tela e de anúncio, inclusive quando vem de um erro de
// transporte que carrega o que o outro lado escreveu.
func TestOMotivoDaFalhaEhSaneadoAntesDeVirarTexto(t *testing.T) {
	sujo := errors.New("falhou \x1b[31mvermelho\x1b[0m com\nquebra e \u202Eversão")
	motivo := reasonFor(sujo)

	if strings.ContainsAny(motivo, "\x1b\n\r\u202e") {
		t.Errorf("motivo = %q, ainda carrega escape, quebra ou marca invisível", motivo)
	}
	if !strings.Contains(motivo, "não foi possível buscar o índice") {
		t.Errorf("motivo = %q, quer nomear o que falhou", motivo)
	}
	if reasonFor(nil) != "" {
		t.Errorf("reasonFor(nil) = %q, quer vazio", reasonFor(nil))
	}
}

// relogio é o tempo controlado pelo teste. Ele é lido também pela revalidação em
// segundo plano, e por isso é protegido.
type relogio struct {
	mu     sync.Mutex
	agoraT time.Time
}

func novoRelogio() *relogio {
	return &relogio{agoraT: time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)}
}

func (r *relogio) agora() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.agoraT
}

func (r *relogio) avanca(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agoraT = r.agoraT.Add(d)
}

// servidorDeIndice é o registro de mentira: serve o corpo que o teste apontar,
// conta as buscas e sabe cair.
type servidorDeIndice struct {
	*httptest.Server

	mu           sync.Mutex
	corpo        string
	contentType  string
	codigo       int
	contagem     int
	ultimoHeader http.Header
}

func novoServidor(t *testing.T, corpo string) *servidorDeIndice {
	t.Helper()
	servidor := &servidorDeIndice{corpo: corpo, contentType: "application/json", codigo: http.StatusOK}
	servidor.Server = httptest.NewServer(servidor)
	t.Cleanup(servidor.Close)
	return servidor
}

func (s *servidorDeIndice) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	corpo, contentType, codigo := s.corpo, s.contentType, s.codigo
	s.contagem++
	s.ultimoHeader = r.Header.Clone()
	s.mu.Unlock()

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(codigo)
	_, _ = w.Write([]byte(corpo))
}

func (s *servidorDeIndice) serve(corpo string) {
	s.serveComo(corpo, "application/json")
}

func (s *servidorDeIndice) serveComo(corpo, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.corpo, s.contentType, s.codigo = corpo, contentType, http.StatusOK
}

func (s *servidorDeIndice) status(codigo int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codigo = codigo
}

func (s *servidorDeIndice) buscas() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contagem
}

func (s *servidorDeIndice) ultimosCabecalhos() http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ultimoHeader
}

// derruba fecha o servidor mantendo a URL: é o "sem rede" dos testes, sem
// depender de rede de verdade nem de um endereço inventado.
func (s *servidorDeIndice) derruba() {
	s.Close()
}

// novoServico monta o serviço apontado para o servidor de teste, com relógio
// controlado. A URL é fixa no pacote de propósito (só a CDN do registro, D9), e
// o teste a troca por dentro — é o mesmo pacote.
func novoServico(t *testing.T, endereco, dir string) (*Service, *relogio) {
	t.Helper()
	relogio := novoRelogio()
	servico := New(Config{Dir: dir})
	servico.url = endereco
	servico.now = relogio.agora
	// Nenhuma revalidação em segundo plano sobrevive ao fim do teste.
	t.Cleanup(servico.wg.Wait)
	return servico, relogio
}

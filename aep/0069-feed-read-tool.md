# AEP-0069 — Tool `feed_read` (RSS/Atom/JSON Feed/Podcast → JSON canônico)

Status: Draft
Data: 2026-06-04
Autor: Inclunet + Cursor Agent

## Resumo

Esta AEP introduz uma **única builtin tool `feed_read`**, stateless e read-only,
que busca uma URL de feed e a converte para um **JSON canônico** estável,
independente do formato de origem. Suporta **RSS (0.9–2.0)**, **Atom (0.3/1.0)**,
**JSON Feed** e **feeds de podcast** (extensão iTunes: duração, episódio/temporada,
enclosures de áudio).

A tool **reusa a pilha HTTP existente** (`internal/tools/http` + `credentials.Manager`),
de modo que feeds protegidos são lidos com **autenticação automática por domínio**
(bearer/basic/custom/oauth2) — o modelo nunca manipula tokens. O resultado JSON
vira matéria-prima para outras tools e para o LLM (resumir, classificar, extrair,
gerar tasks/cards, etc.).

Escopo da v1 é deliberadamente mínimo: **sem persistência, sem polling, sem UI,
sem scraping de HTML arbitrário**.

## Motivação

Hoje o produto só tem ferramentas de HTTP cru/HTML:

- `web_fetch` (`internal/tools/web/web_fetch.go`) — GET + extração de texto/markdown
  de HTML;
- `http_request` — chamadas REST genéricas;
- `web_search` — busca web.

Nenhuma delas entende **feeds**: o LLM teria que baixar XML/JSON bruto e parsear
"na unha", sem normalização de datas, autores, enclosures ou metadados de podcast.
Feeds são uma fonte recorrente e estruturada de informação (notícias, blogs,
releases, podcasts), e convertê-los a um JSON canônico habilita pipelines simples
com as tools já existentes.

## Decisões

- **D1 — Reuso da pilha HTTP + credmanager.** `feed_read` constrói um
  `httpclient.Client` via `httpclient.New(&httpclient.Config{CredentialManager: credMgr})`,
  igual a `web_fetch`/`http_request`. A autenticação é resolvida por domínio
  (`Manager.ResolveForURLWithContext`); o modelo nunca vê credenciais. Atende ao
  requisito de "ler autenticado usando credmanager" sem nova superfície de auth.
- **D2 — Parser: `github.com/mmcdole/gofeed`.** Biblioteca de fato em Go para
  RSS/Atom/JSON Feed, com extensão iTunes/podcast (enclosures, duração,
  episódio/temporada). Detecção de formato é automática.
- **D3 — Fetch desacoplado do parse.** A função pura `parseFeed(io.Reader, opts)`
  faz toda a normalização e é testada com fixtures, sem rede. `Execute` apenas
  monta o GET (User-Agent + Accept de feed), aplica `io.LimitReader` (10MB) e
  delega ao `parseFeed`. Isso facilita testes e a evolução para a fase 2.
- **D4 — JSON canônico estável.** Forma única para todos os formatos: metadados do
  feed + itens normalizados (datas em RFC3339 quando parseáveis, autores como
  lista de nomes, categorias, enclosures). Quando há extensão iTunes ou enclosure
  de áudio, `is_podcast=true` e os blocos `podcast` (feed e item) são preenchidos.
- **D5 — Read-only e seguro.** Tool sem efeitos colaterais; `Risk: "network"` no
  catálogo. Bloqueia hosts locais/privados (defesa básica anti-SSRF), espelhando
  `web_fetch`. Exige `http`/`https`.
- **D6 — Parâmetros enxutos.** `url` (obrigatório), `max_items` (default 20),
  `include_content` (default false), `strip_html` (default true), `since`
  (RFC3339; filtra itens mais antigos, mantendo itens sem data parseável).

## Relação com outras AEPs

- **AEP-0016/0017 (http_request tool e segurança)** e **AEP-0018/0019 (cliente HTTP
  unificado/centralização)**: `feed_read` é mais um consumidor da pilha
  `internal/tools/http`, herdando timeout/retry e o interceptor de auth.
- **AEP-0014/0015 (persistência e auto-credencial)**: a auth por domínio sai do
  `credentials.Manager`; basta uma credencial registrada para o host do feed.
- **AEP-0002/0039 (tool calling)**: `feed_read` é uma `tools.Tool` comum,
  registrada em `app_tool_registry.go` e exposta ao LLM pelo fluxo padrão.

## Fora de escopo / evolução (fase 2)

- **Assinaturas persistidas** (feeds inscritos + itens no banco, com dedup por GUID).
- **Polling periódico** via sistema de jobs (AEP-0001: triggers `cron`/`interval`)
  e eventos ao chegar item novo.
- **HTML**: autodiscovery de `<link rel="alternate">` e/ou scraping configurável
  por seletores CSS.
- **UI** de leitor de feeds.

A estrutura `parseFeed` desacoplada já deixa a porta aberta para essas evoluções
sem reescrever a normalização.

## Arquivos

- `internal/tools/feed/parse.go` — `parseFeed` + tipos `Canonical*`.
- `internal/tools/feed/html.go` — `htmlToText` (strip de HTML em summary/content).
- `internal/tools/feed/feed_read.go` — `FeedRead` (`tools.Tool`).
- `internal/tools/feed/feed_read_test.go` — fixtures RSS/Atom/JSON/podcast.
- `internal/app/app_tool_registry.go` — registro da tool.
- `internal/tools/catalog.go` — metadado (`Risk: network`).

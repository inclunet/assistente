# AEP-0070 — Tool `web_search` (busca web → JSON canônico paginável)

Status: Done — saída canônica paginável implementada em `internal/tools/web/web_search.go` e testes.
Brave, Tavily e Bing implementadas como elos da cadeia
(`brave_provider.go`, `tavily_provider.go`, `bing_provider.go`; cadeia
Brave → Tavily → Bing → DuckDuckGo em `searchWithFallback`). Autenticação
manual única via `WithManualAuth` em `internal/tools/http`.
Data: 2026-06-05
Autor: Inclunet + Cursor Agent

## Resumo

Esta AEP documenta a **builtin tool `web_search`**, stateless e read-only, que
executa uma busca na web e retorna um **JSON canônico** estável:
`{query, provider, offset, count, has_more, results[{title, url, snippet}]}`.

O formato JSON é o **contrato canônico** (não há mais saída em markdown): serve tanto
para LLMs quanto para **consumo programático em jobs** — o job executor faz
`json.Unmarshal` do `Content` da tool, então uma saída estruturada vira diretamente
o output do job (encadeável em `output.map`). A tool também expõe **paginação por
offset**, permitindo varrer mais páginas de resultados quando necessário.

A descoberta de conteúdo é separada da leitura: `web_search` devolve **links e
trechos**; para ler o conteúdo de um resultado, chama-se `web_fetch` na URL
escolhida. A tool usa a cadeia **Brave → Tavily → Bing → DuckDuckGo**,
atrás da interface `SearchProvider` plugável: com chave Brave cadastrada
no credmanager usa-se a Brave; sem credencial (ou com 401/403/429/422),
avança para a **Tavily** (chave no credmanager; 401/403/429/432/433
avançam); sem credencial Tavily, com limite de plano ou com offset além
da janela de 20, avança para o **Bing** (chave no credmanager;
401/403/429 avançam); sem credencial Bing, cai para o **DuckDuckGo**
(HTML, sem API key) como fallback universal.

Escopo: **sem persistência, sem cache, sem ranking próprio** — as chaves
vivem exclusivamente no credmanager (nunca em env/flag/argumento) e o
contrato JSON permanece estável independente do provedor que respondeu.
Cada provedor resolve a credencial uma única vez e declara auth manual
(`httpclient.WithManualAuth`), para o interceptor não resolver nem injetar
de novo — o Bing inclusive rejeita com 401 múltiplos métodos de auth na
mesma request.

## Motivação

O produto precisa de uma capacidade de descoberta na web que sirva aos dois modos de
consumo:

- **LLM** — o modelo descobre links relevantes e decide o que ler em seguida;
- **Jobs/automações** — fluxos não-interativos precisam dos dados de busca de forma
  estruturada para filtrar, encadear e agir programaticamente.

Originalmente a tool retornava **markdown**, ótimo para o LLM mas inútil para jobs: o
executor de jobs faz `json.Unmarshal` do `Content`, e markdown virava apenas uma
string crua, sem `url`/`title`/`snippet` acessíveis. JSON é bom para os dois
consumos, então ter um toggle de formato adicionaria complexidade sem benefício
claro — por isso JSON passou a ser o **formato canônico único**.

Além disso, uma única página de resultados frequentemente não basta (pesquisa,
varredura de fontes); por isso a tool ganhou **paginação por offset**.

## Decisões

- **D1 — JSON canônico, sem toggle de formato.** A saída é sempre o objeto
  `{query, provider, offset, count, has_more, results[]}`. Mesmo sem resultados, a
  estrutura é válida (`results: []`, `has_more: false`), evitando que o chamador
  programático precise tratar texto. Isso espelha o padrão de `feed_read`
  (AEP-0069), que também retorna JSON canônico no `Content`.
- **D2 — Paginação por offset.** Parâmetro de entrada `offset` (0-based, default 0).
  Para a próxima página: `offset = offset anterior + count`. O provedor recebe
  `(query, offset, maxResults)`.
- **D3 — `has_more` heurístico.** Nem todo provedor expõe total de
  resultados (o fallback DuckDuckGo é scraping de HTML), então `has_more` é
  estimado pela página ter vindo "cheia" (`count >= max_results`). É
  documentado como heurística; o contrato (offset/count/has_more) é
  independente de provedor, então trocar ou encadear provedores não quebra
  consumidores.
- **D4 — Reuso da pilha HTTP + credmanager.** `web_search` constrói um
  `httpclient.Client` via `httpclient.New(...)`, igual a `web_fetch`/`http_request`/
  `feed_read`, herdando timeout/retry e o interceptor de auth (AEP-0018/0019).
- **D5 — Provedor plugável com cadeia padrão.** Interface `SearchProvider`
  (`Search(ctx, client, query, offset, maxResults) ([]SearchResult, error)` + `Name()`).
  A cadeia padrão é Brave → Tavily → Bing → DuckDuckGo (`searchWithFallback`):
  cada elo com credencial no credmanager é tentado em ordem; sem credencial
  ou com status fallbackable, avança. `NewWebSearchWithProvider` permite
  injetar outro ou um mock em testes.
- **D6 — Descoberta separada da leitura.** `web_search` não baixa o conteúdo das
  páginas — só retorna links/trechos. Ler conteúdo é responsabilidade de `web_fetch`
  (que aplica a barreira anti-SSRF ao buscar a URL escolhida). Assim a busca não
  precisa validar host: ela só consulta o endpoint fixo do provedor.
- **D7 — Parâmetros enxutos.** `query` (obrigatório), `max_results` (default 8, máx
  20 por página), `offset` (default 0), `provider` (default `auto`; `brave`,
  `tavily`, `bing` ou `duckduckgo` para iniciar a cadeia num elo específico).
  `Risk: network` no catálogo.
- **D9 — Seleção pelo modelo + aviso de fallback.** O LLM escolhe o elo
  inicial via `provider` (ex.: após resultado fraco, tenta outra fonte); o
  servidor impõe disponibilidade e avança pela cadeia, reportando quem
  atendeu no campo `provider`. Quando o DuckDuckGo atende sem ter sido
  pedido, a saída traz `notice` (omitempty, sem quebrar jobs) orientando a
  cadastrar chave de um provedor melhor. Pedido explícito de DDG não recebe
  aviso. O match é por chave canônica, nunca por nome de exibição.
- **D8 — Auth manual única.** Cada provedor com API resolve a credencial uma
  única vez, aplica o material na request e chama `client.Do` com
  `httpclient.WithManualAuth(ctx)`; o interceptor pula a resolução. Evita
  duplo custo/efeito em fontes dinâmicas e headers redundantes (o Bing
  retorna 401 com múltiplos métodos de auth na mesma request).

## Realidade do código (estado atual)

- Saída JSON: `webSearchJSONOutput{Query, Provider, Offset, Count, HasMore, Results}`.
- `SearchResult{Title, URL, Snippet}` (tags JSON).
- Provedor DuckDuckGo: GET em `https://html.duckduckgo.com/html/?q=...`, com `&s=offset`
  quando `offset > 0`; parse do HTML lite (`result__a`/`result__snippet`) e extração
  da URL real do redirect (`uddg=`). É o fallback da cadeia padrão.
- Provedor Brave: GET em `https://api.search.brave.com/res/v1/web/search`
  (`q`/`count`/`offset`), header `X-Subscription-Token` com chave do
  credmanager; parse de `web.results[]` (`title`/`url`/`description`). Sem
  credencial ou com 401/403/429/422, avança na cadeia.
- Provedor Bing: GET em `https://api.bing.microsoft.com/v7.0/search`
  (`q`/`count`/`offset`, paginação nativa), header
  `Ocp-Apim-Subscription-Key` com chave do credmanager; parse de
  `webPages.value[]` (`name`/`url`/`snippet`; bloco ausente = zero matches,
  por semântica documentada da API). Sem credencial ou com 401/403/429,
  avança na cadeia.
- Provedor Tavily: POST em `https://api.tavily.com/search`
  (`query`/`search_depth=basic`/`max_results`), header `Authorization:
  Bearer` com chave do credmanager; parse de `results[]`
  (`title`/`url`/`content`, `answer` ignorado). A API não tem offset: serve
  a janela inicial pedindo `offset+maxResults` (teto 20) e fatia
  localmente; offset além do teto, sem credencial ou com
  401/403/429/432/433, avança na cadeia sem gastar créditos.
- Limites: `max_results` default 8, teto 20; body limitado a 2MB no fetch do provedor.
- Registro em `internal/app/app_tool_registry.go` (`web.NewWebSearch(a.credMgr)`).

## Relação com outras AEPs

- **AEP-0069 (feed_read)**: mesmo princípio de "JSON canônico no `Content`" para
  consumo por LLM e por jobs.
- **AEP-0016/0017 (http_request e segurança)** e **AEP-0018/0019 (cliente HTTP
  unificado/centralização)**: `web_search` é mais um consumidor da pilha
  `internal/tools/http`.
- **AEP-0001 (jobs)** e **AEP-0063 (tool invocations/executor comum)**: o executor de
  jobs faz `json.Unmarshal` do `Content`; o JSON canônico habilita encadear busca em
  `output.map` e em pipelines automatizados.
- **AEP-0002/0039 (tool calling)**: `web_search` é uma `tools.Tool` comum, exposta ao
  LLM pelo fluxo padrão.

## Fora de escopo / evolução (próximas fases)

- **Demais provedores com API key**: ranking melhor, paginação
  confiável e **total de resultados real** (substituindo o `has_more`
  heurístico por um valor exato/`total`). A interface `SearchProvider` já
  comporta isso. **Brave implementado**: `braveProvider` consulta a Brave
  Search API (`/res/v1/web/search`, `count`/`offset`) com chave resolvida por
  URL no credmanager (`api.search.brave.com`, bearer ou custom com header
  `X-Subscription-Token`); sem credencial ou com 401/403/429/422, a tool
  avança na cadeia, com o provedor atendente identificado pelo campo
  `provider`. Evidências: `internal/tools/web/brave_provider.go`,
  `internal/tools/web/brave_provider_test.go`, `searchWithFallback` em
  `internal/tools/web/web_search.go`.
- **Tavily implementada**: `tavilyProvider` consulta a Tavily Search API
  (`POST /search`, `search_depth=basic`) com chave Bearer resolvida por URL
  no credmanager (`api.tavily.com`); parse de `results[]`
  (`title`/`url`/`content`, `answer` ignorado). Sem credencial, com
  401/403/429/432/433 ou com offset além da janela de 20, avança na cadeia
  (offset excedente não consome créditos). Evidências:
  `internal/tools/web/tavily_provider.go`,
  `internal/tools/web/tavily_provider_test.go`.
- **Bing implementado**: `bingProvider` consulta a Bing Web Search API v7
  (`GET /v7.0/search`, `count`/`offset` nativos) com chave resolvida por URL
  no credmanager (`api.bing.microsoft.com`, bearer ou custom com header
  `Ocp-Apim-Subscription-Key`); sem credencial ou com 401/403/429, avança
  na cadeia. Usa `WithManualAuth` (o Bing rejeita múltiplos métodos de auth
  com 401). Evidências: `internal/tools/web/bing_provider.go`,
  `internal/tools/web/bing_provider_test.go`.
- **Seleção por preferência do usuário** (perfil/config): a seleção atual é
  do modelo via parâmetro `provider`, com disponibilidade imposta pelo
  servidor; ordem preferencial configurável permanece futura.
- **Parâmetros de busca**: região/idioma (`region`, `language`), `safe_search`,
  janela temporal (`freshness`), e tipos de resultado (web/news/images).
- **Dedup e normalização** de URLs entre páginas (evitar repetição ao paginar).
- **Cache de curta duração** por `(query, offset)` para reduzir requisições repetidas
  em jobs e melhorar latência.
- **Robustez do scraping**: tolerância a mudanças de layout do DuckDuckGo e métricas
  de parsing vazio (alertar quando o seletor parar de casar).

A interface `SearchProvider` desacoplada e o contrato JSON estável já deixam a porta
aberta para essas evoluções sem quebrar os consumidores atuais.

## Provedores futuros (intenção)

A arquitetura `SearchProvider` é propositalmente plugável: cada buscador é uma
implementação isolada de `Search(ctx, client, query, offset, maxResults)` que devolve
`[]SearchResult`, e a tool apenas serializa o JSON canônico. Isso permite oferecer
vários backends de busca, selecionáveis por configuração/credencial, com fallback
para o DuckDuckGo quando não houver chave configurada. A intenção é suportar três
classes de provedores:

### 1. APIs oficiais (preferenciais quando houver credencial)

- **Brave Search API — implementada** (primeiro elo da cadeia): resultados
  de qualidade, paginação e contagem confiáveis; requer API key no
  credmanager.
- **Tavily Search API — implementada** (segundo elo): busca otimizada para
  agentes, com trechos de conteúdo densos por fonte; `search_depth=basic`
  (1 crédito); requer API key no credmanager.
- **Bing Web Search API — implementada** (terceiro elo): índice distinto
  (Microsoft), paginação nativa `count`/`offset`, `safeSearch` moderado;
  requer subscription key no credmanager (`api.bing.microsoft.com`).
  Bloco `webPages` ausente = zero matches (semântica da API).
- Outras APIs especializadas (ex.: SerpAPI-like) podem entrar pela mesma
  interface.
- **Google CSE — descartado**: Custom Search JSON API fechada para novos
  clientes, com desligamento em 01/01/2027; sem caminho público viável.

Vantagens: paginação determinística, `total` exato (substitui o `has_more`
heurístico), região/idioma/safe-search nativos e menor risco de quebra.

### 2. Busca via modelo (LLM com web search)

- **OpenAI no modo web search** (e equivalentes que exponham busca nativa): o
  provedor delega a busca ao modelo e normaliza a resposta para `SearchResult`
  (título/URL/snippet). Útil quando já há credencial de LLM e se quer resultados
  "curados" pelo modelo. Atenção a custo por chamada e à necessidade de extrair
  URLs/citações de forma estruturada.

### 3. Web scraping (ousadias, sem API key)

Para cenários sem credencial, manter alternativas por scraping — explicitamente
assumidas como **frágeis e best-effort**:

- **DuckDuckGo HTML** — provedor atual (fallback universal).
- **Google via scraping** — extrai resultados da página de busca pública.
- Outros buscadores conforme necessidade.

Riscos/cuidados a documentar e tratar nesses provedores: mudança de layout (parsing
quebra), bloqueio/captcha e rate limiting, e conformidade com os Termos de Uso de
cada buscador. Por isso scraping fica como camada de fallback, atrás das APIs
oficiais quando disponíveis, com métricas de "parsing vazio" para detectar quebras.

### Seleção e fallback (implementado)

- Cadeia Brave → Tavily → Bing → DuckDuckGo: com credencial Brave usa-se
  a Brave; sem credencial ou com 401/403/429/422, avança para a Tavily; sem
  credencial Tavily, com 401/403/429/432/433 ou com offset além da janela
  de 20, avança para o Bing; sem credencial Bing ou com 401/403/429, cai
  para o DuckDuckGo.
- O modelo escolhe o elo inicial via parâmetro `provider` (`auto` = cadeia
  completa); valor desconhecido é erro com a lista válida. O servidor nunca
  expõe previamente quais chaves existem: a disponibilidade é imposta em
  runtime e o atendente é reportado post-hoc no campo `provider`.
- Queda no DuckDuckGo sem pedido explícito devolve `notice` orientando a
  configurar Brave/Tavily/Bing no credmanager. Seleção por preferência do
  usuário (perfil) permanece futura.
- O campo `provider` no JSON canônico já identifica qual backend respondeu, de modo
  transparente para LLM e jobs.
- Cadeia de fallback automática em caso de erro/quota de um provedor.

A adição de qualquer um desses provedores **não altera o contrato JSON** nem os
parâmetros da tool — apenas troca a implementação por trás de `SearchProvider`.

## Fases da v1

- [x] Contrato JSON canônico e resultado estruturado.
- [x] Provedor DuckDuckGo atrás de `SearchProvider`.
- [x] Paginação por `offset`, limites e `has_more` heurístico.
- [x] Registro no catálogo como builtin de risco `network`.
- [x] Testes unitários e HTTP fake sem dependência de rede externa.

## Critérios de aceitação da v1

- [x] Resultado contém query, provider, offset, count, has_more e results.
- [x] Página vazia preserva `results: []`.
- [x] `offset` chega ao provedor e pagina resultados.
- [x] `max_results` aplica default e teto documentados.
- [x] Erros do provedor são propagados sem fabricar resultados.
- [x] DuckDuckGo extrai título, URL real e snippet do HTML.
- [x] Output é marcado como estruturado para LLM e jobs.

Evidências: `internal/tools/web/web_search_test.go`,
`internal/tools/web/brave_provider_test.go`,
`internal/tools/web/tavily_provider_test.go`,
`internal/tools/web/bing_provider_test.go`,
`internal/tools/web/provider_selection_test.go` (parâmetro `provider`,
cadeia a partir do pedido, aviso DDG),
`internal/tools/http/client_test.go` (auth manual),
`internal/tools/catalog_test.go` e registro em
`internal/app/app_tool_registry.go`. Demais provedores com API, cache e
seleção por preferência do usuário permanecem fora do escopo (Brave,
Tavily e Bing implementadas como provedoras com API key, mantendo o
contrato v1).

## Arquivos

- `internal/tools/web/web_search.go` — `WebSearch` (`tools.Tool`), `SearchProvider`,
  `SearchResult`, `webSearchJSONOutput`, `duckDuckGoProvider` e cadeia
  `searchWithFallback` (Brave → Tavily → Bing → DuckDuckGo).
- `internal/tools/web/brave_provider.go` — `braveProvider` (Brave Search API
  via credmanager) e `parseBraveResponse`.
- `internal/tools/web/tavily_provider.go` — `tavilyProvider` (Tavily Search
  API via credmanager) e `parseTavilyResponse`.
- `internal/tools/web/bing_provider.go` — `bingProvider` (Bing Web Search
  API v7 via credmanager) e `parseBingResponse`.
- `internal/tools/web/web_search_test.go` — testes (mock provider, JSON, paginação).
- `internal/tools/web/brave_provider_test.go` — testes do Brave (token,
  parse, fallback, erro operacional) com HTTP fake, sem rede externa.
- `internal/tools/web/tavily_provider_test.go` — testes da Tavily (token,
  parse, janela de offset, cadeia) com HTTP fake, sem rede externa.
- `internal/tools/web/bing_provider_test.go` — testes do Bing (token,
  parse, redundância de auth, cadeia) com HTTP fake, sem rede externa.
- `internal/tools/http/trust.go` + `client.go` — `WithManualAuth` (auth
  manual única, interceptor pula a resolução).
- `internal/app/app_tool_registry.go` — registro da tool.
- `internal/tools/catalog.go` — metadado (`Category: web`, `Risk: network`).

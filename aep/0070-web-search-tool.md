# AEP-0070 — Tool `web_search` (busca web → JSON canônico paginável)

Status: Done — saída canônica paginável implementada em `internal/tools/web/web_search.go` e testes
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
escolhida. O provedor padrão é o **DuckDuckGo** (HTML, sem API key), atrás de uma
interface `SearchProvider` plugável.

Escopo atual é deliberadamente mínimo: **sem persistência, sem cache, sem ranking
próprio, sem API keys** — um único provedor de fallback universal.

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
- **D3 — `has_more` heurístico.** Por ser **scraping** (DuckDuckGo HTML), não há
  total de resultados exposto. `has_more` é estimado pela página ter vindo "cheia"
  (`count >= max_results`). É documentado como heurística; o contrato
  (offset/count/has_more) é independente de provedor, então trocar o provedor por
  uma API oficial no futuro não quebra consumidores.
- **D4 — Reuso da pilha HTTP + credmanager.** `web_search` constrói um
  `httpclient.Client` via `httpclient.New(...)`, igual a `web_fetch`/`http_request`/
  `feed_read`, herdando timeout/retry e o interceptor de auth (AEP-0018/0019).
- **D5 — Provedor plugável.** Interface `SearchProvider`
  (`Search(ctx, client, query, offset, maxResults) ([]SearchResult, error)` + `Name()`).
  O default é `duckDuckGoProvider`; `NewWebSearchWithProvider` permite injetar outros
  (Google/Bing/Brave) ou um mock em testes.
- **D6 — Descoberta separada da leitura.** `web_search` não baixa o conteúdo das
  páginas — só retorna links/trechos. Ler conteúdo é responsabilidade de `web_fetch`
  (que aplica a barreira anti-SSRF ao buscar a URL escolhida). Assim a busca não
  precisa validar host: ela só consulta o endpoint fixo do provedor.
- **D7 — Parâmetros enxutos.** `query` (obrigatório), `max_results` (default 8, máx
  20 por página), `offset` (default 0). `Risk: network` no catálogo.

## Realidade do código (estado atual)

- Saída JSON: `webSearchJSONOutput{Query, Provider, Offset, Count, HasMore, Results}`.
- `SearchResult{Title, URL, Snippet}` (tags JSON).
- Provedor DuckDuckGo: GET em `https://html.duckduckgo.com/html/?q=...`, com `&s=offset`
  quando `offset > 0`; parse do HTML lite (`result__a`/`result__snippet`) e extração
  da URL real do redirect (`uddg=`).
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

- **Provedores com API key** (Google CSE, Bing, Brave): ranking melhor, paginação
  confiável e **total de resultados real** (substituindo o `has_more` heurístico por
  um valor exato/`total`). A interface `SearchProvider` já comporta isso.
- **Seleção/config de provedor** por credencial/preferência do usuário, com fallback
  automático para o DuckDuckGo quando não houver API key.
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

- **Brave Search API** — resultados de qualidade, paginação e contagem confiáveis;
  requer API key.
- **Google (Programmable Search / Custom Search JSON API)** — API oficial com
  `cx` + API key; cota limitada, mas estável e com `total` real.
- **Bing Web Search API** (Azure) — API key; resultados ricos (web/news/images).
- Outras APIs especializadas (ex.: Tavily/SerpAPI-like) podem entrar pela mesma
  interface.

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
- **Google via scraping** e **Bing via scraping** — extraem resultados da página de
  busca pública.
- Outros buscadores conforme necessidade.

Riscos/cuidados a documentar e tratar nesses provedores: mudança de layout (parsing
quebra), bloqueio/captcha e rate limiting, e conformidade com os Termos de Uso de
cada buscador. Por isso scraping fica como camada de fallback, atrás das APIs
oficiais quando disponíveis, com métricas de "parsing vazio" para detectar quebras.

### Seleção e fallback

- Provedor escolhido por preferência do usuário e/ou presença de credencial
  (ex.: se há `brave_api_key`, usa Brave; senão tenta Google API; senão cai para
  scraping DuckDuckGo).
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
`internal/tools/catalog_test.go` e registro em
`internal/app/app_tool_registry.go`. Provedores com API, cache e seleção
automática permanecem fora do escopo da v1.

## Arquivos

- `internal/tools/web/web_search.go` — `WebSearch` (`tools.Tool`), `SearchProvider`,
  `SearchResult`, `webSearchJSONOutput` e `duckDuckGoProvider`.
- `internal/tools/web/web_search_test.go` — testes (mock provider, JSON, paginação).
- `internal/app/app_tool_registry.go` — registro da tool.
- `internal/tools/catalog.go` — metadado (`Category: web`, `Risk: network`).

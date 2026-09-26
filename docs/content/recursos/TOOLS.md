---
title: "Ferramentas"
weight: 12
---

# Ferramentas

O assistente expõe 15 famílias de tools ao LLM: filesystem, shell, web, feed, http, memory, tasklist, mcpserver, history, job, questionnaire, skillloader, subagent e outras. Cada tool tem baseline operacional por perfil e é auditável no histórico do turno.

## Descoberta e carregamento

Tools fora do baseline do perfil ficam disponíveis pelo `tool_catalog`. A busca
aceita uma descrição da tarefa e filtros de origem, categoria, classe, pacote,
risco e disponibilidade. Os resultados priorizam relevância, pacotes
preferenciais do perfil e tools usadas recentemente na mesma conversa.

No primeiro turno, o assistente pode antecipar uma busca pequena e carregar até
três tools de leitura relevantes. Esse mecanismo nunca carrega automaticamente
tools de escrita, shell, rede ou operações destrutivas, nem habilita tools
desativadas ou opt-in.

O carregamento aceita nomes exatos e seletores como `mcp/atlassian/*` e
`package/history/*`. Cada seletor é limitado a 20 tools e continua sujeito à
política do perfil, disponibilidade, allowlists, confirmações e orçamento de
schemas. Para operações sensíveis, o carregamento apenas disponibiliza a
capacidade; ele não aprova sua execução.

## Acompanhar ferramentas no chat

Cada execução aparece diretamente na posição cronológica como um card próprio;
não é necessário expandir um agrupamento da rodada. O card apresenta seu estado
por texto, além do ícone: em execução, concluída, falhou ou cancelada. Enquanto
a execução não terminou, a saída disponível é identificada como parcial.
Ferramentas nativas usam descrições localizadas; integrações MCP usam o nome
público do provedor, sem exigir adaptações no servidor.

Os anúncios do leitor de telas usam a mesma descrição amigável e o mesmo estado
do card. Nomes internos de tools e caminhos absolutos ficam restritos aos
detalhes técnicos; por exemplo, o anúncio principal pode dizer `Lendo arquivo:
config.ts. Em execução` ou `Consultando Atlassian. Concluída`.

Use **Ver detalhes técnicos**, por teclado ou pelo contexto da ferramenta,
para consultar os parâmetros sanitizados e a resposta. O modal acompanha as
atualizações da chamada enquanto estiver aberto, incluindo a passagem para o
resultado persistido ao terminar. Escape fecha o modal e devolve o foco ao
controle de origem.

Buscas nativas compatíveis oferecem **Ver resultados**. O modal apresenta
20 itens por página; arquivos abrem no editor e sites no navegador externo.
O modal preserva no máximo 100 itens para exibição. Quando a lista estiver
limitada, o aviso informa a contagem apresentada e orienta restringir a busca;
ela não deve ser interpretada como o conjunto completo de resultados.

Os indicadores de autorização refletem decisões explícitas registradas pelo
host, não inferências sobre o texto da ferramenta. Se uma operação tiver
aprovação em uma etapa e bloqueio em outra, o bloqueio prevalece no resumo.
Decisões já registradas permanecem disponíveis mesmo que a execução seja
cancelada ou exceda o tempo limite. A ausência de indicador não significa
aprovação; o estado da execução continua sendo uma informação separada.

## Resultados grandes

Resultados model-facing não recebem frases de truncamento dentro do conteúdo.
Quando há mais dados, um envelope estruturado informa `has_more`, o intervalo
devolvido e, quando há continuação determinística, o próximo offset ou um
`result_id`. Buscas e listagens podem apenas anunciar `has_more`: nesse caso,
refaça a consulta com escopo ou limite mais restrito. A tool `read_tool_result`
relê por bytes resultados efêmeros preservados pelo host, como outputs extensos
de comandos, páginas web e tools MCP via bridge. O identificador só pode ser
usado pelo mesmo usuário e, quando aplicável, na mesma conversa; ele expira com
reinício ou pressão do armazenamento.

Se a janela de contexto reduzir uma página que já tinha continuação própria, o
envelope mantém esse cursor de origem em `source_window`: conclua primeiro a
leitura por bytes e depois retome a consulta original conforme esse campo.

`read_file` devolve no máximo 2.000 linhas e 50 KiB por chamada, pelo limite
atingido primeiro. Continue com o `next_offset` informado. Com `raw:true`, a
tool devolve somente o texto exato do trecho pedido, sem cabeçalho, números de
linha ou envelope; se o trecho não couber, a chamada falha e deve ser repetida
com `offset`/`limit` menor.

JSON canônico e qualquer resultado `raw` nunca são cortados silenciosamente:
cabem integralmente no limite ou produzem erro explícito. MCP nativo é executado
no provedor e, por isso, não passa pela proteção local; nesse modo aplicam-se os
limites do próprio provedor.

### Respostas HTTP grandes

`http_request` mantém o comportamento atual por padrão (`auto`, `text`, `json`
e `raw`). Para uma API que devolve um JSON grande, use `extract_mode: "file"`:

```json
{
  "url": "https://api.example.com/runs/15f4c620-b2af-11f1-861b-f0d192625af9",
  "method": "GET",
  "extract_mode": "file",
  "output_path": "workflows-run.json"
}
```

O corpo é baixado em streaming para a pasta de artefatos HTTP do workspace,
com limite de segurança de 10 MiB. O modelo recebe somente status, tipo,
tamanho, SHA-256, headers de resposta não sensíveis e o caminho seguro do
arquivo; `Set-Cookie`, `Authorization` e outros headers de credencial nunca
são retornados. Headers permitidos acima de 128 bytes são omitidos.
`output_path` aceita um nome simples ou caminho absoluto diretamente dentro
da pasta controlada; caminhos externos, subpastas e arquivos existentes são
rejeitados. Se omitido, é gerado um nome único. O download não é limitado por
`max_response_size`: acima de 10 MiB ele falha e remove o arquivo parcial.
Artefatos expiram após 30 minutos e são removidos no encerramento do app;
somente arquivos criados pela instância são removidos. Um encerramento abrupto
do processo pode deixar arquivos para remoção manual.

Para extrair somente campos de um JSON grande, use o seletor restrito de
`jsonpath`. Ele não executa jq, scripts ou comandos e suporta campos por ponto
e descendência recursiva no primeiro segmento (expressões de até 2048 bytes):

```json
{
  "url": "https://api.example.com/runs/15f4c620-b2af-11f1-861b-f0d192625af9",
  "extract_mode": "jsonpath",
  "jsonpath": "$..metadata.name",
  "max_response_size": 4096
}
```

O limite é aplicado ao resultado extraído, não ao documento original (que
continua sujeito ao teto de download de 10 MiB e é parseado na memória do executor). JSON
inválido, seletor inválido e resultado extraído grande produzem erros explícitos.
Para processamentos adicionais, leia o `path` retornado no modo `file` com
`read_file` ou use `run_command` com uma ferramenta local apropriada.

Por exemplo, substitua o caminho abaixo pelo `path` retornado e execute localmente:

```sh
jq -r '.. | objects | select((.metadata.name? // "") | test("deploy-to-prod")) | .metadata.name' /caminho/retornado/workflows-run.json
```

## MCP nos perfis padrão

Os perfis **Padrão** e **Programação** deixam todas as tools MCP disponíveis
sob demanda. Elas não entram no payload inicial: o agente as descobre e carrega
quando necessário. A regra cobre automaticamente tools de servidores MCP
conectados no futuro, sem exigir edição manual do perfil.

Disponibilidade sob demanda não concede aprovação de execução. Allowlists,
classificação de risco, confiança de rede e confirmações continuam sendo
aplicadas normalmente. Tools opt-in também permanecem bloqueadas até uma
autorização explícita.

Quando um skill declara `allowed-tools`/`tools` (allowlist) ou uma denylist, esse
escopo passa a valer também para o que é **anunciado** ao modelo, e não só para a
execução: tools fora da allowlist (quando ela existe) ou dentro da denylist não
são oferecidas ao modelo enquanto aquele skill estiver ativo. Assim o modelo não
tenta usar uma tool que o skill bloqueia — antes ela era oferecida e só rejeitada
na hora de executar. O bloqueio de execução permanece como salvaguarda adicional.

Esse mesmo escopo também alinha as instruções do system prompt às tools
disponíveis: se o skill remove o `tool_catalog` (ou todas as tools iniciais), o
protocolo de seleção catalog-first deixa de instruir o uso do catálogo, evitando
que o prompt peça uma tool que já não está mais disponível no turno.

### Conjunto base protegido

A allowlist de um skill restringe apenas as tools de **domínio**. Ela nunca
remove, por si só, o **conjunto base protegido** — as capacidades de
control-plane (`tool_catalog`, `load_skill`) e a base de runtime do agente
(`memory`, `task`, `task_list`, `task_note`, `update_plan`, `read_tool_result`).
Sem essa proteção, um skill com allowlist focada no próprio domínio deixaria o
agente sem descobrir/carregar tools, registrar memória, planejar tarefas ou
reler resultados truncados durante o restante do turno.

A proteção vale só contra essa remoção implícita: se o skill **negar** uma
dessas tools explicitamente (denylist), ou se o **perfil** a marcar como
desativada, ela continua indisponível — base ou não. O bloqueio na exposição e
o bloqueio na execução usam exatamente a mesma lista, então o que o prompt
anuncia e o que o agente pode executar permanecem coerentes.

## Sub-agentes

A tool `subagent` delega trabalho especializado, paralelizável, longo ou que se
beneficie de contexto isolado. Para tarefas curtas que uma tool direta resolve,
delegar adiciona latência, consumo de contexto e ocupa uma das vagas limitadas
de concorrência.

Por padrão, o modo síncrono espera e retorna um envelope JSON com status e IDs,
preservando a compatibilidade das chamadas existentes. Em envios síncronos,
`raw:true` devolve diretamente a resposta integral do sub-agente como conteúdo
da tool; os IDs continuam disponíveis nos metadados. Se a resposta não couber
no limite geral, a chamada falha sem devolver conteúdo parcial.

`background:true` retorna os IDs imediatamente, mantém a execução em segundo
plano e entrega o resultado posteriormente à conversa pai. A combinação
`raw:true` com background é rejeitada, pois o modo assíncrono precisa preservar
o handle e o contrato de entrega. `raw` também não se aplica a consultas de
status nem a cancelamentos.

## Busca na web

A tool `web_search` descobre fontes na web e devolve links com título e trecho
em JSON (`query`, `provider`, `offset`, `count`, `has_more`, `results`). Para
ler o conteúdo de um resultado, use `web_fetch` na URL escolhida.

O provedor é selecionado automaticamente:

- **Brave Search API** — usada quando há chave cadastrada no gerenciador de
  credenciais para o domínio `api.search.brave.com` (tipo bearer com o token,
  ou tipo custom com o header `X-Subscription-Token`). Oferece ranking,
  paginação e contagem confiáveis.
- **DuckDuckGo (HTML)** — fallback universal sem chave, usado quando não há
  credencial Brave, a API responde 401/403/429 (auth/quota) ou o `offset`
  pedido está além da janela da Brave (422).

O campo `provider` na resposta identifica qual backend respondeu. Demais erros
da API Brave são propagados sem fabricar resultados.

## Histórico

As tools de histórico permitem localizar e recuperar contexto de conversas anteriores:

- `search_conversations` pesquisa mensagens e retorna trechos com seus IDs;
- `get_conversation_info` consulta os metadados e o resumo de uma conversa;
- `get_messages` recupera o conteúdo textual integral de até 20 mensagens pelos IDs. O parâmetro opcional `include_tool_results` inclui também os resultados de tools dos mesmos turnos.

`get_messages` respeita a conta autenticada: mensagens de outro usuário não são retornadas. Áudio e mídias binárias em base64 são omitidos para evitar payloads excessivos; `content`, `tool_calls` e `tool_call_id` permanecem disponíveis.

### Busca no histórico

A tool `search_conversations` faz busca textual nas mensagens do usuário. O
parâmetro `query` é obrigatório e aceita palavras, frases exatas entre aspas,
prefixos com `*` e os operadores `OR`, `AND` e `NOT`.

Por padrão, a busca é global: omitir `conversation_id` pesquisa todas as
conversas do usuário, preservando o comportamento das chamadas existentes.
Para limitar os resultados, informe o ID de uma conversa em
`conversation_id`. Dentro de um chat, também é possível usar o valor especial
`current`; nesse caso, a tool obtém com segurança a conversa corrente do
contexto da chamada. Se não houver uma conversa corrente disponível, a chamada
é rejeitada em vez de executar uma busca global.

Exemplos:

- `{"query": "autenticação JWT"}` — busca global.
- `{"query": "decisão final", "conversation_id": "019...", "limit": 10}` —
  busca apenas na conversa informada.
- `{"query": "próximos passos", "conversation_id": "current"}` — busca apenas
  na conversa em andamento.

Em todos os casos, os resultados permanecem restritos à conta autenticada.

# AEP-0084 — Agentes de código ACP como providers LLM

**Status:** ✅ Done

## Resumo

Agentes de codificação que falam **ACP (Agent Client Protocol)** — Cursor CLI
(`agent acp`), OpenCode (`opencode acp`), GitHub Copilot CLI (`copilot --acp`) e
Claude Code (via adaptador) — entram no app como **providers LLM comuns**,
selecionáveis por perfil, conversando na superfície de chat como qualquer outro
provider. Um novo `APIFormat = "acp"` no barramento, um client
JSON-RPC sobre stdio em `internal/acp`, e o pipeline de streaming existente
(`StreamHandler` → eventos `chat:*`) permanece intacto.

O que muda em relação a um provider HTTP: o "modelo" é um agente com ferramentas
próprias, que edita arquivos e roda comandos na máquina; o transporte é um
subprocesso; a conversa tem estado do lado do agente; e o agente **pergunta** —
pede permissão para agir e faz perguntas de múltipla escolha no meio do turno.

## Motivação

O barramento multi-provedor já permite conversar com qualquer LLM. Falta poder
conversar com um **agente de código**: quem já tem Cursor ou Claude Code
instalados e autenticados tem, na prática, um assistente com contexto de
repositório, ferramentas de edição e MCP do projeto — e hoje precisa sair do app
para usá-lo, perdendo tudo o que o app oferece de acessibilidade (navegação por
teclado, TTS, announcer, arbitragem de voz).

Colocar esses agentes no barramento significa: escolher um perfil, digitar, e
ouvir a resposta pelo mesmo caminho de sempre.

## O que este AEP não faz

- **Não é uma tool agêntica.** O agente ACP não entra pelo padrão da tool
  `subagent` (AEP-0068) nem por qualquer outra tool.
- **Não é uma superfície nova.** Nada de aba especial de "agente de código": é a
  mesma superfície de chat, o mesmo `SendMessage`, os mesmos eventos (AEP-0040).
- **Não exporta as tools do app para o agente.** O Cursor usa as ferramentas
  dele (busca, edição, shell, MCP do `.cursor/mcp.json`). O app não anuncia
  capacidades de filesystem nem de terminal no `initialize` e o `ToolPlanner`
  (AEP-0077) não planeja tools para um provider ACP.
- **Não leva o prompt do app para o agente.** Persona, skills, memória e blocos
  de contexto ficam de fora do turno: o agente tem recurso próprio para tudo
  isso, e o app não tem como fazer melhor de fora (D4, revisto na Fase 8).
- **Não gerencia credenciais do agente.** A autenticação é local ao CLI.

## Descobertas empíricas

Sonda executada em 2026-07-31 contra o Cursor CLI `2026.07.23-e383d2b` no
Windows, a partir do diretório do projeto (script em [Apêndice: sonda](#apêndice-sonda-de-verificação)).
Ela resolve a dúvida que bloqueava o desenho — se a troca de modelo é real ou
degradada:

**`initialize`** devolve `authMethods: [cursor_login]` e
`agentCapabilities: { loadSession: true, mcpCapabilities: { http, sse },
promptCapabilities: { image: true, audio: false, embeddedContext: false },
sessionCapabilities: { list } }`.

**`session/new { cwd, mcpServers }`** devolve o `sessionId` e — apesar de a
documentação do Cursor mostrar só o `sessionId` — **os dois formatos de seleção
de modelo ao mesmo tempo**:

- `configOptions[]` (formato estável do ACP desde fevereiro de 2026) com uma
  opção `category: "model"` contendo **32 modelos** e `currentValue`, e uma
  `category: "mode"` com `agent`/`plan`/`ask`;
- os campos legados `models` (`availableModels[].modelId`/`name`,
  `currentModelId`) e `modes`.

**Troca de modelo funciona pelos dois caminhos**: `session/set_model
{ sessionId, modelId }` responde `{}`, e `session/set_config_option
{ sessionId, configId: "model", value }` responde com o **estado completo** de
todas as opções. `session/set_mode { sessionId, modeId }` também funciona.

**Um turno real** (`session/prompt` pedindo para listar arquivos) produziu estas
notificações `session/update`: `agent_thought_chunk` (22), `agent_message_chunk`
(9), `tool_call` (3), `tool_call_update` (6), `current_mode_update`,
`session_info_update` (com um `title` gerado — "List Markdown Files") e
`available_commands_update` (os slash commands do usuário). O turno terminou com
`stopReason: end_turn`.

**Permissão**: ao tentar rodar um comando, o agente enviou
`session/request_permission` com o `toolCall` (título com o comando literal,
`kind: "execute"`, e o motivo — "Not in allowlist: Get-ChildItem,
Select-Object") e três opções: `allow-once`, `allow-always`, `reject-once`. A
documentação é explícita: **sem resposta, a execução trava**.

Dois detalhes de implementação que só aparecem na prática:

- **No Windows o `agent` é um wrapper PowerShell** (`agent.ps1` em
  `%LOCALAPPDATA%\cursor-agent`) que executa `node.exe index.js` de um diretório
  versionado. Não existe `agent.exe` para spawnar diretamente.
- **`toolCallId` pode conter quebra de linha** (visto:
  `"call-…-0\nfc_…"`). IDs vindos do agente precisam ser normalizados antes de
  virar chave, atributo de DOM ou texto anunciado.

### Segunda sonda: as extensões bloqueantes não apareceram

Sonda executada em 2026-08-04 contra o mesmo Cursor CLI `2026.07.23-e383d2b`,
desta vez atrás de `cursor/ask_question` e `cursor/create_plan`: três turnos, nos
modos `ask`, `plan` e `agent`, com prompts que mandavam o agente perguntar antes
de agir ("Pergunte-me qual arquivo usando sua ferramenta de pergunta ao usuário;
não adivinhe") e pedir aprovação de um plano.

**Nenhuma das duas extensões foi emitida.** Nos três turnos o agente respondeu no
próprio texto do turno e encerrou com `stopReason: end_turn` — e o raciocínio
dele diz por quê: *"Não encontrei uma ferramenta dedicada para perguntar ao
usuário"*, *"Vou formular perguntas clarificadoras diretamente na conversa, sem
usar a ferramenta de pergunta"*. Nesta versão do CLI as duas ferramentas
aparentemente só existem quando o cliente as anuncia de algum modo que o
`initialize` do ACP não cobre, ou não existem. **O schema de
`cursor/ask_question` e `cursor/create_plan` continua sem confirmação empírica**;
o que o app implementa vem da documentação e do que já foi observado das outras
extensões.

O que a sonda capturou de novo, e que muda a implementação:

- **`cursor/task` chega sem `sessionId`.** O payload cru observado traz
  `toolCallId`, `description`, `prompt`, `subagentType`, `model`, `agentId` e
  `durationMs` — e nenhum campo de sessão, ao contrário de
  `session/request_permission`, que traz. Ou seja: **extensão `cursor/*` não
  carrega sessão por padrão**, e um handler que dependa de `params.sessionId`
  para achar a conversa dona do pedido não vai achar nada. Quem resolve isso é o
  transporte, atribuindo o pedido ao único turno em voo quando o payload não
  nomeia sessão; sem isso, o cancelamento do turno não alcançaria a extensão
  pendente e o pedido ficaria pendurado até o teto de tempo.
- O `toolCallId` com quebra de linha reapareceu aqui também (`"call-…-0\nfc_…"`),
  agora numa extensão — o saneamento não é exclusividade do `session/update`.

Uma premissa anterior também caiu: **`internal/mcp` não implementa JSON-RPC** —
ele usa o SDK oficial de MCP. O que existe de reaproveitável ali é o padrão de
subprocesso (spawn com contexto, `osutil.HideConsoleWindow`, env, backoff,
health), não o protocolo.

### Terceira sonda: os outros três agentes

Sonda executada em 2026-08-05 contra os outros três agentes que este AEP quer
atender, na mesma máquina Windows e pelo mesmo script do apêndice. Ela responde à
pergunta de que as fases seguintes dependem: quanto do desenho é do protocolo e
quanto era do Cursor.

**O Claude Code não fala ACP.** A Anthropic não adotou o protocolo; quem faz a
ponte é um adaptador npm mantido pelo ecossistema ACP, escrito sobre o Claude
Agent SDK. O pacote `@zed-industries/claude-code-acp` foi **renomeado** para
`@agentclientprotocol/claude-agent-acp` — o antigo está deprecado e avisa isso na
instalação. O binário do pacote novo é `claude-agent-acp`, com entry point em
`dist/index.js`, e ele sobe em modo ACP **sem subcomando**. Como o `.cmd` que o
npm escreve não pode ser criado como processo no Windows, o que se spawna é o par
`node.exe <caminho>\dist\index.js` — a mesma forma do Cursor, por outro motivo.

Com o adaptador 0.65.0 e o Claude Code 2.1.223, o `initialize` responde
`protocolVersion: 1` e se identifica como `@agentclientprotocol/claude-agent-acp`
("Claude Agent", 0.65.0). **`authMethods` vem vazio**: a autenticação é a do
próprio Claude Code, feita fora do protocolo pelo CLI `claude`, e não existe
método ACP de login como o `cursor_login`. As capacidades são generosas —
`loadSession: true`; `sessionCapabilities` com `additionalDirectories`, `close`,
`delete`, `fork`, `list` e `resume`; `promptCapabilities` com `image` e
`embeddedContext`; MCP por `http` e `sse`; `auth: { logout: {} }`; e dois sinais
em `_meta`, `claudeCode.promptQueueing: true` e `steering.supported: true` na
raiz.

O `session/new` dele devolve os dois formatos, como o Cursor, e traz **três**
categorias de `configOptions`: `model` (`default`, `opus[1m]`, `sonnet` e
`haiku`, com nome legível e descrição que inclui preço por Mtok); `mode`, que
aqui é de **permissão**, e não de raciocínio — `auto`, `default` (Manual),
`acceptEdits`, `plan`, `dontAsk` e `bypassPermissions`; e `thought_level`, de
identificador `effort`, categoria que o app não conhece.

**O OpenCode fala ACP nativamente**, por subcomando (`opencode acp`), como o
Cursor. O que muda é o binário: o pacote npm `opencode-ai` (1.18.14) instala um
**executável nativo** (`node_modules/opencode-ai/bin/opencode.exe`), e não o par
node+js. Ele anuncia um método de login, `opencode-login`, cuja descrição é a
instrução literal — "Run `opencode auth login` in the terminal". As capacidades
são `loadSession: true`, `sessionCapabilities` com `close`, `fork`, `list` e
`resume`, `promptCapabilities` com `image` e `embeddedContext`, e MCP por `http`
e `sse`. O `session/new` responde **só no formato novo**, com o `model` trazendo
valor e nome legível (`opencode/big-pickle` → "OpenCode Zen/Big Pickle") — que é
justamente o formato que o app prefere.

**O GitHub Copilot CLI fala ACP nativamente por flag**, e não por subcomando:
`copilot --acp` (`@github/copilot` 1.0.78, em preview público desde janeiro de
2026). O stdio é o padrão; há um `--port` para TCP que não nos interessa. No
Windows o npm instala um executável nativo por plataforma
(`@github/copilot-win32-x64/copilot.exe`) e deixa um `npm-loader.js` como entry
do pacote principal. Ele anuncia um método de login, `copilot-login` ("Run
`copilot login` in the terminal"), e o método traz `_meta["terminal-auth"]` com
`command`, `args` e `label`: **o agente diz qual comando rodar para logar**, em
vez de deixar o cliente adivinhar.

O `session/new` dele responde **só no formato legado** — `models.availableModels`
com `modelId`/`name`/`description` (`auto` → "Auto", `claude-sonnet-5` → "Claude
Sonnet 5") —, sem `configOptions`. E as capacidades de sessão são as mais magras
dos quatro: só `close` e `list`, sem `fork` nem `resume`, embora `loadSession`
venha `true`.

## Decisões

### D1. Provider, não tool: `APIFormat = "acp"`

Nova constante `APIFormatACP = "acp"` em `internal/llm/provider.go` e um `case`
em `NewChatProvider` (`chat_provider.go`) devolvendo `NewACPChatProvider`. O
provider implementa a interface `ChatProvider` inteira; `NativeMCPCapable()`
devolve `false` (o MCP do agente é do agente, configurado no `.cursor/mcp.json`
do projeto) e `WithMCPServers` é no-op.

### D2. O transporte vive em `internal/acp`, sobre um SDK de terceiros

Não existe SDK oficial de ACP em Go. Adotamos o **`coder/acp-go-sdk`** (o mais
maduro, com exemplos de bridge para Claude Code e Gemini) **atrás de uma
interface interna** em `internal/acp`:

```go
type Client interface {
    NewSession(ctx context.Context, cwd string) (Session, error)
    LoadSession(ctx context.Context, id string, cwd string) (Session, error)
    Close() error
}

type Session interface {
    ID() string
    Prompt(ctx context.Context, text string, sink UpdateSink) (StopReason, error)
    Cancel(ctx context.Context) error
    ConfigOptions() []ConfigOption
    SetConfigOption(ctx context.Context, id string, value any) ([]ConfigOption, error)
}
```

A interface precisa de uma **saída para métodos que o SDK não tipa**: as
extensões `cursor/*` não são do padrão, e o seletor legado `session/set_model`
tende a sair dos SDKs conforme o `configOptions` vira o caminho único. Sem uma
chamada crua de JSON-RPC em `internal/acp`, o app fica refém do que o SDK
resolveu tipar naquela versão.

A implementação confirmou que isso vale nas duas direções, e a de entrada é a
que morde: o cliente pronto do SDK (`ClientSideConnection`) só repassa ao app os
métodos que ele tipou e as extensões com prefixo `_`. As do Cursor são
`cursor/*` e seriam respondidas automaticamente como "método não encontrado",
**sem o app nunca ver o pedido** — inclusive as bloqueantes. Por isso o
transporte é construído sobre a conexão de baixo nível do SDK
(`acp.NewConnection` + `acp.SendRequest`), que aceita qualquer nome de método
nos dois sentidos, e não sobre o cliente pronto. Trocar por ele "para
simplificar" reintroduziria o problema.

O resto do app conversa só com essa interface. Se o SDK estagnar ou divergir,
troca-se a implementação sem tocar no provider. A versão fica pinada e o
comportamento é coberto por testes contra um **agente ACP falso** (um processo
de teste que fala o protocolo), não contra o Cursor real.

### D3. Um processo por provider, sessões multiplexadas

O `agent acp` aceita várias sessões na mesma conexão. Mantemos **um processo por
provider ACP configurado**, com as sessões multiplexadas por `sessionId`. Ele é
iniciado sob demanda **no primeiro uso, seja qual for** — um turno de chat, uma
consulta de modelos na tela de settings ou um health check — e encerrado no
shutdown do app. Quem precisa do agente pede a conexão ao gerenciador; nunca
spawna por conta própria. Isso evita pagar o custo de spawn e autenticação mais
de uma vez e impede processos duplicados do mesmo provider.

Morte do processo é tratada como o MCP trata: reconecta com backoff e marca as
sessões como perdidas.

O dono desse ciclo de vida é um **serviço de longa duração**, no molde do
`mcp.Manager` — não o objeto `ChatProvider`. `GetChatProvider` constrói uma
instância nova a cada chamada; guardar processo ou sessão dentro dela daria um
`agent acp` por turno. A instância só empresta a conexão do serviço.

### D4. Sessão ACP é o estado da conversa

Cada conversa do app mapeia para uma sessão ACP (`conversationId` ↔
`sessionId`, persistido). Consequência direta, e é o ponto onde um provider ACP
mais difere de um provider HTTP: **o histórico não é reenviado**. O
`StreamChat` recebe `[]Message` como qualquer provider, mas envia ao agente
**apenas a última mensagem do usuário** — o agente mantém a própria conversa.
Reenviar o histórico duplicaria contexto e custo.

Na retomada (app reiniciado, conversa reaberta), tenta-se `session/load` — o
Cursor anuncia `loadSession: true`. Falhando, cria-se uma sessão nova e a
conversa segue com o agente sem a memória anterior; isso é informado à pessoa,
porque muda o que o agente sabe.

Limpar ou excluir a conversa encerra a sessão correspondente.

#### As instruções do perfil não vão ao agente (revisto na Fase 8)

A primeira versão desta decisão dizia o contrário, e chegou a ser implementada:
o pipeline injeta persona, skills e blocos de contexto (AEP-0075) numa única
mensagem `system`, e o provider a entregava ao agente em blocos delimitados —
`<app_instructions>` para o prefixo estável, `<app_context>` para o que muda —,
guardando por sessão o hash do que já tinha dito para não repetir a persona a
cada turno. O raciocínio era que descartá-las faria do ACP o único provider do
barramento a ignorar o perfil.

O uso mostrou o outro lado. Um agente de código chega com as regras do próprio
projeto (`AGENTS.md`, `.cursor/rules`), com memória própria e com um jeito
próprio de montar contexto a partir da árvore de arquivos — e faz isso com
acesso direto ao disco, enquanto o app mandaria texto descrevendo a mesma coisa.
Somar as nossas instruções às dele não completa o perfil: disputa espaço no
prompt de quem já resolve aquilo, e com informação de segunda mão.

A regra passa a ser a mais simples que existe: **o turno de um provider ACP leva
só a mensagem da pessoa**. Sem persona, sem skills do app, sem memória do
usuário, sem resumo da conversa, sem blocos de contexto — nem como prefixo, nem
como sufixo, nem embutidos na própria mensagem. A mensagem vai como foi escrita,
sem embrulho de contexto em volta. Anexos continuam indo, porque são conteúdo
que a pessoa mandou, não instrução do app.

O preço está aceito e é conhecido: numa conversa com agente, o assistente não
tem persona e não lembra do que a pessoa contou em outra superfície. Quem
escolhe um perfil com agente está escolhendo falar com o agente. O que o app
ainda faz por essa conversa é tudo o que não é prompt: permissões (D9),
diretório (D5), segmentação da fala (D13), avisos e histórico na tela.

#### Anexos

A mensagem do usuário pode ser multimodal (`[]ContentPart` com `image_url`), e o
`session/prompt` recebe uma lista de blocos — o mapeamento é direto: cada parte
de texto vira um bloco de texto, cada imagem vira um bloco de imagem, na ordem
original. O que o agente aceita vem do `initialize`
(`promptCapabilities.image`, `audio`, `embeddedContext`); o Cursor aceita
imagem. **Anexo que o agente não aceita nunca é descartado em silêncio**: o
turno segue com o texto e a pessoa é avisada do que ficou de fora.

#### Quando o histórico do app e a sessão do agente divergem

A sessão tem memória própria, então retry, edição e exclusão de mensagens podem
deixar as duas visões diferentes — o app mostrando uma conversa que o agente não
viveu. A regra é não tentar reconciliar em silêncio:

- **Retry de um turno que o agente nunca aceitou** (falha ao enviar o
  `session/prompt`): reenvia sem cerimônia — para o agente aquele pedido não
  existiu. Se a falha foi a **morte do processo**, a sessão morreu com ele (D3):
  o retry passa antes pelo caminho de retomada — `session/load` quando o agente
  recupera, sessão nova com aviso quando não.
- **Retry de um turno que o agente aceitou mas não concluiu** (transporte caiu
  no meio, permissão negada, cancelamento): reenviar o mesmo texto pode fazer o
  agente **refazer trabalho que já executou** — e aqui "trabalho" é arquivo
  editado e comando rodado. A sessão é mantida, porque é ela que sabe o que já
  foi feito, e o reenvio vai acompanhado da nota de que o pedido anterior pode
  ter sido interrompido e o estado deve ser conferido antes de refazer.
  Recriar a sessão seria pior: jogaria fora justamente a memória que evita a
  duplicação.
- **Retry de um turno que já produziu resposta**, edição ou exclusão de
  mensagens: a sessão é marcada como divergente e **recriada no próximo envio**,
  com aviso de que o agente perdeu o contexto anterior.

Perder memória é ruim; fazer o agente responder com base em uma conversa que a
pessoa já não vê na tela é pior.

Essas regras valem também para o **retry automático**: a recuperação de
streaming reinvoca `StreamChat` sozinha depois de um erro de transporte, e para
um provider HTTP isso é inofensivo. Para um agente de código, repetir em
silêncio um pedido que ele aceitou é repetir edição e comando. A auto-recuperação
só refaz o que o agente não chegou a aceitar; se o turno já tinha sido aceito,
ela para e devolve o controle para a pessoa, com o estado explicado, em vez de
tentar de novo por conta própria.

"Aceito" precisa ser observável, e a definição é deliberadamente conservadora:
**o turno conta como aceito assim que o `session/prompt` sai para o agente sem
falhar**. Só é "não aceito" o que falhou antes disso — processo morto, erro ao
escrever, ou rejeição imediata do JSON-RPC (sessão desconhecida, por exemplo).
Silêncio depois do envio não é prova de que nada aconteceu: a linha já está com
o agente, e ele pode estar editando. Esperar por um `session/update` para
declarar aceite seria mais preciso e menos seguro.

Hoje o laço não tem como obedecer: ele retenta pela **presença** de erro
(`LastError()` não vazio), sem noção de retentabilidade — diferente da execução
de tools, que já classifica. Então a mudança é nos dois lados: o provider ACP
marca como **não retentável** o erro de um turno já aceito, e o laço de
recuperação passa a respeitar essa marca. Não é um `if` sobre formato de
provider espalhado pelo `agent`: quem sabe se repetir é seguro é o provider, e o
laço só para de ignorar a resposta.

#### Continuar resposta

A ação de continuar uma resposta truncada existe em duas formas no app: prefill
do assistente, quando o provider suporta, e continuação por mensagem de usuário.
Um provider ACP **declara que não suporta prefill** — não há como injetar
palavras na boca do agente pelo protocolo —, então a continuação é uma mensagem
comum na mesma sessão, que o agente lê com todo o contexto do que já tinha
feito. Ela obedece à serialização do D10: continuar não fura a fila de um turno
ainda em voo.

### D5. `cwd` é o diretório do workspace ativo

O `session/new` exige um diretório: é ele que define onde o agente edita
arquivos e qual `.cursor/mcp.json` vale. Nesta fase **não há seletor por
conversa**: usa-se o **workspace ativo** do app, o mesmo que o terminal e a
allowlist de rede já seguem, com o diretório de onde o app foi iniciado como
recurso quando não há workspace.

O cwd cru do processo seria mais simples e estaria errado: o workspace ativo
muda em runtime sem mexer nele, e o agente ficaria editando arquivos de uma
árvore enquanto o terminal roda comandos em outra.

A consequência precisa estar visível, não implícita: **o agente age sobre o
workspace ativo**. A UI mostra qual é esse diretório junto ao provider, e o
primeiro turno de cada sessão o anuncia. Trocar de workspace no meio da conversa
**recria a sessão** — a anterior fala de outros arquivos —, com o mesmo aviso de
perda de memória que a recriação sempre carrega.

O seletor por conversa, que a Fase 6 entregou, é a mesma regra por conversa: a
escolha vive na conversa e vazio significa "siga o workspace ativo". Uma conversa
que escolheu diretório deixa de acompanhar a troca de workspace do app —
conversar sobre um projeto não pode passar a editar outro só porque a pessoa foi
olhar outra coisa —, e trocar a escolha recria a sessão com o mesmo aviso.

### D6. Modelos: `configOptions` com fallback para o formato legado

`GetModels` devolve os `options[]` da opção `category: "model"` quando o agente
oferece `configOptions`, e cai para `models.availableModels[]` quando não. A
troca usa `session/set_config_option` (que devolve o estado completo, incluindo
opções dependentes) com fallback para `session/set_model`.

`GetModels` é chamado fora de uma conversa (tela de settings), mas a descoberta
no ACP é acoplada à sessão. Resolvemos com uma **sessão de descoberta**: um
`session/new` sem prompt, cujas opções são lidas, **na mesma conexão do D3** (o
processo do provider, iniciado ali se ainda não estava de pé) — o processo nunca
é efêmero. O resultado vai para um **cache** por provider, invalidado quando um
`config_option_update` chega, quando a pessoa pede refresh na UI e quando o
processo cai e reconecta (D3) — a sessão que produziu aquela lista já não
existe, e o agente pode ter voltado com outra —, para a tela de settings não
bater no agente a cada render nem exibir uma lista de um processo morto.

Essa sessão **não é aberta e esquecida a cada consulta**. Ela é fechada pelo
método do protocolo quando o agente anuncia essa capacidade; como o Cursor de
hoje não anuncia, o provider mantém **uma única** sessão de descoberta por
processo e a reaproveita — ela é barata justamente por nunca receber prompt. O
que não pode acontecer é deixar um rastro de sessões abandonadas no agente.

O agente também troca de modelo sozinho (fallback de rate limit, por exemplo) e
notifica via `config_option_update`. Isso vira um evento para a UI refletir o
modelo corrente — a pessoa precisa saber com quem está falando.

### D7. As tool calls do agente são informativas, não executáveis

`tool_call` e `tool_call_update` do ACP descrevem o que o agente **já está
fazendo com as ferramentas dele**. Elas mapeiam para os eventos
`chat:tool_start` / `chat:tool_end` / `chat:tool_failure`, que a UI já sabe
renderizar e anunciar.

**Nunca** para `StreamHandler.OnToolCalls`: esse callback significa "o modelo
pediu que o app execute uma tool" e dispararia o loop agêntico do app, que
tentaria executar uma ferramenta que não é dele. Um provider ACP jamais chama
`OnToolCalls`.

Não basta o provider se comportar: **o turno de um provider ACP é planejado com
as tools desligadas**, pelo mesmo interruptor que o perfil já oferece. O
roteamento do envio escolhe o loop agêntico pela simples existência de tool
definitions no turno; um perfil com tools habilitadas levaria a conversa para o
loop errado, e junto com ele iriam a segmentação (D13) e a promessa de não
oferecer as tools do app ao agente.

O critério é o **conjunto final** que chega ao roteamento ser vazio — não apenas
o que o planner pré-carrega. Tools de runtime e expansões dinâmicas entram na
mesma conta; uma sobrevivente basta para mandar a conversa pelo caminho errado.

#### Marcadas como do agente, não do app

Reaproveitar os eventos de tool é o certo para a UI e para o announcer — a
pessoa precisa saber que algo está acontecendo —, mas eles **não podem se passar
por tools do app**. Os eventos já têm o campo `Origin` (`builtin`,
`mcp_bridge`, `mcp_native`); as do agente ganham um valor próprio,
`acp_agent`, e a UI diz de quem é a ferramenta em vez de sugerir que o
assistente agiu.

A distinção não é cosmética. Hoje há consumidor que **age** por nome de tool: o
chat inline do editor observa `chat:tool_start`/`chat:tool_end` e, ao ver
`edit_file`, `text_edit` ou `write_file`, recarrega o documento, restaura a
seleção e fecha o modal. Se uma edição feita pelo Cursor disparasse esse
caminho, o editor se comportaria como se o próprio app tivesse editado o
arquivo, dentro de um fluxo de aprovação que não aconteceu. **Consumidores que
reagem a nome de tool ignoram eventos de origem `acp_agent`**; para edições
externas o canal honesto continua sendo o observador de arquivo
(`editor:fileChanged`).

O que também **não** vale para as ferramentas do agente: a política de tools por
perfil (AEP-0081), a allowlist de comandos, o `nettrust` e os limites de saída
estruturada. Nada disso governa o Cursor — ele obedece à allowlist dele. O único
controle do app é responder ao pedido de permissão (D9).

#### Nome e argumentos

O ACP manda `kind` (`read`, `edit`, `execute`, `search`…), um `title` legível e,
quando existe, `rawInput` — que na sonda veio vazio. O `Name` do evento recebe o
`kind`, que é enumerável e traduzível; o `title` vai para o resumo, **depois de
saneado** (D11). Um `title` pode ser a linha de comando literal, e comando cru
não vira nome de ferramenta nem texto anunciado como se fosse rótulo do app.

### D8. Mapeamento do streaming

| ACP | Barramento | Observação |
|---|---|---|
| `agent_message_chunk` | `OnChunk` | texto da resposta |
| `agent_thought_chunk` | `OnThinking` / `OnThinkingDone` | raciocínio, já tratado pela UI |
| `tool_call` / `tool_call_update` | eventos `chat:tool_*` (D7) | informativo |
| `stopReason` da resposta de `session/prompt` | `OnDone` | `end_turn`, `cancelled`, `max_tokens`… |
| erro JSON-RPC ou morte do processo | `OnError` | mensagem acionável |
| `config_option_update` | evento de modelo/modo corrente | D6 |
| `session_info_update.title` | `conversation:renamed` | Fase 6 |
| `available_commands_update` | lista de comandos da sessão | slash commands do agente no menu da barra, Fase 6 |

O ACP não reporta consumo de tokens: `Usage` vai zerada e o painel de tokens
mostra "indisponível" para providers ACP, em vez de zero — zero seria mentira.
O custo é cobrado na conta do agente, fora do app.

### D9. Permissões passam pelo `questionnaire`, com allowlist por perfil

`session/request_permission` e os métodos bloqueantes do Cursor
(`cursor/ask_question`, `cursor/create_plan`) são traduzidos para o
`questionnaire` manager — o mesmo mecanismo já usado por shell, HTTP mutável e
confirmação de edição, que já é acessível por teclado e leitor de telas.

- As opções vêm do próprio pedido (`allow-once`, `allow-always`,
  `reject-once`), com os rótulos que o agente mandou.
- `allow-always` é registrado numa **allowlist por perfil**, com o mesmo padrão
  de escopo das allowlists existentes: um arquivo por perfil em
  `.assistente/acp-permissions/`, gravado de forma atômica, como a allowlist de
  rede.
- A autorização permanente vale **por classe de ação** (`execute`, `edit`,
  `read`…), e não pelo texto do pedido. O título que o agente manda é a linha de
  comando literal e muda a cada chamada: guardar por texto nunca casaria de
  novo, e casar por pedaço de comando seria pior, porque um prefixo idêntico não
  diz nada sobre o que vem depois dele. Como a classe é mais ampla do que o
  comando que está na tela, o diálogo diz isso antes de alguém escolher
  "permitir sempre" — quem autoriza precisa saber o alcance do que autoriza.
  Pedido em que o agente não declara classe cai em `other`, e autoriza só
  `other`.
- O que já foi autorizado é respondido **sem abrir diálogo**, com a permissão
  pontual do próprio pedido: quem lembra da decisão é o app, e é ele que a
  repete a cada pedido. Dizer "sempre" ao agente todas as vezes faria ele
  guardar do lado dele uma decisão que a pessoa pode revogar aqui. Um pedido que
  não ofereça nenhuma opção de permitir volta para a tela: sem ela não há como
  dizer sim no idioma do método.
- A autorização permanente **também é consultada só pelo desktop**, pelo mesmo
  motivo que só pode ser concedida ali: um canal remoto não pode colher o sim que
  alguém deu na tela. Antes da Fase 5, isso saía de graça — turno sem
  interlocutor negava na hora, antes de olhar a allowlist. Com a pergunta em
  canal, a conversa de canal passou a ter interlocutor, e a regra virou
  explícita: só a superfície de desktop consulta a allowlist, e só ela grava.
  Turno sem interlocutor nenhum continua negando na hora.
- **Conceder o "sempre" vira aviso na conversa**, dizendo a classe que passou a
  valer e onde revogá-la. A escolha muda o comportamento do app daí em diante, e
  o diálogo que a recebeu some da tela junto com a única pista disso — sem o
  aviso, a autorização existiria só num arquivo que ninguém sabe que foi
  escrito. O aviso nomeia a classe com o mesmo texto da tela de autorizações,
  para que quem for revogar reconheça a linha. Não conseguir guardar também é
  contado: a ação desta vez seguiu autorizada, mas a próxima volta a perguntar,
  e quem escolheu "sempre" precisa saber disso antes de estranhar a repetição.
- **Toda pergunta tem prazo.** Sem resposta dentro do prazo, respondemos o
  desfecho negativo **daquele método** — `reject-once` em
  `session/request_permission`, `skipped` em `cursor/ask_question`, `rejected`
  em `cursor/create_plan` — e informamos. Um turno travado para sempre é pior do
  que uma ação negada, mas o desfecho precisa ser o que o método aceita: mandar
  um `optionId` de permissão numa pergunta de múltipla escolha é erro de
  protocolo.
- **O transporte tem um teto próprio, bem maior que o prazo da pergunta.** O
  prazo acima é da camada que pergunta à pessoa; o transporte não confia que ela
  exista nem que se comporte. Como o contexto que o SDK entrega no pedido não
  traz prazo nenhum, um handler travado penduraria o agente para sempre. O teto
  é folgado de propósito — cortar antes do prazo da tela tiraria da pessoa a
  chance de responder —, e ao estourar responde o mesmo desfecho negativo do
  prazo normal.

  No pedido de permissão o transporte monta esse desfecho sozinho, porque as
  opções vêm no próprio pedido. Nas extensões, quem monta é a camada que
  implementa o método: só ela conhece o formato que cada um aceita, e uma
  resposta de forma errada corre o risco de o agente ler o "não" como decisão de
  verdade — o mesmo erro de protocolo do item anterior. Sem esse desfecho
  declarado, o transporte responde erro interno, que é a única resposta honesta
  quando não se sabe dizer "não" no idioma do método.
- **Cancelar o turno cancela a pergunta.** O ACP obriga quem manda
  `session/cancel` a responder `outcome: cancelled` aos pedidos de permissão
  ainda pendentes — e não `reject-once`, que é a resposta de prazo estourado.
  São coisas diferentes: uma é decisão de negar, a outra é a pergunta ter
  perdido o dono. Na prática isso também fecha o diálogo na tela, porque
  perguntar sobre um turno que a pessoa acabou de abortar é ruído para quem usa
  leitor de telas. Vale igual para as extensões bloqueantes do Cursor, que
  pertencem ao turno: elas recebem o erro JSON-RPC de pedido cancelado, e não
  "método não encontrado" — que faria o agente concluir que o app não suporta a
  extensão. Encerrar a conversa tem o mesmo efeito de cancelar.
- **Quem descobre a sessão da extensão é o transporte.** O pedido de permissão
  nomeia a sessão; a extensão `cursor/*` observada na sonda não nomeia nenhuma.
  Sem sessão não há conversa dona, não há perfil, não há para quem perguntar — e
  o cancelamento do turno não alcançaria o pedido pendente. O transporte
  atribui o pedido ao **único turno em voo** quando o payload não diz a qual
  sessão ele pertence; havendo mais de um, não há como adivinhar, e o pedido
  segue sem sessão para a camada acima resolvê-lo pelo desfecho negativo. Isso
  é do transporte, e não de cada método: é ele que sabe quais turnos existem.
- O título do `toolCall` contém o comando literal e é **dado não confiável**:
  passa pelo mesmo saneamento de texto de diálogo antes de virar rótulo ou
  anúncio.
- **O pedido nunca fica sem resposta de protocolo.** O `questionnaire` de hoje
  sinaliza expiração devolvendo erro para quem chamou; erro em Go não desbloqueia
  o agente, que continua esperando uma resposta JSON-RPC. A camada ACP traduz
  **qualquer** desfecho — resposta da pessoa, prazo estourado, erro interno,
  conversa excluída, app encerrando — na resposta que o método espera. É a
  diferença entre negar uma ação e pendurar o agente.

Nenhuma fase entrega o modo `agent` sem esse caminho pronto.

#### A pergunta vai para a superfície dona da conversa

O `questionnaire` de hoje é só desktop: emite um evento Wails e espera até vinte
minutos. Mas o mesmo pipeline de envio atende canais (Telegram, Signal), jobs
agendados, subagentes e a CLI. Um agente de código nessas superfícies pediria
permissão para editar arquivo e ninguém veria o diálogo — o turno ficaria
pendurado por vinte minutos.

A pergunta passa a ser **roteada pela superfície de origem da conversa**
(AEP-0042/0080), e não presumida como desktop:

- **App desktop:** o `questionnaire` de sempre.
- **Canal:** vira mensagem na própria conversa, com as opções numeradas; a
  resposta da pessoa decide. Só vale a resposta de **quem é dono do canal**, só
  enquanto houver uma pergunta pendente naquela conversa, e com **prazo curto** —
  minutos, não vinte. Expirou, é negado, e a conversa recebe o aviso de que
  expirou.
- **Sem interlocutor** (job agendado, subagente, CLI não interativa): **nega na
  hora**. Não existe "esperar" quando não há quem responda; o turno segue e a
  resposta diz o que foi negado e por quê.

Duas restrições que valem registrar. `allow-always` **só pelo desktop**:
autorizar para sempre por mensagem de texto amplia execução silenciosa futura a
partir de um canal remoto, e o ganho não paga o risco. E o texto do pedido
mostra o comando literal saneado — quem autoriza precisa ver o que está
autorizando.

O roteamento é um mecanismo **genérico**, não uma gambiarra de ACP: as
confirmações que já existem (shell, HTTP destrutivo, edição de arquivo) têm hoje
o mesmo buraco em canais e podem adotá-lo depois. Este AEP só exige o que o
provider ACP precisa.

### D10. Cancelar no app cancela o turno no agente

O cancelamento explícito do app (AEP-0064) envia `session/cancel`, que interrompe
o turno em andamento sem encerrar a sessão, e trata o `stopReason: cancelled`
como fim normal do turno. A sessão continua viva para a próxima mensagem.

Cancelamento não é só o botão: mandar uma mensagem nova numa conversa que ainda
está respondendo **cancela o turno anterior** (`StreamingManager.Register`
derruba o contexto em voo). Com um provider HTTP isso apenas descarta uma
resposta. Com um agente de código, um turno abandonado **continuaria editando
arquivos e rodando comandos** — um processo que o app já não observa mexendo no
disco. Todo caminho de cancelamento, incluindo esse, propaga `session/cancel` e
só considera o turno encerrado quando o agente confirma.

Daí decorre a serialização: **uma sessão tem no máximo um turno em voo**. O
`StreamingManager` cancela o contexto Go e já dispara o próximo `StreamChat` em
outra goroutine; do lado do agente isso sobreporia dois `session/prompt` no
mesmo `sessionId`. O novo turno espera a confirmação do cancelamento do
anterior antes de promptar.

E se a confirmação não vier no prazo? A vez **continua ocupada** — soltá-la
seria justamente permitir os dois `session/prompt` simultâneos que a fila
existe para impedir, com o agasalho falso de um prazo. Mas o turno seguinte não
fica esperando calado: ele é **recusado na hora com o motivo** ("o agente não
confirmou o cancelamento do turno"), em vez de bloquear até o contexto de quem
pediu morrer e devolver um `context deadline exceeded` que não explica nada.
Quem usa leitor de telas precisa ouvir o que está acontecendo, não um silêncio
de trinta segundos.

Essa fila mora no **serviço compartilhado do D3**, junto da sessão que ela
protege. Serializar dentro do `ChatProvider` não resolveria: `GetChatProvider`
devolve uma instância nova por chamada, e as duas goroutines do barge-in
seguram objetos diferentes — cada uma com o próprio mutex, guardando nada.

### D11. A saída do agente é dado não confiável

Mesmo padrão do AEP-0068: o texto que volta do agente passa por
`sanitizeUntrusted` na fronteira, e IDs vindos do protocolo são normalizados
(sem quebras de linha, como visto na sonda) antes de virar chave ou texto de UI.

### D12. Provider sem HTTP: o que precisa ceder

Hoje **nenhum caminho de produção** aceita um provider sem `BaseURL`:
`ProviderConfig.Validate` exige, a coluna `base_url` é `NOT NULL`,
`ProbeConnection`/`ListModels`/`TestConnection`/`CheckHealth` batem em
`GET /models`, `ExtractHostname` deriva o `CredentialPattern` da URL e o
formulário do frontend exige `http(s)`. As mudanças:

- `ProviderConfig` ganha `ACPCommand`, `ACPArgs` e `ACPEnv`; `BaseURL` deixa de
  ser obrigatório **quando** `APIFormat == acp`, e a validação passa a exigir
  `ACPCommand` nesse caso;
- as colunas novas entram por `AutoMigrate`. A coluna `base_url` **mantém** o
  `NOT NULL`: o campo é `string` (não ponteiro), então um provider ACP grava
  string vazia — que a coluna aceita. Tirar o `NOT NULL` no SQLite exigiria
  recriar a tabela inteira (criar, copiar, renomear), o procedimento mais
  arriscado do banco, para trocar vazio por nulo sem mudar comportamento
  nenhum;
- descoberta e health ramificam por formato: para ACP, "saudável" é **spawnar,
  fazer `initialize` e receber `authMethods`** — e a falta de autenticação vira
  um estado próprio, com a instrução (`agent login`), não um erro genérico;
- credenciais não se aplicam: `CredentialPattern` vazio, `AuthMode` = none.
  Chave de API mandada para um provider ACP é **recusada** em vez de ignorada:
  quem a informou espera que ela autentique algo, e aqui o login é feito no CLI
  do agente, fora do app. Esta recusa continua valendo, e o AEP-0086 D12 abre ao
  lado dela um caminho nomeado e desligado por padrão: por provedor, uma
  credencial já guardada no cofre pode ser entregue ao agente por variável de
  ambiente. O campo de credencial do provider — que autentica chamadas HTTP —
  segue recusado;
- export/import (`ProviderExport`) hoje carrega `BaseURL` como obrigatório e não
  tem onde guardar comando e argumentos. Ganha os campos novos, com o
  `MCPServerExport` como precedente — ele já exporta `Command`/`Args`. O
  **ambiente fica de fora do arquivo exportado** e só é aceito na importação,
  pela mesma razão que o `Env` do MCP: variável de ambiente de processo é onde
  token costuma parar, e arquivo de export viaja entre máquinas. Caminho
  de binário é específico da máquina: na importação, um provider ACP cujo
  comando não existe **entra com aviso**, em vez de falhar a importação inteira.
  Não existe provider desativado no app — nem coluna, nem efeito na resolução de
  perfil — e inventar esse estado só para a importação criaria um provider
  invisível que ninguém sabe reativar. O caminho é o oposto: o provider ACP é
  criado como qualquer outro, escolhendo o tipo, e o formulário da Fase 3 lista
  os agentes encontrados na máquina e deixa de pedir o que não se aplica (URL e
  chave). Corrigir um comando errado é editar o provider, como em qualquer
  outro.

### D13. Segmentar a resposta é responsabilidade do provider ACP

Hoje quem fecha segmento é o loop agêntico: `chat:segment_done` e a fala com
origem `segment` (AEP-0041) saem de `RunAgenticLoop`, quando uma iteração
termina em `finish_reason: tool_calls`. Um turno ACP não passa por lá — as tools
são do agente. Sem tratamento, um turno que alterna texto e ferramenta por dois
minutos ficaria **mudo até o fim**, e só então falaria tudo de uma vez. É
exatamente o cenário que a segmentação existe para resolver.

O provider ACP fecha os segmentos: **cada bloco de texto encerrado por atividade
de ferramenta vira um segmento**, com `chat:segment_done` e pedido de fala de
origem `segment`; o texto final do turno é o segmento final, com origem
`assistant_message`. Assim a leitura acompanha o trabalho do agente em vez de
esperar por ele.

O segmento final é leitura protegida (AEP-0058): os avisos de progresso do turno
não podem atropelá-lo.

### D14. Tarefas auxiliares não vão para o agente

Sumarização, geração de título e afins chamam `SimpleChat` no provider do
perfil. Num provider ACP, cada uma dessas chamadas seria **um turno de agente de
código** — sessão, ferramentas, permissões e custo — para produzir um resumo.

- A **sumarização automática não roda em conversa ACP**. Ela existe para caber o
  histórico na janela do modelo, e aqui o histórico não é enviado (D4): quem
  administra o contexto é o agente. Compactar do lado do app não teria efeito no
  wire e só criaria divergência.
- Um provider ACP **não é elegível** para os papéis auxiliares do perfil
  (sumarização, título). Se um perfil apontar para ele nesses papéis, o app
  recusa com explicação em vez de gastar um turno de agente.

Sem contabilidade de tokens (D8), o aviso de contexto (`chat:context_warning`)
nunca dispararia de qualquer forma; melhor declarar a limitação do que exibir
uma ocupação de 0% que mente sobre o estado real.

### D15. Spawn no Windows

O provider guarda **comando e argumentos**, não um caminho mágico. O template
builtin do Cursor detecta a instalação e, no Windows, aponta para o wrapper
(`powershell -File …\agent.ps1 acp`) ou para o par `node.exe index.js` da versão
instalada. Reusamos `osutil.HideConsoleWindow` (senão cada spawn rouba o foco) e
o `buildEnv` do MCP.

## Fases

### Fase 1 — Transporte (`internal/acp`) (feita)

Client sobre o SDK atrás da interface do D2: spawn com contexto, handshake,
sessões multiplexadas, `prompt` com sink de updates, cancelamento, morte e
reconexão do processo. Testes contra um agente ACP falso.

**Aceite:** um teste faz um turno completo contra o agente falso, incluindo
pedido de permissão respondido e cancelamento.

### Fase 2 — Provider no barramento (feita)

`APIFormatACP`, `NewACPChatProvider`, mapeamento do D8, tool events do D7,
segmentação do D13, permissões do D9 no desktop e saneamento do D11.
Persistência mínima do D12 para conseguir registrar um provider ACP. Fora do
desktop, a regra de negar na hora já vale aqui: nada pode ficar pendurado
esperando quem não existe.

Duas guardas entram junto, não depois, porque ambas estão **ligadas por padrão**
e agem sozinhas: a do D14, senão a sumarização automática dispara no fim do
primeiro turno e gasta um turno inteiro de agente de código para escrever um
resumo; e a do D4 sobre auto-recuperação, senão um erro de transporte faz o app
repetir em silêncio um pedido que o agente já aceitou — repetindo edições e
comandos na máquina.

**Aceite:** com um provider ACP configurado à mão, uma conversa no app fala com
o Cursor de ponta a ponta — texto segmentado e falado durante o turno,
raciocínio, eventos de ferramenta, pedido de permissão acessível e cancelamento
que chega ao agente. Nenhum caminho novo de envio (AEP-0040).

### Fase 3 — Provider de primeira classe na UI (feita)

Template builtin do Cursor com detecção da instalação, formulário (comando,
argumentos, diretório visível), health do D12, indicador de conexão e estado de
"não autenticado" com instrução acionável.

**Aceite:** dá para criar, testar e usar um provider Cursor sem editar
configuração na mão; o estado não autenticado é anunciado e explica o que fazer.

### Fase 4 — Modelos e modos (feita)

`GetModels` com a sessão de descoberta do D6 e cache, troca por
`set_config_option` com
fallback, `config_option_update` refletido na UI, seleção de modo
(`agent`/`plan`/`ask`) exposta junto do modelo, `Chat.Model` do perfil aplicado
na criação da sessão.

**Aceite:** trocar de modelo pelo app muda o modelo do turno seguinte; troca
feita pelo agente aparece na UI e é anunciada.

A descoberta vive em `internal/acp/discovery.go`: uma sessão sem prompt na mesma
conexão de todo mundo, aberta na primeira consulta e reaproveitada até o processo
morrer. Ela é fechada pelo método do protocolo quando o agente anuncia essa
capacidade — o Cursor de hoje não anuncia, e aí a sessão vive até o fim do
processo em vez de virar rastro de sessões abandonadas a cada consulta.

Decisões que a fase fixou:

- **A invalidação do cache é sem trava.** A geração do cache é um contador
  atômico porque quem invalida é a goroutine que entrega as notificações do
  agente: se ela precisasse do mesmo mutex que a abertura da sessão de descoberta
  segura, um `config_option_update` chegando durante a abertura pararia o
  protocolo — e a abertura espera resposta pelo protocolo.
- **Recarregar a lista é ato explícito da pessoa.** A abertura da tela lista o que
  já se sabia; só o botão de recarregar (e a nova tentativa depois de um erro)
  descarta o que o provedor guardou. Invalidar a cada render faria a tela de
  perfil abrir uma descoberta no agente sem ninguém ter pedido.
- **O recarregar atravessa as camadas por capacidade, não por tipo.** Provedor que
  não guarda lista é listado normalmente; o embrulho de limite de uso repassa a
  capacidade, senão ele a esconderia e a lista velha continuaria aparecendo,
  correta o suficiente para ninguém desconfiar.
- **Troca por `session/set_config_option`, com o seletor anterior como
  alternativa.** O erro de "método não encontrado" é o único que faz cair para
  `session/set_model` e `session/set_mode`; qualquer outro erro é do agente e
  sobe. Quando a alternativa não devolve o estado, o app confirma localmente o
  valor pedido — o agente aceitou, e mostrar o valor antigo seria mentir.
- **O agente trocando de modelo sozinho é evento de conversa, não do turno.** O
  aviso chega por `config_option_update` inclusive entre turnos, então ele vira
  `chat:agent_options`, com o `conversationId` sempre presente (AEP-0040). O
  evento só pede anúncio quando algo mudou de fato: o agente também repete o
  estado, e anunciar cada repetição atropelaria a leitura da resposta em curso.
- **O seletor da barra de ferramentas só aparece quando há o que escolher.**
  Consultar as opções não sobe processo nem abre sessão: conversa que ainda não
  falou com o agente não tem estado a mostrar, e pagar um processo porque uma
  barra renderizou seria caro e invisível.
- **O modelo do perfil é aplicado no começo do turno**, junto da sessão. Quando o
  agente não oferece o modelo escolhido, ou quando a troca não vale, o turno segue
  e a conversa recebe um aviso dizendo que modelo atendeu — descobrir isso pela
  resposta estranha é pior do que ouvir a troca.

### Fase 5 — Perguntar fora do desktop (feita)

Roteamento da pergunta pela superfície de origem (D9): pedido vira mensagem no
canal, com opções numeradas, resposta restrita ao dono do canal, prazo curto e
`allow-always` barrado fora do desktop. Mecanismo genérico, aproveitável pelas
confirmações que já existem.

**Aceite:** uma conversa de canal com perfil ACP consegue autorizar e negar uma
ação pela própria conversa; sem resposta no prazo, o pedido é negado e a pessoa
é informada; resposta de quem não é dono do canal é ignorada.

O mecanismo vive no `questionnaire`, e não no ACP: `Surface` diz de onde a
conversa veio e `Router` leva o `RequestPayload` para lá, devolvendo a mesma
`Response` do diálogo de tela. Quem pergunta não sabe onde a pessoa está — é o
que torna o mecanismo aproveitável por qualquer diálogo do backend.

Do lado do canal, `messaging.ChannelQuestions` traduz o diálogo em mensagem
(título, o bloco do que foi pedido, opções numeradas e o prazo) e lê a resposta
em `handleIncoming`, o mesmo ponto por onde toda mensagem de canal entra
(AEP-0040) — a mensagem que decide não vira turno, senão o barge-in cancelaria
justamente o turno que espera a decisão. A superfície aceita um diálogo de uma
única decisão (escolha ou sim/não); o que não couber num número é recusado com o
desfecho negativo na hora, em vez de virar mensagem que ninguém sabe responder.

Os três diálogos bloqueantes do D9 usam o mesmo roteamento:
`session/request_permission`, `cursor/ask_question` e `cursor/create_plan`. Antes
desta fase os dois últimos desistiam na hora quando o turno não tinha tela, e
conversa de canal caía nesse caso; agora a pergunta e o plano vão para o canal
pelo mesmo `Router`, e o que volta ao agente continua sendo o desfecho que o
método dele entende — pergunta pulada, plano recusado. O motivo que o agente
repete e o aviso que fica na conversa saem de uma classificação única de "acabou
sem decisão", partilhada pelos três: turno cancelado não vira aviso, pergunta que
nunca apareceu não é contada como prazo estourado.

Decisões que a fase fixou:

- **Prazo de 3 minutos** no canal, contra os 20 do desktop: na tela a pessoa está
  diante do diálogo; numa mensagem, o turno do agente fica parado. O prazo cabe
  com folga no teto do transporte, que nunca corta antes da chance de responder.
- **Só o contato dono da conversa decide.** A pergunta guarda de quem ela é, e a
  mensagem de outra pessoa é ignorada por decisão — a pergunta continua pendente,
  e nada é respondido a quem não é dono, porque a própria existência do pedido já
  contaria a um terceiro o que o agente está tentando fazer.
- **`allow-always` barrado em duas camadas.** A opção não é oferecida fora do
  desktop, e o caminho que grava na allowlist recusa quando a decisão não veio da
  tela. Um pedido que só ofereça o "sempre" é negado sem virar mensagem: sem como
  dizer sim, a pergunta custaria uma espera por uma decisão já tomada.
- **O bloco do que foi pedido vai para o canal, saneado e com teto.** Autorizar
  sem ver o que se autoriza seria pior do que o registro ficar no histórico de um
  app de terceiro; o corte avisa que o texto não veio inteiro e onde ele está
  completo.
- **A superfície de origem é resolvida no início do turno**, e não no pedido de
  permissão: o pedido chega por outra goroutine, num contexto de transporte sem
  escopo de usuário, e descobrir de qual canal a conversa veio exige saber de quem
  ela é (AEP-0052). Por isso `TurnOwner` carrega o dono do turno.

Fora do escopo desta fase: as confirmações de shell, HTTP mutável e edição de
arquivo continuam só na tela. Elas já usam o mesmo `RequestPayload`, mas o
diálogo delas nasce na camada de tools, que hoje não sabe de que conversa o turno
é — levar a superfície até lá é trabalho próprio, não um detalhe desta fase.

### Fase 6 — Continuidade e conveniências (feita)

`session/load` na reabertura, título vindo de `session_info_update`, slash
commands de `available_commands_update`, seletor de diretório por conversa.

**Aceite:** reabrir uma conversa volta a falar com a mesma sessão do agente
quando ele sabe retomá-la, e conta que o contexto se perdeu quando não sabe; o
nome que o agente dá à sessão vira o título da conversa sem apagar nome
escolhido por alguém; os comandos que o agente anuncia aparecem no menu da barra
junto das skills; e o diretório em que o agente trabalha é visível e trocável
por conversa.

Decisões que a fase fixou:

- **A retomada é tentada sempre, e a perda é contada uma vez.** `session/load` já
  era chamado na montagem; o que faltava era dizer que ele falhou. A sessão que
  nasce recriada carrega um aviso que o primeiro turno consome
  (`TurnNoticeAgentMemoryLost`): repeti-lo a cada turno diria que o agente
  esqueceu de novo, e omiti-lo faria a pessoa descobrir pela resposta estranha.
  O aviso é consumido no turno que o entrega, e não na montagem — turno que nem
  chegou ao agente não contou nada a ninguém.
- **Sessão retomada é registrada como a que nasce agora.** O registro é o que
  permite anotar a intenção de uma troca de modelo antes de pedi-la ao agente
  (Fase 4); sem ele, o eco da troca que a pessoa pediu voltaria anunciado como
  decisão do agente. O sintoma só apareceria no anúncio, então o caso tem teste
  próprio.
- **O título do agente só substitui título automático.** O nome gerado troca o
  padrão de conversa nova e o recorte da primeira mensagem, e mais nada: nome
  diferente desses dois foi escolhido por alguém, na tela, e sobrescrevê-lo
  trocaria uma decisão por um palpite. O título chega ao fim do turno, e não
  durante: renomear é escrita no banco, e o sink das atualizações roda na
  goroutine de entrega do transporte — enquanto ela não volta, o protocolo do
  agente fica parado.
- **Comandos do agente entram no menu que já existe.** A barra abre uma lista só,
  com as skills do app e os comandos do agente agrupados e rotulados: dois menus
  de barra obrigariam a pessoa a saber de antemão de quem é o comando que ela
  quer. Comando com espaço no nome é descartado no mapeamento, porque o menu
  separa nome e argumento pelo espaço.
- **O diretório é por conversa, visível e resolvido no backend.** Ele é o alcance
  do que a pessoa autorizou o agente a ler e editar, e por isso fica na barra em
  vez de escondido em configuração. A escolha vive na conversa
  (`Conversation.AgentWorkDir`), e vazio significa "siga o workspace ativo", que
  segue sendo o padrão. O caminho é conferido na hora da escolha — existe, e é
  diretório — e resolvido para absoluto: caminho equivalente escrito de outro
  jeito não pode custar a memória da conversa na comparação da próxima montagem.
- **Trocar o diretório recria a sessão, com o mesmo aviso.** Quem recria é a
  montagem do turno seguinte, que vê o diretório diferente; a tela conta antes
  disso que a recriação está pendente, porque a decisão de trocar agora ou
  terminar o assunto primeiro é de quem conversa. Não conseguir ler a escolha da
  conversa falha o turno em vez de cair no workspace: supor um diretório poria o
  agente a editar uma árvore que ninguém autorizou.

### Fase 7 — Claude Code (feita)

Segundo alvo pelo mesmo client, para separar o que é do protocolo do que era do
Cursor. A sonda não desmentiu essa parte — o turno, as ferramentas e o
cancelamento são iguais —, mas desmentiu a premissa de partida: **o Claude Code
não fala ACP**. Quem atende é um adaptador de terceiros, e é ele que a fase
integra. As extensões `cursor/*` de fato não existem lá, como se esperava; o que
mudou de verdade foi a autenticação, o significado de "modo" e o que se spawna.

Decisões que a fase toma:

- **A detecção procura o adaptador, não o `claude`.** O CLI da Anthropic não
  responde ao protocolo, e apontar o provider para ele daria um agente que sobe e
  nunca se apresenta. O que se procura é o pacote npm, pelos **dois nomes**: o
  atual (`@agentclientprotocol/claude-agent-acp`) primeiro e o deprecado
  (`@zed-industries/claude-code-acp`) depois. Aceitar o antigo não é
  complacência: quem o instalou meses atrás tem uma instalação que funciona, e
  mandar reinstalar o que já está lá é pedir trabalho para não resolver nada. A
  tela já diz de qual arquivo veio o comando sugerido, e é por aí que se descobre
  qual dos dois está em uso.
- **O modo ACP é a ausência de argumento, e isso é do agente.** O `acp` que o
  Cursor e o OpenCode pedem é subcomando deles, não do protocolo; o adaptador do
  Claude sobe sem nenhum. Os argumentos passam a ser parte do template de cada
  agente, e não uma constante que o app aplica a todos.
- **A autenticação não passa pelo protocolo, e `authMethods` vazio não quer
  dizer "não precisa logar".** O diagnóstico de saúde continua correto sem
  mudança, porque ele não olha a lista de métodos: quem revela a falta de login é
  o `session/new` recusando. O que quebra é o comando que a tela mostra para
  resolver — hoje ele é montado a partir do comando do agente, trocando o `acp`
  por `login`, o que aqui produziria um subcomando do adaptador que não existe.
  **O comando de login passa a ser do template do agente**, e no Claude Code ele
  é o login do próprio CLI. A Fase 10 melhora isso para quem informa o comando
  pelo protocolo; o Claude Code não informa, e por isso precisa da resposta que o
  template dá.
- **"Modo", no Claude, é permissão.** Os seis valores (`auto`, `default`
  (Manual), `acceptEdits`, `plan`, `dontAsk`, `bypassPermissions`) descrevem o
  quanto o agente pergunta antes de agir, e não como ele raciocina — o oposto
  da leitura de `agent`/`plan`/`ask` que o app herdou do Cursor. Então **o app
  para de presumir o trio**: o seletor mostra o nome que o agente deu a cada
  valor, e a tradução por categoria fica só para o formato legado, que não manda
  nome nenhum. Presumir significado seria pior do que não traduzir: um rótulo do
  app dizendo "planejar" sobre um modo que na verdade autoriza edições sem
  perguntar descreveria errado justamente a escolha mais perigosa da lista.
- **Escolher um modo que desliga a pergunta vira aviso na conversa.** O app não
  esconde `dontAsk` nem `bypassPermissions` da lista — são modos do agente, que a
  pessoa liga fora do app de qualquer jeito, e esconder daria a falsa impressão
  de que não existem. Mas eles dispensam o `session/request_permission`, que é a
  única barreira que o app tem (D9), e isso é exatamente o caso do aviso de
  "permitir sempre": a escolha muda o comportamento daí em diante, e o seletor
  que a recebeu não fica na tela contando isso.
- **`promptQueueing` e `steering` ficam de fora.** O adaptador anuncia que sabe
  enfileirar prompts e ser guiado no meio do turno; o app serializa em um turno
  por sessão de propósito (D10), porque turno abandonado de agente de código
  continua editando arquivo. Adotar a fila do agente seria abrir mão da regra que
  torna o cancelamento observável, em troca de uma conveniência que a superfície
  de chat não pede.
- **A categoria `thought_level` fica registrada como pendência, e não entra
  aqui.** O transporte já a carrega — qualquer opção de seleção vira
  `ConfigOption`, seja qual for a categoria —, então nada se perde no fio; o que
  falta é a tela. Generalizar o seletor para N categorias é trabalho próprio: o
  par modelo/modo tem evento com campos nomeados (`chat:agent_options`), anúncio
  próprio e persistência no perfil, e um terceiro membro obrigaria a repensar os
  três de uma vez. Fazer isso dentro de uma fase que existe para provar que o
  contrato é do protocolo trocaria a pergunta da fase. O gatilho para tratá-la é
  concreto: **quando um segundo agente oferecer categoria fora do par**, o
  seletor genérico deixa de ser especulação e vira desenho com dois casos reais.
  Até lá o `effort` fica no valor padrão do agente, que é o que ele usaria se o
  app não existisse.

**Aceite:** um perfil apontando para o Claude Code conversa de ponta a ponta pelo
mesmo caminho do Cursor — texto segmentado, raciocínio, ferramentas do agente,
permissão acessível e cancelamento que chega ao agente —; a lista de modelos
aparece com os nomes que o adaptador dá; o seletor de modo mostra os nomes dele,
e escolher um modo que dispensa a pergunta é anunciado na conversa; e o estado
sem login manda rodar o login do Claude Code, e não um subcomando do adaptador
que não existe.

### Fase 8 — O perfil diante de um agente (feita)

Duas coisas apareceram no uso, e são a mesma coisa vista de dois lados: o perfil
oferece ajustes que o agente ignora, e não oferece direito o único ajuste que o
agente respeita.

Do lado do que sobra: as guias de ferramentas, skills e provedores de contexto
seguem editáveis num perfil de agente, e o turno passa por cima delas (D7, D14).
Configuração visível que não muda nada é pior do que configuração ausente —
quem a preenche fica achando que ajustou o comportamento.

Do lado do que falta: o modelo do agente se escolhe no perfil desde a Fase 4,
mas a tela recebe do backend só os valores crus. O Cursor oferece
`grok-4.5[effort=high,fast=true]` com o rótulo "Cursor Grok 4.5", e o caminho de
listagem achata o par num texto só, jogando o nome fora. Escolher modelo numa
lista de identificadores é ruim de ler na tela e pior no leitor de telas.

**Aceite:** num perfil com provedor de agente, a guia de modelos lista os
modelos pelo nome que o agente dá, diz quando o agente não deixa escolher e
explica o que fazer quando ele não sobe; as guias sem efeito não aparecem; e o
turno chega ao agente sem nada além da mensagem da pessoa.

Decisões que a fase fixou:

- **O turno leva só a mensagem da pessoa.** É a revisão do D4 descrita lá em
  cima: nem persona, nem skills, nem memória, nem contexto do app. Interferência
  mínima com quem já tem recurso próprio para tudo isso.
- **As skills do app ficam de fora inteiras, inclusive as invocadas por barra.**
  A skill por `/` viaja dentro da mensagem, e não nas instruções, então dava para
  mantê-la sem contradizer a decisão acima — mas mantê-la obrigaria a guia de
  skills a continuar na tela com outro sentido ("o que dá para invocar", em vez
  de "o que o assistente sabe"), e a explicar essa diferença a cada vez. Num
  perfil de agente o menu da barra fica só com os comandos do próprio agente
  (Fase 6). É reversível: se o uso pedir, a guia volta com o papel novo.
- **O editor esconde o que não tem efeito, em vez de mostrar desabilitado.**
  Guias de ferramentas, skills e provedores de contexto somem enquanto o
  provedor do perfil for um agente, e com elas os parâmetros de amostragem da
  guia de modelos — o provider ACP lê `params.Model` e nada mais. Esconder e
  desabilitar informam a mesma coisa, mas o formulário desabilitado ainda pede
  atenção de quem navega por teclado, guia por guia, para descobrir que não há
  nada a fazer ali.
- **Esconder não apaga.** O que estava configurado continua no arquivo do
  perfil; voltar para um provedor HTTP traz tudo de volta como estava. Limpar
  seria destruir configuração por causa de uma escolha de provedor, e uma troca
  de ideia sairia cara.
- **Sumiço de guia é mudança anunciada.** A contagem de guias muda debaixo de
  quem navega, então a troca de provedor anuncia o que passou a valer, e o foco
  vai para lugar previsível quando a guia aberta é justamente uma das que somem.
- **A lista de modelos do agente vem com rótulo.** A consulta que a tela de
  configuração usa passa a devolver valor e nome, como a barra da conversa já
  recebe. O valor continua sendo o que se grava no perfil; o nome é só para ler.
- **Agente que não oferece modelo não é erro.** Lista vazia quer dizer "quem
  escolhe é ele", e a tela diz isso em vez de acusar falha — o app não tem o que
  consertar aí.
- **Agente que não sobe rende mensagem acionável.** Comando inexistente ou
  desatualizado é o caso comum (o CLI se atualiza sozinho e muda de caminho), e
  a tela do perfil não é o lugar de resolver: ela nomeia o problema e manda para
  a tela de provedores, que já detecta e testa o agente (Fase 3). Duplicar ali o
  diagnóstico faria o editor de perfil subir agente só para sondar.
- **O modo continua só na conversa.** `agent`, `plan` e `ask` mudam com o
  assunto, não com o perfil; guardar um padrão no perfil criaria uma segunda
  fonte de verdade para algo que a pessoa troca no meio do caminho.

### Fase 9 — Os modelos que só vêm no formato anterior (feita)

> **Reescrita.** Esta fase era "OpenCode", e o que ela tinha de trabalho era
> ensinar o app a encontrar e instalar mais um agente. O AEP-0086 passou a
> resolver isso para os 38 do registro de uma vez, e o OpenCode é um deles. O
> que sobrou dela e não sobrou de lá é o que está escrito abaixo, junto com o
> que era da Fase 10 pelo mesmo assunto: o app não sabia ler os modelos que um
> agente anuncia pelo formato anterior ao `configOptions`, e isso não é sobre
> agente nenhum em particular.

O D6 decidiu, lá atrás, que os modelos vêm do `configOptions` e caem para
`models.availableModels[]` quando o agente não oferece o primeiro. A metade da
frente foi escrita; a de trás, não — e ninguém sentiu falta, porque o Cursor
manda os dois formatos no mesmo payload e a leitura de um bastava.

O GitHub Copilot CLI é o agente que cobra a dívida: o `session/new` dele
responde **só** `models`, sem `configOptions` nenhum. O efeito era pior do que
uma lista vazia — a tela diria que o agente não deixa escolher modelo, o que é
mentira, e a troca nunca chegaria ao `session/set_model`. O seletor anterior
está escrito e funciona desde o começo, mas ele é escolhido pela categoria da
opção, e sem opção não há categoria.

O que a fase entrega é só a leitura, porque só ela faltava: o campo `models` das
respostas de `session/new` e `session/load` passa a ser lido pela saída de baixo
nível do D2 — o SDK não o tipa, e é para campo assim que ela existe —, e vira
uma opção de categoria `model`, do mesmo jeito que os modos anteriores já
viravam. A partir daí o resto do caminho já existia.

Decisões que a fase toma:

- **A resposta do SDK é embutida, e não copiada.** O tipo que lê a sessão
  embute o do SDK e acrescenta um campo. Copiar a estrutura inteira para
  acrescentar uma linha a deixaria envelhecer sozinha a cada versão do SDK, e
  campo que o SDK tipar amanhã chegaria sem ninguém precisar copiá-lo de novo.
- **Modelo sem identificador não vira escolha.** O identificador é o que a
  troca manda de volta ao agente; uma linha na lista sem ele só serviria para a
  pessoa tentar. Lista inteira sem identificador nenhum não vira seletor: um
  seletor vazio diz que a escolha existe e não deixa escolher.
- **O que veio pelo formato anterior sobrevive ao conjunto seguinte.** Ele chega
  uma vez, na abertura da sessão, e o conjunto que o agente manda depois fala só
  do que ele guarda em `configOptions`. Isso já valia para o modo, pela mesma
  razão, e passa a valer para os dois: sem isso, trocar de modo faria o seletor
  de modelo sumir da tela no meio da conversa.

**Aceite:** um agente que anuncia modelos só pelo formato anterior tem lista de
modelos na tela, com os nomes que ele deu; escolher um chega a ele por
`session/set_model` e vale para o turno seguinte; reabrir a conversa devolve a
lista, e não uma tela sem escolha; e nada disso muda para quem já mandava
`configOptions`.

### Fase 10 — O login que o agente informa (feita)

> **Reescrita.** Esta fase era "GitHub Copilot CLI", e o trabalho dela era
> encontrar e instalar mais um agente — o que o AEP-0086 passou a resolver para
> os 38 do registro. A leitura dos modelos anteriores, que também era daqui,
> foi para a Fase 9, onde ela é o assunto inteiro. Sobrou o comando de login, e
> ele não é sobre agente nenhum em particular: o app o adivinha para todos, e o
> palpite erra em qualquer um que não suba o ACP por subcomando.

O app monta o comando de login trocando o argumento que sobe o ACP por `login`.
Isso acerta no Cursor por coincidência de forma, e erra em quem não tem essa
forma: o login do OpenCode é `opencode auth login`, e o de um agente que sobe
com `--acp` viraria `copilot --acp login`, que não existe. Mandar alguém ao
terminal para ver um "not found" é pior do que não dizer nada.

Só que o agente frequentemente **diz** como se autentica nele, e o app não
estava ouvindo. Ele diz de dois jeitos: publicando comando, argumentos e rótulo
em `_meta["terminal-auth"]` do método de autenticação, que é o que o Copilot
CLI faz; ou usando a variante de terminal do protocolo, que dá os argumentos
porque o programa é ele mesmo. Nos dois casos o handshake já trazia a
informação, e ela era descartada na conversão.

A fase passa a ler as duas formas e as leva até a tela, com uma ordem: o que o
agente informou vem primeiro, depois o que a procura conhece daquele agente — o
Claude Code, cujo ACP vem de um adaptador npm sem login nenhum, e quem autentica
é o CLI `claude` —, e o palpite por último.

Decisões que a fase toma:

- **Nada do que o agente informa é executado.** Comando e argumentos vindos dele
  são texto não confiável (D11): são saneados e **mostrados** para a pessoa
  copiar. O escape de terminal sai antes, porque uma linha de comando que se
  pinta na tela diferente do que se copia é exatamente o ataque que o saneamento
  existe para impedir. O saneamento aqui não é o de rótulo, que resume em 200
  runas: rótulo é para ler, comando é para colar, e um caminho cortado no meio
  vira uma linha com cara de inteira que não roda. Comando e argumentos usam um
  saneamento que não resume e só achata em linha única; o rótulo do método,
  esse sim, continua saneado como rótulo.
- **A descrição do método é mostrada como o agente escreveu, e não parseada.**
  Vários agentes explicam o login em texto — "Run `opencode auth login` in the
  terminal" — em vez de publicá-lo. Extrair um comando dali seria adivinhar
  outra vez, com mais passos: o texto é de quem escreveu o agente, muda quando
  ele quiser, e um parser errado produziria uma linha que parece oficial e não
  é. Mostrar a frase inteira entrega a mesma instrução sem fingir precisão que
  não existe.
- **Quando o agente explica o login, o palpite some da tela.** Os dois juntos
  seriam duas ordens contraditórias, e a que tem cara de comando pronto é
  justamente a errada: um `opencode login` num quadro logo acima de "rode
  `opencode auth login`". O palpite continua valendo para quem não disse nada,
  que é o caso do Cursor.
- **Argumento sem programa só vira linha quando o app sabe qual é o programa.**
  A variante de terminal descreve o login como argumentos do binário do agente,
  e sem completá-los o que sobraria na tela seria `auth login`, que não roda em
  lugar nenhum. O app completa quando o que está configurado é o executável do
  agente com um subcomando; não completa quando é um interpretador rodando um
  script — `node ...\index.js acp` —, porque ali o agente é o par, e nem o
  executável sozinho (`node auth login`) nem o par inteiro
  (`node ...\index.js acp auth login`) autenticam coisa nenhuma. Linha errada
  com ar de oficial é pior do que linha nenhuma, e nesse caso o que fica na tela
  é a descrição que o agente escreveu.

**Aceite:** um agente que publica o comando de login mostra o dele, e não o que
o app adivinharia; um que explica em texto tem a explicação na tela, com o nome
do método; um que não diz nada continua caindo no que a procura sabe e, por
último, no palpite; e nenhuma dessas linhas é executada pelo app.

## Riscos

- **SDK não oficial.** Mitigado pela interface interna (D2), versão pinada e
  testes com agente falso; a troca de SDK não vaza para o provider.
- **O protocolo está em movimento.** Modelos já vêm em dois formatos no mesmo
  payload; o legado deve sumir. Suportamos os dois e preferimos o estável.
- **Um dos agentes é atendido por adaptador de terceiros** (Fase 7): o Claude
  Code não fala ACP, e a ponte é um pacote npm do ecossistema, que já mudou de
  nome uma vez. O risco não é o do SDK, que a interface interna isola: é o de a
  detecção procurar um pacote que passou a se chamar outra coisa. Mitigado por
  aceitar os nomes conhecidos e por a tela dizer de onde veio o comando que ela
  sugeriu — quem for corrigir precisa saber o que o app achou.
- **O agente age no disco.** Com `cwd` do app (D5), quem inicia o app decide o
  que o agente alcança. Risco aceito e mitigado por visibilidade + permissões;
  não é aceitável esconder o diretório.
- **Prompt injection com consequência real.** Diferente de um provider comum, o
  texto processado pode levar a edições e comandos. As permissões (D9) são a
  barreira, e por isso não podem ser auto-aprovadas em massa por padrão.
- **Turno travado.** Toda pergunta bloqueante tem prazo e resposta padrão (D9).
- **Autenticação expira** fora do app; o estado precisa ser diagnóstico, não um
  erro genérico de conexão.
- **Sem contabilidade de tokens** (D8): o painel de custo fica cego para ACP.
- **Windows**: wrapper `.ps1`, atualização automática do CLI mudando o caminho
  versionado, e o risco de processo órfão.
- **A autenticação do agente é da máquina, não do usuário do app.** O app é
  multiusuário (AEP-0052) e isola providers e credenciais por `user_id`, mas o
  `agent login` vive fora disso: dois usuários do app que usem o mesmo binário
  conversam com a **mesma conta** do Cursor. O app não tem como isolar isso;
  cabe declarar a limitação onde o provider é configurado.
- **Barge-in com efeito colateral** (D10): mandar mensagem por cima de um turno
  em andamento cancela um agente que pode estar no meio de uma edição.
- **Conversa com agente é conversa sem o assistente** (Fase 8): sem persona e
  sem memória do app, a mesma pergunta feita a um perfil comum e a um perfil de
  agente rende respostas de interlocutores diferentes. É o efeito desejado, mas
  precisa estar claro na tela de quem escolhe o provedor, senão a diferença
  aparece como comportamento errático.

## Critérios de aceitação

- Um perfil apontando para um provider ACP conversa na superfície de chat comum,
  com streaming de texto e de raciocínio, sem fluxo de envio alternativo.
- Nenhuma tool do app é oferecida ao agente, e nenhuma tool call do agente entra
  no loop agêntico do app.
- A atividade de ferramenta do agente aparece e é anunciada como **dele**, com
  origem `acp_agent`, e não aciona consumidores que reagem a nome de tool do app
  (o chat inline do editor, em particular).
- Pedido de permissão é anunciado, navegável por teclado, respondível e tem
  prazo com resposta padrão; em superfície sem quem responda, é negado na hora
  em vez de pendurar o turno.
- A resposta é falada em segmentos durante o turno, não só no fim, e o segmento
  final é leitura protegida.
- Sumarização automática e papéis auxiliares do perfil não gastam turnos do
  agente.
- Lista de modelos e troca de modelo funcionam com o Cursor, pelos dois formatos
  de seleção, e com os demais agentes pelo formato que cada um fala — inclusive
  com quem só fala o legado.
- Subir cada agente é configuração do template dele — subcomando, flag ou
  argumento nenhum —, e não uma suposição do app sobre como todos sobem.
- O comando de login que a tela mostra é o do agente, quando ele o informa em
  `_meta` ou o descreve no método; o comando que o app monta é o último recurso,
  para quem não diz nada.
- Categoria de opção que o app não representa não impede a conversa: ela fica
  fora do seletor, o agente segue no valor padrão dele, e a ausência está
  declarada em vez de acontecer em silêncio.
- Um provider ACP é criado, testado e diagnosticado pela UI sem `BaseURL`.
- O turno do agente leva só a mensagem da pessoa: persona, skills, memória,
  resumo e blocos de contexto do app ficam de fora.
- Num perfil com provedor de agente, a guia de modelos lista o que o agente
  oferece pelo nome dele, e as guias sem efeito não aparecem — sem apagar o que
  o perfil já tinha configurado.
- Reabrir uma conversa retoma a sessão do agente ou informa que a memória se
  perdeu; retry, edição e exclusão nunca deixam o agente respondendo sobre um
  histórico que a pessoa não vê.
- Cancelar o turno cancela do lado do agente.
- Texto e IDs vindos do agente são saneados antes de virar UI, anúncio ou chave.

## Apêndice: sonda de verificação

A verificação que produziu as descobertas acima é reproduzível. Requer o CLI
instalado (`irm 'https://cursor.com/install?win32=true' | iex` no Windows) e
autenticado (`agent login`). No Windows, `AGENT_BIN`/`AGENT_ARGS` apontam para o
par `node.exe index.js` da versão instalada, porque não há `agent.exe`.

As mesmas duas variáveis sondam os outros agentes: `AGENT_ARGS` é `["acp"]` no
OpenCode, `["--acp"]` no Copilot CLI e `[]` no adaptador do Claude Code, que sobe
em modo ACP sem argumento nenhum; e `AGENT_BIN` é o `node.exe` do par versionado
para quem se instala como pacote npm com entry `.js`, ou o executável nativo nos
demais. A chamada de `authenticate` é do Cursor e já vem com o erro engolido de
propósito: quem não anuncia `cursor_login` a recusa, e a sonda segue.

```js
import { spawn } from 'node:child_process';
import readline from 'node:readline';

const bin = process.env.AGENT_BIN ?? 'agent';
const args = process.env.AGENT_ARGS ? JSON.parse(process.env.AGENT_ARGS) : ['acp'];
const agent = spawn(bin, args, { stdio: ['pipe', 'pipe', 'inherit'] });

let nextId = 1;
const pending = new Map();
const send = (method, params) => {
  const id = nextId++;
  agent.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }));
};
const respond = (id, result) =>
  agent.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, result }) + '\n');

readline.createInterface({ input: agent.stdout }).on('line', (line) => {
  const msg = JSON.parse(line);
  if (msg.id !== undefined && (msg.result !== undefined || msg.error !== undefined)) {
    const waiter = pending.get(msg.id);
    pending.delete(msg.id);
    msg.error ? waiter?.reject(msg.error) : waiter?.resolve(msg.result);
    return;
  }
  if (msg.method === 'session/update') {
    console.log('[update]', msg.params.update.sessionUpdate);
    return;
  }
  console.log(`[pedido] ${msg.method}`, JSON.stringify(msg.params).slice(0, 600));
  if (msg.id === undefined) return;
  // Negar por padrão: a sonda não deve autorizar nada na máquina.
  if (msg.method === 'session/request_permission') {
    respond(msg.id, { outcome: { outcome: 'selected', optionId: 'reject-once' } });
  } else {
    respond(msg.id, { outcome: { outcome: 'cancelled' } });
  }
});

const main = async () => {
  console.log(JSON.stringify(await send('initialize', {
    protocolVersion: 1,
    clientCapabilities: { fs: { readTextFile: false, writeTextFile: false }, terminal: false },
    clientInfo: { name: 'assistente-acp-probe', version: '0.1.0' },
  }), null, 2));

  await send('authenticate', { methodId: 'cursor_login' }).catch(() => {});
  const session = await send('session/new', { cwd: process.cwd(), mcpServers: [] });
  console.log(JSON.stringify(session, null, 2));

  const alvo = session.configOptions?.find((o) => o.category === 'model')?.options?.[1]?.value
    ?? session.models?.availableModels?.[1]?.modelId;
  if (alvo) {
    await send('session/set_model', { sessionId: session.sessionId, modelId: alvo })
      .then((r) => console.log('set_model:', JSON.stringify(r)), (e) => console.log('set_model erro:', JSON.stringify(e)));
    await send('session/set_config_option', { sessionId: session.sessionId, configId: 'model', value: alvo })
      .then((r) => console.log('set_config_option:', JSON.stringify(r).slice(0, 300)), (e) => console.log('erro:', JSON.stringify(e)));
  }

  const turn = await send('session/prompt', {
    sessionId: session.sessionId,
    prompt: [{ type: 'text', text: 'Liste os arquivos markdown na raiz usando suas ferramentas.' }],
  });
  console.log('stopReason:', turn.stopReason);
};

main().finally(() => { agent.stdin.end(); setTimeout(() => agent.kill(), 500); });
```

O valor da sonda não se esgota: quando o Cursor mudar o formato de `session/new`,
é ela que diz o que mudou.

## Referências

- Protocolo: <https://agentclientprotocol.com/> (Session Config Options
  estabilizadas em 2026-02-04)
- Cursor CLI em modo ACP: <https://cursor.com/docs/cli/acp>
- Adaptador do Claude Code: `@agentclientprotocol/claude-agent-acp` (antes
  `@zed-industries/claude-code-acp`, deprecado)
- OpenCode: pacote `opencode-ai`, subcomando `acp`
- GitHub Copilot CLI: pacote `@github/copilot`, flag `--acp`
- SDK adotado: `github.com/coder/acp-go-sdk`
- AEP-0012 (barramento multi-provedor), AEP-0037 (contrato `ChatProvider`),
  AEP-0040 (messaging backend-driven), AEP-0064 (cancelamento), AEP-0068
  (subagentes e dados não confiáveis), AEP-0076 (migrações), AEP-0077
  (ToolPlanner), AEP-0081 (política de tools por perfil)

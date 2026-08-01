# AEP-0084 — Agentes de código ACP como providers LLM

**Status:** 📝 Draft

## Resumo

Agentes de codificação que falam **ACP (Agent Client Protocol)** — Cursor CLI
(`agent acp`) e Claude Code (via adapter) — entram no app como **providers LLM
comuns**, selecionáveis por perfil, conversando na superfície de chat como
qualquer outro provider. Um novo `APIFormat = "acp"` no barramento, um client
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
  capacidades de filesystem nem de terminal no `initialize`, o `ToolPlanner`
  (AEP-0077) não planeja tools para um provider ACP e o context provider
  `tool_protocol` fica de fora do system prompt — senão o prefixo estável
  mandaria o agente chamar `tool_catalog`, uma ferramenta que ele não tem.
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

Uma premissa anterior também caiu: **`internal/mcp` não implementa JSON-RPC** —
ele usa o SDK oficial de MCP. O que existe de reaproveitável ali é o padrão de
subprocesso (spawn com contexto, `osutil.HideConsoleWindow`, env, backoff,
health), não o protocolo.

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

#### As instruções do perfil precisam chegar ao agente

O pipeline injeta persona, skills e blocos de contexto (AEP-0075) em uma única
mensagem `system` antes do `StreamChat`. "Enviar só a última mensagem do
usuário" não pode significar descartá-las — seria o único provider do barramento
a ignorar o perfil. Mas o ACP não tem papel `system`: `session/prompt` recebe
blocos de conteúdo do usuário.

O app já marca, nessa mensagem, onde termina o **prefixo estável**
(`SystemCacheControlPrefixLen`, usado hoje para cache de prompt). Usamos essa
mesma fronteira:

- o **prefixo estável** (persona, skills) vai **uma vez por sessão**, como bloco
  delimitado no primeiro `session/prompt`;
- **tudo o que vem depois do prefixo** vai **no turno em que mudar**, como bloco
  próprio junto da mensagem do usuário. Não é só o contexto de workspace: o
  sufixo dinâmico carrega tasklists vinculadas, memória do usuário e o resumo da
  conversa, e deixar qualquer um de fora seria entregar ao agente um perfil pela
  metade. O que o builder já coloca dentro da própria mensagem do usuário
  (`turn_context`) segue junto sem tratamento especial;
- o provider guarda o hash do que já enviou **por sessão, não por conversa**:
  sessão nova é agente sem memória nenhuma, então tudo é reenviado. Isso vale
  para a sessão recriada por divergência e pela falha do `session/load` — o
  estado do que "já foi dito" morre junto com a sessão que o ouviu. O hash é
  persistido no mesmo registro do `sessionId`, e não só em memória: uma sessão
  retomada com sucesso depois de reiniciar o app já ouviu a persona, e repetir
  tudo seria desperdício. Trocar de perfil no meio da conversa reenvia o
  prefixo, porque as instruções passaram a ser outras.

Os blocos são delimitados como instrução do app, e não se confundem com o texto
da pessoa. O agente também tem as regras do próprio projeto (`AGENTS.md`,
`.cursor/rules`), que continuam sendo assunto dele.

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

### D5. `cwd` é o diretório de trabalho do app

O `session/new` exige um diretório: é ele que define onde o agente edita
arquivos e qual `.cursor/mcp.json` vale. Nesta fase **não há seletor**: usa-se o
diretório de trabalho do app.

A consequência precisa estar visível, não implícita: **o agente age sobre o
diretório de onde o app foi iniciado**. A UI mostra qual é esse diretório junto
ao provider, e o primeiro turno de cada sessão o anuncia. Um seletor por
conversa é evolução natural (Fase 5), não requisito.

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
| `session_info_update.title` | `conversation:renamed` | opcional, Fase 5 |
| `available_commands_update` | ignorado por ora | slash commands do agente, Fase 5 |

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
  de escopo das allowlists existentes.
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
- a coluna `base_url` perde o `NOT NULL` (migração versionada, AEP-0076) e as
  novas colunas entram pelo mesmo caminho;
- descoberta e health ramificam por formato: para ACP, "saudável" é **spawnar,
  fazer `initialize` e receber `authMethods`** — e a falta de autenticação vira
  um estado próprio, com a instrução (`agent login`), não um erro genérico;
- credenciais não se aplicam: `CredentialPattern` vazio, `AuthMode` = none;
- export/import (`ProviderExport`) hoje carrega `BaseURL` como obrigatório e não
  tem onde guardar comando e argumentos. Ganha os campos novos, com o
  `MCPServerExport` como precedente — ele já exporta `Command`/`Args`. Caminho
  de binário é específico da máquina: na importação, um provider ACP cujo
  comando não existe entra **desativado com aviso**, em vez de falhar a
  importação inteira ou fingir que funciona.

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

### Fase 1 — Transporte (`internal/acp`)

Client sobre o SDK atrás da interface do D2: spawn com contexto, handshake,
sessões multiplexadas, `prompt` com sink de updates, cancelamento, morte e
reconexão do processo. Testes contra um agente ACP falso.

**Aceite:** um teste faz um turno completo contra o agente falso, incluindo
pedido de permissão respondido e cancelamento.

### Fase 2 — Provider no barramento

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

### Fase 3 — Provider de primeira classe na UI

Template builtin do Cursor com detecção da instalação, formulário (comando,
argumentos, diretório visível), health do D12, indicador de conexão e estado de
"não autenticado" com instrução acionável.

**Aceite:** dá para criar, testar e usar um provider Cursor sem editar
configuração na mão; o estado não autenticado é anunciado e explica o que fazer.

### Fase 4 — Modelos e modos

`GetModels` com a sessão de descoberta do D6 e cache, troca por
`set_config_option` com
fallback, `config_option_update` refletido na UI, seleção de modo
(`agent`/`plan`/`ask`) exposta junto do modelo, `Chat.Model` do perfil aplicado
na criação da sessão.

**Aceite:** trocar de modelo pelo app muda o modelo do turno seguinte; troca
feita pelo agente aparece na UI e é anunciada.

### Fase 5 — Perguntar fora do desktop

Roteamento da pergunta pela superfície de origem (D9): pedido vira mensagem no
canal, com opções numeradas, resposta restrita ao dono do canal, prazo curto e
`allow-always` barrado fora do desktop. Mecanismo genérico, aproveitável pelas
confirmações que já existem.

**Aceite:** uma conversa de canal com perfil ACP consegue autorizar e negar uma
ação pela própria conversa; sem resposta no prazo, o pedido é negado e a pessoa
é informada; resposta de quem não é dono do canal é ignorada.

### Fase 6 — Continuidade e conveniências

`session/load` na reabertura, título vindo de `session_info_update`, slash
commands de `available_commands_update`, seletor de diretório por conversa.

### Fase 7 — Claude Code

Segundo alvo pelo mesmo client, validando que o contrato é do protocolo e não do
Cursor. Ajustes esperados: método de autenticação diferente e ausência das
extensões `cursor/*`.

## Riscos

- **SDK não oficial.** Mitigado pela interface interna (D2), versão pinada e
  testes com agente falso; a troca de SDK não vaza para o provider.
- **O protocolo está em movimento.** Modelos já vêm em dois formatos no mesmo
  payload; o legado deve sumir. Suportamos os dois e preferimos o estável.
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
  de seleção.
- Um provider ACP é criado, testado e diagnosticado pela UI sem `BaseURL`.
- As instruções do perfil chegam ao agente, sem reenvio do prefixo estável a
  cada turno.
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
- SDK adotado: `github.com/coder/acp-go-sdk`
- AEP-0012 (barramento multi-provedor), AEP-0037 (contrato `ChatProvider`),
  AEP-0040 (messaging backend-driven), AEP-0064 (cancelamento), AEP-0068
  (subagentes e dados não confiáveis), AEP-0076 (migrações), AEP-0077
  (ToolPlanner), AEP-0081 (política de tools por perfil)

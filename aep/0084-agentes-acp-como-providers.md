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
  capacidades de filesystem nem de terminal no `initialize`, e o `ToolPlanner`
  (AEP-0077) não planeja tools para um provider ACP.
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

O resto do app conversa só com essa interface. Se o SDK estagnar ou divergir,
troca-se a implementação sem tocar no provider. A versão fica pinada e o
comportamento é coberto por testes contra um **agente ACP falso** (um processo
de teste que fala o protocolo), não contra o Cursor real.

### D3. Um processo por provider, sessões multiplexadas

O `agent acp` aceita várias sessões na mesma conexão. Mantemos **um processo por
provider ACP configurado**, iniciado sob demanda no primeiro turno e encerrado
no shutdown do app, com as sessões multiplexadas por `sessionId`. Isso evita
pagar o custo de spawn e autenticação por conversa. Morte do processo é tratada
como o MCP trata: reconecta com backoff e marca as sessões como perdidas.

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
no ACP é acoplada à sessão. Resolvemos com uma **sessão efêmera** (spawn →
`initialize` → `authenticate` → `session/new` → leitura → descarte) e **cache**
por provider, invalidado quando um `config_option_update` chega ou quando a
pessoa pede refresh na UI.

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
- **Toda pergunta tem prazo.** Sem resposta dentro do prazo, respondemos
  `reject-once` e informamos: um turno travado para sempre é pior do que uma
  ação negada.
- O título do `toolCall` contém o comando literal e é **dado não confiável**:
  passa pelo mesmo saneamento de texto de diálogo antes de virar rótulo ou
  anúncio.

Nenhuma fase entrega o modo `agent` sem esse caminho pronto.

### D10. Cancelar no app cancela o turno no agente

O cancelamento explícito do app (AEP-0064) envia `session/cancel`, que interrompe
o turno em andamento sem encerrar a sessão, e trata o `stopReason: cancelled`
como fim normal do turno. A sessão continua viva para a próxima mensagem.

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
- credenciais não se aplicam: `CredentialPattern` vazio, `AuthMode` = none.

### D13. Spawn no Windows

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
permissões do D9 e saneamento do D11. Persistência mínima do D12 para conseguir
registrar um provider ACP.

**Aceite:** com um provider ACP configurado à mão, uma conversa no app fala com
o Cursor de ponta a ponta — texto, raciocínio, eventos de ferramenta, pedido de
permissão acessível e cancelamento. Nenhum caminho novo de envio (AEP-0040).

### Fase 3 — Provider de primeira classe na UI

Template builtin do Cursor com detecção da instalação, formulário (comando,
argumentos, diretório visível), health do D12, indicador de conexão e estado de
"não autenticado" com instrução acionável.

**Aceite:** dá para criar, testar e usar um provider Cursor sem editar
configuração na mão; o estado não autenticado é anunciado e explica o que fazer.

### Fase 4 — Modelos e modos

`GetModels` com sessão efêmera e cache, troca por `set_config_option` com
fallback, `config_option_update` refletido na UI, seleção de modo
(`agent`/`plan`/`ask`) exposta junto do modelo, `Chat.Model` do perfil aplicado
na criação da sessão.

**Aceite:** trocar de modelo pelo app muda o modelo do turno seguinte; troca
feita pelo agente aparece na UI e é anunciada.

### Fase 5 — Continuidade e conveniências

`session/load` na reabertura, título vindo de `session_info_update`, slash
commands de `available_commands_update`, seletor de diretório por conversa.

### Fase 6 — Claude Code

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

## Critérios de aceitação

- Um perfil apontando para um provider ACP conversa na superfície de chat comum,
  com streaming de texto e de raciocínio, sem fluxo de envio alternativo.
- Nenhuma tool do app é oferecida ao agente, e nenhuma tool call do agente entra
  no loop agêntico do app.
- Pedido de permissão é anunciado, navegável por teclado, respondível e tem
  prazo com resposta padrão.
- Lista de modelos e troca de modelo funcionam com o Cursor, pelos dois formatos
  de seleção.
- Um provider ACP é criado, testado e diagnosticado pela UI sem `BaseURL`.
- Reabrir uma conversa retoma a sessão do agente ou informa que a memória se
  perdeu.
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

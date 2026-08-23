# AEP-0068 — Sub-agentes em segundo plano (tool de sub-conversas)

Status: Done — Fases 1–5 entregues nos PRs empilhados de sub-agentes
Data: 2026-06-02
Autor: Inclunet + Cursor Agent

## Resumo

Esta AEP introduz uma **única builtin tool `subagent`**, dirigida por parâmetros,
que permite ao LLM delegar tarefas a **sub-agentes** que executam em
**sub-conversas próprias, persistidas e visíveis** (do mesmo usuário). A mesma
tool cobre, variando apenas parâmetros:

- **Executar e esperar** (`background:false`) — responde em tempo real no fluxo do
  pai, como uma tool normal; ou **executar em segundo plano** (`background:true`)
  — retorna um handle (`conversation_id`/`run_id`) na hora e **avisa ao concluir**.
- **Criar uma sub-conversa nova** (sem `conversation_id`) ou **continuar/reusar**
  uma existente (`conversation_id` presente), preservando todo o contexto — igual
  ao "resume com agent ID" do Cursor/Claude Code.
- **Resetar** o histórico antes de enviar (`clear:true`) quando se quer reaproveitar
  a conversa para um problema novo.
- **Trocar modelo/comportamento** do sub-agente via `profile` (slug do profile de
  interação), reaproveitando `ChatParams.ProfileSlug`.

Como `subagent` é uma tool comum (executada pelo executor/`tool_invocations`),
**jobs também podem chamá-la** — habilitando processos de background recorrentes,
inclusive os que mantêm histórico numa mesma sub-conversa (ex.: um batch diário).

## Motivação

Hoje o "agentic loop" (`internal/agent/service.go`, `RunAgenticLoop`) é sempre
**um único LLM chamando tools** numa conversa. Não há como:

- delegar uma subtarefa a um agente paralelo com seu próprio contexto e modelo;
- pedir algo "em segundo plano" e ser avisado ao concluir, sem travar a conversa;
- reabrir um agente anterior para continuar resolvendo um problema mal resolvido
  (ex.: vários PRs abertos, pedindo a cada review que o sub-agente original corrija
  os problemas do seu PR);
- usar um sub-agente conversacional como passo de um job recorrente.

O AEP-0001 já previa "LLM no loop como tool type" como evolução futura. Esta AEP
concretiza isso sem caso especial: o sub-agente é uma tool, então tanto o chat
quanto os jobs o acionam pelo mesmo caminho.

## Relação com outras AEPs

- **AEP-0040 (Backend-Driven Messaging)**: o disparo reusa o pipeline oficial
  (`SendMessageUseCase.Execute`); nada de criar mensagens locais no frontend nem
  fluxo de envio alternativo. O **aviso de conclusão** entra como conteúdo do
  assistente na conversa do pai (ver Decisões) — uma extensão que esta AEP
  documenta e alinha ao contrato de eventos/mensagens.
- **AEP-0052 (Contas de Usuário)**: toda sub-conversa pertence ao mesmo `userID`;
  `resume` valida owner.
- **AEP-0025/0062 (Profiles)**: o `profile` do sub-agente define modelo,
  comportamento e as **tools habilitadas** via `Profile.Chat.EnabledTools` (lista;
  `nil` = seleção dinâmica/catálogo). O gate global de tool calling é o booleano
  `Profile.Chat.DisableTools` (`true` = sub-agente **sem nenhuma** tool, não é
  denylist). Assim, se a tool `subagent` estiver fora de `EnabledTools` (ou
  `DisableTools=true`), o sub-agente **não** pode criar novos sub-agentes.
- **AEP-0001/0048/0063 (Jobs e Tool Invocations)**: jobs chamam a tool `subagent`;
  a persistência separa run de negócio de `tool_invocations`; proveniência via
  `eventctx` e circuit-breaker são reaproveitados como backstop anti-runaway.
- **AEP-0059 (Performance de conversas longas) + sumarização**: sub-conversas
  reusadas por muito tempo (ex.: batch diário) usam a sumarização existente.
- **AEP-0065 (Rate Limiting)**: chamadas dos sub-agentes passam pelo rate limiter.

## Decisões

### Contrato da tool `subagent` (única, dirigida por parâmetros)

- `prompt` (string, **opcional**): tarefa/mensagem. Presente = envia/continua;
  omitido (com `conversation_id`, opcionalmente `run_id`, e sem `cancel`) =
  consulta de **status**.
- `conversation_id` (string UUIDv7, opcional — padrão de IDs do projeto, AEP-0046):
  **ausente → cria** sub-conversa nova; **presente → continua** na mesma (resume),
  preservando histórico.
- `background` (bool, default `false`): `false` → executa e o pai **espera** o
  resultado (inline); `true` → retorna o handle na hora e **avisa ao concluir**.
- `clear` (bool, default `false`): reseta o histórico da sub-conversa **e envia** a
  nova mensagem na mesma chamada (requer `conversation_id` **e** `prompt`; não é
  válido em consultas de `status`).
- `profile` (string, opcional): slug do profile do sub-agente
  (`ChatParams.ProfileSlug`). Default (vazio): chamadas originadas do chat/workspace
  **herdam o profile já resolvido do pai**; só na ausência de pai (job/system) cai no
  perfil ativo global.
- `title` (string, opcional): título da sub-conversa (persistido em `Conversation.Title`).
- `model` (string, opcional): modelo de execução do sub-agente (`llm.ChatParams.Model`),
  **sobrescreve** o modelo derivado do `profile` para aquele run. Não é persistido na
  entidade `Conversation` (que só tem `Title`) — é parâmetro de execução do envio.
- `tools` (string[], opcional): **restringe** (subconjunto) sobre as tools já
  habilitadas pelo profile — o profile é o gate primário. Semântica de vazio (alinhada
  à base, que distingue `nil` de `[]`): **omitido/`nil`** = herda as tools do profile;
  **`[]` (lista vazia)** = nenhuma tool (sub-agente sem tool calling). Itens fora do
  habilitado pelo profile são ignorados (nunca expandem privilégio).
- `run_id` (string UUIDv7, opcional — AEP-0046): identifica um **run específico**
  (turno) de uma sub-conversa para `status`/`cancel`. Se omitido, as operações abaixo
  agem sobre o **run mais recente** da `conversation_id` informada.
- `cancel` (bool, opcional): cancela um run em andamento. Requer `conversation_id`
  (e opcionalmente `run_id` para escolher um run específico; sem ele, cancela o run
  ativo mais recente). É **mutuamente exclusivo com `prompt`/`clear`** — uma chamada
  ou envia/continua, ou cancela, nunca ambos.

#### Validações mínimas (combinações inválidas → erro)

- `clear:true` sem `conversation_id` → erro (nada a resetar).
- `clear:true` sem `prompt` → erro. `clear` é sempre **reset + envio** na mesma
  chamada; nunca limpa o histórico sem enviar uma nova mensagem. Logo, `clear` não é
  válido em consultas de `status` (sem `prompt`).
- `cancel:true` sem `conversation_id` → erro (não há run a cancelar sem a conversa alvo).
- `cancel:true` junto com `prompt` ou `clear` → erro (operações mutuamente exclusivas).
- `run_id` sem `conversation_id` em **send/cancel** → erro (a operação exige a
  conversa alvo explícita). Em **status** (sem `prompt` e sem `cancel`), `run_id`
  **sozinho é permitido**: o run sempre pertence a uma conversa, mas o Manager o
  resolve pelo próprio `run_id` e recupera a conversa dele (ver "Resolução de run
  em `status`/`cancel`"). A invariante "run pertence a uma conversa" continua
  válida — apenas não exige que o chamador informe a conversa quando o `run_id`
  já identifica o run.
- `run_id`/`conversation_id` que não pertencem ao usuário → erro (escopo AEP-0052).
- Sem `prompt`, sem `cancel` e sem `conversation_id`/`run_id` → erro (nada a fazer).

#### Retorno da tool

Campos **sempre presentes** na resposta: `conversation_id`, `run_id`, `status`
(`queued`/`running`/`succeeded`/`failed`/`cancelled`/`timed_out` — alinhado ao
enum já usado em `tool_invocations`). Variações por modo:

- `background:false` (síncrono): além do handle, retorna o **resultado final**
  (`result_summary`/conteúdo da resposta do sub-agente e `assistant_message_id`). O
  pai guarda `conversation_id`/`run_id` para retomar (`resume`) ou cancelar depois.
- `background:true`: retorna o handle imediatamente (`status` tipicamente `queued`/
  `running`); o resultado chega depois pelo aviso de conclusão.
- `status` (sem `prompt`): retorna o estado atual do run alvo (`status`, e
  `result_summary`/`assistant_message_id`/`error` quando já concluído).
- `cancel`: se havia run ativo (`queued`/`running`), retorna o handle com
  `status=cancelled` e `cancelled:true`. Se **não** havia run ativo (run já terminal
  ou inexistente), é **no-op**: retorna o `status` atual real do run (ex.: `succeeded`/
  `failed`/`timed_out`/`cancelled`) com `cancelled:false` e uma `message`
  informativa. O chamador (LLM/job) diferencia pelo campo `cancelled` (e/ou pela
  transição do `status`), não apenas pelo `status` final.

#### Resolução de run em `status`/`cancel` (múltiplos runs por conversa)

Como cada `prompt` cria um novo run (turno) sobre a mesma `conversation_id`, as
operações de consulta/cancelamento resolvem o alvo assim:

- `run_id` informado → opera exatamente nesse run (validando que pertence ao
  usuário; e, **se** `conversation_id` também for informado, que pertence a ela).
  Em `status`, `conversation_id` é **opcional** quando há `run_id` (o run é
  resolvido pelo id); em `cancel`, `conversation_id` permanece obrigatório.
- `run_id` omitido → opera no **run mais recente** da `conversation_id` (para `cancel`,
  o run ativo mais recente; se nenhum estiver ativo, é no-op com status informativo).
- Precedência de parâmetros (mutuamente exclusivos): `cancel:true` → cancelar;
  senão `prompt` presente → criar/continuar; senão (só `conversation_id`/`run_id`,
  sem `prompt`) → `status`.

Listar sub-conversas é responsabilidade da **UI**, não uma tool — para não inchar a
superfície exposta ao LLM; ele reabre usando o `conversation_id` que já recebeu. A
listagem é **unificada**: o mesmo binding `GetConversations` retorna conversas comuns e
sub-conversas (campo `kind`), com o `latestStatus` do run mais recente preenchido via
`LEFT JOIN sub_agent_runs` (ver Fase 5). Não há binding separado para sub-agentes.

### Sub-conversa e identidade

- **1 conversa UUID por sub-agente** (não reusa a do pai: há um stream por conversa
  no `StreamingManager`; reusar causaria cancelamento mútuo).
- `Conversation` ganha `parent_conversation_id` e `kind` (`subagent`) para vínculo
  e distinção no histórico (badge/filtro na UI). O **handle durável é o
  `conversation_id`**.

### Disparo e execução

- Headless, reusando o padrão dos canais: `CreateConversationWithContext` +
  `SendMessageUseCase.Execute(Source="subagent")` com o `ctx` autenticado do pai.
- Background real em **goroutine** com `context.WithoutCancel` + `database.WithUserID`
  (modelo dos jobs).
- **Detecção de conclusão por callback in-process** (análogo ao `ResponseNotifier`
  dos canais), não por evento que só chega ao frontend.

### Aviso de conclusão (background) — entrega pelo lado do assistente

Ao concluir, o resultado é entregue como **conteúdo do assistente** na conversa do
pai e o **loop do pai é re-disparado (auto-wake)** para reagir, com proveniência
propagada:

- Se a **última interação do pai for do assistente** (pai ocioso desde que pediu o
  sub-agente) → **continua/prepend** a resposta do assistente (mesmo turno).
- Se o **usuário interagiu depois** → entra como **nova mensagem do assistente**.
- Guardas: **fila serializada por conversa-pai** (evita corrida no `StreamingManager`)
  e **idempotência por `run_id`** (sem aviso duplicado em retry/recovery).
- No modo síncrono não há injeção — o resultado volta pelo retorno da tool.

### Profundidade e segurança

- **Profundidade governada pelo profile**: o sub-agente cria novos sub-agentes
  apenas se o profile dele habilitar a tool `subagent`. Sem `max_depth` próprio nem
  proibição hardcoded. Qualquer profile é permitido (responsabilidade do usuário).
- **Backstop anti-runaway** (não é limite de profundidade): proveniência `eventctx`
  + circuit-breaker (`chain_id`/histórico) compartilhados com jobs, limite de
  concorrência por usuário/pai e timeout por run.
- Job (origin `job_run`) chamando `subagent` é ponto de entrada legítimo (não conta
  como recursão de sub-agente), mas carrega proveniência.

### Persistência (2 camadas, AEP-0048/0063)

- **Sessão = a sub-conversa**; cada chamada com `prompt` gera uma linha em
  `sub_agent_runs` (um run por turno): `id`, `user_id`, `parent_conversation_id`,
  `parent_turn_id`, `child_conversation_id`, `turn_index`, `status`,
  `result_summary`, `assistant_message_id`, `error`, `chain_*`, timestamps.
- Reusar `tool_invocations` **sem expandir o enum `origin_type`** da AEP-0063. O
  `origin_type` da invocação da tool `subagent` segue a **origem do executor** que a
  disparou (chat → `chat`; job → `job_run`; etc.), exatamente como qualquer outra tool
  call. O vínculo pai↔filho **não** vem de um `origin_type` próprio, e sim de
  `ParentInvocationID` + da tabela `sub_agent_runs`. O campo `ParentInvocationID` já
  existe e é suportado no modelo/repositório
  (`internal/database/models_tool_invocations.go`, `internal/toolinvocations/repository.go`);
  o que falta é o **pipeline de chat preenchê-lo** — esta AEP passa a populá-lo para
  encadear a invocação ao turno pai. (Caso a telemetria futura exija distinguir runs de
  sub-agente já na linha de `tool_invocations`, isso será uma evolução explícita da
  AEP-0063, não uma extensão silenciosa aqui.)

## Fases

- **Fase 1 — Núcleo síncrono**: modelo de dados (`sub_agent_runs`,
  `Conversation.kind/parent_conversation_id`); pacote `internal/subagent`
  (Manager+Repository); tool `subagent` mínima (cria conversa nova + `background:false`
  + `profile`); detecção de conclusão por callback in-process.
- **Fase 2 — Background + aviso**: goroutine `WithoutCancel`; `background:true` com
  handle imediato; aviso pelo lado do assistente (continua vs nova mensagem) +
  auto-wake + proveniência; fila serializada por conversa-pai + idempotência;
  `prompt` omitido = status; `cancel`.
- **Fase 3 — Reuso/continuidade**: `conversation_id` (resume) preservando contexto;
  `clear` (reset); integração com sumarização para conversas longas.
- **Fase 4 — Jobs**: job chamando a tool `subagent` (conversa fixa = histórico
  recorrente; ou conversa nova por run); proveniência/circuit-breaker compartilhados;
  reconciliação de runs órfãos no startup.
- **Fase 5 — UI + limites**: sub-conversas são **mescladas na listagem do Histórico**
  (`HistoryPage`), não em página separada — são conversas comuns do ponto de vista do
  usuário (mesma tabela, mesmas ações: abrir, renomear, excluir, exportar). A listagem
  vem de **um único binding `GetConversations`**, que inclui as sub-conversas (campo
  `kind`) e o `latestStatus` do run mais recente (via `LEFT JOIN sub_agent_runs`). A
  `HistoryPage` exibe cada sub-conversa com um **badge "sub-agente"** e, quando o run
  está ativo (`queued`/`running`), um **indicador de status**; um **filtro "mostrar
  sub-agentes"** permite ocultá-las. A busca (FTS) já cobre todas as conversas (o índice
  de mensagens não filtra por `kind`), então é uniforme. Teto global de concorrência por
  usuário + visibilidade de custo e passagem pelo rate-limiter (AEP-0065).
  - **Dois tetos de concorrência, não um**: o teto por usuário
    (`MaxConcurrentPerUser`, default 4) não limita o custo do processo — com N
    usuários ativos ele autoriza N × 4 runs, cada um segurando uma goroutine, um
    stream LLM e tokens. Existe também um teto **agregado**
    (`MaxConcurrentGlobal`, default 16 = quatro usuários no teto individual ao
    mesmo tempo), verificado antes do individual porque é o mais restritivo em
    termos de sistema. Os dois são reservados no mesmo ponto (`acquireSlot`) e
    liberados em todo caminho terminal; o erro identifica qual teto barrou o run,
    para a pessoa saber se deve esperar ou cancelar algo próprio.
  - **Visibilidade e cancelamento pela UI**: a listagem de sub-conversas no
    Histórico responde "que conversas existem", não "o que está rodando agora".
    Trabalho em segundo plano precisa de uma superfície própria de *runs*:
    `ListSubAgentRuns` devolve os runs do usuário (ativos primeiro, depois os
    recentes) com a ocupação dos dois tetos, e `CancelSubAgentRun` reusa o mesmo
    `Manager.Cancel` da tool (sem caminho alternativo de cancelamento). Isso não
    reabre a página dedicada removida acima: continua não havendo rota nem
    listagem de *conversas* separada — é um painel de runs dentro do Histórico.
  - **Anúncio de início/fim (AEP-0058)**: o Manager emite `subagent:run-started`
    e `subagent:run-finished` (payload `RunEvent`, sempre com `conversationId`) em
    todo run; o par é garantido porque o início é emitido junto do registro do run
    ativo e o fim no `finalize`, o ponto único de finalização. O frontend anuncia
    só os runs em **background** — o síncrono já aparece no turno do pai, e
    anunciá-lo seria ruído. O anúncio de conclusão é `completion`, então respeita
    a proteção de leitura do conteúdo do assistente da AEP-0058 §2.1.
  - **Histórico decisório**: a versão inicial desta fase entregou uma página dedicada
    (`/subagents`) com colunas operacionais (status, runs, tokens) e um binding próprio
    (`GetSubAgentConversations` → `Manager.ListSubConversations`). Isso foi revisto em
    duas etapas: (1) o enquadramento de sub-agentes como "tarefas em background a
    monitorar" (próximo de Jobs) estava errado — são conversas, e qualquer sub-conversa
    pode ser usada/aberta como conversa comum; a página separada e seu item de menu foram
    removidos em favor da listagem no Histórico; (2) por serem a mesma entidade, manter
    dois métodos de listagem era distinção desnecessária — `GetSubAgentConversations`,
    `Manager.ListSubConversations`, o `ConversationLister` e a agregação de runs para a UI
    (`AggregateByChildConversation`) foram removidos; `GetConversations` passou a ser o
    único caminho de listagem.

Entrega em PRs **empilhados** e mergeáveis em ordem (F1→F5), pois cada fase depende
da anterior.

## Riscos

- **Injeção concorrente no pai** (vários background terminando juntos) → mitigado por
  fila serializada por conversa-pai + idempotência por `run_id`.
- **Loop reativo** (auto-wake re-dispara o pai, que pode delegar de novo) → proveniência
  propagada + circuit-breaker compartilhado com jobs.
- **Crescimento de contexto** em sub-conversas reusadas → sumarização/trim (Fase 3).
- **Detecção de conclusão**: precisa de callback in-process; depender de evento Wails
  (frontend) não funciona em background.
- **Ciclo de vida com app fechado**: runs `running` órfãos → reconciliação no startup
  (Fase 4), como nos jobs.
- **Custo/rate-limit**: trabalho de fundo pode gastar tokens silenciosamente → teto de
  concorrência + visibilidade + rate limiter (Fase 5).
- **Atribuição do aviso**: entregar como conteúdo do assistente é uma extensão do
  fluxo de mensagens; precisa respeitar o contrato do AEP-0040.

## Critérios de aceitação

- O LLM inicia um sub-agente, recebe `run_id`/`conversation_id`, e o sub-agente roda
  numa conversa visível do mesmo usuário.
- `background:false` retorna o resultado inline; `background:true` permite consultar
  status e injeta o aviso de conclusão pelo lado do assistente, com auto-wake.
- Passar um `conversation_id` reabre a sub-conversa preservando o contexto; `clear:true`
  reseta antes de enviar.
- `profile=<slug>` faz o sub-agente rodar com modelo/comportamento do profile indicado.
- Um job consegue chamar `subagent` com `conversation_id` fixo mantendo histórico
  (batch recorrente) ou criando conversa nova por run.
- O sub-agente só cria novos sub-agentes se o profile dele habilitar a tool `subagent`;
  proveniência/circuit-breaker impedem runaway; limites de concorrência e timeout
  respeitados.
- Testes backend (manager, tool, runtime, resume, jobs) e frontend (eventos/UI)
  passando; CI verde a cada fase.

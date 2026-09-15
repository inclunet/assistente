# AEP-0103: Comandos, acionadores e camadas contextuais

**Status:** In Progress

**Data:** 2026-09-12

**Relacionados:** AEP-0001, AEP-0023, AEP-0045, AEP-0046, AEP-0047,
AEP-0048, AEP-0052, AEP-0058, AEP-0060, AEP-0063, AEP-0067, AEP-0074-B,
AEP-0080, AEP-0091, AEP-0101

## Resumo

Esta AEP propõe um sistema único e configurável para executar comandos do
Assistente a partir de teclado local, hotkeys globais, Stream Deck conectado
diretamente por USB/HID, Command Palette, chat, CLI e eventos.

O sistema separa cinco conceitos:

1. **comando**: capacidade identificada por um ID estável;
2. **acionador**: gesto ou evento que solicita um comando;
3. **binding**: associação entre acionador, comando e argumentos;
4. **camada**: conjunto combinável de bindings;
5. **regra de ativação**: condição ou evento que ativa uma camada.

Camadas padrão fornecidas pelo aplicativo permanecem ativas e contribuem com
seus bindings. Camadas adicionais são combinadas sobre elas, sem substituir o
mapa completo. Um binding mais específico pode sobrescrever apenas o mesmo
acionador no contexto em que ambos concorrem. Assim, ativar uma camada de chat
não desabilita comandos padrão como `Ctrl+N`.

O Stream Deck não dependerá do software oficial da Elgato. O backend Go será o
proprietário do dispositivo, receberá seus eventos e renderizará diretamente as
teclas. Imagens e estados físicos serão uma apresentação dos mesmos comandos e
camadas usados pelos demais acionadores.

## Motivação

Os atalhos atuais estão distribuídos entre hooks e componentes do frontend,
constantes locais e um gerenciador separado de hotkeys globais. Esse desenho
torna difícil:

- personalizar atalhos sem alterar código;
- detectar conflitos entre handlers registrados em lugares diferentes;
- preservar corretamente escopo, foco e precedência de diálogos;
- descobrir e executar ações por uma Command Palette;
- configurar as mesmas ações por teclado, chat ou dispositivos físicos;
- reagir à tela, aba, workspace ou programa em primeiro plano;
- expor jobs e tools MCP sem criar um caminho de execução diferente;
- restaurar padrões de forma seletiva;
- oferecer gerenciamento acessível de um Stream Deck sem usar o software
  oficial, que não atende ao fluxo com leitor de telas.

O projeto já possui peças relacionadas, mas ainda independentes:

- a AEP-0001 prevê hotkeys como triggers de jobs;
- a AEP-0063 fornece execução comum para tools nativas e MCP;
- a AEP-0080 define contexto de surfaces;
- `internal/hotkey` registra hotkeys globais;
- o frontend possui handlers locais em hooks e componentes;
- a CLI e os deep links já expõem algumas ações por interfaces alternativas.

Sem um registro canônico de comandos e um resolvedor comum, acrescentar Stream
Deck, Command Palette e configuração por chat multiplicaria fluxos paralelos e
os mesmos problemas de escopo já observados nos atalhos locais.

## Decisões

### D1 — Arquitetura orientada a comandos

O núcleo seguirá o fluxo:

```text
Acionador → resolvedor de bindings e camadas → registro de comandos → executor
```

O teclado não é o núcleo do sistema. Ele é um adapter de entrada, assim como
Stream Deck, Command Palette, chat e CLI.

O nome conceitual do subsistema será **Gerenciador de Comandos e Acionadores**.
O gerenciador de teclado atual será integrado gradualmente como adapter, e não
expandido para conhecer layouts físicos, regras de janela ou execução de tools.

### D2 — Registro canônico de comandos

Todo comando terá:

- ID estável e namespaced, como `workspace.tab.new`, `chat.model.select`,
  `job.run` ou `layer.toggle`;
- nome, descrição, categoria e aliases versionados nos três locales;
- schema tipado de argumentos;
- escopos e contextos em que pode executar;
- `context_policy` com fatos obrigatórios e regra de staleness;
- `allowed_source_types`, usando exatamente a taxonomia de D3, inclusive a
  origem interna reservada `system`;
- paths sensíveis de input/output e política de persistência;
- classificação de risco;
- `effect_class`, com `read`, `write` ou `destructive`;
- `decision_requirement`, incluindo `none` ou diálogo interativo;
- `mutates_effective_capability`, derivado do contrato do handler;
- estado de disponibilidade e motivo quando indisponível;
- apresentação padrão opcional, incluindo ícone e estados;
- handler ou rota de execução.

IDs apresentados pelo catálogo são canônicos. Chat, importação e UI não podem
inventar IDs nem executar handlers por nome aproximado.

Aliases pertencem ao registro, em mapa versionado `locale → string[]`, e passam
pela mesma normalização de busca da Command Palette. A UI não mantém listas
paralelas.

`context_policy` lista fatos por provider e modo: `exact_version`, `max_age_ms`,
`event_snapshot` para captura confiável no instante do acionamento, ou `none`
somente para leitura que não depende de alvo atual. `event_snapshot` exige
`max_age_ms > 0` e timestamp autenticado do provider.
`internal/commandcontext.VersionService` registra providers e monta
`context_version` como fingerprint dos pares `(fact_name, fact_version)`.
Providers iniciais:

- surface: consulta `surfaceId`/`snapshotVersion` pelo contrato da AEP-0080;
- diálogo: geração do stack topmost;
- foco/controle e janela/processo: geração dos adapters de UI/SO;
- workspace/aba: versão do store canônico;
- camadas ativas: `active_layers_generation` do resolvedor;
- job: `run_id` + último `job_run_events.sequence`;
- sessão/lock: epochs do `EpochService`.

Na revalidação, cada provider compara a versão atual ou a idade exigida pelo
comando. Provider ausente, versão incomparável ou TTL vencido torna o comando
indisponível/falha fechado; não há heurística comum aplicada a contextos
diferentes.

O registro rejeita `context_policy = none` quando `effect_class` não for
`read`, quando houver alvo mutável ou quando o comando alterar capacidade
efetiva. `CommandExecutionService` revalida essa invariância; metadata de comando
não pode optar por escapar de staleness.

`effect_class = destructive` exige `decision_requirement = interactive`,
segue AEP-0091 e exige
`allowed_source_types ∩ {cli, event, system} = ∅`, mesmo quando a coleção
contém outras origens. O registro rejeita origens inerentemente headless e o
executor rejeita qualquer chamada sem um `DecisionPresenter` interativo
autenticado. Combinação
`destructive + none` é inválida no registro e recusada novamente pelo executor.

`DecisionPresenter` é uma porta interna injetada pelo bootstrap, nunca um campo
ou capability declarada pelo chamador. Na v1, somente a sessão Wails local
registra uma implementação; ela envia `kind: decision` ao
`questionnaire.Manager` e o frontend apenas renderiza o `DecisionDialog` da
AEP-0091. A porta recebe `DecisionRequest` com `decision_id` UUIDv7,
`invocation_id` ou ID da mutação de configuração, usuário/sessão, fingerprint
da solicitação, gerações de autenticação/segurança, ações permitidas e
`expires_at`. O `DecisionDialog` continua produzindo o corpo da AEP-0091:
`{ actionId }` ou `{ cancelled: true }`. O host confiável anexa o
`decision_id` da solicitação backend à correlação, resultando em
`{ decision_id, actionId }` ou `{ decision_id, cancelled: true }` para a porta;
ESC, fechamento e cancelamento explícito fazem o CAS para `cancelled`
imediatamente, sem aguardar expiração.

`DecisionReceiptService` valida a resposta contra a solicitação criada no
backend e faz CAS único de `pending` para `accepted`, `denied`, `cancelled` ou
`expired`; resposta duplicada ou ação fora do conjunto falha fechado. Para
despacho destrutivo, o `CommandExecutionService` exige receipt `accepted`
correspondente a usuário, sessão, invocação, fingerprint, ação afirmativa e
gerações atuais. Sob o `DispatchGate`, consome a receipt e efetiva o CAS de
`evaluating` para `queued` na mesma transação; replay, receipt já
consumida ou geração alterada não executa. `authorization_decision_id` é o
`decision_id` consumido.
Mutação confirmável de configuração usa o mesmo contrato com
`subject_type = config_mutation` e consome a receipt ao gravar a mudança.
Origem sem presenter registrado nunca cria receipt afirmativa.

Comandos de UI podem ser executados no frontend por uma ponte tipada. Comandos
de backend são enviados ao serviço correspondente. Jobs usam o runtime de jobs;
tools internas e MCP usam o executor comum da AEP-0063. O registro de comandos
não duplica o `tool_catalog`: ele pode expor um comando parametrizado que delega
a uma entrada existente do catálogo.

### D2.1 — Envelope e ponto único de execução

Nenhum adapter chama diretamente o handler final. Toda solicitação converge
para `CommandExecutionService`, inclusive comandos de UI. O serviço recebe um
envelope versionado:

```text
CommandInvocation
  version, invocation_id, command_id?, arguments?
  observed_trigger_type?, trigger_type?, trigger_spec?
  user_id?, auth_context_type, auth_context_id, auth_generation
  session_id?, security_generation
  actor_type, actor_id
  source_type?, observer_type?, source_instance_id?, source_event_id?
  source_occurred_at?, source_replay_policy_generation?
  source_replay_deadline?
  workspace_id?, binding_ids?, registry_version
  global_config_generation?, workspace_config_generation?
  active_layers_generation?
  foreground_snapshot?
  conversation_id?, turn_id?, surface_type?, surface_id?
  surface_snapshot_version?, context_version?
  context_captured_at_by_provider?
  source_profile_slug?, target_profile_slug?
  authorization_decision_id?, delegation_fingerprint?, grant_generation?
  job_id?, job_slug?, job_definition_fingerprint?, run_id?
  provenance?, correlation_id, request_fingerprint_version?
  request_fingerprint?, client_requested_at?, received_at
```

`source_type` distingue teclado local/global, Stream Deck, palette, chat, CLI,
evento e sistema. `source_instance_id` é um UUIDv7 novo para cada abertura,
reconexão ou geração física do adapter; a identidade estável do dispositivo
permanece em `trigger_spec`. `source_event_id` identifica uma ocorrência única
de adapter físico/de evento e é UUIDv7 gerado pela borda confiável. Execuções
diretas o omitem. Contador/ID nativo do protocolo pode ficar em metadata
redigida, mas não substitui a identidade canônica.
`actor_type` distingue usuário, agente e automação.

`source_type` é a origem lógica vencedora que governa binding e
`allowed_source_types`; `observer_type` e `observed_trigger_type` registram quem
observou o evento físico. Em solicitação por trigger, `source_type` fica nulo até
a resolução escolher o candidato; em execução direta, é fixado na borda.

`actor_type` e `actor_id` são derivados no backend do principal e da origem
autenticados; valores recebidos de adapter/cliente são ignorados e divergência
falha fechado. O mesmo vale para `user_id`. O payload nunca escolhe a identidade
que será usada por autorização ou auditoria.

`auth_context_*`, `source_type`, `observer_type`, `source_instance_id` e
`source_event_id` também são derivados ou validados pela borda autenticada e
pelo registro do adapter. Cada endpoint fixa as origens que pode produzir;
cliente não promove a si próprio a `system`, hotkey ou dispositivo.
`trigger_spec` é normalizado pelo dispatcher e validado contra a capacidade do
adapter antes de chegar ao resolvedor.

`context_captured_at_by_provider` é mapa `provider_id → RFC3339 com timezone`
e preserva separadamente o instante informado por cada provider confiável. O
ingresso nunca substitui timestamp ausente por `received_at`, pois isso faria
snapshot antigo parecer novo; valor de cliente não confiável é ignorado.
Política `max_age_ms` ou `event_snapshot` exige entrada para cada provider e
falha fechado quando alguma faltar. `exact_version` pode omitir a entrada
daquele provider, pois reconsulta sincronamente a versão autoritativa e exige
igualdade antes do despacho. `none` não declara provider e omite o mapa e
`context_version`. Versões/epochs monotônicos ficam em `context_version`, não
são serializados como timestamp.

Invocação com usuário exige `global_config_generation` e
`active_layers_generation`; workspace exige também
`workspace_config_generation`. `system` sem usuário omite as três, pois não
acessa bindings/camadas. Policy que declara provider exige `context_version`;
`none` o omite. Não há string vazia, zero ou sentinel para esses casos.

Adapters físicos e de evento exigem `source_instance_id` e `source_event_id`.
Adapter de evento exige ainda `source_occurred_at`, obtido do timestamp
autenticado e persistido do produtor; adapter físico o omite. Palette,
`ui.action`, chat, CLI e `system` omitem os três e deduplicam pela PK
`invocation_id`. A borda Wails cria o UUIDv7 de cada clique/formulário antes do
serviço. Após resolução, `binding_ids` é sempre materializado como lista, ainda
que vazia.
`source_replay_policy_generation` e `source_replay_deadline` são derivados
internamente depois de validar a fonte; valor recebido de cliente é ignorado.

Na primeira tentativa, IDs são gerados em borda confiável: Wails para
palette/UI, contexto persistido da tool call para chat, processo backend para
`system`, serviço CLI para terminal e cada adapter de teclado, Stream Deck ou
evento para sua ocorrência. Todos geram `invocation_id` UUIDv7. Somente
adapters físicos/de evento também
geram `source_event_id`, usado para deduplicar a ocorrência; palette, UI, chat,
CLI e `system` o omitem. A CLI imprime/devolve o ID e aceita `--request-id`
apenas em retry autenticado; chat reutiliza o ID associado ao mesmo tool call.
Valor reapresentado nunca troca ownership e sempre passa pelo fingerprint/ledger.

No envelope de ingresso, `request_fingerprint_version` e
`request_fingerprint` ficam ausentes: adapters não os calculam nem podem
fornecê-los. `authorization_decision_id` também fica ausente e só é preenchido
pelo `DecisionReceiptService` após resposta válida. Depois da normalização e,
para trigger, após fixar o candidato vencedor, o backend calcula o HMAC por
JSON Canonicalization Scheme (RFC 8785).
Ambos se tornam obrigatórios antes da reserva no ledger. O fingerprint inclui
schema, comando ou trigger, argumentos, IDs/versões de contexto, usuário/ator
derivados, tipo/ID do contexto autenticado, origem/observador, workspace,
`source_event_id`, `source_occurred_at`, geração e deadline de replay quando
presentes;
`source_instance_id` somente quando não houver `source_event_id`; versões de
catálogo/configuração, profiles de origem/destino,
política/requisito de decisão, `delegation_fingerprint`, `grant_generation`,
`job_id`, `job_slug`, `job_definition_fingerprint`, `run_id` e proveniência.
Exclui token bruto, `authorization_decision_id`, `auth_generation` rotativa e
timestamps. O ID da decisão é resultado da interação, não entrada semântica da
solicitação; a receipt o vincula ao `invocation_id` e ao fingerprint já
calculado. É calculado no backend e persistido
sem revelar segredos. Reentrega com o mesmo `invocation_id` só é aceita se o
fingerprint for idêntico; divergência é conflito e falha fechado.

A chave `command-request-hmac:v1` vive no secret manager e permanece disponível
por pelo menos a maior expiração dos ledgers. O ledger guarda
`request_fingerprint_version`. Rotação cria versão nova para requests novos e
mantém chaves antigas até seus ledgers expirarem. Chave esperada indisponível
faz a reentrega falhar fechado, sem executar novamente.

`provenance` é um documento versionado e redigido com `_source`,
`_source_job_id`, `_chain_id` e `_chain_history` da AEP-0067, além de
`command_chain_history` separado, quando a solicitação vier de cadeia reativa.
O dispatcher o copia sem reconstruir por heurística.

Uma solicitação informa `command_id` para execução direta ou
`trigger_type`/`trigger_spec` para resolução de binding. Depois da resolução,
solicitação por trigger contém também o `command_id` vencedor; solicitação
direta mantém `trigger_*` nulo. A única exceção é a resolução terminal
`effect = suppress` da D4: ela preserva `command_id` nulo, registra o marcador
`suppressed` no ledger e não cria `CommandInvocation`. Fora dessa exceção,
ausência do campo exigido por cada modalidade ou combinação incoerente falha
antes de qualquer efeito.

Se qualquer um entre `surface_type`, `surface_id` e
`surface_snapshot_version` estiver presente, os três tornam-se obrigatórios e
não vazios. Comando direcionado a surface sem o trio completo falha fechado,
conforme AEP-0080.

Na borda, o mapeamento é único e explícito:
`SurfaceContext.surfaceType → surface_type`, `surfaceId → surface_id` e
`snapshotVersion → surface_snapshot_version`. O envelope e o SQLite usam
snake_case; `context_version` é o fingerprint composto do `VersionService`, não
um alias de `snapshotVersion`.
Esses três valores recebidos são candidatos, não autoridade. O backend localiza
a surface no registro canônico do usuário/contexto autenticado, reconsulta o
snapshot provider da AEP-0080 e exige igualdade de tipo, ID e versão antes de
materializá-los no envelope interno. Surface ausente, pertencente a outro
usuário ou divergente falha fechado; cliente não atesta a própria atualidade.

O serviço, nessa ordem:

1. autentica o principal e valida usuário/proveniência mínima;
2. normaliza e resolve o binding/candidato num snapshot sem efeitos, capturando
   versões e bindings contribuintes; falha de resolução também produz resultado
   determinístico;
3. calcula o fingerprint canônico da solicitação resolvida ou recusada;
4. para `suppress`, adquire `DispatchGate`, revalida autenticação, segurança,
   staleness e gerações e só então reserva o ledger terminal enquanto mantém o
   gate; se stale, não grava `suppressed`, mas reserva uma recusa terminal
   `rejected_stale` para os mesmos IDs/ownership e registra o evento no log de
   segurança. Tanto `suppressed` quanto `rejected_stale` encerram o
   processamento sem seguir aos passos seguintes e sem criar
   `CommandInvocation`. Nos demais casos, reserva atomicamente ledger e
   auditoria como `evaluating`;
5. se a resolução falhou, conclui `denied`; caso contrário valida origem
   permitida, disponibilidade, argumentos, contexto e política;
   falhas após autenticação terminam a tentativa como `denied`. Quando a
   política exige interação, obtém uma receipt pelo presenter interno antes de
   continuar; não mantém `DispatchGate` aberto enquanto aguarda o usuário;
6. revalida contexto de autenticação, geração de segurança, staleness e
   autorização imediatamente antes do despacho, incluindo e consumindo a
   receipt no CAS para `queued`; cancela a invocação se qualquer gate estiver
   obsoleto;
7. ao retirar da fila, revalida novamente os mesmos gates e faz CAS atômico de
   `queued` para `running`; se o contexto mudou, grava `cancelled_stale` sem
   chamar o handler. Também compara `registry_version`,
   `global_config_generation` e `workspace_config_generation` atuais; mudança de
   comando, camada, binding ou prioridade cancela como stale. Compara também
   `active_layers_generation`; claim ativada/desativada desde a resolução
   cancela a invocação. Se a transição vencer, encaminha ao handler;
8. conclui a trilha como `succeeded`, `failed`, `cancelled`, `timed_out` ou
   `outcome_unknown`; `denied` e `cancelled_stale` já encerram nos gates
   anteriores.

`registry_version` identifica o catálogo/defaults carregado.
Cada usuário possui `global_config_generation`; cada workspace possui
`workspace_config_generation`. Mutação global incrementa a primeira e invalida
invocações em todos os workspaces; mutação local incrementa somente a segunda.
A invocação captura `workspace_id` e o par de gerações na resolução. Sem
workspace, captura apenas a global.

O resolvedor mantém contador global por usuário e contador por
usuário+workspace. `active_layers_generation` é fingerprint composto somente
dos dois contadores aplicáveis ao `user_id`/`workspace_id` da invocação.
Mudança de claim global incrementa o contador do usuário e afeta seus
workspaces; mudança local incrementa apenas o workspace. `layer.back`,
expiração, pin e eventos seguem o mesmo escopo. Mudança de outra conta ou
workspace não cancela a invocação.

`internal/commandsecurity.DispatchGate` serializa admissão com mudanças de
segurança/configuração. Logout, lock, troca de principal e mutações de mapa
adquirem o gate exclusivo antes de incrementar gerações. O worker adquire o gate
compartilhado, revalida, faz CAS para `running` e entra sincronamente em
`handler.Start(ctx)` antes de liberar o gate. `Start` é obrigatoriamente
não bloqueante: apenas aceita o handoff, enfileira/inicia o worker e devolve um
handle/ack de início; nunca espera job, tool ou UI terminar e não readquire o
gate. Adapter de API síncrona precisa envolvê-la em fila/goroutine controlada
antes de anunciar suporte; caso contrário o comando fica indisponível. Assim
não existe janela entre CAS e início, nem lock mantido durante trabalho longo.
Invalidação posterior cancela o contexto quando suportado; efeito já admitido
não é retroativamente desfeito. Testes cobrem ack limitado, logout concorrente e
ausência de deadlock.

`Start` devolve `ExecutionHandle` com ID, canal/future `Done` e `Cancel`.
`CommandExecutionService` aguarda fora do gate e faz CAS terminal único para
`succeeded`, `failed`, `cancelled`, `timed_out` ou `outcome_unknown`,
atualizando auditoria e ledger na mesma transação. Antes de entrar no handler,
erro/panic comprovadamente sem
handoff vira `failed`; depois de entrar em `Start`, panic sem rejeição
conclusiva, canal fechado sem outcome e perda do prazo sem cancelamento
confirmado viram `outcome_unknown`, pois o efeito pode ter ocorrido. `failed`
pós-handoff exige outcome explícito do `ExecutionHandle`. Outcome tardio após
terminal é ignorado. Handler síncrono curto usa handle já concluído.

`timed_out` só vale antes do handoff ou quando o handler confirma cancelamento
sem efeito. Depois do ack, prazo vencido, canal perdido ou cancelamento não
confirmado vira `outcome_unknown`, nunca `timed_out`; retry automático é
proibido e reconciliação explícita consulta o executor/recurso antes de nova
invocação. Outcome tardio verificável pode reconciliar
`outcome_unknown → succeeded|failed` por CAS auditado; não dispara novo efeito.

A reserva de execução é uma transação que cria a chave no ledger de
idempotência e a linha de auditoria `evaluating` antes do handler; supressão
segue a exceção terminal sem auditoria definida em D4. O ledger usa PK `id`, `key` UNIQUE,
`invocation_id` UNIQUE e índice parcial UNIQUE de `source_event_id` quando
presente. Replay com usuário/origem/fingerprint divergente é conflito, não nova
execução. Reentrega recebe o resultado
existente ou falha como
duplicada, nunca chama novamente o handler. Palette, chat e CLI fornecem um
`invocation_id` UUIDv7 idempotente por solicitação. Eventos físicos recebem
`source_event_id` no único adapter que os possui. Para eles, a garantia de
deduplicação é limitada à sessão física ou até `expires_at` do ledger; depois
disso, nova observação física é nova solicitação.

Evento durável não segue essa regra curta. Na v1, o adapter relê
`command_job_activation_outbox` por PK/ownership, exige igualdade de
`source_event_id` e `source_occurred_at`. A linha nasce na mesma transação do
`job_run_events` de origem e não possui cascade com `job_runs`.
`command_event_replay_policy_epochs` registra cada
mudança de `maintenance.job_retention_hours` com geração, `effective_at` e
horizonte; o evento usa o epoch vigente em `source_occurred_at` para calcular
um `source_replay_deadline` imutável. O ledger recebe `expires_at` nunca
anterior a esse deadline.

Antes de remover um ledger de evento, a manutenção exige que o deadline tenha
vencido. Se não houver ledger, evento após seu `source_replay_deadline` é
rejeitado como `rejected_stale` antes da resolução, nunca aceito como novo.
Aumentar retenção cria epoch apenas para ocorrências seguintes e não reabre
eventos cujo deadline anterior venceu; diminuir também não encurta ledgers já
reservados. Não se infere idade do UUIDv7.

`received_at` é sempre atribuído pelo backend ao receber o envelope e governa
retenção/idade das origens não duráveis. Para evento durável,
`source_occurred_at` autenticado governa a admissibilidade e o piso de retenção
do ledger. `client_requested_at`, quando fornecido, é apenas metadado validado
e nunca altera expiração, ordenação de segurança ou caps.

Reentrega consulta primeiro o ledger pelo ID e recebe status, `result_summary`
e `result_ref` redigidos por `CommandExecutionService.GetInvocation`; não
repete o handler mesmo se a auditoria detalhada já tiver sido compactada. A
consulta exige o mesmo contexto autenticado, filtra por
`(user_id, invocation_id)` e reaplica autorização do ator; buscar somente pela
PK é proibido. A API pública não consulta invocações `system`. No startup, um
método interno e não exposto, `ReconcileSystemInvocations`, opera sob capability
privilegiada da instância e seleciona apenas `user_id IS NULL AND
auth_context_type = 'system'` de epochs anteriores nos estados recuperáveis;
não exige que `auth_context_id` antigo seja igual ao epoch atual e não aceita ID
fornecido externamente. Assim,
registros `evaluating`, `queued` ou `running` de uma geração encerrada viram
`outcome_unknown` em `command_invocations` e
`command_idempotency_keys` na mesma transação, nunca são reexecutados
automaticamente. O usuário ou fluxo
chamador precisa consultar o efeito e criar uma nova invocação explícita. Esse
tratamento reconhece que exatamente-uma-vez não é garantível para todo handler
de UI ou sistema após queda entre efeito e commit.

Recusas anteriores à autenticação — sem usuário confiável ao qual associar a
linha — vão para o log de segurança, não para uma conta indicada pelo payload.
Toda recusa posterior é persistida como `denied`, com código redigido.

`allowed_source_types` usa os valores exatos `keyboard.local`,
`keyboard.global`, `streamdeck.key`, `palette`, `ui.action`, `chat`, `cli`,
`event` e `system`. `actor_type` é ortogonal: por exemplo, uma tool chamada pelo agente
tem `source_type = chat` e `actor_type = agent`. Não existe categoria implícita
`desktop`; cada comando declara explicitamente quais entradas aceita.

**Override pendente da AEP-0052:** a D6 daquela AEP continua canônica hoje e
define `JWT sub = user_id`. A implementação parcial desta AEP não a substitui nem
autoriza interpretação concorrente. O mapa `(iss, sub) → users.id` abaixo é o
contrato alvo proposto; `CommandExecutionService` permanece indisponível em
`auth.mode=external` até um PR de implementação atualizar a AEP-0052 e o
middleware no mesmo ciclo, migrar identidades e registrar evidências em ambas
as AEPs. APIs existentes seguem exclusivamente a AEP-0052 até essa migração.

Os contextos de autenticação são:

- `local_session`: o `SessionService` da AEP-0052 fornece somente `user_id` e
  `session_id`; o `EpochService` desta AEP fornece as gerações. Desktop e CLI
  recebem esses dados do backend; `auth_context_id = session_id` e
  `auth_generation` é mantida por esse session ID, não por usuário; IDs vindos
  como argumentos são ignorados;
- `external_token`, somente depois do override acima: JWT validado fornece `sub`, scopes e um
  `auth_context_id` derivado de `iss` + `sub` + `jti` ou fingerprint do token;
  `(iss, sub)` sempre precisa resolver por mapeamento administrativo explícito.
  `sub` isolado nunca é aceito como `users.id`, pois não é global entre issuers.
  Não há provisionamento automático nem fallback para usuário atual;
  ausência/ambiguidade falha fechado. Antes de habilitar comandos nesse modo, o
  upgrade exige que o administrador migre identidades usadas pelo middleware
  vigente para `external_identity_mappings`. JWT/scopes são revalidados;
- `job_service`: automação usa o usuário proprietário, ID e versão persistida do
  job, representada por `job_definition_fingerprint`, além dos grants exatos
  aplicáveis; o gate final relê a definição e compara o fingerprint. Não pode
  abrir diálogo nem executar comando que exija interação;
- evento externo: só vira um dos contextos acima após autenticação do ingress e
  mapeamento inequívoco para usuário; caso contrário falha fechado;
- `system`: contexto interno criado pelo processo, sem `user_id`, restrito a
  comandos explicitamente seguros que não acessam dados ou bindings de usuário.

Hotkeys e Stream Deck exigem um contexto autenticado atualmente ativo. Contexto
`system` sem usuário só executa comandos internos que declarem essa origem e não
acessa bindings ou dados de usuário.
Em `auth.mode=external`, teclado físico global e Stream Deck ficam
indisponíveis: não existe sessão desktop autoritativa à qual vincular o evento.
Palette/UI/chat continuam usando o principal do JWT da própria requisição.
Habilitar adapters físicos nesse modo exige AEP posterior para um broker local
que vincule e revogue explicitamente um principal externo ativo; “último token”
ou usuário inferido nunca é aceito.

Bootstrap do modo externo é pré-requisito explícito: endpoint administrativo
fora do command manager, protegido por issuer configurado + scope admin, cria
`(iss, sub) → users.id` para usuário local existente. O primeiro admin só pode
vincular o próprio token legado quando `sub` já coincide com esse `users.id`;
demais vínculos são escolhas explícitas auditadas. Concluído o lote, uma flag de
readiness faz o middleware resolver todos os principals pelo mapa. Até então,
CommandExecutionService fica desabilitado em `auth.mode=external`. Não há JIT.

As gerações não são atribuídas à AEP-0052.
`internal/commandsecurity.EpochService`, definido aqui, cria um epoch aleatório
novo no startup, mantém `auth_generation` por contexto autenticado e uma
`security_generation` do processo. Logout, revogação observada, troca de usuário
e substituição do principal invalidam o epoch de autenticação; lock/unlock e
mudança de principal incrementam o epoch de segurança. O serviço consulta a
validade real da sessão/JWT/job na revalidação, portanto o epoch é proteção
adicional contra trabalho obsoleto, não uma nova autoridade de autenticação.
Para `system`, o epoch aleatório do processo preenche `auth_context_id` e
`auth_generation`; restart o invalida e o gate continua limitado aos handlers
puros definidos acima.

Comandos que delegam para tools passam então pelo executor da AEP-0063 e
correlacionam `command_invocations.invocation_id` com `tool_invocations`. Jobs passam pelo
runtime de jobs; ações de frontend recebem da ponte somente um despacho já
autorizado, vinculado ao `invocation_id`. Command Palette, chat e CLI podem
selecionar diretamente um `command_id`, mas não ignoram validação, autorização
ou auditoria.

Contexto `system` sem `user_id` não pode delegar para tool, MCP ou job, pois
esses contratos exigem proprietário. Ele fica restrito a handlers puros e
internos sem dados de usuário; tentativa diferente falha fechado.

Argumentos sensíveis são redigidos ou resumidos na auditoria conforme a política
do comando. Erro, status, origem, ator, comando e correlação permanecem
diagnosticáveis.

Se o comando delegar a uma tool, paths sensíveis do registro são propagados ao
`ToolInvocationService`: o valor bruto existe somente em memória para execução,
enquanto input/output persistidos usam placeholders e fingerprint do conteúdo
redigido. Essa extensão deve atualizar a AEP-0063 no mesmo PR que a implementar.
Até esse suporte existir, comando com paths sensíveis é indisponível para
delegação a tools e falha fechado.

Quando `actor_type = agent`, o envelope preserva conversa, turno,
`surface_type`, `surface_id` e `surface_snapshot_version`; esses campos vêm dos
nomes camelCase do `SurfaceContext` pelo mapeamento de borda já definido, além
dos profiles de origem e destino. `CommandExecutionService` não trata o agente como
o usuário autenticado: delegação cross-profile interativa exige
`DecisionDialog` e registra `authorization_decision_id`; origem sem interlocutor
falha fechado.

Jobs cross-profile transportam `job_id`, `target_profile_slug`,
`delegation_fingerprint` e `grant_generation`. O serviço relê o grant pela chave
`(user_id, job_id, target_profile_slug, delegation_fingerprint)` e pela geração
exata da AEP-0101 imediatamente antes do handler. `job_id` é UUID; o profile
alvo permanece identificado pelo slug canônico, como exige aquela AEP. O
envelope transporta a decisão, mas não cria nem amplia grants.

### D3 — Acionadores são adapters, não comandos

Tipos iniciais de acionador:

- `keyboard.local`: combinação recebida dentro da janela do Assistente;
- `keyboard.global`: hotkey registrada no sistema operacional;
- `streamdeck.key`: tecla física, incluindo dispositivo e posição;
- `palette`: escolha na Command Palette;
- `ui.action`: clique, formulário ou ação direta da UI autenticada;
- `chat`: execução estruturada solicitada pelo agente;
- `cli`: execução solicitada pelo entrypoint de terminal;
- `event`: na primeira versão, somente fato de `job_run_events` espelhado
  transacionalmente em `command_job_activation_outbox`;
  outros produtores exigem outbox antes de entrar na taxonomia operacional.

`system` é uma origem interna reservada para execução direta pelo processo. Não
é acionador configurável, não aparece em bindings do usuário e obedece às
restrições sem proprietário da D2.1. Assim, a taxonomia completa de
`allowed_source_types` é a lista acima mais essa origem interna explícita.

Adapters normalizam a entrada para uma identidade de acionador e nunca executam
diretamente a ação final. Uma entrada física gera no máximo uma execução, mesmo
quando mais de um observador puder enxergá-la.

Cada adapter físico normaliza primeiro uma transição `up → down` e só depois
gera `source_event_id`. Em `keyboard.local`, `keydown` com
`KeyboardEvent.repeat = true` é descartado e `keyup` libera a combinação; perda
de foco, blur ou troca de geração limpa o estado pressionado sem disparar ação.
O adapter global solicita a opção nativa de no-repeat quando disponível e
mantém a mesma máquina de estado de pressão/liberação; plataforma que não
consiga garantir essa borda não anuncia suporte ao binding global. Stream Deck
deduplica callbacks até o release correspondente. Testes mantêm a tecla
pressionada, simulam repeat/reconexão e provam uma única ocorrência.

Eventos de teclado têm ownership exclusivo. Uma combinação registrada como
`keyboard.global` pertence ao adapter do sistema operacional inclusive quando o
Assistente está em foco; o adapter DOM recebe a lista correspondente e não emite
`keyboard.local` para ela. No ingresso, o adapter global preenche somente
`observer_type`/`observed_trigger_type = keyboard.global`; após a resolução,
`source_type`/`trigger_type` também ficam `keyboard.global` em qualquer foco.
Binding local da mesma combinação fica
marcado como shadowed/conflitante e não participa; para variar a ação dentro do
Assistente, o binding global usa condições/camadas de foco e surface. O adapter
local possui somente combinações não registradas globalmente. Alterações de
registro são aplicadas por geração antes de publicar o novo mapa. Stream Deck
possui um único listener por dispositivo. Essa exclusão evita dupla execução e
define uma única observação física.

Pressão normal, pressão longa, alternância e dial podem ser acrescentados como
gestos normalizados quando o dispositivo oferecer esses sinais. Pressão longa
exige máquina de estado própria que emita exatamente um gesto normal ou longo
por ciclo físico, nunca reaproveita os `keydown.repeat` descartados. Capacidade
não detectada não deve ser simulada de forma ambígua.

### D4 — Bindings associam acionadores a comandos

Um binding contém:

- camada;
- tipo e especificação normalizada do acionador;
- `command_id` para `execute`;
- argumentos validados pelo schema do comando para `execute`;
- condição tipada opcional;
- `resolution_priority`;
- `effect`, com `execute` ou `suppress`;
- `replaces_default_id`, `replaces_default_version` e
  `replaces_default_fingerprint` para delta de default;
- `review_status`, com `active` ou `needs_review`;
- estado habilitado/desabilitado;
- origem: padrão do aplicativo ou personalização do usuário;
- metadados de apresentação específicos do acionador.

`effect = suppress` exige os três campos `replaces_default_*`, mantém
`command_id` nulo e argumentos vazios; o default alvo é a única autoridade.
Override executável de default também exige o trio; binding inteiramente novo os
mantém nulos. `needs_review` sempre bloqueia delta e default, sem fallback.

O mesmo comando pode ter vários bindings. O mesmo acionador pode aparecer em
várias camadas. Reutilização não é conflito enquanto as condições ou camadas
não puderem estar ativas simultaneamente.

Quando candidatos simultâneos possuem o mesmo `command_id`, argumentos
normalizados e escopo de execução, o resolvedor os deduplica e produz uma única
invocação, preservando a proveniência de todos os bindings equivalentes.
Diferença de comando, argumentos ou escopo continua sendo conflito e falha
fechado se a precedência não escolher um único vencedor.

Bindings `effect = suppress` são aplicados antes dessa deduplicação. Eles
removem os defaults referenciados no contexto, consomem o acionador quando não
restar candidato e nunca criam `CommandInvocation`. Antes de retornar, o
serviço reserva `command_idempotency_keys` em estado terminal `suppressed`, com
`invocation_id`/`source_event_id`, ownership, fingerprint do request resolvido,
gerações e `expires_at`; não cria linha de auditoria de comando. Reentrega
encontra essa chave antes de qualquer despacho e retorna `suppressed` ou
conflito de fingerprint, mesmo se configuração posterior remover o tombstone.
Somente bindings `effect = execute` participam do agrupamento por
comando/argumentos.

Todo delta é materializado antes da tupla de D7: override substitui o candidato
default referenciado e herda seu `scope_rank`/especificidade base; condição do
usuário só pode estreitar o contexto. Tombstone remove o candidato. Portanto
delta nunca compete nem perde para o próprio default; vínculo sem default válido
vira `needs_review`.

### D5 — Camadas são conjuntos aditivos

Camadas agrupam bindings de qualquer tipo de acionador. Elas não são exclusivas
do Stream Deck e não representam cópias completas de um mapa de teclas.

Exemplo:

```text
Camada Chat
  Ctrl+M             → chat.model.select
  Ctrl+Tab           → workspace.tab.next
  Stream Deck tecla 1 → workspace.tab.previous
  Stream Deck tecla 2 → workspace.tab.next
```

Várias camadas podem estar ativas ao mesmo tempo. O mapa efetivo é uma
composição:

```text
Padrões do aplicativo
+ padrão da surface ativa
+ camadas ativadas pelo usuário
+ camada contextual do programa em foco
+ camada temporária de uma operação
```

Uma camada contribui apenas com os bindings que declara. Binding ausente cai
para a próxima camada aplicável; não significa "sem comando".

`enabled = false` apenas desliga uma personalização e, portanto, permite
fallback. Para desabilitar deliberadamente um default, o sistema cria um
binding `effect = suppress` que referencia o binding padrão e bloqueia o
fallback somente no contexto declarado. Esse tombstone participa da mesma
precedência dos bindings executáveis e pode ser restaurado.

### D6 — Estrutura padrão é permanente e versionada

O aplicativo fornecerá camadas padrão, no mínimo:

- aplicativo;
- workspace;
- chat;
- editor;
- tasklists;
- terminal;
- diálogos e decisões.

Elas são versionadas no código e ativadas automaticamente por seu contexto.
Personalizações ficam no banco como deltas. O usuário pode:

- substituir um binding padrão ou criar tombstone explícito para desabilitá-lo;
- restaurar um binding;
- restaurar uma camada;
- restaurar todas as personalizações.

Ativar uma camada adicional nunca desabilita implicitamente uma camada padrão.
Uma atualização pode adicionar novos defaults sem regravar nem apagar
personalizações existentes. Todo override ou tombstone de default armazena
`replaces_default_id` e `replaces_default_version`, permitindo detectar se o
default mudou desde a personalização.

No upgrade, se o fingerprint semântico do default referenciado não mudou, o
sistema apenas avança `replaces_default_version`. Se comando, trigger,
argumentos, escopo ou risco mudou, o delta vira `needs_review`: ele e o novo
default ficam bloqueados naquele acionador/contexto, sem fallback, até decisão
do usuário. Restaurar remove o delta e adota o novo default; rebase confirmado
atualiza versão/fingerprint. A UI anuncia e lista pendências.

O fingerprint usa RFC 8785 sobre toda semântica executável: comando, argumentos,
trigger, condição, effect, escopo, prioridades, origens permitidas,
`context_policy`, `effect_class`, risco, `decision_requirement`,
`mutates_effective_capability` e requisitos do adapter. Somente apresentação
puramente visual fica fora. Qualquer mudança desse conjunto exige review.

Atalhos obrigatórios da AEP-0091, incluindo `Ctrl+Shift+R` para repetir a
pergunta do `DecisionDialog`, são invariantes não suprimíveis. O usuário pode
adicionar uma alternativa, mas não remover a rota exigida pelo contrato. Outros
atalhos essenciais de acessibilidade só admitem substituição quando o contrato
correspondente permitir e houver alternativa equivalente validada.

### D7 — Resolução determinística de conflitos

Condições de bindings e de ativação usam predicados tipados; JavaScript, Go
templates e expressões de shell livres não são aceitos. Essa regra não altera
`when` e `emit_when` internos de jobs da AEP-0001: o binding de hotkey apenas
solicita o trigger identificado ao runtime de jobs, que continua avaliando seus
templates e políticas. Converter essas expressões de jobs exige decisão
separada e não faz parte desta AEP.

Contextos previstos incluem:

- diálogo no topo;
- controle focado e capacidade do controle;
- surface e instância ativas;
- aba e workspace ativos;
- janela do Assistente focada ou desfocada;
- processo e identidade da janela em primeiro plano;
- perfil e modo operacional;
- estado de job ou recurso;
- dispositivo de origem.

A precedência conceitual é:

1. diálogo ou decisão bloqueante;
2. controle focado;
3. surface/aba ativa;
4. workspace;
5. camadas explícitas ou temporárias;
6. programa em primeiro plano, somente quando o Assistente estiver sem foco;
7. aplicativo;
8. global padrão.

Dentro do mesmo nível, especificidade tipada vem antes da prioridade explícita;
empate não resolvido é conflito de configuração e não pode executar duas ações.

O procedimento operacional determinístico compara a tupla:

1. posição do escopo na lista acima;
2. especificidade do predicado, comparada por campos tipados — identidade exata
   antes de tipo e tipo antes de curinga;
3. `command_layers.resolution_priority`;
4. `command_bindings.resolution_priority`.

Comparação é lexicográfica: menor posição na lista de escopos vence (diálogo
antes de global), identidade exata vence tipo/curinga e, somente nos campos
`resolution_priority`, o inteiro maior vence. Prioridades são persistidas e
incluídas no export. Se comandos/argumentos diferentes ainda empatarem após a tupla, a
configuração é inválida e o evento falha fechado; IDs não são usados como
desempate oculto.

Predicados sobre campos diferentes são incomparáveis nessa etapa e, portanto,
empatam em especificidade; não existe precedência oculta entre dispositivo,
perfil, modo, estado ou processo. As prioridades persistidas resolvem esse caso.
Se também forem iguais e os destinos diferirem, aplica-se o conflito fail-closed
descrito acima.

Na v1, condição é somente conjunção de cláusulas `campo eq escalar`, com campos
de enum fechado e valores normalizados por tipo (string exata com case definido
pelo campo, booleano ou ID opaco). Ausência de cláusula é curinga; OR, NOT,
regex, intervalos e operadores de conjunto ficam fora. B domina A somente
quando contém todas as cláusulas idênticas de A e ao menos uma adicional; por
exemplo, `surface=editor ∧ process=code.exe` domina `surface=editor`. Valores
diferentes ou sem relação de subconjunto são incomparáveis e dependem de
prioridade explícita ou terminam em conflito.

O stack compartilhado de `Modal` registra no dispatcher um
`DialogCommandScope { dialog_id, kind, generation, allowed_command_ids,
allowed_trigger_specs }` ao abrir e remove ao fechar. Para `DecisionDialog`, a
ponte deriva o scope das `actions` reais da AEP-0091 e dos atalhos invariantes,
mapeando respostas para o comando fixo `decision.respond`; payload não injeta
command IDs arbitrários. O topo do stack é a fonte autoritativa e mudanças
incrementam `generation`.

Diálogo bloqueante no topo é barreira, não apenas camada prioritária. Enquanto
existir, somente bindings declarados pelo `DialogCommandScope` topmost são
avaliados. Se não houver candidato permitido, o acionador é consumido ou
recusado sem cair para surface, workspace, aplicativo ou global. Isso vale
também para hotkey do SO e Stream Deck e preserva a AEP-0091.
Atalhos invariantes exigidos pela AEP-0091 integram implicitamente toda allowlist
do `DecisionDialog` e não podem ser omitidos nem bloqueados por configuração.
Enquanto houver diálogo topmost, o dispatcher reserva essas combinações antes
de consultar qualquer binding configurável ou ownership global. Assim,
`Ctrl+Shift+R` chega ao `DecisionDialog` mesmo se existir binding concorrente.
No caminho local, a reserva só ocorre depois dos guardas obrigatórios da
AEP-0091: evento não repetido, sem composição IME e fora de input, textarea,
contenteditable e Monaco. Se um guarda bloquear, o dispatcher ignora sem
capturar a digitação. No caminho global, o adapter confiável do SO registra
temporariamente `Ctrl+Shift+R` somente enquanto houver `DecisionDialog`
topmost, inclusive com outro programa em foco, e remove o registro ao fechar o
diálogo. Essa reserva intencional preserva o caso Alt+Tab da AEP-0091, não é
binding configurável e não permanece ativa fora do diálogo.

A UI deve detectar sobreposição possível no momento da edição, explicar em quais
contextos ela ocorre e pedir confirmação antes de criar uma substituição. Um
conflito confirmado sobrescreve somente o acionador concorrente naquele
contexto.

### D8 — Ativação de camadas

Uma camada pode ser:

- sempre ativa;
- ativa enquanto uma condição for verdadeira;
- ativada/desativada manualmente;
- alternada por um comando;
- ativada por evento e removida por evento correlato;
- temporária, com duração ou ciclo de vida definido.

Cada regra produz claims independentes. Uma camada habilitada fica ativa quando
ao menos uma claim válida está ativa; desativação encerra somente o
`activation_id` correspondente. Não há prioridade entre regras. Fixar uma
camada cria claim manual persistente que eventos automáticos não removem;
outras camadas continuam compondo o mapa. `layer.back` encerra a claim manual
mais recente da mesma `manual_stack_key`, ordenada por
`(activated_at, activation_id)`. A chave é derivada no backend da origem
normalizada e de sua sessão/dispositivo, nunca inventada pelo payload.

`layer_disable` alterna `command_layers.enabled` para falso, incrementa a
geração de configuração e, se a camada estava efetiva, o contador de claims do
escopo, tudo sob o `DispatchGate` e na mesma transação. Claims persistidas não
são encerradas, mas o resolvedor as ignora enquanto `enabled = false`.
`layer_enable` revalida ownership, expiração, autenticação e fonte de cada
claim, recalcula condições contextuais e só então torna a camada elegível;
claim expirada/stale não ressuscita. A geração efetiva é incrementada se o
conjunto ativo mudar.

Regras de surface, foco, controle e programa em primeiro plano são condições
síncronas dos context providers da D2, recalculadas em memória quando sua versão
muda. Elas não usam `LayerActivationEvent`, outbox ou ledger de evento. O
envelope abaixo é apenas para ativações event-driven; na v1, somente jobs.

O produtor entrega primeiro um candidato sem autoridade sobre regra, camada ou
escopo:

```text
LayerActivationCandidate
  version, event_name, source_type, source_instance_id, source_event_id
  source_correlation_id?, sequence, source_state, occurred_at, expires_at?
  source_job_database_id?, source_job_slug?, run_id?
  claimed_user_id?, claimed_workspace_id?, provenance?
```

O adapter autenticado valida produtor e ocorrência persistida, resolve o job e
deriva o usuário. `claimed_user_id`/`claimed_workspace_id`, quando um protocolo
legado os transportar, são apenas assertions: divergência é rejeitada e esses
campos nunca compõem chave, lookup ou ownership. O dispatcher localiza regras
e layers somente em catálogo/SQLite pelo owner derivado e produz um evento
normalizado por regra correspondente. Esse envelope interno, não aceito
diretamente de produtor/cliente, é:

```text
LayerActivationEvent
  version, activation_id, rule_ref_kind, rule_ref, user_id, workspace_id?
  source_type, source_instance_id, source_event_id
  source_correlation_id?, sequence
  state, occurred_at, expires_at?
  auth_context_type, auth_context_id, auth_generation, security_generation
  source_replay_policy_generation, source_replay_deadline
  source_job_database_id?, source_job_slug?, chain_id?, chain_history?
```

No evento normalizado, `rule_ref_*`, `user_id`, `workspace_id`,
`activation_id`, contexto de autenticação e gerações/deadline são todos
derivados ou revalidados pelo backend. Nenhum valor homônimo do candidato pode
substituí-los; divergência detectável falha fechado antes de reservar o ledger.

`activation_id` é UUIDv7 novo a cada ciclo; ativar e desativar o mesmo ciclo
reutiliza esse ID. `sequence` cresce dentro de
`(user_id, workspace_id, rule_ref_kind, rule_ref,
activation_id)`. Evento duplicado com mesma sequência é idempotente; sequência
menor é ignorada; mesma sequência com conteúdo diferente falha fechado. Uma
desativação atrasada só encerra seu próprio `activation_id`, nunca uma ativação
mais nova. Expiração local não fabrica `LayerActivationEvent`: um scheduler usa
a chave estável `(activation_id, expires_at)` e, em transação, faz CAS de claim
ativa para terminal `expired` quando o prazo persistido vence, grava
`terminal_reason = expiry` e incrementa a geração do escopo. Repetição ou
restart encontra o mesmo estado/PK e vira no-op idempotente.

Estado `deactivate` ou expirado é terminal. Depois de `deactivate`, o CAS aceita
apenas replay idempotente da mesma sequência/fingerprint; depois de expiração
local, somente repetir a mesma chave de expiração é no-op. Ambos rejeitam
qualquer `activate`, mesmo com sequência maior. Novo ciclo exige novo
`activation_id`; evento tardio não ressuscita claim encerrada.

No envelope genérico, `state` aceita somente `activate` ou `deactivate`.
Cada adapter mapeia seu domínio antes de publicá-lo; no caso de jobs,
`queued`/`started`/`retry_scheduled` viram `activate` e estados terminais viram
`deactivate`. `source_event_id` é sempre UUIDv7 estável emitido pelo produtor e
`occurred_at` é timestamp autenticado; contador ou ID opaco de protocolo não é
aceito nesse envelope.

Na primeira versão, somente fatos persistidos em `job_run_events` e espelhados
na outbox durável desta AEP podem produzir esse envelope. Replay/reconciliação
consome a outbox, não o EventBus best-effort nem a linha sujeita à cascade do
run.

Para eventos da AEP-0067 entrarem depois, uma atualização daquela AEP precisa
definir outbox durável e publicar `_event_id` UUIDv7, `_occurred_at`,
`_correlation_id` e `_sequence`, persistidos transacionalmente antes do
dispatch e reutilizados no replay. O produtor fornece correlação/ordem; o
adapter resolve `activation_id` por regra. Até esse contrato existir, eventos
legados continuam para seus consumidores atuais, mas são indisponíveis como
ativadores de camada.

O ledger de ativação persiste `workspace_id` e `event_fingerprint`. A chave
única por ocorrência usa dois índices parciais: para camada global,
`(user_id, rule_ref_kind, rule_ref, source_event_id) WHERE workspace_id IS
NULL`; para camada local, `(user_id, workspace_id, rule_ref_kind, rule_ref,
source_event_id) WHERE workspace_id IS NOT NULL`.
`source_correlation_id` serve somente para localizar o ciclo em
`command_layer_activation_state`. Há índices auxiliares não únicos, separados
para escopo global/local, sobre usuário, workspace quando aplicável,
`rule_ref_kind`, `rule_ref`, `source_type` e `source_correlation_id`; a consulta
considera também estados terminais e falha fechado se encontrar mais de um
ciclo. O dispatcher processa criação/avanço sob o `DispatchGate` exclusivo, de
modo que dois eventos do mesmo ciclo não podem inserir `activation_id`
concorrentes. Ele
nunca substitui o `source_event_id`, que permanece obrigatório e é a única
chave idempotente de cada transição. Na mesma transação, o estado de PK
`activation_id` avança por CAS sobre `sequence`; update exige o cursor anterior.
Mesmo número com fingerprint diferente grava conflito e não altera a camada.

A regra persiste `event_name` exato e `allowed_internal_producer_types`; na
primeira versão, `event_name` só aceita
`command-context.job-run-state.v1` e o producer type só aceita `jobs.runtime`.
Webhook, plugin e outro produtor externo são rejeitados e ficam fora do escopo
até uma AEP definir identidade de ingress e grants próprios.
Criar/habilitar essa regra é mutação confirmável e persiste
um grant próprio desta AEP em `command_layer_automation_grants`. Esse grant
não reutiliza os grants de delegação de jobs da AEP-0101. Sua chave natural é
o escopo canônico `(user_id, workspace_id, layer_ref_kind, layer_ref,
rule_ref_kind, rule_ref)`; índices parciais separados representam workspace
global e local. A linha vincula `authorization_decision_id`, fingerprint
imutável da regra, `event_name` e fingerprint dos produtores permitidos.

Cada ciclo de concessão recebe `automation_grant_generation` monotônica dentro
da chave natural. O fingerprint do grant cobre chave natural, fingerprint da
regra, fato, produtores e geração. Só pode existir uma linha ativa por chave;
criação e revogação serializam sob o `DispatchGate`, releem a maior geração e
fazem CAS da linha ativa. Revogar preenche `revoked_at`, `revoked_by` e
`revocation_reason`; reconceder insere nova linha com geração maior, preservando
o histórico. Excluir ou desabilitar regra/layer, alterar condição, lifecycle,
fato, produtor, owner ou workspace revoga o grant ativo. Importar, duplicar ou
restaurar configuração nunca cria nem transporta grant; habilitar novamente
exige nova decisão explícita.

`command_layer_activation_rules` referencia o grant ativo por ID, geração e
fingerprint. Cada evento relê, pela chave natural derivada no backend, tanto a
regra quanto a linha ativa e exige que ID, geração e fingerprints coincidam;
grant ausente, revogado, divergente ou concorrente desabilita a regra e falha
fechado. Resposta pendente de `DecisionDialog` também carrega a geração
observada e é rejeitada se ela mudou antes da persistência. Essa ativação
pré-autorizada ocorre somente pelo adapter D8, não executa
`layer.activate`/`layer.toggle` como comando headless.

Sem usuário, regra, autenticação ou identidade válida — correlação, ou o par
instância/evento quando a correlação for ausente — eventos internos não alteram
camadas. Antes de atualizar o estado, o serviço compara
`auth_generation` e `security_generation` atuais; evento de sessão anterior,
logout ou estação bloqueada é descartado.

`user_id` e `workspace_id` não são aceitos como autoridade do payload. O
dispatcher deriva o dono do principal autenticado e o escopo da layer/rule
resolvida; `workspace_id = NULL` representa camada global. Isso também vale
para refs `builtin`, que não dependem de FK para recuperar o escopo. Jobs não
possuem workspace na AEP-0048: o dispatcher relê
`source_job_database_id`/`source_job_slug`/`run_id` apenas no usuário e deriva
o workspace exclusivamente da layer/rule, validando o acesso do mesmo usuário.
O mesmo evento pode alimentar regras de workspaces distintos,
cada uma com estado/chave próprios. Divergência, workspace inacessível ou
ausência de ownership falha fechado.

`source_instance_id` identifica a geração do dispatcher somente para
proveniência. A chave de idempotência é a chave global/local por
`source_event_id` definida acima, portanto replay estável após reinício
continua duplicata sem atravessar workspaces.
`source_replay_policy_generation` e `source_replay_deadline` são derivados pelo
adapter do mesmo `command_event_replay_policy_epochs` de D2.1 e não são aceitos
como autoridade do payload. Estado e ledger de ativação persistem ambos; seu
`expires_at` nunca antecede o maior entre o deadline da fonte e a retenção
terminal de ativações.

Se ledger/cursor terminal já tiver sido removido, evento posterior ao deadline
imutável do epoch vigente em `occurred_at` é rejeitado antes do insert.
Aumentar a retenção depois não reabre ocorrência de epoch anterior; diminuir
não encurta ledger existente. A idade vem somente do timestamp autenticado,
nunca do UUIDv7. Assim, limpeza delimita a deduplicação sem permitir replay
antigo reativar uma camada.

Exemplos:

```text
surface = chat                         → ativa Chat
foreground.process = code.exe          → ativa Desenvolvimento
app.focused = false                    → ativa Global
job run em andamento                   → ativa Execução
streamdeck.key.5 → layer.toggle        → alterna Trabalho
```

Mudanças de tela/estado notificam um `ContextFactBus` in-process e não durável,
separado de `LayerActivationEvent`, com
`{ provider_id, instance_id, version, captured_at }`. Produtores confiáveis são
a ponte tipada da UI e adapters do SO; duplicata de versão é idempotente. A
notificação apenas invalida cache: antes de cada resolução, o `VersionService`
consulta o snapshot atual do provider, portanto perda/reordenação não conserva
camada incorreta. O monitor de janela em primeiro plano é adapter específico por
sistema operacional; no Windows, não depende do software do Stream Deck.

Consumidor do `ContextFactBus` adquire `DispatchGate` exclusivo antes de trocar
snapshot e recalcular claims. Troca de snapshot sempre altera a versão do
provider em `context_version`, mas só incrementa o contador global/workspace de
`active_layers_generation` quando o conjunto efetivo de claims mudar.
Resolução/admissão lê providers sob o gate compartilhado. Se a versão
autoritativa diferir da versão usada no último cálculo de claims, libera o gate
compartilhado, adquire o exclusivo, compara novamente, reconcilia
sincronamente snapshot/claims/gerações e reinicia a resolução. Assim,
notificação perdida ou mudança já observada não conserva camada antiga nem
atravessa o CAS/início com geração antiga.

Para jobs, a integração publica o fato contextual interno versionado
`command-context.job-run-state.v1`, cujo `event_name` é exatamente esse nome,
com `user_id` como assertion candidata, `job_database_id`, `job_slug`,
`run_id`, `run_event_id`, `sequence`, `state`, `occurred_at`,
`root_origin_type`, `root_origin_id`, `_source`, `_source_job_id`, `_chain_id` e
`_chain_history`. `_source` deve ser
`job` nesse fato; outro valor falha fechado. `run_event_id` é o UUIDv7 de
`job_run_events.id` e vira o `source_event_id` estável, inclusive em replay.
`job_slug` recebe `Job.ID`, que no modelo atual é o slug público usado por
`eventctx.SourceJobID` conforme AEP-0067; `job_database_id` recebe
`Job.DatabaseID`, UUID de `jobs.id` referenciado por `job_runs.job_id`.
O adapter resolve primeiro `(user_id, job_slug)`, exige que a linha encontrada
tenha exatamente esse `DatabaseID` e então usa o UUID em correlação e
autorização. Fato legado que envia `job.ID` no campo ambíguo `job_id` ou omite
`job_database_id` é rejeitado.

`run_id` é tratado nesta borda como ID opaco e precisa corresponder exatamente
a uma linha `job_runs.id` do mesmo usuário e job. O adapter não valida
prefixo/formato nem converte IDs. A divergência entre o formato UUIDv7
documentado na AEP-0048 e produtores atuais deve ser corrigida em PR próprio,
com status/evidência da AEP-0048 atualizados, mas não bloqueia lookup seguro por
PK/ownership nesta integração. O adapter D8 permanece desabilitado até o
produtor persistir/publicar `Job.DatabaseID` como `job_database_id` e a AEP-0048
ser atualizada no mesmo PR; não há fallback silencioso do slug para UUID.

Esta AEP é dona de `root_origin_type` e de sua normalização:
`manual → manual`, `cron → cron`, `interval → interval`,
`hotkey → user_hotkey`, `webhook → external_event`; trigger `event` herda a
raiz autenticada do payload/provenance e vira `internal_event` somente quando o
produtor for interno conhecido. Raiz ausente vira `unknown`. Na v1, `manual`,
`cron`, `interval`, `user_hotkey` e `internal_event` são elegíveis;
`external_event`/`unknown` não ativam camada, mesmo quando o run intermediário
tenha `_source = job`.
O futuro PR da integração adiciona e persiste esse campo no runtime/timeline,
atualiza a AEP-0048 no mesmo ciclo e só então habilita o adapter; inferência
retroativa a partir do trigger atual é proibida.

Persistência incremental exige migração prévia da AEP-0048:
`job_runs.status` passa a aceitar `queued`, `running`, `retrying`, `completed`,
`failed` e `skipped`; ganha `queued_at` NOT NULL e torna `started_at` nullable.
Linhas existentes recebem `queued_at = started_at`; estados terminais não
mudam. O run é inserido como `queued`, muda para `running` ao iniciar e só então
preenche `started_at`. Duração continua calculada desde `started_at`, não da
fila. `job_run_events` referencia a linha já criada.

Como pré-requisito do adapter, o executor persiste
`job_run_events.type = queued` antes do despacho, `started` antes da tool e
`retry_scheduled` antes do backoff. Em paralelo, `job_runs.status` usa
respectivamente `queued`, `running` e `retrying`; os enums não são
intercambiáveis. Sem essas transições incrementais o fato v1 fica desabilitado.

O runtime gera `run_event_id` ao criar cada `RunEvent`, persiste
todos os estados mapeados — inclusive `completed`, `failed` e `skipped` —
incremental e idempotentemente antes de publicar o fato, reutilizando o mesmo
UUID na linha. O `defer LogRun` final vira upsert pelo UUID e não duplica eventos
já persistidos. O PR dessa integração atualiza a AEP-0048 e seus testes no mesmo
ciclo.

Na mesma transação de cada `job_run_events` elegível, o runtime insere
`command_job_activation_outbox` com o fato normalizado, owner, IDs
job/run/event, provenance, epoch/deadline de replay e fingerprint. Só publica
após commit. A outbox não referencia `job_runs`/`job_run_events` por FK com
cascade; `CleanRunsExceedingCount` pode remover runs e timeline sem apagar uma
ocorrência ainda reprocessável. O dispatcher faz claim com lease, processa
todas as regras elegíveis e marca `delivered`; crash devolve `processing`
vencido para `pending`. Falha permanente auditada vira `dead_letter`.

Outbox `pending`/`processing` não é removida por idade ou count-cap. Linha
`delivered`/`dead_letter` só sai depois de `source_replay_deadline`; até lá,
replay encontra a mesma PK/fingerprint. O
`InstanceMaintenanceCoordinator` processa/reconcilia essa outbox antes da
limpeza de jobs. O PR de implementação atualiza AEP-0048 e AEP-0074-B para
substituir a cascade como fronteira de replay; até outbox, ordem de manutenção e
testes de count-cap existirem, o adapter D8 permanece desabilitado.

`state` aceita `queued`, `started`,
`retry_scheduled`, `completed`, `failed` e `skipped`. A chave de
correlação é `(user_id, run_id)`; `sequence` impede regressão por entrega fora
de ordem. Estados `queued`, `started` e `retry_scheduled` mantêm a regra ativa;
`completed`, `failed` e `skipped` a encerram. Esse fato deriva do
runtime e da timeline `job_run_events` da AEP-0048; não inventa nomes no event
bus público da AEP-0001.

Essa lista é allowlist exaustiva. `triggered`, `event_emitted`,
`event_received` e qualquer tipo desconhecido são ignorados idempotentemente e
não produzem `LayerActivationEvent`. Gaps de `sequence` são permitidos; CAS
aceita somente valor maior que o cursor, não exige contiguidade.

O adapter usa `run_id` como `source_correlation_id` e resolve ou cria um
`activation_id` distinto por
`(user_id, workspace_id, rule_ref_kind, rule_ref, run_id)`. Assim, duas regras
que observam o mesmo run mantêm ciclos independentes. Ele preserva
`job_run_events.sequence` e mapeia `job_runs.status = retrying` para o fato
`state = retry_scheduled`. A timeline é a fonte de ordem; o status do run serve
apenas para reconstrução no startup.

Claim derivada de job usa lease própria, renovada por heartbeat do runtime:
`maintenance.command_job_activation_lease_seconds` (padrão 180), com heartbeat
antes da metade do TTL. Runs não terminais que sustentam claim ficam excluídos
da política/ciclo de limpeza da AEP-0074-B somente enquanto a lease estiver
válida. Lease expirada marca a claim inativa e devolve o run órfão à retenção no
próximo ciclo. O PR dessa integração deve atualizar
AEP-0074-B e AEP-0048 no mesmo ciclo. No startup, fonte ausente, lease vencida
ou estado não autoritativo torna a claim inativa até confirmação nova do
runtime. A linha pode permanecer para auditoria, mas nunca mantém a camada
efetiva sem lease válida.

O adapter preserva a proveniência anti-loop da AEP-0067. Se um binding ativado
por esse ciclo iniciar job, tool que publica evento ou outro comando reativo, a
invocação herda `_chain_id`/`_chain_history` sem acrescentar namespaces que não
sejam jobs. `provenance.command_chain_history` é lista ordenada de
`{ command_id, invocation_id, layer_refs }`, persistida no mesmo documento
redigido do envelope. `CommandExecutionService` é dono do limite constante
versionado `CommandMaxChainDepth = 16`: antes da reserva, valida a lista, rejeita
repetição do mesmo `command_id` na cadeia e acrescenta a entrada atual. Ao
iniciar novo job, somente o runtime de jobs acrescenta o job à
`_chain_history` e chama `DetectLoop`/`MaxChainDepth`. Evento derivado de job sem
proveniência não pode habilitar comando capaz de ampliar a cadeia; falha
fechado.

Cancelamento não pertence ao enum vigente da AEP-0048 e, portanto, não é
inventado aqui. Se o runtime ganhar esse estado, a AEP-0048 deve ser atualizada
antes de ele entrar no fato contextual v1 ou em uma versão posterior.

Trocas rápidas passam por estabilização curta. O usuário pode fixar uma camada,
criando a claim manual persistente descrita acima; isso impede sua remoção por
automação, mas não congela as outras camadas. Se o contexto deixar de ser
confiável, o resolvedor retorna ao conjunto padrão seguro.

### D9 — Command Palette

A Command Palette usa o mesmo registro e deve permitir:

- buscar por nome, descrição, categoria e aliases localizados;
- executar comandos disponíveis;
- mostrar atalho efetivo no contexto atual;
- listar recentes e favoritos;
- solicitar argumentos por formulário quando faltarem;
- abrir diretamente a configuração do comando ou binding.

Comandos indisponíveis podem ser exibidos com o motivo, em vez de falhar
silenciosamente.

A palette abre com foco no campo de busca, usa o padrão acessível
combobox/listbox, permite setas para navegar e Enter para executar. Escape fecha
e restaura o foco ao elemento que a abriu. Quantidade de resultados, comando
indisponível, sucesso e erro são anunciados pelo announcer global. Formulários de
argumentos e confirmações seguem os componentes compartilhados; abrir uma nova
surface transfere o foco segundo o contrato dessa surface. Testes cobrem
teclado, focus trap, restauração de foco e axe, com validação manual por NVDA
antes de concluir a fase.

### D10 — Gerenciamento por chat

O agente gerencia o sistema por duas tools compostas, seguindo a convenção da
AEP-0048 para não inflar o catálogo:

- `command_catalog`, com ações `list`, `describe` e `execute`;
- `command_config`, com ações `layer_list`, `layer_get`, `layer_create`,
  `layer_update`, `layer_delete`, `layer_enable`, `layer_disable`,
  `layer_restore`, `binding_list`, `binding_check_conflict`,
  `binding_create`, `binding_update`, `binding_delete`, `binding_enable`,
  `binding_disable`, `binding_restore`, `config_import` e `config_export`.

Alterações destrutivas, conflitos e comandos sensíveis continuam sujeitos ao
contrato de decisão da AEP-0091. A resposta da tool inclui IDs reais e o efeito
resolvido; o modelo não edita tabelas diretamente.

Além disso, toda ação mutável de `command_config` solicitada por agente mostra
ao usuário o diff exato e exige decisão explícita, mesmo que não seja
destrutiva: criar ou habilitar um binding pode conceder capacidade futura.
Origem headless falha fechado. A confirmação autoriza somente aquela mutação e
não concede grant reutilizável para executar o comando configurado.

O registro marca `mutates_effective_capability` em comandos como
`layer.activate`, `layer.toggle`, `layer.back` persistente e registro de hotkey.
Para todo `actor_type != user`, `CommandExecutionService` exige decisão em
`command_catalog.execute` se esse flag estiver ativo ou se
`decision_requirement` exigir; comandos read-only com requisito `none` seguem
sem diálogo. Para os casos confirmáveis, origem headless falha fechado.
Automação de ativação usa exclusivamente a regra previamente confirmada da D8;
um binding `event` não pode chamar esses comandos mutáveis. Assim, não existe
segunda rota para alterar o mapa efetivo.

A marca não é declarada livremente pelo autor do comando.
`CommandHandler.EffectClass()` e `CommandHandler.Mutability()` fornecem,
respectivamente, `effect_class` e `mutates_effective_capability`; o registro
rejeita divergência com metadata e combinações inválidas de risco/decisão. Em
`command_config`, somente list/get/check_conflict e
`config_export` sem credenciais são leitura; create, update, delete, enable, disable, restore e
import são mutações de capacidade e sempre exigem o gate. Teste de catálogo enumera todas as ações/handlers para impedir que
um verbo novo nasça sem classificação.

`config_export_sensitive` existe apenas no registro/UI, não na tool
`command_config`: risco alto, `decision_requirement=interactive`, somente
`ui.action`, formulário de senha e criptografia/redaction da AEP-0047; chat e
origem headless são proibidos. `config_export` rejeita
`includeCredentials=true` em vez de promover silenciosamente.

### D11 — Persistência

Defaults ficam no código. SQLite guarda entidades do usuário e deltas:

```text
command_layers
  id, user_id, workspace_id nullable_for_global, name, description, enabled, source,
  resolution_priority, created_at, updated_at

command_layer_activation_rules
  id, user_id, workspace_id nullable_for_global,
  layer_ref_kind, layer_ref, rule_ref_kind, rule_ref,
  mode, condition, lifecycle, event_name,
  allowed_internal_producer_types, authorization_decision_id,
  automation_grant_id, automation_grant_generation,
  automation_grant_fingerprint, enabled, source,
  replaces_default_id nullable, replaces_default_version nullable,
  replaces_default_fingerprint nullable, review_status

command_layer_automation_grants
  id, user_id, workspace_id nullable_for_global,
  layer_ref_kind, layer_ref, rule_ref_kind, rule_ref,
  rule_fingerprint, event_name, producer_types_fingerprint,
  automation_grant_generation, automation_grant_fingerprint,
  authorization_decision_id, granted_at, granted_by,
  revoked_at nullable, revoked_by nullable, revocation_reason nullable

command_bindings
  id, user_id, workspace_id nullable_for_global, layer_ref_kind, layer_ref,
  trigger_type, trigger_spec, command_id nullable_for_suppress,
  arguments empty_for_suppress,
  condition, effect, enabled, source, resolution_priority, replaces_default_id,
  replaces_default_version, replaces_default_fingerprint, review_status,
  presentation

command_config_generations
  id, user_id, workspace_id nullable_for_global, generation, updated_at

command_config_mutations
  mutation_id PK UUIDv7, schema_version, user_id, session_id,
  scope, operation, binding_id, decision_id UNIQUE, request_fingerprint,
  auth_generation, security_generation, generation_id,
  before_generation, after_generation, before_enabled, after_enabled,
  occurred_at

command_event_replay_policy_epochs
  id, producer_type, generation, effective_at, replay_horizon_seconds,
  created_at

command_key_versions
  id PK UUIDv7, version UNIQUE, digest SHA-256, active

command_job_activation_outbox
  source_event_id PK UUIDv7, user_id, job_database_id, job_slug, run_id,
  sequence, state, occurred_at, root_origin_type, provenance,
  source_replay_policy_generation, source_replay_deadline,
  event_fingerprint, delivery_state, lease_owner nullable,
  lease_expires_at nullable, attempts, last_error_code nullable,
  created_at, delivered_at nullable

command_decision_receipts
  decision_id PK UUIDv7, user_id, auth_context_type, auth_context_id,
  auth_generation, security_generation, subject_type, subject_id,
  request_fingerprint, allowed_action_ids, accepted_action_id nullable,
  status, expires_at, responded_at nullable, consumed_at nullable

command_layer_activation_state
  activation_id PK UUIDv7, layer_ref_kind, layer_ref, rule_ref_kind, rule_ref,
  user_id, workspace_id nullable_for_global,
  auth_context_type, auth_context_id, auth_generation,
  security_generation, source_type, source_instance_id nullable,
  source_event_id nullable, source_correlation_id nullable, sequence nullable,
  source_job_database_id nullable, source_job_slug nullable,
  event_fingerprint nullable, source_replay_policy_generation nullable,
  source_replay_deadline nullable, state, terminal_reason nullable,
  provenance nullable, manual_stack_key nullable, activated_at,
  expires_at nullable, updated_at

command_activation_idempotency_keys
  id, key, user_id, workspace_id nullable_for_global,
  rule_ref_kind, rule_ref, source_type, source_instance_id, source_event_id,
  source_correlation_id nullable, sequence, event_fingerprint,
  source_job_database_id nullable, source_job_slug nullable,
  source_replay_policy_generation, source_replay_deadline,
  terminal_state, created_at, expires_at

external_identity_mappings
  id, issuer, subject, user_id, enabled, created_at, updated_at

command_invocations
  invocation_id PK UUIDv7, schema_version, user_id nullable_for_system,
  auth_context_type, auth_context_id,
  auth_generation, session_id nullable_for_non_local, security_generation,
  workspace_id nullable_without_workspace, registry_version,
  global_config_generation nullable_for_system,
  workspace_config_generation nullable_without_workspace,
  active_layers_generation nullable_for_system,
  command_id nullable_until_resolved, binding_ids nonnull_default_empty,
  observed_trigger_type nullable_for_direct,
  trigger_type nullable_for_direct, trigger_spec_snapshot nullable_for_direct,
  trigger_fingerprint nullable_for_direct,
  actor_type, actor_id, source_type nullable_until_resolved,
  observer_type nullable,
  source_instance_id nullable, source_event_id nullable,
  source_occurred_at nullable_for_non_event,
  source_replay_policy_generation nullable_for_non_event,
  source_replay_deadline nullable_for_non_event,
  arguments_summary, arguments_fingerprint,
  conversation_id nullable, turn_id nullable,
  surface_type nullable, surface_id nullable, surface_snapshot_version nullable,
  context_version nullable_for_none,
  context_captured_at_by_provider nullable_for_exact_version_or_none,
  context_summary nullable_for_none, foreground_summary nullable,
  source_profile_slug nullable, target_profile_slug nullable,
  authorization_decision_id nullable_until_decided,
  delegation_fingerprint nullable, grant_generation nullable,
  job_id nullable, job_slug nullable, job_definition_fingerprint nullable,
  run_id nullable, provenance nullable,
  correlation_id, request_fingerprint_version, request_fingerprint,
  risk, policy_decision,
  result_summary nullable_until_terminal, result_ref nullable,
  status, error_code nullable_for_succeeded_or_nonterminal,
  client_requested_at nullable, received_at,
  completed_at nullable_until_terminal

command_idempotency_keys
  id, key, invocation_id, user_id nullable_for_system,
  auth_context_type, auth_context_id,
  source_type nullable_until_resolved, source_instance_id nullable,
  source_event_id nullable, source_occurred_at nullable_for_non_event,
  source_replay_policy_generation nullable_for_non_event,
  source_replay_deadline nullable_for_non_event,
  request_fingerprint_version, request_fingerprint,
  status, result_summary, result_ref, received_at, expires_at
```

No ledger, `id` é PK UUIDv7, `key` é UNIQUE e vale
`invocation:<invocation_id>` para toda solicitação sem `source_event_id`,
direta ou resolvida por trigger, e `event:<source_event_id>` quando esse ID
existir. Portanto palette, `ui.action`, chat e CLI por trigger usam a primeira
forma. `invocation_id` também é UNIQUE e
`source_event_id` tem índice único parcial quando não nulo. Conflito relê
ownership e fingerprint antes de classificar como reentrega; divergência falha
fechado. `status` aceita os estados de invocação, `suppressed` e
`rejected_stale`; nos dois últimos, `invocation_id` continua obrigatório, mas
não é FK para
`command_invocations`, pois não há linha de auditoria e o ledger sobrevive à
compactação.

No ledger de ativação, `key` inclui escopo canônico:
`activation:<user_id>:global:<rule_ref_kind>:<rule_ref>:<source_event_id>` ou
`activation:<user_id>:workspace:<workspace_id>:<rule_ref_kind>:<rule_ref>:<source_event_id>`,
e é UNIQUE. Os dois índices parciais equivalentes são os definidos na D8.
Assim, o mesmo evento pode alimentar regras ou workspaces distintos sem
colisão e não reaplica a mesma regra no mesmo escopo.

Condições, argumentos, especificações e apresentação são documentos JSON
versionados e validados. Alterações relevantes mantêm auditoria suficiente para
desfazer.

`command_config_mutations` registra mudanças confirmadas de configuração, não
invocações de comandos. O subconjunto v1 é fechado em
`scope=global`/`operation=binding_enabled`: IDs e gerações vinculam a receipt,
os booleanos antes/depois preservam o efeito reversível, e o fingerprint
versionado identifica a proposta sem persistir seu texto ou documentos. Registro,
consumo da receipt e mudança de configuração pertencem à mesma transação.
Uma operação revertida não deixa linha de sucesso. Consulta interna é escopada
por usuário/sessão e UUID da mutação. Não há exclusão automática ou undo
automático; política de retenção e restauração confirmada continuam pendentes.

`command_bindings` usa a mesma referência polimórfica `builtin|user` do estado
de ativação. Ref `user` precisa apontar para `command_layers` do mesmo
usuário/workspace; ref `builtin` é validada no catálogo e só aceita delta com
`replaces_default_*`. O owner e o escopo ficam na própria linha para que
restore/import funcionem sem materializar defaults. Binding inteiramente novo
só pode referenciar layer `user`.

Campos marcados com `?` no envelope são nullable no SQLite; ausência vira
`NULL`, nunca string vazia. `command_id` fica nulo em `evaluating`/`denied`
somente quando a resolução prévia não encontrou comando; em toda reserva
resolvida executável é preenchido e imutável. Supressão terminal é a exceção
sem `CommandInvocation` da D4. `trigger_*` é nulo em execução direta;
`session_id` é nulo fora de `local_session`; campos de surface, conversa, job,
profile, workspace e decisão são nulos quando o contexto não se aplica.
`source_occurred_at` é obrigatório somente para
`source_type = event`, vem da fonte autenticada e fica nulo nas demais origens.
Nesse caso, `source_replay_policy_generation` e `source_replay_deadline`
também são obrigatórios e derivam do epoch de política, nunca do payload.
Campos obrigatórios do envelope e `binding_ids` (default `[]`) são NOT NULL.

Constraints condicionais validam os grupos: conversa e turno aparecem juntos;
surface exige o trio tipo/ID/versão ou todos `NULL`; delegation fingerprint e
generation aparecem juntos; `job_service` exige job ID/slug/fingerprint e
run ID, enquanto outros contextos os deixam `NULL`; policy com providers exige
`context_summary`, e a que declara foreground exige também
`foreground_summary`; origem job/evento reativa exige `provenance`. Ator agente
exige `source_profile_slug` e `target_profile_slug`; demais atores só os
preenchem quando houver delegação explícita. Combinação parcial falha antes da
reserva.

Todas as PKs persistidas criadas por esta AEP são UUIDv7 conforme AEP-0046.
FKs entre essas tabelas também usam UUIDv7. IDs de defaults que vivem no código
são strings namespaced estáveis; `replaces_default_id` referencia essa
identidade lógica, não uma linha SQLite.

`command_event_replay_policy_epochs` tem UNIQUE
`(producer_type, generation)` e `(producer_type, effective_at)`. A implantação
cria o epoch inicial antes de habilitar o adapter D8; alteração da retenção de
jobs persiste o novo epoch na mesma seção crítica que publica a configuração.
Lookup por `source_occurred_at` escolhe o maior `effective_at` não posterior ao
evento. Ausência ou ambiguidade falha fechado.

Regras persistidas e estado de ativação usam referências polimórficas
validadas, não FKs:
`layer_ref_kind`/`rule_ref_kind` aceitam `builtin` ou `user`; refs builtin são
IDs namespaced do catálogo em código e refs user são UUIDv7 que precisam
pertencer ao mesmo usuário. Isso permite ativar defaults sem copiá-los para
`command_layers` e mantém restore sob ownership do catálogo.

Em `command_layer_activation_rules`, `id` é a PK física da linha. Regra criada
pelo usuário usa `rule_ref_kind = user` e `rule_ref = id`; delta de regra
padrão usa `rule_ref_kind = builtin`, o ID namespaced do catálogo em `rule_ref`
e o trio `replaces_default_*`. A referência de layer é independente: tanto
regra user quanto delta builtin podem apontar para layer `builtin` ou `user`,
desde que o owner e o workspace canônicos coincidam. Default puro continua no
código e não exige linha SQLite. Pares de índices únicos parciais impedem duas
linhas com a mesma referência de regra no escopo global/local; `needs_review`
bloqueia a regra sem fallback, como nos bindings.
Habilitar uma regra builtin event-driven que exija grant materializa primeiro
esse delta confirmado; assim, grant, geração e revogação têm uma linha
autoritativa sem copiar a camada/default inteira.

`binding_ids` é uma lista JSON ordenada que registra todos os bindings
equivalentes considerados na deduplicação; fica vazia para execução direta.
O acionador efetivo também fica em snapshot normalizado/redigido e fingerprint,
portanto excluir o binding ou atualizar defaults não apaga sua origem histórica.
`status` aceita `evaluating`, `queued`, `running`, `succeeded`, `failed`, `denied`,
`cancelled`, `cancelled_stale`, `timed_out` e `outcome_unknown`. `result_summary` é redigido e
`result_ref` guarda somente referência estável e não sensível, como o ID de uma
aba ou run, permitindo consultar uma reentrega sem repetir efeitos.
Em `evaluating`, `queued` e `running`, `result_summary`, `result_ref`,
`error_code` e `completed_at` são `NULL`. Todo estado terminal exige
`result_summary` redigido — objeto vazio quando não houver retorno — e
`completed_at`; `result_ref` continua opcional. `error_code` é `NULL` em
`succeeded` e obrigatório nos demais terminais, com código estável inclusive
para cancelamento, staleness, timeout e outcome desconhecido.

`command_layer_activation_state` mantém o último cursor de cada ciclo. No
startup, regras `always` e contextuais são recalculadas e não ocupam essa
tabela; ciclo manual só é
restaurado se seu lifecycle for persistente e o usuário for autenticado
novamente; temporário expirado ou session-scoped termina; ciclo de evento/job é
reconciliado com a fonte. Todo estado sem autenticação válida fica inativo;
evento/job também exige provenance e fonte revalidáveis. Claim manual
persistente exige dono autenticado e lifecycle válido, mas não provenance de
job. Ciclos terminais permanecem pela mesma retenção curta das invocações para
deduplicar reentregas; ativos não são removidos pela idade.

Ao restaurar claim manual persistente após login/restart, o serviço revalida
ownership/layer e, numa transação, substitui auth/security generations e
`manual_stack_key` pelo novo contexto antes de ativá-la. Origem/dispositivo não
mais disponível deixa a claim inativa e visível para revisão; `layer.back` só
opera sobre claims já rebindadas.

Matriz de nulabilidade do estado: claims manuais exigem `manual_stack_key` e
podem deixar `source_instance`, `source_event`, correlação, sequence e
provenance nulos; claims de evento deixam `manual_stack_key` nula e exigem
instância, evento UUIDv7, sequence, fingerprint, geração/deadline de replay;
correlação é opcional; claim temporária exige `expires_at`; provenance é
obrigatória quando a origem for job.
Campos de auth/segurança e refs de layer/rule são sempre obrigatórios.
`workspace_id` é obrigatório para layer local e nulo somente para layer global;
é persistido também no ledger mínimo. Ausência vira `NULL`, nunca sentinel vazio.

`workspace_id` nulo identifica camada global do usuário; preenchido identifica
camada daquele workspace. A consulta efetiva carrega somente camadas globais do
usuário autenticado mais as do workspace atual. SQLite usa dois índices únicos
parciais: `(user_id, name) WHERE workspace_id IS NULL` para globais e
`(user_id, workspace_id, name) WHERE workspace_id IS NOT NULL` para workspaces.

Os grants de automação também usam pares de índices parciais para a chave
natural global/local. Outro par de índices únicos parciais, filtrado por
`revoked_at IS NULL`, garante no máximo um grant ativo por chave. Um índice
único adicional sobre chave natural mais `automation_grant_generation`
preserva a monotonicidade auditável; a transação sob `DispatchGate` relê a
maior geração antes do insert. `automation_grant_id` referencia exatamente a
linha ativa e nunca é inferido apenas pelo fingerprint.
Bindings herdam o escopo da camada, evitando misturar configurações.

`command_config_generations` usa os mesmos dois índices únicos parciais de
escopo: uma linha global por usuário e uma por usuário+workspace. Toda mutação
incrementa a linha aplicável na mesma transação dos dados alterados.

`external_identity_mappings` tem índice único `(issuer, subject)` e FK para
`users.id`. Só administração autenticada pode criá-lo; ele não é importado,
exportado nem inferido por login.

Abrir ou carregar um workspace não autoriza conteúdo controlado pelo workspace
— arquivos do projeto, metadados importados ou eventos emitidos por ele — a
criar ou habilitar bindings de shell, MCP, hotkeys globais ou ações externas.
Essas operações exigem ator autenticado e o fluxo explícito de configuração.

Exportação e importação integram o envelope versionado da AEP-0047 pela seção
`resources.commandLayers`. Cada camada inclui UUID, escopo portátil,
`activationRules` e `bindings`; overrides incluem ID, versão e fingerprint do
default para round-trip de `needs_review`. Defaults puros, invocações e todo
`command_layer_activation_state` — inclusive pin/claim manual persistente — não
são exportados. Camadas importadas começam sem claims manuais; regras
contextuais são recalculadas no destino. Referências internas
são remapeadas em conjunto e a importação é idempotente por UUID.
Para `activationRules`, refs builtin são validadas no catálogo e refs user são
remapeadas com a camada/lote; owner e workspace vêm do destino autenticado.
Regra event-driven importada permanece desabilitada sem grant e exige
confirmação local para habilitar.

Bindings persistentes não armazenam segredo bruto. Paths marcados como
sensíveis pelo comando aceitam somente referência ao cofre/credencial; quando
o schema não permite separar o segredo, aquele comando só admite execução
ad hoc e não pode virar binding persistente. O export inclui a referência, nunca
o valor. Se o usuário escolher `includeCredentials`, o segredo viaja apenas no
bloco criptografado definido pela AEP-0047. Exportação faz nova validação e
recusa o arquivo, com relatório, se encontrar configuração antiga que viole
essa regra.

Referência portátil de credencial usa `{ kind: "credential", pattern }`, nunca
UUID local. Na importação, credenciais do bloco criptografado são tratadas
primeiro pela AEP-0047; depois, cada binding resolve o `pattern` exato somente
no usuário de destino. Ausência, ambiguidade ou pattern pertencente a outro
usuário deixa o binding desabilitado e entra no relatório. Não há associação
automática por posição, nome aproximado ou ID da instância de origem.

Conflito de UUID com conteúdo diferente exige escolha explícita entre manter,
substituir ou importar como cópia com novos UUIDs. Referência a workspace,
comando, dispositivo ou default ausente fica desabilitada e entra no relatório
de importação; não é aproximada por nome. Grants, autorizações e ativações
temporárias nunca são exportados ou concedidos. A configuração importada só
entra no mapa efetivo após validação e confirmação dos conflitos.

Toda busca/upsert usa `(user_id autenticado, id)`. UUID já pertencente a outro
usuário retorna conflito `foreign_owner` sem revelar conteúdo, sobrescrever ou
associar referência. UUID ausente é criado para o usuário autenticado,
ignorando qualquer owner do arquivo. A opção “cópia” gera novos UUIDs e remapeia
somente relações internas validadas daquele lote.

`workspace_id` portátil nunca é aceito sem resolução. O import recebe mapa
explícito origem→workspace de destino; ID igual só é reutilizado após repository
confirmar ownership/acesso do usuário autenticado. Ausência, ambiguidade ou
destino não autorizado desabilita a camada e entra no relatório antes do
commit.

Se “cópia” colidir com nome único no mesmo escopo, exige novo nome explícito
antes do commit; a UI pode sugerir rótulo localizado, mas não persiste enquanto
ele não for único. Validação e insert ocorrem na mesma transação para fechar
corrida.

Essa seção só é habilitada depois que o mesmo PR atualizar a AEP-0047, registrar
`commandLayers` e incrementar a versão do envelope portátil com regras de
compatibilidade. Até lá, comandos/camadas são recurso não suportado pelo export:
o relatório avisa a omissão e a UI não promete backup deles. Importador antigo
continua ignorando seção desconhecida com warning, como define a AEP-0047.

`command_invocations` é auditoria técnica efêmera.
`internal/commandinvocations.MaintenanceService`, definido por esta AEP e
executado pelo novo `InstanceMaintenanceCoordinator`, opera em escopo
privilegiado da instância: enumera todos os usuários e também `user_id IS NULL`,
sem depender do usuário ativo. A implementação adiciona APIs de manutenção
separadas: uma enumera IDs de usuário; cada limpeza por usuário recebe contexto
interno com aquele `user_id` e preserva `RequireUserID`; registros system usam
métodos dedicados que exigem capability da instância. O coordenador nunca
remove o guard nem passa contexto sem usuário a APIs comuns. Ele absorve a
cadência hoje iniciada por `jobs.Manager.runRetention` e chama, por interfaces,
numa única goroutine e nesta ordem: reconciliação de leases da
`command_job_activation_outbox`; retenção de jobs;
`ToolInvocations.CleanOldDryRuns`; `CleanOrphanChat`; `CleanOldChat`; retenção
de invocações/ledgers de comandos; retenção de ativações; e compactação física
por `maybeCompact`. O futuro PR de implementação atualizará a AEP-0074-B e
moverá a responsabilidade sem perder nenhuma limpeza nem vacuum/compactação;
não criará loop paralelo. Este PR documental apenas registra esse requisito.

O serviço lê exclusivamente
`maintenance.command_invocation_retention_days` (padrão 30) e
`maintenance.command_invocations_per_user_keep` (padrão 10.000) de
`MaintenanceSettings`/`config.json`. Invocações internas sem usuário usam
`maintenance.command_invocations_system_keep` (padrão 1.000), além do mesmo
limite por idade. Para ciclos terminais, usa
`maintenance.command_activation_terminal_retention_days` (padrão 30) e
`maintenance.command_activation_terminal_keep_per_user` (padrão 10.000), além
de `maintenance.command_job_activation_lease_seconds` (padrão 180). Estados
ativos ficam fora da limpeza por idade/quantidade. As seis chaves
aparecem na mesma UI de manutenção. O PR que implementar esta fase deve
atualizar settings e UI no mesmo ciclo; não se cria configuração paralela.

O serviço remove registros antigos/acima do limite. Índices mínimos:
`(user_id, received_at)`, `(user_id, status, received_at)`, PK única por
`invocation_id` para chamadas diretas e índice único parcial
`(source_event_id) WHERE source_event_id IS NOT NULL AND source_event_id <> ''`
para eventos de adapter. Evento físico sempre tem usuário autenticado; contexto
`system` sem usuário não usa esse índice e possui
`(received_at) WHERE user_id IS NULL` para sua limpeza global.

Os limites de quantidade removem somente auditoria detalhada em
`command_invocations` e estado terminal em
`command_layer_activation_state`. Os dois ledgers mínimos não são removidos por
cap: permanecem até `expires_at`, calculado com a respectiva retenção
configurada. Para `source_type = event`, o piso adicional, a consulta da fonte e
a rejeição por `source_occurred_at` seguem D2.1; a chave não é removida enquanto
a fonte ainda puder reentregá-la. Só depois desses gates a chave pode ser
reutilizada. Status/resultado redigido e último
fingerprint/sequence são atualizados no ledger na mesma transação da mudança de
estado. `command_idempotency_keys.key` e
`command_activation_idempotency_keys.key` têm índices únicos. Assim, compactar
uma linha recente não reabre a execução nem a ativação.

Não se persiste `arguments` bruto nem o `SurfaceContext` completo:
`arguments_summary` é redigido; `arguments_fingerprint` e
`request_fingerprint` são HMACs domain-separated calculados no backend sobre os
valores canônicos ainda não redigidos e só o digest é persistido. Assim, valores
sensíveis diferentes não colapsam no mesmo placeholder. Identidades de
conversa/turno/surface, `surface_snapshot_version`, `context_version` e resumo
redigido permitem rastrear a origem sem copiar segredos. O conteúdo completo do `SurfaceContext` não é
reconstituível depois de expirar e esta AEP não promete reprodução integral.
Auditorias de decisão/grant que tenham retenção própria na AEP-0091 ou AEP-0101
não são substituídas por esta tabela.

Quando um comando delega para tool, `tool_invocations` precisa ganhar
`origin_type = command_invocation` e
`origin_id = command_invocations.invocation_id`. O primeiro PR dessa integração
atualiza a AEP-0063, enum, consultas e retenção no mesmo ciclo. Até isso existir,
comando que delega para tool fica indisponível; usar `system` como fallback é
proibido porque misclassificaria ação de usuário/agente como automação interna.
A origem completa permanece em `command_invocations` e a UI tolera o lado
técnico já expirado.

### D12 — Resolução eficiente

O resolvedor não percorre todas as camadas nem consulta o banco a cada tecla.

- configurações são carregadas e validadas na memória;
- bindings são indexados pela identidade normalizada do acionador;
- o conjunto de camadas ativas é mantido separadamente;
- para uma entrada, somente candidatos daquele acionador são avaliados;
- resultados frequentes podem ser cacheados pela tupla exata
  `(user_id, workspace_id, trigger_identity, source_type, context_version,
  registry_version, global_config_generation, workspace_config_generation,
  active_layers_generation)`;
- mudanças de contexto invalidam apenas entradas afetadas.

O cache positivo e o negativo exigem igualdade de todos os componentes; valor
nullable usa marcador tipado, nunca string vazia. Mudar catálogo, default,
binding, prioridade, camada ou claim incrementa a geração correspondente sob o
`DispatchGate` antes de publicar o novo snapshot, portanto entrada anterior não
é reutilizada nem para executar nem para repetir recusa stale. Usuário,
workspace e identidade normalizada do acionador nunca são inferidos apenas de
`context_version`.

Para o Stream Deck, a composição é recalculada quando camadas, contexto ou
estado visível mudam. O renderer compara o estado anterior e atual e envia ao
dispositivo somente teclas alteradas. Imagens redimensionadas ficam em cache.
Ao abrir ou reconectar um handle, invalida o estado renderizado daquele
dispositivo e força frame completo antes de voltar ao diff incremental.

### D13 — Stream Deck direto por Go

O Assistente controla o Stream Deck diretamente por USB/HID. Não há dependência
do aplicativo ou plugin oficial da Elgato.

O adapter deve:

- enumerar modelos suportados e suas geometrias;
- abrir e possuir exclusivamente cada dispositivo;
- receber eventos de tecla;
- renderizar imagem, título e estado;
- detectar remoção e reconectar com backoff;
- suportar mais de um dispositivo sem confundir identidade e posição;
- liberar os dispositivos no shutdown;
- degradar sem impedir a inicialização do Assistente.

Uma prova de conceito externa existente usa
`rafaelmartins.com/p/streamdeck` em Go. Antes de torná-la dependência do produto,
a implementação deve verificar licença, manutenção, modelos suportados,
reconexão, distribuição nos sistemas-alvo e compatibilidade com o build Wails.

Enquanto o Assistente possuir o dispositivo, outro processo, incluindo a prova
de conceito, pode não conseguir abri-lo. A UI deve informar essa disputa sem
encerrar o aplicativo.

O processo pode manter o dispositivo aberto antes do login, mas sem sessão
autenticada ele fica em estado seguro: imagem neutra ou apagada, sem bindings
ativos, e todo evento físico é rejeitado. No logout ou troca de usuário, o
gerenciador invalida atomicamente a geração da sessão, cancela despachos ainda
não iniciados, remove do estado em memória as camadas, bindings e caches do
usuário anterior — sem excluir sua persistência — e renderiza o estado seguro
antes de carregar outra conta. Callbacks carregam a geração da sessão e são
recusados se ficarem obsoletos. Somente depois de carregar e validar o novo mapa
ocorre nova renderização.

O estado `os.session_locked` faz parte do contexto de segurança. Ao bloquear a
estação, o adapter do sistema operacional incrementa a geração de segurança,
cancela invocações reservadas ainda não despachadas, renderiza o Stream Deck no
estado seguro e rejeita hotkeys globais e eventos de dispositivo. Não há
allowlist implícita durante o bloqueio. Ao desbloquear, bindings só voltam após
revalidar a sessão autenticada e reconstruir o mapa do usuário atual.

O estado visual de uma tecla é apresentação do binding efetivo. Pode ter título,
ícone padrão, imagem escolhida pelo usuário e variantes como ligado, desligado,
executando, concluído e erro. Texto, anúncio e estado não podem depender apenas
de imagem ou cor.

Apresentações builtin usam `title_key` e `status_label_keys` existentes em
pt-BR, en e es. Conteúdo personalizado pode fornecer `title_by_locale`; locale
ausente cai para o nome localizado do comando, nunca para string builtin
hardcoded. Anúncios usam as mesmas chaves/fallbacks.

Uma tecla que ativa outra camada oferece navegação semelhante a pasta, mas
continua usando o mecanismo genérico `layer.activate`, `layer.toggle` ou
`layer.back`. Teclado, chat ou outro dispositivo podem ativar a mesma camada.

### D14 — Contexto de programas externos

Quando o Assistente estiver sem foco, hotkeys globais e dispositivos físicos
podem usar camadas ativadas pelo programa em primeiro plano.

O adapter do SO captura processo, identidade da janela e versão em
`foreground_snapshot` no instante do evento, antes de qualquer
`WindowPort.Show`/bring-to-front. O resolvedor usa esse snapshot com
`context_policy = event_snapshot`; trazer o Assistente à frente não troca a
camada daquele evento. Snapshot só é aceito do adapter confiável e continua
sujeito a auth/security generation e limite de idade.

`foreground_snapshot` é transitório e não vai ao SQLite. A auditoria guarda
apenas `foreground_summary` allowlisted/redigido: identidade normalizada do
executável, classe da janela e versão do provider. Título, URL, documento,
caminho e texto da janela nunca são persistidos.

Exemplo:

```text
OBS em foco     → camada OBS
VS Code em foco → camada Desenvolvimento
nenhuma regra   → camada Global
```

Detectar foco não autoriza automaticamente controlar outro programa. Integrações
usam, em ordem de preferência:

1. comando ou API específica;
2. tool MCP autorizada;
3. job;
4. envio de teclas ao programa em primeiro plano, se uma capacidade privilegiada
   futura for explicitamente habilitada.

Injeção de teclado, shell e controle externo exigem política, confirmação e
auditoria próprias. Título de janela pode conter dados sensíveis e não deve ser
persistido ou enviado ao modelo sem necessidade.

Comandos de shell continuam obrigatoriamente passando pelo avaliador único
`internal/commandpolicy` definido na AEP-0060. `CommandExecutionService` não
implementa uma segunda allowlist nem transforma uma aprovação de camada em
autorização de shell.

A exposição na CLI não altera os non-goals da AEP-0045. A CLI pode listar e
descrever todo o catálogo, mas só executa comandos que declarem suporte à origem
`cli`, não dependam de runtime visual e tenham
`decision_requirement = none`. Comando perigoso ou que exija `DecisionDialog`
aparece indisponível com motivo e falha fechado no `CommandExecutionService`;
esta AEP não cria confirmação textual alternativa. Comandos de workspace,
editor ou foco também permanecem indisponíveis e esta AEP não leva essas
surfaces ao terminal.

### D15 — Interface de configuração por camadas

A entrada fica em **Configurações → Comandos e acionadores**. A tela inicial é
uma lista de camadas:

```text
Padrão do aplicativo   — sempre ativa
Chat                   — tela de chat
Desenvolvimento        — VS Code em foco
OBS                    — OBS em foco
Trabalho               — ativação manual
```

O detalhe de uma camada possui duas seções principais:

1. **Quando esta camada fica ativa**;
2. **Comandos desta camada**.

A lista de comandos mostra acionador, comando, condição e conflito. Opções
específicas aparecem somente quando relevantes; bindings de Stream Deck expõem
posição, imagem, título e estados, enquanto bindings de teclado oferecem captura
da combinação.

Camadas padrão podem ser inspecionadas. Editá-las cria overrides reversíveis,
sem alterar o default versionado.

Configuração avançada de prioridade e predicados permanece em divulgação
progressiva. O fluxo comum é encontrar um comando, capturar uma tecla e,
opcionalmente, definir quando funciona.

Toda operação deve funcionar em lista estruturada com teclado e leitor de telas.
Grade visual do Stream Deck é uma visualização opcional, nunca o único editor.
Reordenação oferece botões mover anterior/próximo e não depende de arrastar.

### D16 — Segurança e limites

- Deep links da AEP-0023 não ganham execução arbitrária de comandos.
- Acionadores não contornam permissões do comando executado.
- Tools MCP seguem catálogo, política e executor comum.
- Comandos perigosos fora de foco não são silenciosamente autorizados.
- Entrar em uma camada ou pasta não executa suas ações.
- Configurações importadas começam sem novos grants.
- Identidade de processo/janela é dado de contexto, não prova de confiança.
- Diálogos bloqueantes e foco obedecem AEP-0091.
- Eventos externos não ativam camadas nesta versão; suporte futuro exige
  contrato de identidade e grants em AEP própria.

## Fases

O [tasklist de infraestrutura e entrega](0103-tasklist-infraestrutura.md)
consolida a baseline de acompanhamento: pacotes, dependências, critérios de
saída e rastreabilidade dos critérios finais. Ele não altera os contratos desta
AEP nem declara concluídos os incrementos parciais abaixo. A infraestrutura e
a posterior migração/população de comandos possuem marcos separados.

### Evidência I01 — armazenamento e chaves operacionais

`internal/commandbootstrap` compõe as migrações de configuração, receipts e
ledger. A migração v20 `command_storage_initial` pertence ao registro central
da AEP-0076: fica adiada na abertura genérica do banco e é concluída pelo host
com dados e carimbo na mesma transação. O App chama a preparação após carregar
o cofre, tanto na inicialização quanto em sua reconfiguração. Não há executor,
mapa ou adapter habilitado por esse passo; prontidão de armazenamento é distinta
de prontidão de execução. Falha é retida e não impede autenticação legada.

O schema conhecido é validado antes de alteração e após migração, inclusive
quando já carimbado. Schema desconhecido/índice incompatível falha fechado e
preserva os dados. A comparação tolera somente reordenação de definições de
colunas/constraints de tabela; expressões e ordem de colunas dos índices são
preservadas. SQLite privado em memória fornece a referência a partir das mesmas
definições dos repositories, evitando manter uma segunda cópia do DDL.

`credentials.Manager.EnsureInstanceSecret` lê o armazenamento exato de instância
e usa insert-if-absent, nunca upsert destrutivo nem fallback user-scoped. Exige
persistência e cofre previamente validado. O vencedor de uma corrida é relido e
decifrado; entrada ilegível/vazia/tipo incorreto não é substituída.

`PrepareKeys` cria a chave de fingerprint apenas em armazenamento sem histórico
que dependa dela, valida base64 e vincula cada versão a digest não secreto em
`command_key_versions`. Ausência ou troca de chave já registrada falha fechado.
`RotateKeys` é uma porta interna de manutenção explícita com versão esperada e
CAS: conserva versões anteriores e não altera JWT, pepper ou DEK. Todas as
versões são conservadas indefinidamente nesta etapa, portanto nunca menos que
o horizonte dos ledgers; eventual coleta segura pertence a I12. Queda depois de
criar a próxima chave, mas antes de ativar a versão, permite retentar sem trocar
a chave. A futura composição do executor deve suspender admissões durante a
rotação e publicar a versão retornada; não existe endpoint público de rotação.

Testes usam bancos/DEKs sintéticos e cobrem primeira abertura, reabertura,
adoção do schema experimental, drift, rollback/carimbo, concorrência, cofre
indisponível, recuperação de prontidão e rotação. Validação integral de releases,
race, ferramentas de lint/review e integração final de execução continuam em
I15/I14; o AEP permanece In Progress.

### Evidência incremental — protótipo da Fase 0

O pacote `internal/commandbindings` inicia o experimento de seleção pura de
candidatos. Ainda não está conectado ao aplicativo e não executa handlers.
Não representa a conclusão da Fase 0 nem do registro/resolvedor da Fase 1.

- Implementado: índice por acionador, filtros de habilitação/ativação,
  precedência de escopos, conjunção de igualdades tipadas, conflito sem desempate
  por ID, agrupamento de destinos equivalentes e barreira de diálogo.
- A especificidade parcial é avaliada eliminando condições estritamente
  dominadas dentro do melhor escopo; prioridades comparam somente os máximos
  restantes. Não se usa ordenação com comparador parcial. O teste de todas as
  permutações cobre três candidatos com condições comparáveis e incomparáveis.
- No protótipo, condição por `surface.id` exige também `surface.type`,
  fornecido pelo normalizador confiável, para representar identidade antes de
  tipo por inclusão de cláusulas. Identidade sem tipo é recusada. Candidatos de
  diálogo possuem `DialogID` e somente os do topmost participam, inclusive na
  proveniência de equivalentes.
- Testes: `go test ./internal/commandbindings`; benchmark do índice:
  `go test ./internal/commandbindings -run '^$' -bench . -benchmem`.
  O benchmark distribui bindings por acionadores distintos; não mede a ponte
  Wails, persistência nem o pior caso de muitos candidatos na mesma tecla.
- Validação local inicial: testes (98,9% de cobertura) e `go vet` do pacote
  passaram, assim como build/vet do backend completo. A suíte geral não ficou
  verde: `config`/`wailsapi` falharam por acesso negado à pasta real `.assistente`
  e `acp`/`acpregistry` encerraram com `exit status 0xffffffff`. Esses pacotes
  não foram alterados neste incremento. `-race` ainda não foi validado: o ambiente Windows de teste
  está sem compilador C configurado e o Go exige cgo para esse detector.
- Limites: candidatos chegam já normalizados, com identidade de argumentos e
  alvo fornecida pelo chamador confiável. O protótipo não calcula fingerprint,
  não autentica e não valida o schema de argumentos do catálogo.
- Pendentes: completar catálogo e materialização de deltas/overrides/tombstones
  e `needs_review` (incrementos parciais descritos na Fase 1), claims,
  persistência, executor/ledger, providers autoritativos, reserva dos atalhos
  invariantes de diálogo, adapters físicos e migração de handlers.
  Um resultado `selected` é apenas seleção, nunca autorização de execução.

Pontos já identificados para o inventário, ainda não migrados:

O [inventário inicial de atalhos](0103-inventario-atalhos.md) detalha símbolos,
combinações, guards e testes existentes. É documento de apoio, não conclusão
da Fase 0; os IDs sugeridos ainda não estão registrados no produto.

- `frontend/src/hooks/useWorkspaceKeyboardShortcuts.ts`: abas e sequências
  `Ctrl+N` seguidas de letra; preservar cancelamento, timeout e foco.
- `frontend/src/hooks/useActivePanelShortcut.ts`: `Ctrl+N` do painel ativo.
- `frontend/src/components/chat/ChatToolbar.tsx`: seletores e ações do chat.
- `frontend/src/pages/useEditorMenus.tsx`: ações e modos do editor.
- `controllers/hotkeys_controller.go` e
  `frontend/src/hooks/useInteractionProfile.ts`: hotkeys de voz por perfil.
- `internal/jobs/manager.go`: hotkeys que solicitam triggers de jobs.

O próximo incremento deve completar o inventário e o catálogo de comandos,
definir os contratos de sequências/ponte de UI e validar os protótipos físicos.
Os critérios de aceitação finais permanecem abertos: testes do seletor isolado
não demonstram as garantias de execução, persistência e segurança do sistema.

### Fase 0 — Inventário e protótipos

- Inventariar atalhos locais, hotkeys de perfis, comandos de menu e ações
  executáveis existentes.
- Prototipar o registro de comandos e a resolução de camadas sem migrar handlers.
- Validar a biblioteca Go do Stream Deck, exclusividade, reconexão e modelos.
- Prototipar observação de janela em primeiro plano no Windows.
- Medir latência e estabilidade com muitas camadas e bindings.

### Fase 1 — Registro, defaults e resolvedor

Incrementos isolados adicionais (sem conexão ao dispatcher):

- `internal/commandcontext`: comparação de versões exatas e validade temporal
  dos fatos capturados. A autenticidade dos snapshots é pré-condição do host;
  `VersionService` registra providers injetados, captura fatos exigidos e
  calcula uma identidade contextual determinística de provider/fato/versão.
  Não implementa os providers reais do aplicativo, fingerprint de request
  RFC 8785, autorização ou revalidação atômica sob `DispatchGate`.
- `Configuration.WithoutDeltas`: restauração seletiva em um novo snapshot de
  memória, preservando as personalizações não removidas e seus ajustes de
  revisão. Não representa restore transacional no SQLite ou por camada.
- `internal/commandinput`: máquina de pressão/liberação com descarte de repeat
  e invalidação monotônica por geração. Ainda não registra hotkeys, não observa
  eventos DOM/SO/Stream Deck, não gera IDs de ocorrência e não arbitra ownership
  entre teclado local e global. O host deverá integrar esses contratos.
- `configuration_bench_test.go`: mede candidatos no mesmo acionador, além da
  distribuição entre buckets. A amostra local com 1000 candidatos na mesma
  tecla ficou em aproximadamente 4 ms e 2 MB/op; não constitui SLA e evidencia
  custo de materialização/seleção a otimizar antes de integrar o produto.

Validação conjunta desta rodada: testes de `commandbindings`, `commandcatalog`,
`commandcontext` e `commandinput` passaram, com coberturas de 92,9%, 99,2%, 96,0%
e 90,2%, respectivamente. Build e vet gerais também passaram. Os percentuais
medem instruções instrumentadas dos pacotes, não progresso do AEP.
Continuam pendentes a suíte geral verde, o detector de corrida com compilador C,
lint v2 e revisão Bugbot antes de push.

#### Integração inicial de diagnóstico (sem execução)

`internal/commandpreflight.Service.Inspect` conecta configuração de bindings,
catálogo e `VersionService` num fluxo interno restrito à origem teclado local.
O host injetado deve autenticar cada consulta e fornecer snapshots estáveis do
mesmo escopo de usuário/sessão dos providers. A entrada contém somente o
acionador normalizado; não escolhe usuário, origem, fatos ou permissões.
O serviço recusa contratos fora do subconjunto de leitura sem interação e sem
mutabilidade, revalida fatos, reconsulta o host e verifica novamente o TTL ao
final. Mudança de sessão, configuração ou contexto recusa o diagnóstico sem
resultado parcial.

`read_checks_passed` não é autorização nem token de despacho. `would_suppress`
é uma simulação que não consome a tecla nem cria marcador terminal. Não existe
rota de handler, invocação ou endpoint Wails nesta integração. Argumentos,
disponibilidade completa, autorização, receipts, ledger/auditoria duráveis e
gerações de segurança reais precisam existir antes de habilitar execução.
O teste de integração usa comando e providers de teste, não comando registrado
no produto. Os atalhos atuais permanecem intactos.

`internal/commandsecurity.DispatchGate` fornece o primitivo de admissão
compartilhada/mutação exclusiva, sem conectá-lo a logout ou handlers ainda.
O handoff não bloqueante deverá ocorrer dentro do gate; aguardar trabalho
longo ocorre fora. A espera pelo RWMutex não é cancelável imediatamente: o
cancelamento é observado antes e depois de adquirir o lock.

Os testes de integração cobrem leitura, recusa de escrita, contexto alterado,
logout/troca de usuário ou sessão, geração modificada, erro final sem resultado
parcial, snapshots equivalentes reconstruídos e TTL vencido durante a consulta
do host. Testes focados dos pacotes envolvidos, build e vet gerais passaram.
Essa evidência não substitui o ledger durável, validação NVDA, suíte geral verde
ou revisão Bugbot; nenhum critério de execução ponta a ponta está concluído.

#### Persistência inicial isolada (sem execução)

`internal/commandledger` inicia as tabelas `command_idempotency_keys` e
`command_invocations`, com migração explícita, sem bootstrap no aplicativo.
A API interna aceita somente solicitações diretas `local_session` de leitura
sem argumentos/contexto, nas origens `palette`, `ui.action` e `cli`. O chamador
confiável ainda deve autenticar, validar o contrato e produzir os HMACs; o
repositório compara fingerprints, mas não os autentica nem autoriza execução.

Reserva e auditoria são transacionais. A reentrega verifica usuário, sessão,
origem e fingerprint, sem renovar a expiração. Transições CAS atualizam ambos
os registros ou revertem a transação; terminais não são reiniciados pela API.
Os testes usam SQLite em arquivos temporários e incluem reabertura do banco,
rollback por falha da auditoria e isolamento de escopo.

`RecoverClosedGeneration` acrescenta recuperação interna explícita de uma
sessão local e geração de segurança encerrada: pares `evaluating`, `queued`
ou `running` passam atomicamente para `outcome_unknown`, sem novo efeito.
O host precisa comprovar o encerramento e impedir admissão da geração antes
da chamada; o repositório não deduz isso de relógio ou expiração. Gerações e
sessões fora do escopo e estados terminais são preservados. Testes cobrem
idempotência, replay, isolamento, rollback e par com fingerprint divergente.

`Reconcile` fornece o CAS interno `outcome_unknown → succeeded|failed` após
consulta por `OutcomeVerifier` confiável, fora da transação SQLite. A consulta
recebe uma cópia do registro escopado; falha, cancelamento e outcome inconclusivo
preservam a incerteza. O commit confere novamente identidade, fingerprint e
estado, atualiza ledger e auditoria juntos e não sobrescreve um terminal que
venceu a corrida. O subconjunto atual não produz retorno além de `{}`.
O host ainda precisa selecionar o verificador pelo comando, reaplicar
autenticação/autorização e consultar uma fonte conclusiva sem executar efeitos.
Nenhum verificador de produto ou endpoint está registrado; os verificadores
dos testes não constituem prova de reconciliação ponta a ponta.

O harness exclusivo de teste `pipeline_integration_test.go` combina catálogo,
ledger SQLite e DispatchGate para uma leitura direta de fixture: reserva,
validação estática, CAS de fila/running, handoff, conclusão e replay sem nova
chamada. A espera do resultado libera o gate para mutações. Autenticação,
autorização, identidade e handler são substitutos explícitos de teste, não
serviços de produto. O HMAC usa `SignLocalRead` com chave exclusiva de fixture.
O harness não é executor reutilizável e não
cobre o protocolo completo de cancelamento/panic/receipts. Essa evidência não
habilita comandos reais nem conclui a execução ponta a ponta exigida pelo AEP.

`SignLocalRead` implementa HMAC-SHA256 para a projeção fechada de leitura direta
local sem argumentos/contexto. Recebe somente envelope interno já derivado e
recusa fingerprints preenchidos no ingresso. A projeção JCS contém `version:1`,
IDs de invocação/correlação, comando, usuário/ator, contexto autenticado/sessão,
geração de segurança, origem, versões de catálogo/configuração/camadas e
constantes `effect:read`, `decision:none`, `context_policy:none`, `arguments:{}`
e `binding_ids:[]`. Omite auth_generation e timestamps; opcionais fora do
subconjunto permanecem ausentes. O HMAC de argumentos cobre `{}` separadamente.
As saídas são hexadecimal minúsculo. O serializador é restrito a essa projeção
(chaves ASCII fixas, strings Unicode válidas e constantes), não JSON genérico;
segue escapes e preservação de Unicode da RFC8785, sem normalização.

O provider injetado recebe `command-request-hmac:<versão>` e deve devolver
chave de pelo menos 32 bytes; não existe geração/fallback automático. Testes
comparam um documento canônico literal e exercitam alterações semânticas,
reentrega SQLite com versão retida e recusa quando a chave antiga desaparece.
O host ainda precisa derivar/autenticar a identidade, escolher a versão por
ledger escopado no retry, manter chaves no secret manager e validar a política
antes de assinar. `NewCredentialKeyProvider` conecta a leitura ao Manager já
inicializado: o nome lógico `command-request-hmac:vN` mapeia para o segredo
de instância `internal-auth:command-request-hmac:vN`, tipo `secret`, codificado
em base64url canônico sem padding. A consulta é exata e estritamente sem usuário,
sem o fallback legado de `GetInstanceSecret`. Chave ausente, curta, malformada ou
credencial user-scoped falha fechado. O adapter não cria nem substitui chaves,
não inicializa o cofre e não registra bootstrap. Testes usam Manager real com
criptografia em memória, sem keychain, e reserva SQLite em arquivo temporário.
Provisionamento, rotação, retenção operacionais e integração no host ainda faltam.

`SessionService.AuthenticateLocalAccess` adiciona consulta interna de identidade
para comandos locais. Reutiliza a verificação JWT existente e confere em uma
leitura SQLite a relação sessão/usuário, conta ativa e ausência de revogação;
confere também expiração da sessão e do access token ao final da consulta.
Devolve somente user_id/session_id UUIDv7, sem role, token ou gerações. Não
modifica `VerifyAccessToken`, middleware externo nem o login vigente. O teste
`TestAuthenticatedRequestUsesSessionIdentityAndRejectsLogout` usa sessão/JWT,
Manager e ledger reais em armazenamento de teste: deriva ownership no backend
e recusa nova reserva após logout mesmo com assinatura JWT ainda válida.
Esta leitura não é autorização nem elimina corridas após retornar: todas as
origens de revogação ainda precisam participar da coordenação do host.

`commandsecurity.EpochService` fornece gerações locais em memória sobre um
DispatchGate injetado: identidade aleatória de startup, contador sem reuso,
auth_generation por sessão e security_generation global. Capture recebe IDs
já autenticados; Admit compara snapshot, chama revalidação autoritativa e
handoff sob o mesmo gate compartilhado. Invalidação de sessão é local;
invalidação de principal altera sessão e segurança atomicamente; invalidação
global torna todos os snapshots anteriores obsoletos. Overflow falha fechado.
Testes verificam o lock durante revalidação/handoff e liberação após retorno.
O teste integrado de identidade usa essas gerações e consulta SessionService
sob Admit: logout recusa handoff mesmo antes da invalidação observada do epoch.
Capture
não autentica, não representa estado locked e não deve readquirir um gate já
detido. Lifecycle/limpeza de sessões fora das transições e suporte
system/external/job continuam pendentes; nenhuma execução real é
habilitada por este incremento.

`MutateSession`, `MutatePrincipal` e `MutateSecurity` coordenam uma mutação
autoritativa curta com a invalidação sob o mesmo gate exclusivo. Invalidam antes
do callback e mantêm a invalidação em erro/panic, pois pode ter ocorrido efeito
parcial; não prometem rollback do banco. IDs/escopo e callback são fornecidos pelo
host confiável, nunca por payload. Callbacks não podem readquirir o gate nem
aguardar interação, rede ou conclusão de handlers. O teste de sessão/ledger
agora também executa `SessionService.Logout` dentro de MutatePrincipal; snapshots
anteriores são recusados por staleness e o token revogado não reserva novamente.
Teste concorrente verifica exclusão durante a mutação e recusa do snapshot
antigo na admissão.

`BeginTransition` acrescenta uma barreira para operações longas: invalida todos
os snapshots e impede Capture/Admit enquanto houver transições abertas, sem
manter o gate durante I/O ou parada de runtimes. Encerramentos são idempotentes,
aninháveis e independentes do cancelamento da operação. Exaustão de gerações
desabilita novas admissões permanentemente naquela instância.
O App mantém uma instância inicializada sob demanda por sync.Once e chama a
barreira em Login, RefreshAuth, Logout, rollbackLoginState, SetupVault e
UnlockVault. A política inicial é conservadora: inclusive refresh e tentativas
de autenticação que falham invalidam snapshots de comandos. O retorno legado
dessas operações é preservado; falha de inicialização deixa comandos indisponíveis,
sem impedir o logout existente. A ordem é authSessionMu (quando aplicável),
gate e authMu; callbacks de admissão não podem adquirir authSessionMu.
Testes focados do App verificam os hooks em falhas precoces e Logout real com
sessão SQLite e keychain substituído por callbacks de teste. Permanecem pendentes
outras origens de revogação, bloqueio do SO, executor e refinamento do refresh.

`CaptureAuthenticated` fecha a janela entre autenticação local e captura de
gerações: o callback confiável consulta a identidade autoritativa sob o mesmo
gate exclusivo que publica o snapshot. Uma transição coordenada não pode entrar
entre essas etapas. Callback inválido, identidade inválida, erro ou cancelamento
não publicam snapshot nem sessão; transição aberta recusa antes da consulta.
O callback deve fazer somente consulta local curta e não pode readquirir o gate,
inicializar cofre, aguardar UI/rede ou executar handlers. A API anterior Capture
permanece primitiva de baixo nível, não autenticação para requests de produto.
O teste integrado de sessão/ledger deriva ownership e ambas as gerações da mesma
captura em cada reserva e compara a identidade exata ao revalidar a admissão.
Testes unitários verificam exclusão, cancelamento e liberação do gate em panic.
Isso não concede autorização nem substitui os gates finais de fila/despacho;
o executor de produto e a coordenação de todas as fontes de revogação continuam
pendentes.

Esta projeção não cobre comandos com argumentos, providers,
receipts, delegação ou eventos e não habilita o executor de produto.

Permanecem pendentes integração ao startup e ativação no produto,
comprovação de encerramento de geração, verificadores reais de reconciliação,
ampliação da projeção HMAC/RFC8785 e integração ao secret manager,
resultados, política de retenção, eventos,
supressão, identidades externas e constraints condicionais completas de D11.
As colunas futuras não tornam esses fluxos suportados. Nenhuma fase ou critério
de execução ponta a ponta é concluído por este incremento.

#### Executor interno de leituras diretas (ainda não exposto no produto)

`internal/commandexecution.Service` passa a integrar o caminho que antes existia
somente em harness: sessão local real, captura autenticada de gerações/catálogo,
HMAC, reserva durável, política, CAS de fila/running, Start e conclusão.
O ingresso recebe somente IDs de invocação/correlação e command_id; identidade,
origem (fixa no adapter), relógio, chave e versões são derivados pelo backend.
O construtor exige dependências explícitas e copia as rotas de handlers; recusa
contratos não read/none, alvos mutáveis, argumentos/providers (fora da API),
origens não suportadas e ausência de política. Nenhum handler é inferido por nome.

Autenticação da mesma sessão, lock, versões e autorização são reconsultados nos
dois gates: evaluating→queued e queued→running. Start ocorre sincronamente sob
o gate após o CAS, mas deve apenas devolver um handle não bloqueante. Espera e
Cancel ocorrem fora do gate. Falha/panic antes de Start é failed; erro/panic ao
entrar em Start, handle inválido, canal perdido ou cancelamento sem confirmação
produzem outcome_unknown. Outcomes explícitos succeeded/failed/cancelled são
persistidos com CAS; não há retry do handler. A finalização tem contexto e prazo
próprios, para não perder o registro ao cancelar o chamador. Falha de persistência
é devolvida e nunca tratada como autorização para nova execução.
O ledger grava policy_decision allowed/denied junto ao CAS correspondente e
admite queued→failed para falha conclusiva anterior ao handoff.

`NewLocalReadAuthorizer` fornece a política local inicial: allowlist de roles por
command_id do host e consulta da conta/sessão no banco a cada gate, sem confiar
na role do JWT. Comando/role não listado falha fechado, inclusive para admin.
Atualizações de role/revogação devem participar do mesmo gate; essa política
não torna automaticamente coordenados endpoints legados de mutação.
`App.newCommandReadExecutor` é fábrica interna sem rota Wails: injeta o serviço
de sessão, EpochService e provider de chave do Manager já carregado, exige a
identidade ativa exata e recusa troca de SessionService. Não inicializa, gera
ou grava segredos. Deve ser construído em bootstrap serializado.

Testes do serviço usam sessão/JWT, SQLite e HMAC reais com dados temporários;
testes do App exercitam a fábrica e logout enquanto o handler aguarda resultado,
sem keychain real. Políticas de teste são explícitas, não defaults de produção.
Evidências: `TestServiceConcurrentReplaysObserveRunningWithoutNewHandoff`
verifica seis reentregas concorrentes com um único Start;
`TestServiceFailedQueueCommitNeverStartsOrOverwritesLedger` cobre falha de CAS;
`TestServiceRealPolicyRejectsChangedRoleDespiteValidJWT` cobre a role vigente;
`TestServiceReplayUsesStoredKeyVersionWithoutFallbackForNewRequest` cobre a
versão de chave vinculada ao ledger; e
`TestCommandExecutionAppUsesInstanceKeyAndRejectsLogout` cobre a fábrica do App.

Limites deste recorte: somente palette/ui.action/cli, read/none sem workspace;
fila lógica com retirada imediata (sem scheduler compartilhado); sem payload de
resultado além do status/resumo vazio atual. Execute nunca retoma uma reserva
existente. GetInvocation reautoriza sem criar reserva nem chamar handler, mas
ainda exige request original e fingerprint reproduzível nas gerações atuais;
consulta após mudança de gerações pode falhar como conflito, não reexecutar.
Ainda faltam consulta histórica independente de versões, recusas auditadas para
todo envelope não suportado, log de segurança pré-autenticação, fontes completas
de eventos autoritativos de lock/versões,
provisionamento de chaves e registro de comandos/rotas de produto. Não se habilita
auth.mode=external, atalhos, UI, receipts ou efeitos de escrita neste bloco.

#### Estado do host e cancelamento por invalidação

`EpochService.AdmitExecution` associa um contexto de execução ao snapshot dentro
do mesmo gate compartilhado que revalida e entra em Start. A inscrição não tem
janela após o handoff. Invalidação de sessão cancela apenas os contextos daquela
sessão; mudança de segurança/principal e BeginTransition cancelam todos. A
invalidação fecha apenas contextos internos, nunca chama Cancel do handler sob
o gate. O executor observa esse cancelamento e chama Cancel fora do gate,
persistindo outcome_unknown quando não houve confirmação conclusiva. Release
idempotente remove a inscrição ao terminar; erro/panic de handoff também limpa
a inscrição. Resultado tardio não sobrescreve o terminal. Isso não desfaz um
efeito que já tenha começado.

`MutateUserConfiguration` publica sob gate exclusivo e cancela execuções apenas
do usuário afetado, sem alterar security_generation de outras contas. O
callback deve avançar as gerações de configuração aplicáveis. Erro ou tentativa
de publicação sem mudança efetiva pode cancelar conservadoramente execuções do
próprio usuário; não reativa contexto já cancelado.

`commandexecution.HostState` substitui versões inventadas pelo chamador na
fábrica do App: mantém Configuration imutável, lista detached de camadas ativas e
gerações globais por usuário. Gerações usam startup UUIDv7 e contador monotônico;
overflow desabilita novas leituras do estado. ForgetUserConfiguration remove
somente o snapshot em memória e republicação nunca reutiliza gerações.
Snapshot usa mutex curto próprio, sem readquirir o DispatchGate. As mutações
passam pelo EpochService da mesma instância; nenhuma conta recebe gerações de
outra conta.

O estado começa com cofre fechado e sessão do SO desconhecida. Só sinaliza
Unlocked com cofre aberto E sessão do SO conhecida e desbloqueada; cofre aberto
sozinho não libera comandos. Setters recebem fatos de adapters confiáveis, não
de payload/UI. A fábrica exige HostState explícito com o mesmo EpochService e
não aceita que Config.Snapshot substitua esse estado. A instalação no App é
serializada e rejeita substituir a instância de HostState já instalada.

Login descarta o mapa anterior (inclusive tentativa que falha); logout e
rollback descartam mapa e marcam o cofre fechado. SetupVault/UnlockVault bem
sucedidos e SetupMasterPassword atualizam a observação de cofre; o caminho
legado de SetupMasterPassword também passa pela barreira de transição. Os
retornos legados são preservados e nenhum desses hooks consulta keychain sob
o gate. Testes de App verificam falhas precoces sem I/O real, remoção do mapa,
republicação sem reuso de gerações e logout com cancelamento do executor.

Reconstrução autenticada do mapa: `RebuildUserConfiguration` captura a sessão
local e suas gerações, constrói o snapshot fora do gate e reautentica a mesma
identidade antes de publicar sob gate exclusivo. Mudanças de segurança,
autenticação ou configuração durante o carregamento descartam o resultado.
A revisão de carregamento é conservadora e global nesta instância: mudanças de
outra conta também podem recusar um carregamento concorrente, sem invalidar
execuções já admitidas dessa outra conta. Não há retry implícito.

Cada observação do SO remove os mapas anteriores, inclusive unlock e estado
desconhecido. Publicação simples de configuração não torna bindings utilizáveis:
somente a reconstrução autenticada associa o mapa à sessão exata. O hook interno
do App autentica JWT e sessão persistida antes/depois do builder e confere a
identidade atual e as dependências da instância. Não guarda token, não consulta
keychain e não permite que um builder atrasado ressuscite um mapa removido.

Exaustão do contador de segurança também desabilita o EpochService e cancela
contextos admitidos, sem executar a mutação autoritativa nem reutilizar geração.
Assim, falha ao invalidar na parada do observador não preserva admissões antigas.

Adapter nativo `internal/ossession`: usa janela message-only e notificações
WTS da sessão do processo, consulta WTSInfoEx após registrar o observador e só
aceita desbloqueio com sessão ativa e flags conhecidas. Lock invalida mesmo
que uma consulta posterior já veja unlock. Desconexão, erro e encerramento
deixam o estado desconhecido/fechado. Plataformas não suportadas recusam a
observação; não há inferência por cofre ou JWT. Referências oficiais:
[WTSRegisterSessionNotification](https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/nf-wtsapi32-wtsregistersessionnotification),
[WM_WTSSESSION_CHANGE](https://learn.microsoft.com/en-us/windows/win32/termserv/wm-wtssession-change)
e [WTSINFOEX_LEVEL1_W](https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/ns-wtsapi32-wtsinfoex_level1_w).

A fábrica interna do executor instala um único monitor quando o ciclo de vida
do App já está iniciado e acompanha seu encerramento via bgWG/contexto de
shutdown. Erro do monitor mantém o host fechado, sem reinício silencioso.
O fim do startup também verifica um host previamente instalado. A fábrica
ainda não é chamada pelo startup de produto. Os testes do App
injetam um observador falso; validação manual com lock/unlock real do Windows
permanece pendente, sem bloquear a estação nem consultar segredos em testes.
O pump verifica cancelamento em esperas de até 100 ms; chamadas Win32 síncronas
não recebem cancelamento forçado. Um timeout de limpeza retorna erro e fecha
o host, mas a liberação nativa ainda depende de a chamada do SO retornar.
Falha terminal da fonte descarta eventos enfileirados obsoletos, sem reproduzir
unlock depois de perder a observação.

Evidências deste bloco: testes de reconstrução, publicação exclusiva e
invalidação por overflow; testes de decoder/eventos WTS com fontes falsas;
testes de App para sessão divergente, JWT inválido, remoção durante carregamento,
encerramento do monitor e cancelamento do executor. Build e vet do repositório
e testes focados dos pacotes afetados são a validação local; o teste nativo
interativo e a suíte completa do App permanecem fora desta evidência.

Leitura persistida inicial: `internal/commandconfig` fornece migração explícita
de `command_layers`, `command_bindings` e `command_config_generations`, sem
registrá-la no banco de produto. O carregador lê camadas, bindings e gerações
na mesma transação SQLite. Escopo global carrega só globais; workspace carrega
globais mais exatamente o workspace autenticado. Referências user precisam de
camada do mesmo owner/escopo; builtin exige delta e validação posterior no
catálogo. Ausência de geração não cria defaults nem inventa uma versão.

Snapshot conserva documentos JSON opacos, disabled e needs_review; não gera
Candidate nem ativa camadas. Documentos ainda exigem validação de versão/schema,
catálogo, defaults e referências a segredos pelo projetor confiável. Não existe
projetor permissivo de produto. Um stamp privado, independente dos campos
mutáveis entregues ao projetor, permite reconsultar as gerações antes de publicar.
Troca/remoção da geração e snapshot de outro Store falham fechado.

`rebuildPersistedCommandConfiguration` conecta esse carregador à reconstrução
autenticada do App, obrigando um projetor fornecido pelo bootstrap. O owner vem
do JWT/sessão local revalidada; a geração persistida é conferida novamente sob
DispatchGate. Essa borda aceita somente configuração global enquanto HostState
não isola workspaces. Nenhuma claim manual/evento é restaurada por esse caminho.
Escritores devem alterar dados e geração na mesma transação sob o gate;
escrita direta fora desse protocolo não é suportada. O carregador não oferece
CRUD/Wails ou migração automática.

Evidência do carregador: testes SQLite temporários exercitam constraints
isoladamente, rollback da migração, índices incompatíveis, ownership e
isolamento global/workspace. Um teste com duas conexões e commit concorrente
confirma snapshot consistente, seguido de recusa na revalidação. Testes do
stamp cobrem troca de ID sem avanço de geração e mutação de slices/pointers
entregues ao projetor. A integração do App cobre geração ausente e alterada
durante a projeção, usando somente banco e chaves de fixture.

#### Projeção estrita de leitura local (subconjunto interno)

`commandconfig.ProjectLocalRead` liga os documentos à configuração pura do
resolver. O subconjunto atual aceita somente escopo global de armazenamento,
`keyboard.local`, comandos de catálogo `read`, sem decisão, `Context.None`, sem
alvo mutável e sem alteração de capacidade. Uma allowlist confiável adicional
declara quais handlers não recebem argumentos; o catálogo atual ainda não
descreve esse contrato. Argumentos e apresentação devem ser objetos vazios.
Não há aceitação de argumentos secretos, templates ou referências a tools.

O formato **interno** v1 do acionador é
`{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`.
Usa códigos físicos fechados (letras, dígitos, F1–F24 e navegação básica), com
modificadores Control/Alt/Shift/Meta únicos, normalizados nessa ordem. Não é
ainda contrato de API pública nem adapter DOM/nativo. A condição usa
`{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true}]}`:
conjunção dos campos tipados já suportados pelo resolver, sem campos repetidos.
Versões futuras, campos desconhecidos, chaves JSON duplicadas, tipos incorretos
e documentos excedendo limites são recusados. Até registros desabilitados são
validados; um documento inválido recusa o mapa inteiro, sem fallback permissivo.

Defaults e camadas builtin vêm exclusivamente do bootstrap, com fingerprint
semântico fornecido por ele, sem hash da projeção parcial. Este subconjunto só
aceita defaults de escopo Global. Deltas preservam supressões, invariantes e
`needs_review` do resolver; não persistem automaticamente ajustes de versão.
Referências builtin são conferidas contra as camadas declaradas e a camada do
default. Camadas novas usam precedência ExplicitLayer e só participam quando
habilitadas **e** explicitamente ativas no contexto confiável do chamador.

`rebuildPersistedLocalReadConfiguration` integra esse projetor ao caminho de
reauth/publicação/revalidação de geração do App. Nesse caminho, camadas de
usuário continuam inativas e qualquer tentativa de passar uma lista de claims
é recusada: restore autenticado de claims permanece pendente. Testes usam
apenas SQLite temporário e catálogo/handlers de fixture. Nenhum binding de
produto ou adapter físico é registrado por esse helper.

#### Escrita preparada de enabled (primitivas internas)

`PrepareBindingEnabled` prepara exclusivamente habilitar/desabilitar um binding
global existente. Valida o mapa antes/depois, recusa no-op, documentos fora do
subconjunto e qualquer ajuste/revisão pendente de defaults. A proposta mantém
dados e geração privados; `Diff` retorna cópias independentes, sem aceitar de
volta payload editado pelo cliente. Preparar não escreve nem ativa camadas.

`CommitBindingEnabled` é uma primitiva de repository, não autorização. Sob o
gate exclusivo do host, faz CAS do ID e valor da geração, compara o binding
integral com o estado preparado e troca somente `enabled`. Dados e incremento
da geração pertencem à mesma transação SQLite. Conflito, remoção, erro de
escrita ou cancelamento revertem ambos; replay não reaplica a proposta e
esgotamento de int64 não recicla a geração. Não há retry automático.

`HostState.ChangeUserConfiguration` captura sessão/segurança/revisão do host,
aguarda preparação/decisão fora do gate e revalida antes do commit. Cancela
execuções do usuário e remove seu mapa antes de chamar o escritor; erro/panic
do escritor nunca restaura cache antigo. Outras contas conservam seus mapas.
Uma reconstrução autenticada separada é obrigatória após a tentativa de commit.

O helper não exportado `changeCommandBindingEnabled` conecta esse caminho ao
JWT/sessão atual do App e exige um adapter **confiável** de confirmação do diff.
Exige também política autoritativa de escrita, reavaliada sob o gate antes da
preparação e do commit; ownership ou confirmação não substituem essa política.
Não aceita booleano de aprovação vindo de payload. Esse seam é testado com
decisões de fixture: ainda NÃO está ligado ao DecisionDialog da AEP-0091, ao
ledger/auditoria de comandos write ou a Wails/tools. Portanto não constitui
uma funcionalidade de configuração pronta para uso no produto.

Evidências: SQLite temporário cobre commit, replay, concorrência, ABA, rollback,
overflow e cópias de diff; testes do host/App cobrem negação, sessão inválida,
lock durante confirmação, geração alterada durante espera e descarte de mapa.
Não há migração automática, criação de bindings/camadas/gerações, restore,
rebase ou persistência de claims neste subconjunto.

#### Recibos de decisão e adapter local (subconjunto interno)

`commanddecision` implementa pedidos de `config_mutation`/`local_session` com
IDs UUIDv7, usuário/sessão, fingerprint fornecido pelo produtor confiável,
gerações e expiração. As ações deste subconjunto são fechadas: `apply` e
`deny`. `Decide` registra `pending` antes de invocar o presenter e faz CAS
único para accepted/denied/cancelled/expired, com evento na mesma transação.
IDs divergentes, ações desconhecidas, erros, panic e cancelamento não concedem
aprovação. O prazo absoluto inclui a espera na fila de diálogos.

`Consume` confere todos os vínculos atuais, ação afirmativa e prazo, e grava
consumo/evento na mesma transação do efeito SQLite fornecido pelo host. O
callback só pode usar a transação recebida; falha reverte as três partes.
Reentrega não consome novamente. O serviço não autentica nem adquire o gate:
essa revalidação deve ocorrer no executor/host confiável antes de consumi-lo.
O fingerprint não é calculado por esse repository, e Body/diff nunca é
persistido nos recibos ou nos eventos. Os timestamps desse subconjunto SQLite
são inteiros Unix em milissegundos. Migração continua explícita, fora do App.

`commandDecisionPresenter` usa o `questionnaire.Manager` existente com
`kind=decision`, ações com polaridade/escopo explícitos, corpo documental e
rótulos pt-BR/en/es. O ID curto do questionário serve só ao transporte da UI:
o adapter anexa o UUIDv7 backend à resposta. Não interpreta rótulos traduzidos
como ações e não aceita ID de decisão injetado nas respostas.

O caminho interno `changeCommandBindingEnabledWithDecision` agora compõe essas
peças com o escritor real de bindings. `ChangeUserConfigurationWithEpoch`
entrega à preparação as gerações capturadas junto da autenticação, sem nova
captura fora do gate. `ConfirmBindingEnabled` vincula UUIDv7 privado da proposta,
usuário/sessão, gerações de autenticação/segurança, ID/valor da geração global e
documentos completos antes/depois a um HMAC versionado. O produtor não recebe
fingerprint, ID de decisão ou aprovação da UI. O renderizador confiável recebe
cópias do diff; o texto apresentado não fica no comprovante interno retornado.

`SignConfigurationMutation` usa a chave já reservada
`command-request-hmac:vN`, com domínio/ação próprios para configuração global.
O envelope fechado é JCS de strings; a geração int64 é uma string decimal e
os dois documentos de snapshot são strings JSON opacas, assinadas byte a byte,
não JSON arbitrário recanonizado. Isso é deliberadamente conservador: nenhuma
mudança nos bytes do snapshot privado é ignorada. A leitura da chave e a espera
pelo diálogo ficam fora do gate; o commit não consulta o Credential Manager.
Este incremento não cria segredos nem reutiliza JWT/refresh pepper.

`CommitConfirmedBindingEnabled` usa `ConsumeForDatabase` para verificar que
configuração e recibos compartilham a mesma raiz `sql.DB`, recusando outro banco
ou transação pré-aberta. Uma única transação consome a receipt, registra o
evento de consumo, faz CAS da geração e grava `enabled`. Não há commit interno
ou savepoint independente no escritor. Prazo vencido após o UPDATE, conflito,
erro de escrita ou erro de evento revertem consumo e configuração juntos.
O host reautentica/reautoriza e verifica o epoch original sob o gate exclusivo
antes de invalidar o mapa e entregar o commit. O mapa não é restaurado em falha.

Testes de integração usam SQLite temporário e o presenter real do App com o
Manager de questionários: token inválido, negação, política revogada, lock
durante o diálogo e sucesso confirmado. Testes de repository cobrem replay,
propostas concorrentes, CAS obsoleto, vínculo de epoch, outro banco, rollback e
expiração precisamente após a escrita. O caminho anterior de callback simples
continua como seam interno de testes, sem entrypoint de produto.

Validação deste incremento: dez pacotes do núcleo, testes focados do App,
`go build ./...`, `go vet ./...` e verificador dos AEPs passaram. Concorrência
de propostas e expiração após UPDATE passaram dez repetições. Não se declara
suíte geral verde: sua tentativa encontrou testes legados de `internal/config`
tentando escrever a configuração real do usuário, recusados pelo sandbox.
Os novos testes usam bancos temporários; não há habilitação no produto.

O commit confirmado agora também grava `command_config_mutations`, com IDs,
sessão, fingerprint, receipt e valores antes/depois, sem texto sensível. Falha
nessa inserção reverte binding, geração, consumo e evento. A migração explícita
valida o schema esperado; tabela/view incompatível aborta toda a migração, sem
reescrever o objeto existente. Constraints recusam UUIDs inválidos, decisão
duplicada, no-op e incremento de geração incorreto. Testes cobrem esses casos,
consulta escopada, replay e rollback completo; o teste do App confirma a linha
de auditoria correspondente à única alteração bem-sucedida.

`EpochService.WatchEpoch` agora liga a preparação/decisão ao epoch capturado.
A inscrição ocorre sob gate com revalidação, portanto invalidação entre captura
e inscrição não é perdida. Lock/logout/invalidação cancela o contexto da espera,
sem chamar UI sob o gate. `HostState` libera essa inscrição antes da própria
publicação e mantém a revalidação atômica original para o commit. O teste do App
bloqueia a sessão com o diálogo aberto, não envia resposta da UI e exige
cancelamento, sem esperar o prazo do diálogo. Testes adicionais cobrem isolamento
entre sessões e liberação idempotente das inscrições.

Ainda falta registrar a invocação de write no ledger quando o comando de produto
for registrado: nem histórico de receipt nem auditoria de configuração são
substitutos da auditoria completa de execução.

`commanddecision.ReconcileSession` agora recupera recibos `pending`/`accepted`
da sessão autenticada: prazo vencido vira `expired`; gerações anteriores viram
`cancelled`. Recibos atuais válidos, estados encerrados e outras sessões/contas
permanecem intactos. Cada lote aceita até 128 linhas, em ordem de UUID, e devolve
`More` quando o snapshot contém outros candidatos. Estado e evento são gravados
na mesma transação; erro/cancelamento reverte o lote e não reporta progresso
parcial. O índice por usuário/sessão/status/ID evita varredura entre contas;
a migração recusa índice homônimo incompatível. Não há reapresentação, consumo,
efeito, exclusão ou reativação automática. O horário da resposta anterior é
preservado quando existente; o evento registra o instante do encerramento.

`recoverCommandDecisionSession` compõe a operação com JWT, sessão atual do App,
política, cofre e estado do SO revalidados sob o gate. Processa um lote por
chamada e invalida o mapa antes da tentativa, sem loop prolongado sob a trava.
O chamador deve terminar os lotes e reconstruir o mapa autenticadamente. Testes
do App recuperam a confirmação obsoleta sem repetir binding/auditoria nem abrir
diálogo, recusam token/política inválidos e comprovam idempotência. Testes de
repository cobrem limites, isolamento, recuperação de Store recriado,
concorrência e rollback por falha no evento.

`HostState.RebuildUserConfiguration` também inscreve o carregamento no epoch
capturado, com revalidação da janela entre captura e inscrição. Lock, logout
ou invalidação cancelam o contexto entregue ao builder/projetor, permitindo
interromper I/O cooperativo antes de terminar a leitura. A inscrição é liberada
em erro, configuração inválida, panic e sucesso, antes da publicação. O commit
continua usando o contexto original e revalidando sessão, epoch e revisão do
host: cancelamento não substitui essas verificações. Testes no host e na borda
autenticada do App cobrem cancelamento durante projeção, perda da observação do
SO e reconstrução posterior. Builders que ignoram contexto não são interrompidos
à força; seus resultados obsoletos continuam recusados. Não há trabalho novo
no caminho de resolução por tecla nem promessa de latência da trava/SQLite.

Permanecem pendentes bootstrap autenticado do presenter e chamada automática da
recuperação antes de publicar o mapa. Esta recuperação é estritamente da sessão
retomada: manutenção de recibos de outras sessões abandonadas, retenção e
recuperação completa de invocações ainda não estão integradas. Não há entrypoint Wails/tool novo ou alteração
de configuração acessível ao usuário por esse incremento.

Pendente: ampliar o projetor para contratos de produto, integrar o ledger
de write, ampliar escritores para CRUD/restauração confirmada,
persistência/restore de claims, ligação completa à recuperação pós-unlock e
estado de execução por workspace. O estado do SO não
é inferido da presença de uma sessão/JWT ou da disponibilidade do cofre; sem
observação válida, o host continua fechado. Não há atalhos nem comandos de
produto ativados.

Incremento inicial: `internal/commandcatalog` contém um snapshot imutável dos
contratos estáticos de comando, com IDs exatos e namespaced, efeitos,
mutabilidade, origens permitidas e políticas de contexto por provider/fato.
O registro recusa metadata divergente do descriptor confiável do handler,
escrita/ação com alvo mutável usando contexto `none`, políticas temporais sem
TTL positivo e qualquer origem `cli`/`event`/`system` em comando destrutivo.
`AllowsSource` consulta somente a declaração; não concede autorização.

Validação: `go test ./internal/commandcatalog ./internal/commandbindings`
passou (100% de cobertura no catálogo inicial, 98,9% no seletor); `go vet`
dos dois pacotes passou. Nenhum comando do produto está registrado ainda.
O descriptor do handler será obtido no bootstrap a partir de
`EffectClass()`/`Mutability()`, nunca de cliente. Permanecem pendentes schemas
de argumentos, integração de aliases/locales na UI, risco/redação, disponibilidade, versão do
catálogo, ponte e executor. Este incremento não conclui a Fase 1.

O catálogo agora possui apresentação opcional versionada neste estágio de
protótipo. Quando fornecida, exige nome, descrição e categoria nos três locales
(`pt-BR`, `en`, `es`), com aliases validados pela mesma normalização da busca.
`Registry.Search` localiza por ID e metadata, ordena por ID e devolve cópias
profundas; `Lookup` continua exigindo identidade canônica exata. A normalização
inicial uniformiza caixa e espaços, sem busca aproximada ou remoção de acentos.
Ainda falta exigir essa apresentação no bootstrap de comandos do produto e
ligar a Command Palette ao catálogo, sem listas paralelas no frontend.

Validação do incremento conjunto: testes dos dois pacotes passaram (92,5% de
cobertura em bindings e 96,9% em catálogo), assim como `go build ./...`,
`go vet ./...` e verificador de status dos AEPs. Testes adicionais cobrem
permutações da composição, conflitos entre overrides, proveniência,
restauração sem mutar snapshots e leituras concorrentes com retornos isolados.
O `golangci-lint` local não executou: binário v1 incompatível com configuração
v2 do repositório. A suíte geral e o detector de corrida continuam com as
limitações registradas na Fase 0; não se declara validação integral ou CI verde.

- Implementar registro tipado de comandos.
  - Incremento experimental adicional: `internal/commandbindings/defaults.go`
    materializa overrides do mesmo acionador antes da precedência, aplica
    tombstones antes da deduplicação e permite restaurar reconstruindo o
    snapshot sem o delta. Desabilitar a personalização preserva fallback.
    Testes cobrem condições restritas, versões, referências órfãs, imutabilidade
    e barreira de diálogo. Os dois pacotes passaram dez repetições dos testes;
    `go vet` focado também passou.
  - Não há persistência/restore transacional, remapeamento de acionador,
    fingerprint RFC 8785, ledger de supressão nem reserva de invariantes no
    dispatcher. Em mudança semântica, sem snapshot histórico completo, a
    pendência ainda bloqueia conservadoramente o acionador fora de diálogos;
    isso precisa ser refinado para o contexto exato antes de integrar ao produto.
- Definir schema versionado de contexto, acionadores, bindings e camadas.
- Criar camadas padrão no código e persistência de deltas no SQLite.
- Implementar resolução determinística, índice em memória e diagnóstico de
  conflitos.
- Cobrir precedência, fallback de defaults, restore e concorrência com testes.

### Fase 2 — Teclado local e hotkeys globais

- Migrar incrementalmente os atalhos do workspace e chat para comandos.
- Preservar comportamento de foco, input, menus, modais e abas.
- Integrar `internal/hotkey` como adapter global.
- Migrar hotkeys de perfis e o trigger `hotkey` de jobs para bindings que
  executam comandos canônicos.
- Remover handlers paralelos somente após equivalência automatizada.

### Fase 3 — Command Palette e configuração

- Implementar Command Palette acessível.
- Criar lista e detalhe de camadas.
- Implementar captura de teclado, regras de ativação e análise de conflitos.
- Implementar restauração por binding, camada e conjunto completo.
- Atualizar ajuda de atalhos para consultar o mapa efetivo.

### Fase 4 — Contexto e programas em foco

- Integrar contexto de surface, aba, workspace, foco e diálogos.
- Implementar adapter Windows para janela/processo em primeiro plano.
- Implementar fixação manual, estabilização e fallback seguro.
- Definir adapters equivalentes ou degradação explícita em Linux e macOS.

### Fase 5 — Stream Deck direto

- Implementar gerenciador Go de dispositivos.
- Mapear teclas físicas para acionadores normalizados.
- Renderizar bindings efetivos com cache e atualização diferencial.
- Implementar imagens, estados, navegação por camadas e múltiplos dispositivos.
- Adicionar reconexão, diagnóstico de disputa e desligamento limpo.
- Validar operação sem o software oficial instalado.

### Fase 6 — Chat, CLI e automações

- Expor catálogo, execução e gerenciamento estruturado ao chat.
- Expor listagem e execução na CLI somente para comandos que declarem essa
  origem e não dependam de surface visual.
- Integrar eventos e jobs sem criar executor paralelo.
- Integrar exportação/importação e auditoria.

### Fase 7 — Expansão de adapters

- Avaliar pedais USB, controles MIDI e outros dispositivos.
- Avaliar dial e gestos avançados conforme capacidades detectadas.
- Avaliar controle privilegiado de programas externos em AEP ou decisão de
  segurança específica.

### Evidência parcial: custo de resolução e orçamento de latência

O resolvedor puro já usa snapshot imutável indexado por acionador: leitura de
SQLite, Credential Manager, receipts e reconstrução de configuração não fazem
parte de `Resolver.Resolve`. O caminho de um único candidato evita as listas
intermediárias de dominância/prioridade, preservando validação de fatos,
elegibilidade, bloqueio por diálogo e proveniência. Testes diferenciais contra
o caminho geral cobrem também candidatos desabilitados, camadas inativas,
foreground, contexto inválido e isolamento do resultado retornado.

Benchmarks reproduzíveis em `internal/commandbindings/resolver_benchmark_test.go`
cobrem volume de bindings, colisões e leitura concorrente do mesmo snapshot.
`internal/commandsecurity/gate_benchmark_test.go` mede admissão isolada,
admissões concorrentes e mistura com mutações exclusivas de callback vazio.
Os valores `ns/op` são médias de microbenchmark, não percentis de experiência
do usuário; callbacks vazios não representam contenção de SQLite ou de login.

Amostra local em Windows/amd64, Intel Core Ultra 7 155H, Go 1.26.2,
`-benchtime=200ms -count=1`: seleção de um candidato entre 1/100/1000 bindings
distintos ficou em aproximadamente 65–68 ns/op (16 B, uma alocação); colisões
de 10/100 candidatos no mesmo acionador ficaram em aproximadamente 3,2/33 µs.
São fixtures sintéticas e uma única rodada, não garantia ou limite de produto.
Para reproduzir: `go test ./internal/commandbindings -run '^$' -bench
BenchmarkResolve -benchmem -benchtime=200ms` e o equivalente em
`./internal/commandsecurity` com `-bench BenchmarkDispatchGate`.

Orçamento experimental para integração futura: p95 abaixo de 1 ms de
processamento interno para atalhos locais simples, sem incluir o trabalho da
ação. Essa meta ainda **não foi demonstrada ponta a ponta** e não deve virar
asserção temporal frágil em teste unitário. A validação de produto deve medir
separadamente resolução, fila/gate, autenticação/ledger, handoff e renderização,
com troca de abas, alteração de bindings, sessão bloqueada e carga concorrente.

Não foi introduzida exceção ao fluxo auditado de D4: distinguir ações
puramente visuais de comandos com efeitos exige classificação explícita e
revisão do contrato antes de qualquer dispensa de ledger/autorização. `Ctrl+N`
não é presumido visual/read-only: pode criar dados persistentes. Receipts de
configuração continuam exclusivos da mutação confirmada, não de cada uso do
binding. Uma otimização futura nunca pode reutilizar resultado de autorização
após invalidação nem remover a revalidação atômica para ganhar desempenho.

Risco ainda aberto: `DispatchGate` mantém exclusão durante a transação de
configuração; leitores podem esperar I/O de SQLite, e a aquisição do mutex
atual não é cancelável. Os microbenchmarks não resolvem nem limitam essa espera.
Antes de ativar atalhos no produto, medir caudas de latência com essa contenção
real e definir tratamento de indisponibilidade sem execução com mapa obsoleto.

## Riscos

- **Conflitos difíceis de compreender:** muitas camadas podem tornar o resultado
  surpreendente. Mitigação: resolução determinística, explicação do binding
  efetivo, simulação de contexto e confirmação de sobreposição.
- **Regressão de atalhos locais:** migração pode perder regras de foco ou modal.
  Mitigação: inventário, migração incremental e regressões por surface.
- **Camada personalizada ocultar defaults:** um mapa completo substituiria
  comandos como `Ctrl+N`. Mitigação: composição aditiva e overrides por binding.
- **Listeners duplicados:** frontend, hotkey global e dispositivo podem disparar
  duas vezes. Mitigação: identidade normalizada, ownership por adapter e
  deduplicação de evento físico e de candidatos equivalentes.
- **Troca excessiva de contexto:** foco rápido pode causar oscilação do Stream
  Deck. Mitigação: eventos, estabilização curta, fixação manual e cache.
- **Custo de renderização:** imagens podem consumir CPU e USB. Mitigação: cache,
  pré-processamento e diff por tecla.
- **Biblioteca ou modelo incompatível:** dependência Go pode não cobrir todos os
  aparelhos. Mitigação: capability detection e protótipo na Fase 0.
- **Disputa pelo dispositivo:** outro processo pode possuir o HID. Mitigação:
  erro recuperável e reconexão, sem falhar o aplicativo.
- **Ações perigosas fora de foco:** uma tecla física pode executar mutações sem
  contexto visível. Mitigação: política por comando, confirmação acessível,
  anúncio e grants explícitos.
- **Monitoramento invasivo de janela:** títulos podem conter informação
  sensível. Mitigação: usar identidade do processo por padrão e minimizar
  persistência/log.
- **Interface ainda complexa:** flexibilidade pode sobrecarregar a configuração.
  Mitigação: lista de camadas, detalhe com duas seções e divulgação progressiva.

## Critérios de aceitação

- [ ] Existe registro canônico e pesquisável de comandos com IDs, argumentos,
  disponibilidade, risco, aliases localizados e apresentação.
- [ ] Teclado local, hotkey global, Stream Deck, Command Palette, chat e CLI
  podem convergir para o mesmo comando sem handlers finais duplicados.
- [ ] Todo acionamento que resolve para execução produz `CommandInvocation` e
  passa por `CommandExecutionService`, com sessão, proveniência, autorização,
  deduplicação e auditoria antes do handler final; `effect = suppress` é
  consumido sem criar invocação.
- [ ] A reserva atômica por evento impede reentrega, e ownership exclusivo
  impede duplicidade entre teclado local/global e listeners de dispositivo.
- [ ] Solicitações diretas e triggers sem `source_event_id` usam
  `invocation:<invocation_id>`; eventos usam `event:<source_event_id>`.
- [ ] Manter uma tecla pressionada não repete comando: o adapter descarta
  `KeyboardEvent.repeat`/repetição nativa antes de gerar `source_event_id` e
  testes cobrem release, blur e reconexão.
- [ ] Retirada de `queued` revalida todos os gates no mesmo CAS para `running`.
- [ ] Cada instância física usa geração própria e índice parcial de eventos;
  invocações diretas deduplicam somente pela PK UUIDv7.
- [ ] Execução por agente e automação preserva e revalida os gates da AEP-0101;
  origem headless não herda a identidade do usuário para autorizar mutações.
- [ ] Usuário e ator são derivados pelo backend; payload não escolhe identidade
  de autorização/auditoria.
- [ ] Camadas padrão do aplicativo e das surfaces permanecem ativas e um binding
  ausente em camada superior cai para o default.
- [ ] Overrides afetam somente o acionador e contexto declarados.
- [ ] Tombstone bloqueia o default no contexto declarado, enquanto
  personalização apenas desabilitada permite fallback.
- [ ] Tombstones são aplicados antes da deduplicação e nunca produzem invocação.
- [ ] Tombstone que consome um acionador grava marcador terminal no ledger;
  reentrega do mesmo evento não passa a executar um default após mudança de
  configuração.
- [ ] Acionamento stale não grava `suppressed`, mas recebe marcador terminal
  `rejected_stale`; o mesmo ID nunca executa em reentrega posterior.
- [ ] Override de default persiste ID e versão do default substituído.
- [ ] É possível restaurar um binding, uma camada ou todas as personalizações.
- [ ] Conflitos são detectados considerando a possível interseção de contextos,
  e empate não executa dois comandos.
- [ ] Escopo, especificidade e prioridades persistidas produzem resolução
  determinística após importação/restart; empate termina em conflito fail-closed.
- [ ] Bindings equivalentes por comando, argumentos e escopo produzem uma única
  invocação com proveniência preservada.
- [ ] O resolvedor não consulta SQLite nem percorre o catálogo completo a cada
  acionamento.
- [ ] Mudanças de surface, foco, workspace, janela externa e eventos podem
  ativar e desativar camadas de forma determinística.
- [ ] Desabilitar camada a remove imediatamente do mapa sem ressuscitar claims
  stale ao reabilitá-la; expiração local é idempotente após restart.
- [ ] Ativações por evento têm ID, sequência, correlação e deduplicação; evento
  atrasado não encerra ciclo mais novo.
- [ ] Claim e ledger de ativação preservam o escopo global/workspace, inclusive
  para refs `builtin`; eventos e replay de outro workspace falham fechado.
- [ ] A primeira versão aceita apenas fatos de `job_run_events` espelhados
  transacionalmente na outbox durável; EventBus best-effort e produtores
  externos falham fechado.
- [ ] Count-cap/cascade de runs não remove a outbox antes do deadline; startup
  recupera leases e reprocessa pendências antes da retenção de jobs.
- [ ] Estado de ativação persistido é reconciliado em modo seguro no startup e
  preserva autenticação, geração e proveniência anti-loop da AEP-0067.
- [ ] Claim de job sem lease e fonte autoritativa válidas fica inativa.
- [ ] Replay de ativação fora da retenção é rejeitado, e ownership vem do
  principal autenticado, não do payload.
- [ ] A Command Palette busca e descreve comandos disponíveis e indisponíveis
  com motivo, mas executa somente os disponíveis.
- [ ] A Command Palette tem navegação completa por teclado, anúncios e
  restauração de foco cobertos por testes e validação NVDA.
- [ ] A configuração por chat usa tools estruturadas, IDs reais e confirmações
  de segurança.
- [ ] Toda mutação persistente solicitada por agente mostra diff, exige decisão
  explícita e falha fechado sem interlocutor.
- [ ] `command_catalog.execute` aplica o mesmo gate a comandos que alteram
  capacidade efetiva, incluindo ativação de camada.
- [ ] A tela de configuração oferece lista de camadas, detalhe de ativação e
  bindings, captura de teclas e explicação do resultado efetivo.
- [ ] Toda configuração é operável por teclado e NVDA sem depender de grade,
  arrastar, imagem ou cor.
- [ ] O Assistente controla ao menos um modelo de Stream Deck diretamente por
  Go, sem software oficial, com reconexão e shutdown limpo.
- [ ] Sem sessão autenticada, e durante logout ou troca de usuário, o Stream
  Deck fica em estado seguro e rejeita callbacks de gerações anteriores.
- [ ] O Stream Deck atualiza somente teclas cujo conteúdo efetivo mudou e usa
  cache de imagens.
- [ ] Abertura/reconexão do Stream Deck invalida o diff e força frame completo.
- [ ] Camadas baseadas no programa em primeiro plano funcionam no Windows e
  degradam explicitamente em plataformas sem adapter.
- [ ] Contexto externo é capturado antes de bring-to-front e não muda no meio do
  acionamento.
- [ ] Comandos disparados fora de foco preservam permissões, decisões e
  auditoria do executor de destino.
- [ ] Exportação/importação preserva UUIDs e escopos, relata referências e
  conflitos e não transfere grants nem histórico de invocações.
- [ ] Binding persistente e export não contêm segredos brutos; delegação a tool
  propaga redação ou permanece indisponível.
- [ ] Referência importada de credencial resolve pattern exato no usuário de
  destino ou deixa o binding desabilitado.
- [ ] `command_invocations` tem payload redigido, origem rastreável, índices e
  retenção por idade e quantidade, sem prometer reconstruir o snapshot completo.
- [ ] `command_invocations.invocation_id` é a PK canônica da invocação,
  consulta e correlação com tools; o ledger tem PK própria `id` e referências
  UNIQUE explícitas.
- [ ] Reentrega dentro da janela retorna status/resultado redigido sem repetir o
  handler; invocações interrompidas por queda viram `outcome_unknown`.
- [ ] Evento durável preserva a chave pelo horizonte de replay da fonte e,
  depois dele, é rejeitado por `source_occurred_at` autenticado em vez de ser
  tratado como solicitação nova.
- [ ] Ativações por evento persistem o mesmo epoch/deadline imutável da fonte;
  aumentar retenção não reabre ocorrência antiga.
- [ ] Recuperação de startup atualiza auditoria e ledger para
  `outcome_unknown` na mesma transação.
- [ ] Reutilizar `invocation_id` com request fingerprint diferente falha
  fechado.
- [ ] Caps de auditoria não removem os ledgers antes de `expires_at`; compactar
  registro recente não permite nova execução ou ativação.
- [ ] Consulta de invocação aplica propriedade por usuário e autorização do
  ator, sem lookup cross-user apenas pela PK.
- [ ] Sessão, geração de segurança e staleness de contexto são revalidados
  imediatamente antes de todo handler.
- [ ] Policies `max_age_ms`/`event_snapshot` falham fechado sem timestamp de
  cada provider; ingresso não transforma snapshot sem `capturedAt` em contexto
  recém-capturado.
- [ ] `handler.Start` confirma handoff sem bloquear; logout/mutação concorrente
  não espera o trabalho longo nem entra em deadlock.
- [ ] Versões do catálogo e da configuração são revalidadas ao retirar da fila;
  binding alterado não executa resolução antiga.
- [ ] Cache de resolução inclui usuário, workspace, acionador, origem,
  `context_version` e todas as versões/gerações de catálogo, configuração e
  camadas ativas.
- [ ] Cada comando declara `context_policy`; nas policies que declaram
  providers, provider ausente ou versão/TTL inválido falha fechado.
- [ ] `context_policy = none` é rejeitado para qualquer comando não read-only.
- [ ] Contextos local, JWT externo, job e system têm fontes de identidade e
  revogação explícitas; `EpochService` invalida trabalho obsoleto.
- [ ] Ativação event-driven usa grants próprios de camada, com chave natural,
  geração monotônica, histórico de revogação e revalidação autoritativa por
  evento; não reutiliza nem amplia grants de delegação da AEP-0101.
- [ ] Adapter de jobs exige `job_slug = Job.ID` e
  `job_database_id = Job.DatabaseID`, confirma ambos por owner e permanece
  desabilitado para fatos legados ambíguos.
- [ ] Evento de ativação recebido é candidato sem autoridade; dispatcher
  deriva owner, workspace, regra, layer e epochs antes do envelope interno.
- [ ] Regras e layers builtin/user usam refs polimórficas consistentes no
  schema, grants, estado, ownership, importação e restore.
- [ ] Após o PR atualizar a AEP-0052, identidade externa só acessa usuário
  local por mapeamento administrativo exato de emissor e subject; antes disso,
  o command manager fica indisponível nesse modo.
- [ ] Cada comando declara origens permitidas e o serviço bloqueia origem não
  autorizada, incluindo comandos visuais solicitados pela CLI.
- [ ] `effect_class` e mutabilidade vêm do contrato do handler; metadata
  divergente impede o registro.
- [ ] CLI não executa comando que exija diálogo/decisão interativa.
- [ ] Comando destrutivo só avança com receipt de decisão criada no backend,
  vinculada à solicitação e consumida uma vez no CAS para `queued`.
- [ ] `cli`, `event` e `system` não registram/executam comando destrutivo;
  qualquer origem sem presenter interativo falha fechado.
- [ ] Em autenticação externa, adapters físicos permanecem indisponíveis até
  existir vínculo local explícito e revogável com um principal externo.
- [ ] Estação bloqueada suspende hotkeys globais e dispositivos físicos e
  apresenta estado seguro até revalidar a sessão após desbloqueio.
- [ ] Diálogo topmost bloqueia fallback para camadas inferiores e os atalhos
  obrigatórios da AEP-0091 não aceitam tombstone.
- [ ] Dispatcher reserva atalhos invariantes do diálogo antes de qualquer
  binding configurável.
- [ ] Shell continua passando exclusivamente por `internal/commandpolicy`.
- [ ] Manutenção em escopo de instância cobre todos os usuários e registros
  `system` em uma única cadência.
- [ ] Deep links e configurações importadas não concedem execução arbitrária.
- [ ] Testes cobrem fallback de defaults, sobreposição, múltiplas camadas,
  modais, inputs, múltiplas abas, troca de foco, reconexão de dispositivo e
  prevenção de execução duplicada.

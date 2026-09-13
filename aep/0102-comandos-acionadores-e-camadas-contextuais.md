# AEP-0102: Comandos, acionadores e camadas contextuais

**Status:** Draft

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
   segurança. Nos demais casos, reserva atomicamente ledger e auditoria como
   `evaluating`;
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
define `JWT sub = user_id`. Esta AEP, enquanto `Draft`, não a substitui nem
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

command_event_replay_policy_epochs
  id, producer_type, generation, effective_at, replay_horizon_seconds,
  created_at

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

### Fase 0 — Inventário e protótipos

- Inventariar atalhos locais, hotkeys de perfis, comandos de menu e ações
  executáveis existentes.
- Prototipar o registro de comandos e a resolução de camadas sem migrar handlers.
- Validar a biblioteca Go do Stream Deck, exclusividade, reconexão e modelos.
- Prototipar observação de janela em primeiro plano no Windows.
- Medir latência e estabilidade com muitas camadas e bindings.

### Fase 1 — Registro, defaults e resolvedor

- Implementar registro tipado de comandos.
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


# AEP-0102: Comandos, acionadores e camadas contextuais

**Status:** Draft

**Data:** 2026-09-12

**Relacionados:** AEP-0001, AEP-0023, AEP-0045, AEP-0047, AEP-0048,
AEP-0058, AEP-0060, AEP-0063, AEP-0074-B, AEP-0080, AEP-0091, AEP-0101

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
- nome, descrição e categoria internacionalizáveis;
- schema tipado de argumentos;
- escopos e contextos em que pode executar;
- allowlist de origens permitidas, como `desktop`, `cli`, `agent` e `event`;
- classificação de risco;
- estado de disponibilidade e motivo quando indisponível;
- apresentação padrão opcional, incluindo ícone e estados;
- handler ou rota de execução.

IDs apresentados pelo catálogo são canônicos. Chat, importação e UI não podem
inventar IDs nem executar handlers por nome aproximado.

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
  trigger_type?, trigger_spec?
  user_id, session_id, session_generation, security_generation
  actor_type, actor_id
  source_type, source_instance_id, source_event_id, binding_ids
  conversation_id?, turn_id?, surface_type?, surface_id?
  surface_snapshot_version?, context_version, context_captured_at
  source_profile_slug?, target_profile_slug?
  authorization_decision_id?, delegation_fingerprint?, grant_generation?
  job_uuid?, job_run_id?
  correlation_id, requested_at
```

`source_type` distingue teclado local/global, Stream Deck, palette, chat, CLI,
evento e sistema. `source_instance_id` identifica de forma estável o adapter,
janela ou dispositivo; `source_event_id` identifica uma ocorrência única
produzida por esse adapter, usando contador monotônico ou ID do protocolo.
`actor_type` distingue usuário, agente e automação.

Uma solicitação informa `command_id` para execução direta ou
`trigger_type`/`trigger_spec` para resolução de binding. Depois da resolução, o
envelope interno contém ambos; combinações ausentes ou incoerentes falham antes
de qualquer efeito.

O serviço, nessa ordem:

1. valida sessão, usuário e proveniência;
2. resolve o binding quando a origem for um acionador;
3. valida origem permitida, disponibilidade, argumentos, contexto e política;
4. reserva atomicamente a identidade da invocação/evento como `queued`;
5. revalida sessão, geração, contexto e autorização imediatamente antes do
   despacho, cancelando a invocação se qualquer um estiver obsoleto;
6. encaminha ao handler registrado;
7. conclui a trilha de `command_invocations`.

A reserva é um insert `queued` antes do handler, protegido pela PK de
`invocation_id` e por unicidade de
`(user_id, session_id, source_type, source_instance_id, source_event_id)` quando
há evento de adapter. Reentrega recebe o resultado existente ou falha como
duplicada, nunca chama novamente o handler. Palette, chat e CLI fornecem um
`invocation_id` UUIDv7 idempotente por solicitação. Eventos físicos recebem
`source_event_id` no único adapter que os possui. A garantia de deduplicação é
limitada à sessão física ou à janela de retenção da invocação. Solicitação
direta cujo UUIDv7/`requested_at` esteja fora dessa janela é rejeitada como
obsoleta, mesmo se sua linha já tiver sido removida.

Reentrega consulta a invocação pelo ID e recebe status, `result_summary` e
`result_ref` redigidos por `CommandExecutionService.GetInvocation`; não repete o
handler. No startup,
registros `queued` ou `running` de uma geração encerrada viram
`outcome_unknown`, nunca são reexecutados automaticamente. O usuário ou fluxo
chamador precisa consultar o efeito e criar uma nova invocação explícita. Esse
tratamento reconhece que exatamente-uma-vez não é garantível para todo handler
de UI ou sistema após queda entre efeito e commit.

Comandos que delegam para tools passam então pelo executor da AEP-0063 e
correlacionam `command_invocations.id` com `tool_invocations`. Jobs passam pelo
runtime de jobs; ações de frontend recebem da ponte somente um despacho já
autorizado, vinculado ao `invocation_id`. Command Palette, chat e CLI podem
selecionar diretamente um `command_id`, mas não ignoram validação, autorização
ou auditoria.

Argumentos sensíveis são redigidos ou resumidos na auditoria conforme a política
do comando. Erro, status, origem, ator, comando e correlação permanecem
diagnosticáveis.

Quando `actor_type = agent`, o envelope preserva conversa, turno, `surfaceType`,
`surfaceId` e `snapshotVersion` do `SurfaceContext` da AEP-0080, além dos
profiles de origem e destino. `CommandExecutionService` não trata o agente como
o usuário autenticado: delegação cross-profile interativa exige
`DecisionDialog` e registra `authorization_decision_id`; origem sem interlocutor
falha fechado.

Jobs cross-profile transportam `job_uuid`, `target_profile_slug`,
`delegation_fingerprint` e `grant_generation`. O serviço relê o grant pela chave
e geração exatas da AEP-0101 imediatamente antes do handler; slug público serve
para apresentação, não substitui o UUID na consulta. O envelope transporta a
decisão, mas não cria nem amplia grants.

### D3 — Acionadores são adapters, não comandos

Tipos iniciais de acionador:

- `keyboard.local`: combinação recebida dentro da janela do Assistente;
- `keyboard.global`: hotkey registrada no sistema operacional;
- `streamdeck.key`: tecla física, incluindo dispositivo e posição;
- `palette`: escolha na Command Palette;
- `chat`: execução estruturada solicitada pelo agente;
- `cli`: execução solicitada pelo entrypoint de terminal;
- `event`: evento interno permitido e tipado.

Adapters normalizam a entrada para uma identidade de acionador e nunca executam
diretamente a ação final. Uma entrada física gera no máximo uma execução, mesmo
quando mais de um observador puder enxergá-la.

Eventos de teclado têm ownership exclusivo. Uma combinação registrada como
`keyboard.global` pertence ao adapter do sistema operacional inclusive quando o
Assistente está em foco; o adapter DOM recebe a lista correspondente e não emite
`keyboard.local` para ela. O adapter local possui somente combinações não
registradas globalmente. Alterações de registro são aplicadas por geração antes
de publicar o novo mapa. Stream Deck possui um único listener por dispositivo.
Essa exclusão evita depender de um ID que DOM e API global não compartilham.

Pressão normal, pressão longa, alternância e dial podem ser acrescentados como
gestos normalizados quando o dispositivo oferecer esses sinais. Capacidade não
detectada não deve ser simulada de forma ambígua.

### D4 — Bindings associam acionadores a comandos

Um binding contém:

- camada;
- tipo e especificação normalizada do acionador;
- `command_id`;
- argumentos validados pelo schema do comando;
- condição tipada opcional;
- estado habilitado/desabilitado;
- origem: padrão do aplicativo ou personalização do usuário;
- metadados de apresentação específicos do acionador.

O mesmo comando pode ter vários bindings. O mesmo acionador pode aparecer em
várias camadas. Reutilização não é conflito enquanto as condições ou camadas
não puderem estar ativas simultaneamente.

Quando candidatos simultâneos possuem o mesmo `command_id`, argumentos
normalizados e escopo de execução, o resolvedor os deduplica e produz uma única
invocação, preservando a proveniência de todos os bindings equivalentes.
Diferença de comando, argumentos ou escopo continua sendo conflito e falha
fechado se a precedência não escolher um único vencedor.

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

Atalhos essenciais de acessibilidade e decisão podem exigir aviso reforçado ou
não aceitar remoção sem alternativa equivalente, conforme AEP-0091 e regras de
acessibilidade do projeto.

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

Exemplos:

```text
surface = chat                         → ativa Chat
foreground.process = code.exe          → ativa Desenvolvimento
app.focused = false                    → ativa Global
job run em andamento                   → ativa Execução
streamdeck.key.5 → layer.toggle        → alterna Trabalho
```

Mudanças de tela e estado do Assistente devem chegar por eventos internos. O
monitor de janela em primeiro plano é um adapter específico por sistema
operacional. No Windows, ele observa a janela e o processo em foco sem depender
do software do Stream Deck.

Para jobs, a integração publica o fato contextual interno versionado
`command-context.job-run-state.v1`, com `user_id`, `job_uuid`, `job_slug`,
`run_id`, `sequence`, `state` e `occurred_at`. `job_uuid` é a FK canônica de
`job_runs` da AEP-0048; `job_slug` é a identidade pública usada por
`eventctx.SourceJobID` na AEP-0101 e serve para apresentação/resolução inicial.
Depois da resolução, correlação e autorização usam o UUID.

`state` aceita `queued`, `started`,
`retry_scheduled`, `completed`, `failed` e `skipped`. A chave de
correlação é `(user_id, run_id)`; `sequence` impede regressão por entrega fora
de ordem. Estados `queued`, `started` e `retry_scheduled` mantêm a regra ativa;
`completed`, `failed` e `skipped` a encerram. Esse fato deriva do
runtime e da timeline `job_run_events` da AEP-0048; não inventa nomes no event
bus público da AEP-0001.

Cancelamento não pertence ao enum vigente da AEP-0048 e, portanto, não é
inventado aqui. Se o runtime ganhar esse estado, a AEP-0048 deve ser atualizada
antes de ele entrar no fato contextual v1 ou em uma versão posterior.

Trocas rápidas passam por estabilização curta, e o usuário pode fixar uma
camada para suspender trocas automáticas. Se o contexto deixar de ser confiável,
o resolvedor retorna ao conjunto padrão seguro.

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
  `layer_update`, `layer_delete`, `binding_list`, `binding_check_conflict`,
  `binding_create`, `binding_update` e `binding_delete`.

Alterações destrutivas, conflitos e comandos sensíveis continuam sujeitos ao
contrato de decisão da AEP-0091. A resposta da tool inclui IDs reais e o efeito
resolvido; o modelo não edita tabelas diretamente.

### D11 — Persistência

Defaults ficam no código. SQLite guarda entidades do usuário e deltas:

```text
command_layers
  id, user_id, workspace_id, name, description, enabled, source,
  created_at, updated_at

command_layer_activation_rules
  id, layer_id, mode, condition, priority, lifecycle, enabled

command_bindings
  id, layer_id, trigger_type, trigger_spec, command_id, arguments,
  condition, effect, enabled, source, replaces_default_id,
  replaces_default_version, presentation

command_invocations
  id, schema_version, user_id, session_id, session_generation,
  security_generation, command_id, binding_ids,
  actor_type, actor_id, source_type, source_instance_id, source_event_id,
  arguments_summary, arguments_fingerprint, conversation_id, turn_id,
  surface_type, surface_id, surface_snapshot_version, context_version,
  context_captured_at, context_summary, source_profile_slug, target_profile_slug,
  authorization_decision_id, delegation_fingerprint, grant_generation,
  job_uuid, job_run_id, correlation_id, risk, policy_decision,
  result_summary, result_ref, status, error_code, requested_at, completed_at
```

Condições, argumentos, especificações e apresentação são documentos JSON
versionados e validados. Alterações relevantes mantêm auditoria suficiente para
desfazer.

`binding_ids` é uma lista JSON ordenada que registra todos os bindings
equivalentes considerados na deduplicação; fica vazia para execução direta.
`status` aceita `queued`, `running`, `succeeded`, `failed`, `denied`,
`cancelled_stale` e `outcome_unknown`. `result_summary` é redigido e
`result_ref` guarda somente referência estável e não sensível, como o ID de uma
aba ou run, permitindo consultar uma reentrega sem repetir efeitos.

`workspace_id` nulo identifica camada global do usuário; preenchido identifica
camada daquele workspace. A consulta efetiva carrega somente camadas globais do
usuário autenticado mais as do workspace atual. SQLite usa dois índices únicos
parciais: `(user_id, name) WHERE workspace_id IS NULL` para globais e
`(user_id, workspace_id, name) WHERE workspace_id IS NOT NULL` para workspaces.
Bindings herdam o escopo da camada, evitando misturar configurações.

Abrir ou carregar um workspace não autoriza conteúdo controlado pelo workspace
— arquivos do projeto, metadados importados ou eventos emitidos por ele — a
criar ou habilitar bindings de shell, MCP, hotkeys globais ou ações externas.
Essas operações exigem ator autenticado e o fluxo explícito de configuração.

Exportação e importação integram o envelope versionado da AEP-0047 pela seção
`resources.commandLayers`. Cada camada inclui UUID, escopo portátil,
`activationRules` e `bindings`; overrides incluem ID e versão do default.
Defaults puros e `command_invocations` não são exportados. Referências internas
são remapeadas em conjunto e a importação é idempotente por UUID.

Conflito de UUID com conteúdo diferente exige escolha explícita entre manter,
substituir ou importar como cópia com novos UUIDs. Referência a workspace,
comando, dispositivo ou default ausente fica desabilitada e entra no relatório
de importação; não é aproximada por nome. Grants, autorizações e ativações
temporárias nunca são exportados ou concedidos. A configuração importada só
entra no mapa efetivo após validação e confirmação dos conflitos.

`command_invocations` é auditoria técnica efêmera.
`internal/commandinvocations.MaintenanceService`, definido por esta AEP e
executado pela manutenção periódica do aplicativo, remove registros com mais de
`command_invocation_retention_days` (padrão 30) e mantém no máximo
`command_invocations_per_user_keep` (padrão 10.000), removendo os mais antigos
acima do limite. A implementação futura pode integrar esse serviço ao
orquestrador da AEP-0074-B, mas não altera implicitamente seu contrato vigente.
Índices mínimos:
`(user_id, requested_at)`, `(user_id, status, requested_at)` e a unicidade de
evento definida em D2.1. Não se persiste `arguments` bruto nem o snapshot:
`arguments_summary` redigido, fingerprint do JSON canônico já redigido,
identidades de conversa/turno/surface, `surface_snapshot_version`,
`context_version` e um resumo redigido permitem rastrear a origem sem copiar ou
hashear diretamente segredos. O conteúdo completo do `SurfaceContext` não é
reconstituível depois de expirar e esta AEP não promete reprodução integral.
Auditorias de decisão/grant que tenham retenção própria na AEP-0091 ou AEP-0101
não são substituídas por esta tabela.

Quando um comando delega para tool, `tool_invocations` usa o contrato já vigente
da AEP-0063: `origin_type = system`, `origin_id = command_invocations.id` e
metadata `command_origin_schema = command-invocation.v1`. A origem humana ou
automatizada completa permanece em `command_invocations`; a correlação não
exige criar um novo enum em AEP-0063. A retenção de cada tabela continua
independente e a UI tolera o lado técnico já expirado.

### D12 — Resolução eficiente

O resolvedor não percorre todas as camadas nem consulta o banco a cada tecla.

- configurações são carregadas e validadas na memória;
- bindings são indexados pela identidade normalizada do acionador;
- o conjunto de camadas ativas é mantido separadamente;
- para uma entrada, somente candidatos daquele acionador são avaliados;
- resultados frequentes podem ser cacheados por versão de contexto;
- mudanças de contexto invalidam apenas entradas afetadas.

Para o Stream Deck, a composição é recalculada quando camadas, contexto ou
estado visível mudam. O renderer compara o estado anterior e atual e envia ao
dispositivo somente teclas alteradas. Imagens redimensionadas ficam em cache.

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

Uma tecla que ativa outra camada oferece navegação semelhante a pasta, mas
continua usando o mecanismo genérico `layer.activate`, `layer.toggle` ou
`layer.back`. Teclado, chat ou outro dispositivo podem ativar a mesma camada.

### D14 — Contexto de programas externos

Quando o Assistente estiver sem foco, hotkeys globais e dispositivos físicos
podem usar camadas ativadas pelo programa em primeiro plano.

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
`cli` e não dependam de runtime visual. Comandos de workspace, editor ou foco
aparecem indisponíveis com motivo; esta AEP não leva essas surfaces ao terminal.

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
- Eventos externos precisam de origem autenticada e allowlist antes de poderem
  ativar comandos.

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
  disponibilidade, risco e apresentação.
- [ ] Teclado local, hotkey global, Stream Deck, Command Palette, chat e CLI
  podem convergir para o mesmo comando sem handlers finais duplicados.
- [ ] Todos os adapters produzem `CommandInvocation` e passam por
  `CommandExecutionService`, com sessão, proveniência, autorização, deduplicação
  e auditoria antes do handler final.
- [ ] A reserva atômica por evento impede reentrega, e ownership exclusivo
  impede duplicidade entre teclado local/global e listeners de dispositivo.
- [ ] Execução por agente e automação preserva e revalida os gates da AEP-0101;
  origem headless não herda a identidade do usuário para autorizar mutações.
- [ ] Camadas padrão do aplicativo e das surfaces permanecem ativas e um binding
  ausente em camada superior cai para o default.
- [ ] Overrides afetam somente o acionador e contexto declarados.
- [ ] Tombstone bloqueia o default no contexto declarado, enquanto
  personalização apenas desabilitada permite fallback.
- [ ] Override de default persiste ID e versão do default substituído.
- [ ] É possível restaurar um binding, uma camada ou todas as personalizações.
- [ ] Conflitos são detectados considerando a possível interseção de contextos,
  e empate não executa dois comandos.
- [ ] Bindings equivalentes por comando, argumentos e escopo produzem uma única
  invocação com proveniência preservada.
- [ ] O resolvedor não consulta SQLite nem percorre o catálogo completo a cada
  acionamento.
- [ ] Mudanças de surface, foco, workspace, janela externa e eventos podem
  ativar e desativar camadas de forma determinística.
- [ ] A Command Palette busca e descreve comandos disponíveis e indisponíveis
  com motivo, mas executa somente os disponíveis.
- [ ] A Command Palette tem navegação completa por teclado, anúncios e
  restauração de foco cobertos por testes e validação NVDA.
- [ ] A configuração por chat usa tools estruturadas, IDs reais e confirmações
  de segurança.
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
- [ ] Camadas baseadas no programa em primeiro plano funcionam no Windows e
  degradam explicitamente em plataformas sem adapter.
- [ ] Comandos disparados fora de foco preservam permissões, decisões e
  auditoria do executor de destino.
- [ ] Exportação/importação preserva UUIDs e escopos, relata referências e
  conflitos e não transfere grants nem histórico de invocações.
- [ ] `command_invocations` tem payload redigido, origem rastreável, índices e
  retenção por idade e quantidade, sem prometer reconstruir o snapshot completo.
- [ ] Reentrega dentro da janela retorna status/resultado redigido sem repetir o
  handler; invocações interrompidas por queda viram `outcome_unknown`.
- [ ] Sessão, geração de segurança e staleness de contexto são revalidados
  imediatamente antes de todo handler.
- [ ] Cada comando declara origens permitidas e o serviço bloqueia origem não
  autorizada, incluindo comandos visuais solicitados pela CLI.
- [ ] Estação bloqueada suspende hotkeys globais e dispositivos físicos e
  apresenta estado seguro até revalidar a sessão após desbloqueio.
- [ ] Shell continua passando exclusivamente por `internal/commandpolicy`.
- [ ] Deep links e configurações importadas não concedem execução arbitrária.
- [ ] Testes cobrem fallback de defaults, sobreposição, múltiplas camadas,
  modais, inputs, múltiplas abas, troca de foco, reconexão de dispositivo e
  prevenção de execução duplicada.


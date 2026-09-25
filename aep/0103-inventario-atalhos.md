# AEP-0103 — Inventário de migração de atalhos

**Status:** Documento de apoio — migração em andamento; histórico preservado.

## Varredura de Ctrl+N após integrar a main — 24/09/2026

A integração de `origin/main` em `5c9278082` acrescentou Ctrl+N aos editores
de workflow e ações customizadas. A inspeção dos handlers, usos dos hooks e
catálogo também identificou seis apresentações de criação ainda fora do
catálogo. As reconciliações históricas abaixo não significam migração integral
de toda ação existente no frontend.

- `useActivePanelNewShortcut`: AllowlistPage, CredentialsPage, ProvidersPage,
  McpPage, SkillsPage e ChannelsPage abrem criação/menu local por `onNew`.
  Não possuem ID correspondente no catálogo de comandos. A integração futura
  deve preservar painel ativo, contexto da página e recusa de editáveis/modais;
  não transformar essas ações em comandos globais indiscriminados.
- `useNewItemShortcut`: WorkflowEditor abre novo status e CustomActionsEditor
  abre nova ação. São apresentações locais do editor/modal topmost, sem IDs de
  catálogo. Hoje não são reconfiguráveis pela tela de comandos nem executáveis
  pela paleta/Deck. Migrá-las exige representar explicitamente a instância do
  editor e o modal autorizado, preservando a exclusão do modal de item filho.
- Já integrados: Ctrl+N seguido de C/E/R/T para novas abas; apresentações
  `tasklists.create.open` e `profiles.create.open`; Ctrl+N em Histórico para
  `navigation.workspace.open`; N simples da lista aberta solicita
  `tasklist.task.create.open`, sem confundir esse gesto com Ctrl+N global.
- Navegação/edição nativa de grids, inputs e widgets não é, por si só, uma
  ação global a publicar no catálogo. Deve continuar respeitando o contexto
  local e a precedência de eventos consumidos.

Pendência explícita: decidir/implementar a exposição das seis criações de
Configurações e, separadamente, das duas apresentações dos editores. Esta
varredura não declara essa migração concluída. A seção158 da tasklist acompanha
os guards corrigidos e a validação da integração para o PR.

## Leitura vigente — reconciliação de 22/09/2026

A seção140 encerra as três ligações não reconciliadas de C02 (Sobre,
bloco de código e recolhimento de thread), com evidência distribuída de
backend e consumidor UI real. Aceite manual e gates agregados permanecem
separados; detalhes em “Ligações reconciliadas” abaixo e na tasklist.

O catálogo atual é `product-v40-agent-commands`, com **149 comandos / 61 apresentações locais /
67 defaults locais**. A seção118 acrescentou os três comandos de camada e as
seções134–135 acrescentaram as tools compostas e o ingresso CLI;
as seções119–128 ampliaram os contextos, não a quantidade de comandos.
Paleta e Deck têm, cada um, 81 IDs admitidos pelo seu helper contextual;
esses conjuntos não devem ser somados nem confundidos com os 84 critérios.
Contagens conferidas no catálogo e em `app_command_chat_navigation_test.go`,
`app_command_palette_conditions_test.go` e `app_command_deck_conditions_test.go`.
A seção109 conecta `voice.input.activate` e `job.run`, exclusivos de
`keyboard.global`, aos callbacks nativos e ao executor comum. A configuração
continua nas telas de perfil e jobs; os bindings globais são projeções dessas
fontes, não uma segunda configuração editável. O evento direto de voz e a
execução direta do job no callback são substituídos pelo handoff autenticado.
A seção110 prepara o profile dinâmico e revalida o grant e o alvo real por
tentativa. O prazo de admissão não consome o prazo de execução do runtime de
jobs; cancelamento e deadline do caller continuam vinculados. Templates sem
dados suficientes falham fechado. Aceite físico permanece pendente.
As seções anteriores abaixo descrevem o estado de suas respectivas entregas.

As pendências de migração comprovadas na seção103 (Histórico, lista aberta,
voz e hotkey de job) foram implementadas nas seções104 e109–110. Não permanecem
como quatro funcionalidades ausentes. A reconciliação da seção129 da tasklist
separa esse avanço da certificação de equivalência de todas as famílias e dos
aceites físicos. `command_catalog`/`command_config` estão no registry de tools
(`internal/app/app_tool_registry.go`) e a CLI pública está em `cmd/asst`;
isso não significa que algum ID produtivo tenha ganhado `CLI`: hoje a CLI
lista/descreve os 149, mas todos permanecem indisponíveis para execução CLI por
origem não permitida.

## Mapa vigente — catálogo v40, ingressos e efeitos (22/09/2026)

Esta é a fotografia verificável do código atual. Os IDs abaixo são os registros
construídos por `internal/app/app_command_product_catalog.go`; não são
`candidate.*` nem nomes derivados de atalhos. `P/K/D` abreviam,
respectivamente, `palette`, `keyboard.local` e `streamdeck`; `U` é `ui`, `C`
é `chat` e `G` é `keyboard.global`. A classe `local_ui` é apresentação local
sem ledger; `audited_ui` passa pelo broker de UI e pelo executor; `durable_*`
indica efeito de domínio/ledger.

O mapa de handler é fechado no mesmo bootstrap: salvo onde indicado, o mapa
`handlers` usa `a.startCommandUI`. `workspace.list` usa
`a.startWorkspaceList`, `job.run` usa `a.startGlobalCommandJob` e `layer.*`
usa `a.startCommandLayerAction`. As rotas abaixo são as rotas declaradas no
contrato do handler; a cadeia concreta é ingresso → resolução do
binding/contexto → `CommandInvocation`/broker correspondente → rota indicada →
efeito.

### Aplicativo, workspace e navegação — 37 IDs

- `workspace.list`: `Read`, `durable_backend`, `internal/workspace/list`,
  origens `P/K/U/C`; sem default. Lê o manager e devolve DTO sem path bruto.
- `help.shortcuts.show`: `Read`, `local_ui`, `ui/help/shortcuts/show`,
  `P/K/D`; sem default.
- `workspace.panel.focus`: `Read`, `local_ui`, `ui/workspace/panel/focus`,
  `P/K/D`; sem default.
- Criação de abas — `workspace.tab.chat.create`,
  `workspace.tab.editor.create`, `workspace.tab.terminal.create`,
  `workspace.tab.tasklist.create`: `Write`, `durable_backend`,
  `contextual/workspace/tab/{chat|editor|terminal|tasklist}/create`, `P/K/D`.
  Defaults: `Ctrl+N,C/E/R/T` (sequência v2) e `Ctrl+T` para chat.
- `workspace.tab.close`: `Write`, `durable_backend`,
  `contextual/workspace/tab/close`, `P/K/D`; defaults `Ctrl+W` e `Ctrl+F4`.
- `workspace.create`: `Write`, `durable_backend`, `contextual/workspace/create`,
  `P/K/D`; default `Ctrl+Shift+N`.
- `workspace.chat.open`: `Write`, `durable_backend`,
  `contextual/workspace/chat/open`, `P/K/D`; default `Ctrl+Shift+I`.
- Navegação de abas — `workspace.tab.next`, `workspace.tab.previous`,
  `workspace.tab.first`, `workspace.tab.second`, `workspace.tab.third`,
  `workspace.tab.fourth`, `workspace.tab.fifth`, `workspace.tab.sixth`,
  `workspace.tab.seventh`, `workspace.tab.eighth`, `workspace.tab.ninth`:
  `Read`, `local_ui`, `ui/workspace/tab/navigate`, `P/K/D`. Defaults:
  `Ctrl+Tab`, `Ctrl+Shift+Tab`, `Ctrl+PageDown`, `Ctrl+PageUp` e
  `Ctrl+1..9`.
- Navegação de aplicação — `navigation.landmark.next`,
  `navigation.landmark.previous`, `navigation.landmark.default`,
  `navigation.workspace.open`, `navigation.history.open`,
  `navigation.memories.open`, `navigation.tasklists.open`,
  `navigation.jobs.open`, `navigation.profiles.open`,
  `navigation.settings.open`, `navigation.help.open`, `navigation.about.open`,
  `navigation.menu.open`, `navigation.palette.open`,
  `navigation.data.export.open`, `navigation.data.import.open`:
  `Read`, `local_ui`, `ui/navigation/<ação>`, `P/K/D`. Defaults efetivos:
  `Alt+W`, `Alt+Backspace` e `Ctrl+N` contextual em Histórico para
  `navigation.workspace.open`; `Alt+C/H/L/T/J/P/M`, `Ctrl+K`, `Alt+E/I`,
  `F1` e `F6/Shift+F6` para os respectivos IDs. `default` e `about` não têm
  default publicado.

Handler/efeito comprovados por `app_command_product_catalog.go`,
`app_command_workspace_chat.go`, `app_command_workspace_create.go`,
`app_command_ui_catalog.go`, `app_command_keyboard.go` e
`app_command_ui.go`; ingressos e efeitos têm provas em
`app_command_product_catalog_test.go`, `app_command_product_readiness_test.go`,
`app_command_keyboard_navigation_test.go`, `app_command_keyboard_sequence_test.go`,
`app_command_palette_contextual_test.go`, `app_command_deck_contextual_test.go`,
`app_command_workspace_chat_test.go`, `app_command_workspace_create_test.go` e
`app_command_ui_integration_test.go`.

### Chat — 24 IDs

- Apresentações/pickers — `chat.focus.input`, `chat.focus.messages`,
  `chat.message.read.open`, `chat.message.menu.open`,
  `chat.message.reasoning.toggle`, `chat.message.thread.expand`,
  `chat.message.thread.collapse`, `chat.message.edit.open`,
  `chat.pinned.open`, `chat.tokens.open`, `chat.model.open`,
  `chat.history.open`, `chat.profile.open`: `Read`, `local_ui`, rotas
  `ui/chat/...`, `P/K/D`; os sete primeiros IDs (até `thread.collapse`) também
  aceitam `U`. Defaults: `Ctrl+M/H/P` para model/history/profile.
- Ações de turno — `chat.message.send`, `chat.message.retry`,
  `chat.response.cancel`: `Write`, `durable_backend`,
  `contextual/chat/{send|retry|cancel}`, `P/K/D`; sem default.
- `chat.conversation.clear`: `Destructive` + `Interactive`, `durable_backend`,
  `contextual/chat/conversation/clear`, `P/K/D`; default `Ctrl+L`.
- Mensagem — os sete registros `chat.message.copy`,
  `chat.message.copy_markdown`, `chat.message.speak`,
  `chat.message.pin.toggle`, `chat.message.delete`,
  `chat.message.edit.save`, `chat.message.send_to_editor`: os quatro efeitos
  locais (`copy`, `copy_markdown`, `speak`, `send_to_editor`) são `Write`,
  `audited_ui`, `HandlerUI`, rotas `contextual/chat.message.*`, `P/K/D`;
  `pin.toggle` e `edit.save` são `Write`, `durable_backend`,
  `HandlerBackend`, nas mesmas rotas; `delete` é `Destructive` + `Interactive`,
  `durable_backend`, `HandlerBackend`, `contextual/chat.message.delete`,
  `P/K/D`. `chat.message.edit.open` é o picker local acima. Não há default de
  teclado para essas ações.

As rotas/classes finais vêm de `app_command_chat_actions.go`,
`app_command_conversation_clear.go` e `app_command_chat_message.go`; a cadeia
ingresso → captura de conversa/mensagem → commit está coberta por
`app_command_chat_navigation_test.go`, `app_command_chat_actions_test.go`,
`app_command_conversation_clear_test.go`, `app_command_chat_message_test.go`,
`app_command_chat_inspection_test.go` e pelos testes frontend
`commandChatNavigation.test.ts`, `commandChatMessaging.protocol.test.ts`,
`commandChatMessageWails.test.ts` e `commandChatEditorTransfer.test.ts`.

### Editor — 60 IDs

- Menus/presentações — `editor.mermaid.open`, `editor.menu.insert.open`,
  `editor.menu.file.open`, `editor.menu.format.open`, `editor.menu.mode.open`,
  `editor.slides.open`, `editor.presentation.fullscreen`,
  `editor.table.cell.next`, `editor.table.cell.previous`: `Read`, `local_ui`,
  rotas `ui/editor/...`, `P/K/D`; defaults `Alt+I` contextual no editor,
  `Alt+S` para slides e `F5` para fullscreen. Os demais não têm default.
- Modos — `editor.mode.markdown`, `editor.mode.rich`, `editor.mode.view`:
  `Write`, `durable_backend`, `contextual/workspace/editor/mode/{markdown|rich|view}`,
  `P/K/D`; defaults `Alt+1/2/3`.
- Arquivos — `editor.file.open`, `editor.file.save`,
  `editor.file.save_copy`: `Write`, `durable_backend`,
  `contextual/editor/file/{open|save|save_copy}`, `P/K/D`; defaults
  `Ctrl+O`, `Ctrl+S` e `Ctrl+Shift+S`.
- Mermaid/formatação — `editor.mermaid.apply`, `editor.mermaid.remove`,
  `editor.format.bold`, `editor.format.italic`, `editor.format.strike`,
  `editor.format.paragraph`, `editor.format.heading.h1`,
  `editor.format.heading.h2`, `editor.format.heading.h3`,
  `editor.format.heading.h4`, `editor.format.heading.h5`,
  `editor.format.heading.h6`, `editor.format.blockquote`,
  `editor.format.code_block`, `editor.format.list.bullet`,
  `editor.format.list.ordered`, `editor.format.clear_marks`,
  `editor.format.link.remove`, `editor.format.table.row.before`,
  `editor.format.table.row.after`, `editor.format.table.row.delete`,
  `editor.format.table.column.before`, `editor.format.table.column.after`,
  `editor.format.table.column.delete`, `editor.format.table.header.row`,
  `editor.format.table.header.column`, `editor.format.table.header.cell`,
  `editor.format.table.merge`, `editor.format.table.split`,
  `editor.format.table.delete`, `editor.format.link.set`,
  `editor.format.table.insert`, `editor.format.code_block.insert`,
  `editor.format.mermaid.insert`: `Write`, `audited_ui`, `HandlerUI`,
  `ui/editor/mermaid/{apply|remove}` ou `ui/editor/format/<sufixo>`, `P/K/D`.
  Defaults existentes: `Ctrl+B`, `Ctrl+I`, `Ctrl+Shift+X`,
  `Ctrl+Alt+0..6`, `Ctrl+Shift+B`, `Ctrl+Alt+C`, `Ctrl+Shift+8/7`.
- Slides — `editor.slide.insert.basic`, `editor.slide.insert.title`,
  `editor.slide.insert.two_columns`, `editor.slide.insert.image_right`,
  `editor.slide.insert.image_left`, `editor.slide.insert.section`,
  `editor.slide.insert.agenda`, `editor.slide.insert.quote`,
  `editor.slide.insert.comparison`, `editor.slide.insert.code`,
  `editor.slide.insert.diagram`: `Write`, `audited_ui`, `HandlerUI`,
  `ui/editor/slide/insert/<sufixo>`, `P/K/D`; sem default.

Nos três IDs de diálogo/formulário (`editor.format.link.set`,
`editor.format.table.insert`, `editor.mermaid.remove`), o handler continua
`a.startCommandUI` e o timeout produtivo é de cinco minutos; isso é diferente
de tornar o comando executável pela CLI. As fontes finais são
`app_command_editor_format.go`, `app_command_editor_file.go`,
`app_command_editor_mode.go`, `app_command_ui.go` e
`app_command_product_catalog.go`. A cadeia está exercitada por
`app_command_editor_format_test.go`, `app_command_editor_file_catalog_test.go`,
`app_command_editor_file_commit_test.go`, `app_command_editor_file_lifetime_test.go`,
`app_command_editor_mode_test.go`, `app_command_editor_mode_commit_test.go`,
`app_command_mermaid_test.go` e pelos testes frontend
`commandEditorFormatting.test.ts`, `commandEditorFormatting.preparation.test.ts`,
`commandEditorFileExecution.test.ts`, `commandEditorMode.test.ts`,
`commandEditorMermaid.test.ts` e `commandEditorPresentation.test.ts`.

### Tasklists, perfis e terminal — 23 IDs

- Apresentações de página — `tasklist.task.create.open`,
  `tasklists.create.open`, `tasklists.edit.open`, `tasklists.search.focus`,
  `profiles.create.open`, `profiles.edit.open`, `profiles.search.focus`,
  `terminal.sessions.open`, `terminal.focus.input`, `terminal.focus.history`:
  `Read`, `local_ui`, `HandlerUI`, rotas `ui/tasklist/...`, `ui/tasklists/...`,
  `ui/profiles/...` e `ui/terminal/...`, origens `P/K/D/U`. Defaults somente
  `Ctrl+N` contextual para `tasklists.create.open` e `profiles.create.open`.
- Mutações de listas — `tasklists.create`, `tasklists.update`,
  `tasklists.duplicate`, `tasklists.delete`, `tasklists.clear`: `Write`, salvo
  `delete`/`clear` que são `Destructive` + `Interactive`, todos
  `durable_backend`, `HandlerBackend`, rotas `contextual/page/tasklists.*`,
  origens `P/K/D`; sem default.
- Mutações de perfis — `profiles.create`, `profiles.update`,
  `profiles.duplicate`, `profiles.delete`, `profiles.activate`: `Write`, salvo
  `delete` que é `Destructive` + `Interactive`, todos `durable_backend`,
  `HandlerBackend`, rotas `contextual/page/profiles.*`, origens `P/K/D`; sem
  default. As mutações de perfil também marcam
  `MutatesEffectiveCapability`.
- Sessão de terminal — `terminal.session.create` é `Write` e
  `terminal.session.close` é `Destructive` + `Interactive`; ambos
  `durable_backend`, `HandlerBackend`, rotas
  `contextual/terminal/session/{create|close}`, `P/K/D`; sem default.
- `terminal.command.interrupt`: `Write`, `durable_backend`, `HandlerBackend`,
  `contextual/terminal/command/interrupt`, `P/K/D`; sem default.

O ingresso → captura de alvo → commit desses grupos está em
`app_command_page_presentation.go`, `app_command_page_mutation.go`,
`app_command_terminal_session.go` e `app_command_terminal_operation.go`; as
provas concretas incluem `app_command_page_presentation_test.go`,
`app_command_page_mutation_test.go`, `app_command_profile_mutation_test.go`,
`app_command_terminal_session_test.go`, `app_command_terminal_operation_test.go`,
`app_command_terminal_readiness_test.go` e os testes frontend de
`TaskListView`, páginas de perfis e superfícies do terminal.

### Hotkeys globais e camadas — 5 IDs

- `voice.input.activate`: `Read`, `audited_ui`, `HandlerUI`,
  `ui/voice/input/activate`, origem exclusiva `G`; sem default local. O
  ingresso vem da projeção do perfil/Manager nativo e termina no handoff
  autenticado, não no evento de voz direto.
- `job.run`: `Destructive` + `Interactive`, `durable_job`, `HandlerJob`,
  `internal/jobs/run`, origem exclusiva `G`; sem default local. O handler
  preserva `when`, profile, grant, cancelamento e deadline do runtime.
- `layer.activate`, `layer.toggle`, `layer.back`: `Write`,
  `durable_backend`, `HandlerBackend`, `internal/layer/action`, origens
  `P/K/D/C`; sem default automático porque `scope`/`rule_id` são argumentos
  escolhidos na configuração. Ações de camada alteram a capacidade efetiva.

As fontes finais são `app_command_global_catalog.go`,
`app_command_global_execution.go`, `app_command_global_projection.go`,
`app_command_layer_catalog.go` e `app_command_product_catalog.go`. Há provas
em `app_command_global_execution_test.go`,
`app_command_global_job_integration_test.go`,
`app_command_job_reactive_integration_test.go`,
`app_command_layer_catalog_test.go`, `app_command_layer_keyboard_integration_test.go`,
`app_command_palette_layer_contextual_test.go` e
`app_command_deck_layer_contextual_test.go`. A prova de Chat no mesmo ciclo,
incluindo `layer.activate`, `layer.toggle`, `layer.back` e replay, está em
`app_command_layer_origin_convergence_test.go::TestCommandLayerChatToggleAndBackPersistAndReplay`.

### Defaults e projeções

Os **67 defaults de teclado** não são inferidos do número de comandos: são os
63 itens de `commandKeyboardDefaultSpecs` mais os quatro itens de
`commandKeyboardSequenceDefaultSpecs`, publicados na camada builtin
`application.keyboard`. O teste `app_command_keyboard_defaults_test.go` confere
quantidade, IDs, triggers e fingerprints; restauração/override é coberta por
`internal/commandbindings/defaults_test.go`, `restore_test.go` e
`projection_equivalence_test.go`.

A camada builtin `application.palette` projeta os IDs elegíveis pela definição
e pelo schema de `{}`; as condições contextuais de paleta e Deck admitem 81
IDs cada, conforme `app_command_palette_conditions_test.go` e
`app_command_deck_conditions_test.go`. Isso é elegibilidade/projeção, não uma
prova de que 81 efeitos distintos foram exercitados. Defaults pessoais,
Stream Deck, condições de superfície e camadas de usuário passam pelo mesmo
resolver (`internal/commandbindings`, `internal/commandconfig`) e não criam
IDs novos.

### Tools e CLI: ingressos existentes, disponibilidade distinta

- As tools compostas `command_catalog` e `command_config` são registradas em
  `internal/app/app_tool_registry.go` e terminam em
  `commandAgentTools.Catalog/Config` (`app_command_agent.go`/
  `app_command_agent_config.go`). `command_catalog` fecha `list/describe/execute`;
  `command_config` fecha os verbos D10 de camadas, bindings e import/export.
  São tools de chat com identidade de `tool_invocation`; não são IDs adicionais
  do catálogo v40 e não autorizam uma origem `CLI`. Provas: `internal/tools/command/tools_test.go`,
  `internal/app/app_command_agent_test.go`, `app_command_agent_config_test.go` e
  `app_command_agent_portability_test.go`.
- A CLI pública existe em `cmd/asst/commands.go`, com
  `list/describe/execute/retry/status`; `internal/commandcli/service.go` usa
  `ExecuteEnvelopeWithResult` e mantém `request_id`/replay. A borda de App é
  `internal/app/app_command_cli.go`. `app_command_cli_test.go`,
  `internal/commandcli/integration_test.go`, `cmd/asst/commands_test.go` e
  `cmd/asst/tools_test.go` são evidência do ingresso e das recusas.
- A lista CLI devolve os 149 IDs, mas nenhum registro atual contém `CLI` em
  `AllowedSources`; logo `Executable=false`/`source_cli_not_allowed` é o estado
  vigente. Comandos visuais, interativos, destrutivos, de camada ou com contexto
  visual também permanecem bloqueados por gates headless. Não registrar “CLI
  ausente” nem “CLI executa comandos produtivos”: o ingresso existe e a saída
  positiva ainda não foi qualificada/autorizada para uma família.

### Ligações reconciliadas — seção140, 22/09/2026

- `navigation.about.open` aceita `P/K/D` e declara a rota
  `ui/navigation/about/open`. `app_command_product_catalog_test.go::TestCommandProductCatalogNavigationMetadataPaletteAndKeyboard`
  prova contrato, fontes e handler; `frontend/src/lib/commandNavigation.test.ts`
  prova literalmente o mapa `/about`. `Topbar.palette.integration.test.tsx`
  e `Topbar.test.tsx` agora atravessam paleta Combobox, teclado configurado
  e evento Deck até `navigate('/about')`, incluindo recusa de contexto
  inválido. **Ligação reconciliada**, sem inventar atalho padrão para Sobre.
- `editor.format.code_block` aceita `P/K/D` e tem o default
  `Ctrl+Alt+C`. `app_command_keyboard_defaults_test.go::TestCommandKeyboardDefaultsProjectStableApplicationLayer`
  confirma o binding e
  `app_command_editor_format_test.go::TestEditorFormatBeginTakeCompleteAndRepeatDenied`
  confirma o contrato/handoff genérico dos IDs de formatação. Agora
  `Topbar.editorMode.integration.test.tsx` injeta Ctrl+Alt+C pela cadeia
  `keyboard.local` até `codeBlock` no TipTap real, com conteúdo preservado;
  paleta, solicitação de menu e reserva Deck convergem para a transformação.
  Readonly e alvo stale não a executam. **Ligação reconciliada**.
- `chat.message.thread.collapse` aceita `U/P/K/D`. O registro e a origem são
  verificados em `app_command_product_catalog_test.go`, e
  `frontend/src/components/chat/ChatSessionView.messageActions.test.tsx`
  verifica o alvo local. `ChatNavigation.origins.integration.test.tsx` monta
  Topbar, ChatSessionView e MessageNode reais: clique no botão da mensagem,
  paleta, teclado e evento Deck levam a `aria-expanded=false` e remoção
  do filho do DOM, sem recolher a outra mensagem. **Ligação reconciliada**.
- `internal/app/app_command_family_origins_test.go` qualifica a projeção e
  emissão local de 16 IDs sem ledger, os 45 IDs de formatação pela oferta
  contextual Deck e os 14 defaults de formatação pela entrada de teclado.
  Os testes frontend mockam o transporte Wails e provam o consumidor real;
  os Go provam o produtor e persistência/ausência de ledger. Não representam
  uma execução HID/Wails ponta a ponta nem substituem aceite físico/NVDA.
- Revisão independente, comandos executados, resultados e limitações na
  seção140 da tasklist. C02 passa a implementação identificada, mantendo
  checkbox final e gates abertos.

Não há gap de implementação de CLI a listar: o contrato atual não permite
`CLI` nos 149 registros, e `app_command_cli_test.go::TestCommandCLIListsFullCatalogAndDescribesUnavailableWorkspaceCommand`
e `TestCommandCLIRejectsVisualWorkspaceAndLayerCommandsWithoutQuestionnaireOrEffect`
documentam precisamente listagem/recusa. A promoção de C02 decorre das
ligações qualificadas por família e das exceções explícitas, não das
contagens do catálogo. O aceite agregado de R07 continua pendente.

### Contexto histórico das seções104–108 (superado pela seção109)

A seção104 da [tasklist](0103-tasklist-conclusao.md) implementa o lote de
Histórico e lista no workspace auditado na seção103. Ctrl+N do Histórico é
binding contextual de retorno; N/D/Ctrl+L da lista são gestos locais dos
controles que solicitam os mesmos comandos dos botões, não novos defaults.
Residuais dessa auditoria ainda abertos: hotkeys de voz/perfil e jobs.
A seção105 corrige bordas dos ingressos existentes; não equivale à migração
integral, que continua pendente de publicação e resolução `keyboard.global`.
A seção106 substitui a borda nativa Windows por registro com no-repeat e
teardown cancelável. Não acrescenta comandos nem conclui os ingressos comuns.
A seção107 acrescenta gramática global na projeção, exclusividade no Manager
e preservação de hotkey/condição no handler de jobs. Os registros atuais ainda
não foram substituídos por bindings publicados no executor comum.
A seção108 conecta a reserva nativa Windows à exclusão DOM antes do registro,
com confirmação por instância/revisão. A ponte é comum à raiz do app e ao
teclado local e não acrescenta tráfego por tecla. O ingresso comum de voz/jobs
continua pendente; esta entrega não aumenta a contagem de comandos migrados.
Outros controles não auditados não são declarados migrados por este registro.
As tabelas `candidate.*` abaixo são o levantamento inicial, não uma lista de
comandos ainda por implementar. Não usar suas lacunas antigas como diagnóstico
atual nem contar listeners DOM como migrações pendentes automaticamente.

Pendências históricas resolvidas: broker de abrir/salvar/copiar arquivos
(seção83), modos do editor (81), link (86), Mermaid (94) e catálogo/ingressos
das cinco mutações de perfis (102). Seus aceites manuais permanecem separados.

## Histórico dos lotes

**Atualização seção102 (20/09/2026):** `profiles.create`, `.update`,
`.duplicate`, `.delete` e `.activate` são comandos backend contextuais.
Botões, paleta, teclado pessoal e Stream Deck compartilham preparo e commit;
salvar exige o formulário correspondente, operações de seleção exigem alvo
na página de perfis. Exclusão usa decisão backend, sem confirmação paralela.
Catálogo v35: **142 comandos / 60 locais / 66 defaults**, sem atalhos padrão
adicionais. Gate automatizado e manual na seção102 da tasklist. Os recortes
anteriores abaixo conservam o estado histórico, não a pendência vigente.

**Seção101 (20/09/2026):** as mutações nativas da tela de perfis recebem
coordenação de persistência e invalidação de comandos. Não há cinco novos
comandos nem novos atalhos; catálogo permanece137/locais60/defaults66.
Paleta/teclado/Deck de CRUD/ativação de perfis aguardam handoff de capability.

**Atualização seção100 (20/09/2026):** quatro mutações backend de
listas (`tasklists.create`, `.update`, `.duplicate`, `.delete`) e lifecycle
`terminal.session.create`/`.close`. Catálogo137/locais60/defaults66, v34,
sem inventar atalhos globais novos. UI integrada; regressão frontend de 2593
testes aprovada. Validação manual acumulada permanece aberta na tasklist.
CRUD/ativação de perfis continuam pendentes da coordenação com grants/epochs;
journal de arquivos isoladamente não fecha essa migração.

**Atualização seção99 (19/09/2026):** interrupção do terminal é um comando
contextual backend (`terminal.command.interrupt`), disponível na paleta e para
bindings pessoais/Deck. Ctrl+C sem seleção e botão da página solicitam o mesmo
efeito, sem chamar diretamente o store legado; Ctrl+C continua gesto local,
não novo binding global. Catálogo131/locais60/defaults66, produto v33.
Criar/encerrar sessão e CRUD/ativação de listas/perfis continuam pendentes.
Não confundir a correção do salvar lista com migração desse domínio.

**Atualização seção98 (19/09/2026):** Ctrl+N nas páginas de listas e perfis
migrado, sem listener concorrente. Abrir criação/edição e focar busca nessas
páginas, mais seletor de sessões/foco de entrada/histórico do terminal,
compartilham paleta/teclado/Deck. Produto v32: 130 comandos/60 locais/66 defaults.
Não migrados: CRUD de listas/perfis, ativação de perfil, processos do terminal
(incluindo Ctrl+C), hotkeys de perfil/job e demais controles já pendentes.
Detalhes e gates na seção98; baseline global inalterada.

**Atualização seção97 (19/09/2026):** ajuda e rótulos do Topbar, workspace,
chat e editor usam o mapa efetivo autenticado. Suprimir/remapear remove/altera
os anúncios; não há fallback de defaults quando a projeção está inválida.
Ctrl+? e gestos nativos permanecem próprios dos componentes, explicitamente
identificados na ajuda; não se afirma migração dos listeners restantes.
Catálogo121/locais51/defaults64 e baseline global inalterados.

**Atualização seção96 (19/09/2026):** implementadas sete apresentações
do chat: `chat.focus.input`, `.messages`, `chat.message.read.open`,
`.menu.open`, `.reasoning.toggle`, `.thread.expand` e `.thread.collapse`.
Sem novos defaults. Gestos da mensagem preservam seu escopo local;
paleta/teclas pessoais/Deck usam a mensagem de origem capturada. Não migra
type-ahead, paginação do histórico ou navegação por irmãos por inferência.
Produto v31: catálogo121/locais51/bindings padrão64. Evidências e pendências
na seção96 da tasklist; baseline global inalterada. Automação concluída;
geração oficial de bindings e aceite manual pendentes.

**Atualização seção95 (19/09/2026):** `navigation.landmark.next`, `.previous`
e `.default` migram a navegação entre regiões para execução local sem ledger.
F6/Shift+F6 são defaults configuráveis; Escape permanece gesto contextual
após os componentes, não binding global. Paleta preserva a região de origem
mesmo quando aberta pelo mouse; Deck usa a mesma ação. Modal só navega suas
próprias regiões habilitadas. Sem expansão da allowlist de repetição.
Produto v30: catálogo114/locais44/bindings padrão64. Automação concluída;
geração oficial e aceite manual pendentes. Baseline **58 I / 24 P / 2 N**
inalterada; evidências na seção95 da tasklist.

**Atualização seção94 (19/09/2026):** `editor.mermaid.open`, `.apply` e
`.remove` integram a edição existente. Abrir é apresentação local; aplicar e
remover usam execução auditada com bloco/documento/instância capturados.
Ctrl+S, Cmd+S e Ctrl+Enter no modal são controles invariantes do formulário,
com origem `keyboard.local`, não bindings globais remapeáveis. Ctrl+S fora
do modal continua salvando o documento. Teclas pessoais e Deck usam os IDs
canônicos; aplicar exige a sessão Mermaid topmost. A paleta permanece fora
dos modais. Geração oficial e aceite manual pendentes; evidências na seção94
da tasklist, sem promover a baseline global **58 I / 24 P / 2 N**.
Produto v29: catálogo111/locais41/bindings padrão62.

**Atualização seção93 (19/09/2026):** `chat.message.send_to_editor` integra
botão/menu, paleta, teclado configurável e Deck, sem nova tecla padrão.
Produto v28: catálogo108/locais40/bindings padrão62.
O padrão é mensagem inteira em Markdown → novo documento; menus preservam
snippets, formatos e destino escolhidos. Fonte capturada, transição protegida
e ACK de aplicação real no editor; não há retry automático. Resultado
desconhecido pode deixar uma aba criada: conferir antes de repetir.
Bindings oficiais e aceite manual pendentes. Baseline **58 I / 24 P / 2 N**
inalterada; evidências focadas na seção93 da tasklist.

**Atualização seção92 (19/09/2026):** salvar edição agora usa
`chat.message.edit.save`. Botão e Ctrl+Enter no formulário compartilham a ação
com paleta, teclado configurável e Deck; Ctrl+Enter permanece um controle
nativo da edição, não novo default global remapeável. Catálogo107/locais40/
defaults62. Não inclui envio ao editor. Geração oficial e aceite manual
pendentes; evidências na seção92 da tasklist.

**Atualização seções90–91 (19/09/2026):** envio, cancelamento e nova
tentativa usam o fluxo de comandos contextual. O lote91 integra ações sobre
mensagem selecionada (copiar texto/Markdown, falar, abrir edição, fixar e
excluir), validadas automaticamente. Abrir edição é apresentação local, não salvamento.
Controles da lista como F2, Delete, Espaço e Ctrl+C acionam a mesma ação,
mas continuam interações locais da lista, não novos defaults globais. O
remapeamento desses gestos nativos não é declarado entregue. Paleta e
acionadores configuráveis usam IDs canônicos e captura explícita do alvo.
Geração oficial de bindings e aceite manual permanecem pendentes. Estado
detalhado e evidências na tasklist, seções90–91; o inventário abaixo é histórico.

**Leitura atual (18/09/2026):** este inventário é a fotografia inicial, não
o estado atual dos atalhos. Catálogo e migrações até o lote79 estão na
[tasklist reconciliada](0103-tasklist-conclusao.md), seções4/5/76. Workspace,
navegação e chat contextual já têm comandos reais; famílias específicas de
chat/editor/perfis/jobs ainda mantêm caminhos legados. A atualização completa
do inventário permanece em R07.1/R12.3; não tratar os `candidate.*` abaixo
como IDs públicos nem as lacunas históricas como defeitos ainda presentes.

**Atualização seções88–89 (19/09/2026):** Ctrl+L foi migrado para
`chat.conversation.clear`, com decisão destrutiva no backend e limpeza
atômica da conversa capturada. A referência ao Ctrl+L legado abaixo descreve
o estado do lote78, não o atual. `chat.pinned.open` e `chat.tokens.open`
abrem consultas existentes por botão/paleta/acionadores configuráveis, sem
novas teclas padrão nem ledger. Aceite manual acumulado continua pendente;
ações internas dessas consultas e outras operações de chat não estão migradas.

**Atualização seção83 (19/09/2026):** Ctrl+O/S/Shift+S agora correspondem a
`editor.file.open`, `editor.file.save` e `editor.file.save_copy`, pelo broker
de arquivo, incluindo paleta/menu/Deck. Os listeners anteriores dessas três
teclas foram retirados. Alt+1/2/3 já foram migrados na seção81. As referências
a esses atalhos como legados abaixo são históricas; aceite manual da seção83
permanece pendente, sem encerrar outras ações de conteúdo ou R07 integral.

Lote78: Ctrl+M/H/P foram migrados para `chat.model.open`, `chat.history.open`
e `chat.profile.open`, com teclado, paleta e Deck no dispatcher comum.
O chat modal usa escopo fechado de apresentação topmost; Ctrl+L continua
legado, por excluir conteúdo. Aceite manual do lote permanece pendente.

Lote79: menus Arquivo/Formatar/Modo, seletor de slides (Alt+S) e fullscreen
(F5) do editor são apresentação local no dispatcher. Seção80 acrescenta
`editor.menu.insert.open`: Alt+I é resolvido contextualmente para Inserir no
editor apto e para Importar fora dele, sem fallback legado. Ctrl+S/O,
Ctrl+Shift+S, Alt+1/2/3 e ações de conteúdo permanecem em seus fluxos atuais.

Seção80: o registro do editor inclui Arquivo, Formatar, Inserir, Modo, slides
e fullscreen. A captura preserva owner/session/workspace/aba/documento/
instância e bloqueia readonly, view, painel inativo, modal, overlay, IME,
target externo e contexto ABA. A abertura reutiliza o botão/controle existente;
não seleciona nem escreve conteúdo. Catálogo 43, local UI 35, defaults 41
(37 v1 + quatro v2), 40 combinações efetivas.

Seção81 acrescenta os comandos duráveis `editor.mode.markdown`,
`editor.mode.rich` e `editor.mode.view`, com Alt+1/2/3. O contrato usa
Begin/Take/Commit e `workspace/active_tab` com `ExactVersion`; o backend faz
CAS e só altera `Tab.State.displayMode`. Replay/ABA, inclusive repetição do
mesmo enum, falha de persistência e concorrência são rejeitados com rollback.
A UI faz flush do rich antes da confirmação e só aplica depois dela; mudança
de alvo durante a preparação cancela sem roubar foco. IME ativo bloqueia,
com exceção `UnknownIME` restrita a modos `native`/`rich` reconhecidos pelo
registry. Ctrl+S/O/Shift+S continuam legados. Contagens: catálogo 46, local
UI 35, defaults 44 (40 v1 + 4 v2), 43 combinações efetivas.

**Escopo das tabelas abaixo:** fotografia histórica inicial, com algumas
anotações intermediárias. Não representa o estado atual do código. Para
pendências vigentes, consultar a seção103 da tasklist.

`candidate.*` são IDs sugeridos, não IDs registrados. Combinações e ações
abaixo são somente as observadas no código.

## Workspace e navegação

**Atualização da seção81:** este inventário agora deve ser lido até o lote81;
as referências ao lote79 e à data de 18/09/2026 são históricas. Alt+1/2/3
não são legados: são os comandos persistentes `editor.mode.markdown`,
`editor.mode.rich` e `editor.mode.view`. IME ativo sempre bloqueia;
`UnknownIME` é estado distinto e só aceita controles native/rich suportados
quando reconhecidos por registry válido.
| Arquivo/símbolo exato | Combinação/gesto | Ação e contexto atual | Candidato, migração e cobertura |
|---|---|---|---|
| `frontend/src/hooks/useWorkspaceKeyboardShortcuts.ts` — `handleKeyDown`, `CHORD_MAP` | `Ctrl+?`/`Ctrl+Shift+/`; `Ctrl+Shift+I`; `Ctrl+Shift+N` | Ajuda; chat modal da aba ativa; novo workspace. Modal bloqueia a UI de fundo, exceto alternância da ajuda; editor read-only bloqueia o chat modal. | `candidate.workspace.shortcuts-help`, `.chat-modal`, `.new-workspace`. Preservar captura, prevenção do DevTools e aba ativa. Cobertura: `useWorkspaceKeyboardShortcuts.test.ts(x)`. |
| mesmo — modo chord | `Ctrl+N`, depois `C/E/R/T`; timeout 1500 ms | Abre `workspace:open-new-tab-menu`; cria aba chat/editor/terminal/tasklist. Modal cancela; datagrid é excluído; timeout limpa o chord. | `candidate.workspace.new-tab-menu`/`.new-tab.<tipo>`. Resolver conflito real com o listener de painel sem dupla execução. Cobertura no mesmo par de testes. |
| mesmo — `navigateTab` e ramos numéricos | `Ctrl+T`; `Ctrl+W`/`Ctrl+F4`; `Ctrl+Tab`/`Ctrl+Shift+Tab`; `Ctrl+PageDown`/`Ctrl+PageUp`; `Ctrl+1..9` | Cria/fecha/circula/seleciona abas; modal bloqueia; `[data-tab-scope]` delega; troca/remoção restaura foco e anuncia posição. | `candidate.workspace.new-chat-tab`, `.close-tab`, `.next-tab`, `.previous-tab`, `.select-tab`. Não inferir bloqueio de inputs onde o código não o faz. Cobertura do hook. |
| `frontend/src/hooks/useActivePanelShortcut.ts` — `useActivePanelNewShortcut` | `Ctrl+N` | CRUD `onNew()` só no `ActivePanelContext` ativo; ignora editáveis e modal; listener de captura separado. | `candidate.panel.new-resource`. Requer precedência/consumo único contra o chord. Cobertura: `useActivePanelShortcut.test.tsx`. |
| `frontend/src/components/layout/Topbar.tsx` — `Topbar`/`handleKeyDown` | `F1`; `Alt+M`; `Alt+Backspace`; `Alt+W/C/H/L/T/J/P/I` | Ajuda, menu, retorno e rotas. F1 sempre previne default; Alt+Backspace previne fora de editáveis mesmo com modal, mas não navega; demais Alt exigem foco não editável e ausência de modal. | `candidate.navigation.help`, `.menu`, `.back`, `.route.<rota>`. Preservar distinção consumir/executar. Cobertura: `Topbar.test.tsx`. |
| `frontend/src/hooks/useLandmarkNavigation.ts` — efeitos `F6`/`Escape` | `F6`/`Shift+F6`; `Escape` | Percorre landmarks em captura; volta ao landmark default em bubbling, com guards de modal e foco. | `candidate.navigation.next-landmark`, `.previous-landmark`, `.default-focus`. Cobertura: `useLandmarkNavigation.test.tsx`. |

## Chat

| Arquivo/símbolo exato | Combinação/gesto | Ação e contexto atual | Candidato, migração e cobertura |
|---|---|---|---|
| `frontend/src/components/chat/ChatToolbar.tsx` — efeito `handleKeyDown` | `Ctrl+M/L/H/P` | Modelo, limpar conversa, histórico, perfil. Captura; sem repeat/IME; superfície ativa e modal topmost; overlays bloqueiam (com prevenção seletiva). | `candidate.chat.model-picker`, `.clear`, `.history-picker`, `.profile-picker`. Preservar `enableShortcuts`, portais e consumo sem execução. Cobertura extensa em `ChatToolbar.test.tsx`. |
| `frontend/src/components/chat/ChatInput.tsx` — `handleKeyDown` | `Enter`, `Shift+Enter`, `Escape`, `ArrowUp`; setas/Tab/Enter no `/` | Envia, nova linha, cancela streaming, vai à lista; menu slash navega/seleciona/fecha. Foco retorna ao textarea após envio. | `candidate.chat.send`, `.cancel-generation`, `.message-history`, `.slash-*`. Depende de input, streaming e menu. Cobertura em `ChatInput*.test.tsx`, inclusive `ChatInput.agentCommands.test.tsx`. |
| `frontend/src/components/chat/ChatSessionView.tsx` — listeners de `Escape`/`?` | `Escape`; `?` | Fecha menu ou restaura foco no input dentro do chat; `?` abre ajuda fora de input, em superfície ativa e sem modal, via `keypress`. | `candidate.chat.restore-input-focus` e o mesmo `candidate.workspace.shortcuts-help`. Preservar ordem local/bubbling/landmark. Cobertura: `ChatSessionView.test.tsx`. |
| `frontend/src/hooks/useChatKeyboardNav.ts` — `handleKeyDown` | `ArrowUp` no input; caractere imprimível ao navegar mensagens | Foca última mensagem; retorna ao input e injeta caractere via setter nativo + evento `input`; ignora modificadores. | `candidate.chat.focus-messages` (não é comando global). Cobertura: `useChatKeyboardNav.test.tsx`. |
| `frontend/src/components/chat/MessageList.tsx` — `handleListKeyDown`; `ChatMessage.tsx` — `handleKeyDown/Up` | `Ctrl+Home/End`; `ArrowDown`; `Space`; `Escape`; `Ctrl+Enter`; Tab/Shift+Tab; `ContextMenu`/`Shift+F10` | Salta janela, entra em mensagem, fala TTS, cancela/salva edição, prende foco e abre contexto. Leitura deixa controles internos; edição não propaga outras teclas. | `candidate.chat.jump-start/end`, `.speak-message`, `.save-edit`, `.context-menu`. Dependem de leitura/edição, virtualização e foco. Cobertura em `MessageList`, `MessageNode` e `ChatSessionView`. |

## Editor

| Arquivo/símbolo exato | Combinação/gesto | Ação e contexto atual | Candidato, migração e cobertura |
|---|---|---|---|
| `frontend/src/pages/useEditorMenus.tsx` + `frontend/src/lib/commandEditorPresentation.ts` | `Alt+S`, `F5`, `Alt+I`; `Alt+1/2/3`, `Ctrl+S`, `Ctrl+Shift+S`, `Ctrl+O` | Apresentação local de slides/fullscreen e Inserir pelo mapa contextual; Alt+I resolve Importar fora do editor. Arquivo/Formatar/Modo também abrem pelos comandos locais, com capacidades próprias. Persistentes Ctrl+S/O/Shift+S e Alt+1/2/3 permanecem fora da migração. | IDs públicos `editor.menu.file.open`, `.format.open`, `.insert.open`, `.mode.open`, `editor.slides.open`, `editor.presentation.fullscreen`; cobertura real em `EditorPage.test.tsx`/`EditorToolbar.test.tsx` e registry. |
| `internal/workspace/command_editor_mode.go` + comando editor.mode | `Alt+1`, `Alt+2`, `Alt+3` | Persiste Markdown, Rich ou View na aba editor ativa; exige confirmação e alvo `workspace/active_tab` versionado. Não altera conteúdo/arquivos; IME ativo bloqueia. | IDs públicos `editor.mode.markdown`, `.rich`, `.view`; contrato Begin/Take/Commit, CAS/rollback e testes de replay/ABA, cancelamento, storage e concorrência. |
| `frontend/src/lib/commandEditorFormatting.ts` + `commandEditorInputs.ts` | Sem default novo; Ctrl+K permanece da paleta | `editor.format.link.set` abre formulário compartilhado e aplica o link à seleção capturada; handler Ctrl+K legado removido. | Migração seção86: paleta/menu/tecla configurada/Deck; testes reais de preparação, cancelamento e editor. |
| `frontend/src/pages/editorMenus/formatMenu.ts` + `EditorToolbar.tsx` — itens | `Ctrl+B/I/Shift+X`, `Ctrl+Alt+0..6/C`, `Ctrl+Shift+B/7/8`; outros rótulos `Ctrl+K`, `Alt+I/S` | Seções84–85: 28 ações `editor.format.*` auditadas, incluindo parágrafo/títulos/listas/blocos, remover link e 12 alterações de tabela. Keymaps migrados removidos; Ctrl+Alt depende de Ctrl e Alt esquerdo observados, nunca AltGr. Diálogo de link e navegação de células ainda legados. | Testes TipTap reais em `commandEditorFormatting*.test.ts`, menu e integração Topbar/Combobox/Deck; capture/dispatcher com Ctrl+Alt explícito. Inserção parametrizada e aceite manual permanecem pendentes. |
| `frontend/src/components/editor/EditorContentArea.tsx` — preview Mermaid/reading | `Enter`; `Backspace`/`Delete`; caractere imprimível | Abre/remove Mermaid, type-to-edit e modo leitura. Só wrapper focado e documento editável. | `candidate.editor.mermaid.open/remove/type-to-edit`, `.reading-mode`. Cobertura: `EditorContentArea.test.tsx`. |
| `frontend/src/components/editor/MermaidEditorModal.tsx` — `onKeyDown` | `Escape`; `Ctrl/Cmd+S`; `Ctrl+Enter` | Fecha/aplica no modal; Escape não propaga. | `candidate.editor.mermaid.apply`; deve obedecer `DialogCommandScope` topmost. Cobertura: `MermaidEditorModal.test.tsx`. |

## Perfis, hotkeys globais e jobs

| Arquivo/símbolo exato | Combinação/gesto | Ação e contexto atual | Candidato, migração e cobertura |
|---|---|---|---|
| `internal/profiles/types.go` — `TriggerConfig`; `internal/profiles/testdata/published/0.1.9.json` | `input.triggers[]`: `type=hotkey`, `hotkey` (ex. `Ctrl+Shift+Space`), `hotkey_global`, `hotkey_bring_to_front` | Valida tipo/presença; fixture é dado publicado, não registro de comando. | `candidate.profile.voice.toggle` (sugestão). Preservar perfil efetivo e global/local; cobertura de validação/fixtures de profiles. |
| `controllers/hotkeys_controller.go` — `RegisterActiveProfileHotkeys` | combinação do perfil ativo | Remove e registra triggers habilitados no `internal/hotkey.Manager`; callback throttle 1 s, emite `interaction:hotkey:triggered`; `WindowPort.Show()` só com global+bring-to-front. Usa `profileID=1` e contador local para throttle. | `candidate.profile.voice.trigger`. Esses números não são IDs canônicos. Manter troca de perfil, evento, throttle e foreground. Não há teste unitário específico do controller; há `internal/app/app_wire_test.go` e `internal/wailsapi/hotkeys_test.go`. |
| `frontend/src/hooks/useInteractionProfile.ts` — `ensureHotkeyListener`/callback | evento Wails `interaction:hotkey:triggered` | Listener singleton, throttle adicional 1 s, alterna gravação/wakeword via ref; perfil pode ser tab/workspace/global. | `candidate.profile.voice.dispatch`. Resolver deduplicação backend/frontend e origem. Não há cobertura de hotkey explícita localizada no hook. |
| `internal/hotkey/hotkey.go` — `Manager.Register`, `RegisterProfileHotkey`, `ParseCombination`; `hotkey_{windows,linux,darwin}.go` | hotkey OS com Ctrl/Shift/Alt/Win/Cmd | Listener global em goroutine; IDs inteiros de processo; unregister individual/lote; parser toma último token como tecla e ignora modificadores desconhecidos. | `candidate.adapter.os-global-hotkey` (adapter, não comando). Exige exclusividade/conflito, ciclo de vida e layout. Não foram localizados testes no pacote. |
| `internal/jobs/types.go` — `Trigger`/`TriggerHotkey`; `internal/jobs/parser.go` | `type: hotkey`, `keys: "Ctrl+Alt+D"`, `when` | Parser exige `keys`; `when` continua condição/template do runtime de jobs. | `candidate.job.trigger.<job-slug>`; slug/IDs devem ser derivados, não inventados. AEP-0103 não converte `when` nesta etapa. |
| `internal/jobs/manager.go` — `registerTriggersLocked`, `registerJobHotkey`, `unregisterJobHotkeys` | `Trigger.Keys` | Faz parse, registra no mesmo manager; callback cria `TriggerContext` e chama `executeJob`; unregister acompanha recarga/remoção. | `candidate.job.invoke`. Preservar `when`, contexto, cancelamento e unregister. Cobertura relacionada: `manager_runtime_test.go`, `executor*_test.go`, `scheduler_test.go`; sem teste focalizado de colisão/registro. |
| `frontend/src/components/jobs/builder/TriggerEditor.tsx`; `internal/app/builtin/skills/job-manager/SKILL.md` | edição/descrição de `hotkey.keys` | Configuração/documentação de trigger, sem catálogo canônico nem executor frontend. | Superfície de configuração, não novo caminho de execução. Cobertura: `JobBuilder.test.tsx`. |

## Lacunas históricas da inspeção inicial — não são pendências atuais

- Não existe evidência de catálogo canônico conectado aos handlers; o protótipo
  `internal/commandbindings` apenas seleciona candidatos e não autoriza/executa.
- `Ctrl+N` tem dois listeners reais; falta contrato de precedência e teste de
  consumo único.
- Faltam testes focados de conflito/colisão, registro do pacote
  `internal/hotkey`, controller de perfil e registro de hotkey de jobs.
- Faltam IDs estáveis, argumentos, `effect_class`, origens permitidas e
  fingerprint/claims; IDs inteiros atuais são efêmeros.
- Faltam contratos para modal topmost, inputs/editáveis, menus portalados,
  keep-alive, foco restaurado, composição IME/repeat e listeners capture/bubble.
- Stream Deck, exclusividade/reconexão física e foreground externo são
  pendências da Fase 0, não resultados desta inspeção.
- A migração deve manter `job.when` no runtime de jobs, respeitar
  `DialogCommandScope`/AEP-0091 e não criar execução alternativa no frontend.

## Próximos passos históricos da inspeção inicial

1. Testar precedência de `Ctrl+N`, modal topmost, input/editável, aba ativa e
   hotkey global sem migrar handlers.
2. Só então converter candidatos em catálogo com IDs estáveis e contratos de
   origem/argumentos/efeito.
3. Medir duplicação e latência e registrar protótipos Stream Deck/foreground
   como pendências, sem declarar conclusão da Fase 0.

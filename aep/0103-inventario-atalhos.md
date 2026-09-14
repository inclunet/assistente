# AEP-0103 — Inventário inicial de atalhos (Fase 0)

**Status:** Documento de apoio — Fase 0 em andamento  
**Escopo:** fotografia compacta do código existente; não é novo AEP, não
altera o AEP-0103 principal e não conclui a Fase 0.

`candidate.*` são IDs sugeridos, não IDs registrados. Combinações e ações
abaixo são somente as observadas no código.

## Workspace e navegação

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
| `frontend/src/pages/useEditorMenus.tsx` — efeito `onKeyDown` | `F5`; `Alt+1/2/3`, `Alt+I/S`; `Ctrl+S`, `Ctrl+Shift+S`, `Ctrl+O` | Fullscreen Reveal, modo, Inserir/slides, salvar/salvar cópia/abrir. Só painel ativo/aba com ID; modal/menu bloqueia; read-only só permite `Alt+3`; `isAsking` bloqueia modo. | `candidate.editor.fullscreen`, `.mode.*`, `.insert-menu`, `.slide-picker`, `.save`, `.save-as`, `.open`. Não há teste focalizado identificado; cobertura indireta em `EditorPage.test.tsx`/`EditorToolbar.test.tsx`. |
| `frontend/src/components/editor/RichTextEditor.tsx` — `onKeyDown` | `Ctrl+K`/`Cmd+K` | Abre diálogo de link na região do editor e previne navegador. | `candidate.editor.link-dialog`; coexistência com menu Formatar. Cobertura: `RichTextEditor.test.tsx`. |
| `frontend/src/pages/editorMenus/formatMenu.ts` + `EditorToolbar.tsx` — itens | rótulos `Ctrl+B`, `Ctrl+I`, `Ctrl+Shift+X`, `Ctrl+K`, `Alt+I/S` | Rótulos de ações Rich Text/controles; a execução dos formatadores ocorre no menu/TipTap, sem outro listener global observado. | Só catalogar após confirmar handler nativo; não tratar rótulo como binding. Cobertura: `EditorToolbar.test.tsx` e `RichTextEditor.test.tsx`. |
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

## Lacunas e dependências explícitas

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

## Próximos passos verificáveis

1. Testar precedência de `Ctrl+N`, modal topmost, input/editável, aba ativa e
   hotkey global sem migrar handlers.
2. Só então converter candidatos em catálogo com IDs estáveis e contratos de
   origem/argumentos/efeito.
3. Medir duplicação e latência e registrar protótipos Stream Deck/foreground
   como pendências, sem declarar conclusão da Fase 0.

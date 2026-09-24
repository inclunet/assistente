# AEP-0109 — Controle de concorrência na configuração da tasklist

**Status:** Done — gravação verificada de workflow e custom actions pelos editores
da UI, com recarga e aviso em conflito (issue #831).

## Resumo

Os editores de workflow e de custom actions salvam a cada alteração (AEP-0036 e
AEP-0067). Cada gravação envia a configuração inteira. Sem controle de
concorrência, uma segunda aba ou o agente, pelas ferramentas de tasklist, podia
alterar a configuração enquanto o editor estava aberto, e a próxima gravação do
editor apagava essa alteração sem aviso.

Este AEP define a gravação verificada: o editor envia, junto com a configuração
nova, o estado sobre o qual a calculou. O backend só grava se o estado atual
ainda for equivalente a ele. Caso contrário, recusa com um erro de conflito, e o
editor recarrega a configuração e avisa o usuário.

## Motivação

- A perda era silenciosa: nem quem editava nem quem teve a alteração apagada
  recebia aviso.
- O agente edita workflow e custom actions por ferramenta enquanto o usuário
  pode estar com o editor aberto. Esse é o caso normal, não um caso raro.
- `TaskListWorkflow` e a coluna `custom_actions` não têm campo de versão. Criar
  um exigiria migração de schema e mudaria o contrato de todas as gravações.

## Decisões

### D1 — Compare-and-swap por conteúdo, dentro da transação

- `UpdateWorkflowFullCheckedWithContext` e
  `SetTaskListCustomActionsCheckedWithContext`, em `internal/database`, recebem o
  estado esperado. A comparação e a gravação acontecem na mesma transação.
- A transação é `IMMEDIATE` (`withSQLiteImmediateTransaction`). Com WAL e pool
  de várias conexões, uma transação `DEFERRED` deixava duas gravações lerem o
  mesmo snapshot, e a segunda falhava com `SQLITE_BUSY` ao promover a leitura a
  escrita, em vez de receber o conflito. Com o lock pego antes da leitura, a
  segunda espera a primeira e vê o conflito.
- O workflow é comparado de forma canônica. Não contam como alteração:
  - a ordem do array de statuses (ordenado por `order` e depois por `id`);
  - a ordem dos destinos de uma transição;
  - origens sem destinos.
- As custom actions são comparadas pelo resultado de
  `ParseTaskListCustomActionsJSON` serializado de novo. Formatação, ordem das
  chaves e string vazia contra `{"actions":[]}` não contam como alteração.
- Dentro da transação, o workflow é verificado antes da contagem de tarefas por
  status, para o conflito não virar um erro genérico de "status em uso". Em
  conflito, nada é gravado e nenhuma tarefa é migrada.
- A contagem que impede remover status com tarefas sem `status_migration` também
  é feita sob o lock. Contada antes, uma tarefa criada ou movida no intervalo para
  um status removido ficaria apontando para um status inexistente.

### D2 — Erro sentinela com código estável

- `database.ErrTaskListConfigConflict` tem mensagem iniciada por
  `TASKLIST_CONFIG_CONFLICT`. O Wails só entrega o texto do erro ao frontend, que
  o reconhece com `isTaskListConfigConflict` (`frontend/src/lib/taskListConfigConflict.ts`)
  exigindo o código no início da mensagem: o código citado no meio de outro erro
  não conta como conflito. Por isso nenhuma camada entre o banco e o editor pode
  acrescentar texto antes da mensagem.
- Em conflito, o serviço não emite `workflow:updated`, `taskList:updated` nem
  eventos de domínio.

### D3 — Métodos novos; os antigos continuam

- A API Wails ganhou `Tasklist.UpdateWorkflowFullChecked` e
  `TasklistActions.SetTaskListCustomActionsChecked`. Os bindings foram regerados
  com `wails generate module`.
- `UpdateWorkflowFull` e `SetTaskListCustomActions` continuam sem verificação.
  As ferramentas do agente os usam e sempre gravam a última palavra, que é o
  comportamento esperado de uma instrução explícita. Quem detecta o conflito é o
  editor da UI.
- No serviço, a gravação verificada exige um store que implemente
  `checkedConfigWriter`. Um store sem suporte falha, em vez de gravar sem
  verificação.

### D4 — Comportamento dos editores em conflito

- **Workflow:**
  - a base de cada gravação é o último workflow aceito pelo backend (`persistedRef`);
  - em conflito, as alterações que estavam na fila são descartadas (mesma regra de
    falha do AEP-0036), um toast de aviso é exibido e anunciado, e o
    `TaskListView` espera a fila, relê a lista e as contagens e incrementa
    `syncToken`;
  - com o novo `syncToken`, o editor passa a mostrar o gravado e a usá-lo como base;
  - se a releitura da lista ou das contagens falhar, o `syncToken` não muda: o
    editor fica como estava, aparece um erro, e a próxima gravação volta a dar
    conflito e tenta de novo. `getTaskCountsByStatus` repassa o erro em vez de
    devolver contagens vazias;
  - o formulário aberto mantém o rascunho para ser aplicado de novo.
- **Custom actions:**
  - a base é o JSON lido ou o último gravado;
  - em conflito, o editor avisa e relê as ações;
  - as ações que continuam existindo mantêm o id de UI, então o foco do grid e o
    formulário de edição seguem apontando para elas.
- Editar um status ou ação removido em outro lugar avisa, fecha o formulário e não
  grava.

## Fases

- [x] Fase 1 — Backend: gravação verificada, erro sentinela e testes
  (`internal/database/tasklist_config_conflict.go`,
  `tasklist_config_conflict_test.go`, `internal/tasklist/domain_events_test.go`).
- [x] Fase 2 — API Wails e bindings regerados (`internal/wailsapi/tasklist.go`,
  `tasklist_actions.go`, `frontend/wailsjs/go/wailsapi/*`).
- [x] Fase 3 — Frontend: store, editores, `TaskListView`, i18n nos 3 locales e
  testes Vitest.

## Riscos

- **Falso conflito por normalização divergente.** Mitigado: a comparação usa a
  mesma interpretação para os dois lados (parse das custom actions e canonicalização
  do workflow), e o editor envia como base exatamente o que leu ou gravou. Testes
  cobrem a reordenação de arrays, as origens sem destino e a formatação diferente.
- **Rascunho perdido.** O formulário aberto mantém o rascunho. Só o status ou a
  ação apagados em outro lugar fecham o formulário, com aviso.
- **Ferramentas do agente seguem sem verificação.** É intencional (D3): a instrução
  do agente é a mais recente, e o editor aberto é quem detecta e recarrega.

## Critérios de aceitação

- [x] Gravação do editor sobre um estado desatualizado é recusada sem alterar o
  banco nem migrar tarefas
  (`TestUpdateWorkflowFullChecked_ConflictDoesNotWriteNorMigrate`,
  `TestSetCustomActionsChecked_ConflictDoesNotWrite`).
- [x] Estado equivalente com outra ordem ou formatação não gera conflito
  (`TestUpdateWorkflowFullChecked_IgnoresOrderOfArraysAndEmptyTransitions`,
  `TestSetCustomActionsChecked_SavesWhenUnchangedAndIgnoresFormatting`).
- [x] Gravações verificadas simultâneas sobre a mesma base, em WAL com várias
  conexões: uma vence e as demais recebem conflito, nunca `SQLITE_BUSY`
  (`TestCheckedWritesUnderConcurrencyYieldConflictNotBusy`).
- [x] Remover um status que recebeu uma tarefa enquanto a gravação esperava o
  lock é recusado, e o status continua existindo
  (`TestUpdateWorkflowFull_RevalidatesTasksUnderLock`).
- [x] Conflito não emite eventos (`TestCheckedConfigWritesEmitOnlyWhenSaved`).
- [x] O editor de workflow avisa, pede a recarga e, sincronizado, grava sobre a
  versão atual sem colidir IDs (`WorkflowEditor.test.tsx`, bloco "edição
  concorrente"; `TaskListView.test.tsx`).
- [x] O editor de custom actions avisa, recarrega e mantém o rascunho e a edição
  em curso (`CustomActionsEditor.test.tsx`, bloco "edição concorrente").
- [x] A documentação de usuário explica o aviso (`docs/content/recursos/TASK_LISTS.md`).

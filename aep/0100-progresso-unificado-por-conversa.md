# AEP-0100 — Progresso unificado por conversa

**Status:** Done

## Resumo

Introduzir a tool builtin `update_plan` como capability única e pequena para o
modelo criar e atualizar o plano de execução da conversa. Cada chamada envia o
snapshot completo e ordenado, com itens estáveis em `pending`, `in_progress` ou
`completed`.

A capability não cria um segundo sistema de tarefas. O plano é persistido como
uma `TaskList`, e cada passo como uma `Task`; todas as mutações passam pelo
`tasklist.Service`, preservando banco, eventos Wails, eventos de domínio,
automação e UI existentes. As tools amplas `task_list`, `task` e `task_note`
continuam disponíveis sob demanda.

Esta AEP implementa a fase 3 da AEP-0096.

## Motivação

O domínio de tasklists já oferece CRUD completo, workflows, notas, vínculos e
automações, mas expõe três schemas extensos ao modelo. Para acompanhar uma
tarefa de programação, o modelo precisa descobrir essas tools, escolher entre
operações de lista e card, transportar UUIDs e conhecer status numéricos. Isso
torna uma ação operacional básica — comunicar plano e progresso — mais cara e
menos previsível do que nos principais agentes de código.

Criar estado efêmero apenas no loop resolveria o custo do schema, mas separaria
o progresso da UI e descartaria recursos já maduros. O contrato deve ser simples
para o modelo e nativo para o produto.

## Decisões

### D1 — Uma capability canônica chamada `update_plan`

`update_plan` recebe:

- `title`: título curto no idioma do usuário;
- `explanation`: justificativa opcional da atualização;
- `plan`: snapshot completo e ordenado de até 100 itens;
- por item: `id` estável, `step` e `status`.

Os status públicos são somente `pending`, `in_progress` e `completed`. Há no
máximo um item `in_progress`. Array vazio limpa os itens gerenciados, sem apagar
a lista.

O resultado é JSON estruturado com `plan_id`, snapshot e contagens por status.
Erros usam códigos estáveis e não são confundidos com falhas do transporte da
tool.

### D2 — Escopo implícito e exclusivo da conversa atual

A capability lê `ConversationID` do `invocationctx`. Sem conversa corrente,
falha antes de qualquer mutação. Isso evita exigir ao modelo a sequência
`get_conversation_info` → `task_list` apenas para manter progresso.

Esta é uma exceção intencional e estreita à decisão da AEP-0073: as tools gerais
`task` e `task_list` continuam exigindo `conversation_id` explícito. A exceção
só vale para `update_plan`, cuja própria identidade semântica é “plano desta
conversa”.

Cada conversa tem uma lista reservada, encontrada por slug determinístico
`assistente-plan-<hash>`. O hash não expõe o identificador da conversa e cabe no
limite atual de slug. A lista é vinculada à conversa pelo método existente
`SetTaskListConversation`.

### D3 — Task List Manager permanece fonte única

O mapeamento é:

- plano → `TaskList`;
- `title` → `TaskList.Title`;
- `explanation` → `TaskList.Description`;
- item → `Task`;
- `plan:<id>` → `Task.Code`;
- `step` → `Task.Title`;
- `pending` / `in_progress` / `completed` → status IDs 1 / 2 / 3 do workflow
  padrão.

Somente tasks com prefixo reservado `plan:` são reconciliadas. Cards manuais
eventualmente adicionados à mesma lista são preservados. Na reordenação de uma
coluna, os itens do plano vêm primeiro e os cards manuais mantêm sua ordem
relativa depois deles; a ordem completa só é persistida quando realmente muda.
Itens `plan:` são normalizados como cards raiz. Se uma edição manual os
rebaixar, o próximo snapshot os promove novamente. Antes de remover um item-pai
omitido, a reconciliação promove descendentes preservados e remove os demais do
nível mais profundo para a raiz, evitando exclusão em cascata acidental.

Não há escrita direta no repository. A tool chama os métodos existentes do
`tasklist.Service` pelo adapter da aplicação. Assim, criação, atualização,
mudança de status, remoção e reordenação emitem os mesmos eventos que qualquer
outra origem.

### D4 — Snapshot idempotente, sem promessa falsa de transação global

Todos os argumentos são validados antes da primeira mutação. Atualizações da
mesma conversa são serializadas dentro do processo para não intercalar duas
reconciliações.

Criação de `TaskList` e workflow ocorre numa transação única, inclusive com o
slug reservado já validado, para não deixar uma lista órfã se a segunda etapa
falhar.

O `tasklist.Service` atual transaciona cada mutação e emite eventos por entidade;
não oferece uma transação composta que retenha eventos até o commit. A
reconciliação multi-item, portanto, não declara atomicidade global. Se uma falha
de banco ocorrer no meio, a tool retorna `reconcile_failed` com o `plan_id`; o
próximo envio do mesmo snapshot é idempotente e converge o estado. Uma futura
transação de domínio pode fortalecer essa garantia sem alterar o schema público.

### D5 — Baseline de Programação

`update_plan` fica `preloaded` no profile builtin `Programação` e entra na
allowlist da skill `coding`. A skill orienta:

- usar plano apenas em trabalho com múltiplas etapas;
- enviar o snapshot completo em cada atualização;
- manter no máximo um item em andamento;
- concluir o item atual antes de avançar.

As tools gerais de tasklists continuam `on_demand`. Os demais profiles não
recebem a capability automaticamente. A skill `tasklist-manager` também
distingue o plano de execução (`update_plan`) de boards, workflows, notas e
sincronizações administrados pelas tools amplas.

## Fases

- [x] **Fase 1:** implementar `update_plan`, reconciliação e resultado
  estruturado — `internal/tools/tasklist/update_plan.go`.
- [x] **Fase 2:** registrar a tool e conectá-la ao `tasklist.Service` —
  `internal/app/app_tool_registry.go` (`serviceTaskListManager` +
  `NewUpdatePlan`).
- [x] **Fase 3:** pré-carregar no profile `Programação` e atualizar a skill
  `coding` — `internal/app/builtin/profiles/programacao.json` e
  `internal/app/builtin/skills/coding/SKILL.md`.
- [x] **Fase 4:** cobrir contrato, escopo por conversa, persistência,
  reconciliação e compatibilidade —
  `internal/tools/tasklist/update_plan_test.go`.
- [x] **Fase 5:** marcar a fase 3 da AEP-0096 como implementada.

### Evidências verificáveis

- Contrato, validação anterior à mutação, escopo por conversa, criação da lista
  vinculada, IDs estáveis, remoção, preservação de cards manuais, hierarquia,
  retry idempotente e workflow incompatível:
  `internal/tools/tasklist/update_plan.go` e `update_plan_test.go`.
- O adapter `serviceTaskListManager` em
  `internal/app/app_tool_registry.go` encaminha o lifecycle para
  `internal/tasklist/service.go`; os testes da tool usam a interface
  `PlanManager` para verificar as mutações e sua ordem, sem alegar um teste DB
  end-to-end inexistente.
- Registro e metadata também são protegidos por
  `internal/tools/catalog_equivalence_test.go`.
- Preload e skills:
  `internal/app/builtin_profiles_tools_test.go` verifica `update_plan` como
  `preloaded`; `internal/app/builtin_skills_test.go` verifica a allowlist e as
  instruções das skills `coding` e `tasklist-manager`.

## Riscos

- **Workflow editado manualmente:** a capability exige os IDs 1, 2 e 3. Se a
  lista reservada perder algum deles, falha explicitamente em vez de gravar um
  status incorreto.
- **Falha parcial:** cada mutação é transacional, mas o snapshot inteiro ainda
  não é. O retry idempotente repara o estado; a resposta não afirma atomicidade.
- **Colisão de slug:** o hash de 128 bits torna colisão acidental desprezível.
  Se o slug reservado estiver ligado a outra conversa, a tool falha fechada.
- **Remoção indevida:** apenas tasks com `Task.Code` prefixado por `plan:` e
  ausentes do snapshot são removidas.
- **Plano em tarefa trivial:** a skill proíbe o uso ornamental, reduzindo ruído
  no banco e no contexto.

## Critérios de aceitação

- [x] `update_plan` expõe um único schema pequeno de plano/progresso.
- [x] Ausência de conversa falha antes de mutar.
- [x] Cada conversa reutiliza uma única TaskList vinculada.
- [x] IDs estáveis atualizam a mesma Task; itens omitidos são removidos.
- [x] No máximo um item pode ficar `in_progress`.
- [x] Cards não gerenciados pela capability são preservados.
- [x] Mutações passam pelo `tasklist.Service` e preservam os eventos existentes.
- [x] `Programação` recebe `update_plan` preloaded; tools amplas continuam sob
  demanda.
- [x] Regressões Go cobrem o contrato da tool e os contratos de profile/skill
  nos arquivos listados em **Evidências verificáveis**.

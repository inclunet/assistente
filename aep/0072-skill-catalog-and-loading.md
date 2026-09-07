# AEP-0072 — Skill Loading Runtime

**Status**: Done
**Criado em**: 2026-06-08
**Revisado em**: 2026-06-19
**Implementada em**: 2026-06-18
**Implementada por**: PR #293
**Substitui**: versão anterior da AEP-0072 (Skill Catalog & Loading amplo)
**Pré-requisito atendido**: AEP-0075 (Context Providers)
**Relacionado**: AEP-0074 (Prompt Cache), AEP-0051 (Skills DB, opcional/futura), AEP-0063 (Tool Invocations e Executor Comum)

---

## Resumo

> **Nota de revisão (2026-06-19).** A versão original desta AEP tentou resolver ao mesmo tempo catálogo de skills, migração para banco, gating, templates, autoload e contexto dinâmico. Essa combinação se mostrou ampla demais. A separação foi concluída pela AEP-0075 e esta AEP agora fica restrita ao runtime de skill loading:
>
> - AEP-0075: `memory`, `workspace`, tasklists e estado dinâmico são **Context Providers**.
> - AEP-0072: skills ficam apenas como **módulos de instrução/workflow** e esta AEP passa a definir o runtime de carregamento.
> - AEP-0074: otimização de prompt cache vem depois, sobre a arquitetura separada.
>
> Portanto, esta AEP não deve reabrir configuração de memória/workspace/tasklists nem montagem de contexto dinâmico. Ela também não depende de migrar skills para banco antes de corrigir o runtime. AEP-0051 pode continuar como evolução de persistência, mas não é pré-requisito para o modelo de carregamento.

## Resumo revisado

Redesenhar o carregamento de skills para seguir progressive disclosure:

- catálogo leve e estável no prompt inicial;
- skills ordenadas por relevância no perfil, com a primeira skill carregável podendo atuar como base;
- skills sob demanda invocáveis por `/skill`, menção ou decisão do modelo quando tool calling estiver disponível;
- corpo completo da skill carregado pelo runtime de forma observável quando a skill é ativada;
- `memory` e `workspace` fora do sistema de skills;
- template engine fora do caminho novo de skills.

Skills passam a ser módulos de instrução. Contexto dinâmico pertence aos Context Providers da AEP-0075.

## Motivação revisada

O runtime atual de `/skill` carrega o corpo da skill de forma silenciosa dentro do system prompt do turno. Isso funciona tecnicamente, mas é ruim porque:

- o usuário não percebe claramente que a skill foi carregada;
- a ativação não fica auditável como evento de turno;
- a skill se mistura ao system prompt;
- templates e includes tornam o corpo da skill dinâmico e pouco cacheável;
- skills passaram a carregar estado (`memory`, `workspace`) que não pertence ao conceito de skill.

Ferramentas de mercado convergem para outro modelo:

- Cursor: skills invocadas por `/` ou `@`; o runtime torna a skill selecionada disponível no contexto.
- Claude Code: descrições de skills ficam visíveis; o corpo entra quando a skill é usada, não como parte fixa do prompt inicial.
- Codex: expõe lista compacta com nome/descrição/caminho e carrega conteúdo completo apenas quando a skill é selecionada.
- OpenClaw: usa catálogo compacto e carregamento progressivo do conteúdo completo.

O ponto comum não é obrigar o modelo a chamar uma ferramenta `read_file` para carregar uma skill. O padrão de mercado é **progressive disclosure**: catálogo leve primeiro; corpo completo só quando a skill é ativada; carregamento feito de forma explícita pelo runtime/ferramenta/plataforma e visível o suficiente para auditoria.

## Decisões revisadas

### D1. Skill é workflow/instrução, não estado

Skills não devem carregar memória, workspace, tasklists ou estado dinâmico do app. Esses dados pertencem a Context Providers.

Uma skill pode declarar:

- nome;
- descrição;
- quando usar;
- instruções;
- supporting files;
- tools úteis/permitidas;
- escopo de filesystem/network quando aplicável.

Uma skill não deve depender de Go templates para acessar contexto dinâmico.

### D2. Perfil define modo e ordem das skills

O perfil deve controlar como cada skill participa do runtime.

Modos:

- `base`: entra no prompt inicial como parte da persona/instrução do perfil.
- `on_demand`: aparece no catálogo e pode ser carregada quando invocada ou relevante.
- `disabled`: não aparece no catálogo e não pode ser invocada.

O perfil também preserva a ordem das skills. A ordem representa prioridade/relevância: skills mais relevantes ficam acima; menos relevantes ficam abaixo; desabilitadas ficam no fim ou fora do conjunto carregável. Essa ordem deve guiar o orçamento do catálogo quando houver omissão.

Exemplo conceitual:

```json
{
  "enabled_skills": [
    "tech-support",
    "github",
    "flock-api"
  ]
}
```

Semântica efetiva:

- `enabled_skills: null`/campo ausente → perfil legado; o runtime aplica fallback por `auto_load`;
- `enabled_skills: []` → seleção explícita vazia; todas as skills ficam `disabled`;
- `enabled_skills: ["tech-support", "github", "flock-api"]` → seleção explícita ordenada;
- primeira skill marcada (`tech-support`) → `base`;
- demais skills marcadas (`github`, `flock-api`) → `on_demand`;
- skills disponíveis mas não marcadas → `disabled`;
- a ordem da lista é a ordem de prioridade.

`memory` e `workspace` não aparecem aqui; eles são configurados como context providers na AEP-0075.

`disable_skills=true` significa perfil de prompt enxuto. Além de omitir a seção de skills, o runtime também deve omitir blocos dinâmicos criados para substituir templates de skills quando esses blocos contiverem instruções imperativas para o modelo, como `linked_task_lists`. Esse bloco também só deve ser injetado quando a skill `tasklist-manager` estiver efetivamente habilitada (`base` ou `on_demand`) no perfil ativo. Context Providers puramente informativos podem ter política própria, mas não devem reintroduzir instruções de workflow de skill por trás desse flag.

### D3. Primeira skill carregável pode atuar como base

O contrato revisado preserva a utilidade da ordenação já existente no editor de perfil. A primeira skill marcada na lista atua como skill base do perfil. As demais skills marcadas ficam disponíveis sob demanda, seguindo a mesma ordem de prioridade.

Isso mantém compatibilidade com o schema atual (`enabled_skills`) e evita uma migração estrutural desnecessária. A diferença para o runtime antigo é semântica: `enabled_skills` deixa de significar "todas entram no prompt" e passa a significar "lista ordenada de skills habilitadas", onde a primeira é `base`, as demais são `on_demand`, e as não marcadas são `disabled`.

### D4. Catálogo inicial é leve, ordenado e cacheável

O prompt inicial inclui apenas catálogo de skills `on_demand`:

- slug;
- display name;
- descrição;
- quando usar;
- caminho ou identificador para carregamento.

O corpo completo não entra no prompt inicial.

O catálogo deve respeitar a ordem definida pelo perfil. Essa ordem é a principal regra de prioridade: skills mais acima entram primeiro; skills mais abaixo são candidatas a omissão se houver orçamento. A aplicação de orçamento é desejável, mas o algoritmo exato de corte/encurtamento será definido na implementação. O requisito mínimo é manter a ordenação e não promover skills menos relevantes acima das mais relevantes.

Refinamento implementado após a issue #331:

- a skill `base` e o catálogo de skills `on_demand` são emitidos por um Context Provider `skills`;
- o prompt base hardcoded não participa do caminho novo; a identidade/instrução inicial deve vir da `base_skill` resolvida pelo perfil/provider;
- o protocolo catalog-first de tools é emitido por Context Provider separado (`tool_protocol`), sem misturar catálogo de skills com protocolo de ferramentas.

### D5. `/skill` é uma ativação explícita e observável

Quando o usuário digita `/skill args`, o backend deve:

1. resolver a skill;
2. validar se ela está habilitada no perfil;
3. registrar evento de skill carregada no turno;
4. preservar argumentos como bloco separado;
5. anexar o corpo da skill como contexto do turno, não como mutação silenciosa do system prompt estável;
6. expor erro claro se a skill não existir ou estiver desabilitada.

O usuário deve conseguir perceber que a skill foi carregada.

As permissões declaradas pela skill (`tools`, `bashCommands`, `filesystem` e `network`) devem ser preservadas no contexto de execução do turno. Para rede, o enforcement ocorre no cliente HTTP compartilhado e em redirects, de modo que skills carregadas por `/skill` ou por `load_skill` tenham o mesmo bloqueio de hosts `allowed`/`denied`.

### D6. Carregamento sob demanda é explícito no runtime

Quando uma skill listada no catálogo é ativada, o runtime deve carregar o corpo completo de forma explícita e observável no turno.

Primeira implementação aceitável:

- resolver a skill a partir do catálogo/perfil;
- carregar o `SKILL.md` completo pelo runtime usando a fonte atual de skills no filesystem;
- registrar a ativação e o conteúdo carregado como contexto do turno;
- manter supporting files lidos sob demanda.

Implementação futura:

- se AEP-0051 for retomada, o DB pode virar fonte canônica. Isso não exige, por si só, uma tool nova `load_skill`; a decisão deve ser tomada apenas se houver benefício claro.

### D6.1. Catálogos e autoativação pelo modelo dependem de tool calling

`tool_catalog` e qualquer futura tool/ação de autoativação de skill pelo modelo, como `load_skill`, são capacidades mediadas por tool calling. Se `toolCallingEnabled=false`, o runtime deve desativar essas portas de entrada para o modelo.

Regra efetiva:

- `tool_catalog` não deve ser exposta ao modelo;
- `load_skill` ou mecanismo equivalente de autoativação de skill não deve ser exposto ao modelo;
- skills `on_demand` não podem ser autoativadas pelo modelo;
- `/skill` explícito do usuário continua permitido, porque o backend carrega a skill antes da chamada ao modelo;
- a skill `base` continua funcionando, desde que seja compatível com o modo sem tool calling;
- skills que dependem de tools/filesystem/network/MCP devem ser omitidas ou degradadas quando tool calling estiver indisponível.

Resumo do contrato: carregamento sob demanda dirigido pelo modelo exige tool calling; carregamento sob demanda dirigido pelo usuário via `/skill` não exige.

### D7. Auto-load vira `base`

`auto_load` como booleano genérico deve ser migrado para o modo `base`. Não há remoção funcional de comportamento: o que antes entrava automaticamente passa a ser representado explicitamente como skill base.

Equivalências:

- `auto_load=true` que define persona do perfil → `base`;
- `auto_load=true` que era workflow essencial do perfil → `base`;
- workflows ocasionais devem ser revisados caso a caso e podem virar `on_demand`, mas isso é decisão de migração, não remoção automática.

### D8. Template engine é removido de skills

O caminho novo de skills não deve processar Go templates. Skills devem ser instruções/workflows estáticos ou parametrizados por argumentos estruturados do turno.

Não é depreciação: é remoção do template engine do runtime de skills. O conteúdo de `SKILL.md` é markdown literal; sequências como `{{ ... }}` podem aparecer em exemplos, mas não são interpretadas pelo runtime.

Política:

- `$ARGUMENTS` deve ser substituído por bloco estruturado de argumentos;
- `{{ now }}`, includes e variáveis dinâmicas deixam de ser executados no caminho novo;
- built-in skills não devem depender de Go templates para produzir contexto dinâmico;
- skills com exemplos de template são carregadas como markdown literal;
- esta AEP não exige criar novos Context Providers para substituir cada uso de template. Se algum contexto dinâmico for necessário no futuro, ele deve ser planejado em AEP própria ou extensão específica, não como requisito desta implementação.

## Fases revisadas

### Fase 0 — AEP-0075 primeiro

Implementar Context Providers para remover `memory` e `workspace` do pool de responsabilidades de skills.

Status: concluída. `memory`, `workspace` e `tasklist` já são Context Providers no caminho novo; `memory` tem records estruturados, APIs Wails e tela de governança; `workspace` produz contexto mínimo via provider; tasklists vinculadas são bloco dinâmico do provider `tasklist`; e a configuração por perfil (`context_providers`) resolve `enabled`, `budget` e `settings` antes da montagem dos blocos.

Consequência para esta AEP: nenhuma fase posterior deve recolocar memória, workspace, tasklists, budgets de providers ou settings de providers no runtime de skills. O escopo restante é exclusivamente a política e a experiência de carregamento de skills como módulos de workflow/instrução.

### Fase 1 — Política ordenada por perfil ✅

- [x] Reinterpretar `enabled_skills` como lista ordenada de skills habilitadas.
- [x] Garantir que a primeira skill marcada seja `base`.
- [x] Garantir que as demais skills marcadas sejam `on_demand`.
- [x] Garantir que skills não marcadas sejam `disabled`.
- [x] Manter a ordenação como regra de prioridade do catálogo.

### Fase 2 — Catálogo leve ✅

- [x] Construir catálogo só com skills `on_demand`.
- [x] Preservar a ordenação do perfil como ordem de prioridade.
- [x] Aplicar orçamento sem inverter prioridade.

### Fase 3 — Slash explícito e observável ✅

- [x] Reimplementar `/skill` como ativação de turno.
- [x] Emitir evento/segmento de carregamento.
- [x] Mostrar erro quando desabilitada.

### Fase 4 — Carregamento sob demanda ✅

- [x] Permitir ativação explícita de skill do catálogo e carregamento pelo runtime.
- [x] Registrar skill carregada como tool/context event.
- [x] Garantir que `tool_catalog` e autoativação de skills pelo modelo sejam desativadas quando tool calling estiver indisponível.

### Fase 5 — Remover templates de skills e migrar built-ins ✅

- [x] Migrar built-in skills para o novo formato sem execução de Go templates pelo runtime de skills.
- [x] Bloquear uso do template engine no caminho novo de skills.
- [x] Migrar `auto_load` para `base`.
- [x] Remover dependência do template engine no caminho novo.

### Evidências da implementação

- Política e invocação: `internal/skills/policy.go`,
  `internal/skills/policy_test.go` e `internal/skills/invocation_test.go`.
- Runtime e catálogo sob demanda: commits `5a26eb14` e `4c9516a6`, com regressões
  em `internal/skills/skills_test.go`.
- Integração do prompt e remoção de templates: testes de `internal/prompt` e
  built-ins atuais sem execução de Go templates.

## Registro do plano implementado

> **Nota histórica (2026-06-17).** O plano abaixo foi implementado pelos commits
> `5a26eb14` (runtime da AEP-0072) e `4c9516a6` (carregamento sob demanda pelo
> modelo), preservando `enabled_skills`: primeira = `base`, demais =
> `on_demand`, não marcadas = `disabled`.

### Escopo entregue

A entrega implementou, em conjunto:

1. política ordenada baseada em `enabled_skills`;
2. compatibilidade com perfis existentes sem migração estrutural de schema;
3. runtime central de seleção de skills por modo;
4. catálogo leve para skills `on_demand`;
5. `/skill` com validação por perfil e ativação observável;
6. carregamento sob demanda explícito pelo runtime;
7. UI de perfis e slash menu alinhados aos modos;
8. migração de `auto_load` para `base` e remoção de templates das built-in skills.

AEP-0051 continua fora do caminho crítico. Nesta implementação, `SKILL.md` no filesystem permanece a fonte inicial de leitura; um banco de skills ou tool dedicada `load_skill` ficam como evolução futura.

### Contrato de perfil

O novo contrato canônico é:

```json
{
  "enabled_skills": [
    "coding",
    "job-manager",
    "editor-texto"
  ]
}
```

Semântica:

- primeira skill marcada: `base`, entra no prompt inicial como instrução base do perfil;
- demais skills marcadas: `on_demand`, aparecem no catálogo leve e podem ser carregadas por `/skill`, menção ou decisão do modelo quando tool calling estiver disponível;
- skills não marcadas: `disabled`, não aparecem no catálogo e não podem ser invocadas;
- a ordem da lista é relevante e representa prioridade.

`enabled_skills` continua sendo o contrato de armazenamento, preservando compatibilidade com perfis existentes e com o editor atual. A mudança é na política efetiva derivada dessa lista:

- `enabled_skills[0]` → `base`;
- `enabled_skills[1:]` → `on_demand`;
- skills disponíveis ausentes de `enabled_skills` → `disabled`;
- perfis com `disable_skills=true` devem produzir uma política efetiva sem skills carregáveis;
- perfis com `disable_on_demand_skills=true` devem produzir política efetiva com apenas `enabled_skills[0]` como `base` e o restante como `disabled`.
- se `toolCallingEnabled=false`, o catálogo para o modelo deve omitir `tool_catalog` e qualquer autoativação de skills; permanecem apenas a skill `base` e `/skill` explícito do usuário.

Depois da migração semântica, os perfis builtin podem continuar declarando `enabled_skills`, mas devem ordenar a lista por relevância.

### Runtime de seleção

Deve existir uma política efetiva de seleção, por exemplo `SkillSelectionPolicy`, responsável por resolver:

- skills `base` do perfil;
- skills `on_demand` elegíveis para catálogo;
- skills `disabled`;
- compatibilidade temporária com `auto_load`;
- bloqueios globais de skill do perfil, se ainda necessários durante migração.

O `prompt.Builder` não deve interpretar `enabledSkills` diretamente como "autoload de todas". Ele deve receber a política efetiva ou uma estrutura equivalente que deixe claro qual é a skill `base` e quais são `on_demand`.

### Catálogo leve

O prompt inicial deve conter apenas catálogo leve de skills `on_demand`, com:

- slug;
- display name;
- descrição ou quando usar;
- caminho ou identificador de carregamento do `SKILL.md`.

O corpo completo da skill não entra no catálogo. O catálogo deve preservar a ordenação do perfil e pode aplicar orçamento. Quando houver orçamento e ele estourar:

- descrições podem ser encurtadas;
- skills menos prioritárias, mais abaixo na lista, podem ser omitidas;
- o prompt deve conter aviso observável de omissão/encurtamento.

O algoritmo exato de orçamento não fica fechado nesta AEP. O requisito é que qualquer corte respeite a ordem: relevantes acima, menos relevantes abaixo, desabilitadas no fim ou fora do catálogo.

### `/skill` observável

Quando o usuário enviar `/skill args`, o backend deve:

1. resolver a skill;
2. validar a política efetiva do perfil;
3. rejeitar skill `disabled` com erro claro;
4. registrar evento de ativação de skill no turno;
5. preservar os argumentos em bloco separado;
6. anexar o corpo da skill como contexto do turno carregado explicitamente pelo runtime;
7. manter as permissões de filesystem/network da skill para o turno.

O carregamento não deve ser uma mutação silenciosa do system prompt estável. O usuário deve conseguir perceber que a skill foi carregada.

### Carregamento sob demanda pelo modelo

Na primeira implementação, a skill listada no catálogo é carregada pelo runtime a partir do `SKILL.md` existente quando ativada. Supporting files continuam sob demanda.

Regras:

- skill `disabled` não aparece no catálogo;
- skill `disabled` não pode ser carregada por `/skill`;
- skill `on_demand` pode ser carregada quando ativada;
- skill `base` entra como instrução base e a ordenação do perfil define sua prioridade relativa.
- ativação dirigida pelo modelo só é permitida quando tool calling estiver disponível;
- `tool_catalog` e qualquer tool de controle como `load_skill` devem ser removidas do conjunto exposto ao modelo quando tool calling estiver indisponível.

### Frontend

A UI de perfis deve manter a seleção ordenada já existente, mas expor a semântica efetiva:

- marcada e primeira na ordem: `base`;
- marcada e não primeira: `on_demand`;
- não marcada: `disabled`.

A UI atual já permite marcar e ordenar skills. O trabalho principal é deixar a semântica clara para o usuário, por exemplo mostrando que a primeira marcada é a base e que as demais marcadas são sob demanda. O slash menu deve listar somente skills invocáveis segundo a política efetiva do perfil ativo.

### Remoção de templates e compatibilidade de `auto_load`

Templates em skills não continuam como compatibilidade do caminho novo. O runtime novo não deve executar Go templates em skills. Built-in skills que ainda usam templates devem ser migradas para instruções estáticas ou argumentos estruturados antes de entrar no catálogo/base.

Equivalências temporárias:

- `auto_load=true` que representava persona vira `base` durante migração;
- `auto_load=true` que era workflow essencial também vira `base` inicialmente;
- workflows ocasionais podem ser revisados para entrar depois da primeira posição como `on_demand`;
- esta AEP não planeja criar novos Context Providers para substituir templates.
- skills com templates não são carregáveis no caminho novo até migração explícita.

### Testes mínimos do PR

O PR deve cobrir:

- compatibilidade de perfil existente com `enabled_skills`;
- perfis builtin com `enabled_skills` ordenados por prioridade;
- política efetiva com `base`, `on_demand` e `disabled`;
- catálogo leve sem corpo completo de skill;
- orçamento de catálogo com encurtamento/omissão;
- `/skill` permitido para skill habilitada;
- `/skill` rejeitado para skill `disabled`;
- preservação de argumentos e permissões de filesystem no turno;
- slash menu mostrando apenas skills invocáveis;
- `tool_catalog` e autoativação pelo modelo indisponíveis quando `toolCallingEnabled=false`;
- `/skill` explícito do usuário funcionando mesmo quando `toolCallingEnabled=false`;
- UI de perfil mantendo/ordenando `enabled_skills` e explicando a semântica efetiva;
- migração de `auto_load` e built-in skills sem templates no caminho novo.

## Critérios de aceitação revisados

- [x] Context providers substituem pseudo-skills dinâmicas, conforme AEP-0075.
- [x] Perfil ordena `enabled_skills`; a primeira é `base`.
- [x] Skills `on_demand` aparecem em catálogo leve.
- [x] `/skill` produz ativação observável no turno.
- [x] Skill desabilitada não aparece nem carrega.
- [x] Corpo completo não entra silenciosamente no prompt estável.
- [x] Go templates não existem no runtime novo.
- [x] Perfis builtin/novos usam ordem determinística.
- [x] Perfis antigos permanecem compatíveis.
- [x] Catálogo respeita budget e sinaliza cortes.
- [x] UI explica base/on-demand/disabled.
- [x] Gating sem tool calling preserva `/skill` explícito e skill base.

Regressões: `internal/skills/policy_test.go`, `invocation_test.go`,
`skills_test.go`, `internal/prompt/builder_test.go`,
`internal/app/builtin_skills_test.go` e testes da UI de perfil.

---

## Histórico da versão anterior

O conteúdo abaixo representa a versão original da AEP-0072. Ele permanece temporariamente para rastreabilidade, mas foi supersedido pelas decisões revisadas acima.

Redesenhar a descoberta, o gating e o carregamento de skills para seguir o padrão de **progressive disclosure** consolidado nas ferramentas líderes (Claude Code, Codex, Cursor, Copilot), usando como fundação as skills persistidas em banco (AEP-0051).

Hoje o assistente injeta o conteúdo completo das skills `auto_load` no system prompt e expõe as demais como referências para o modelo ler via `read_file`. Falta um **catálogo compacto com orçamento de contexto (budget)**, faltam **metadados de gating** (quando uma skill é aplicável) e o `auto_load` é usado em excesso, inflando o prompt. Esta AEP define um `skill_catalog` (espelhando o `tool_catalog` da AEP-0049), uma `SkillSelectionPolicy` por perfil/contexto, e um fluxo de carregamento em três níveis com cap de budget — mantendo a ativação pela ferramenta de leitura já existente, sem inventar uma tool nova de carregamento.

Escopo: **apenas skills**. Profiles (AEP-0050) está fora (adiado). As skills de filesystem entram no banco como **importação legada não-destrutiva** (AEP-0051, D9), no mesmo fluxo de MCP e Jobs.

---

## Motivação

### Problemas observados (issue #126)

- Skills `auto_load` incham o system prompt (instruções longas e dependentes de tools).
- Skills sob demanda não são descobertas/declaradas de forma útil; o modelo não sabe quando/como carregá-las.
- A separação entre autoload e sob demanda não fica clara no comportamento do modelo.
- Skills com templates/tools podem vazar conteúdo errado quando não são aplicáveis.
- Não há orçamento de contexto para o bloco de skills; muitas skills competem com a conversa.
- O design não acompanha o padrão "catálogo leve primeiro, conteúdo completo só quando necessário".

### Benchmark: como as ferramentas líderes fazem

O padrão convergente é **progressive disclosure em três níveis**:

1. **Descoberta** — no startup, só `name` + `description` (+ path) de cada skill entram no system prompt (~100 tokens/skill). É um catálogo compacto.
2. **Ativação** — quando a tarefa casa com a descrição, o agente lê o corpo completo do `SKILL.md` para o contexto, usando a ferramenta genérica de leitura (não uma tool dedicada de "carregar skill").
3. **Execução** — arquivos de referência/scripts são lidos sob demanda.

Pontos específicos:

- **Claude Code / Anthropic**: roteamento decidido pelo LLM a partir das descrições ("elimina roteamento algorítmico"); ativação por leitura do arquivo. Descrição em 3ª pessoa, com gatilhos específicos ("This skill should be used when...").
- **Codex (OpenAI)**: idêntico, e **impõe budget** — a lista inicial de skills é limitada a ~2% da janela de contexto (ou ~8.000 caracteres quando desconhecida); encurta descrições primeiro, omite skills excedentes e **emite warning**. Invocação explícita (`$skill`, `/skills`) + implícita (por descrição), com `allow_implicit_invocation: false` por skill.
- **Cursor**: rules `.mdc` com frontmatter — `alwaysApply: true` (sempre) vs. escopo por glob (carrega só quando arquivos casam).
- **Copilot**: `copilot-instructions.md` único, sempre ativo, sem roteamento por skill.

Conclusão: o `<available_skills>` + `read_file` atual já é, no fundo, o padrão Claude/Codex. O trabalho desta AEP é **fechar os gaps** (catálogo compacto persistido, budget, descrições/gatilhos, gating), não trocar o mecanismo de ativação.

---

## Estado atual

- `internal/skills/manager.go`: `discoverAll()` varre o filesystem a cada chamada; `GetAutoSkills()` / `GetAvailableSkills()` / `GetAllSkillsFull()` carregam e parseiam tudo do disco.
- `internal/prompt/builder.go` (`BuildSkillsSection`): emite `<auto_skills>` (conteúdo completo, níveis 1+2 juntos) e `<available_skills>` (referências + path para o modelo ler via `read_file`). Já há gating parcial: quando o tool-calling está desabilitado, `filterSkillsWithoutToolDependencies` remove qualquer skill que dependa de tools, filesystem, network **ou** MCP (ver `skillDependsOnTools`), e o bloco `<available_skills>` é omitido.
- `internal/skills/types.go`: `SkillMetadata` é rico (filesystem, network, tools, dependencies, mcp, triggers de hooks, `DisableModelInvocation`, `UserInvocable`), mas **não** tem `context_budget`, `requires_*` semânticos para gating, nem gatilhos de descoberta (o `Triggers` atual é para hooks PreToolUse/PostToolUse).

---

## Decisões

### D1. Catálogo paralelo: `skill_catalog` espelha o `tool_catalog`

Skills e tools são conceitos distintos (skills = instruções lidas para o contexto; tools = function-calling com JSON schema, executadas), portanto **não** vão para a mesma tabela. O `skill_catalog` é uma entidade própria que **reutiliza o padrão** do `tool_catalog` (AEP-0049) e da futura plataforma de tools (#119–#122): repositório/serviço próprio, política de seleção e planner com budget. Ganho = reuso de desenho, não de schema.

### D2. Progressive disclosure fiel ao padrão

- **Nível 1 (descoberta)**: catálogo compacto no system prompt — `name`, `description` (com gatilhos), path e custo estimado. Sem corpo.
- **Nível 2 (ativação)**: o modelo lê o corpo da skill pela ferramenta de leitura já existente (`read_file`). **Sem tool nova** de carregamento.
- **Nível 3 (execução)**: arquivos de referência lidos sob demanda (já suportado por `GetSkillFiles`).
- Invocação explícita do usuário continua via `/slash` (já existente).

**Fonte canônica vs. path lido (resolve a divergência com AEP-0051 D8).** O banco é a fonte canônica do conteúdo da skill. Para manter a ativação por leitura genérica sem ler o arquivo de importação (que pode ficar defasado após edição via UI), o conteúdo é **materializado pelo backend num cache read-only em disco**, derivado do DB e regenerado quando a skill muda. O `path` exposto no catálogo (Nível 1) aponta para esse cache materializado — **não** para o `SKILL.md` original importado. Assim: DB canônico, `read_file` lê do cache consistente, e o arquivo de importação (AEP-0051 D9) permanece apenas como origem de importação, sem ser fonte de runtime. (Os arquivos complementares de Nível 3 que ainda vivem em disco — AEP-0051 D4 — seguem sendo lidos diretamente via `GetSkillFiles`.)

### D3. Orçamento de contexto (budget) no catálogo, estilo Codex

O bloco de descoberta (Nível 1) tem um **cap configurável** (ex.: percentual da janela do modelo, com fallback em caracteres). Ao exceder: primeiro encurtam-se descrições, depois omitem-se skills de menor prioridade, e a omissão é **sinalizada** (log/telemetria). O budget aplica-se só ao Nível 1; ao ativar uma skill, o corpo completo é lido normalmente.

### D4. Metadados de catálogo/gating no frontmatter e no DB

Estender `SkillMetadata` (e o schema da AEP-0051) com campos voltados a descoberta e gating:

- `context_budget`: custo aproximado (em tokens/bytes) do corpo da skill, para o planner do Nível 1.
- `requires_tools` / `requires_filesystem` / `requires_network` / `requires_mcp`: pré-condições de capability. Skill incompatível com o contexto/perfil atual é **omitida ou degradada**, não injetada.
- `autoload_reason`: justificativa textual obrigatória quando `auto_load=true` (desencoraja autoload sem motivo — ver D5).
- Gatilhos de descrição: orientação para descrições em 3ª pessoa com frases-gatilho (padrão Claude/Codex), validadas no import (lições de #165/#159).

### D5. `auto_load` é exceção, não regra

O default é **sob demanda**. `auto_load=true` só para skills que realmente precisam estar sempre no prompt, e passa a exigir `autoload_reason`. O objetivo é manter o prompt inicial enxuto e empurrar conteúdo para o Nível 2.

### D6. Skills de filesystem = importação legada (delegada à AEP-0051)

A origem dos dados (filesystem → DB) é a **importação legada não-destrutiva e idempotente** definida na AEP-0051 (D9), no mesmo fluxo pós-login de MCP e Jobs (`runPostLoginLegacyImports` + `ImportLegacyResourcesWithContext`). Esta AEP **não** redefine esse mecanismo; apenas consome as skills já no banco. O runtime serve skills do DB; os `SKILL.md` originais não são renomeados/apagados.

---

## Fases

### Fase 1 — Fundação de dados + importação legada
Implementar a AEP-0051 (tabelas `skills`, `skill_tools`; preserva `Parse()`/`Compose()`) e o importador legado de skills (AEP-0051, D9) plugado em `runPostLoginLegacyImports`, espelhando MCP/Jobs. Depende da AEP-0046 (já mergeada).

### Fase 2 — Metadados de catálogo/gating (D4)
Estender `SkillMetadata` e o schema com `context_budget`, `requires_*`, `autoload_reason` e gatilhos de descrição; validação de descrição no import (3ª pessoa, frases-gatilho, limites). Backward-compat: campos ausentes assumem default sob demanda.

### Fase 3 — `skill_catalog` + `SkillSelectionPolicy` (D1)
Catálogo compacto persistido e política única de seleção/gating por perfil/tools/contexto, espelhando `ToolCatalogRepository` / `ToolSelectionPolicy` (#119/#120). Fonte única de verdade para "esta skill é aplicável/visível agora?".

### Fase 4 — Progressive disclosure + budget (D2/D3)
Refatorar `BuildSkillsSection` (`internal/prompt/builder.go`): Nível 1 compacto com cap de budget; minimizar `<auto_skills>` (só com `autoload_reason`); formalizar a ativação por leitura; aplicar gating de D4 para omitir/degradar skills incompatíveis.

### Fase 5 — Importação observável + testes
Evoluir `runPostLoginLegacyImports` para resultado estruturado/telemetria (#123), cobrindo skills. Testes de regressão: autoload vs. sob demanda; tools on/off; templates inválidos; seleção por perfil; budget (encurtamento/omissão/warning); importação idempotente e não-destrutiva.

> Fora de escopo (dependências externas): vínculo perfil↔conversa (#118) e Profiles DB (AEP-0050), ambos adiados.

---

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|-------|---------------|---------|-----------|
| R1 | Importação filesystem→DB com duplicação/efeito colateral | Baixa | Alto | Delegada à AEP-0051 D9 (idempotente, não-destrutiva, dedup por slug com prioridade workdir>home>exe) |
| R2 | Regressão no system prompt (skills sumindo ou inflando) | Média | Médio | Testes de snapshot de `BuildSkillsSection`; rollout com `auto_load` preservado para skills que já eram autoload |
| R3 | Budget cortar skills relevantes | Média | Médio | Ranking por perfil/relevância; warning observável; cap configurável |
| R4 | Divergência entre os campos de catálogo (D4) e o schema da AEP-0051 | Média | Médio | D4 estende o schema da 0051 explicitamente (Fase 2); migração aditiva |
| R5 | Runtime ainda lendo do filesystem após o corte para DB | Baixa | Médio | AEP-0051 D8: runtime serve do DB; `discoverAll` só como Source de import |
| R6 | Descrições fracas prejudicam o roteamento pelo LLM | Média | Médio | Validação de descrição no import (3ª pessoa, gatilhos), lições de #165/#159 |

---

## Critérios de aceitação históricos (versão supersedida)

Os seis critérios da issue #126:

1. O prompt inicial não injeta conteúdo completo de skills sob demanda.
2. Skills sob demanda aparecem em uma declaração compacta e útil para o modelo.
3. Existe caminho explícito para carregar/executar uma skill sob demanda quando relevante.
4. Skills dependentes de tools/filesystem/network/MCP são omitidas ou degradadas corretamente quando essas capacidades estão desabilitadas.
5. Autoload continua funcionando para skills que realmente precisam ser carregadas automaticamente.
6. Há testes cobrindo autoload, sob demanda, tools desabilitadas, templates inválidos e seleção por perfil.

Mais os específicos desta AEP:

7. **Budget aplicado**: o bloco de descoberta respeita um cap configurável, encurtando descrições e omitindo skills com sinalização observável.
8. **Gating testável sem LLM**: a `SkillSelectionPolicy` decide visibilidade/aplicabilidade de forma determinística e coberta por testes.
9. **Importação não-destrutiva e idempotente** (herdada da AEP-0051): re-executar não duplica nem altera os `SKILL.md` originais.

---

## Relações

- **Âncora**: issue #126 (redesenho de carregamento e execução de skills).
- **Fundação**: AEP-0051 (Skills DB) — esta AEP consome as skills no banco; AEP-0046 (UUIDv7).
- **Reuso de padrão**: AEP-0049 (catalog-first) e bloco de plataforma de tools #119 (`ToolSelectionPolicy`), #120 (`ToolCatalogRepository`), #121 (`ToolPlanner`/budget), #122 (metadados declarados).
- **Importação**: #123 (serviço de importação observável), AEP-0047 (import/export).
- **Fora de escopo (adiado)**: #118 (perfil↔conversa) e AEP-0050 (Profiles DB) — rastreado por issue própria.
- **Lições (issues fechadas, não reabrir)**: #165, #159 (contrato de template/schema), #154/#155, #173/#176/#171 (job-manager skill).

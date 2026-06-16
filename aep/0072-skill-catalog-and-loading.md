# AEP-0072 — Skill Loading Runtime

**Status**: Proposta revisada
**Criado em**: 2026-06-08
**Revisado em**: 2026-06-16
**Substitui**: versão anterior da AEP-0072 (Skill Catalog & Loading amplo)
**Depende de**: AEP-0075 (Context Providers)
**Relacionado**: AEP-0074 (Prompt Cache), AEP-0051 (Skills DB, opcional/futura), AEP-0063 (Tool Invocations e Executor Comum)

---

## Resumo

> **Nota de revisão (2026-06-16).** A versão original desta AEP tentou resolver ao mesmo tempo catálogo de skills, migração para banco, gating, templates, autoload e contexto dinâmico. Essa combinação se mostrou ampla demais. A nova direção separa responsabilidades:
>
> - AEP-0075: `memory`, `workspace`, tasklists e estado dinâmico viram **Context Providers**.
> - AEP-0072: skills ficam apenas como **módulos de instrução/workflow** e esta AEP passa a definir o runtime de carregamento.
> - AEP-0074: otimização de prompt cache vem depois, sobre a arquitetura separada.
>
> Portanto, esta AEP não depende mais de migrar skills para banco antes de corrigir o runtime. AEP-0051 pode continuar como evolução de persistência, mas não é pré-requisito para o modelo de carregamento.

## Resumo revisado

Redesenhar o carregamento de skills para seguir progressive disclosure:

- catálogo leve e estável no prompt inicial;
- skill de perfil/base explicitamente marcada;
- skills sob demanda invocáveis por `/skill`, menção ou decisão do modelo;
- corpo completo da skill carregado de forma observável;
- `memory` e `workspace` fora do sistema de skills;
- Go templates fora do caminho novo de skills.

Skills passam a ser módulos de instrução. Contexto dinâmico pertence aos Context Providers da AEP-0075.

## Motivação revisada

O runtime atual de `/skill` carrega o corpo da skill de forma silenciosa dentro do system prompt do turno. Isso funciona tecnicamente, mas é ruim porque:

- o usuário não percebe claramente que a skill foi carregada;
- a ativação não fica auditável como evento de turno;
- a skill se mistura ao system prompt;
- templates e includes tornam o corpo da skill dinâmico e pouco cacheável;
- skills passaram a carregar estado (`memory`, `workspace`) que não pertence ao conceito de skill.

Ferramentas de mercado convergem para outro modelo:

- Cursor: skills invocadas por `/` ou `@`; rules são escopadas separadamente.
- Claude Code: skill descriptions ficam visíveis; corpo da skill entra quando usada, no ponto da conversa.
- Codex: lista inicial com nome, descrição e path; lê `SKILL.md` completo apenas quando seleciona.
- OpenClaw: injeta lista compacta e instrui o modelo a usar `read` para carregar `SKILL.md`.

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

### D2. Perfil define o modo da skill

O perfil deve controlar como cada skill participa do runtime.

Modos:

- `base`: entra no prompt inicial como parte da persona/instrução do perfil.
- `on_demand`: aparece no catálogo e pode ser carregada quando invocada ou relevante.
- `disabled`: não aparece no catálogo e não pode ser invocada.

Exemplo conceitual:

```json
{
  "skill_modes": {
    "tech-support": "base",
    "github": "on_demand",
    "flock-api": "on_demand",
    "job-manager": "disabled"
  }
}
```

`memory` e `workspace` não aparecem aqui; eles são configurados como context providers na AEP-0075.

### D3. Skill base substitui o uso implícito de "primeira skill"

Não deve existir contrato por posição, como "a primeira skill do perfil é a instrução base". Isso é frágil e difícil de auditar.

Se uma skill define a persona do perfil, ela deve ser marcada explicitamente como `base`.

### D4. Catálogo inicial é leve e cacheável

O prompt inicial inclui apenas catálogo de skills `on_demand`:

- slug;
- display name;
- descrição;
- quando usar;
- caminho ou identificador para carregamento.

O corpo completo não entra no prompt inicial.

O catálogo deve ser ordenado deterministicamente e ter orçamento de tamanho. Se passar do orçamento, descrições podem ser encurtadas e skills menos prioritárias omitidas com aviso observável.

### D5. `/skill` é uma ativação explícita e observável

Quando o usuário digita `/skill args`, o backend deve:

1. resolver a skill;
2. validar se ela está habilitada no perfil;
3. registrar evento de skill carregada no turno;
4. preservar argumentos como bloco separado;
5. anexar o corpo da skill como contexto do turno, não como mutação silenciosa do system prompt estável;
6. expor erro claro se a skill não existir ou estiver desabilitada.

O usuário deve conseguir perceber que a skill foi carregada.

### D6. Carregamento sob demanda pelo modelo usa leitura explícita

Quando o modelo decide usar uma skill listada no catálogo, ele deve carregar o corpo completo por ferramenta de leitura/carregamento.

Primeira implementação aceitável:

- usar `read_file`/equivalente sobre o `SKILL.md` existente;
- registrar o resultado como contexto de tool no turno;
- manter supporting files lidos sob demanda.

Implementação futura:

- se AEP-0051 for retomada, o DB vira fonte canônica e o backend materializa um caminho read-only ou fornece uma tool `load_skill`.

### D7. Auto-load vira `base` ou deixa de existir

`auto_load` como booleano genérico deve ser descontinuado.

Equivalências:

- `auto_load=true` que define persona do perfil → `base`;
- `auto_load=true` que é workflow ocasional → `on_demand`;
- `auto_load=true` que traz estado/contexto → migrar para Context Provider.

### D8. Templates em skills são legado

O caminho novo de skills não deve exigir Go templates.

Política:

- `$ARGUMENTS` pode ser substituído por bloco estruturado de argumentos;
- `{{ now }}` deve virar context provider temporal, se necessário;
- `include "memory/..."` deve virar memory provider;
- `.Surface`, `.TaskLists`, `.Tabs` devem virar context providers;
- templates existentes continuam em compatibilidade temporária com aviso.

## Fases revisadas

### Fase 0 — AEP-0075 primeiro

Implementar Context Providers para remover `memory` e `workspace` do pool de responsabilidades de skills.

### Fase 1 — Skill modes por perfil

- Adicionar representação de `base`, `on_demand`, `disabled`.
- Migrar `enabled_skills` atual para modo compatível.
- Garantir que posição na lista não muda semântica.

### Fase 2 — Catálogo leve

- Construir catálogo só com skills `on_demand`.
- Ordenar deterministicamente.
- Aplicar orçamento.

### Fase 3 — Slash explícito e observável

- Reimplementar `/skill` como ativação de turno.
- Emitir evento/segmento de carregamento.
- Mostrar erro quando desabilitada.

### Fase 4 — Carregamento sob demanda

- Permitir o modelo carregar skill do catálogo por leitura explícita.
- Registrar skill carregada como tool/context event.

### Fase 5 — Deprecar templates e autoload legado

- Avisar quando skill usa Go templates.
- Migrar builtins.
- Remover `auto_load` após compatibilidade.

## Critérios de aceitação revisados

- `memory` e `workspace` não são skills no caminho novo.
- Perfil declara skill `base` explicitamente.
- Skills `on_demand` aparecem em catálogo leve.
- `/skill` gera ativação observável no turno.
- Skill desabilitada não aparece nem carrega.
- O corpo completo de skill não é injetado silenciosamente no system prompt estável.
- Go templates não são necessários para skills novas.

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

## Critérios de aceitação

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

# AEP-0072 — Skill Catalog & Loading (descoberta, gating e carregamento sob demanda)

**Status**: Proposta  
**Criado em**: 2026-06-08  
**Depende de**: AEP-0051 (Skills DB) — fundação de dados; AEP-0046 (UUIDv7)  
**Relacionado**: AEP-0049 (MCP DB / catalog-first), AEP-0047 (Import/Export), AEP-0063 (Tool Invocations e Executor Comum)  
**Issues**: #126 (âncora), #123 (importação observável), #119–#122 (plataforma de tools — reuso de padrão)

---

## Resumo

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

### Fase 4b — Catálogo como fonte de runtime da descoberta (D1+D2+D3)
Fechar o laço entre a Fase 3 (catálogo persistido) e a Fase 4 (disclosure): o `BuildSkillsSection` passa a montar o Nível 1 **diretamente do `skill_catalog`** (`SkillReader.ListCatalog` + `SkillSelectionPolicy.DecideAllCatalog`), sem recarregar o corpo das skills. Só as skills classificadas como **autoload** têm o corpo carregado sob demanda (por slug). O caminho legível do corpo (`Path`) é **pré-materializado** no rebuild do catálogo e persistido na entrada (`skill_catalog.path`), tornando a ativação por leitura (Nível 2) servível direto do catálogo. Snapshot de saída (`TestBuildSkillsSection_GoldenSnapshot`) e teste de não-carregamento de corpo na descoberta protegem contra regressão.

Completam-se aqui também os dois pontos de D2/D3 que faltavam:

- **Budget por janela do modelo (D3)**: o cap do Nível 1 é resolvido como percentual da janela de contexto do modelo (`Builder.SkillCatalogBudgetPercent`, default ~2% estilo Codex), lido de `profile.Chat.ContextWindow`; quando a janela é desconhecida, cai no fallback fixo de ~8.000 caracteres. Coberto por `TestBuildSkillsSection_BudgetScalesWithContextWindow`.
- **Detecção de defasagem via `ContentHash` (D2)**: o rebuild calcula um hash canônico (frontmatter + corpo via `Compose`, mais origem builtin/custom) por skill e compara com o `skill_catalog.content_hash` persistido. Em sincronia → no-op (não re-materializa nem reescreve); com defasagem (hash/contagem/`path` ausente quando há materializador) → reconstrói. Coberto por `TestRebuildCatalogSkipsWhenInSync`.

**Gating por universo de tools do perfil (D4) — fonte confiável.** O gating do Nível 1 valida cada skill contra o **universo de tools realmente disponível ao perfil**, não contra o conjunto inicialmente exposto. Princípio: o `tool_catalog` (AEP-0049) apenas *esconde* tools no começo; ele não reduz o universo. Logo:

- Perfil **sem allowlist** (`profile.Chat.EnabledTools == nil`) → universo completo: o catalog-first pode revelar qualquer tool (ou todas já estão diretas). Uma skill que exige rede/filesystem/MCP fica **disponível** mesmo que a tool comece escondida no catálogo.
- Perfil com **allowlist fixa** → o universo é exatamente essa lista (o catálogo não excede a allowlist; ver `TestBuild_CatalogFirst_NotActiveWhenCatalogPlusOtherTools`). Cada capability (`requires_filesystem/network/mcp`) é derivada das tools presentes via `tools.ToolCapabilityKind` (categorias de `internal/tools/catalog.go`; nomes desconhecidos = MCP dinâmico); o próprio `tool_catalog` não concede capability. Uma skill que exige uma capability ausente da allowlist é **ocultada**.
- A ativação por leitura (Nível 2, sob demanda) exige `read_file` no universo; sem ele, `<available_skills>` é omitido (não adianta instruir `read_file` se a tool não existe no perfil).

Coberto por `TestBuildSkillsSection_CatalogFirstProfileKeepsNetworkSkillAvailable`, `TestBuildSkillsSection_AllowlistWithCatalogStillGatesMissingCapability`, `TestBuildSkillsSection_RestrictiveEnabledToolsGatesOnDemandAndCapabilities` e `TestBuildSkillsSection_ReadFileEnabledKeepsOnDemandAndFilesystem`.

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

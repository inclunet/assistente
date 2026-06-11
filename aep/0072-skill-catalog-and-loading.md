# AEP-0072 — Skill Discovery, Gating & Loading (descoberta, gating e carregamento backend-driven)

**Status**: Proposta (revisada — carregamento backend-driven, sem materialização em disco)  
**Criado em**: 2026-06-08  
**Revisado em**: 2026-06-10  
**Depende de**: AEP-0051 (Skills DB) — fundação de dados; AEP-0046 (UUIDv7)  
**Relacionado**: AEP-0040 (backend-driven messaging — princípio), AEP-0049 (MCP DB / catalog-first), AEP-0047 (Import/Export)  
**Issues**: #126 (âncora), #123 (importação observável)

---

## Resumo

Redesenhar a **descoberta**, o **gating** e o **carregamento** de skills usando como fundação as skills persistidas em banco (AEP-0051). O system prompt passa a expor um **bloco de descoberta compacto** (nome + descrição + custo estimado), com **orçamento de contexto (budget)** e **metadados de gating** (quando uma skill é aplicável), em vez de injetar o conteúdo completo de skills sob demanda.

O carregamento do **corpo** de uma skill é **backend-driven e servido do DB**, por exatamente dois mecanismos:

- **`/slash` (explícito, decidido pelo usuário)** — já existe (`internal/skills/invocation.go`): o backend intercepta `/slug`, carrega o corpo do DB e injeta `<invoked_skill>`.
- **autoload (decidido pelo sistema)** — skills `auto_load=true` têm o corpo carregado do DB no build do prompt e injetadas em `<auto_skills>`.

**Não há ativação implícita do corpo pelo modelo**: nem tool nova de "carregar skill", nem `read_file` sobre arquivo materializado, nem cache em disco. O backend já tem o conteúdo no DB — fazer o modelo dar um round-trip num arquivo seria complexidade gratuita e contrária ao princípio backend-driven (AEP-0040). A descoberta serve para o modelo **saber quais skills existem e sugerir o `/slug`** ao usuário.

Escopo: **apenas skills**. Profiles (AEP-0050) está fora (adiado). As skills de filesystem entram no banco como **importação legada não-destrutiva** (AEP-0051, D9), no mesmo fluxo de MCP e Jobs.

---

## Motivação

### Problemas observados (issue #126)

- Skills `auto_load` incham o system prompt (instruções longas e dependentes de tools).
- Skills sob demanda não são descobertas/declaradas de forma útil; o modelo não sabe que existem nem como sugeri-las.
- A separação entre autoload e sob demanda não fica clara no comportamento do modelo.
- Skills com templates/tools podem vazar conteúdo errado quando não são aplicáveis.
- Não há orçamento de contexto para o bloco de skills; muitas skills competem com a conversa.

### Benchmark: como as ferramentas líderes fazem

- **Claude Code / Anthropic**: catálogo de skills no contexto; ativação por leitura do `SKILL.md` (progressive disclosure baseado em arquivos do filesystem). Descrição em 3ª pessoa, com gatilhos ("This skill should be used when...").
- **Codex (OpenAI)**: catálogo com **budget** (~2% da janela ou ~8.000 caracteres quando desconhecida); encurta descrições, omite excedentes e emite warning. Invocação explícita (`$skill`, `/skills`) + implícita por descrição.
- **Cursor**: rules `.mdc` com frontmatter — `alwaysApply: true` vs. escopo por glob.
- **Copilot**: `copilot-instructions.md` único, sempre ativo.

**O que adotamos e o que não adotamos.** Adotamos: catálogo compacto de descoberta, budget, descrições com gatilhos, gating por aplicabilidade, e **invocação explícita por `/slash`** (o mecanismo que todos os players oferecem para o usuário trazer uma skill ao contexto). **Não** adotamos a ativação implícita por leitura de arquivo (estilo Anthropic file-based): nossas skills moram no **DB**, não em arquivos de runtime, então o carregamento é feito pelo backend direto do DB. Quem ativa o corpo é o usuário (`/slash`) ou o sistema (autoload).

---

## Estado atual (após AEP-0051)

- Skills já persistem no banco (`skills`, `skill_tools`) e o runtime serve do DB (AEP-0051 Fases 1–6b). `discoverAll()` do filesystem é apenas origem de importação legada.
- `internal/skills/invocation.go` (`Invoke`): `/slash` já carrega o corpo do DB (`manager.Get`) e injeta `<invoked_skill>`. **Sem arquivo, sem cache.**
- `internal/prompt/builder.go` (`BuildSkillsSection`): emite `<auto_skills>` (corpo completo das `auto_load`) e `<available_skills>` (referências). Ainda sem catálogo compacto, budget ou gating semântico.
- `internal/skills/types.go`: `SkillMetadata` é rico, mas **não** tem `context_budget` nem `requires_*` semânticos para gating.

---

## Decisões

### D1. Descoberta derivada on-the-fly da tabela `skills` (sem catálogo persistido)

> **Revisão.** A versão original previa uma tabela `skill_catalog` persistida, "espelhando o `tool_catalog`" (AEP-0049), com rebuild, `ContentHash` e detecção de drift. **Descartado.** O paralelo com tools não se sustenta: tools vêm de fontes heterogêneas e dinâmicas (builtins + MCP descoberto em runtime), justificando um catálogo persistido; **skills** são poucas e já moram inteiras numa única tabela. A descoberta só precisa de `name`, `description`, `context_budget`, `requires_*`, `auto_load` e `user_invocable` — tudo derivável de um `SELECT` leve em `skills` **sem a coluna `content`**. Uma tabela de catálogo seria cópia denormalizada redundante; manter rebuild/drift em sincronia é complexidade pura.

A descoberta (Nível 1) é uma **projeção leve, calculada sob demanda** a partir da tabela `skills` (sem carregar o corpo). Sem tabela nova, sem rebuild, sem hash de defasagem.

### D2. Carregamento backend-driven: `/slash` + autoload, servidos do DB

- **Descoberta (Nível 1)**: bloco compacto no system prompt — `name`, `description` (com gatilhos) e custo estimado. **Sem corpo e sem path.** Serve para o modelo saber quais skills existem e **sugerir ao usuário** a invocação por `/slug`.
- **Ativação por `/slash` (explícita)**: ao digitar `/slug`, o backend carrega o corpo do DB (`Invoke` → `manager.Get`) e injeta `<invoked_skill>`. Sem arquivo, sem `read_file`, sem cache.
- **Autoload (sistema)**: skills `auto_load=true` têm o corpo carregado do DB no build do prompt e injetadas em `<auto_skills>`.
- **Arquivos de referência (Nível 3)**: arquivos complementares que vivem em disco (AEP-0051 D4) seguem sendo lidos sob demanda via `read_file`/`GetSkillFiles` **depois que a skill foi invocada** — são legitimamente de filesystem e não fazem parte do corpo canônico.

**Não há ativação implícita do corpo pelo modelo** (nem `read_file`, nem tool nova). Consequência: some o conceito de "model invocable" (`DisableModelInvocation`/`ModelInvocable`); o que importa é `user_invocable` (pode `/slash`) e `auto_load`.

**Divergência com AEP-0051 D8 — resolvida trivialmente.** Como `/slash` e autoload servem o corpo direto do DB, o banco permanece a fonte canônica e **nunca** é preciso expor um path nem materializar nada em disco. O `SKILL.md` importado (AEP-0051 D9) segue apenas como origem de importação.

### D3. Orçamento de contexto (budget) no bloco de descoberta

O bloco de descoberta tem um **cap simples** (em caracteres; opcionalmente derivado da janela do modelo). Ao exceder: omitem-se skills de menor prioridade, e a omissão é **sinalizada** (log/telemetria). Skills `auto_load` não competem nesse cap (já entram pelo corpo). O budget aplica-se só à descoberta; `/slash` e autoload carregam o corpo normalmente.

### D4. Metadados de gating no frontmatter e no DB

Estender `SkillMetadata` (e o schema da AEP-0051) com campos de descoberta/gating:

- `context_budget`: custo aproximado (em caracteres/tokens) do corpo da skill, para o planner da descoberta.
- `requires_tools` / `requires_filesystem` / `requires_network` / `requires_mcp`: pré-condições de capability (explícitas ou inferidas das permissões).
- `autoload_reason`: justificativa textual obrigatória quando `auto_load=true` (ver D5).
- Gatilhos de descrição: orientação para descrições em 3ª pessoa com frases-gatilho, validadas no import (lições de #165/#159).

**Aplicação do gating:**
- **Autoload**: uma skill que exige uma capability ausente do perfil/contexto **não é auto-injetada** (não adianta colar instruções que dependem de rede com rede off).
- **`/slash` (explícito)**: sempre disponível se `user_invocable` — o usuário pediu explicitamente; não bloqueamos a invocação por capability (a skill pode degradar, mas a decisão é do usuário).
- **Descoberta**: skills `user_invocable` aparecem no bloco compacto (respeitando budget), para o modelo poder sugerir.

### D5. `auto_load` é exceção, não regra

O default é **sob demanda** (via `/slash`). `auto_load=true` só para skills que realmente precisam estar sempre no prompt, e exige `autoload_reason`. Objetivo: prompt inicial enxuto.

### D6. Skills de filesystem = importação legada (delegada à AEP-0051)

A origem dos dados (filesystem → DB) é a **importação legada não-destrutiva e idempotente** (AEP-0051 D9), no fluxo pós-login de MCP/Jobs (`runPostLoginLegacyImports`). Esta AEP **não** redefine esse mecanismo; consome as skills no banco. Os `SKILL.md` originais não são renomeados/apagados.

---

## Fases

### Fase 1 — Fundação de dados + importação legada (AEP-0051) — **concluída**
Tabelas `skills`/`skill_tools`, runtime servido do DB, `/slash` do DB e importador legado plugado em `runPostLoginLegacyImports`.

### Fase 2 — Metadados de gating (D4)
Estender `SkillMetadata` e o schema com `context_budget`, `requires_*`, `autoload_reason` e gatilhos de descrição; validação de descrição no import (3ª pessoa, frases-gatilho, comprimento por runes). Backward-compat: campos ausentes assumem default sob demanda.

### Fase 3 — `SkillSelectionPolicy` (D1/D4)
Política única, determinística e testável sem LLM, que classifica cada skill em **autoload / sob demanda (descobrível) / oculta** a partir do perfil/tools/contexto. Opera sobre a projeção leve de `skills` (sem corpo). Fonte única de verdade para "esta skill é aplicável/visível agora?".

### Fase 4 — Descoberta + budget + carregamento (D2/D3)
Refatorar `BuildSkillsSection` (`internal/prompt/builder.go`): `<auto_skills>` carregado do DB só para `auto_load` (com `autoload_reason`); `<available_skills>` compacto e **sem path**, instruindo que skills são invocadas pelo usuário via `/slug`; aplicar budget (omissão observável) e gating de D4. Sem materialização, sem `read_file` do corpo.

### Fase 5 — Importação observável + testes
Resultado estruturado/telemetria na importação legada (#123), cobrindo skills. Testes de regressão: autoload vs. sob demanda; gating por capability/perfil; templates inválidos; budget (omissão/warning); `/slash` servindo do DB; importação idempotente e não-destrutiva.

> Fora de escopo (adiado): vínculo perfil↔conversa (#118) e Profiles DB (AEP-0050).

---

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|-------|---------------|---------|-----------|
| R1 | Importação filesystem→DB com duplicação/efeito colateral | Baixa | Alto | Delegada à AEP-0051 D9 (idempotente, não-destrutiva, dedup por slug com prioridade workdir>home>exe) |
| R2 | Regressão no system prompt (skills sumindo ou inflando) | Média | Médio | Teste de snapshot de `BuildSkillsSection`; `auto_load` preservado para skills que já eram autoload |
| R3 | Budget cortar skills relevantes da descoberta | Média | Médio | Ranking por prioridade; warning observável; cap configurável |
| R4 | Divergência entre campos de gating (D4) e o schema da AEP-0051 | Média | Médio | D4 estende o schema da 0051 explicitamente (Fase 2); migração aditiva |
| R5 | Projeção de descoberta recalculada a cada prompt | Baixa | Baixo | `SELECT` leve sem `content`; poucas skills; cacheável em memória se necessário |
| R6 | Descrições fracas prejudicam a sugestão pelo modelo | Média | Médio | Validação de descrição no import (3ª pessoa, gatilhos), lições de #165/#159 |

---

## Critérios de aceitação

Os seis critérios da issue #126:

1. O prompt inicial não injeta conteúdo completo de skills sob demanda.
2. Skills sob demanda aparecem em uma declaração compacta e útil para o modelo.
3. Existe caminho explícito para carregar uma skill sob demanda quando relevante: **`/slash`, servido do DB pelo backend** (sem `read_file`, sem cache).
4. Skills dependentes de tools/filesystem/network/MCP são omitidas/degradadas no **autoload** quando essas capacidades estão desabilitadas; `/slash` permanece disponível se `user_invocable`.
5. Autoload continua funcionando (do DB) para skills que realmente precisam ser carregadas automaticamente.
6. Há testes cobrindo autoload, sob demanda, gating por capability/perfil, templates inválidos e `/slash` do DB.

Mais os específicos desta AEP:

7. **Budget aplicado**: o bloco de descoberta respeita um cap configurável, omitindo skills com sinalização observável.
8. **Gating testável sem LLM**: a `SkillSelectionPolicy` decide visibilidade/aplicabilidade de forma determinística e coberta por testes.
9. **Sem materialização/cache de disco**: o corpo da skill nunca é gravado em disco para runtime; `/slash` e autoload servem do DB.
10. **Importação não-destrutiva e idempotente** (herdada da AEP-0051): re-executar não duplica nem altera os `SKILL.md` originais.

---

## Relações

- **Âncora**: issue #126 (redesenho de carregamento e execução de skills).
- **Princípio**: AEP-0040 (backend-driven messaging) — o backend orquestra; o modelo não busca arquivos para carregar skills.
- **Fundação**: AEP-0051 (Skills DB); AEP-0046 (UUIDv7).
- **Importação**: #123 (importação observável), AEP-0047 (import/export).
- **Fora de escopo (adiado)**: #118 (perfil↔conversa) e AEP-0050 (Profiles DB).
- **Lições (issues fechadas, não reabrir)**: #165, #159 (contrato de template/schema), #154/#155, #173/#176/#171 (job-manager skill).
- **Histórico**: a abordagem de catálogo persistido + materialização em disco + ativação por `read_file` (PRs #220–#228) foi revertida; o trabalho é refeito de forma enxuta a partir de `feat/aep-0051-skills-api-cleanup`.

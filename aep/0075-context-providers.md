# AEP-0075 — Context Providers

Status: Implementada
Criado em: 2026-06-16
Implementada em: 2026-06-19
Relacionado: AEP-0072, AEP-0074, AEP-0059, AEP-0042, AEP-0057

## Resumo

Separar **contexto dinâmico** de **skills**.

Hoje `memory` e `workspace` são modelados como skills, mas conceitualmente não são workflows nem instruções procedurais. Eles são fontes de estado e conhecimento dinâmico que alimentam o prompt. Essa mistura tornou o carregamento de skills confuso, prejudicou cache de prompt e forçou skills a suportarem templates Go para resolver necessidades que pertencem a outra camada.

Esta AEP cria a arquitetura de `Context Providers`, responsáveis por produzir blocos de contexto dinâmico e ferramentas de recuperação/ação associadas. Com isso:

- skills voltam a ser módulos de instrução/workflow;
- memória e workspace deixam de ser skills;
- Go templates deixam de ser requisito para skills;
- o prompt passa a ter uma ordem clara por volatilidade;
- a AEP-0074 de prompt cache pode otimizar um contexto já bem separado.

Resultado da implementação:

- `memory`, `workspace` e `tasklist` são registrados como Context Providers no runtime;
- `memory` usa records estruturados em banco no caminho novo, com fallback legado somente como compatibilidade de leitura;
- tasklists vinculadas são renderizadas pelo provider `tasklist`, preservando o gating da skill `tasklist-manager`;
- perfis configuram Context Providers por `enabled`, `budget` em caracteres/runes e `settings`;
- a UI de perfil tem aba dedicada de Context Providers, separada de skills e de cache;
- há APIs e tela de governança para memórias;
- skills builtin não dependem de execução de templates para acessar memória, workspace ou tasklists.

## Motivação

O sistema atual usa skills para responsabilidades diferentes:

- workflows e instruções (`tech-support`, `github`, `flock-api`);
- memória persistente (`memory`);
- estado do workspace (`workspace`);
- contexto de tasklists;
- includes dinâmicos e templates (`{{ now }}`, `include`, `.Surface`, `.TaskLists`).

Isso cria problemas:

- `/skill` não dá sensação de carregamento explícito;
- corpo de skill pode ser injetado silenciosamente no system prompt;
- memória/workspace mudam com frequência e invalidam prefix cache;
- Go templates tornam skills difíceis de razonar, testar e cachear;
- autores de skills misturam instrução estável com estado dinâmico;
- AEP-0072 ficou ampla demais ao tentar resolver catálogo, banco, gating e contexto.

Separar context providers antes de refazer skill loading reduz o escopo e torna as decisões mais parecidas com Cursor, Claude Code, Codex, OpenClaw e Hermes.

## Decisões

### D1. Context Provider é uma categoria nova

Um `ContextProvider` é uma fonte de contexto para o LLM. Ele pode produzir:

- instruções estáveis curtas sobre como usar aquele contexto;
- blocos dinâmicos para o prompt;
- tools de busca, leitura, escrita ou inspeção;
- metadados de volatilidade, orçamento e prioridade.

Contrato conceitual:

```go
type ContextProvider interface {
    Slug() string
    StableInstructions(ctx ContextBuildContext) ContextBlock
    DynamicContext(ctx ContextBuildContext) []ContextBlock
    Tools(ctx ContextBuildContext) []ToolDefinition
}
```

O contrato real pode variar, mas deve preservar a separação entre instrução estável e contexto dinâmico.

### D2. Memória vira Context Provider

`memory` deixa de ser skill.

Novo desenho:

- instruções estáveis sobre como usar memória entram no prefixo estável;
- memórias pinned/essenciais entram em um bloco dinâmico pequeno;
- memórias longas ou raramente usadas ficam acessíveis por tools;
- escrita/atualização de memória é ação explícita via tool;
- memória completa não é despejada automaticamente no system prompt.

Tool esperada:

- `memory`, com `action` explícita para `search`, `list`, `create`, `update` e `delete`.

Política:

- fatos essenciais e preferências globais podem entrar sempre, com orçamento baixo;
- FAQs, históricos longos e notas operacionais são recuperáveis;
- mudanças de memória não reescrevem o prefixo estável.

### D2.1. Memória persistida em banco, sem migrador automático

O Memory Provider deve usar records estruturados em banco como fonte canônica no caminho novo.

Cada record deve ter classificação suficiente para decidir se entra automaticamente no contexto ou se fica disponível sob demanda. Campos conceituais:

- `load_policy`: `core`, `pinned`, `auto`, `retrievable`, `archived`;
- `kind`: `user_preference`, `identity`, `project_fact`, `decision`, `convention`, `historical_note`, `resolved_issue`;
- `scope`: `global`, `user`, `workspace`, `project`, `conversation`;
- `importance`, `confidence`, `tags`, `last_used_at`, `expires_at`;
- origem opcional, como conversa, arquivo legado ou tool call.

Política de carregamento:

- `core`: sempre carregado, com orçamento muito baixo;
- `pinned`: carregado automaticamente quando o escopo combina com perfil/workspace;
- `auto`: candidato a entrar por score de relevância, recência e orçamento;
- `retrievable`: histórico buscado sob demanda pela action `search` da tool `memory`;
- `archived`: preservado, mas fora do fluxo normal.

Não haverá migrador automático dos arquivos antigos em `~/.assistente/memory`.

Migração dos dados legados:

- os arquivos antigos continuam como fonte de referência temporária;
- o usuário poderá pedir ao modelo para ler esses arquivos e recompor a memória em records estruturados;
- o modelo deve classificar cada record ao gravar no banco;
- a recomposição assistida deve preferir qualidade e deduplicação, não conversão mecânica 1:1;
- após validação manual, os arquivos podem ser mantidos apenas como backup/legado.

### D2.2. Memórias precisam de governança no frontend

Ao sair de arquivos Markdown e passar a usar records estruturados em banco, a memória deixa de ser diretamente editável pelo usuário no filesystem. Portanto, o Memory Provider exige uma tela de governança no frontend.

Essa tela não é um extra de conveniência; ela substitui a capacidade atual de abrir e editar `~/.assistente/memory/*.md`.

Escopo inicial:

- listar memórias salvas;
- pesquisar e filtrar por política, tipo, escopo, tags e texto;
- visualizar conteúdo e metadados essenciais;
- editar conteúdo, `load_policy`, `kind`, `scope`, tags, importância e confiança;
- criar nova memória manualmente;
- arquivar/desarquivar records;
- excluir records com confirmação;
- indicar claramente quais records podem entrar automaticamente no prompt;
- apoiar a recomposição assistida dos arquivos legados, permitindo revisão dos records criados pelo modelo.

Regras de UX:

- records `core`, `pinned` e candidatos `auto` devem ser visualmente distinguíveis por texto, badges e agrupamento, nunca apenas por cor;
- records `archived` devem ficar fora da lista padrão, mas acessíveis por filtro;
- a tela deve explicar o impacto de cada `load_policy` no contexto do modelo;
- edições devem ser auditáveis o suficiente para diagnóstico: registrar timestamps e, quando possível, origem/última atualização;
- exclusão deve usar confirmação acessível.

Regras técnicas do frontend:

- usar componentes existentes em `frontend/src/components/ui/`, especialmente `DataGrid`, `Modal`, `ConfirmDialog`, `Button` e `Toolbar`;
- todas as strings visíveis devem usar i18n em `frontend/src/locales/pt-BR.ts`, `frontend/src/locales/en.ts` e `frontend/src/locales/es.ts`;
- não usar cores hardcoded; usar tokens de `frontend/src/theme.css`;
- a navegação por teclado deve permitir listar, filtrar, abrir detalhe, salvar, arquivar e cancelar sem mouse.

### D2.3. APIs Wails para memória

O backend deve expor APIs Wails próprias para CRUD e busca de records de memória. Essas APIs são separadas das tools do modelo: a UI usa APIs de aplicação; o LLM usa tools do Context Provider.

APIs conceituais:

- `ListMemoryRecords(filter)`: lista records com paginação/filtros;
- `GetMemoryRecord(id)`: obtém detalhe;
- `CreateMemoryRecord(input)`: cria record manual ou assistido;
- `UpdateMemoryRecord(id, input)`: atualiza conteúdo, classificação e metadados editáveis;
- `ArchiveMemoryRecord(id)` / `UnarchiveMemoryRecord(id)`: muda estado sem apagar;
- `DeleteMemoryRecord(id)`: remove record com confirmação no frontend;
- `SearchMemoryRecords(query, filter)`: busca textual/FTS para UI e apoio às tools;
- `GetMemoryPolicySummary()`: retorna contagens e orçamento por `load_policy` para explicar impacto no prompt.

Contrato de segurança:

- operações devem respeitar `UserID` e escopo multiusuário existente;
- records arquivados não entram em contexto automático;
- exclusões devem ser físicas ou lógicas conforme política de retenção futura, mas a primeira implementação deve deixar o comportamento explícito na UI;
- dados sensíveis devem seguir o mesmo cuidado de armazenamento e exposição das demais entidades locais do app.

A tool `memory` pode reutilizar o serviço interno de memória, mas não deve expor diretamente APIs de UI ao modelo. A consolidação em uma única tool reduz superfície de contrato e mantém as ações explícitas via `action`.

### D2.4. Integração frontend da tela de memórias

A tela deve seguir os padrões já usados em páginas administrativas do frontend, como `frontend/src/pages/SkillsPage.tsx`.

Integração esperada:

- criar uma página dedicada, por exemplo `frontend/src/pages/MemoriesPage.tsx`;
- registrar a rota e a entrada de navegação no mesmo padrão das demais páginas de configuração/gestão;
- usar `DataGrid` para a lista principal, com seleção por teclado e foco restaurável;
- usar `Toolbar` para busca, filtros e ações principais;
- usar `Modal` para criação/edição de record;
- usar `ConfirmDialog`/`useConfirm` para exclusão;
- usar `useAnnouncer` para anunciar criação, edição, arquivamento, restauração e exclusão;
- usar `useGridFocus` e `useGridPageLandmarks` quando aplicável, mantendo navegação por landmarks consistente com páginas como Skills e Providers;
- usar tipos gerados em `frontend/wailsjs/go/models.ts` para os records e inputs expostos pelo backend;
- manter CSS próprio da página somente com tokens de `frontend/src/theme.css`.

Filtros mínimos:

- texto livre;
- `load_policy`;
- `kind`;
- `scope`;
- tags;
- incluir/excluir arquivadas.

Colunas mínimas:

- resumo ou conteúdo curto;
- política de carregamento;
- tipo;
- escopo;
- importância;
- última atualização;
- origem quando existir.

O detalhe de edição deve mostrar uma explicação curta da política selecionada. Isso é necessário porque mudar `core`, `pinned`, `auto`, `retrievable` ou `archived` altera diretamente quais memórias podem impactar o prompt.

### D3. Workspace vira Context Provider

`workspace` deixa de ser skill.

Novo desenho:

- contexto mínimo entra no prompt: workspace ativo, superfície/aba ativa e, quando aplicável, arquivo ativo;
- listas grandes de abas, arquivos abertos, estado completo e payloads de superfície ficam sob demanda;
- ferramentas expõem inspeção quando necessário.

Tools esperadas:

- `workspace_state`
- `workspace_tabs`
- `workspace_active_surface`

O contexto de workspace deve ser pequeno, ordenado e classificado como dinâmico.

### D4. Tasklists e outros estados seguem o mesmo padrão

Tasklists vinculadas, estado de editor, terminal, tickets ou superfícies específicas não devem entrar em skills. Eles devem ser context providers próprios ou extensões de providers existentes.

Regra:

- se é estado do aplicativo ou dado dinâmico, é context provider;
- se é workflow/instrução, é skill;
- se é ação/busca, é tool.

Implementação atual:

- `linked_task_lists` é produzido por um Context Provider `tasklist`;
- o provider recebe as listas vinculadas já resolvidas para a conversa e só renderiza quando o runtime informa que o contexto de tasklists está habilitado;
- esse gating preserva a decisão de perfil: `tasklist-manager` precisa estar habilitada (`base` ou `on_demand`) e `disable_skills=true` continua implicando prompt enxuto;
- as instruções estáveis de workflow continuam na skill `tasklist-manager`; o provider carrega o estado dinâmico vinculado à conversa e uma instrução mínima delimitada ao uso daquele bloco.

### D5. Skills deixam de depender de Go templates

Com memória/workspace/tasklists fora das skills, Go templates deixam de fazer parte do runtime de skills.

Decisão:

- skills são Markdown estático com frontmatter declarativo;
- argumentos de `/skill` podem ser passados como bloco separado, não por substituição textual no corpo;
- includes dinâmicos devem ser substituídos por context providers ou supporting files lidos sob demanda;
- `{{ now }}` não pertence a skill; data/hora é contexto dinâmico produzido por provider quando necessário.

Compatibilidade:

- não há compatibilidade de runtime para templates em skills;
- templates executáveis ou dependentes de dados dinâmicos em skills são bug de migração e devem ser removidos;
- sequências como `{{ ... }}` podem existir como texto literal em exemplos Markdown, sem interpretação pelo runtime;
- Context Providers e supporting files são as formas aceitas de fornecer contexto dinâmico.

### D6. Prompt passa a ser montado por blocos

O prompt deve ser montado em blocos classificados por origem e volatilidade.

Ordem alvo:

```text
stable:
  - system base
  - instruções base do perfil
  - instruções estáveis de context providers
  - base skills estáveis
  - tool schemas estáveis
  - catálogo de skills on-demand

slow_dynamic:
  - resumo da conversa
  - memórias pinned/essenciais

rolling_history:
  - janela recente de mensagens

fast_dynamic:
  - workspace/surface atual
  - slash skill do turno
  - resultados recuperados por tools/context providers
  - mensagem atual do usuário
```

Esta AEP não implementa otimização de cache diretamente; ela prepara o terreno para a AEP-0074.

### D7. Perfil controla context providers

Perfis devem poder configurar quais Context Providers estão disponíveis naquele perfil, quanto contexto cada um pode injetar automaticamente e quais opções específicas daquele provider se aplicam.

Essa configuração não é uma configuração de cache. Ela existe porque Context Providers são fontes de contexto/estado com custo, relevância e risco diferentes conforme o perfil. Um perfil de programação pode querer mais workspace; um perfil de gestão pode querer mais tasklists; um perfil enxuto pode desligar providers que não fazem sentido para aquele fluxo.

Regras:

- cada provider pode estar habilitado ou desabilitado por perfil;
- providers habilitados podem ter budget próprio em caracteres/runes do bloco produzido, não em tokens;
- providers podem ter settings específicos, validados pelo próprio provider ou por um contrato registrado;
- defaults por provider continuam existindo para perfis antigos ou campos omitidos;
- a UI de perfil deve ter uma aba própria de Context Providers, separada de skills e separada de cache;
- a configuração resolvida deve alimentar `contextprovider.BuildRequest.ProviderBudgets` e futuras opções do provider;
- desabilitar um provider remove seus blocos automáticos do prompt, mas não necessariamente remove tools de ação relacionadas se elas forem habilitadas por outro mecanismo do perfil.

Exemplo conceitual:

```json
{
  "context_providers": {
    "memory": {
      "enabled": true,
      "budget": 1200,
      "settings": {
        "mode": "pinned_plus_auto"
      }
    },
    "workspace": {
      "enabled": true,
      "budget": 500,
      "settings": {
        "mode": "minimal"
      }
    },
    "tasklist": {
      "enabled": true,
      "budget": 4000,
      "settings": {}
    }
  }
}
```

Formato conceitual no Go:

```go
type ContextProviderProfileConfig struct {
    Enabled  *bool          `json:"enabled,omitempty"`
    Budget   int            `json:"budget,omitempty"` // caracteres/runes, não tokens
    Settings map[string]any `json:"settings,omitempty"`
}
```

Semântica:

- `enabled == nil`: usar default do provider/perfil legado;
- `enabled == false`: não montar blocos automáticos daquele provider;
- `enabled == true`: provider pode montar blocos, respeitando budget/defaults;
- `budget <= 0`: usar default do provider;
- `budget > 0`: limite máximo em caracteres/runes do bloco final produzido pelo provider; não é orçamento em tokens;
- `settings`: espaço namespaced para opções específicas, sem transformar `ChatConfig` em uma lista de campos por provider.

Responsabilidade de implementação:

- resolver defaults, overrides, `enabled`, `budget` e `settings` por perfil é responsabilidade da implementação de Context Providers;
- o resultado resolvido deve ser aplicado antes da montagem dos blocos, alimentando `contextprovider.BuildRequest` e a decisão de quais providers automáticos podem contribuir;
- esta resolução é independente da AEP-0074: prompt cache consome os blocos já produzidos e ordenados, sem conhecer detalhes de budget/settings de cada provider.

UI:

- criar uma aba "Context Providers" no editor de perfil;
- listar providers registrados com nome, descrição, estado e budget efetivo;
- permitir habilitar/desabilitar provider por perfil;
- permitir editar budget quando aplicável;
- mostrar settings específicas só quando o provider declarar suporte;
- usar i18n e componentes acessíveis existentes (`DataGrid`, `Toolbar`, `Modal`, `Button`, `ConfirmDialog`);
- não usar cores hardcoded; usar tokens do tema.

Esta seção é pré-requisito funcional para controlar budgets por perfil, mas não é pré-requisito para a AEP-0074 reorganizar o layout cache-friendly da request. A AEP-0074 pode usar defaults até esta configuração estar implementada.

## Fases

### Fase 1 — Contrato e separação conceitual

Status: concluída.

- Criar abstrações internas de context block/provider.
- Identificar os trechos atuais de `memory` e `workspace` que são instrução estável vs. dados dinâmicos.
- Não mudar ainda a UI de skills.

### Fase 2 — Memory provider

Status: concluída no caminho novo, mantendo fallback legado de leitura conforme previsto.

- Extrair a skill `memory` para provider.
- Criar bloco de instruções estáveis de memória.
- Criar bloco dinâmico pequeno de memórias pinned/essenciais.
- Persistir records de memória em banco com classificação de carregamento.
- Introduzir tools de busca/leitura/escrita de memória.
- Expor APIs Wails para CRUD, busca, arquivamento e resumo de política.
- Criar tela frontend para listar, filtrar, editar, arquivar e excluir memórias.
- Manter fallback legado até a migração estar validada.

### Fase 3 — Workspace provider

Status: concluída.

- Extrair a skill `workspace` para provider.
- Produzir contexto mínimo de workspace.
- Criar tools de inspeção de workspace.
- Remover includes/templates de workspace de skills.

### Fase 4 — Prompt block builder

Status: concluída como base de Context Providers; a AEP-0074 aprofunda o layout cache-friendly e determinismo da request.

- Substituir montagem monolítica do system prompt por blocos ordenados.
- Classificar cada bloco por volatilidade.
- Adicionar testes snapshot para ordem e conteúdo.

### Fase 5 — Configuração de providers por perfil

Status: concluída.

- Adicionar `context_providers` ao perfil.
- Resolver defaults por provider.
- Preencher `contextprovider.BuildRequest.ProviderBudgets` a partir do perfil.
- Respeitar `enabled=false` para blocos automáticos.
- Criar aba "Context Providers" no editor de perfil.
- Permitir edição de budget e settings específicas declaradas pelo provider.

### Fase 6 — Auditar skills estáticas

Esta fase é uma auditoria de segurança da migração: confirmar que o runtime novo não executa templates em skills e que os builtins migrados usam Context Providers/supporting files para conteúdo dinâmico, sem degradar o conteúdo instrucional das skills.

Status: concluída para os builtins atuais. O runtime novo trata `SKILL.md` como Markdown estático; exemplos literais com `{{ ... }}` permanecem válidos em skills como `job-manager` e `tasklist-manager`, porque documentam templates de jobs/custom actions e não são executados pelo runtime de skills.

- Remover apenas dependências reais de execução de templates em skill.
- Garantir que builtins não dependem de `include`, `now`, `.Surface`, `.TaskLists`.
- Atualizar editor/validação de skills para tratar templates executáveis como inválidos, preservando exemplos literais em Markdown.
- Não invalidar skills apenas por conterem marcações parecidas com templates, como `{{ ... }}`, quando forem exemplos ou texto literal.
- Não remover blocos inteiros de instrução só por conterem sintaxe parecida com template; revisar o propósito do bloco e preservar instruções úteis sempre que elas não dependem de renderização dinâmica.

## Riscos

- Memória via tool pode ser menos lembrada pelo modelo que memória no prompt.
- Bloco pinned pequeno pode omitir fatos úteis.
- Sem UI, a mudança para banco reduziria a autonomia do usuário em comparação ao Markdown.
- UI de memória pode induzir o usuário a promover registros demais para `core`/`pinned`, aumentando tokens e reduzindo cache.
- Workspace minimalista pode exigir mais tool calls.
- Migração de skills legadas pode quebrar workflows existentes se não houver fallback.
- Perfil pode ficar complexo se todos os providers tiverem knobs expostos na UI.

## Critérios de aceitação

- [x] `memory` não é mais carregada como skill no caminho novo.
- [x] `workspace` não é mais carregada como skill no caminho novo.
- [x] Memórias são persistidas como records estruturados no banco no caminho novo.
- [x] Não existe migrador automático dos arquivos antigos de memória; a recomposição é assistida pelo modelo.
- [x] Existe tela frontend para visualizar, filtrar, editar, arquivar e excluir memórias salvas.
- [x] A tela deixa claro quais memórias impactam automaticamente o prompt.
- [x] APIs Wails permitem CRUD, busca e arquivamento de records de memória.
- [x] Existe pelo menos um bloco estável e um bloco dinâmico produzido por context provider.
- [x] Skills não usam Go templates para acessar memória/workspace/tasklists.
- [x] Prompt builder tem ordem testável: stable → slow_dynamic → rolling_history → fast_dynamic.
- [x] AEP-0072 revisada pode focar apenas em Skill Loading Runtime.
- [x] AEP-0074 passa a depender desta AEP para otimização de cache.

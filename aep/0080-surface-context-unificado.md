# AEP-0080: SurfaceContext Unificado

## Status: In Progress

Criado em: 2026-06-27
Relacionado: AEP-0034, AEP-0040, AEP-0042, AEP-0057, AEP-0058, AEP-0074-A, AEP-0074-B, AEP-0075, AEP-0079

## Resumo

Esta AEP define um contrato unificado de `SurfaceContext` para todas as surfaces do workspace que enviam contexto ao chat, ao prompt e às tools.

O objetivo é substituir payloads parciais, ambíguos ou específicos demais por um envelope comum, versionado e seguro, mantendo especializações por tipo de surface. O contrato se aplica inicialmente a editor, tasklists e terminal, sem criar fluxos paralelos de mensagem e sem mudar a decisão da AEP-0040: novas mensagens continuam passando por `SendMessage` e retries por `RetryMessage`.

A AEP-0042 continua sendo a base histórica do `surfaceContextJson`; esta AEP evolui esse campo para um contrato estruturado e mais explícito. Durante a migração, `surfaceContextJson` pode continuar existindo como transporte compatível, desde que carregue o novo formato ou seja adaptado para ele antes da renderização.

## Motivação

O workspace já captura contexto rico nas surfaces, mas parte desse contexto se perde ou chega ambígua no prompt e nas tools.

Problemas atuais:

- o prompt pode saber que existe uma surface ativa, mas não consegue distinguir com segurança o alvo exato da ação;
- o editor carrega dados úteis como arquivo, seleção, cursor, modo Markdown/Reveal e slide atual, mas nem tudo chega em formato previsível;
- tasklists têm lista, card, status/coluna e seleção, mas o contexto pode ficar espalhado entre bloco de tasklist, surface transitória e texto do usuário;
- terminal tem diretório, shell, input, seleção e output recente, mas esses dados precisam de limites claros para não despejar histórico sensível ou grande demais;
- tools podem inferir alvo por heurística quando deveriam receber target explícito e validar se o snapshot ainda é atual;
- contexto sintético no texto do usuário prejudica transparência, cache e rastreabilidade.

O resultado é uma classe de bugs em que o assistente age sobre o arquivo, card, coluna, seleção ou terminal errado. Esta AEP não preserva compatibilidade com bugs de contexto ambíguo: quando o alvo for necessário, ele deve ser explícito ou a ação deve pedir confirmação/contexto adicional.

## Decisões

### 1. Contrato comum `SurfaceContext`

Toda surface que participa do envio ao chat deve produzir um envelope lógico com campos comuns:

```ts
type SurfaceType = "editor" | "tasklist" | "terminal" | (string & {})

type SurfaceContext = {
  surfaceType: SurfaceType
  surfaceId: string
  title?: string
  mode?: string
  selection?: SurfaceSelection
  focus?: SurfaceFocus
  content?: SurfaceContent
  metadata?: Record<string, unknown>
  snapshotVersion: string
  capturedAt?: string
  staleAfterMs?: number
}
```

Semântica:

- `surfaceType` identifica o tipo canônico da surface, alinhado ao `WorkspaceTab.type` quando a origem for uma aba. Extensões fora dos tipos canônicos devem usar um tipo aberto como `string & {}` para não degradar autocomplete e checagem dos literais conhecidos em TypeScript.
- `surfaceId` identifica a instância de origem, como tab ID, session key ou ID estável equivalente.
- `title` é rótulo humano para prompt e transparência.
- `mode` descreve variação relevante de comportamento, como `markdown`, `rich`, `reveal`, `kanban`, `list` ou `shell`.
- `selection` descreve o trecho, card, coluna, output ou item selecionado.
- `focus` descreve o alvo atualmente focado quando diferente da seleção.
- `content` contém snapshot limitado e permitido para o modelo.
- `metadata` carrega dados auxiliares não essenciais para renderização.
- `snapshotVersion` é obrigatório para validar staleness em tools e ações destrutivas. Deve ser sempre uma string opaca, estável para comparação e segura para transporte JSON, logs e tool targets.
- `capturedAt`, quando presente, deve ser uma string ISO 8601/RFC3339 com timezone explícito. Ele e `staleAfterMs` ajudam o backend a sinalizar contexto antigo, principalmente em terminal e edição concorrente.

Exemplo curto:

```json
{
  "surfaceType": "editor",
  "surfaceId": "tab-3",
  "title": "slides.md",
  "mode": "reveal",
  "selection": {
    "kind": "text",
    "range": { "startLine": 12, "startColumn": 1, "endLine": 18, "endColumn": 1 },
    "text": "## Objetivo\n..."
  },
  "focus": { "kind": "slide", "slideIndex": 2, "label": "Objetivo" },
  "content": { "language": "markdown", "currentSlideMarkdown": "## Objetivo\n..." },
  "metadata": { "filePath": "C:/projeto/slides.md", "slideCount": 8 },
  "snapshotVersion": "editor:tab-3:42"
}
```

### 2. Providers/adaptadores por surface no frontend

Cada surface deve ter um provider/adaptador frontend responsável por produzir `SurfaceContext`.

Regras:

- o provider conhece detalhes da UI local, como seleção visual, foco, modo de edição e item ativo;
- o provider emite apenas dados necessários para o turno atual;
- a surface não serializa contexto diretamente em texto de usuário;
- o chat modal, a aba de chat vinculada e futuras entradas de envio usam o mesmo contrato compartilhado;
- dados grandes devem ser resumidos no frontend quando isso preservar melhor a intenção do usuário, mas a decisão final de truncamento para prompt pertence ao backend.

Providers iniciais:

- `editorSurfaceContextProvider`;
- `tasklistSurfaceContextProvider`;
- `terminalSurfaceContextProvider`.

O provider do workspace pode orquestrar a escolha da surface ativa/focada, mas não deve reconstruir dados efêmeros por inferência quando a surface pode fornecê-los explicitamente.

### 3. Renderização backend segura para prompt

O backend deve renderizar o contexto em bloco delimitado, preferencialmente dentro de `<turn_context>`, usando um bloco `<surface_context>`.

Exemplo:

```xml
<surface_context surface_type="editor" surface_id="tab-3" title="slides.md" mode="reveal" snapshot_version="editor:tab-3:42">
  <focus kind="slide" slide_index="2">Objetivo</focus>
  <selection kind="text" range="12:1-18:1">
## Objetivo
...
  </selection>
</surface_context>
```

Regras de segurança:

- atributos derivados do contrato devem usar `snake_case` previsível no XML do prompt, como `surface_type`, `surface_id` e `snapshot_version`;
- renderizar por allowlist de `surfaceType` e campos permitidos;
- escapar conteúdo textual para não quebrar tags do prompt;
- truncar por campo e por bloco total, com limites configuráveis por provider/perfil;
- nunca renderizar objetos arbitrários de `metadata` sem allowlist;
- registrar quando campos foram omitidos ou truncados, de forma visível ao modelo;
- classificar o bloco como contexto dinâmico do turno, não como instrução estável.

`surfaceContextJson` pode continuar como campo de transporte durante a migração, mas o renderer backend deve normalizar para `SurfaceContext` antes de montar o prompt. Payloads legados sem `surfaceType`, `surfaceId` ou `snapshotVersion` devem ser tratados como contexto incompleto e não como alvo confiável para tools.

### 4. Especialização do editor

O editor deve preencher o envelope comum e especializar:

- seleção textual com range, texto selecionado e linguagem;
- cursor/foco com linha, coluna, símbolo ou seção quando disponível;
- caminho do arquivo em `metadata.filePath`, preservando a necessidade de tools de arquivo;
- modo `markdown`, `rich`, `preview` ou `reveal`;
- para Reveal, slide atual, contagem de slides, label acessível do slide e Markdown do slide atual quando dentro do orçamento;
- versão do snapshot derivada do estado do buffer, arquivo ou draft.

O `tabType` continua sendo `editor` para apresentações Reveal. Reveal é modo especializado do editor, conforme AEP-0079, não uma nova surface.

### 5. Especialização de tasklists

Tasklists devem preencher:

- lista ativa (`taskListId`, slug e título quando permitido);
- modo `list` ou `kanban`;
- seleção/foco de card, coluna/status, linha da grid ou agrupamento;
- status atual e coluna de origem/destino quando relevante;
- resumo curto da lista e do card selecionado;
- versão do snapshot baseada em `updatedAt`, versão de workflow ou contador de atualização da lista.

Quando uma tool for alterar card, status, nota ou coluna, o alvo deve ser explícito: `taskListId`, `taskId` e, quando aplicável, `statusId`/coluna. Se o contexto estiver stale, a tool deve recusar, revalidar ou pedir confirmação, conforme criticidade da ação.

### 6. Especialização do terminal

Terminal deve preencher:

- `cwd` e shell atual;
- input atual, se o usuário estiver compondo um comando;
- seleção de output, quando houver;
- output recente limitado, com indicação de truncamento;
- exit code/estado do último comando quando disponível;
- versão do snapshot associada à sessão e ao offset do buffer.

O backend deve aplicar limites mais conservadores ao terminal, porque output pode conter segredos, logs grandes ou dados sensíveis. O prompt deve deixar claro se o output é seleção explícita do usuário ou apenas janela recente.

### 7. Alvos explícitos para tools

Tools que atuam sobre surfaces não devem depender apenas do texto do usuário nem de "surface ativa" implícita.

Contrato alvo conceitual:

```json
{
  "target": {
    "surfaceType": "tasklist",
    "surfaceId": "tab-7",
    "snapshotVersion": "tasklist:abc:18",
    "entity": { "taskListId": "tl_abc", "taskId": "task_123" }
  },
  "operation": "update_status",
  "args": { "statusId": 3 }
}
```

Regras:

- toda tool com efeito sobre editor, tasklist ou terminal deve receber alvo estruturado ou resolver alvo por uma etapa explícita de leitura/seleção;
- `snapshotVersion` deve ser validado antes de ações destrutivas ou sensíveis;
- se a versão divergir, a tool deve retornar erro recuperável com instrução para recapturar contexto;
- ações de leitura podem tolerar staleness maior, mas devem informar a idade/versão quando isso afetar a resposta;
- a UI deve conseguir mostrar ao usuário qual alvo será afetado antes de aplicar mudanças relevantes.

### 8. Acessibilidade e transparência

Quando o contexto selecionado/focado for relevante para a ação, o usuário deve conseguir entender qual alvo está sendo usado.

Regras:

- elementos de UI que disparam chat contextual devem expor nome acessível com o alvo, como arquivo, slide, card, coluna ou terminal;
- mudanças de alvo/foco relevantes devem ser percebidas por teclado e leitor de tela, usando padrões existentes de foco e announcer global;
- menus e botões icon-only precisam de `aria-label` específico, não genérico;
- o prompt/renderização deve preferir rótulos humanos em `title`, `label` e `focus`, sem depender apenas de IDs;
- quando uma ação proposta afetar uma seleção, card ou arquivo, a UI deve apresentar esse alvo de forma textual, não apenas por cor ou posição visual.

### 9. Compatibilidade e migração

Não há obrigação de preservar bugs de contexto ambíguo. Se uma ação antes funcionava por inferência frágil, o comportamento correto é exigir alvo explícito, recapturar contexto ou pedir confirmação.

Compatibilidade permitida:

- manter `surfaceContextJson` como transporte temporário;
- adaptar payloads legados para o novo envelope quando houver dados suficientes;
- manter campos especializados existentes, como `activeFilePath`, enquanto tools de arquivo dependerem deles;
- aceitar ausência de especializações em surfaces ainda não migradas, desde que não sejam tratadas como alvos confiáveis para mutação.

Compatibilidade não permitida:

- criar wrappers alternativos de envio de mensagem por surface;
- injetar contexto sintético no texto do usuário como substituto do contrato;
- inferir `surfaceId` a partir de aba ativa quando a origem real do envio foi outra;
- permitir tool mutável operar sobre alvo ambíguo sem validação.

## Estado implementado e pendências

Entregue:

- [x] Normalização do envelope e adaptação de payloads legados em
  `internal/workspace/context_provider.go`.
- [x] Renderização allowlisted de `<surface_context>`, escaping, limites por
  campo/bloco e avisos de truncamento no mesmo provider.
- [x] Fallback seguro: payload incompleto recebe identidade legada e aviso de que
  não é alvo confiável para mutação; blocos malformados ou sem atributos
  obrigatórios são omitidos.
- [x] Testes de renderização, allowlist, legado, orçamento, truncamento,
  seleção mínima e fallback em `internal/workspace/context_provider_test.go`.

Pendente:

- [ ] Provider completo de tasklists com card/coluna/seleção e target mutável.
- [ ] Provider completo de terminal com input, seleção, output recente e limites
  próprios de dados sensíveis.
- [ ] Validação efetiva de `snapshotVersion`/staleness nas tools.
- [ ] Migração das mutações de editor, tasklist e terminal para targets
  estruturados, com erro recuperável e transparência acessível.

## Fases

### Fase 1 — AEP e contrato ✅

- Revisar e aceitar esta AEP.
- Definir tipos conceituais de `SurfaceContext`, `SurfaceSelection`, `SurfaceFocus` e `SurfaceContent`.
- Mapear payloads legados de `surfaceContextJson` para o envelope novo.
- Não alterar comportamento funcional neste PR de AEP.

### Fase 2 — Renderer backend genérico ✅

- Criar normalização backend para `SurfaceContext`.
- Renderizar `<surface_context>` por allowlist, escaping e truncamento.
- Adicionar testes de renderização, truncamento e payload malformado.
- Classificar o bloco como contexto dinâmico do turno.

### Fase 3 — Editor 🚧

- Implementar provider do editor.
- Cobrir seleção, cursor, arquivo, Markdown e modo Reveal.
- Validar staleness antes de ações de edição/aplicação de patch.
- Garantir transparência do alvo em UI e anúncios quando relevante.

### Fase 4 — Tasklists ⏳

- Implementar provider de tasklists.
- Cobrir lista, card, status/coluna, seleção e modos list/kanban.
- Exigir target explícito para tools mutáveis de cards/status/notas.
- Validar versionamento de lista/workflow antes de mutações.

### Fase 5 — Terminal ⏳

- Implementar provider do terminal.
- Cobrir input, seleção de output, output recente, cwd e shell.
- Aplicar limites conservadores e indicação de truncamento.
- Distinguir seleção explícita de janela recente.

### Fase 6 — Tools com alvo explícito ⏳

- Atualizar tools afetadas para receber target estruturado.
- Padronizar erro recuperável para snapshot stale.
- Expor leitura/inspeção de surface quando o alvo precisar ser resolvido antes da mutação.
- Remover dependências de inferência ambígua após migração.

## Riscos

- O envelope comum pode ficar genérico demais e esconder diferenças importantes entre surfaces. Mitigação: manter campos comuns pequenos e especializações por allowlist.
- Truncamento agressivo pode remover contexto útil. Mitigação: limites por campo, sinalização explícita de truncamento e tools de leitura sob demanda.
- Versionamento/staleness pode bloquear ações legítimas em flows rápidos. Mitigação: política diferenciada entre leitura, edição reversível e mutação destrutiva.
- Durante a migração, payloads legados podem parecer confiáveis. Mitigação: normalizador marca contexto incompleto e tools mutáveis exigem `surfaceType`, `surfaceId` e `snapshotVersion`.
- Terminal pode vazar dados sensíveis. Mitigação: limites conservadores, preferência por seleção explícita e allowlist rigorosa.
- A UI pode não deixar claro o alvo usado pelo assistente. Mitigação: critérios de acessibilidade/transparência como requisito de aceite, não refinamento opcional.

## Critérios de aceitação

- [x] Existe contrato documentado de `SurfaceContext` com campos comuns e semântica de staleness.
- [x] O backend normaliza `surfaceContextJson` para `SurfaceContext` ou marca payload legado como incompleto.
- [x] O prompt renderiza `<surface_context>` com allowlist, escaping, truncamento e indicação de campos omitidos/truncados.
- [ ] Editor envia seleção, foco/cursor, arquivo, modo Markdown e modo Reveal sem criar fluxo paralelo de mensagens.
- [ ] Tasklists enviam lista, card, status/coluna, seleção e modo de visualização com alvo explícito para mutações.
- [ ] Terminal envia cwd, shell, input, seleção de output e output recente com limites conservadores.
- [ ] Tools mutáveis validam `surfaceType`, `surfaceId` e `snapshotVersion` antes de agir sobre editor, tasklist ou terminal.
- [ ] Contexto stale gera erro recuperável, recaptura ou confirmação, conforme criticidade da ação.
- [ ] A UI comunica o alvo selecionado/focado de forma acessível quando ele influencia a ação.
- [ ] Não há novo fluxo de envio de mensagem nem mensagens locais no frontend.
- [ ] Compatibilidade com `surfaceContextJson` existe apenas como transporte/migração, não como justificativa para manter contexto ambíguo.

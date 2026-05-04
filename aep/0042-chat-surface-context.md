# AEP-0042: Chat Surface Context

## Status: 🚧 In Progress

> Nota de contrato vigente: após os PRs #110 e #111, esta AEP deve ser lida junto com as AEPs 0056, 0057 e 0058. `surfaceContext` descreve dados transitórios da interação, mas a identidade da superfície não é inferida por aba ativa: ela viaja explicitamente por `ChatSurfaceOrigin` e pelos campos estruturados de `ChatParams`.

## Resumo

Formalizar um contrato único para superfícies de chat ligadas ao workspace, mantendo:

- `WorkspaceTab.state` como fonte persistida e canônica do conteúdo da aba
- `conversationId` por aba como vínculo da conversa persistida
- `surfaceContext` como bloco transitório e derivado da superfície ativa

O objetivo é permitir que aba de chat, chat modal do editor, terminal e tasklist usem o mesmo pipeline de envio, sem injetar contexto artificial no texto do usuário.

---

## Motivação

O pipeline de envio já foi unificado por `conversationId`, mas ainda faltava formalizar **como a superfície ativa descreve seu contexto** para o backend, skills e ferramentas.

Problemas do estado anterior:

- O editor dependia de campos ad hoc como `activeFilePath`
- Partes do contexto eram empurradas para a mensagem do usuário
- Terminal e tasklist ainda precisavam anexar contexto de forma oportunista
- O contrato entre `WorkspaceTab.state` e o contexto do chat não estava explicitado

---

## Decisão

### 1. Contrato persistido continua no workspace

O conteúdo persistido da aba continua em `WorkspaceTab.state`.

Exemplos:

- Editor: `state.filePath`, `state.draftId`
- Terminal: `state.sessionId`
- Tasklist: `state.tasklistId`

Esse contrato é o que o workspace usa para reabrir, deduplicar e identificar conteúdo.

### 2. O chat recebe um contrato derivado

Toda superfície que envia mensagens ao chat passa a derivar dois blocos:

- `surfaceStateJson`: espelho serializado de `WorkspaceTab.state`
- `surfaceContextJson`: contexto transitório da interação atual

O campo `tabType` continua sendo o tipo canônico da superfície e permanece alinhado ao `type` da aba do workspace.

### 3. `surfaceContext` não substitui `state`

`surfaceContext` existe para dados efêmeros e sensíveis ao momento do envio, por exemplo:

- seleção atual do editor
- contexto de cursor
- resumo recente do terminal
- snapshot resumido da tasklist

Ele **não** deve carregar identidade persistida do conteúdo quando isso já existe em `WorkspaceTab.state`.

### 4. `filePath` continua existindo

No editor, `filePath` continua sendo necessário porque:

- ferramentas trabalham com caminho real em disco
- o `edit_file` precisa resolver o arquivo ativo
- skills podem referenciar o caminho ativo sem depender de texto artificial

Para documentos novos, o caminho do draft em disco continua sendo criado antes do primeiro envio. `draftId` segue como identificador de ciclo de vida do rascunho; `filePath` segue como referência material do arquivo.

---

## Contrato

### Frontend → backend (`ChatParams`)

```ts
type ChatParams = {
  tabType?: string
  activeFilePath?: string
  surfaceStateJson?: string
  surfaceContextJson?: string
  surfaceSessionKey?: string
  surfaceId?: string
  surfaceType?: string
  surfaceTabId?: string
}
```

### Estrutura lógica exposta às skills/templates

```ts
type SurfaceTemplateData = {
  type: string
  title?: string
  state?: Record<string, unknown>
  context?: Record<string, unknown>
}
```

### Regras

1. `tabType` deve conversar com `WorkspaceTab.type`.
2. `surfaceStateJson` deve conversar com `WorkspaceTab.state`.
3. `surfaceContextJson` é derivado por envio e não deve ser persistido no workspace.
4. Ferramentas continuam podendo usar campos especializados existentes, como `activeFilePath`, quando o dado é parte real do contexto da superfície.
5. `surfaceSessionKey`, `surfaceId`, `surfaceType` e `surfaceTabId` são identidade de origem; não devem ser reconstruídos a partir de `activeTabId`.
6. Perfil efetivo e origem de envio são calculados pela superfície chamadora e propagados como parâmetros estruturados.

---

## Estratégia de implementação

### Fase 1

- Adicionar `surfaceStateJson` e `surfaceContextJson` em `ChatParams`
- Expor `.Surface` nos templates de skill
- Migrar editor para preencher `surfaceContextJson`
- Manter `activeFilePath` para compatibilidade com `edit_file`

### Fase 2

- Terminal e tasklist passam a preencher `surfaceContextJson`
- Perfis/skills específicas consomem esse contexto estruturado
- Anexos sintéticos de contexto deixam de ser obrigatórios

### Fase 3

- Tools passam a poder ler `surfaceState` e `surfaceContext` diretamente do `InvocationContext`
- Ações especiais por superfície (editor, terminal, tasklist, futuras) usam o mesmo contrato base

### Consolidação posterior

- AEP-0056 define o workspace como shell fino e separa `activeTabId` de identidade de dados.
- AEP-0057 define `ChatSurfaceIdentity`, `ChatSurfaceOrigin` e sessões visuais por `sessionKey`.
- AEP-0058 define que announcer, TTS e STT continuam globais, mas recebem origem explícita da superfície.
- O PR #112 adiciona validação de regressão para origem em eventos, voz/anúncios e lifecycle de superfícies fechadas.

---

## Impacto esperado

- Remove a necessidade de poluir a mensagem do usuário com contexto sintético
- Mantém o workspace como dono da identidade persistida da aba
- Abre caminho para skills e tools entenderem a superfície ativa de modo reutilizável
- Evita criar um fluxo paralelo para cada painel

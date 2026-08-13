# Deep Links Internos (`assistente://`)

**Data**: 16 de março de 2026
**Atualizado**: 23 de março de 2026

---

## Visão Geral

O sistema de deep links permite criar links especiais dentro de mensagens do chat que, ao serem clicados, executam ações internas no app — como abrir uma conversa, navegar para uma página, criar abas no workspace ou enviar uma mensagem automaticamente.

Os links usam o protocolo customizado `assistente://` e são renderizados como **chips visuais** (pills) com ícone, diferenciando-se visualmente de links externos.

---

## Formato do Protocolo

```
assistente://{recurso}/{ação}[/{id}][?param=valor&...]
```

Parâmetros de query string devem ser codificados com `encodeURIComponent`.

### Conversas

| URI | Ação |
|-----|------|
| `assistente://conversation/{id}` | Abre/foca aba de chat existente |
| `assistente://conversation/new` | Cria nova conversa |
| `assistente://conversation/new?message={texto}&title={título}` | Cria nova conversa, envia mensagem e define título da aba |
| `assistente://conversation/{id}/send?message={texto}` | Envia mensagem em conversa existente |

### Abas do Workspace

| URI | Ação |
|-----|------|
| `assistente://tasklist/{id}` | Abre/foca aba de tasklist existente |
| `assistente://tasklist/new[?title=...]` | Cria nova tasklist e abre como aba |
| `assistente://editor/{id}` | Abre/foca aba do editor existente |
| `assistente://editor/new[?title=...]` | Cria novo documento vazio no editor |
| `assistente://editor/open?file={caminho}[&title=...]` | Abre arquivo existente no editor |
| `assistente://terminal/{id}` | Abre/foca aba de terminal existente |
| `assistente://terminal/new[?cmd=...&title=...]` | Cria nova sessão de terminal, opcionalmente executa comando |

### Navegação

| URI | Ação |
|-----|------|
| `assistente://navigate/{rota}` | Navega para uma página do app |

### Recursos Editáveis

| URI | Ação |
|-----|------|
| `assistente://{recurso}/new` | Abre formulário de criação do recurso |
| `assistente://{recurso}/edit/{id}` | Abre formulário de edição do recurso |

Recursos: `profiles`, `providers`, `credentials`, `allowlists`, `skills`, `mcp`, `channels`, `tasklists`

### Rotas Válidas para `navigate`

As telas de configuração são **abas** de `settings` (`settings/:tab?` no
roteador), então a rota inclui o prefixo: `settings/allowlists`, e não
`allowlists`. `VALID_ROUTES` em `frontend/src/lib/deepLinks.ts` é a fonte de
verdade — rota fora dela faz o parser devolver `null`.

| Rota | Página |
|------|--------|
| *(vazio)* | Chat (workspace) |
| `settings` | Configurações |
| `settings/providers` | Provedores |
| `settings/mcp` | Servidores MCP |
| `settings/skills` | Skills |
| `settings/channels` | Canais |
| `settings/contacts` | Contatos |
| `settings/credentials` | Credenciais |
| `settings/allowlists` | Allowlists de comandos |
| `settings/network-allowlist` | Allowlist de rede (hosts autorizados apesar do anti-SSRF) |
| `settings/appearance` | Aparência |
| `settings/data` | Dados |
| `settings/restore-defaults` | Restaurar padrões |
| `profiles` | Perfis |
| `history` | Histórico |
| `memories` | Memórias |
| `tasklists` | Listas de Tarefas |
| `help` | Ajuda |
| `about` | Sobre |
| `update` | Atualização |

> **Nota:** Terminal e editor não são páginas — são abas do workspace. Use `assistente://terminal/{id}` ou `assistente://editor/{id}` para abrir abas existentes, e `assistente://terminal/new` ou `assistente://editor/new` para criar novas.

---

## Uso em Markdown

Deep links usam a sintaxe padrão de links Markdown. O texto entre colchetes é livre — qualquer rótulo pode ser usado:

```markdown
[Abrir conversa](assistente://conversation/42)

[Analise o ticket XPTO](assistente://conversation/new?message=analise%20o%20ticket%20XPTO)

[Continuar análise](assistente://conversation/10/send?message=continue%20a%20análise)

[Ver histórico](assistente://navigate/history)

[Ir para configurações](assistente://navigate/settings)
```

### Renderização

Os links são renderizados como **chips** (pills) com:

- Fundo suave (`--accent-dim`) e borda (`--border-default`)
- Ícone automático antes do texto (baseado no tipo de ação)
- Hover com destaque (`--accent-hover`)
- Focus ring visível para navegação por teclado

Exemplo visual (tema escuro):

```
  💬 Abrir conversa       ➕ Analise o ticket XPTO       ➡ Ver histórico
  ╰─ chip clicável ─╯     ╰──── chip clicável ────╯     ╰─ chip clicável ─╯
```

---

## API Programática

O módulo `frontend/src/lib/deepLinks.ts` exporta funções utilitárias para trabalhar com deep links no código:

### `isDeepLink(href: string): boolean`

Verifica se uma string é um deep link `assistente://`.

```typescript
isDeepLink('assistente://conversation/42');  // true
isDeepLink('https://google.com');            // false
```

### `parseDeepLink(uri: string): DeepLinkAction | null`

Faz parse de uma URI e retorna um objeto tipado, ou `null` se inválido.

```typescript
parseDeepLink('assistente://conversation/42');
// → { type: 'conversation:open', conversationId: 42 }

parseDeepLink('assistente://conversation/new?message=oi&title=Teste');
// → { type: 'conversation:new', message: 'oi', title: 'Teste' }

parseDeepLink('assistente://tasklist/5');
// → { type: 'tab:open', tabType: 'tasklist', contentId: '5' }

parseDeepLink('assistente://terminal/new?cmd=npm+install');
// → { type: 'tab:new', tabType: 'terminal', cmd: 'npm install' }

parseDeepLink('assistente://editor/open?file=/tmp/test.md');
// → { type: 'tab:new', tabType: 'editor', file: '/tmp/test.md' }

parseDeepLink('assistente://profiles/edit/programacao');
// → { type: 'resource:edit', resource: 'profiles', resourceId: 'programacao' }

parseDeepLink('https://google.com');
// → null
```

### `buildDeepLink(action: DeepLinkAction): string`

Constrói uma URI a partir de um objeto de ação. Útil para gerar links programaticamente.

```typescript
buildDeepLink({ type: 'conversation:open', conversationId: 42 });
// → 'assistente://conversation/42'

buildDeepLink({ type: 'tab:new', tabType: 'terminal', cmd: 'npm install' });
// → 'assistente://terminal/new?cmd=npm+install'

buildDeepLink({ type: 'resource:new', resource: 'tasklists' });
// → 'assistente://tasklists/new'
```

### `executeDeepLink(action: DeepLinkAction, deps: DeepLinkDeps): Promise<void>`

Executa a ação do deep link. Requer `{ navigate }` do React Router.

### `getDeepLinkLabel(action: DeepLinkAction): string`

Retorna um label legível e internacionalizado para a ação (usado em `aria-label`).

### `getDeepLinkTypeClass(action: DeepLinkAction): string`

Retorna a classe CSS específica do tipo (ex: `deep-link--conversation`).

### Tipo `DeepLinkAction`

```typescript
type TabType = 'tasklist' | 'editor' | 'terminal';

type DeepLinkAction =
  | { type: 'conversation:open'; conversationId: number; title?: string }
  | { type: 'conversation:new'; message?: string; title?: string }
  | { type: 'conversation:send'; conversationId: number; message: string }
  | { type: 'navigate'; route: string }
  | { type: 'resource:edit'; resource: EditableResource; resourceId: string }
  | { type: 'resource:new'; resource: EditableResource }
  | { type: 'tab:open'; tabType: TabType; contentId: string; title?: string }
  | { type: 'tab:new'; tabType: TabType; title?: string; file?: string; cmd?: string };
```

---

## Arquitetura

```
Mensagem Markdown
       │
       ▼
  markdown-it  ──────► markdownItDeepLink plugin
       │                  (adiciona classes, data-deep-link, aria-label)
       ▼
  DOMPurify  ─────────► ALLOWED_URI_REGEXP inclui assistente://
       │                  (tabindex=0, sem target=_blank para deep links)
       ▼
  MarkdownRenderer
       │
       ├─ click handler ──► parseDeepLink() ──► executeDeepLink()
       │                                              │
       │                           ┌──────────────────┼───────────────────┐
       │                           ▼                  ▼                   ▼
       │                     workspaceStore      navigate()          announce()
       │                     chatStore           navigationStore
       │                     editorStore
       │
       └─ keydown handler (Enter/Space) ──► mesmo fluxo

  Backend (Go)
       │
  open_deep_link tool ──► EmitDeepLink(uri) ──► evento "deeplink:execute"
       │                                              │
       ▼                                              ▼
  Validação prefixo                            App.tsx EventsOn
  assistente://                                parseDeepLink → executeDeepLink
```

### Arquivos Envolvidos

| Arquivo | Responsabilidade |
|---------|------------------|
| `frontend/src/lib/deepLinks.ts` | Parser, builder, executor, tipos |
| `frontend/src/lib/markdownItDeepLink.ts` | Plugin markdown-it |
| `frontend/src/components/ui/MarkdownRenderer.tsx` | Registro do plugin, DOMPurify config, handlers de click/keyboard |
| `frontend/src/components/ui/MarkdownRenderer.css` | Estilos dos chips `.deep-link` |
| `frontend/src/locales/{pt-BR,en,es}.ts` | Traduções (namespace `deepLink`) |
| `frontend/src/store/navigationStore.ts` | Store de pending edit/new para recursos editáveis |
| `frontend/src/hooks/useResourceEditRequest.ts` | Hook consumido pelas páginas de recurso |
| `internal/tools/deeplink/open_deep_link.go` | Tool Go para o agente emitir deep links |
| Context provider/tooling de workspace | Documentação operacional de deep links para o agente. Historicamente ficava em `builtin/skills/workspace/SKILL.md`, removido pela migração de workspace para Context Provider. |

### Testes

| Arquivo | Testes | Cobertura |
|---------|--------|-----------|
| `frontend/src/lib/deepLinks.test.ts` | 105 | Parser, builder, roundtrip, isDeepLink, classes CSS, executor (todos os action types), validações de segurança |
| `frontend/src/lib/markdownItDeepLink.test.ts` | 12 | Plugin: classes, atributos, ARIA, mix com links normais, URIs inválidos |
| `internal/tools/deeplink/open_deep_link_test.go` | 5 | Validação de prefixo, URIs válidas/inválidas, JSON malformado |

---

## Segurança

- **Validação rigorosa**: IDs devem ser inteiros positivos; rotas são validadas contra lista fixa
- **DOMPurify**: apenas o protocolo `assistente://` é adicionado; outros protocolos custom continuam bloqueados
- **Sem `target="_blank"`**: deep links não abrem janelas externas
- **Mensagens sanitizadas**: o conteúdo do parâmetro `message` já passa pela sanitização do DOMPurify antes de ser renderizado

---

## Acessibilidade

- **`role="link"`** e **`aria-label`** descritivo (ex: "Abrir conversa #42") em cada deep link
- **`tabindex="0"`**: deep links são navegáveis por Tab (diferente de links normais que usam `tabindex="-1"`)
- **Teclado**: Enter e Espaço ativam o deep link
- **`focus-visible`**: anel de foco visível para navegação por teclado
- **`announce()`**: após executar a ação, um anúncio é feito via `useAnnouncer` para leitores de tela

---

## Extensibilidade

O sistema foi projetado para ser facilmente extensível. Para adicionar um novo tipo de deep link:

1. Adicionar o tipo em `DeepLinkAction` em `deepLinks.ts`
2. Adicionar o parsing em `parseDeepLink()`
3. Adicionar a construção em `buildDeepLink()`
4. Adicionar o label em `getDeepLinkLabel()`
5. Adicionar a execução em `executeDeepLink()`
6. Adicionar a classe CSS em `getDeepLinkTypeClass()` e os estilos em `MarkdownRenderer.css`
7. Adicionar as chaves i18n nos 3 locales
8. Atualizar a `Description()` em `internal/tools/deeplink/open_deep_link.go`
9. Atualizar a documentação operacional do provider/tooling de workspace, quando houver impacto para o agente

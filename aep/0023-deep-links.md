# Deep Links Internos (`assistente://`)

**Data**: 16 de março de 2026

---

## Visão Geral

O sistema de deep links permite criar links especiais dentro de mensagens do chat que, ao serem clicados, executam ações internas no app — como abrir uma conversa, navegar para uma página ou enviar uma mensagem automaticamente.

Os links usam o protocolo customizado `assistente://` e são renderizados como **chips visuais** (pills) com ícone, diferenciando-se visualmente de links externos.

---

## Formato do Protocolo

```
assistente://{recurso}/{ação}[/{id}][?param=valor&...]
```

Parâmetros de query string devem ser codificados com `encodeURIComponent`.

### Ações Disponíveis

| URI | Ação | Ícone |
|-----|------|-------|
| `assistente://conversation/{id}` | Abre conversa existente em nova aba | 💬 |
| `assistente://conversation/new` | Cria nova conversa | ➕ |
| `assistente://conversation/new?message={texto}` | Cria nova conversa e envia mensagem | ➕ |
| `assistente://conversation/new?message={texto}&title={título}` | Idem, com título para a aba | ➕ |
| `assistente://conversation/{id}/send?message={texto}` | Envia mensagem em conversa existente | 📨 |
| `assistente://navigate/{rota}` | Navega para uma página do app | ➡ |

### Rotas Válidas para `navigate`

| Rota | Página |
|------|--------|
| *(vazio)* | Chat |
| `terminal` | Terminal |
| `editor` | Editor |
| `allowlists` | Allowlists |
| `skills` | Skills |
| `mcp` | Servidores MCP |
| `channels` | Canais |
| `credentials` | Credenciais |
| `providers` | Provedores |
| `settings` | Configurações |
| `profiles` | Perfis |
| `history` | Histórico |
| `help` | Ajuda |
| `about` | Sobre |
| `update` | Atualização |

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

parseDeepLink('assistente://conversation/10/send?message=continue');
// → { type: 'conversation:send', conversationId: 10, message: 'continue' }

parseDeepLink('assistente://navigate/history');
// → { type: 'navigate', route: 'history' }

parseDeepLink('https://google.com');
// → null
```

### `buildDeepLink(action: DeepLinkAction): string`

Constrói uma URI a partir de um objeto de ação. Útil para gerar links programaticamente.

```typescript
buildDeepLink({ type: 'conversation:open', conversationId: 42 });
// → 'assistente://conversation/42'

buildDeepLink({ type: 'conversation:new', message: 'analise o ticket #123' });
// → 'assistente://conversation/new?message=analise+o+ticket+%23123'
```

### `executeDeepLink(action: DeepLinkAction, deps: DeepLinkDeps): Promise<void>`

Executa a ação do deep link. Requer `{ navigate }` do React Router.

### `getDeepLinkLabel(action: DeepLinkAction): string`

Retorna um label legível e internacionalizado para a ação (usado em `aria-label`).

### `getDeepLinkTypeClass(action: DeepLinkAction): string`

Retorna a classe CSS específica do tipo (ex: `deep-link--conversation`).

### Tipo `DeepLinkAction`

```typescript
type DeepLinkAction =
  | { type: 'conversation:open'; conversationId: number }
  | { type: 'conversation:new'; message?: string; title?: string }
  | { type: 'conversation:send'; conversationId: number; message: string }
  | { type: 'navigate'; route: string };
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
       │                                    ┌─────────┼──────────┐
       │                                    ▼         ▼          ▼
       │                              chatStore   navigate()  announce()
       │
       └─ keydown handler (Enter/Space) ──► mesmo fluxo
```

### Arquivos Envolvidos

| Arquivo | Responsabilidade |
|---------|------------------|
| `frontend/src/lib/deepLinks.ts` | Parser, builder, executor, tipos |
| `frontend/src/lib/markdownItDeepLink.ts` | Plugin markdown-it |
| `frontend/src/components/ui/MarkdownRenderer.tsx` | Registro do plugin, DOMPurify config, handlers de click/keyboard |
| `frontend/src/components/ui/MarkdownRenderer.css` | Estilos dos chips `.deep-link` |
| `frontend/src/locales/{pt-BR,en,es}.ts` | Traduções (namespace `deepLink`) |

### Testes

| Arquivo | Testes | Cobertura |
|---------|--------|-----------|
| `frontend/src/lib/deepLinks.test.ts` | 37 | Parser, builder, roundtrip, isDeepLink, classes CSS, validações de segurança |
| `frontend/src/lib/markdownItDeepLink.test.ts` | 12 | Plugin: classes, atributos, ARIA, mix com links normais, URIs inválidos |

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
4. Adicionar a execução em `executeDeepLink()`
5. Adicionar a classe CSS em `getDeepLinkTypeClass()` e os estilos em `MarkdownRenderer.css`
6. Adicionar as chaves i18n nos 3 locales

Exemplos de extensões futuras:

```
assistente://profile/{slug}          → Abrir editor de perfil
assistente://skill/{slug}            → Abrir editor de skill
assistente://editor/open?file={path} → Abrir arquivo no editor
assistente://mcp/{server}            → Abrir configuração de servidor MCP
```

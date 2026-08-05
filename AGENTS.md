# Regras do projeto (assistente)

Este arquivo é a fonte única de regras para TODOS os agentes de código
(Claude Code, Cursor, GitHub Copilot, Codex, OpenCode, etc.). Não crie
arquivos de regras paralelos por ferramenta; alterações de regra acontecem
aqui. `CLAUDE.md` apenas importa este arquivo e
`.github/copilot-instructions.md` apenas aponta para ele.

## Idioma
- Sempre responder em Português (pt-BR).

## Fluxo de entrega (OBRIGATÓRIO para features e correções)

Toda feature ou correção não-trivial segue este fluxo, sem exceções:

1. **Consulte os AEPs** relevantes em `aep/` antes de implementar (veja a seção AEPs).
2. **Isole o trabalho**: crie um git worktree próprio com branch descritiva a partir de `origin/main` (ex.: `git worktree add ../assistente-wt-<tema> -b <tipo>/<tema> origin/main`). Não trabalhe direto na `main`.
3. **Commits temáticos em português**, um assunto por commit.
4. **Valide localmente antes do push**: backend `go build ./...`, `go vet ./...`, `go test ./...` (e `golangci-lint run` se disponível); frontend `tsc --noEmit`, eslint, stylelint e `npm test` (vitest).
5. **Abra um PR para `main`** com título e corpo em português (motivação, mudanças, testes). Um PR por assunto; prefira PRs pequenos. Se um PR depende de outro ainda aberto, empilhe (base = branch do PR dependido) e informe a ordem de merge no corpo.
6. **Acompanhe o CI até ficar verde** (`gh pr checks <n> --watch`). Corrija falhas e repita.
7. **Loop de review até zerar**: a review do Copilot é solicitada automaticamente (ruleset do repositório, incluindo novos pushes). Se não houver review automática, solicite via `gh api repos/<owner>/<repo>/pulls/<n>/requested_reviewers -X POST -f "reviewers[]=copilot-pull-request-reviewer[bot]"`. Para cada comentário: corrija (ou justifique tecnicamente se for falso positivo — de preferência com teste que prove), responda a thread e marque como resolvida. Repita até **zero threads não resolvidas com CI verde**.
8. **Nunca faça merge do PR.** O merge é decisão do dono do projeto.
9. Ao final, reporte: número/URL do PR, o que mudou, resultado de testes/checks e rodadas de review.

## Testes (não deixar feature descoberta)
- Sempre verificar se a mudança/feature está coberta por testes.
  - Frontend: preferir **Vitest** para utilitários, hooks e componentes.
  - Backend: preferir `go test ./...` com testes de unidade/integração onde fizer sentido.
- Se não houver teste cobrindo o comportamento alterado (ou se a cobertura estiver fraca), criar/atualizar testes junto com a mudança.
- Ao adicionar testes, manter o escopo focado na feature tocada (evitar "mass refactor" de testes).

### Regras rígidas
- Nunca deletar testes para "resolver" falhas.
- Nunca simplificar testes para facilitar aprovação; corrigir o código é sempre o caminho.

## Logs (performance + ruído)
- Evite adicionar `console.log` em código de produção.
- Ao mexer em um recurso/arquivo, remova `console.log` existentes no trecho tocado.
- `console.warn` e `console.error` são permitidos quando representam condição anômala real.
- Se for indispensável logar para diagnóstico, prefira log temporário removido antes do merge, ou log protegido por flag (ex.: `import.meta.env.DEV` + toggle em `localStorage`).

## Escopo de mudanças
- Não faça "varreduras globais" (ex.: remoção de logs, renomeações em massa) sem necessidade do PR; aplique limpezas incrementalmente quando o recurso for alterado.

## Sistema de Temas e Cores (OBRIGATÓRIO)

O app usa um sistema centralizado de design tokens definido em `frontend/src/theme.css`.

### Regras absolutas
- **NUNCA** use cores hardcoded (`#hex`, `rgb()`, `rgba()`, nomes de cor) em arquivos CSS ou inline styles.
- **SEMPRE** use variáveis CSS do tema. Consulte `frontend/src/theme.css` para a lista completa.
- **NUNCA** use `@media (prefers-color-scheme: dark/light)` — o sistema de temas usa `data-theme` no `<html>`.

### Variáveis principais

| Categoria | Variáveis |
|---|---|
| Backgrounds | `--bg-base`, `--bg-surface`, `--bg-elevated`, `--bg-hover`, `--bg-muted`, `--bg-input`, `--bg-overlay` |
| Texto | `--text-primary`, `--text-secondary`, `--text-muted`, `--text-inverse`, `--text-code` |
| Bordas | `--border-subtle`, `--border-default`, `--border-strong` |
| Acento | `--accent`, `--accent-hover`, `--accent-dim`, `--accent-strong` |
| Semântico | `--color-success`, `--color-warning`, `--color-danger`, `--color-info` (cada um com `-hover` e `-dim`) |
| Foco | `--focus-ring`, `--color-focus` |
| Raio | `--radius-sm` (4px), `--radius-md` (6px), `--radius-lg` (8px), `--radius-xl` (12px) |

### Temas disponíveis
- **Assistente** (padrão) — escuro azul vibrante
- **Ametista** — escuro violeta
- **Meia-Noite** — escuro cinza-azulado
- **Claro** — fundo branco com acentos azuis
- **Alto Contraste** — máximo contraste para acessibilidade

Para gerenciar temas no código: `useTheme()` de `frontend/src/hooks/useTheme.ts`.

## Acessibilidade (NUNCA negligenciar)

A acessibilidade é um requisito fundamental do projeto, não opcional. O
mantenedor usa NVDA; toda decisão de UI deve funcionar por teclado e leitor
de telas.

### Contraste WCAG 2.1
- `--text-primary` sobre `--bg-surface` ≥ 12:1 (AAA)
- `--text-secondary` sobre `--bg-surface` ≥ 6:1 (AA)
- `--text-muted` sobre `--bg-surface` ≥ 4.5:1 (AA)
- Ao adicionar cores, sempre verificar contraste suficiente

### Navegação por teclado
- Todo elemento interativo DEVE ser acessível por Tab
- Menus: setas para navegar, Enter para selecionar, ESC para fechar
- Foco DEVE ser restaurado ao fechar modais/menus

### ARIA e Leitores de tela
- Usar `announce()` via `useAnnouncer` para feedback de ações
- Labels obrigatórios: `htmlFor` em `<label>`, `aria-label` em botões sem texto
- Nunca usar apenas cor para transmitir informação
- Modais de leitura pesada usam `readingMode` (`role="document"`) no `Modal`; modais de formulário mantêm `role="application"`

### Componentes reutilizáveis
Sempre usar componentes existentes em `frontend/src/components/ui/`:
- `DataGrid` para tabelas (já tem role="grid" e navegação por teclado)
- `Modal` para diálogos em geral (focus trap, ESC, aria-hidden); `ConfirmDialog` (wrapper sobre o `Modal`) para confirmações
- `Button` para botões (variantes: primary, secondary, danger, ghost, outline)
- `Toolbar` para barras de ferramentas (ARIA toolbar)

## Stack Técnica
- **Backend**: Go + Wails v2
- **Frontend**: React + TypeScript + Vite
- **Estado**: Zustand (stores em `frontend/src/store/`)
- **Comunicação Frontend↔Backend**: Funções Wails em `wailsjs/go/app/App`
- **Testes**: Vitest (frontend), `go test` (backend)
- **i18n**: react-i18next (locales em `frontend/src/locales/`)

## i18n (Internacionalização — OBRIGATÓRIO)

Todas as strings visíveis ao usuário DEVEM ser internacionalizadas.

- Sempre usar `useTranslation()` + `t('namespace.key')` via react-i18next
- NUNCA hardcode strings em qualquer idioma diretamente no JSX
- Idiomas suportados: `pt-BR`, `en`, `es`
- Locales: `frontend/src/locales/pt-BR.ts`, `en.ts`, `es.ts`
- Config: `frontend/src/lib/i18n.ts`
- Detecção automática do idioma da máquina, fallback para inglês
- Ao criar nova string de UI, adicionar a chave nos 3 arquivos de locale
- Troca de idioma via menu principal, persistida com Zustand

## Messaging — Arquitetura backend-driven (PROIBIÇÕES)

O sistema de envio/recebimento de mensagens segue uma arquitetura backend-driven (AEP-0040).

### Regras absolutas
- **NUNCA** crie fluxo alternativo de envio de mensagens. Existe UMA única `SendMessage` no backend (`app_chat.go`) para mensagens novas e `RetryMessage` para retry explícito. No frontend, componentes e controllers por aba/conversa devem reutilizar o contrato/pipeline compartilhado de envio; não duplique validação, serialização, parâmetros ou chamada ao backend em wrappers divergentes.
- **NUNCA** crie mensagens locais no frontend. O frontend não gera IDs temporários, não insere mensagens otimistas, não cria placeholders. Só renderiza o que o backend emite via eventos.
- **Mensagens só para conversas existentes.** `SendMessage` com `conversationID=0` ou inexistente retorna erro. Criação de conversa é responsabilidade separada.
- **Todo evento de chat carrega `conversationId`.** Sem exceções. Eventos são tipados com structs Go e interfaces TypeScript.
- **Conversas são independentes de abas.** Existem no banco sem vínculo com UI. Canais criam e mantêm conversas sem abas.
- **Controllers por aba/conversa são permitidos.** Cada controller deve filtrar eventos por `conversationId`, manter apenas seu estado visual próprio e delegar envio/retry ao contrato compartilhado.
- **Announcer, TTS e STT são serviços globais arbitrados.** Não crie múltiplas live regions por aba; TTS não pode falar em paralelo; STT local só pode escutar a aba ativa.
- **Protocolo de eventos é contrato central.** O backend usa eventos para orquestrar TTS, rename, notificação de canais. Alterar schema de evento exige atualizar todos os consumidores.

### Referência
- Detalhes completos no AEP de backend-driven-messaging em `aep/`.

## AEPs — Architecture Evolution Proposals (OBRIGATÓRIO)

O diretório `aep/` é o repositório único de decisões arquiteturais do projeto. Contém documentos numerados que definem contratos, protocolos, decisões de design e planos de evolução.

### Regras absolutas
- **NUNCA** crie outro diretório para AEPs (ex.: `aeps/`, `docs/aep/`, `proposals/`). Tudo fica em `aep/`.
- **Antes de implementar qualquer feature significativa**, consulte os AEPs relevantes em `aep/` para verificar se já existe decisão arquitetural sobre o tema.
- **O código DEVE estar alinhado com os AEPs.** Se encontrar divergência entre um AEP e o código:
  1. NÃO assuma que o código está certo e o AEP desatualizado.
  2. Pergunte ao usuário: "O AEP `aep/XXXX` diz X, mas o código faz Y. O AEP precisa ser atualizado ou o código precisa ser corrigido?"
  3. Só prossiga após confirmação.
- **Ao criar novo AEP**, numere sequencialmente a partir do último existente (consulte `aep/` para o maior número).
- **Formato**: Markdown, em português, com seções: Resumo, Motivação, Decisões, Fases, Riscos, Critérios de aceitação.
- **Para descobrir AEPs relevantes**, liste `aep/` e leia os títulos — os nomes dos arquivos descrevem o tema.

## Enforcement Automatizado (CI)

Todo PR roda automaticamente, inclusive o empilhado (aquele cuja base é a
branch de outro PR):
- **Go**: build, vet e testes (incluindo job com detector de corrida)
- **TypeScript**: `tsc --noEmit`
- **ESLint** com `jsx-a11y`: detecta ARIA inválido, roles ausentes
- **Stylelint**: impede cores e font-sizes hardcoded (deve usar tokens do `theme.css`)
- **Vitest** com `axe-core`: testes de acessibilidade nos componentes UI
- **E2E**: Playwright

### O que o CI bloqueia
- Cores hardcoded (#hex, rgb, rgba) em CSS — use variáveis do tema
- Atributos ARIA inválidos — use os padrões documentados
- Componentes sem labels de acessibilidade
- Testes falhando (incluindo testes axe-core)

### A main descendo nos PRs abertos
Check verde envelhece: ele conta o encontro entre a branch e a main de quando
rodou. Por isso, todo push na `main` dispara o workflow `Atualizar PRs com a
main`, que mescla a main em cada PR aberto (fora rascunhos e forks) e pede o CI
de novo. Quando o merge conflita, o PR é listado no resumo do run e a resolução
é sua — o workflow não tenta adivinhar.
### Checklist para novo código
- [ ] Strings visíveis usam `t('key', 'fallback')` nos 3 locales
- [ ] Ícones decorativos têm `aria-hidden="true"`
- [ ] Botões icon-only têm `aria-label`
- [ ] Cores vêm de variáveis CSS (`--bg-*`, `--text-*`, etc.)
- [ ] Font-sizes usam tokens (`--font-size-sm`, `--font-size-base`, etc.)
- [ ] Inputs/selects têm `height: 32px`, botões têm `min-height: 36px`

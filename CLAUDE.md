# Instruções para Claude Code (assistente)

## Idioma
- Sempre responder em Português (pt-BR).

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

A acessibilidade é um requisito fundamental do projeto, não opcional.

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

### Componentes reutilizáveis
Sempre usar componentes existentes em `frontend/src/components/ui/`:
- `DataGrid` para tabelas (já tem role="grid" e navegação por teclado)
- `Modal` / `SimpleModal` para diálogos (focus trap, ESC, aria-hidden)
- `Button` para botões (variantes: primary, secondary, danger, ghost, outline)
- `Toolbar` para barras de ferramentas (ARIA toolbar)

## Stack Técnica
- **Backend**: Go + Wails v2
- **Frontend**: React + TypeScript + Vite
- **Estado**: Zustand (stores em `frontend/src/store/`)
- **Comunicação Frontend↔Backend**: Funções Wails em `wailsjs/go/main/App`
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
- **NUNCA** crie uma nova função de envio de mensagens. Existe UMA única `SendMessage` no backend (`app_chat.go`) e UMA única `sendMessage` no frontend (`chatStore`). Se precisar customizar, use parâmetros — não crie wrappers ou métodos alternativos.
- **NUNCA** crie mensagens locais no frontend. O frontend não gera IDs temporários, não insere mensagens otimistas, não cria placeholders. Só renderiza o que o backend emite via eventos.
- **Mensagens só para conversas existentes.** `SendMessage` com `conversationID=0` ou inexistente retorna erro. Criação de conversa é responsabilidade separada.
- **Todo evento de chat carrega `conversationId`.** Sem exceções. Eventos são tipados com structs Go e interfaces TypeScript.
- **Conversas são independentes de abas.** Existem no banco sem vínculo com UI. Canais criam e mantêm conversas sem abas.
- **Protocolo de eventos é contrato central.** O backend usa eventos para orquestrar TTS, rename, notificação de canais. Alterar schema de evento exige atualizar todos os consumidores.

### Referência
- `aep/0040-backend-driven-messaging.md`

## AEPs — Architecture Evolution Proposals (OBRIGATÓRIO)

O diretório `aep/` é o repositório único de decisões arquiteturais do projeto. Contém 45+ documentos numerados que definem contratos, protocolos, decisões de design e planos de evolução.

### Regras absolutas
- **NUNCA** crie outro diretório para AEPs (ex.: `aeps/`, `docs/aep/`, `proposals/`). Tudo fica em `aep/`.
- **Antes de implementar qualquer feature significativa**, consulte os AEPs relevantes em `aep/` para verificar se já existe decisão arquitetural sobre o tema.
- **O código DEVE estar alinhado com os AEPs.** Se encontrar divergência entre um AEP e o código:
  1. NÃO assuma que o código está certo e o AEP desatualizado.
  2. Pergunte ao usuário: "O AEP `aep/XXXX` diz X, mas o código faz Y. O AEP precisa ser atualizado ou o código precisa ser corrigido?"
  3. Só prossiga após confirmação.
- **Ao criar novo AEP**, numere sequencialmente a partir do último existente (consulte `aep/` para o maior número).
- **Formato**: Markdown, em português, com seções: Resumo, Motivação, Decisões, Fases, Riscos, Critérios de aceitação.

### AEPs chave (consultar frequentemente)

| AEP | Tema |
|---|---|
| `aep/0040-backend-driven-messaging.md` | Protocolo de mensagens (contrato central) |
| `aep/0039-tool-calling-revamp.md` | Sistema de ferramentas |
| `aep/0045-cli-interface.md` | Interface CLI alternativa ao Wails |
| `aep/0034-unified-workspace.md` | Workspace unificado |
| `aep/0028-componentization.md` | Arquitetura de componentes frontend |
| `aep/0024-speech-architecture.md` | Arquitetura de fala (TTS/STT) |
| `aep/0012-llm-provider-manager.md` | Gerenciamento de provedores LLM |

## Enforcement Automatizado (CI)

Todo PR para `main` roda automaticamente:
- **TypeScript**: `tsc --noEmit`
- **ESLint** com `jsx-a11y`: detecta ARIA invalido, roles ausentes
- **Stylelint**: impede cores e font-sizes hardcoded (deve usar tokens do `theme.css`)
- **Vitest** com `axe-core`: testes de acessibilidade nos componentes UI

### O que o CI bloqueia
- Cores hardcoded (#hex, rgb, rgba) em CSS — use variaveis do tema
- Atributos ARIA invalidos — use os padroes documentados
- Componentes sem labels de acessibilidade
- Testes falhando (incluindo testes axe-core)

### Checklist para novo codigo
- [ ] Strings visiveis usam `t('key', 'fallback')` nos 3 locales
- [ ] Icones decorativos tem `aria-hidden="true"`
- [ ] Botoes icon-only tem `aria-label`
- [ ] Cores vem de variaveis CSS (`--bg-*`, `--text-*`, etc.)
- [ ] Font-sizes usam tokens (`--font-size-sm`, `--font-size-base`, etc.)
- [ ] Inputs/selects tem `height: 32px`, botoes tem `min-height: 36px`

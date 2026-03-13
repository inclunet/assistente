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

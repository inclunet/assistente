# Instruções para GitHub Copilot (assistente)

## Sistema de Temas e Cores (OBRIGATÓRIO)
- NUNCA use cores hardcoded (`#hex`, `rgb()`, `rgba()`, nomes de cor) em arquivos CSS.
- Todas as cores DEVEM vir das variáveis CSS definidas em `frontend/src/theme.css`.
- Variáveis principais: `--bg-base`, `--bg-surface`, `--bg-elevated`, `--bg-hover`, `--text-primary`, `--text-secondary`, `--text-muted`, `--border-subtle`, `--border-default`, `--accent`, `--color-success`, `--color-warning`, `--color-danger`, `--color-info`, `--focus-ring`.
- NUNCA use `@media (prefers-color-scheme: dark/light)` — o app usa `data-theme` no `<html>`.
- Use `var(--radius-sm)` / `var(--radius-md)` / `var(--radius-lg)` para border-radius.

## Acessibilidade (NUNCA negligenciar)
- Contraste mínimo: texto sobre fundo ≥ 4.5:1 (AA WCAG 2.1); preferir ≥ 7:1 (AAA).
- Todo elemento interativo deve ser acessível por teclado (Tab, Enter, ESC, setas).
- Usar `announce()` via `useAnnouncer` para feedback de ações para leitores de tela.
- Nunca usar apenas cor para transmitir informação — combinar com ícones ou texto.
- Usar componentes existentes (`DataGrid`, `Modal`, `Button`, `Toolbar`) que já têm ARIA correto.
- Labels obrigatórios: `htmlFor` em `<label>`, `aria-label` em botões sem texto visível.

## Logs (performance + ruído)
- Evite adicionar `console.log` em código de produção.
- Ao mexer em um recurso/arquivo, remova `console.log` existentes no trecho tocado.
- `console.warn` e `console.error` são permitidos quando representam condição anômala real.
- Se for indispensável logar para diagnóstico, prefira:
  - log temporário removido antes do merge; ou
  - log protegido por flag (ex.: `import.meta.env.DEV` + toggle em `localStorage`).

## Escopo de mudanças
- Não faça “varreduras globais” para remover logs em arquivos grandes sem necessidade do PR; aplique a limpeza incrementalmente quando o recurso for alterado.

## Execução (preferir VS Code Tasks)
- Para lint/build/test, prefira rodar **VS Code Tasks** ao invés de comandos via shell/terminal.
- Use `run_task`/`create_and_run_task` quando precisar executar algo repetível e seguro.
- Tasks já disponíveis em `.vscode/tasks.json` (use estas primeiro):
  - `Frontend: lint`
  - `Frontend: test`
  - `Frontend: build`
  - `Go: test ./...`
  - `Check: all (go test + frontend lint+test+build)`
- Só use shell/terminal quando não existir Task equivalente ou quando for necessário diagnosticar algo pontual.

## Testes (não deixar feature descoberta)
- Sempre verificar se a mudança/feature está coberta por testes.
  - Frontend: preferir **Vitest** para utilitários, hooks e componentes.
  - Backend: preferir `go test ./...` com testes de unidade/integração onde fizer sentido.
- Se não houver teste cobrindo o comportamento alterado (ou se a cobertura estiver fraca), criar/atualizar testes junto com a mudança.
- Ao adicionar testes, manter o escopo focado na feature tocada (evitar “mass refactor” de testes).
- Antes de concluir, rodar as tasks de teste relevantes (preferência por `Frontend: test` e/ou `Go: test ./...`).

## Testes (regras rígidas)
- Nunca deletar testes para “resolver” falhas.
- Nunca simplificar testes para facilitar aprovação; corrigir o código é sempre o caminho.
